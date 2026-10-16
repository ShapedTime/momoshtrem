package trakt

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/tmdb"
	"github.com/shapedtime/momoshtrem/internal/trakt/trakttest"
)

type obj = map[string]any

// The fake account's titles. The Matrix and Breaking Bad are in the
// library; the ones without a TMDB id are skipped.
var (
	matrix    = obj{"title": "The Matrix", "year": 1999, "ids": obj{"trakt": 481, "slug": "the-matrix-1999", "tmdb": 603}}
	dune      = obj{"title": "Dune: Part Two", "year": 2024, "ids": obj{"trakt": 287071, "tmdb": 693134}}
	noTMDB    = obj{"title": "Obscure", "year": 1970, "ids": obj{"trakt": 999, "tmdb": nil}}
	bb        = obj{"title": "Breaking Bad", "year": 2008, "ids": obj{"trakt": 1388, "tvdb": 81189, "tmdb": 1396}}
	severance = obj{"title": "Severance", "year": 2022, "ids": obj{"trakt": 154997, "tmdb": 95396}}
	twd       = obj{"title": "The Walking Dead", "year": 2010, "ids": obj{"trakt": 1393, "tmdb": 1402}}
	lost      = obj{"title": "Lost", "year": 2004, "ids": obj{"trakt": 511, "tmdb": 4607}}
	noTMDBTV  = obj{"title": "Obscure Show", "year": 1970, "ids": obj{"trakt": 998, "tmdb": nil}}
	theOA     = obj{"title": "The OA", "year": 2016, "ids": obj{"trakt": 101530, "tmdb": 69851}}
)

func ep(season, number, traktID, tmdbID int, title string) obj {
	return obj{"season": season, "number": number, "title": title, "ids": obj{"trakt": traktID, "tmdb": tmdbID}}
}

func moviePlay(id int, at string, movie obj) obj {
	return obj{"id": id, "watched_at": at, "action": "watch", "type": "movie", "movie": movie}
}

func episodePlay(id int, at string, show, episode obj) obj {
	return obj{"id": id, "watched_at": at, "action": "scrobble", "type": "episode", "episode": episode, "show": show}
}

var (
	bb1x1 = ep(1, 1, 73482, 62085, "Pilot")
	bb1x2 = ep(1, 2, 73483, 62086, "Cat's in the Bag...")
	bb1x3 = ep(1, 3, 73484, 62087, "...And the Bag's in the River")
	sev3  = ep(3, 1, 999001, 7000001, "Hello, Ms. Cobel")
)

// fakeArtwork is TMDB for the details step: every title has a poster and a
// backdrop, every episode a still. It counts lookups and records the sync
// step each happened in.
type fakeArtwork struct {
	s       *Syncer
	mu      sync.Mutex
	version string
	calls   []string
	steps   map[SyncStep]bool
}

func (f *fakeArtwork) record(call string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, call)
	if f.s != nil {
		f.s.mu.Lock()
		if f.s.run.Step != nil {
			f.steps[*f.s.run.Step] = true
		}
		f.s.mu.Unlock()
	}
	return f.version
}

func (f *fakeArtwork) Calls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.calls)
}

func (f *fakeArtwork) MovieDetails(_ context.Context, id int) (*tmdb.MovieDetails, error) {
	v := f.record(fmt.Sprintf("movie/%d", id))
	return &tmdb.MovieDetails{ID: id, PosterPath: fmt.Sprintf("/m%d%s.jpg", id, v), BackdropPath: fmt.Sprintf("/m%d-bg%s.jpg", id, v)}, nil
}

func (f *fakeArtwork) ShowDetails(_ context.Context, id int) (*tmdb.ShowDetails, error) {
	v := f.record(fmt.Sprintf("tv/%d", id))
	return &tmdb.ShowDetails{ID: id, Status: "Returning Series", PosterPath: fmt.Sprintf("/t%d%s.jpg", id, v), BackdropPath: fmt.Sprintf("/t%d-bg%s.jpg", id, v)}, nil
}

func (f *fakeArtwork) Season(_ context.Context, showID, number int, status string) (*tmdb.Season, error) {
	v := f.record(fmt.Sprintf("tv/%d/season/%d (%s)", showID, number, status))
	season := &tmdb.Season{SeasonNumber: number}
	for e := 1; e <= 10; e++ {
		season.Episodes = append(season.Episodes, tmdb.Episode{SeasonNumber: number, EpisodeNumber: e, StillPath: fmt.Sprintf("/t%d-%dx%d%s.jpg", showID, number, e, v)})
	}
	return season, nil
}

// apiCall is a Trakt API request the sync made, and the step it was in.
type apiCall struct {
	path     string
	query    url.Values
	step     SyncStep
	progress SyncProgress
}

// watchedAuth is the connection, recording each request with the sync's
// step and progress.
type watchedAuth struct {
	*Auth
	s        *Syncer
	mu       sync.Mutex
	requests []apiCall
}

func (w *watchedAuth) Get(ctx context.Context, path string, query url.Values, v any) (http.Header, error) {
	w.s.mu.Lock()
	r := apiCall{path: path, query: query, progress: w.s.run.Progress}
	if w.s.run.Step != nil {
		r.step = *w.s.run.Step
	}
	w.s.mu.Unlock()
	w.mu.Lock()
	w.requests = append(w.requests, r)
	w.mu.Unlock()
	return w.Auth.Get(ctx, path, query, v)
}

