package torrent

import (
	"context"
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/anacrolix/missinggo/v2/filecache"
	atorrent "github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/anacrolix/torrent/storage"
)

// The lifecycle tests run a real anacrolix client on loopback, with no DHT
// or trackers, so nothing reaches the network: torrents load from stored
// metainfo, or from info bytes a test hands over as peers would.

const testPieceLength = 16 * 1024

// fixture is a three-piece, single-file torrent and its content.
type fixture struct {
	mi   *metainfo.MetaInfo
	info metainfo.Info
	hash string
	data []byte
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	data := make([]byte, 3*testPieceLength-100)
	if _, err := rand.Read(data); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "Film.2020.1080p.mkv")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	info := metainfo.Info{PieceLength: testPieceLength}
	if err := info.BuildFromFilePath(path); err != nil {
		t.Fatal(err)
	}
	infoBytes, err := bencode.Marshal(info)
	if err != nil {
		t.Fatal(err)
	}
	mi := &metainfo.MetaInfo{InfoBytes: infoBytes}
	return fixture{mi: mi, info: info, hash: mi.HashInfoBytes().HexString(), data: data}
}

func (f fixture) magnet() string { return "magnet:?xt=urn:btih:" + f.hash + "&dn=Film" }

// testEnv is a service over a real client, its piece storage and its
// metainfo store.
type testEnv struct {
	svc    *service
	pieces storage.ClientImpl
	store  *MetainfoStore
}

func newTestEnv(t *testing.T, addTimeout time.Duration) *testEnv {
	t.Helper()
	fc, err := filecache.NewCache(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	pieces := storage.NewResourcePieces(fc.AsResourceProvider())
	cfg := atorrent.TestingConfig(t)
	cfg.DefaultStorage = pieces
	client, err := atorrent.NewClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { client.Close() })
	store := NewMetainfoStore(filepath.Join(t.TempDir(), "metainfo"))
	svc := newService(client, nil, addTimeout, time.Second, WithMetainfoStore(store), WithPieceStorage(pieces))
	return &testEnv{svc: svc, pieces: pieces, store: store}
}

// fill writes every piece of f to the cache and marks it complete, as a
// finished download would.
func (e *testEnv) fill(t *testing.T, f fixture) {
	t.Helper()
	ts, err := storage.NewClient(e.pieces).OpenTorrent(context.Background(), &f.info, f.mi.HashInfoBytes())
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < f.info.NumPieces(); i++ {
		p := f.info.Piece(i)
		piece := ts.Piece(p)
		if _, err := piece.WriteAt(f.data[p.Offset():p.Offset()+p.Length()], 0); err != nil {
			t.Fatal(err)
		}
		if err := piece.MarkComplete(); err != nil {
			t.Fatal(err)
		}
	}
	if got := e.complete(t, f); got != f.info.NumPieces() {
		t.Fatalf("filled %d of %d pieces", got, f.info.NumPieces())
	}
}

// complete counts f's pieces the cache holds.
func (e *testEnv) complete(t *testing.T, f fixture) int {
	t.Helper()
	ts, err := storage.NewClient(e.pieces).OpenTorrent(context.Background(), &f.info, f.mi.HashInfoBytes())
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for i := 0; i < f.info.NumPieces(); i++ {
		if ts.Piece(f.info.Piece(i)).Completion().Complete {
			n++
		}
	}
	return n
}

func (e *testEnv) stored(t *testing.T, hash string) bool {
	t.Helper()
	mi, err := e.store.Load(hash)
	if err != nil {
		t.Fatal(err)
	}
	return mi != nil
}

// peersSend makes the next adds get f's info dict as if peers sent it,
// counting them; each blocks until release is closed (nil: no wait).
func (e *testEnv) peersSend(f fixture, release <-chan struct{}) (calls *atomic.Int32, entered <-chan struct{}) {
	var n atomic.Int32
	in := make(chan struct{}, 16)
	add := e.svc.addSpec
	e.svc.addSpec = func(spec *atorrent.TorrentSpec) (*atorrent.Torrent, error) {
		n.Add(1)
		in <- struct{}{}
		if release != nil {
			<-release
		}
		if spec.InfoBytes == nil {
			spec.InfoBytes = f.mi.InfoBytes
		}
		return add(spec)
	}
	return &n, in
}

func TestAddTorrent_LoadsStoredMetainfoWithoutPeers(t *testing.T) {
	e := newTestEnv(t, 5*time.Second)
	f := newFixture(t)
	if err := e.store.Save(f.hash, f.mi); err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	info, err := e.svc.AddTorrent(f.magnet())
	if err != nil {
		t.Fatalf("AddTorrent: %v", err)
	}
	// No peers exist: without the stored file this would time out.
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("took %v; the stored metainfo should load at once", elapsed)
	}
	if info.InfoHash != f.hash || info.Name != "Film.2020.1080p.mkv" || len(info.Files) != 1 || info.TotalSize != int64(len(f.data)) {
		t.Errorf("info = %+v", info)
	}
}

func TestAddTorrent_StoresMetainfoFromPeers(t *testing.T) {
	e := newTestEnv(t, 5*time.Second)
	f := newFixture(t)
	e.peersSend(f, nil)

	if _, err := e.svc.AddTorrent(f.magnet()); err != nil {
		t.Fatalf("AddTorrent: %v", err)
	}
	mi, err := e.store.Load(f.hash)
	if err != nil || mi == nil {
		t.Fatalf("stored metainfo = %v, %v; want the torrent's", mi, err)
	}
	if string(mi.InfoBytes) != string(f.mi.InfoBytes) {
		t.Error("stored info dict differs from the torrent's")
	}
}

