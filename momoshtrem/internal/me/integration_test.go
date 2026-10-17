package me

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/shapedtime/momoshtrem/internal/catalog"
	"github.com/shapedtime/momoshtrem/internal/library"
)

// queryCounter counts the queries pgx runs.
type queryCounter struct{ n atomic.Int32 }

func (q *queryCounter) TraceQueryStart(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryStartData) context.Context {
	q.n.Add(1)
	return ctx
}

func (q *queryCounter) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

// openSchemaDB opens MOMOSHTREM_TEST_DATABASE_URL in a private schema,
// migrated from scratch (the Trakt tables hold one account, so tests don't
// share them), with a query counter; it skips without one.
func openSchemaDB(t *testing.T) (*library.DB, *queryCounter) {
	t.Helper()
	url := os.Getenv("MOMOSHTREM_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("MOMOSHTREM_TEST_DATABASE_URL not set; skipping Postgres integration test")
	}
	base, err := library.NewDB(url)
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	t.Cleanup(func() { base.Close() })
	schema := fmt.Sprintf("me_%d", time.Now().UnixNano())
	if _, err := base.Exec(`CREATE SCHEMA ` + schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = base.Exec(`DROP SCHEMA ` + schema + ` CASCADE`) })
	sep := "?"
	if strings.Contains(url, "?") {
		sep = "&"
	}
	url += sep + "search_path=" + schema
	migrated, err := library.NewDB(url)
	if err != nil {
		t.Fatalf("NewDB in %s: %v", schema, err)
	}
	migrated.Close()

	cfg, err := pgx.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	counter := &queryCounter{}
	cfg.Tracer = counter
	db := library.NewDBFromSQL(stdlib.OpenDB(*cfg))
	t.Cleanup(func() { db.Close() })
	return db, counter
}

type seeder struct {
	t  *testing.T
	db *library.DB
}

func (s seeder) exec(query string, args ...any) {
	s.t.Helper()
	if _, err := s.db.Exec(query, args...); err != nil {
		s.t.Fatalf("%s: %v", query, err)
	}
}

// show adds a library show and its seasons, each episode airing on the
// given dates; it returns the episode ids by "SxE".
func (s seeder) show(tmdbID int, title string, seasons map[int][]string) (*library.Show, map[string]int64) {
	s.t.Helper()
	ctx := context.Background()
	show := &library.Show{TMDBID: tmdbID, Title: title, Year: 2020}
	if err := library.NewShowRepository(s.db).Create(show); err != nil {
		s.t.Fatal(err)
	}
	meta := library.NewMetadataRepository(s.db)
	ids := map[string]int64{}
	for n, dates := range seasons {
		row := &library.Season{ShowID: show.ID, SeasonNumber: n}
		if _, err := meta.UpsertSeason(ctx, row); err != nil {
			s.t.Fatal(err)
		}
		eps := make([]library.EpisodeMetadata, len(dates))
		for i, d := range dates {
			eps[i] = library.EpisodeMetadata{EpisodeNumber: i + 1, Name: fmt.Sprintf("Episode %d", i+1), AirDate: d}
		}
		if _, err := meta.UpsertEpisodes(ctx, row.ID, eps, false); err != nil {
			s.t.Fatal(err)
		}
		for i := range dates {
			var id int64
			if err := s.db.QueryRow(`SELECT id FROM episodes WHERE season_id = $1 AND episode_number = $2`, row.ID, i+1).Scan(&id); err != nil {
				s.t.Fatal(err)
			}
			ids[fmt.Sprintf("%dx%d", n, i+1)] = id
		}
	}
	return show, ids
}

func (s seeder) file(episodeID int64, path string) {
	s.t.Helper()
	s.exec(`INSERT INTO torrent_assignments (item_type, item_id, info_hash, magnet_uri, file_path, file_size)
		VALUES ('episode', $1, 'hash-me', 'magnet:?xt=urn:btih:hash-me', $2, 100)`, episodeID, path)
}

