package library

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"
)

// activityJob seeds a job by hand: status, dry run, size and creation time
// as given. showID or movieID is 0 for neither.
func activityJob(t *testing.T, db *DB, showID, movieID int64, hash, name string, size *int64, status JobStatus, dryRun bool, created time.Time) int64 {
	t.Helper()
	media, tmdb := "tv", 0
	var show, movie any
	if showID != 0 {
		show = showID
		_ = db.QueryRow(`SELECT tmdb_id FROM shows WHERE id = $1`, showID).Scan(&tmdb)
	}
	if movieID != 0 {
		media, movie = "movie", movieID
		_ = db.QueryRow(`SELECT tmdb_id FROM movies WHERE id = $1`, movieID).Scan(&tmdb)
	}
	var id int64
	if err := db.QueryRow(`
		INSERT INTO assignment_jobs (media_type, tmdb_id, show_id, movie_id, info_hash, magnet_uri, torrent_name, size_bytes, status, dry_run, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11) RETURNING id`,
		media, tmdb, show, movie, hash, "magnet:?xt=urn:btih:"+hash+"&dn=job-dn", name, size, status, dryRun, created).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

// activityAssign seeds a torrent assignment.
func activityAssign(t *testing.T, db *DB, itemType ItemType, itemID int64, hash, path string, active bool, created time.Time) {
	t.Helper()
	if _, err := db.Exec(`
		INSERT INTO torrent_assignments (item_type, item_id, info_hash, magnet_uri, file_path, file_size, is_active, created_at)
		VALUES ($1, $2, $3, $4, $5, 100, $6, $7)`,
		itemType, itemID, hash, "magnet:?xt=urn:btih:"+hash+"&dn=assigned-dn", path, active, created); err != nil {
		t.Fatal(err)
	}
}

func TestKnownTorrents(t *testing.T) {
	db := openSchemaDB(t)
	ctx := context.Background()
	repo := NewAssignmentJobRepository(db)
	h := func(c string) string { return strings.Repeat(c, 40) }
	pack, movieHash, jobOnly, dry, inactive, orphan := h("a"), h("b"), h("c"), h("d"), h("e"), h("f")
	t0 := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)

	show, eps := jobShow(t, db, 12)
	movie := seedMovie(t, db, "Known Movie", 2021)
	if _, err := db.Exec(`UPDATE shows SET poster_path = '/show.jpg' WHERE id = $1`, show.ID); err != nil {
		t.Fatal(err)
	}

	// The season pack: every episode, from an applied job whose size is
	// only in its proposals.
	for n := 1; n <= 12; n++ {
		activityAssign(t, db, ItemTypeEpisode, eps[n], pack, "Pack/E"+string(rune('a'+n))+".mkv", true, t0.Add(time.Duration(n)*time.Minute))
	}
	applied := activityJob(t, db, show.ID, 0, pack, "Jobs.Show.S01.1080p", nil, JobApplied, false, t0)
	if _, err := db.Exec(`
		INSERT INTO assignment_proposals (job_id, file_path, file_size, kind, review_group, suggested_source, source, decision)
		VALUES ($1, 'a.mkv', 700, 'episode', 'matched', 'rules', 'rules', 'accepted'),
		       ($1, 'b.mkv', 300, 'episode', 'matched', 'rules', 'rules', 'accepted')`, applied); err != nil {
		t.Fatal(err)
	}
	// A newer dry run that knows nothing yet: the applied job still names
	// it, and a dry run's title isn't one of the torrent's.
	activityJob(t, db, 0, movie.ID, pack, pack, nil, JobQueued, true, t0.Add(time.Hour))

	// The movie's torrent, with an open job for the show too.
	activityAssign(t, db, ItemTypeMovie, movie.ID, movieHash, "Movie.mkv", true, t0.Add(2*time.Hour))
	size := int64(5_000)
	open := activityJob(t, db, show.ID, 0, movieHash, "Movie.And.Show.Pack", &size, JobNeedsReview, false, t0.Add(3*time.Hour))

	activityJob(t, db, show.ID, 0, jobOnly, jobOnly, nil, JobQueued, false, t0.Add(4*time.Hour))
	activityJob(t, db, 0, movie.ID, dry, "Dry.Run", nil, JobReady, true, t0)       // dry runs don't count
	activityJob(t, db, 0, movie.ID, dry, "Done.Job", nil, JobApplied, false, t0)   // nor closed jobs
	activityAssign(t, db, ItemTypeMovie, movie.ID, inactive, "Old.mkv", false, t0) // nor inactive rows
	activityAssign(t, db, ItemTypeEpisode, 999_999, orphan, "Gone.mkv", true, t0)  // an episode that's gone: known, no title
	if _, err := db.Exec(`
		INSERT INTO torrent_assignments (item_type, item_id, kind, stream_ref, file_path, file_size, is_active)
		VALUES ('movie', $1, 'hdrezka', '{"provider":"hdrezka","path":"/x.html","post_id":7,"translator_id":56}', 'hdrezka/7/t56/movie/1080p.mp4', 1, TRUE)`,
		movie.ID); err != nil {
		t.Fatal(err)
	}

	all, err := repo.KnownTorrents(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	var hashes []string
	for _, k := range all {
		hashes = append(hashes, k.InfoHash)
	}
	if want := []string{pack, movieHash, jobOnly, orphan}; !slices.Equal(hashes, want) {
		t.Fatalf("known = %v, want %v", hashes, want)
	}

	p := all[0]
	if p.JobName != "Jobs.Show.S01.1080p" || p.SizeBytes == nil || *p.SizeBytes != 1_000 || p.OpenJob != nil ||
		!p.AddedAt.Equal(t0.Add(time.Minute)) || !strings.Contains(p.MagnetURI, "assigned-dn") {
		t.Errorf("pack = %+v", p)
	}
	if len(p.Titles) != 1 {
		t.Fatalf("pack titles = %+v", p.Titles)
	}
	st := p.Titles[0]
	if st.MediaType != JobTV || st.LibraryID != show.ID || st.TMDBID != show.TMDBID || st.Title != "Jobs Show" ||
		st.Year != 2020 || st.PosterPath != "/show.jpg" || len(st.Episodes) != 12 || st.SeasonSizes[1] != 12 ||
		st.Episodes[0] != (EpisodeNumber{Season: 1, Episode: 1}) || st.Episodes[11] != (EpisodeNumber{Season: 1, Episode: 12}) {
		t.Errorf("pack's show = %+v", st)
	}

	m := all[1]
	if m.JobName != "Movie.And.Show.Pack" || *m.SizeBytes != 5_000 || m.OpenJob == nil || m.OpenJob.ID != open ||
		m.OpenJob.Status != JobNeedsReview || !m.AddedAt.Equal(t0.Add(2*time.Hour)) {
		t.Errorf("movie torrent = %+v", m)
	}
	if len(m.Titles) != 2 || m.Titles[0].Title != "Jobs Show" || len(m.Titles[0].Episodes) != 0 ||
		m.Titles[1].MediaType != JobMovie || m.Titles[1].LibraryID != movie.ID || m.Titles[1].PosterPath != "" {
		t.Errorf("movie torrent's titles = %+v", m.Titles)
	}

	if j := all[2]; j.OpenJob == nil || j.JobName != jobOnly || j.SizeBytes != nil || len(j.Titles) != 1 ||
		!strings.Contains(j.MagnetURI, "job-dn") {
		t.Errorf("job only = %+v", j)
	}
	if o := all[3]; len(o.Titles) != 0 || o.JobName != "" {
		t.Errorf("orphan = %+v", o)
	}

	one, err := repo.KnownTorrents(ctx, movieHash)
	if err != nil || len(one) != 1 || one[0].InfoHash != movieHash || len(one[0].Titles) != 2 {
		t.Errorf("KnownTorrents(movie) = %+v, %v", one, err)
	}
	for _, unknown := range []string{dry, inactive, h("9")} {
		if got, err := repo.KnownTorrents(ctx, unknown); err != nil || len(got) != 0 {
			t.Errorf("KnownTorrents(%s…) = %+v, %v; want none", unknown[:4], got, err)
		}
	}
}

func TestTorrentPauses(t *testing.T) {
	db := openSchemaDB(t)
	ctx := context.Background()
	settings := NewSettingsRepository(db)
	a, b, c := strings.Repeat("a", 40), strings.Repeat("b", 40), strings.Repeat("c", 40)
	t1 := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	t2 := t1.Add(time.Hour)

	if got, err := settings.TorrentPauses(ctx); err != nil || len(got) != 0 {
		t.Fatalf("TorrentPauses before any = %v, %v", got, err)
	}
	// Another setting in the same document survives every write.
	if err := settings.Set(ctx, TorrentSettingsKey, []byte(`{"keep": 1}`)); err != nil {
		t.Fatal(err)
	}
	if err := settings.PauseTorrents(ctx, map[string]time.Time{a: t1, b: t1}); err != nil {
		t.Fatal(err)
	}
	// Pausing again keeps the first time; a new one is added.
	if err := settings.PauseTorrents(ctx, map[string]time.Time{a: t2, c: t2}); err != nil {
		t.Fatal(err)
	}
	got, err := settings.TorrentPauses(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || !got[a].Equal(t1) || !got[b].Equal(t1) || !got[c].Equal(t2) {
		t.Fatalf("pauses = %v", got)
	}

	if err := settings.ResumeTorrents(ctx, []string{a, strings.Repeat("9", 40)}); err != nil {
		t.Fatal(err)
	}
	if got, _ = settings.TorrentPauses(ctx); len(got) != 2 || !got[b].Equal(t1) {
		t.Fatalf("after resuming a = %v", got)
	}
	var keep int
	if err := db.QueryRow(`SELECT (value->>'keep')::int FROM app_settings WHERE key = $1`, TorrentSettingsKey).Scan(&keep); err != nil || keep != 1 {
		t.Errorf("the other setting = %d, %v", keep, err)
	}

	// Deleting a torrent clears its pause, in ForgetTorrent's transaction.
	forgotten, err := NewAssignmentJobRepository(db).ForgetTorrent(ctx, b)
	if err != nil || forgotten.Unpaused != 1 {
		t.Fatalf("ForgetTorrent = %+v, %v; want its pause cleared", forgotten, err)
	}
	if forgotten, _ = NewAssignmentJobRepository(db).ForgetTorrent(ctx, a); forgotten.Unpaused != 0 {
		t.Errorf("ForgetTorrent of an unpaused torrent cleared %d pauses", forgotten.Unpaused)
	}
	if got, _ = settings.TorrentPauses(ctx); len(got) != 1 || got[c].IsZero() {
		t.Fatalf("after forgetting b = %v", got)
	}
}
