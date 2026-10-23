package library

import (
	"context"
	"errors"
	"testing"
	"time"
)

// downloadsFixture is a private schema with a movie, and a show with a
// special and two seasons of two episodes.
type downloadsFixture struct {
	t        *testing.T
	db       *DB
	repo     *DownloadsRepository
	movie    *Movie
	show     *Show
	seasons  map[int]int64
	episodes map[[2]int]int64
}

func newDownloadsFixture(t *testing.T) *downloadsFixture {
	t.Helper()
	db := openSchemaDB(t)
	f := &downloadsFixture{t: t, db: db, repo: NewDownloadsRepository(db),
		seasons: map[int]int64{}, episodes: map[[2]int]int64{}}
	f.movie = seedMovie(t, db, "Interstellar", 2014)
	f.show = seedShow(t, db, "Severance", 2022)
	if _, err := db.Exec(`UPDATE shows SET status = 'Returning Series' WHERE id = $1`, f.show.ID); err != nil {
		t.Fatal(err)
	}
	for _, s := range []int{0, 1, 2} {
		var id int64
		if err := db.QueryRow(`INSERT INTO seasons (show_id, season_number) VALUES ($1, $2) RETURNING id`, f.show.ID, s).Scan(&id); err != nil {
			t.Fatal(err)
		}
		f.seasons[s] = id
		n := 2
		if s == 0 {
			n = 1
		}
		for e := 1; e <= n; e++ {
			var eid int64
			if err := db.QueryRow(`INSERT INTO episodes (season_id, episode_number, name, air_date, tmdb_episode_id)
				VALUES ($1, $2, 'Episode', '2025-01-01', $3) RETURNING id`, id, e, 9000+s*10+e).Scan(&eid); err != nil {
				t.Fatal(err)
			}
			f.episodes[[2]int{s, e}] = eid
		}
	}
	return f
}

func (f *downloadsFixture) exec(query string, args ...any) {
	f.t.Helper()
	if _, err := f.db.Exec(query, args...); err != nil {
		f.t.Fatalf("%s: %v", query, err)
	}
}

func (f *downloadsFixture) assign(itemType ItemType, id int64, hash, path string, size int64) {
	f.t.Helper()
	f.exec(`INSERT INTO torrent_assignments (item_type, item_id, info_hash, magnet_uri, file_path, file_size, is_active)
		VALUES ($1, $2, $3, $4, $5, $6, TRUE)`, itemType, id, hash, "magnet:?xt=urn:btih:"+hash, path, size)
}