// progress adds a Trakt progress row: last watched S1E{last}, next
// S{nextSeason}E{nextEpisode} (nextSeason -1: none).
func (s seeder) progress(traktID, tmdbID int, title string, watched time.Time, last, nextSeason, nextEpisode int) {
	s.t.Helper()
	var ns, ne any
	if nextSeason >= 0 {
		ns, ne = nextSeason, nextEpisode
	}
	s.exec(`INSERT INTO trakt_show_progress (show_trakt_id, show_tmdb_id, title, aired, completed, last_watched_at,
			last_season, last_episode, next_season, next_episode)
		VALUES ($1, $2, $3, 10, $4, $5, 1, $4, $6, $7)`, traktID, tmdbID, title, last, watched, ns, ne)
	for e := 1; e <= last; e++ {
		s.exec(`INSERT INTO trakt_watched_episodes (show_tmdb_id, season, episode, plays, last_watched_at) VALUES ($1, 1, $2, 1, $3)`,
			tmdbID, e, watched)
	}
}

func (s seeder) calendar(traktID, tmdbID, season, episode int, firstAired time.Time) {
	s.t.Helper()
	s.exec(`INSERT INTO trakt_calendar (show_trakt_id, season, episode, show_tmdb_id, show_title, episode_title, first_aired)
		VALUES ($1, $2, $3, $4, 'Calendar', $5, $6)`, traktID, season, episode, tmdbID, fmt.Sprintf("S%dE%d", season, episode), firstAired)
}

