package library

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"
)

func syncStore(t *testing.T) (*DB, *TraktSyncStore, time.Time) {
	t.Helper()
	db := openSchemaDB(t)
	gen, err := NewTraktAuthRepository(db).ForMember(1).Connect(context.Background(), traktAuthRow("teymur"))
	if err != nil {
		t.Fatal(err)
	}
	return db, NewTraktSyncStore(db).ForMember(1), gen
}

func at(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

func moviePlay(id int64, tmdbID int, watched string) TraktPlayRow {
	return TraktPlayRow{ID: id, MediaType: "movie", TMDBID: tmdbID, TraktID: int64(tmdbID) + 10_000, Title: "Movie", Year: 1999, WatchedAt: at(watched)}
}

func episodePlay(id int64, showTMDB, season, episode, episodeTMDB int, watched string) TraktPlayRow {
	return TraktPlayRow{
		ID: id, MediaType: "episode", TMDBID: showTMDB, TraktID: int64(showTMDB) + 10_000, Title: "Show",
		Season: &season, Episode: &episode, EpisodeTMDBID: episodeTMDB, WatchedAt: at(watched),
	}
}

func queryStrings(t *testing.T, db *DB, query string) []string {
	t.Helper()
	rows, err := db.Query(query)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	cols, _ := rows.Columns()
	var out []string
	for rows.Next() {
		vals := make([]sql.NullString, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			t.Fatal(err)
		}
		line := ""
		for i, v := range vals {
			if i > 0 {
				line += " "
			}
			if v.Valid {
				line += v.String
			} else {
				line += "NULL"
			}
		}
		out = append(out, line)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestTraktSyncStore_History(t *testing.T) {
	db, s, gen := syncStore(t)
	ctx := context.Background()

	err := s.ReplaceHistory(ctx, gen, []TraktPlayRow{
		moviePlay(1, 603, "2026-10-10T20:00:00Z"),
		moviePlay(2, 603, "2026-10-14T21:00:00Z"),
		episodePlay(3, 1396, 1, 1, 62085, "2026-10-15T19:00:00Z"),
		episodePlay(4, 1396, 0, 1, 0, "2026-10-15T19:30:00Z"), // a special without a TMDB id
	})
	if err != nil {
		t.Fatal(err)
	}
	// An overlap upserts onto the same rows, also twice in one batch.
	err = s.UpsertHistory(ctx, gen, []TraktPlayRow{
		episodePlay(3, 1396, 1, 1, 62085, "2026-10-15T19:00:00Z"),
		episodePlay(5, 1396, 1, 1, 62085, "2026-10-16T08:00:00Z"),
		episodePlay(5, 1396, 1, 1, 62085, "2026-10-16T08:00:00Z"),
	})
	if err != nil {
		t.Fatal(err)
	}

	if got, want := queryStrings(t, db, `SELECT id, media_type, tmdb_id, season, episode, episode_tmdb_id FROM trakt_history ORDER BY id`),
		[]string{"1 movie 603 NULL NULL NULL", "2 movie 603 NULL NULL NULL", "3 episode 1396 1 1 62085", "4 episode 1396 0 1 NULL", "5 episode 1396 1 1 62085"}; !reflect.DeepEqual(got, want) {
		t.Errorf("history = %q, want %q", got, want)
	}
	if got, want := queryStrings(t, db, `SELECT tmdb_id, plays, to_char(last_watched_at AT TIME ZONE 'UTC', 'MM-DD HH24:MI') FROM trakt_watched_movies`),
		[]string{"603 2 10-14 21:00"}; !reflect.DeepEqual(got, want) {
		t.Errorf("watched movies = %q, want %q", got, want)
	}
	if got, want := queryStrings(t, db, `SELECT show_tmdb_id, season, episode, episode_tmdb_id, plays FROM trakt_watched_episodes ORDER BY season, episode`),
		[]string{"1396 0 1 NULL 1", "1396 1 1 62085 2"}; !reflect.DeepEqual(got, want) {
		t.Errorf("watched episodes = %q, want %q", got, want)
	}
	movie, episode, err := s.LatestPlays(ctx)
	if err != nil || !movie.Equal(at("2026-10-14T21:00:00Z")) || !episode.Equal(at("2026-10-16T08:00:00Z")) {
		t.Errorf("LatestPlays = %v, %v, %v", movie, episode, err)
	}

	// A reconcile replaces: a deleted play goes, with what it counted.
	if err := s.ReplaceHistory(ctx, gen, []TraktPlayRow{moviePlay(1, 603, "2026-10-10T20:00:00Z")}); err != nil {
		t.Fatal(err)
	}
	if got := queryStrings(t, db, `SELECT tmdb_id, plays FROM trakt_watched_movies`); !reflect.DeepEqual(got, []string{"603 1"}) {
		t.Errorf("watched movies after a replace = %q", got)
	}
	if got := queryStrings(t, db, `SELECT COUNT(*) FROM trakt_watched_episodes`); got[0] != "0" {
		t.Errorf("watched episodes after a replace = %s, want 0", got[0])
	}
}

func TestTraktSyncStore_GenerationGuard(t *testing.T) {
	db, s, gen := syncStore(t)
	ctx := context.Background()
	plays := []TraktPlayRow{moviePlay(1, 603, "2026-10-10T20:00:00Z")}

	if err := s.ReplaceHistory(ctx, gen.Add(time.Microsecond), plays); !errors.Is(err, ErrTraktConnectionReplaced) {
		t.Errorf("a write for another connection = %v, want ErrTraktConnectionReplaced", err)
	}
	if err := NewTraktAuthRepository(db).ForMember(1).Purge(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveRun(ctx, gen, json.RawMessage(`{}`)); !errors.Is(err, ErrTraktConnectionReplaced) {
		t.Errorf("a write after a disconnect = %v, want ErrTraktConnectionReplaced", err)
	}
	for table, n := range rowCounts(t, db, 1) {
		if n != 0 {
			t.Errorf("%s has %d rows", table, n)
		}
	}
}

// A purge waits for a sync write in flight, then deletes what it wrote: no
// deadlock, nothing left.
func TestTraktSyncStore_PurgeWaitsForAWrite(t *testing.T) {
	db, s, gen := syncStore(t)
	ctx := context.Background()

	written, release := make(chan struct{}), make(chan struct{})
	errs := make(chan error, 1)
	go func() {
		errs <- s.write(ctx, gen, func(tx *sql.Tx, member int64) error {
			if err := writeHistory(ctx, tx, member, []TraktPlayRow{moviePlay(1, 603, "2026-10-10T20:00:00Z")}); err != nil {
				return err
			}
			close(written)
			<-release
			return nil
		})
	}()
	<-written
	purged := make(chan error, 1)
	go func() { purged <- NewTraktAuthRepository(db).ForMember(1).Purge(ctx) }()
	select {
	case err := <-purged:
		t.Fatalf("Purge finished during a write: %v", err)
	case <-time.After(200 * time.Millisecond):
	}
	close(release)
	if err := <-errs; err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := <-purged; err != nil {
		t.Fatalf("Purge: %v", err)
	}
	for table, n := range rowCounts(t, db, 1) {
		if n != 0 {
			t.Errorf("%s has %d rows after the purge", table, n)
		}
	}
}

func TestTraktSyncStore_ReplacesKeepArtwork(t *testing.T) {
	db, s, gen := syncStore(t)
	ctx := context.Background()
	seedMovie(t, db, "In The Library", 2001) // not a Trakt row's TMDB id
	if _, err := db.Exec(`INSERT INTO shows (tmdb_id, title, year) VALUES (1396, 'Breaking Bad', 2008)`); err != nil {
		t.Fatal(err)
	}
	next := func(season, episode int) (*int, *int) { return &season, &episode }

	watchlist := []TraktWatchlistRow{
		{MediaType: "movie", TraktID: 1, TMDBID: 693134, Title: "Dune: Part Two", Year: 2024, Rank: 1, ListedAt: at("2026-10-01T00:00:00Z")},
		{MediaType: "tv", TraktID: 2, TMDBID: 95396, Title: "Severance", Year: 2022, Rank: 2, ListedAt: at("2026-10-02T00:00:00Z")},
	}
	s1, e3 := next(1, 3)
	progress := []TraktShowProgressRow{
		{ShowTraktID: 1388, ShowTMDBID: 1396, Title: "Breaking Bad", Aired: 62, Completed: 2, NextSeason: s1, NextEpisode: e3},
		{ShowTraktID: 154997, ShowTMDBID: 95396, Title: "Severance", Aired: 19, Completed: 18, NextSeason: s1, NextEpisode: e3},
	}
	calendar := []TraktCalendarRow{
		{ShowTraktID: 154997, Season: 3, Episode: 1, ShowTMDBID: 95396, ShowTitle: "Severance", FirstAired: at("2026-10-17T02:00:00Z")},
		{ShowTraktID: 1388, Season: 6, Episode: 1, ShowTMDBID: 1396, ShowTitle: "Breaking Bad", FirstAired: at("2026-10-18T02:00:00Z")},
	}
	for _, err := range []error{
		s.ReplaceWatchlist(ctx, gen, watchlist),
		s.ReplaceShowProgress(ctx, gen, progress),
		s.ReplaceCalendar(ctx, gen, calendar, at("2026-10-16T12:00:00Z")),
	} {
		if err != nil {
			t.Fatal(err)
		}
	}

	wants, err := s.ArtworkWants(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := TraktArtworkWants{
		Movies:   []int{693134},
		Shows:    []int{95396}, // Breaking Bad is in the library
		Episodes: []TraktStillKey{{95396, 1, 3}, {95396, 3, 1}},
	}
	if !reflect.DeepEqual(wants, want) {
		t.Errorf("ArtworkWants = %+v, want %+v", wants, want)
	}

	err = s.SetArtwork(ctx, gen, TraktArtwork{
		Movies: map[int]TraktImages{693134: {Poster: "/dune.jpg", Backdrop: ""}},
		Shows:  map[int]TraktImages{95396: {Poster: "/sev.jpg", Backdrop: "/sev-bg.jpg"}},
		Stills: map[TraktStillKey]string{{95396, 1, 3}: "/sev-1x3.jpg", {95396, 3, 1}: "/sev-3x1.jpg"},
	})
	if err != nil {
		t.Fatal(err)
	}
	// Filled only where missing: a second answer doesn't overwrite.
	err = s.SetArtwork(ctx, gen, TraktArtwork{Movies: map[int]TraktImages{693134: {Poster: "/other.jpg", Backdrop: "/dune-bg.jpg"}}})
	if err != nil {
		t.Fatal(err)
	}
	if wants, _ := s.ArtworkWants(ctx); len(wants.Movies)+len(wants.Shows)+len(wants.Episodes) != 0 {
		t.Errorf("ArtworkWants after SetArtwork = %+v, want none", wants)
	}

	// Replacing keeps a row's artwork; a next episode that moved on loses
	// its still, a title that left goes.
	s1, e4 := next(1, 4)
	progress[1].NextSeason, progress[1].NextEpisode, progress[1].Completed = s1, e4, 19
	for _, err := range []error{
		s.ReplaceWatchlist(ctx, gen, watchlist[:1]),
		s.ReplaceShowProgress(ctx, gen, progress),
		s.ReplaceCalendar(ctx, gen, calendar, at("2026-10-17T12:00:00Z")),
	} {
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, c := range []struct {
		query string
		want  []string
	}{
		{`SELECT tmdb_id, poster_path, backdrop_path FROM trakt_watchlist`, []string{"693134 /dune.jpg /dune-bg.jpg"}},
		{`SELECT show_tmdb_id, completed, poster_path, backdrop_path, next_episode, next_still_path FROM trakt_show_progress ORDER BY 1`,
			[]string{"1396 2 NULL NULL 3 NULL", "95396 19 /sev.jpg /sev-bg.jpg 4 NULL"}},
		{`SELECT show_tmdb_id, season, still_path, poster_path FROM trakt_calendar ORDER BY 1`,
			[]string{"1396 6 NULL NULL", "95396 3 /sev-3x1.jpg /sev.jpg"}},
	} {
		if got := queryStrings(t, db, c.query); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s\n = %q, want %q", c.query, got, c.want)
		}
	}
	st, err := s.State(ctx)
	if err != nil || !st.CalendarFetchedAt.Equal(at("2026-10-17T12:00:00Z")) {
		t.Errorf("CalendarFetchedAt = %v, %v", st.CalendarFetchedAt, err)
	}
}

func TestTraktSyncStore_RatingsAndHidden(t *testing.T) {
	db, s, gen := syncStore(t)
	ctx := context.Background()
	season := 1
	ratings := []TraktRatingRow{
		{MediaType: "movie", TraktID: 481, TMDBID: 603, Rating: 9, RatedAt: at("2026-10-01T00:00:00Z")},
		{MediaType: "season", TraktID: 3950, ShowTMDBID: 1396, Season: &season, Rating: 8, RatedAt: at("2026-10-02T00:00:00Z")},
	}
	if err := s.ReplaceRatings(ctx, gen, []string{"movie", "show", "season", "episode"}, ratings); err != nil {
		t.Fatal(err)
	}
	// Only the given types are replaced.
	if err := s.ReplaceRatings(ctx, gen, []string{"movie"}, nil); err != nil {
		t.Fatal(err)
	}
	if got, want := queryStrings(t, db, `SELECT media_type, tmdb_id, show_tmdb_id, season, rating FROM trakt_ratings`),
		[]string{"season NULL 1396 1 8"}; !reflect.DeepEqual(got, want) {
		t.Errorf("ratings = %q, want %q", got, want)
	}

	hidden := []TraktHiddenRow{{"dropped", 1393, 1402}, {"progress_watched", 511, 4607}, {"dropped", 1393, 1402}}
	if err := s.ReplaceHidden(ctx, gen, hidden); err != nil {
		t.Fatal(err)
	}
	if err := s.ReplaceHidden(ctx, gen, hidden[1:2]); err != nil {
		t.Fatal(err)
	}
	if got := queryStrings(t, db, `SELECT section, tmdb_id FROM trakt_hidden`); !reflect.DeepEqual(got, []string{"progress_watched 4607"}) {
		t.Errorf("hidden = %q", got)
	}
}

func TestTraktSyncStore_State(t *testing.T) {
	_, s, gen := syncStore(t)
	ctx := context.Background()
	if st, err := s.State(ctx); err != nil || st.Activities != nil || !st.LastSyncAt.IsZero() || st.LastRun != nil {
		t.Fatalf("State before a sync = %+v, %v", st, err)
	}
	done := TraktSyncDone{
		Activities: json.RawMessage(`{"all": "2026-10-16T08:00:00.000Z"}`), At: at("2026-10-16T12:00:00Z"),
		Run: json.RawMessage(`{"status": "idle"}`),
	}
	if err := s.FinishSync(ctx, gen, done); err != nil {
		t.Fatal(err)
	}
	st, err := s.State(ctx)
	if err != nil || !st.LastSyncAt.Equal(done.At) || !st.LastReconcileAt.IsZero() ||
		string(st.Activities) != `{"all": "2026-10-16T08:00:00.000Z"}` || string(st.LastRun) != `{"status": "idle"}` {
		t.Errorf("State after an incremental sync = %+v (%s, %s), %v", st, st.Activities, st.LastRun, err)
	}
	done.Reconcile, done.At = true, at("2026-10-17T12:00:00Z")
	if err := s.FinishSync(ctx, gen, done); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveRun(ctx, gen, json.RawMessage(`{"status": "error"}`)); err != nil {
		t.Fatal(err)
	}
	st, err = s.State(ctx)
	if err != nil || !st.LastReconcileAt.Equal(done.At) || !st.LastSyncAt.Equal(done.At) || string(st.LastRun) != `{"status": "error"}` {
		t.Errorf("State after a reconcile = %+v, %v", st, err)
	}
}