// since are the requests after the first n.
func (w *watchedAuth) since(n int) []apiCall {
	w.mu.Lock()
	defer w.mu.Unlock()
	return slices.Clone(w.requests[n:])
}

func (w *watchedAuth) count() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.requests)
}

func paths(rs []apiCall) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.path
	}
	return out
}

type syncEnv struct {
	t      *testing.T
	ctx    context.Context
	clock  Clock
	srv    *trakttest.Server
	db     *library.DB
	store  *library.TraktAuthRepository
	auth   *Auth
	conn   *watchedAuth
	art    *fakeArtwork
	syncer *Syncer

	mu         sync.Mutex
	activities map[string]map[string]string
}

// newSyncEnv is a connected account on the fake Trakt (the device flow
// skipped), with The Matrix and Breaking Bad in the library.
func newSyncEnv(t *testing.T, clock Clock) *syncEnv {
	t.Helper()
	db := openSchemaDB(t)
	se := &syncEnv{
		t: t, ctx: context.Background(), clock: clock, srv: trakttest.New(t), db: db,
		store: library.NewTraktAuthRepository(db), art: &fakeArtwork{steps: map[SyncStep]bool{}},
	}
	se.srv.SetClock(clock.Now)
	box := mustBox(t, testKey)
	access, refresh := se.srv.Grant()
	created := clock.Now().UTC().Truncate(time.Second)
	_, err := se.store.Connect(se.ctx, library.TraktAuth{
		Username: "Teymur", UserSlug: "teymur",
		AccessTokenEnc: box.Seal([]byte(access), aadAccessToken), RefreshTokenEnc: box.Seal([]byte(refresh), aadRefreshToken),
		TokenCreatedAt: created, ExpiresAt: created.Add(7 * 24 * time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	se.auth = NewAuth(AuthDeps{
		Client: newTestClient(se.srv, clock), Box: box, Store: se.store, Settings: library.NewSettingsRepository(db), Clock: clock,
	})
	t.Cleanup(se.auth.Close)
	se.conn = &watchedAuth{Auth: se.auth}
	se.syncer = NewSyncer(SyncDeps{Auth: se.conn, Store: library.NewTraktSyncStore(db), Artwork: se.art, Clock: clock})
	se.conn.s, se.art.s = se.syncer, se.syncer
	se.auth.SetHooks(se.syncer.Hooks())
	t.Cleanup(se.syncer.Stop)

	se.exec(`INSERT INTO movies (tmdb_id, title, year) VALUES (603, 'The Matrix', 1999)`)
	se.exec(`INSERT INTO shows (tmdb_id, title, year) VALUES (1396, 'Breaking Bad', 2008)`)
	se.serveAccount()
	return se
}

func (se *syncEnv) exec(query string, args ...any) {
	se.t.Helper()
	if _, err := se.db.Exec(query, args...); err != nil {
		se.t.Fatalf("%s: %v", query, err)
	}
}

// serveAccount serves the fake account: every list the sync reads, and
// last_activities from se.activities.
func (se *syncEnv) serveAccount() {
	se.activities = map[string]map[string]string{
		"movies":    {"watched_at": "2026-10-14T21:00:00.000Z", "rated_at": "2026-10-01T10:00:00.000Z", "watchlisted_at": "2026-10-02T10:00:00.000Z", "collected_at": "2026-01-01T00:00:00.000Z"},
		"episodes":  {"watched_at": "2026-10-15T20:00:00.000Z", "rated_at": "2026-10-03T10:00:00.000Z", "paused_at": "2026-10-15T20:30:00.000Z"},
		"shows":     {"rated_at": "2026-10-04T10:00:00.000Z", "watchlisted_at": "2026-10-05T10:00:00.000Z", "hidden_at": "2026-10-06T10:00:00.000Z", "dropped_at": "2026-10-07T10:00:00.000Z"},
		"seasons":   {"rated_at": "2026-10-08T10:00:00.000Z", "hidden_at": "2026-10-09T10:00:00.000Z"},
		"watchlist": {"updated_at": "2026-10-05T10:00:00.000Z"},
	}
	se.srv.Handle("/sync/last_activities", func(w http.ResponseWriter, _ *http.Request) {
		se.mu.Lock()
		body := obj{"all": "2026-10-15T20:30:00.000Z"}
		for k, v := range se.activities {
			body[k] = v
		}
		b, _ := json.Marshal(body)
		se.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(b)
	})
	se.srv.List("/sync/history/movies", []obj{
		moviePlay(1002, "2026-10-14T21:00:00.000Z", matrix),
		moviePlay(1001, "2026-10-10T20:00:00.000Z", matrix),
		moviePlay(1003, "2026-10-09T20:00:00.000Z", noTMDB),
	})
	se.srv.List("/sync/history/episodes", []obj{
		episodePlay(2002, "2026-10-15T20:00:00.000Z", bb, bb1x2),
		episodePlay(2001, "2026-10-15T19:00:00.000Z", bb, bb1x1),
	})
	se.srv.List("/sync/ratings/movies", []obj{{"rated_at": "2026-10-01T10:00:00.000Z", "rating": 9, "type": "movie", "movie": matrix}})
	se.srv.List("/sync/ratings/shows", []obj{{"rated_at": "2026-10-04T10:00:00.000Z", "rating": 10, "type": "show", "show": bb}})
	se.srv.List("/sync/ratings/seasons", []obj{{"rated_at": "2026-10-08T10:00:00.000Z", "rating": 8, "type": "season",
		"season": obj{"number": 1, "ids": obj{"trakt": 3950, "tmdb": 3572}}, "show": bb}})
	se.srv.List("/sync/ratings/episodes", []obj{{"rated_at": "2026-10-03T10:00:00.000Z", "rating": 7, "type": "episode", "episode": bb1x1, "show": bb}})
	se.srv.List("/sync/watchlist/movies/added/desc", []obj{
		{"rank": 1, "id": 101, "listed_at": "2026-10-02T10:00:00.000Z", "notes": nil, "type": "movie", "movie": dune},
		{"rank": 2, "id": 102, "listed_at": "2026-09-02T10:00:00.000Z", "notes": nil, "type": "movie", "movie": matrix},
	})
	se.srv.List("/sync/watchlist/shows/added/desc", []obj{
		{"rank": 1, "id": 103, "listed_at": "2026-10-05T10:00:00.000Z", "notes": nil, "type": "show", "show": severance},
	})
	se.srv.List("/users/hidden/dropped", []obj{{"hidden_at": "2026-10-07T10:00:00.000Z", "type": "show", "show": twd}})
	se.srv.List("/users/hidden/progress_watched", []obj{
		{"hidden_at": "2026-10-06T10:00:00.000Z", "type": "show", "show": lost},
		{"hidden_at": "2026-10-06T10:00:00.000Z", "type": "show", "show": noTMDBTV},
	})
	se.srv.List("/sync/progress/watched", []obj{
		{"show": bb, "progress": obj{"aired": 62, "completed": 2, "last_watched_at": "2026-10-15T20:00:00.000Z", "reset_at": nil,
			"next_episode": bb1x3, "last_episode": bb1x2, "stats": obj{"play_count": 2}}},
		{"show": severance, "progress": obj{"aired": 19, "completed": 19, "last_watched_at": "2026-03-21T02:00:00.000Z", "reset_at": nil,
			"next_episode": nil, "last_episode": ep(2, 10, 999000, 6999999, "Cold Harbor")}},
		// Watchlisted, never started: not progress.
		{"show": theOA, "progress": obj{"aired": 16, "completed": 0, "last_watched_at": nil, "reset_at": nil,
			"next_episode": ep(1, 1, 2178520, 1213372, "Homecoming"), "last_episode": nil}},
	})
	// The calendar isn't paginated; it's served for whichever day the
	// clock is on.
	calendar, _ := json.Marshal([]obj{
		{"first_aired": "2026-10-17T02:00:00.000Z", "episode": sev3, "show": severance},
		{"first_aired": "2026-10-18T02:00:00.000Z", "episode": ep(1, 1, 5, 6, "Pilot"), "show": noTMDBTV},
	})
	for day := 0; day < 14; day++ {
		start := time.Date(2026, 10, 9+day, 0, 0, 0, 0, time.UTC).Format(time.DateOnly)
		se.srv.Handle("/calendars/my/shows/"+start+"/33", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(calendar)
		})
	}
}

// bump moves an activity on by a minute.
func (se *syncEnv) bump(section, field string) {
	se.mu.Lock()
	defer se.mu.Unlock()
	at, _ := time.Parse(time.RFC3339, se.activities[section][field])
	se.activities[section][field] = isoTime(at.Add(time.Minute))
}

func (se *syncEnv) seq() int {
	se.syncer.mu.Lock()
	defer se.syncer.mu.Unlock()
	return se.syncer.seq
}

// waitRuns waits until runs more syncs than seq have ended, and answers
// the sync's report.
func (se *syncEnv) waitRuns(seq, runs int) SyncStatus {
	se.t.Helper()
	waitFor(se.t, "the sync", func() bool {
		se.syncer.mu.Lock()
		defer se.syncer.mu.Unlock()
		return se.syncer.seq >= seq+runs && !se.syncer.running
	})
	return se.status()
}

func (se *syncEnv) status() SyncStatus {
	se.t.Helper()
	st, err := se.syncer.Status(se.ctx)
	if err != nil {
		se.t.Fatal(err)
	}
	return st
}

// syncNow runs one sync (POST /integrations/trakt/sync) to its end.
func (se *syncEnv) syncNow() SyncStatus {
	se.t.Helper()
	seq := se.seq()
	if _, err := se.auth.SyncNow(se.ctx); err != nil {
		se.t.Fatalf("SyncNow: %v", err)
	}
	return se.waitRuns(seq, 1)
}

func (se *syncEnv) rows(query string) []string {
	se.t.Helper()
	rows, err := se.db.Query(query)
	if err != nil {
		se.t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			se.t.Fatal(err)
		}
		out = append(out, s)
	}
	return out
}

func (se *syncEnv) countRows(table string) int {
	se.t.Helper()
	var n int
	if err := se.db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n); err != nil {
		se.t.Fatal(err)
	}
	return n
}

