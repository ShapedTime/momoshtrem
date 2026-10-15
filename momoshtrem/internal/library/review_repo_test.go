package library

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

// seedJob creates a job with drafts as its proposals and the status.
func seedJob(t *testing.T, db *DB, job *AssignmentJob, status JobStatus, drafts []ProposalDraft) *AssignmentJob {
	t.Helper()
	repo := NewAssignmentJobRepository(db)
	ctx := context.Background()
	if _, err := repo.Create(ctx, job); err != nil {
		t.Fatal(err)
	}
	err := repo.Locked(ctx, job.ID, func(tx *JobTx) error {
		if err := tx.ReplaceProposals(drafts); err != nil {
			return err
		}
		tx.Job.Status = status
		return tx.SaveJob()
	})
	if err != nil {
		t.Fatal(err)
	}
	return job
}

func draft(path string, group ProposalGroup, decision Decision, episodes ...int64) ProposalDraft {
	kind := KindEpisode
	if group == GroupExtras {
		kind = KindExtra
	}
	return ProposalDraft{FilePath: path, FileSize: 1, Kind: kind, Group: group, EpisodeIDs: episodes,
		Source: SourceRules, Decision: decision}
}

// The review rules in SQL: pending files that map, and files in a
// conflict, of open, non-dry-run jobs past classification.
func TestReviewState(t *testing.T) {
	db := openSchemaDB(t)
	ctx := context.Background()
	repo := NewCatalogRepository(db)
	show, eps := jobShow(t, db, 5)
	movie := seedMovie(t, db, "Review Movie", 2020)

	sub := draft("Pack/E01.en.srt", GroupExtras, DecisionIgnored)
	sub.Kind, sub.LinkedPath = KindSubtitle, "Pack/E01.mkv"
	stray := draft("Pack/stray.en.srt", GroupExtras, DecisionPending, eps[5])
	stray.Kind = KindSubtitle
	older := seedJob(t, db, newShowJob(show, strings.Repeat("1", 40)), JobReady, []ProposalDraft{
		draft("Old/E01.mkv", GroupMatched, DecisionAccepted, eps[1]),
	})
	job := seedJob(t, db, newShowJob(show, strings.Repeat("2", 40)), JobNeedsReview, []ProposalDraft{
		draft("Pack/E01.mkv", GroupMatched, DecisionAccepted, eps[1]),              // accepted: fine
		draft("Pack/E02.mkv", GroupNeedsReview, DecisionPending, eps[2]),           // pending and maps
		draft("Pack/E03.mkv", GroupNeedsReview, DecisionAccepted, eps[3]),          // conflict
		draft("Pack/E03.Extended.mkv", GroupNeedsReview, DecisionAccepted, eps[3]), // conflict
		draft("Pack/E03.Ignored.mkv", GroupNeedsReview, DecisionIgnored, eps[3]),   // ignored: no conflict
		draft("Pack/E04E04.mkv", GroupMatched, DecisionAccepted, eps[4], eps[4]),   // one file twice isn't a conflict
		draft("Pack/Mystery.mkv", GroupUnmatched, DecisionPending),                 // doesn't map
		draft("Pack/NCOP.mkv", GroupExtras, DecisionIgnored),                       // an extra
		sub,   // follows E01
		stray, // a pending subtitle that maps
	})
	// Neither a job still classifying, nor a dry run, nor a closed job counts.
	seedJob(t, db, newShowJob(show, strings.Repeat("3", 40)), JobClassifying, []ProposalDraft{
		draft("Busy/E05.mkv", GroupNeedsReview, DecisionPending, eps[5])})
	dry := newShowJob(show, strings.Repeat("4", 40))
	dry.DryRun = true
	seedJob(t, db, dry, JobNeedsReview, []ProposalDraft{draft("Dry/E05.mkv", GroupNeedsReview, DecisionPending, eps[5])})
	seedJob(t, db, newShowJob(show, strings.Repeat("5", 40)), JobCancelled, []ProposalDraft{
		draft("Gone/E05.mkv", GroupNeedsReview, DecisionPending, eps[5])})

	state, err := repo.ReviewState(ctx, "tv", show.ID)
	if err != nil {
		t.Fatal(err)
	}
	if state.JobID == nil || *state.JobID != job.ID || state.NeedsReview != 4 ||
		!reflect.DeepEqual(state.EpisodeIDs, []int64{eps[2], eps[3], eps[5]}) {
		t.Errorf("state = job %v, %d files, episodes %v (want job %d, 4, %v)", state.JobID, state.NeedsReview, state.EpisodeIDs,
			job.ID, []int64{eps[2], eps[3], eps[5]})
	}
	if older.ID >= job.ID {
		t.Fatal("ids out of order")
	}

	movieJob := &AssignmentJob{MediaType: JobMovie, TMDBID: movie.TMDBID, MovieID: &movie.ID,
		InfoHash: strings.Repeat("6", 40), MagnetURI: "m", TorrentName: "Films"}
	candidate := func(path string) ProposalDraft {
		d := draft(path, GroupNeedsReview, DecisionPending)
		d.Kind = KindMovie
		return d
	}
	seedJob(t, db, movieJob, JobResolving, []ProposalDraft{candidate("Films/A.mkv"), candidate("Films/B.mkv")})
	state, err = repo.ReviewState(ctx, "movie", movie.ID)
	if err != nil || state.JobID == nil || *state.JobID != movieJob.ID || state.NeedsReview != 2 || state.EpisodeIDs != nil {
		t.Errorf("movie = %+v, %v", state, err)
	}

	empty := seedShow(t, db, "Quiet Show", 2021)
	if state, err := repo.ReviewState(ctx, "tv", empty.ID); err != nil || state.JobID != nil || state.NeedsReview != 0 || state.EpisodeIDs != nil {
		t.Errorf("no jobs = %+v, %v", state, err)
	}
	if _, err := repo.ReviewState(ctx, "music", 1); err == nil {
		t.Error("an unknown media type was accepted")
	}

	// /library carries the same counts and filters on them.
	for _, status := range []string{"", LibraryStatusNeedsReview} {
		items, total, err := repo.LibraryItems(ctx, LibraryQuery{Status: status, Today: "2026-01-01"})
		if err != nil {
			t.Fatal(err)
		}
		got := map[string]int{}
		for _, item := range items {
			got[item.Title] = item.NeedsReview
		}
		want := map[string]int{"Jobs Show": 4, "Review Movie": 2}
		if status == "" {
			want["Quiet Show"] = 0
		}
		if !reflect.DeepEqual(got, want) || total != len(want) {
			t.Errorf("status %q: %v (total %d), want %v", status, got, total, want)
		}
	}
}

