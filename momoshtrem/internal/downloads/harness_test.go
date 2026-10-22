package downloads

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/metainfo"

	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/sessions"
	"github.com/shapedtime/momoshtrem/internal/storagecap"
	"github.com/shapedtime/momoshtrem/internal/torrent"
	"github.com/shapedtime/momoshtrem/internal/torrent/piececache"
)

// The integration harness: a private, freshly migrated schema; a real
// piece cache, metainfo store and storage governor (over a made-up disk);
// a fake torrent client, sessions and clock.

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
	schema := fmt.Sprintf("downloads_%d", time.Now().UnixNano())
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

// fakeClock is time the test moves; its timers never fire (the tests run
// passes themselves).
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) After(time.Duration) <-chan time.Time { return make(chan time.Time) }

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// fakeClient is the torrent client: what it can load, what's loaded, the
// priorities set, pauses, wakes, and what it believes is complete.
type fakeClient struct {
	mu       sync.Mutex
	infos    map[string]*metainfo.Info
	loaded   map[string]int64 // speed
	failLoad map[string]bool
	loads    chan string
	normal   map[string]map[string]bool // hash → paths at Normal
	paused   map[string]bool
	marks    map[string]int
	complete map[string][]bool
	updated  map[string][]int
	progress map[string]int64
}

func newFakeClient() *fakeClient {
	return &fakeClient{infos: map[string]*metainfo.Info{}, loaded: map[string]int64{}, failLoad: map[string]bool{},
		loads: make(chan string, 64), normal: map[string]map[string]bool{}, paused: map[string]bool{},
		marks: map[string]int{}, complete: map[string][]bool{}, updated: map[string][]int{}, progress: map[string]int64{}}
}

func (c *fakeClient) Loaded() (map[string]int64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := map[string]int64{}
	for h, s := range c.loaded {
		out[h] = s
	}
	return out, nil
}

func (c *fakeClient) Info(hash, _ string) *metainfo.Info {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.loaded[hash]; !ok {
		return nil
	}
	return c.infos[hash]
}

func (c *fakeClient) Load(magnet string) error {
	hash := torrent.ExtractInfoHash(magnet)
	c.mu.Lock()
	fail := c.failLoad[hash]
	if !fail {
		c.loaded[hash] = 0
	}
	c.mu.Unlock()
	c.loads <- hash
	if fail {
		return torrent.ErrMetadataTimeout
	}
	return nil
}

func (c *fakeClient) PausedAt(hash string) (time.Time, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return time.Time{}, c.paused[hash]
}

func (c *fakeClient) MarkActive(hash string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.marks[hash]++
}

func (c *fakeClient) SetPriorities(hash string, normal, none []string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.normal[hash] == nil {
		c.normal[hash] = map[string]bool{}
	}
	for _, p := range none {
		delete(c.normal[hash], p)
	}
	for _, p := range normal {
		c.normal[hash][p] = true
	}
	return nil
}

func (c *fakeClient) Completion(hash, _ string) ([]bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]bool(nil), c.complete[hash]...), nil
}

func (c *fakeClient) UpdateCompletion(hash, _ string, pieces []int) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.updated[hash] = append(c.updated[hash], pieces...)
	for _, i := range pieces {
		c.complete[hash][i] = false
	}
	return nil
}

func (c *fakeClient) Progress(hash string, _ []string) int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.progress[hash]
}

func (c *fakeClient) isNormal(hash, path string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.normal[hash][path]
}

func (c *fakeClient) markCount(hash string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.marks[hash]
}

func (c *fakeClient) set(f func(c *fakeClient)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	f(c)
}

// fakeSessions are the streaming sessions the test sets.
type fakeSessions struct {
	mu   sync.Mutex
	list []sessions.Session
}

func (f *fakeSessions) Sessions(context.Context) []sessions.Session {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]sessions.Session(nil), f.list...)
}

func (f *fakeSessions) set(list ...sessions.Session) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.list = list
}

// testTorrent is a torrent the harness built: its info, hash and files.
type testTorrent struct {
	info *metainfo.Info
	hash string
	pm   *PieceMap
}

func (tt testTorrent) magnet() string { return "magnet:?xt=urn:btih:" + tt.hash }
func (tt testTorrent) path(i int) string {
	return tt.info.Name + "/" + fileName(i)
}