func TestIntegration_Me(t *testing.T) {
	db, counter := openSchemaDB(t)
	ctx := context.Background()
	s := seeder{t: t, db: db}
	date := func(days int) string { return now.AddDate(0, 0, days).Format(time.DateOnly) }

	// Alpha has specials; Trakt names a special next, so the calendar's
	// next regular episode is it, and the library has its file.
	alphaShow, alpha := s.show(9001, "Alpha", map[int][]string{0: {date(-90), date(-30)}, 1: {date(-60), date(-53), date(-46), date(-39), date(-32), date(-1)}})
	s.file(alpha["1x6"], "Alpha/S01E06.mkv")
	s.progress(1, 9001, "Alpha", now.Add(-24*time.Hour), 5, 0, 2)
	s.calendar(1, 9001, 1, 6, now.Add(-30*time.Hour))
	// Bravo isn't in the library; its next episode airs tomorrow.
	s.progress(2, 9002, "Bravo", now.Add(-72*time.Hour), 2, 1, 3)
	s.calendar(2, 9002, 1, 3, now.Add(26*time.Hour))
	// Charlie was caught up; then E9 aired an hour ago.
	charlieShow, charlie := s.show(9003, "Charlie", map[int][]string{1: {date(-70), date(-63), date(-56), date(-49), date(-42), date(-35), date(-28), date(-21), date(0)}})
	s.progress(3, 9003, "Charlie", now.Add(-48*time.Hour), 8, -1, 0)
	s.calendar(3, 9003, 1, 9, now.Add(-time.Hour))
	s.calendar(3, 9003, 1, 10, now.Add(7*24*time.Hour))
	// Foxtrot is only watchlisted: on Trakt's calendar, never started, not
	// in the library. Its episode isn't new.
	s.calendar(6, 9006, 1, 1, now.Add(-2*time.Hour))
	// Delta is dropped.
	s.progress(4, 9004, "Delta", now.Add(-time.Hour), 1, 1, 2)
	s.exec(`INSERT INTO trakt_hidden (section, trakt_id, tmdb_id) VALUES ('dropped', 4, 9004)`)
	// Echo is watchlisted and in the library, not started.
	echoShow, echo := s.show(9005, "Echo", map[int][]string{1: {date(-3), date(4)}})
	s.exec(`INSERT INTO trakt_watchlist (media_type, trakt_id, tmdb_id, title, year, listed_at) VALUES
		('tv', 5, 9005, 'Echo', 2026, $1), ('movie', 6, 603, 'The Matrix', 1999, $2), ('tv', 7, 9100, 'Elsewhere', 2025, $3)`,
		now.Add(-5*24*time.Hour), now.Add(-time.Hour), now.Add(-10*24*time.Hour))
	s.exec(`INSERT INTO trakt_history (id, media_type, tmdb_id, trakt_id, title, year, season, episode, episode_title, watched_at) VALUES
		(11, 'episode', 9001, 1, 'Alpha', 2020, 1, 5, 'Five', $1), (12, 'movie', 603, 6, 'The Matrix', 1999, NULL, NULL, NULL, $2)`,
		now.Add(-24*time.Hour), now.Add(-200*24*time.Hour))

	repo := library.NewCatalogRepository(db)
	svc := New(Deps{
		Store:           repo,
		Settings:        library.NewSettingsRepository(db),
		Catalog:         catalog.New(catalog.Deps{Store: repo, Now: func() time.Time { return now }}),
		WebDAVPublicURL: "https://webdav.local",
		Now:             func() time.Time { return now },
	})
	queries := func(fn func() error) int {
		t.Helper()
		before := counter.n.Load()
		if err := fn(); err != nil {
			t.Fatal(err)
		}
		return int(counter.n.Load() - before)
	}

	var upNext []UpNextItem
	n := queries(func() (err error) { upNext, err = svc.UpNext(ctx); return err })
	t.Logf("up next: %d queries (budget 4)", n)
	if n > 4 {
		t.Errorf("up next: %d queries, budget 4", n)
	}
	var got []string
	for _, it := range upNext {
		got = append(got, fmt.Sprintf("%s S%dE%d %s", it.Show.Title, it.NextEpisode.Season, it.NextEpisode.Episode, it.Availability))
	}
	want := "Alpha S1E6 ready, Charlie S1E9 needs_torrent, Echo S1E1 needs_torrent, Bravo S1E3 airs_on"
	if strings.Join(got, ", ") != want {
		t.Errorf("up next = %s\nwant      %s", strings.Join(got, ", "), want)
	}
	if len(upNext) == 4 {
		a, b, e := upNext[0], upNext[3], upNext[2]
		if a.WebDAVPath == nil || *a.WebDAVPath != "/TV Shows/Alpha (2020)/Season 01/Alpha - S01E06 - Episode 6.mkv" || a.Show.LibraryID == nil {
			t.Errorf("Alpha = %+v", a)
		}
		if b.AirsAt == nil || !b.AirsAt.Equal(now.Add(26*time.Hour)) || b.Show.LibraryID != nil || deref(b.NextEpisode.Name) != "S1E3" {
			t.Errorf("Bravo = %+v (episode %+v)", b, b.NextEpisode)
		}
		if e.Source != "watchlist" || e.Progress.Completed != 0 || e.Progress.Aired != 1 || e.LastWatchedAt != nil {
			t.Errorf("Echo = %+v", e)
		}
	}

	// What smart downloads read (#203): the library's shows with the
	// library episode each names next; Bravo isn't in the library.
	var nextEps []UpNextEpisode
	n = queries(func() (err error) { nextEps, err = svc.UpNextEpisodes(ctx); return err })
	if n > 3 {
		t.Errorf("up next episodes: %d queries, budget 3", n)
	}
	got = nil
	for _, e := range nextEps {
		last := "never"
		if e.LastWatchedAt != nil {
			last = e.LastWatchedAt.UTC().Format(time.RFC3339)
		}
		got = append(got, fmt.Sprintf("%d:%d S%dE%d %s", e.ShowID, e.EpisodeID, e.SeasonNumber, e.EpisodeNumber, last))
	}
	slices.Sort(got)
	wantEps := []string{
		fmt.Sprintf("%d:%d S1E6 %s", alphaShow.ID, alpha["1x6"], now.Add(-24*time.Hour).Format(time.RFC3339)),
		fmt.Sprintf("%d:%d S1E9 %s", charlieShow.ID, charlie["1x9"], now.Add(-48*time.Hour).Format(time.RFC3339)),
		fmt.Sprintf("%d:%d S1E1 never", echoShow.ID, echo["1x1"]),
	}
	slices.Sort(wantEps)
	if !slices.Equal(got, wantEps) {
		t.Errorf("up next episodes = %v\nwant                %v", got, wantEps)
	}

	var fresh []NewEpisodeItem
	n = queries(func() (err error) { fresh, err = svc.NewEpisodes(ctx, 7); return err })
	t.Logf("new episodes: %d queries (budget 3)", n)
	if n > 3 {
		t.Errorf("new episodes: %d queries, budget 3", n)
	}
	got = nil
	for _, it := range fresh {
		got = append(got, fmt.Sprintf("%s S%dE%d %s", it.Show.Title, it.Episode.Season, it.Episode.Episode, it.Availability))
	}
	// Bravo's E3 hasn't aired; Delta is dropped; Foxtrot isn't started;
	// Alpha's E6 is on the calendar and in the library: once.
	if want := "Charlie S1E9 needs_torrent, Alpha S1E6 ready, Echo S1E1 needs_torrent"; strings.Join(got, ", ") != want {
		t.Errorf("new episodes = %s\nwant           %s", strings.Join(got, ", "), want)
	}

	var watchlist []WatchlistItem
	n = queries(func() (err error) { watchlist, err = svc.Watchlist(ctx, ""); return err })
	t.Logf("watchlist: %d queries (budget 2)", n)
	if n > 2 {
		t.Errorf("watchlist: %d queries, budget 2", n)
	}
	if len(watchlist) != 3 || watchlist[0].TMDBID != 603 || watchlist[0].Library != nil || watchlist[1].TMDBID != 9005 ||
		watchlist[1].Library == nil || watchlist[1].Library.AvailabilityStage != catalog.StageNeedsTorrent || watchlist[2].TMDBID != 9100 {
		t.Errorf("watchlist = %+v", watchlist)
	}
	if shows, err := svc.Watchlist(ctx, "tv"); err != nil || len(shows) != 2 {
		t.Errorf("tv watchlist = %+v, %v", shows, err)
	}

	var history *HistoryPage
	n = queries(func() (err error) { history, err = svc.History(ctx, 1, 50); return err })
	if n != 1 {
		t.Errorf("history: %d queries, budget 1", n)
	}
	if history.Total != 2 || len(history.Items) != 2 || history.Items[0].MediaType != "tv" || *history.Items[0].EpisodeName != "Five" ||
		history.Items[1].MediaType != "movie" || history.Items[1].Season != nil {
		t.Errorf("history = %+v", history)
	}

	// A disconnect purges every Trakt table: /me answers empty, but new
	// episodes of the library's shows.
	if err := library.NewTraktAuthRepository(db).Purge(ctx); err != nil {
		t.Fatal(err)
	}
	if upNext, err := svc.UpNext(ctx); err != nil || len(upNext) != 0 {
		t.Errorf("up next without Trakt = %+v, %v", upNext, err)
	}
	if eps, err := svc.UpNextEpisodes(ctx); err != nil || len(eps) != 0 {
		t.Errorf("up next episodes without Trakt = %+v, %v", eps, err)
	}
	if watchlist, err := svc.Watchlist(ctx, ""); err != nil || len(watchlist) != 0 {
		t.Errorf("watchlist without Trakt = %+v, %v", watchlist, err)
	}
	if history, err := svc.History(ctx, 1, 50); err != nil || history.Total != 0 || len(history.Items) != 0 {
		t.Errorf("history without Trakt = %+v, %v", history, err)
	}
	fresh, err := svc.NewEpisodes(ctx, 7)
	got = nil
	for _, it := range fresh {
		got = append(got, fmt.Sprintf("%s S%dE%d", it.Show.Title, it.Episode.Season, it.Episode.Episode))
	}
	if want := "Charlie S1E9, Alpha S1E6, Echo S1E1"; err != nil || strings.Join(got, ", ") != want {
		t.Errorf("new episodes without Trakt = %s, %v; want %s", strings.Join(got, ", "), err, want)
	}
}