// 010: one active download per target, soft releases, the checks.
func TestMigration010(t *testing.T) {
	f := newDownloadsFixture(t)
	ctx := context.Background()
	rec, err := f.repo.CreateDownload(ctx, DownloadRecord{Reason: DownloadKept, Scope: DownloadScopeMovie, MovieID: f.movie.ID, UntilWatched: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.repo.CreateDownload(ctx, DownloadRecord{Reason: DownloadKept, Scope: DownloadScopeMovie, MovieID: f.movie.ID}); !errors.Is(err, ErrDownloadExists) {
		t.Errorf("a second active download of the movie: %v", err)
	}
	ids, err := f.repo.ReleaseDownloads(ctx, []DownloadRelease{{ID: rec.ID, Reason: ReleaseDeleted}}, time.Now())
	if err != nil || len(ids) != 1 {
		t.Fatalf("release: %v %v", ids, err)
	}
	if _, err := f.repo.GetDownload(ctx, rec.ID); !errors.Is(err, ErrDownloadNotFound) {
		t.Errorf("a released download reads as active: %v", err)
	}
	if ids, _ := f.repo.ReleaseDownloads(ctx, []DownloadRelease{{ID: rec.ID, Reason: ReleaseExpired}}, time.Now()); len(ids) != 0 {
		t.Error("releasing twice released again")
	}
	if _, err := f.repo.CreateDownload(ctx, DownloadRecord{Reason: DownloadKept, Scope: DownloadScopeMovie, MovieID: f.movie.ID}); err != nil {
		t.Errorf("keeping it again after a release: %v", err)
	}
	for name, query := range map[string]string{
		"two targets":                `INSERT INTO downloads (reason, scope, movie_id, show_id, until_watched) VALUES ('kept', 'movie', $1, $2, TRUE)`,
		"a kept download's source":   `INSERT INTO downloads (reason, smart_source, scope, movie_id, until_watched) VALUES ('kept', 'up_next', 'movie', $1, TRUE)`,
		"a smart pick's no source":   `INSERT INTO downloads (reason, scope, movie_id, until_watched) VALUES ('smart', 'movie', $1, TRUE)`,
		"a release without reason":   `INSERT INTO downloads (reason, scope, movie_id, until_watched, released_at) VALUES ('kept', 'movie', $1, TRUE, NOW())`,
		"a local watch of two items": `INSERT INTO local_watches (movie_id, episode_id, watched_at) VALUES ($1, $2, NOW())`,
	} {
		args := []any{f.movie.ID}
		if name == "two targets" {
			args = append(args, f.show.ID)
		}
		if name == "a local watch of two items" {
			args = append(args, f.episodes[[2]int{1, 1}])
		}
		if _, err := f.db.Exec(query, args...); err == nil {
			t.Errorf("%s: no error", name)
		}
	}

	// Removing the title removes its downloads.
	f.exec(`DELETE FROM movies WHERE id = $1`, f.movie.ID)
	var n int
	if err := f.db.QueryRow(`SELECT count(*) FROM downloads WHERE movie_id = $1`, f.movie.ID).Scan(&n); err != nil || n != 0 {
		t.Errorf("downloads left after the movie went: %d %v", n, err)
	}
}

// The resolve: each download's items in episode order, their active files
// and their latest watch (Trakt by the matching rule, or local).
func TestDownloadsRepository_Resolve(t *testing.T) {
	f := newDownloadsFixture(t)
	ctx := context.Background()
	e11, e12, e21 := f.episodes[[2]int{1, 1}], f.episodes[[2]int{1, 2}], f.episodes[[2]int{2, 1}]
	f.assign(ItemTypeMovie, f.movie.ID, "aa", "Interstellar/Interstellar.mkv", 1000)
	f.assign(ItemTypeEpisode, e11, "bb", "S1/E1-E2.mkv", 300)
	f.assign(ItemTypeEpisode, e12, "bb", "S1/E1-E2.mkv", 300)
	f.exec(`INSERT INTO torrent_assignments (item_type, item_id, kind, stream_ref, file_path, file_size, is_active)
		VALUES ('episode', $1, 'hdrezka', '{"provider":"hdrezka"}', 'hdrezka/1/t1/s02e01/1080p.mp4', 0, TRUE)`, e21)
	// E1 watched on Trakt (by TMDB id), E2 here.
	watchedTrakt := time.Date(2026, 10, 1, 20, 0, 0, 0, time.UTC)
	f.exec(`INSERT INTO trakt_watched_episodes (member_id, show_tmdb_id, season, episode, episode_tmdb_id, plays, last_watched_at)
		VALUES (1, $1, 9, 9, 9011, 1, $2)`, f.show.TMDBID, watchedTrakt)
	watchedHere := time.Date(2026, 10, 2, 20, 0, 0, 0, time.UTC)
	if err := f.repo.UpsertLocalWatch(ctx, ItemRef{ItemTypeEpisode, e12}, watchedHere); err != nil {
		t.Fatal(err)
	}
	if err := f.repo.UpsertLocalWatch(ctx, ItemRef{ItemTypeEpisode, e12}, watchedHere.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}

	movie, _ := f.repo.CreateDownload(ctx, DownloadRecord{Reason: DownloadKept, Scope: DownloadScopeMovie, MovieID: f.movie.ID, UntilWatched: true})
	show, _ := f.repo.CreateDownload(ctx, DownloadRecord{Reason: DownloadKept, Scope: DownloadScopeShow, ShowID: f.show.ID})
	season, _ := f.repo.CreateDownload(ctx, DownloadRecord{Reason: DownloadKept, Scope: DownloadScopeSeason, SeasonID: f.seasons[1]})
	ep, err := f.repo.CreateDownload(ctx, DownloadRecord{Reason: DownloadSmart, SmartSource: SmartUpNext, Scope: DownloadScopeEpisode, EpisodeID: e21, UntilWatched: true})
	if err != nil {
		t.Fatal(err)
	}

	got, err := f.repo.ResolveDownloads(ctx)
	if err != nil {
		t.Fatal(err)
	}
	byID := map[int64]ResolvedDownload{}
	for _, d := range got {
		byID[d.ID] = d
	}
	if len(got) != 4 {
		t.Fatalf("%d downloads, want 4", len(got))
	}

	m := byID[movie.ID]
	if m.Title.MediaType != "movie" || m.Title.LibraryID != f.movie.ID || len(m.Items) != 1 ||
		m.Items[0].File == nil || m.Items[0].File.FileSize != 1000 || m.Items[0].WatchedAt != nil {
		t.Errorf("movie = %+v, items %+v", m.Title, m.Items)
	}
	s := byID[show.ID]
	if s.Title.MediaType != "tv" || s.Title.LibraryID != f.show.ID || s.Title.ShowStatus != "Returning Series" || len(s.Items) != 5 {
		t.Fatalf("show = %+v with %d items", s.Title, len(s.Items))
	}
	// Episode order, specials last.
	if s.Items[0].ItemID != e11 || s.Items[3].SeasonNumber != 2 || s.Items[4].SeasonNumber != 0 {
		t.Errorf("order: %+v", s.Items)
	}
	if w := s.Items[0].WatchedAt; w == nil || !w.Equal(watchedTrakt) {
		t.Errorf("E1's watch = %v, want Trakt's %v", w, watchedTrakt)
	}
	if w := s.Items[1].WatchedAt; w == nil || !w.Equal(watchedHere) {
		t.Errorf("E2's watch = %v, want the later local one %v", w, watchedHere)
	}
	if f3 := s.Items[2].File; f3 == nil || f3.Kind != AssignmentKindHDRezka {
		t.Errorf("S2E1's file = %+v, want HDRezka", f3)
	}
	sn := byID[season.ID]
	if sn.Title.SeasonNumber == nil || *sn.Title.SeasonNumber != 1 || !sn.Title.LaterSeason || len(sn.Items) != 2 || sn.Title.LibraryID != f.show.ID {
		t.Errorf("season = %+v with %d items", sn.Title, len(sn.Items))
	}
	e := byID[ep.ID]
	if e.SmartSource != SmartUpNext || e.Title.EpisodeNumber == nil || *e.Title.EpisodeNumber != 1 || len(e.Items) != 1 {
		t.Errorf("episode = %+v %+v", e.DownloadRecord, e.Title)
	}

	// A target that isn't a download yet resolves the same way.
	target, err := f.repo.ResolveTarget(ctx, DownloadTarget{Scope: DownloadScopeSeason, ID: f.show.ID, SeasonNumber: 1})
	if err != nil || target.SeasonID != f.seasons[1] || len(target.Items) != 2 || target.Title.Title != "Severance" {
		t.Errorf("ResolveTarget(season 1) = %+v, %v", target, err)
	}
	for _, tt := range []struct {
		target DownloadTarget
		want   error
	}{
		{DownloadTarget{Scope: DownloadScopeMovie, ID: 999999}, ErrMovieNotFound},
		{DownloadTarget{Scope: DownloadScopeShow, ID: 999999}, ErrShowNotFound},
		{DownloadTarget{Scope: DownloadScopeSeason, ID: f.show.ID, SeasonNumber: 7}, ErrSeasonNotFound},
		{DownloadTarget{Scope: DownloadScopeEpisode, ID: 999999}, ErrEpisodeNotFound},
	} {
		if _, err := f.repo.ResolveTarget(ctx, tt.target); !errors.Is(err, tt.want) {
			t.Errorf("ResolveTarget(%+v) = %v, want %v", tt.target, err, tt.want)
		}
	}

	// What a grouped file plays as, and episodes by number.
	items, err := f.repo.FileItems(ctx, "BB", "S1/E1-E2.mkv")
	if err != nil || len(items) != 2 {
		t.Errorf("FileItems = %v, %v", items, err)
	}
	if id, err := f.repo.EpisodeIDByNumber(ctx, f.show.ID, 1, 2); err != nil || id != e12 {
		t.Errorf("EpisodeIDByNumber = %d, %v", id, err)
	}
	if id, err := f.repo.EpisodeIDByNumber(ctx, f.show.ID, 5, 5); err != nil || id != 0 {
		t.Errorf("EpisodeIDByNumber(missing) = %d, %v", id, err)
	}

	// A smart pick turned kept; a kept download's duration.
	kept, err := f.repo.KeepSmartDownload(ctx, ep.ID, false, nil, time.Now())
	if err != nil || kept.Reason != DownloadKept || kept.SmartSource != "" || kept.UntilWatched {
		t.Errorf("KeepSmartDownload = %+v, %v", kept, err)
	}
	if _, err := f.repo.KeepSmartDownload(ctx, ep.ID, false, nil, time.Now()); !errors.Is(err, ErrDownloadNotFound) {
		t.Errorf("keeping a kept one as smart: %v", err)
	}
	expires := time.Now().Add(48 * time.Hour).UTC().Truncate(time.Second)
	updated, err := f.repo.UpdateDownloadDuration(ctx, movie.ID, DownloadDurationPatch{SetExpiresAt: true, ExpiresAt: &expires})
	if err != nil || updated.ExpiresAt == nil || !updated.ExpiresAt.Equal(expires) || !updated.UntilWatched {
		t.Errorf("UpdateDownloadDuration = %+v, %v", updated, err)
	}
}

// The smart re-pick rules: watched never, dismissed never, a lifetime
// expiry only after a later play of the show, an early release at once.
func TestDownloadsRepository_SmartPicks(t *testing.T) {
	f := newDownloadsFixture(t)
	ctx := context.Background()
	e11, e12, e21 := f.episodes[[2]int{1, 1}], f.episodes[[2]int{1, 2}], f.episodes[[2]int{2, 1}]
	now := time.Now().UTC()

	id, created, err := f.repo.UpsertSmartDownload(ctx, e11, SmartUpNext, now, now.Add(14*24*time.Hour))
	if err != nil || !created {
		t.Fatalf("UpsertSmartDownload = %d %v %v", id, created, err)
	}
	again, created, err := f.repo.UpsertSmartDownload(ctx, e11, SmartStreaming, now, now.Add(time.Hour))
	if err != nil || created || again != id {
		t.Errorf("a second pick of E1 = %d %v %v, want the first", again, created, err)
	}
	if _, _, err := f.repo.UpsertSmartDownload(ctx, 999999, SmartUpNext, now, now); !errors.Is(err, ErrEpisodeNotFound) {
		t.Errorf("a pick of no episode: %v", err)
	}

	check := func(episode int64, want string) {
		t.Helper()
		if got, err := f.repo.SmartPickBlocked(ctx, episode); err != nil || got != want {
			t.Errorf("SmartPickBlocked(%d) = %q, %v; want %q", episode, got, err, want)
		}
	}
	check(e11, "")
	// Dismissed: a DELETE of the pick.
	if _, err := f.repo.ReleaseDownloads(ctx, []DownloadRelease{{ID: id, Reason: ReleaseDeleted}}, now); err != nil {
		t.Fatal(err)
	}
	check(e11, "dismissed")
	// Released early (it stopped being a candidate): picked again at once.
	early, _, _ := f.repo.UpsertSmartDownload(ctx, e12, SmartUpNext, now, now.Add(time.Hour))
	if _, err := f.repo.ReleaseDownloads(ctx, []DownloadRelease{{ID: early, Reason: ReleaseExpired}}, now); err != nil {
		t.Fatal(err)
	}
	check(e12, "")
	// Its lifetime ran out, nothing played since: blocked; a play of the
	// show after: not.
	exp, _, _ := f.repo.UpsertSmartDownload(ctx, e12, SmartUpNext, now.Add(-time.Hour), now)
	if _, err := f.repo.ReleaseDownloads(ctx, []DownloadRelease{{ID: exp, Reason: ReleaseExpired}}, now); err != nil {
		t.Fatal(err)
	}
	check(e12, "expired")
	if err := f.repo.UpsertLocalWatch(ctx, ItemRef{ItemTypeEpisode, e11}, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	check(e12, "")
	// Watched (here) ever: never a candidate.
	check(e11, "watched")
	// Watched on Trakt, matched by season and number when TMDB ids are missing.
	f.exec(`UPDATE episodes SET tmdb_episode_id = NULL WHERE id = $1`, e21)
	f.exec(`INSERT INTO trakt_watched_episodes (member_id, show_tmdb_id, season, episode, plays, last_watched_at)
		VALUES (1, $1, 2, 1, 1, NOW())`, f.show.TMDBID)
	check(e21, "watched")
}

// Writing a torrent's file again keeps an active row's time (a download's
// admission order goes by it); one reactivated is stamped anew.
func TestAssignmentUpsert_KeepsAnActiveRowsTime(t *testing.T) {
	db := openSchemaDB(t)
	ctx := context.Background()
	show, eps := jobShow(t, db, 1)
	repo := NewAssignmentRepository(db)
	write := func() {
		t.Helper()
		if err := repo.WriteOverride(ctx, AssignmentWrite{ItemType: ItemTypeEpisode, InfoHash: "aa", MagnetURI: "magnet:?xt=urn:btih:aa",
			ShowID: show.ID, Rows: []TorrentAssignment{{ItemID: eps[1], FilePath: "S1/E1.mkv", FileSize: 10}}}); err != nil {
			t.Fatal(err)
		}
	}
	stamp := func() time.Time {
		t.Helper()
		var at time.Time
		if err := db.QueryRow(`SELECT created_at FROM torrent_assignments WHERE item_id = $1`, eps[1]).Scan(&at); err != nil {
			t.Fatal(err)
		}
		return at
	}
	write()
	first := stamp()
	time.Sleep(10 * time.Millisecond)
	write()
	if again := stamp(); !again.Equal(first) {
		t.Errorf("an active row's time moved from %v to %v", first, again)
	}
	if _, err := db.Exec(`UPDATE torrent_assignments SET is_active = FALSE WHERE item_id = $1`, eps[1]); err != nil {
		t.Fatal(err)
	}
	write()
	if re := stamp(); !re.After(first) {
		t.Errorf("a reactivated row kept its old time %v", re)
	}
}