type harness struct {
	t        *testing.T
	ctx      context.Context
	db       *library.DB
	repo     *library.DownloadsRepository
	settings *library.SettingsRepository
	gov      *storagecap.Governor
	cache    *piececache.Cache
	meta     *torrent.MetainfoStore
	client   *fakeClient
	sessions *fakeSessions
	clock    *fakeClock
	disk     struct {
		mu          sync.Mutex
		total, free int64
	}
	svc      *Service
	restored int
}

const gib = int64(1) << 30

func newHarness(t *testing.T) *harness {
	t.Helper()
	h := &harness{t: t, ctx: context.Background(), db: openSchemaDB(t), client: newFakeClient(),
		sessions: &fakeSessions{}, clock: &fakeClock{now: time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)}}
	h.repo = library.NewDownloadsRepository(h.db)
	h.settings = library.NewSettingsRepository(h.db)
	h.disk.total, h.disk.free = 200*gib, 160*gib
	h.gov = storagecap.New(storagecap.Deps{Settings: h.settings, DiskPath: "/data", Statfs: func(string) (int64, int64, error) {
		h.disk.mu.Lock()
		defer h.disk.mu.Unlock()
		return h.disk.total, h.disk.free, nil
	}})
	var err error
	if h.cache, err = piececache.NewCache(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	h.meta = torrent.NewMetainfoStore(filepath.Join(t.TempDir(), "metainfo"))
	h.svc = h.newService()
	return h
}

// newService is a Service over the harness, as a (re)start makes one.
func (h *harness) newService() *Service {
	svc := New(Deps{Store: h.repo, Limits: h.gov, Cache: h.cache, Metainfo: h.meta, Client: h.client,
		Sessions: h.sessions, Clock: h.clock, Location: time.UTC, OnRestored: func() { h.restored++ }})
	h.gov.SetDownloads(svc)
	return svc
}

// storage patches the storage settings as stored, unchecked (caps far below
// what a PUT accepts keep the test's files small).
func (h *harness) storage(doc string) {
	h.t.Helper()
	var patch map[string]json.RawMessage
	if err := json.Unmarshal([]byte(doc), &patch); err != nil {
		h.t.Fatal(err)
	}
	p, err := storagecap.ParsePatch([]byte(doc))
	if err != nil {
		h.t.Fatal(err)
	}
	if _, err := h.settings.UpdateStorageSettings(h.ctx, p); err != nil {
		h.t.Fatal(err)
	}
}

func (h *harness) exec(query string, args ...any) {
	h.t.Helper()
	if _, err := h.db.Exec(query, args...); err != nil {
		h.t.Fatalf("%s: %v", query, err)
	}
}

func (h *harness) id(query string, args ...any) int64 {
	h.t.Helper()
	var id int64
	if err := h.db.QueryRow(query, args...).Scan(&id); err != nil {
		h.t.Fatalf("%s: %v", query, err)
	}
	return id
}

var tmdbSeq int

func (h *harness) movie(title string) int64 {
	tmdbSeq++
	return h.id(`INSERT INTO movies (tmdb_id, title, year) VALUES ($1, $2, 2020) RETURNING id`, 100000+tmdbSeq, title)
}

// show adds a show with seasons of n episodes each (all aired), returning
// its id and its episodes by [season, episode].
func (h *harness) show(title string, episodes ...int) (int64, map[[2]int]int64) {
	tmdbSeq++
	show := h.id(`INSERT INTO shows (tmdb_id, title, year, status) VALUES ($1, $2, 2020, 'Returning Series') RETURNING id`, 200000+tmdbSeq, title)
	ids := map[[2]int]int64{}
	for s, n := range episodes {
		season := h.id(`INSERT INTO seasons (show_id, season_number) VALUES ($1, $2) RETURNING id`, show, s+1)
		for e := 1; e <= n; e++ {
			ids[[2]int{s + 1, e}] = h.id(`INSERT INTO episodes (season_id, episode_number, name, air_date)
				VALUES ($1, $2, $3, '2025-01-01') RETURNING id`, season, e, fmt.Sprintf("Episode %d", e))
		}
	}
	return show, ids
}