func TestAddTorrent_TimeoutDropsTheTorrent(t *testing.T) {
	e := newTestEnv(t, 50*time.Millisecond)
	const hash = "0123456789abcdef0123456789abcdef01234567"

	_, err := e.svc.AddTorrent("magnet:?xt=urn:btih:" + hash)
	if !errors.Is(err, ErrMetadataTimeout) {
		t.Fatalf("err = %v, want ErrMetadataTimeout", err)
	}
	if _, ok := e.svc.client.Torrent(metainfo.NewHashFromHex(hash)); ok {
		t.Error("the timed-out torrent is still in the client")
	}
	if e.stored(t, hash) {
		t.Error("a torrent without metadata was stored")
	}
}

// Concurrent adds of one hash (jobs, WebDAV reads) make one add, one wait
// and one drop decision, and all get its result.
func TestAddTorrent_ConcurrentAddsShareOneAttempt(t *testing.T) {
	e := newTestEnv(t, 5*time.Second)
	f := newFixture(t)
	release := make(chan struct{})
	calls, entered := e.peersSend(f, release)

	const callers = 6
	var wg sync.WaitGroup
	errs := make([]error, callers)
	infos := make([]*TorrentInfo, callers)
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if i%2 == 0 {
				infos[i], errs[i] = e.svc.AddTorrent(f.magnet())
			} else {
				infos[i], errs[i] = e.svc.GetOrAddTorrent(f.magnet()) // the VFS's lazy load
			}
		}(i)
	}
	<-entered
	time.Sleep(50 * time.Millisecond) // let the others pile up behind the first
	close(release)
	wg.Wait()

	if n := calls.Load(); n != 1 {
		t.Errorf("client adds = %d, want 1", n)
	}
	for i := range errs {
		if errs[i] != nil || infos[i] == nil || infos[i].InfoHash != f.hash {
			t.Errorf("caller %d: %+v, %v", i, infos[i], errs[i])
		}
	}
}

// RemoveTorrent waits for an add of the same hash in flight, then removes
// what it loaded.
func TestRemoveTorrent_WaitsForAnAddInFlight(t *testing.T) {
	e := newTestEnv(t, 5*time.Second)
	f := newFixture(t)
	release := make(chan struct{})
	_, entered := e.peersSend(f, release)

	added := make(chan error, 1)
	go func() {
		_, err := e.svc.AddTorrent(f.magnet())
		added <- err
	}()
	<-entered

	removed := make(chan error, 1)
	go func() { removed <- e.svc.RemoveTorrent(f.hash, false) }()
	select {
	case err := <-removed:
		t.Fatalf("RemoveTorrent returned %v while the add was in flight", err)
	case <-time.After(100 * time.Millisecond):
	}

	close(release)
	if err := <-added; err != nil {
		t.Fatalf("AddTorrent: %v", err)
	}
	if err := <-removed; err != nil {
		t.Fatalf("RemoveTorrent: %v, want the torrent the add loaded removed", err)
	}
	if e.svc.loaded(f.hash) != nil {
		t.Error("torrent still loaded")
	}
	if e.stored(t, f.hash) {
		t.Error("metainfo still stored")
	}
}

func TestRemoveTorrent(t *testing.T) {
	tests := []struct {
		name         string
		load         bool // added to the client first
		store        bool // metainfo stored (adding stores it too)
		upper        bool // the hash as uppercase hex
		deleteData   bool
		wantErr      error
		wantComplete bool // the pieces are still in the cache
	}{
		{name: "loaded, keep data", load: true, wantComplete: true},
		{name: "loaded, delete data", load: true, deleteData: true},
		{name: "loaded, uppercase hash", load: true, upper: true, deleteData: true},
		{name: "not loaded, stored, delete data", store: true, deleteData: true},
		{name: "not loaded, stored, keep data", store: true, wantComplete: true},
		{name: "unknown", deleteData: true, wantErr: ErrTorrentNotFound, wantComplete: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newTestEnv(t, 5*time.Second)
			f := newFixture(t)
			e.fill(t, f)
			if tt.store || tt.load {
				if err := e.store.Save(f.hash, f.mi); err != nil {
					t.Fatal(err)
				}
			}
			if tt.load {
				if _, err := e.svc.AddTorrent(f.magnet()); err != nil {
					t.Fatal(err)
				}
			}
			hash := f.hash
			if tt.upper {
				hash = strings.ToUpper(hash)
			}

			err := e.svc.RemoveTorrent(hash, tt.deleteData)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if e.svc.loaded(f.hash) != nil {
				t.Error("torrent still loaded")
			}
			if _, ok := e.svc.client.Torrent(f.mi.HashInfoBytes()); ok {
				t.Error("torrent still in the client")
			}
			if e.stored(t, f.hash) {
				t.Error("metainfo still stored")
			}
			got := e.complete(t, f)
			if tt.wantComplete && got != f.info.NumPieces() {
				t.Errorf("%d of %d pieces left, want all", got, f.info.NumPieces())
			}
			if !tt.wantComplete && got != 0 {
				t.Errorf("%d pieces left, want none", got)
			}
		})
	}
}

func TestRemoveTorrent_ThenAddFetchesAgain(t *testing.T) {
	e := newTestEnv(t, 50*time.Millisecond)
	f := newFixture(t)
	if err := e.store.Save(f.hash, f.mi); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.AddTorrent(f.magnet()); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.RemoveTorrent(f.hash, false); err != nil {
		t.Fatal(err)
	}
	// The metainfo went with it, so the next add waits for peers (none).
	if _, err := e.svc.AddTorrent(f.magnet()); !errors.Is(err, ErrMetadataTimeout) {
		t.Errorf("re-add err = %v, want ErrMetadataTimeout", err)
	}
}