func wantRun(t *testing.T, run SyncRun, status SyncState, kind SyncKind) {
	t.Helper()
	if run.Status != status || ptrValue(run.Kind) != kind {
		t.Fatalf("sync = %s %v (step %v, error %v), want %s %s", run.Status, ptrValue(run.Kind), ptrValue(run.Step), ptrValue(run.Error), status, kind)
	}
}

func TestSync_InitialImport(t *testing.T) {
	se := newSyncEnv(t, newFakeClock())
	st := se.syncNow()

	wantRun(t, st.Run, SyncIdle, SyncInitial)
	if st.Run.Step != nil || st.Run.Error != nil || st.Run.FinishedAt == nil || st.LastSyncAt == nil {
		t.Errorf("finished sync = %+v, last_sync_at %v", st.Run, st.LastSyncAt)
	}
	if want := (SyncProgress{Plays: 5, PlaysTotal: 5, Ratings: 4, Watchlist: 3, Shows: 2}); st.Run.Progress != want {
		t.Errorf("progress = %+v, want %+v", st.Run.Progress, want)
	}

	// Every step in order, each request in its step.
	wantSteps := []struct {
		path string
		step SyncStep
	}{
		{"/sync/last_activities", StepHistory},
		{"/sync/history/movies", StepHistory},
		{"/sync/history/episodes", StepHistory},
		{"/sync/ratings/movies", StepRatings},
		{"/sync/ratings/shows", StepRatings},
		{"/sync/ratings/seasons", StepRatings},
		{"/sync/ratings/episodes", StepRatings},
		{"/sync/watchlist/movies/added/desc", StepWatchlist},
		{"/sync/watchlist/shows/added/desc", StepWatchlist},
		{"/users/hidden/dropped", StepProgress},
		{"/users/hidden/progress_watched", StepProgress},
		{"/sync/progress/watched", StepProgress},
		{"/calendars/my/shows/2026-10-09/33", StepProgress},
	}
	got := se.conn.since(0)
	if len(got) != len(wantSteps) {
		t.Fatalf("requests = %q", paths(got))
	}
	for i, w := range wantSteps {
		if got[i].path != w.path || got[i].step != w.step {
			t.Errorf("request %d = %s in %s, want %s in %s", i, got[i].path, got[i].step, w.path, w.step)
		}
	}
	// What earlier steps imported shows while later ones run.
	if p := got[7].progress; p.Plays != 5 || p.PlaysTotal != 5 || p.Ratings != 4 || p.Watchlist != 0 {
		t.Errorf("progress at the watchlist = %+v", p)
	}
	if got[11].query.Get("limit") != "250" || got[12].query != nil {
		t.Errorf("progress query %v, calendar query %v", got[11].query, got[12].query)
	}
	if !reflect.DeepEqual(se.art.steps, map[SyncStep]bool{StepDetails: true}) {
		t.Errorf("artwork looked up in steps %v", se.art.steps)
	}

	// The rows, every one mapped to TMDB.
	for _, c := range []struct {
		query string
		want  []string
	}{
		{`SELECT concat_ws(' ', id, media_type, tmdb_id, trakt_id, season, episode, episode_tmdb_id) FROM trakt_history ORDER BY id`,
			[]string{"1001 movie 603 481", "1002 movie 603 481", "2001 episode 1396 1388 1 1 62085", "2002 episode 1396 1388 1 2 62086"}},
		{`SELECT concat_ws(' ', tmdb_id, plays) FROM trakt_watched_movies`, []string{"603 2"}},
		{`SELECT concat_ws(' ', show_tmdb_id, season, episode, plays) FROM trakt_watched_episodes ORDER BY episode`,
			[]string{"1396 1 1 1", "1396 1 2 1"}},
		{`SELECT concat_ws(' ', media_type, tmdb_id, show_tmdb_id, season, episode, rating) FROM trakt_ratings ORDER BY rating`,
			[]string{"episode 62085 1396 1 1 7", "season 3572 1396 1 8", "movie 603 9", "show 1396 10"}},
		{`SELECT concat_ws(' ', media_type, tmdb_id, rank, poster_path) FROM trakt_watchlist ORDER BY listed_at`,
			[]string{"movie 603 2", "movie 693134 1 /m693134.jpg", "tv 95396 1 /t95396.jpg"}},
		{`SELECT concat_ws(' ', section, tmdb_id) FROM trakt_hidden ORDER BY 1`, []string{"dropped 1402", "progress_watched 4607"}},
		// The OA, watchlisted but never started, isn't progress.
		{`SELECT concat_ws(' ', show_tmdb_id, aired, completed, last_season, last_episode, next_season, next_episode, next_episode_tmdb_id, next_still_path, poster_path)
		  FROM trakt_show_progress ORDER BY 1`,
			[]string{"1396 62 2 1 2 1 3 62087", "95396 19 19 2 10 /t95396.jpg"}},
		{`SELECT concat_ws(' ', show_tmdb_id, season, episode, episode_tmdb_id, still_path, backdrop_path) FROM trakt_calendar`,
			[]string{"95396 3 1 7000001 /t95396-3x1.jpg /t95396-bg.jpg"}},
	} {
		if got := se.rows(c.query); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s\n = %q, want %q", c.query, got, c.want)
		}
	}
	// Artwork only for titles outside the library, once each.
	if got, want := se.art.Calls(), []string{"movie/693134", "tv/95396", "tv/95396/season/3 (Returning Series)"}; !reflect.DeepEqual(sorted(got), want) {
		t.Errorf("artwork lookups = %q, want %q", got, want)
	}

	in, err := se.auth.Integration(se.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if in.Counts != (library.TraktCounts{History: 4, Ratings: 4, Watchlist: 3, WatchlistInLibrary: 1}) ||
		in.LastSyncAt == nil || in.Sync.Status != SyncIdle || ptrValue(in.Sync.Kind) != SyncInitial {
		t.Errorf("integration = counts %+v, last_sync_at %v, sync %+v", in.Counts, in.LastSyncAt, in.Sync)
	}
}