// torrent builds a v1 torrent of the files (sizes), stores its metainfo
// (stored false: only the client can tell its pieces once loaded) and
// tells the fake client it can load it.
func (h *harness) torrent(name string, stored bool, sizes ...int64) testTorrent {
	h.t.Helper()
	info := v1Info(name, sizes...)
	infoBytes, err := bencode.Marshal(info)
	if err != nil {
		h.t.Fatal(err)
	}
	mi := &metainfo.MetaInfo{InfoBytes: infoBytes}
	tt := testTorrent{info: info, hash: mi.HashInfoBytes().HexString(), pm: NewPieceMap(info)}
	if stored {
		if err := h.meta.Save(tt.hash, mi); err != nil {
			h.t.Fatal(err)
		}
	}
	h.client.set(func(c *fakeClient) {
		c.infos[tt.hash] = info
		c.complete[tt.hash] = make([]bool, tt.pm.NumPieces())
	})
	return tt
}

// assign writes an active assignment at the harness's time, as admission
// orders files by it (the database's now() would follow the wall clock).
func (h *harness) assign(itemType library.ItemType, id int64, tt testTorrent, file int) {
	h.t.Helper()
	span, _ := tt.pm.File(tt.path(file))
	h.exec(`INSERT INTO torrent_assignments (item_type, item_id, info_hash, magnet_uri, file_path, file_size, is_active, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, TRUE, $7)`, itemType, id, tt.hash, tt.magnet(), tt.path(file), span.Length, h.clock.Now())
}

// fill writes the torrent's pieces to the cache, complete, as a download
// would.
func (h *harness) fill(tt testTorrent, pieces ...int) {
	h.t.Helper()
	for _, i := range pieces {
		ref := tt.pm.Ref(i)
		f, err := h.cache.OpenFile("completed/"+ref.Hex, os.O_CREATE|os.O_WRONLY)
		if err != nil {
			h.t.Fatal(err)
		}
		if _, err := f.Write(make([]byte, ref.Length)); err != nil {
			h.t.Fatal(err)
		}
		f.Close()
	}
}

// have reports which of the torrent's pieces the cache holds.
func (h *harness) have(tt testTorrent, pieces ...int) []bool {
	refs := make([]piececache.PieceRef, len(pieces))
	for k, i := range pieces {
		refs[k] = tt.pm.Ref(i)
	}
	return h.cache.HavePieces(refs)
}

func (h *harness) keep(body string) (Download, bool) {
	h.t.Helper()
	d, created, err := h.svc.Keep(h.ctx, []byte(body))
	if err != nil {
		h.t.Fatalf("Keep(%s): %v", body, err)
	}
	return d, created
}

func (h *harness) refresh() {
	h.t.Helper()
	if err := h.svc.Refresh(h.ctx); err != nil {
		h.t.Fatal(err)
	}
}

// download is the listed download with the id, after a pass.
func (h *harness) download(id int64) Download {
	h.t.Helper()
	h.refresh()
	d, ok := h.svc.Snapshot().Download(id)
	if !ok {
		h.t.Fatalf("download %d isn't listed", id)
	}
	return d
}

// awaitLoad waits for the client to be asked to load the torrent.
func (h *harness) awaitLoad(tt testTorrent) {
	h.t.Helper()
	timeout := time.After(5 * time.Second)
	for {
		select {
		case hash := <-h.client.loads:
			if hash == tt.hash {
				h.waitLoadsSettled()
				return
			}
		case <-timeout:
			h.t.Fatalf("%s was never loaded", tt.info.Name)
		}
	}
}

// waitLoadsSettled waits for the loads in flight to finish.
func (h *harness) waitLoadsSettled() {
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(time.Millisecond) {
		h.svc.loadMu.Lock()
		n := len(h.svc.loading)
		h.svc.loadMu.Unlock()
		if n == 0 {
			return
		}
	}
	h.t.Fatal("loads never settled")
}

// session is a torrent session on a file at offset of size.
func session(id string, tt testTorrent, file int, offset, size int64) sessions.Session {
	hash, path := tt.hash, tt.path(file)
	return sessions.Session{ID: id, Source: sessions.SourceTorrent, InfoHash: &hash, FilePath: &path,
		FileName: fileName(file), OffsetBytes: offset, SizeBytes: size}
}
