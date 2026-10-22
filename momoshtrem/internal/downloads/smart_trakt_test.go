package downloads_test

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/anacrolix/torrent/metainfo"

	"github.com/shapedtime/momoshtrem/internal/downloads"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/me"
	"github.com/shapedtime/momoshtrem/internal/member"
	"github.com/shapedtime/momoshtrem/internal/secret"
	"github.com/shapedtime/momoshtrem/internal/sessions"
	"github.com/shapedtime/momoshtrem/internal/storagecap"
	"github.com/shapedtime/momoshtrem/internal/torrent/piececache"
	"github.com/shapedtime/momoshtrem/internal/trakt"
	"github.com/shapedtime/momoshtrem/internal/trakt/trakttest"
)

// Smart picks from Trakt's Up next, end to end: a fake Trakt (trakttest)
// connected and synced into a real Postgres, Up next read as /me reads it
// (me.UpNextEpisodes), and the download manager. It's an external test so
// it can import me (whose catalog may come to import this package).

type obj = map[string]any

var (
	breakingBad = obj{"title": "Breaking Bad", "year": 2008, "ids": obj{"trakt": 1388, "tmdb": 1396}}
)

func traktEpisode(n int) obj {
	return obj{"season": 1, "number": n, "title": fmt.Sprintf("Episode %d", n), "ids": obj{"trakt": 73480 + n}}
}

func play(id, n int, at time.Time) obj {
	return obj{"id": id, "watched_at": at.Format(time.RFC3339), "action": "scrobble", "type": "episode",
		"episode": traktEpisode(n), "show": breakingBad}
}

// testClock is the manager's and Trakt's time: the test moves it; timers
// never fire, and Trakt's waits (the device code's poll) end at once.
type testClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *testClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *testClock) After(time.Duration) <-chan time.Time { return make(chan time.Time) }

func (c *testClock) Sleep(ctx context.Context, _ time.Duration) error { return ctx.Err() }

func (c *testClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// idleClient is a torrent client with nothing loaded that loads nothing:
// the picks are what's checked here, not the fetching.
type idleClient struct{}

func (idleClient) Loaded() (map[string]int64, error)              { return map[string]int64{}, nil }
func (idleClient) Info(string, string) *metainfo.Info             { return nil }
func (idleClient) Load(string) error                              { return nil }
func (idleClient) PausedAt(string) (time.Time, bool)              { return time.Time{}, false }
func (idleClient) MarkActive(string)                              {}
func (idleClient) SetPriorities(string, []string, []string) error { return nil }
func (idleClient) Completion(string, string) ([]bool, error)      { return nil, nil }
func (idleClient) UpdateCompletion(string, string, []int) error   { return nil }
func (idleClient) Progress(string, []string) int64                { return 0 }

// streams are the streaming sessions the test sets.
type streams struct {
	mu   sync.Mutex
	list []sessions.Session
}

func (s *streams) Sessions(context.Context) []sessions.Session {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]sessions.Session(nil), s.list...)
}

func (s *streams) set(list ...sessions.Session) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.list = list
}

// upNext is Up next as main adapts it (wire_smart.go) for one member: the
// seeded member's by default.
type upNext struct {
	me     *me.Service
	member int64
}

func (u upNext) UpNextShows(ctx context.Context) ([]downloads.UpNextShow, error) {
	eps, err := u.me.UpNextEpisodes(member.WithID(ctx, cmp.Or(u.member, 1)))
	out := make([]downloads.UpNextShow, len(eps))
	for i, e := range eps {
		out[i] = downloads.UpNextShow{ShowID: e.ShowID, SeasonNumber: e.SeasonNumber, EpisodeNumber: e.EpisodeNumber,
			LastWatchedAt: e.LastWatchedAt}
	}
	return out, err
}