func sorted(s []string) []string {
	s = slices.Clone(s)
	slices.Sort(s)
	return s
}

func TestSync_Incremental(t *testing.T) {
	se := newSyncEnv(t, newFakeClock())
	se.syncNow()

	t.Run("nothing changed is one request", func(t *testing.T) {
		n, calls := se.conn.count(), len(se.art.Calls())
		st := se.syncNow()
		wantRun(t, st.Run, SyncIdle, SyncIncremental)
		if got := paths(se.conn.since(n)); !reflect.DeepEqual(got, []string{"/sync/last_activities"}) {
			t.Errorf("requests = %q", got)
		}
		if len(se.art.Calls()) != calls {
			t.Errorf("artwork looked up again: %q", se.art.Calls()[calls:])
		}
	})

	t.Run("only changed sections are fetched", func(t *testing.T) {
		se.bump("movies", "rated_at")
		se.bump("seasons", "rated_at")
		se.bump("watchlist", "updated_at")
		se.srv.List("/sync/ratings/movies", []obj{})
		n := se.conn.count()
		st := se.syncNow()
		wantRun(t, st.Run, SyncIdle, SyncIncremental)
		want := []string{"/sync/last_activities", "/sync/ratings/movies", "/sync/ratings/seasons",
			"/sync/watchlist/movies/added/desc", "/sync/watchlist/shows/added/desc"}
		if got := paths(se.conn.since(n)); !reflect.DeepEqual(got, want) {
			t.Errorf("requests = %q, want %q", got, want)
		}
		if got := se.rows(`SELECT media_type FROM trakt_ratings ORDER BY 1`); !reflect.DeepEqual(got, []string{"episode", "season", "show"}) {
			t.Errorf("ratings = %q (the movie's is gone, the others stay)", got)
		}
	})

	t.Run("hidden shows refresh the hidden lists and progress", func(t *testing.T) {
		se.bump("shows", "dropped_at")
		se.srv.List("/users/hidden/dropped", []obj{})
		n := se.conn.count()
		se.syncNow()
		want := []string{"/sync/last_activities", "/users/hidden/dropped", "/users/hidden/progress_watched", "/sync/progress/watched"}
		if got := paths(se.conn.since(n)); !reflect.DeepEqual(got, want) {
			t.Errorf("requests = %q, want %q", got, want)
		}
		if got := se.rows(`SELECT section FROM trakt_hidden`); !reflect.DeepEqual(got, []string{"progress_watched"}) {
			t.Errorf("hidden = %q", got)
		}
	})

	t.Run("new plays come as deltas, the overlap deduplicated", func(t *testing.T) {
		se.bump("episodes", "watched_at")
		se.srv.List("/sync/history/episodes", []obj{
			episodePlay(2003, "2026-10-16T12:00:00.000Z", bb, bb1x3),
			episodePlay(2002, "2026-10-15T20:00:00.000Z", bb, bb1x2), // already stored
		})
		n := se.conn.count()
		st := se.syncNow()
		got := se.conn.since(n)
		want := []string{"/sync/last_activities", "/sync/history/episodes", "/sync/progress/watched", "/calendars/my/shows/2026-10-09/33"}
		if !reflect.DeepEqual(paths(got), want) {
			t.Fatalf("requests = %q, want %q", paths(got), want)
		}
		// From an hour before the earlier of the stored activity and the
		// last stored play, to now.
		q := got[1].query
		if q.Get("start_at") != "2026-10-15T19:00:00.000Z" || q.Get("end_at") != isoTime(se.clock.Now()) {
			t.Errorf("history query = %v", q)
		}
		if st.Run.Progress.Plays != 2 || st.Run.Progress.PlaysTotal != 2 {
			t.Errorf("progress = %+v", st.Run.Progress)
		}
		if got := se.rows(`SELECT id::text FROM trakt_history WHERE media_type = 'episode' ORDER BY id`); !reflect.DeepEqual(got, []string{"2001", "2002", "2003"}) {
			t.Errorf("episode plays = %q", got)
		}
		if got := se.rows(`SELECT concat_ws(' ', episode, plays) FROM trakt_watched_episodes ORDER BY episode`); !reflect.DeepEqual(got, []string{"1 1", "2 1", "3 1"}) {
			t.Errorf("watched episodes = %q", got)
		}
	})

	t.Run("a calendar over 12 hours old is refreshed", func(t *testing.T) {
		se.clock.(*fakeClock).Advance(13 * time.Hour)
		n := se.conn.count()
		se.syncNow()
		want := []string{"/sync/last_activities", "/calendars/my/shows/2026-10-10/33"}
		if got := paths(se.conn.since(n)); !reflect.DeepEqual(got, want) {
			t.Errorf("requests = %q, want %q", got, want)
		}
	})
}