func TestShowRepository_EpisodeList(t *testing.T) {
	db, counter := openTracedDB(t)
	ctx := context.Background()
	repo := NewShowRepository(db)
	show := seedShow(t, db, "Picker Show", 2020)
	ids := map[string]int64{}
	for _, s := range []struct {
		season int
		name   string
		eps    []int
	}{{2, "", []int{1}}, {0, "Specials", []int{2, 1}}, {1, "Book One", []int{1, 2}}, {3, "", nil}} {
		var seasonID int64
		if err := db.QueryRow(`INSERT INTO seasons (show_id, season_number, name) VALUES ($1, $2, NULLIF($3, '')) RETURNING id`,
			show.ID, s.season, s.name).Scan(&seasonID); err != nil {
			t.Fatal(err)
		}
		for _, n := range s.eps {
			var id int64
			if err := db.QueryRow(`INSERT INTO episodes (season_id, episode_number, name, air_date) VALUES ($1, $2, 'Ep', '2020-01-01') RETURNING id`,
				seasonID, n).Scan(&id); err != nil {
				t.Fatal(err)
			}
			ids[string(rune('0'+s.season))+string(rune('0'+n))] = id
		}
	}
	if _, err := db.Exec(`INSERT INTO torrent_assignments (item_type, item_id, info_hash, magnet_uri, file_path, file_size)
		VALUES ('episode', $1, 'h', 'm', 'f.mkv', 1)`, ids["12"]); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM torrent_assignments WHERE item_id = $1 AND info_hash = 'h'`, ids["12"])
	})

	before := counter.n.Load()
	seasons, err := repo.EpisodeList(ctx, show.ID)
	if n := counter.n.Load() - before; n != 1 {
		t.Errorf("queries = %d, want 1", n)
	}
	if err != nil {
		t.Fatal(err)
	}
	var shape []string
	for _, s := range seasons {
		row := s.Name + ":"
		for _, ep := range s.Episodes {
			row += string(rune('0' + ep.Number))
			if ep.HasFile {
				row += "*"
			}
		}
		shape = append(shape, row)
	}
	if want := []string{"Specials:12", "Book One:12*", ":1", ":"}; !reflect.DeepEqual(shape, want) {
		t.Errorf("seasons = %q, want %q", shape, want)
	}
	if seasons[0].Number != 0 || seasons[3].Number != 3 || seasons[3].Episodes == nil {
		t.Errorf("numbers = %+v", seasons)
	}

	if _, err := repo.EpisodeList(ctx, -1); !errors.Is(err, ErrShowNotFound) {
		t.Errorf("unknown show: %v", err)
	}
	if showID, err := repo.EpisodeShowID(ctx, ids["12"]); err != nil || showID != show.ID {
		t.Errorf("EpisodeShowID = %d, %v", showID, err)
	}
	if _, err := repo.EpisodeShowID(ctx, -1); !errors.Is(err, ErrEpisodeNotFound) {
		t.Errorf("unknown episode: %v", err)
	}
}

// A partial write touches only its items: the torrent's other rows on the
// title stay, the item's other rows go, and a pair it had is reactivated.
func TestAssignmentRepository_WriteOverride(t *testing.T) {
	db := openSchemaDB(t)
	ctx := context.Background()
	show, eps := jobShow(t, db, 2)
	row := func(item int64, path string, active bool) int64 {
		var id int64
		if err := db.QueryRow(`INSERT INTO torrent_assignments (item_type, item_id, info_hash, magnet_uri, file_path, file_size, is_active, match_source)
			VALUES ('episode', $1, 'h', 'm', $2, 1, $3, 'rules') RETURNING id`, item, path, active).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	e1 := row(eps[1], "f1.mkv", true)
	e2 := row(eps[2], "f2.mkv", true)
	old := row(eps[2], "f1.mkv", false)

	err := NewAssignmentRepository(db).WriteOverride(ctx, AssignmentWrite{
		ItemType: ItemTypeEpisode, ShowID: show.ID, InfoHash: "h", MagnetURI: "m2",
		Rows: []TorrentAssignment{{ItemID: eps[2], FilePath: "f1.mkv", FileSize: 5, MatchSource: SourceManual}},
	})
	if err != nil {
		t.Fatal(err)
	}
	state := map[int64]string{}
	rows, err := db.Query(`SELECT id, is_active, COALESCE(match_source, ''), magnet_uri FROM torrent_assignments WHERE info_hash = 'h'`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var active bool
		var source, magnet string
		if err := rows.Scan(&id, &active, &source, &magnet); err != nil {
			t.Fatal(err)
		}
		state[id] = map[bool]string{true: "on", false: "off"}[active] + " " + source + " " + magnet
	}
	want := map[int64]string{e1: "on rules m", e2: "off rules m", old: "on manual m2"}
	if !reflect.DeepEqual(state, want) {
		t.Errorf("rows = %v, want %v", state, want)
	}

	if _, err := db.Exec(`DELETE FROM shows WHERE id = $1`, show.ID); err != nil {
		t.Fatal(err)
	}
	err = NewAssignmentRepository(db).WriteOverride(ctx, AssignmentWrite{ItemType: ItemTypeEpisode, ShowID: show.ID, InfoHash: "h",
		Rows: []TorrentAssignment{{ItemID: eps[1], FilePath: "f2.mkv"}}})
	if !errors.Is(err, ErrShowNotFound) {
		t.Errorf("a deleted show: %v", err)
	}
}

func TestTorrentFileSources(t *testing.T) {
	db := openSchemaDB(t)
	ctx := context.Background()
	repo := NewAssignmentJobRepository(db)
	show, eps := jobShow(t, db, 3)
	movie := seedMovie(t, db, "Mapped Movie", 2020)
	hash := strings.Repeat("7", 40)

	if got, err := repo.StoredTorrent(ctx, hash); got != nil || err != nil {
		t.Errorf("no job = %+v, %v", got, err)
	}
	if got, err := repo.TorrentMagnet(ctx, hash); got != "" || err != nil {
		t.Errorf("no magnet = %q, %v", got, err)
	}

	job := newShowJob(show, hash)
	job.TorrentName, job.MagnetURI = "Pack", "magnet:job"
	seedJob(t, db, job, JobApplied, []ProposalDraft{
		draft("Pack/b.mkv", GroupMatched, DecisionAccepted, eps[1]),
		draft("Pack/a.mkv", GroupMatched, DecisionAccepted, eps[2]),
	})
	// A newer job without proposals yet doesn't hide the list.
	if _, err := repo.Create(ctx, newShowJob(show, hash)); err != nil {
		t.Fatal(err)
	}
	got, err := repo.StoredTorrent(ctx, hash)
	if err != nil || got == nil || got.Name != "Pack" || got.MagnetURI != "magnet:job" ||
		!reflect.DeepEqual(got.Files, []TorrentFile{{"Pack/b.mkv", 1}, {"Pack/a.mkv", 1}}) {
		t.Errorf("stored = %+v, %v", got, err)
	}
	if m, err := repo.TorrentMagnet(ctx, hash); m != "magnet:?xt=urn:btih:"+hash || err != nil {
		t.Errorf("magnet from the newest job = %q, %v", m, err)
	}

	// Assignments come first, the active one first.
	for _, r := range []struct {
		item   int64
		typ    string
		path   string
		magnet string
		active bool
	}{
		{eps[3], "episode", "Pack/old.mkv", "magnet:inactive", false},
		{eps[1], "episode", "Pack/b.mkv", "magnet:active", true},
		{eps[2], "episode", "Pack/b.mkv", "magnet:active", true},
		{movie.ID, "movie", "Pack/a.mkv", "magnet:active", true},
		{999_999, "episode", "Pack/a.mkv", "magnet:active", true}, // an episode that's gone
	} {
		if _, err := db.Exec(`INSERT INTO torrent_assignments (item_type, item_id, info_hash, magnet_uri, file_path, file_size, is_active)
			VALUES ($1, $2, $3, $4, $5, 1, $6)`, r.typ, r.item, hash, r.magnet, r.path, r.active); err != nil {
			t.Fatal(err)
		}
	}
	if m, err := repo.TorrentMagnet(ctx, hash); m != "magnet:active" || err != nil {
		t.Errorf("magnet = %q, %v", m, err)
	}

	mappings, err := repo.ActiveMappings(ctx, hash)
	if err != nil {
		t.Fatal(err)
	}
	var shape []string
	for _, m := range mappings {
		switch {
		case m.MovieID != nil:
			shape = append(shape, m.FilePath+" movie")
		default:
			shape = append(shape, m.FilePath+" "+m.Episode.Name)
		}
	}
	if want := []string{"Pack/a.mkv movie", "Pack/b.mkv Episode 1", "Pack/b.mkv Episode 2"}; !reflect.DeepEqual(shape, want) {
		t.Errorf("mappings = %q, want %q", shape, want)
	}
}
