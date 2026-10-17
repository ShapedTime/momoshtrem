package downloads

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	atorrent "github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/anacrolix/torrent/storage"
	"github.com/anacrolix/torrent/types"

	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/torrent"
	"github.com/shapedtime/momoshtrem/internal/torrent/piececache"
)

// A real anacrolix client on loopback with no DHT or trackers: torrents
// load from their stored metainfo, and their data is written straight to
// the piece cache, so nothing reaches the network.

type realTorrent struct {
	svc    torrent.Service
	cache  *piececache.Cache
	pieces storage.ClientImpl
	store  *torrent.MetainfoStore
	info   metainfo.Info
	mi     *metainfo.MetaInfo
	hash   string
	data   map[string][]byte
}

func newRealTorrent(t *testing.T) *realTorrent {
	t.Helper()
	fc, err := piececache.NewCache(t.TempDir())
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
	store := torrent.NewMetainfoStore(filepath.Join(t.TempDir(), "metainfo"))
	r := &realTorrent{cache: fc, pieces: pieces, store: store, data: map[string][]byte{},
		svc: torrent.NewService(client, nil, 5*time.Second, time.Second, torrent.WithMetainfoStore(store), torrent.WithPieceStorage(pieces))}

	// Two files of a season, three and a half pieces each.
	dir := filepath.Join(t.TempDir(), "Season")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"E01.mkv", "E02.mkv"} {
		data := make([]byte, 3*testPiece+testPiece/2)
		if _, err := rand.Read(data); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
			t.Fatal(err)
		}
		r.data["Season/"+name] = data
	}
	r.info = metainfo.Info{PieceLength: testPiece}
	if err := r.info.BuildFromFilePath(dir); err != nil {
		t.Fatal(err)
	}
	infoBytes, err := bencode.Marshal(r.info)
	if err != nil {
		t.Fatal(err)
	}
	r.mi = &metainfo.MetaInfo{InfoBytes: infoBytes}
	r.hash = r.mi.HashInfoBytes().HexString()
	if err := store.Save(r.hash, r.mi); err != nil {
		t.Fatal(err)
	}
	return r
}

func (r *realTorrent) magnet() string { return "magnet:?xt=urn:btih:" + r.hash }

// fill writes every piece to the cache, complete, as a finished download.
func (r *realTorrent) fill(t *testing.T) {
	t.Helper()
	ts, err := storage.NewClient(r.pieces).OpenTorrent(context.Background(), &r.info, r.mi.HashInfoBytes())
	if err != nil {
		t.Fatal(err)
	}
	var all []byte
	for _, name := range []string{"Season/E01.mkv", "Season/E02.mkv"} {
		all = append(all, r.data[name]...)
	}
	for i := 0; i < r.info.NumPieces(); i++ {
		p := r.info.Piece(i)
		piece := ts.Piece(p)
		if _, err := piece.WriteAt(all[p.Offset():p.Offset()+p.Length()], 0); err != nil {
			t.Fatal(err)
		}
		if err := piece.MarkComplete(); err != nil {
			t.Fatal(err)
		}
	}
}

func (r *realTorrent) priority(t *testing.T, path string) types.PiecePriority {
	t.Helper()
	h, err := r.svc.GetFile(r.hash, path)
	if err != nil {
		t.Fatal(err)
	}
	return h.File().Priority()
}

// ServiceClient drives a real client through torrent.Service: loads, file
// priorities, what it believes complete, and re-reading it (F11).
func TestServiceClient(t *testing.T) {
	r := newRealTorrent(t)
	r.fill(t)
	c := ServiceClient{Torrents: r.svc}
	if info := c.Info(r.hash, "Season/E01.mkv"); info != nil {
		t.Fatal("an unloaded torrent has an info dict")
	}
	if err := c.Load(r.magnet()); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if info := c.Info(r.hash, "Season/E01.mkv"); info == nil || info.Name != "Season" {
		t.Fatalf("Info = %+v", info)
	}
	loaded, err := c.Loaded()
	if _, ok := loaded[r.hash]; err != nil || !ok {
		t.Errorf("Loaded = %v, %v", loaded, err)
	}

	if err := c.SetPriorities(r.hash, []string{"Season/E01.mkv"}, []string{"Season/missing.mkv"}); err != nil {
		t.Fatal(err)
	}
	if r.priority(t, "Season/E01.mkv") != types.PiecePriorityNormal || r.priority(t, "Season/E02.mkv") != types.PiecePriorityNone {
		t.Error("only E01 should be at Normal")
	}
	if err := c.SetPriorities(r.hash, nil, []string{"Season/E01.mkv"}); err != nil || r.priority(t, "Season/E01.mkv") != types.PiecePriorityNone {
		t.Errorf("back to none: %v", err)
	}

	// Every piece is complete; the cache then loses piece 1, which the
	// client still believes complete until it reads it again.
	complete, err := c.Completion(r.hash, "Season/E01.mkv")
	if err != nil || len(complete) != r.info.NumPieces() {
		t.Fatalf("Completion = %v, %v", complete, err)
	}
	for i, ok := range complete {
		if !ok {
			t.Fatalf("piece %d isn't complete after the fill", i)
		}
	}
	ref, _ := piececache.PieceRefFor(r.info.Piece(1))
	if err := r.cache.Remove("completed/" + ref.Hex); err != nil {
		t.Fatal(err)
	}
	if complete, _ := c.Completion(r.hash, "Season/E01.mkv"); !complete[1] {
		t.Fatal("the client noticed the loss on its own; the test proves nothing")
	}
	if err := c.UpdateCompletion(r.hash, "Season/E01.mkv", []int{1}); err != nil {
		t.Fatal(err)
	}
	if complete, _ := c.Completion(r.hash, "Season/E01.mkv"); complete[1] || !complete[0] {
		t.Errorf("after the re-read, completion = %v", complete)
	}
	if err := c.UpdateCompletion(r.hash, "Season/E01.mkv", []int{99}); err == nil {
		t.Error("a piece out of range was accepted")
	}
	if n := c.Progress(r.hash, []string{"Season/E02.mkv"}); n != int64(len(r.data["Season/E02.mkv"])) {
		t.Errorf("Progress(E02) = %d", n)
	}
}