func TestSync_ReconcileRemovesDeletedPlays(t *testing.T) {
	se := newSyncEnv(t, newFakeClock())
	se.syncNow()

	// A play deleted on trakt.tv doesn't move watched_at forward: the
	// deltas can't see it, the reconcile does.
	se.srv.List("/sync/history/movies", []obj{moviePlay(1001, "2026-10-10T20:00:00.000Z", matrix)})
	if st := se.syncNow(); ptrValue(st.Run.Kind) != SyncIncremental || se.countRows("trakt_history") != 4 {
		t.Fatalf("incremental sync = %v with %d plays", ptrValue(st.Run.Kind), se.countRows("trakt_history"))
	}
	se.clock.(*fakeClock).Advance(25 * time.Hour)
	st := se.syncNow()
	wantRun(t, st.Run, SyncIdle, SyncReconcile)
	if got := se.rows(`SELECT id::text FROM trakt_history WHERE media_type = 'movie'`); !reflect.DeepEqual(got, []string{"1001"}) {
		t.Errorf("movie plays = %q", got)
	}
	if got := se.rows(`SELECT plays::text FROM trakt_watched_movies`); !reflect.DeepEqual(got, []string{"1"}) {
		t.Errorf("Matrix plays = %q", got)
	}
	state, err := library.NewTraktSyncStore(se.db).State(se.ctx)
	if err != nil || !state.LastReconcileAt.Equal(se.clock.Now()) {
		t.Errorf("last_reconcile_at = %v, %v", state.LastReconcileAt, err)
	}
	// And the next one is incremental again.
	if st := se.syncNow(); ptrValue(st.Run.Kind) != SyncIncremental {
		t.Errorf("after the reconcile: %v", ptrValue(st.Run.Kind))
	}
}