func openSchemaDB(t *testing.T) *library.DB {
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
	schema := fmt.Sprintf("smart_trakt_%d", time.Now().UnixNano())
	if _, err := base.Exec(`CREATE SCHEMA ` + schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = base.Exec(`DROP SCHEMA ` + schema + ` CASCADE`) })
	sep := "?"
	if strings.Contains(url, "?") {
		sep = "&"
	}
	db, err := library.NewDB(url + sep + "search_path=" + schema)
	if err != nil {
		t.Fatalf("NewDB in %s: %v", schema, err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// account is the fake Trakt account: Breaking Bad watched to plays, with
// last_activities moving with each change.
type account struct {
	srv *trakttest.Server
	mu  sync.Mutex
	// episodesAt is last_activities' episodes.watched_at.
	episodesAt time.Time
}

func (a *account) serve(clock *testClock) {
	a.srv.Handle("/sync/last_activities", func(w http.ResponseWriter, _ *http.Request) {
		a.mu.Lock()
		at := a.episodesAt.Format(time.RFC3339)
		a.mu.Unlock()
		old := "2026-01-01T00:00:00.000Z"
		body, _ := json.Marshal(obj{"all": at,
			"movies":    obj{"watched_at": old, "rated_at": old, "watchlisted_at": old, "collected_at": old},
			"episodes":  obj{"watched_at": at, "rated_at": old, "paused_at": old},
			"shows":     obj{"rated_at": old, "watchlisted_at": old, "hidden_at": old, "dropped_at": old},
			"seasons":   obj{"rated_at": old, "hidden_at": old},
			"watchlist": obj{"updated_at": old},
		})
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	})
	for _, path := range []string{"/sync/history/movies", "/sync/ratings/movies", "/sync/ratings/shows", "/sync/ratings/seasons",
		"/sync/ratings/episodes", "/sync/watchlist/movies/added/desc", "/sync/watchlist/shows/added/desc",
		"/users/hidden/dropped", "/users/hidden/progress_watched"} {
		a.srv.List(path, []obj{})
	}
	day := clock.Now().AddDate(0, 0, -7)
	for i := range 3 {
		a.srv.Handle("/calendars/my/shows/"+day.AddDate(0, 0, i).Format(time.DateOnly)+"/33", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[]`))
		})
	}
}

// watched has Trakt hold plays of the first n episodes, the last at at, and
// name episode n+1 next.
func (a *account) watched(n int, at time.Time) { a.watchedBy("", n, at) }

// watchedBy is watched for the user with slug only (every user's for "").
func (a *account) watchedBy(slug string, n int, at time.Time) {
	list := a.srv.List
	if slug != "" {
		list = func(path string, items any) { a.srv.ListFor(slug, path, items) }
	}
	var plays []obj
	for e := n; e >= 1; e-- {
		plays = append(plays, play(1000+e, e, at.Add(-time.Duration(n-e)*time.Hour)))
	}
	list("/sync/history/episodes", plays)
	list("/sync/progress/watched", []obj{{"show": breakingBad, "progress": obj{
		"aired": 7, "completed": n, "last_watched_at": at.Format(time.RFC3339), "reset_at": nil,
		"next_episode": traktEpisode(n + 1), "last_episode": traktEpisode(n), "stats": obj{"play_count": n}}}})
	a.mu.Lock()
	if at.After(a.episodesAt) {
		a.episodesAt = at
	}
	a.mu.Unlock()
}

type pickRow struct {
	Episode int
	Source  library.SmartSource
	Release library.ReleaseReason
}

func pickRows(t *testing.T, db *library.DB) []pickRow {
	t.Helper()
	rows, err := db.Query(`
		SELECT e.episode_number, d.smart_source, COALESCE(d.release_reason, '')
		FROM downloads d JOIN episodes e ON e.id = d.episode_id
		WHERE d.reason = 'smart' ORDER BY d.id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []pickRow
	for rows.Next() {
		var r pickRow
		if err := rows.Scan(&r.Episode, &r.Source, &r.Release); err != nil {
			t.Fatal(err)
		}
		out = append(out, r)
	}
	return out
}

// smartWorld is Breaking Bad's first season in the library, every episode
// a file of one torrent, smart downloads on (2 episodes ahead), and the
// download manager over a real piece cache and governor.
type smartWorld struct {
	ctx   context.Context
	db    *library.DB
	clock *testClock
	repo  *library.DownloadsRepository
	svc   *downloads.Service
	live  *streams
	show  int64
	hash  string
}

func newSmartWorld(t *testing.T) *smartWorld {
	t.Helper()
	ctx := context.Background()
	db := openSchemaDB(t)
	clock := &testClock{now: time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)}
	settings := library.NewSettingsRepository(db)
	repo := library.NewDownloadsRepository(db)

	// The library: Breaking Bad's first season, every episode a file of
	// one torrent.
	var show int64
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := db.Exec(query, args...); err != nil {
			t.Fatalf("%s: %v", query, err)
		}
	}
	if err := db.QueryRow(`INSERT INTO shows (tmdb_id, title, year, status) VALUES (1396, 'Breaking Bad', 2008, 'Ended') RETURNING id`).Scan(&show); err != nil {
		t.Fatal(err)
	}
	exec(`INSERT INTO seasons (show_id, season_number) VALUES ($1, 1)`, show)
	hash := strings.Repeat("ab", 20)
	for e := 1; e <= 7; e++ {
		exec(`INSERT INTO episodes (season_id, episode_number, name, air_date)
			SELECT id, $2, $3, '2008-01-20' FROM seasons WHERE show_id = $1`, show, e, fmt.Sprintf("Episode %d", e))
		exec(`INSERT INTO torrent_assignments (item_type, item_id, info_hash, magnet_uri, file_path, file_size)
			SELECT 'episode', e.id, $2, $3, $4, 1000 FROM episodes e JOIN seasons sn ON sn.id = e.season_id
			WHERE sn.show_id = $1 AND e.episode_number = $5`,
			show, hash, "magnet:?xt=urn:btih:"+hash, fmt.Sprintf("Breaking Bad S01/E%02d.mkv", e), e)
	}
	patch, err := storagecap.ParsePatch([]byte(`{"smart":{"enabled":true,"episodes_ahead":2}}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := settings.UpdateStorageSettings(ctx, patch); err != nil {
		t.Fatal(err)
	}

	// The download manager over a real piece cache and governor.
	gov := storagecap.New(storagecap.Deps{Settings: settings, DiskPath: "/data",
		Statfs: func(string) (int64, int64, error) { return 200 << 30, 160 << 30, nil }})
	cache, err := piececache.NewCache(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	live := &streams{}
	svc := downloads.New(downloads.Deps{Store: repo, Limits: gov, Cache: cache, Client: idleClient{},
		Sessions: live, Clock: clock, Location: time.UTC})
	gov.SetDownloads(svc)
	return &smartWorld{ctx: ctx, db: db, clock: clock, repo: repo, svc: svc, live: live, show: show, hash: hash}
}

// run is a smart pass, then a manager pass.
func (w *smartWorld) run(t *testing.T, picker *downloads.SmartPicker) {
	t.Helper()
	if err := picker.Run(w.ctx); err != nil {
		t.Fatal(err)
	}
	if err := w.svc.Refresh(w.ctx); err != nil {
		t.Fatal(err)
	}
}

func (w *smartWorld) want(t *testing.T, when string, rows ...pickRow) {
	t.Helper()
	if got := pickRows(t, w.db); !reflect.DeepEqual(got, rows) {
		t.Errorf("%s: picks = %v, want %v", when, got, rows)
	}
}

// traktClient is a client of the fake Trakt on clock.
func traktClient(srv *trakttest.Server, clock trakt.Clock) *trakt.Client {
	return trakt.NewClient(trakttest.ClientID, trakttest.ClientSecret, "test",
		trakt.WithBaseURLs(srv.URL, srv.URL), trakt.WithClock(clock), trakt.WithLimiters(nil, nil))
}

func traktBox(t *testing.T) *secret.Box {
	t.Helper()
	box, err := secret.New(strings.Repeat("0f", 32))
	if err != nil {
		t.Fatal(err)
	}
	return box
}

// Up next makes the picks; a watch releases one and the next joins; a
// dismissed pick isn't picked again; with Trakt disconnected, Up next's
// picks expire and the streaming signal alone picks.
func TestSmart_UpNextFromTrakt(t *testing.T) {
	w := newSmartWorld(t)
	ctx, db, clock, repo, svc, live, show, hash := w.ctx, w.db, w.clock, w.repo, w.svc, w.live, w.show, w.hash

	// Trakt, connected through the device code; each sync tells.
	srv := trakttest.New(t)
	srv.SetClock(clock.Now)
	acct := &account{srv: srv}
	acct.serve(clock)
	acct.watched(2, clock.Now().Add(-48*time.Hour))
	auth := trakt.NewAuth(trakt.AuthDeps{
		Client: traktClient(srv, clock), Box: traktBox(t),
		Store: library.NewTraktAuthRepository(db).ForMember(1), Settings: library.NewMemberRepository(db).TraktSettingsFor(1),
		Clock: clock,
	})
	t.Cleanup(auth.Close)
	syncer := trakt.NewSyncer(trakt.SyncDeps{Auth: auth, Store: library.NewTraktSyncStore(db).ForMember(1), Clock: clock})
	auth.SetHooks(syncer.Hooks())
	t.Cleanup(syncer.Stop)
	synced := make(chan struct{}, 4)
	syncer.SetOnSynced(func() { synced <- struct{}{} })
	waitSynced := func(what string) {
		t.Helper()
		select {
		case <-synced:
		case <-time.After(10 * time.Second):
			t.Fatalf("%s: the Trakt sync never ended", what)
		}
	}

	picker := downloads.NewSmartPicker(svc, downloads.SmartDeps{Store: repo, UpNext: upNext{me: me.New(me.Deps{
		Store: library.NewCatalogRepository(db), Settings: library.NewMemberRepository(db), Now: clock.Now})}})
	run := func() {
		t.Helper()
		w.run(t, picker)
	}
	want := func(when string, rows ...pickRow) {
		t.Helper()
		w.want(t, when, rows...)
	}

	run()
	want("before Trakt")

	if _, err := auth.Connect(ctx); err != nil {
		t.Fatal(err)
	}
	waitSynced("the initial import")
	run()
	want("Up next names E3", pickRow{3, library.SmartUpNext, ""}, pickRow{4, library.SmartUpNext, ""})
	list := svc.Snapshot().List()
	if len(list.Items) != 2 || list.Items[0].Reason != library.DownloadSmart || *list.Items[0].Item.EpisodeNumber != 3 ||
		list.Items[0].ExpiresAt == nil || !list.Items[0].ExpiresAt.Equal(clock.Now().Add(downloads.SmartLifetime)) {
		t.Errorf("GET /downloads lists %+v", list.Items)
	}

	// E3 watched (on another device): the planner releases it, E5 joins.
	clock.advance(2 * time.Hour)
	acct.watched(3, clock.Now().Add(-time.Hour))
	if _, err := auth.SyncNow(ctx); err != nil {
		t.Fatal(err)
	}
	waitSynced("the sync after E3")
	run()
	want("E3 watched",
		pickRow{3, library.SmartUpNext, library.ReleaseWatched}, pickRow{4, library.SmartUpNext, ""}, pickRow{5, library.SmartUpNext, ""})

	// E5 dismissed: never picked again, and not counted, so E6 joins.
	var e5 int64
	if err := db.QueryRow(`SELECT d.id FROM downloads d JOIN episodes e ON e.id = d.episode_id
		WHERE d.released_at IS NULL AND e.episode_number = 5`).Scan(&e5); err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(ctx, e5); err != nil {
		t.Fatal(err)
	}
	run()
	dismissed := []pickRow{{3, library.SmartUpNext, library.ReleaseWatched}, {4, library.SmartUpNext, ""},
		{5, library.SmartUpNext, library.ReleaseDeleted}, {6, library.SmartUpNext, ""}}
	want("E5 dismissed", dismissed...)

	// Trakt disconnected: Up next is gone, and so are its picks. Streaming
	// E4 picks what follows it: E6 at once (released early, so it isn't
	// held back) and E7; E5 still left out.
	if err := auth.Disconnect(ctx); err != nil {
		t.Fatal(err)
	}
	run()
	want("Trakt off",
		pickRow{3, library.SmartUpNext, library.ReleaseWatched}, pickRow{4, library.SmartUpNext, library.ReleaseExpired},
		pickRow{5, library.SmartUpNext, library.ReleaseDeleted}, pickRow{6, library.SmartUpNext, library.ReleaseExpired})
	season, episode := 1, 4
	path := "Breaking Bad S01/E04.mkv"
	live.set(sessions.Session{ID: "s1", Source: sessions.SourceTorrent, InfoHash: &hash, FilePath: &path, SizeBytes: 1000,
		Item: &sessions.Item{MediaType: "tv", LibraryID: show, Title: "Breaking Bad", SeasonNumber: &season, EpisodeNumber: &episode}})
	if err := svc.Refresh(ctx); err != nil { // the manager reads the sessions
		t.Fatal(err)
	}
	run()
	want("streaming E4 without Trakt",
		pickRow{3, library.SmartUpNext, library.ReleaseWatched}, pickRow{4, library.SmartUpNext, library.ReleaseExpired},
		pickRow{5, library.SmartUpNext, library.ReleaseDeleted}, pickRow{6, library.SmartUpNext, library.ReleaseExpired},
		pickRow{6, library.SmartStreaming, ""}, pickRow{7, library.SmartStreaming, ""})
}

// pollClock is testClock for a registry, whose syncs run a schedule: a
// wait of a minute or more (a schedule's) lasts until its context ends;
// a shorter one (the device code's poll) ends at once.
type pollClock struct{ *testClock }

func (c pollClock) Sleep(ctx context.Context, d time.Duration) error {
	if d >= time.Minute {
		<-ctx.Done()
	}
	return ctx.Err()
}

// householdUpNext is Up next as main adapts it with several members
// (wire_smart.go): every member's, by member id.
type householdUpNext struct {
	me  *me.Service
	ids []int64
}

func (u householdUpNext) UpNextShows(ctx context.Context) ([]downloads.UpNextShow, error) {
	var out []downloads.UpNextShow
	for _, id := range u.ids {
		shows, err := upNext{me: u.me, member: id}.UpNextShows(ctx)
		if err != nil {
			return nil, err
		}
		out = append(out, shows...)
	}
	return out, nil
}

// Two members watch Breaking Bad, each on their own Trakt account (#244):
// the picks read both Up nexts, and the show anchors once, at the next
// episode of whoever watched it last. Episodes anyone watched are passed
// over. A member's disconnect takes their Up next and their plays with it.
func TestSmart_UpNextFromTwoMembers(t *testing.T) {
	w := newSmartWorld(t)
	ctx, clock := w.ctx, w.clock
	members := library.NewMemberRepository(w.db)
	bob, err := members.Create(ctx, "Bob", "")
	if err != nil {
		t.Fatal(err)
	}
	ann := int64(1)

	// Ann watched E1–E2 two days ago, Bob E1–E4 yesterday.
	srv := trakttest.New(t)
	srv.SetClock(clock.Now)
	acct := &account{srv: srv}
	acct.serve(clock)
	acct.watchedBy("ann", 2, clock.Now().Add(-48*time.Hour))
	acct.watchedBy("bob", 4, clock.Now().Add(-24*time.Hour))
	auths, syncs := library.NewTraktAuthRepository(w.db), library.NewTraktSyncStore(w.db)
	reg := trakt.NewRegistry(trakt.RegistryDeps{
		Client: traktClient(srv, pollClock{clock}), Box: traktBox(t), Clock: pollClock{clock},
		Auths:    func(id int64) trakt.Store { return auths.ForMember(id) },
		Syncs:    func(id int64) trakt.SyncStore { return syncs.ForMember(id) },
		Settings: func(id int64) trakt.Settings { return members.TraktSettingsFor(id) },
	})
	t.Cleanup(reg.Stop)
	reg.Start(ctx, []int64{ann, bob.ID})
	synced := make(chan struct{}, 8)
	reg.SetOnSynced(func() { synced <- struct{}{} })
	waitSynced := func(what string) {
		t.Helper()
		select {
		case <-synced:
		case <-time.After(10 * time.Second):
			t.Fatalf("%s never ended", what)
		}
	}
	connect := func(id int64, username, slug string) {
		t.Helper()
		srv.SetUser(username, slug)
		if _, err := reg.Connect(member.WithID(ctx, id)); err != nil {
			t.Fatal(err)
		}
		waitSynced(username + "'s import")
	}
	connect(ann, "Ann", "ann")
	connect(bob.ID, "Bob", "bob")

	picker := downloads.NewSmartPicker(w.svc, downloads.SmartDeps{Store: w.repo, UpNext: householdUpNext{
		me:  me.New(me.Deps{Store: library.NewCatalogRepository(w.db), Settings: members, Now: clock.Now}),
		ids: []int64{ann, bob.ID}}})
	w.run(t, picker)
	w.want(t, "Bob watched last: from his E5", pickRow{5, library.SmartUpNext, ""}, pickRow{6, library.SmartUpNext, ""})

	// Ann catches up to E3 now: she watched last, and her next, E4, Bob
	// watched, so the window still starts at E5.
	clock.advance(time.Hour)
	acct.watchedBy("ann", 3, clock.Now().Add(-time.Minute))
	if _, err := reg.SyncNow(member.WithID(ctx, ann)); err != nil {
		t.Fatal(err)
	}
	waitSynced("Ann's sync")
	w.run(t, picker)
	w.want(t, "Ann watched last, E4 watched by Bob", pickRow{5, library.SmartUpNext, ""}, pickRow{6, library.SmartUpNext, ""})

	// Bob disconnects: his plays go, and Ann's Up next alone picks from
	// her E4.
	if err := reg.Disconnect(member.WithID(ctx, bob.ID)); err != nil {
		t.Fatal(err)
	}
	w.run(t, picker)
	w.want(t, "Bob disconnected",
		pickRow{5, library.SmartUpNext, ""}, pickRow{6, library.SmartUpNext, library.ReleaseExpired}, pickRow{4, library.SmartUpNext, ""})
}