// The manager over the real client: a Keep of a season loads its torrent,
// sets its files to Normal, and re-reads a piece the cache lost (F11).
func TestManager_RealClient(t *testing.T) {
	r := newRealTorrent(t)
	r.fill(t)
	h := newHarness(t)
	h.cache = r.cache
	h.meta = r.store
	h.svc = New(Deps{Store: h.repo, Limits: h.gov, Cache: r.cache, Metainfo: r.store, Client: ServiceClient{Torrents: r.svc},
		Sessions: h.sessions, Clock: h.clock, Location: time.UTC})
	h.gov.SetDownloads(h.svc)

	show, eps := h.show("Season", 2)
	for e, name := range map[int]string{1: "Season/E01.mkv", 2: "Season/E02.mkv"} {
		h.exec(`INSERT INTO torrent_assignments (item_type, item_id, info_hash, magnet_uri, file_path, file_size, is_active)
			VALUES ('episode', $1, $2, $3, $4, $5, TRUE)`, eps[[2]int{1, e}], r.hash, r.magnet(), name, len(r.data[name]))
	}
	// Loaded (it streamed), and all of it in the cache: done, nothing to
	// fetch.
	c := ServiceClient{Torrents: r.svc}
	if err := c.Load(r.magnet()); err != nil {
		t.Fatal(err)
	}
	d, _ := h.keep(fmt.Sprintf(`{"scope":"season","id":%d,"season_number":1}`, show))
	if d.State != StateDone || d.DoneBytes != d.WantedBytes {
		t.Fatalf("a season all in the cache = %s, %d of %d", d.State, d.DoneBytes, d.WantedBytes)
	}

	// The cache loses piece 4 (E02's start) behind the loaded client's
	// back: the client still believes it complete, until the pass re-reads
	// it.
	ref, _ := piececache.PieceRefFor(r.info.Piece(4))
	if err := r.cache.Remove("completed/" + ref.Hex); err != nil {
		t.Fatal(err)
	}
	if complete, _ := c.Completion(r.hash, "Season/E01.mkv"); !complete[4] {
		t.Fatal("the client noticed the loss on its own; the test proves nothing")
	}
	d = h.download(d.ID)
	if complete, _ := c.Completion(r.hash, "Season/E01.mkv"); complete[4] || !complete[3] {
		t.Errorf("after the pass, completion = %v: no F11 re-sync of piece 4", complete)
	}
	if d.State != StateDownloading {
		t.Errorf("after the loss: %s", d.State)
	}
	if r.priority(t, "Season/E01.mkv") != types.PiecePriorityNormal || r.priority(t, "Season/E02.mkv") != types.PiecePriorityNormal {
		t.Error("the season's files aren't at Normal")
	}
	if k, ok := h.svc.Snapshot().Episode(eps[[2]int{1, 1}]); !ok || k.State != StateDone || k.Pct != 100 {
		t.Errorf("E01's keep = %+v %v", k, ok)
	}
	if k, ok := h.svc.Snapshot().Episode(eps[[2]int{1, 2}]); !ok || k.State != StateDownloading || k.Pct >= 100 {
		t.Errorf("E02's keep = %+v %v", k, ok)
	}

	// Stop keeping: its files go back to no priority.
	if err := h.svc.Delete(h.ctx, d.ID); err != nil {
		t.Fatal(err)
	}
	if r.priority(t, "Season/E01.mkv") != types.PiecePriorityNone {
		t.Error("a released download's files kept their priority")
	}
	_ = library.ItemTypeEpisode
}