func TestSync_DetailsFillOnlyMissingArtwork(t *testing.T) {
	se := newSyncEnv(t, newFakeClock())
	se.syncNow()
	first := len(se.art.Calls())

	// A reconcile replaces every row: the artwork stays and isn't looked
	// up again, even when TMDB's changed.
	se.art.mu.Lock()
	se.art.version = "-v2"
	se.art.mu.Unlock()
	se.clock.(*fakeClock).Advance(25 * time.Hour)
	se.syncNow()
	if calls := se.art.Calls(); len(calls) != first {
		t.Errorf("artwork looked up again: %q", calls[first:])
	}
	// A new watchlist title outside the library gets its own.
	se.bump("watchlist", "updated_at")
	se.srv.List("/sync/watchlist/shows/added/desc", []obj{
		{"rank": 1, "id": 103, "listed_at": "2026-10-05T10:00:00.000Z", "type": "show", "show": severance},
		{"rank": 2, "id": 104, "listed_at": "2026-10-17T10:00:00.000Z", "type": "show", "show": twd},
	})
	se.syncNow()
	if calls := se.art.Calls(); !reflect.DeepEqual(calls[first:], []string{"tv/1402"}) {
		t.Errorf("new lookups = %q, want tv/1402", calls[first:])
	}
	want := []string{"603 ", "1402 /t1402-v2.jpg", "693134 /m693134.jpg", "95396 /t95396.jpg"}
	if got := se.rows(`SELECT concat(tmdb_id, ' ', poster_path) FROM trakt_watchlist`); !reflect.DeepEqual(sorted(got), sorted(want)) {
		t.Errorf("watchlist posters = %q, want %q", got, want)
	}
}

func TestSync_FailedImportPicksUpAtItsStep(t *testing.T) {
	se := newSyncEnv(t, newFakeClock())
	// Trakt fails the watchlist past the client's retries.
	bad := trakttest.Reply{Status: http.StatusBadGateway}
	se.srv.Queue("/sync/watchlist/movies/added/desc", bad, bad, bad, bad)

	st := se.syncNow()
	wantRun(t, st.Run, SyncError, SyncInitial)
	if ptrValue(st.Run.Step) != StepWatchlist || ptrValue(st.Run.Error) != "Trakt didn’t answer while importing your watchlist (502)." ||
		st.LastSyncAt != nil {
		t.Errorf("failed sync = step %v, error %q, last_sync_at %v", ptrValue(st.Run.Step), ptrValue(st.Run.Error), st.LastSyncAt)
	}
	if se.countRows("trakt_history") != 4 || se.countRows("trakt_ratings") != 4 || se.countRows("trakt_watchlist") != 0 {
		t.Errorf("rows: history %d, ratings %d, watchlist %d", se.countRows("trakt_history"), se.countRows("trakt_ratings"), se.countRows("trakt_watchlist"))
	}
	// Reported after a restart too.
	restarted, err := NewSyncer(SyncDeps{Auth: se.conn, Store: library.NewTraktSyncStore(se.db)}).Status(se.ctx)
	if err != nil || restarted.Run.Status != SyncError || ptrValue(restarted.Run.Step) != StepWatchlist {
		t.Errorf("stored run = %+v, %v", restarted.Run, err)
	}

	n := se.conn.count()
	st = se.syncNow()
	wantRun(t, st.Run, SyncIdle, SyncInitial)
	want := []string{"/sync/watchlist/movies/added/desc", "/sync/watchlist/shows/added/desc", "/users/hidden/dropped",
		"/users/hidden/progress_watched", "/sync/progress/watched", "/calendars/my/shows/2026-10-09/33"}
	if got := paths(se.conn.since(n)); !reflect.DeepEqual(got, want) {
		t.Errorf("requests = %q, want %q", got, want)
	}
	if st.Run.Progress.Plays != 5 || st.Run.Progress.Watchlist != 3 || st.LastSyncAt == nil {
		t.Errorf("resumed import = %+v, last_sync_at %v", st.Run.Progress, st.LastSyncAt)
	}
	// It stored the snapshot it started with: the next sync is one request.
	n = se.conn.count()
	if st := se.syncNow(); ptrValue(st.Run.Kind) != SyncIncremental || se.conn.count() != n+1 {
		t.Errorf("next sync = %v with %q", ptrValue(st.Run.Kind), paths(se.conn.since(n)))
	}
}

