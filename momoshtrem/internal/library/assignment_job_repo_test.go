package library

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

// openSchemaDB opens the test database in a private schema, migrated from
// scratch. Claim and Requeue work on every job, so tests that use them
// mustn't share jobs with other packages running at the same time.
func openSchemaDB(t *testing.T) *DB {
	t.Helper()
	base := openTestDB(t)
	schema := fmt.Sprintf("jobs_lib_%d", time.Now().UnixNano())
	if _, err := base.Exec(`CREATE SCHEMA ` + schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = base.Exec(`DROP SCHEMA ` + schema + ` CASCADE`) })
	url := os.Getenv("MOMOSHTREM_TEST_DATABASE_URL")
	sep := "?"
	if strings.Contains(url, "?") {
		sep = "&"
	}
	db, err := NewDB(url + sep + "search_path=" + schema)
	if err != nil {
		t.Fatalf("NewDB in %s: %v", schema, err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// jobShow seeds a show with season 1 episodes 1..n and returns it and the
// episode ids by number.
func jobShow(t *testing.T, db *DB, n int) (*Show, map[int]int64) {
	t.Helper()
	show := seedShow(t, db, "Jobs Show", 2020)
	var seasonID int64
	if err := db.QueryRow(`INSERT INTO seasons (show_id, season_number) VALUES ($1, 1) RETURNING id`, show.ID).Scan(&seasonID); err != nil {
		t.Fatal(err)
	}
	ids := map[int]int64{}
	for e := 1; e <= n; e++ {
		var id int64
		if err := db.QueryRow(`INSERT INTO episodes (season_id, episode_number, name) VALUES ($1, $2, $3) RETURNING id`,
			seasonID, e, fmt.Sprintf("Episode %d", e)).Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids[e] = id
	}
	return show, ids
}

func newShowJob(show *Show, hash string) *AssignmentJob {
	return &AssignmentJob{MediaType: JobTV, TMDBID: show.TMDBID, ShowID: &show.ID,
		InfoHash: hash, MagnetURI: "magnet:?xt=urn:btih:" + hash, TorrentName: hash}
}

// 004 adds its tables and columns and keeps one open job per title and
// torrent.
func TestMigration004(t *testing.T) {
	db := openSchemaDB(t)
	for _, table := range []string{"assignment_jobs", "assignment_proposals", "ai_usage", "ai_cache"} {
		var exists bool
		if err := db.QueryRow(`SELECT to_regclass($1) IS NOT NULL`, table).Scan(&exists); err != nil || !exists {
			t.Errorf("table %s: %v %v", table, exists, err)
		}
	}
	var cols int
	if err := db.QueryRow(`SELECT count(*) FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name = 'torrent_assignments' AND column_name IN ('confidence', 'match_source')`).Scan(&cols); err != nil || cols != 2 {
		t.Errorf("torrent_assignments columns = %d, %v", cols, err)
	}
	if _, err := db.Exec(`INSERT INTO torrent_assignments (item_type, item_id, info_hash, magnet_uri, file_path, file_size, match_source)
		VALUES ('episode', 1, 'h', 'm', 'p', 1, 'robots')`); err == nil {
		t.Error("an unknown match_source was accepted")
	}

	show, _ := jobShow(t, db, 1)
	job := newShowJob(show, strings.Repeat("a", 40))
	if _, err := NewAssignmentJobRepository(db).Create(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	_, err := db.Exec(`INSERT INTO assignment_jobs (media_type, tmdb_id, show_id, info_hash, magnet_uri, torrent_name)
		VALUES ('tv', $1, $2, $3, 'm', 'n')`, show.TMDBID, show.ID, job.InfoHash)
	if err == nil || !strings.Contains(err.Error(), "assignment_jobs_open_key") {
		t.Errorf("a second open job for the same torrent: %v", err)
	}
	if _, err := db.Exec(`UPDATE assignment_jobs SET status = 'done' WHERE id = $1`, job.ID); err == nil {
		t.Error("an unknown status was accepted")
	}
	// Deleting the show deletes its jobs.
	if _, err := db.Exec(`DELETE FROM shows WHERE id = $1`, show.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := NewAssignmentJobRepository(db).Get(context.Background(), job.ID); !errors.Is(err, ErrJobNotFound) {
		t.Errorf("job after its show was deleted: %v", err)
	}
	if err := db.Migrate(context.Background()); err != nil {
		t.Errorf("migrate again: %v", err)
	}
}

func TestJobRepository_CreateFindsTheOpenJob(t *testing.T) {
	db := openSchemaDB(t)
	ctx := context.Background()
	repo := NewAssignmentJobRepository(db)
	show, _ := jobShow(t, db, 1)
	hash := strings.Repeat("b", 40)
	season := 2

	first := newShowJob(show, hash)
	first.Hint = &JobHint{Season: &season, Text: "absolute"}
	first.Indexer = "Nyaa"
	if created, err := repo.Create(ctx, first); err != nil || !created {
		t.Fatalf("first = %v, %v", created, err)
	}
	again := newShowJob(show, hash)
	if created, err := repo.Create(ctx, again); err != nil || created || again.ID != first.ID {
		t.Fatalf("again = %v (id %d, want %d), %v", created, again.ID, first.ID, err)
	}

	got, err := repo.Get(ctx, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != JobQueued || got.Title != "Jobs Show" || got.Year != 2020 || got.Indexer != "Nyaa" ||
		got.Hint == nil || *got.Hint.Season != 2 || got.Hint.Text != "absolute" || got.FileCount != nil ||
		*got.ShowID != show.ID || got.MovieID != nil || got.SeasonCount != 1 || got.EpisodeCount != 1 {
		t.Errorf("job = %+v", got)
	}

	// A dry run is never the open job; a closed job isn't either.
	dry := newShowJob(show, hash)
	dry.DryRun = true
	if created, err := repo.Create(ctx, dry); err != nil || !created || dry.ID == first.ID {
		t.Errorf("dry run = %v (%d), %v", created, dry.ID, err)
	}
	if ok, err := repo.Cancel(ctx, first.ID); err != nil || !ok {
		t.Fatalf("cancel = %v, %v", ok, err)
	}
	if ok, _ := repo.Cancel(ctx, first.ID); ok {
		t.Error("cancelled twice")
	}
	next := newShowJob(show, hash)
	if created, err := repo.Create(ctx, next); err != nil || !created || next.ID == first.ID {
		t.Errorf("after cancel = %v (%d), %v", created, next.ID, err)
	}

	jobs, err := repo.List(ctx, OpenJobStatuses, 10)
	if err != nil || len(jobs) != 1 || jobs[0].ID != next.ID {
		t.Errorf("open jobs = %+v, %v (no dry runs, no closed jobs)", jobs, err)
	}
}

// Workers never claim the same job.
func TestJobRepository_ClaimRequeue(t *testing.T) {
	db := openSchemaDB(t)
	ctx := context.Background()
	repo := NewAssignmentJobRepository(db)
	show, _ := jobShow(t, db, 1)
	for i := 0; i < 6; i++ {
		if _, err := repo.Create(ctx, newShowJob(show, fmt.Sprintf("%040d", i))); err != nil {
			t.Fatal(err)
		}
	}

	var mu sync.Mutex
	claimed := map[int64]int{}
	var wg sync.WaitGroup
	for w := 0; w < 4; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				job, err := repo.Claim(ctx)
				if errors.Is(err, ErrJobNotFound) {
					return
				}
				if err != nil {
					t.Error(err)
					return
				}
				if job.Status != JobFetchingMetadata {
					t.Errorf("claimed job status = %s", job.Status)
				}
				mu.Lock()
				claimed[job.ID]++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if len(claimed) != 6 {
		t.Errorf("claimed %d jobs, want 6", len(claimed))
	}
	for id, n := range claimed {
		if n != 1 {
			t.Errorf("job %d claimed %d times", id, n)
		}
	}

	if n, err := repo.Requeue(ctx); err != nil || n != 6 {
		t.Errorf("requeue = %d, %v", n, err)
	}
	if _, err := repo.Claim(ctx); err != nil {
		t.Errorf("claim after requeue: %v", err)
	}
}

func TestJobTx_ReplaceProposals(t *testing.T) {
	db := openSchemaDB(t)
	ctx := context.Background()
	repo := NewAssignmentJobRepository(db)
	show, eps := jobShow(t, db, 3)
	job := newShowJob(show, strings.Repeat("c", 40))
	if _, err := repo.Create(ctx, job); err != nil {
		t.Fatal(err)
	}
	drafts := []ProposalDraft{
		{FilePath: "P/E01E02.mkv", FileSize: 2, Kind: KindEpisode, Group: GroupMatched, EpisodeIDs: []int64{eps[1], eps[2]}, Confidence: "high", Source: "rules", Reason: "r1", Decision: DecisionAccepted},
		{FilePath: "P/E01E02.en.srt", FileSize: 1, Kind: KindSubtitle, Group: GroupExtras, Source: "rules", Decision: DecisionIgnored, LinkedPath: "P/E01E02.mkv"},
		{FilePath: "P/E03.mkv", Kind: KindExtra, Group: GroupExtras, ExtraType: "superseded", Source: "rules", Decision: DecisionIgnored, SupersededByPath: "P/E03v2.mkv"},
		{FilePath: "P/E03v2.mkv", Kind: KindEpisode, Group: GroupNeedsReview, EpisodeIDs: []int64{eps[3]}, Confidence: "medium", Source: "ai", Decision: DecisionPending},
	}
	for round := 0; round < 2; round++ { // replacing works the same the second time
		err := repo.Locked(ctx, job.ID, func(tx *JobTx) error { return tx.ReplaceProposals(drafts) })
		if err != nil {
			t.Fatal(err)
		}
	}
	byJob, err := repo.Proposals(ctx, []int64{job.ID})
	if err != nil {
		t.Fatal(err)
	}
	ps := byJob[job.ID]
	if len(ps) != 4 {
		t.Fatalf("proposals = %d", len(ps))
	}
	video, sub, v1, v2 := ps[0], ps[1], ps[2], ps[3]
	if !reflect.DeepEqual(video.EpisodeIDs, []int64{eps[1], eps[2]}) || !reflect.DeepEqual(video.SuggestedEpisodeIDs, video.EpisodeIDs) ||
		video.SuggestedConfidence != "high" || video.SuggestedReason != "r1" || video.FirstSeason == nil || *video.FirstSeason != 1 {
		t.Errorf("video = %+v", video)
	}
	if sub.LinkedProposalID == nil || *sub.LinkedProposalID != video.ID || sub.Listed() {
		t.Errorf("subtitle link = %v", sub.LinkedProposalID)
	}
	if v1.SupersededBy == nil || *v1.SupersededBy != v2.ID || v1.ExtraType != "superseded" || v1.EpisodeIDs != nil {
		t.Errorf("superseded = %+v", v1)
	}
	if v2.Source != "ai" || v2.Confidence != "medium" || v2.Decision != DecisionPending {
		t.Errorf("v2 = %+v", v2)
	}

	// Updates write the mutable fields; the job's own fields save too.
	err = repo.Locked(ctx, job.ID, func(tx *JobTx) error {
		p := *tx.Proposals[3]
		p.Decision, p.UserDecided, p.EpisodeIDs, p.Confidence, p.Source = DecisionAccepted, true, []int64{eps[3], eps[2]}, "", "manual"
		if err := tx.UpdateProposals([]*AssignmentProposal{&p}); err != nil {
			return err
		}
		n := 4
		tx.Job.Status, tx.Job.VideoCount, tx.Job.SuggestedHints, tx.Job.AIError = JobNeedsReview, &n, []string{"Numbers are absolute"}, "oops"
		return tx.SaveJob()
	})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := repo.Get(ctx, job.ID)
	if got.Status != JobNeedsReview || *got.VideoCount != 4 || !reflect.DeepEqual(got.SuggestedHints, []string{"Numbers are absolute"}) || got.AIError != "oops" {
		t.Errorf("job = %+v", got)
	}
	byJob, _ = repo.Proposals(ctx, []int64{job.ID})
	if p := byJob[job.ID][3]; p.Decision != DecisionAccepted || !p.UserDecided || p.Confidence != "" || p.Source != "manual" || len(p.EpisodeIDs) != 2 {
		t.Errorf("updated = %+v", p)
	}

	if err := repo.Locked(ctx, 999999, func(*JobTx) error { return nil }); !errors.Is(err, ErrJobNotFound) {
		t.Errorf("unknown job: %v", err)
	}
}

func activeRows(t *testing.T, db *DB, episodeID int64) []*TorrentAssignment {
	t.Helper()
	rows, err := db.Query(`SELECT `+assignmentSelect+` FROM torrent_assignments
		WHERE item_type = 'episode' AND item_id = $1 AND is_active ORDER BY id`, episodeID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []*TorrentAssignment
	for rows.Next() {
		a, err := scanAssignment(rows)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, a)
	}
	return out
}

// Apply's writer: rows upserted (a file the episode had before comes back
// instead of failing), the episodes' other rows deactivated, and the
// torrent's rows the job no longer maps deactivated.
func TestJobTx_WriteAssignments(t *testing.T) {
	db := openSchemaDB(t)
	ctx := context.Background()
	repo := NewAssignmentJobRepository(db)
	show, eps := jobShow(t, db, 3)
	hash, other := strings.Repeat("d", 40), strings.Repeat("e", 40)
	job := newShowJob(show, hash)
	if _, err := repo.Create(ctx, job); err != nil {
		t.Fatal(err)
	}
	// E03 has a file from another torrent.
	seedAssignment(t, db, eps[3], "Other/E03.mkv", true)
	if _, err := db.Exec(`UPDATE torrent_assignments SET info_hash = $1 WHERE item_id = $2`, other, eps[3]); err != nil {
		t.Fatal(err)
	}

	write := func(rows ...TorrentAssignment) {
		t.Helper()
		err := repo.Locked(ctx, job.ID, func(tx *JobTx) error {
			return tx.WriteAssignments(AssignmentWrite{ItemType: ItemTypeEpisode, InfoHash: hash,
				MagnetURI: job.MagnetURI, ShowID: show.ID, Rows: rows})
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	row := func(ep int64, path, conf string) TorrentAssignment {
		return TorrentAssignment{ItemID: ep, FilePath: path, FileSize: 7, Resolution: "1080p", Confidence: conf, MatchSource: "rules"}
	}

	write(row(eps[1], "P/E01E02.mkv", "high"), row(eps[2], "P/E01E02.mkv", "high"), row(eps[3], "P/E03.mkv", "medium"))
	for n, path := range map[int]string{1: "P/E01E02.mkv", 2: "P/E01E02.mkv", 3: "P/E03.mkv"} {
		rows := activeRows(t, db, eps[n])
		if len(rows) != 1 || rows[0].FilePath != path || rows[0].MatchSource != "rules" || rows[0].InfoHash != hash {
			t.Errorf("E%02d active = %+v", n, rows)
		}
	}
	if rows := activeRows(t, db, eps[3]); rows[0].Confidence != "medium" || rows[0].Resolution != "1080p" {
		t.Errorf("E03 = %+v", rows[0])
	}

	// Re-applying the torrent with E02 dropped: E02's row goes, the others
	// are reactivated in place (no unique violation).
	write(row(eps[1], "P/E01E02.mkv", "high"), row(eps[3], "P/E03.mkv", "high"))
	if rows := activeRows(t, db, eps[2]); len(rows) != 0 {
		t.Errorf("E02 still active: %+v", rows)
	}
	if rows := activeRows(t, db, eps[3]); len(rows) != 1 || rows[0].Confidence != "high" {
		t.Errorf("E03 = %+v", rows)
	}
	var total int
	if err := db.QueryRow(`SELECT count(*) FROM torrent_assignments WHERE info_hash = $1`, hash).Scan(&total); err != nil || total != 3 {
		t.Errorf("rows for the torrent = %d, %v (upserted, not inserted again)", total, err)
	}

	// A movie write is authoritative for the movie alone.
	movie := seedMovie(t, db, "Film", 2014)
	err := repo.Locked(ctx, job.ID, func(tx *JobTx) error {
		return tx.WriteAssignments(AssignmentWrite{ItemType: ItemTypeMovie, InfoHash: hash, MagnetURI: job.MagnetURI,
			MovieID: movie.ID, Rows: []TorrentAssignment{{ItemID: movie.ID, FilePath: "P/Film.mkv", FileSize: 9}}})
	})
	if err != nil {
		t.Fatal(err)
	}
	if rows := activeRows(t, db, eps[1]); len(rows) != 1 {
		t.Errorf("a movie write touched the show: %+v", rows)
	}

	err = repo.Locked(ctx, job.ID, func(tx *JobTx) error {
		return tx.WriteAssignments(AssignmentWrite{ItemType: ItemTypeEpisode, InfoHash: hash, ShowID: show.ID,
			Rows: []TorrentAssignment{row(eps[1], "a", ""), row(eps[1], "b", "")}})
	})
	if err == nil {
		t.Error("two rows for one episode were written")
	}
}

// Two jobs of one show, different torrents, applying the same episode at
// once: under READ COMMITTED neither sees the other's insert, so without
// the title lock both rows stayed active.
func TestJobTx_WriteAssignmentsConcurrent(t *testing.T) {
	db := openSchemaDB(t)
	ctx := context.Background()
	repo := NewAssignmentJobRepository(db)
	show, eps := jobShow(t, db, 1)
	movie := seedMovie(t, db, "Film", 2014)

	type writer struct {
		job   *AssignmentJob
		write AssignmentWrite
	}
	var episodeWriters, movieWriters []writer
	for _, hash := range []string{strings.Repeat("1", 40), strings.Repeat("2", 40)} {
		job := newShowJob(show, hash)
		if _, err := repo.Create(ctx, job); err != nil {
			t.Fatal(err)
		}
		episodeWriters = append(episodeWriters, writer{job, AssignmentWrite{ItemType: ItemTypeEpisode, InfoHash: hash,
			MagnetURI: job.MagnetURI, ShowID: show.ID, Rows: []TorrentAssignment{{ItemID: eps[1], FilePath: hash + "/E01.mkv", FileSize: 1}}}})
		movieWriters = append(movieWriters, writer{job, AssignmentWrite{ItemType: ItemTypeMovie, InfoHash: hash,
			MagnetURI: job.MagnetURI, MovieID: movie.ID, Rows: []TorrentAssignment{{ItemID: movie.ID, FilePath: hash + "/Film.mkv", FileSize: 1}}}})
	}

	for _, tt := range []struct {
		name     string
		itemType ItemType
		itemID   int64
		writers  []writer
	}{
		{"episode", ItemTypeEpisode, eps[1], episodeWriters},
		{"movie", ItemTypeMovie, movie.ID, movieWriters},
	} {
		for round := 0; round < 20; round++ {
			start := make(chan struct{})
			var wg sync.WaitGroup
			for _, w := range tt.writers {
				wg.Add(1)
				go func(w writer) {
					defer wg.Done()
					<-start
					err := repo.Locked(ctx, w.job.ID, func(tx *JobTx) error { return tx.WriteAssignments(w.write) })
					if err != nil {
						t.Error(err)
					}
				}(w)
			}
			close(start)
			wg.Wait()
			var active int
			if err := db.QueryRow(`SELECT count(*) FROM torrent_assignments WHERE item_type = $1 AND item_id = $2 AND is_active`,
				tt.itemType, tt.itemID).Scan(&active); err != nil {
				t.Fatal(err)
			}
			if active != 1 {
				t.Fatalf("%s, round %d: %d active rows, want 1", tt.name, round, active)
			}
		}
	}

	// A title that left the library can't be written to.
	err := repo.Locked(ctx, episodeWriters[0].job.ID, func(tx *JobTx) error {
		w := episodeWriters[0].write
		w.ShowID = -1
		return tx.WriteAssignments(w)
	})
	if !errors.Is(err, ErrShowNotFound) {
		t.Errorf("unknown show: %v", err)
	}
}

func TestJobRepository_FailCancelTorrentInUse(t *testing.T) {
	db := openSchemaDB(t)
	ctx := context.Background()
	repo := NewAssignmentJobRepository(db)
	show, eps := jobShow(t, db, 1)
	hash := strings.Repeat("f", 40)
	job := newShowJob(show, hash)
	if _, err := repo.Create(ctx, job); err != nil {
		t.Fatal(err)
	}

	if used, err := repo.TorrentInUse(ctx, hash, job.ID); err != nil || used {
		t.Errorf("in use by itself only = %v, %v", used, err)
	}
	if used, _ := repo.TorrentInUse(ctx, hash, 0); !used {
		t.Error("an open job doesn't count")
	}
	if ok, err := repo.Fail(ctx, job.ID, "metadata_timeout", "No peers"); err != nil || !ok {
		t.Fatalf("fail = %v, %v", ok, err)
	}
	if ok, _ := repo.Fail(ctx, job.ID, "internal", "again"); ok {
		t.Error("a failed job failed again")
	}
	if ok, _ := repo.Cancel(ctx, job.ID); ok {
		t.Error("a failed job was cancelled")
	}
	got, _ := repo.Get(ctx, job.ID)
	if got.Status != JobFailed || got.Error != "No peers" || got.ErrorCode != "metadata_timeout" {
		t.Errorf("job = %+v", got)
	}
	if used, _ := repo.TorrentInUse(ctx, hash, 0); used {
		t.Error("a failed job counts")
	}
	seedAssignment(t, db, eps[1], "x.mkv", true)
	if _, err := db.Exec(`UPDATE torrent_assignments SET info_hash = $1 WHERE item_id = $2`, hash, eps[1]); err != nil {
		t.Fatal(err)
	}
	if used, _ := repo.TorrentInUse(ctx, hash, 0); !used {
		t.Error("an active assignment doesn't count")
	}

	if _, err := db.Exec(`UPDATE assignment_jobs SET dry_run = TRUE, created_at = NOW() - interval '2 days' WHERE id = $1`, job.ID); err != nil {
		t.Fatal(err)
	}
	if n, err := repo.DeleteDryRuns(ctx, 24*time.Hour); err != nil || n != 1 {
		t.Errorf("deleted dry runs = %d, %v", n, err)
	}
}