func TestSync_Unauthorized(t *testing.T) {
	t.Run("one 401 refreshes and retries", func(t *testing.T) {
		se := newSyncEnv(t, newFakeClock())
		se.srv.Queue("/sync/last_activities", trakttest.Reply{Status: http.StatusUnauthorized})
		st := se.syncNow()
		wantRun(t, st.Run, SyncIdle, SyncInitial)
		if se.srv.Count("/oauth/token") != 1 || se.srv.Count("/sync/last_activities") != 2 || !se.auth.Connected() {
			t.Errorf("refreshes %d, last_activities %d, connected %v", se.srv.Count("/oauth/token"), se.srv.Count("/sync/last_activities"), se.auth.Connected())
		}
	})
	t.Run("a second 401 is reauth", func(t *testing.T) {
		se := newSyncEnv(t, newFakeClock())
		se.syncNow()
		unauthorized := trakttest.Reply{Status: http.StatusUnauthorized}
		se.srv.Queue("/sync/last_activities", unauthorized, unauthorized)
		st := se.syncNow()
		wantRun(t, st.Run, SyncIdle, SyncIncremental)
		in, err := se.auth.Integration(se.ctx)
		if err != nil {
			t.Fatal(err)
		}
		if in.State != StateReauth || in.Sync.Error != nil || in.Counts.History != 4 {
			t.Errorf("integration = %s, sync error %v, history %d (the data stays)", in.State, ptrValue(in.Sync.Error), in.Counts.History)
		}
		if _, err := se.auth.SyncNow(se.ctx); err != ErrNotConnected {
			t.Errorf("SyncNow in reauth = %v, want ErrNotConnected", err)
		}
	})
}

func TestSync_DisconnectMidSyncLeavesNoRows(t *testing.T) {
	se := newSyncEnv(t, newFakeClock())
	arrived, release := make(chan struct{}, 1), make(chan struct{})
	t.Cleanup(func() { close(release) })
	se.srv.Handle("/sync/watchlist/movies/added/desc", func(w http.ResponseWriter, r *http.Request) {
		select {
		case arrived <- struct{}{}:
		default:
		}
		select {
		case <-release:
		case <-r.Context().Done():
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	seq := se.seq()
	if _, err := se.auth.SyncNow(se.ctx); err != nil {
		t.Fatal(err)
	}
	<-arrived // history and ratings are stored
	if se.countRows("trakt_history") == 0 {
		t.Fatal("no history before the disconnect")
	}
	if err := se.auth.Disconnect(se.ctx); err != nil {
		t.Fatal(err)
	}
	se.waitRuns(seq, 1)

	var rows int
	for _, table := range []string{"trakt_history", "trakt_watched_movies", "trakt_watched_episodes", "trakt_ratings",
		"trakt_watchlist", "trakt_show_progress", "trakt_hidden", "trakt_calendar", "trakt_sync_state", "trakt_auth"} {
		rows += se.countRows(table)
	}
	st := se.status()
	if rows != 0 || st.Run.Status != "" || st.LastSyncAt != nil {
		t.Errorf("after a disconnect: %d rows, sync %+v, last_sync_at %v", rows, st.Run, st.LastSyncAt)
	}
	if err := se.syncer.SyncNow(se.ctx); err != ErrNotConnected {
		t.Errorf("a sync after the disconnect = %v, want ErrNotConnected", err)
	}
}

func TestSync_QueuedTriggerAndNudge(t *testing.T) {
	se := newSyncEnv(t, newFakeClock())
	se.syncNow()

	arrived, release := make(chan struct{}, 4), make(chan struct{})
	se.srv.Handle("/sync/history/movies", func(w http.ResponseWriter, _ *http.Request) {
		arrived <- struct{}{}
		<-release
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("[]"))
	})
	se.bump("movies", "watched_at")
	seq := se.seq()
	if err := se.syncer.SyncNow(se.ctx); err != nil {
		t.Fatal(err)
	}
	<-arrived
	// Triggers during a sync queue exactly one more; Nudge waits for it.
	for range 3 {
		if err := se.syncer.SyncNow(se.ctx); err != nil {
			t.Fatal(err)
		}
	}
	se.syncer.Nudge()
	if st := se.status(); st.Run.Status != SyncSyncing || ptrValue(st.Run.Step) != StepHistory {
		t.Errorf("while running: %+v", st.Run)
	}
	close(release)
	se.waitRuns(seq, 2)
	time.Sleep(20 * time.Millisecond)
	if n := se.seq() - seq; n != 2 {
		t.Errorf("syncs = %d, want 2", n)
	}

	// Nudge syncs only once the last check is 5 minutes old.
	n := se.conn.count()
	se.syncer.Nudge()
	if se.conn.count() != n || se.seq() != seq+2 {
		t.Fatal("Nudge synced right after a sync")
	}
	se.clock.(*fakeClock).Advance(6 * time.Minute)
	se.syncer.Nudge()
	se.waitRuns(seq+2, 1)
	if got := paths(se.conn.since(n)); !reflect.DeepEqual(got, []string{"/sync/last_activities"}) {
		t.Errorf("nudged sync requests = %q", got)
	}
}

func TestSync_ConnectStartsTheInitialImport(t *testing.T) {
	e := newEnv(t)
	art := &fakeArtwork{steps: map[SyncStep]bool{}}
	syncer := NewSyncer(SyncDeps{Auth: e.auth, Store: library.NewTraktSyncStore(e.db), Artwork: art, Clock: e.clock})
	t.Cleanup(syncer.Stop)
	e.auth.SetHooks(syncer.Hooks())
	se := &syncEnv{t: t, ctx: e.ctx, clock: e.clock, srv: e.srv, db: e.db, syncer: syncer}
	se.serveAccount()

	e.connect(e.auth)
	st := se.waitRuns(0, 1)
	wantRun(t, st.Run, SyncIdle, SyncInitial)
	if se.countRows("trakt_history") != 4 || se.countRows("trakt_calendar") != 1 {
		t.Errorf("imported history %d, calendar %d", se.countRows("trakt_history"), se.countRows("trakt_calendar"))
	}
}

// manualClock is time that moves only when the test says: Sleep waits
// for Advance.
type manualClock struct {
	mu       sync.Mutex
	now      time.Time
	sleepers map[chan struct{}]time.Time
}

func newManualClock() *manualClock {
	return &manualClock{now: time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC), sleepers: map[chan struct{}]time.Time{}}
}

func (c *manualClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *manualClock) Sleep(ctx context.Context, d time.Duration) error {
	c.mu.Lock()
	wake := make(chan struct{})
	c.sleepers[wake] = c.now.Add(d)
	c.mu.Unlock()
	select {
	case <-wake:
		return nil
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.sleepers, wake)
		c.mu.Unlock()
		return ctx.Err()
	}
}

func (c *manualClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
	for wake, until := range c.sleepers {
		if !until.After(c.now) {
			close(wake)
			delete(c.sleepers, wake)
		}
	}
}

func (c *manualClock) sleeping() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.sleepers)
}

func TestSync_Schedule(t *testing.T) {
	clock := newManualClock()
	se := newSyncEnv(t, clock)
	se.syncer.Start()

	// tick waits for the schedule to sleep, moves the clock on and waits
	// for the sync it starts.
	tick := func(d time.Duration) SyncStatus {
		t.Helper()
		waitFor(t, "the schedule to wait", func() bool { return clock.sleeping() == 1 })
		seq := se.seq()
		clock.Advance(d)
		return se.waitRuns(seq, 1)
	}

	waitFor(t, "the schedule to wait", func() bool { return clock.sleeping() == 1 })
	if st := se.status(); st.NextSyncAt == nil || !st.NextSyncAt.Equal(clock.Now().Add(time.Minute)) || se.conn.count() != 0 {
		t.Fatalf("before the first check: next %v, %d requests", st.NextSyncAt, se.conn.count())
	}
	start := clock.Now()
	wantRun(t, tick(time.Minute).Run, SyncIdle, SyncInitial)

	n := se.conn.count()
	st := tick(30 * time.Minute)
	wantRun(t, st.Run, SyncIdle, SyncIncremental)
	if se.conn.count() != n+1 {
		t.Errorf("a check with nothing changed = %q", paths(se.conn.since(n)))
	}
	waitFor(t, "the schedule to wait", func() bool { return clock.sleeping() == 1 })
	if st := se.status(); st.NextSyncAt == nil || !st.NextSyncAt.Equal(start.Add(61*time.Minute)) {
		t.Errorf("next_sync_at = %v, want %v", st.NextSyncAt, start.Add(61*time.Minute))
	}

	// Over a day after the last reconcile, the check is a reconcile.
	wantRun(t, tick(24*time.Hour).Run, SyncIdle, SyncReconcile)
	wantRun(t, tick(30*time.Minute).Run, SyncIdle, SyncIncremental)
}

func TestSyncErrorMessage(t *testing.T) {
	tests := []struct {
		step SyncStep
		err  error
		want string
	}{
		{StepWatchlist, fmt.Errorf("wrapped: %w", &StatusError{StatusCode: 502}), "Trakt didn’t answer while importing your watchlist (502)."},
		{StepHistory, fmt.Errorf("trakt GET /sync/history/movies: %w", &url.Error{Op: "Get", URL: "https://api.trakt.tv", Err: context.DeadlineExceeded}),
			"Couldn’t reach Trakt while importing your history."},
		{StepDetails, fmt.Errorf("failed to store the Trakt artwork: boom"), "The sync stopped while importing artwork."},
	}
	for _, tt := range tests {
		if got := syncErrorMessage(tt.step, tt.err); got != tt.want {
			t.Errorf("%s: %q, want %q", tt.step, got, tt.want)
		}
		if strings.Contains(syncErrorMessage(tt.step, tt.err), "api.trakt.tv") {
			t.Error("the message leaks the URL")
		}
	}
}
