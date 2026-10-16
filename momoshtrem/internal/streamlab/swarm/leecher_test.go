package swarm_test

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"time"

	atorrent "github.com/anacrolix/torrent"

	"github.com/shapedtime/momoshtrem/internal/config"
	"github.com/shapedtime/momoshtrem/internal/streaming"
	"github.com/shapedtime/momoshtrem/internal/streamlab/swarm"
	"github.com/shapedtime/momoshtrem/internal/torrent"
	"github.com/shapedtime/momoshtrem/internal/vfs"
)

// The lab's timeouts. Production waits up to 60 s for metadata and 120 s
// per read; they only bound a stall, and shorter ones end a stalled test
// sooner. A healthy read on loopback takes well under a second, even
// under -race on a loaded machine. The smoke tier's reads are smaller
// still (smokeReadTimeout).
const (
	labAddTimeout  = 30 * time.Second
	labReadTimeout = 20 * time.Second
)

// leecher is the production read path, wired as cmd/momoshtrem/main.go
// wires it: InitStorage's filecache LRU, torrent.ClientConfigFor on
// loopback, the activity manager, the metainfo store and the torrent
// service. Files open as vfs.TorrentFile, as LibraryFS.openTorrentFile
// opens them. S3 (#174) replaces this with the full stack.
type leecher struct {
	client    *atorrent.Client
	svc       torrent.Service
	am        *torrent.ActivityManager
	store     *torrent.MetainfoStore
	tracker   *streaming.ReaderTracker
	streaming streaming.Config
	// readTimeout bounds each read, as torrent.read_timeout does.
	readTimeout time.Duration
}

// newLeecher builds a leecher on config.DefaultConfig(), in a temp folder,
// with readTimeout in place of torrent.read_timeout.
func newLeecher(t *testing.T, startPaused bool, readTimeout time.Duration) *leecher {
	t.Helper()
	cfg := config.DefaultConfig()
	cfg.Torrent.MetadataFolder = t.TempDir()
	cfg.Torrent.StartPaused = startPaused
	dir := cfg.Torrent.MetadataFolder

	pieces, _, completion, err := torrent.InitStorage(dir, cfg.Torrent.GlobalCacheSize)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { completion.Close() })
	peerID, err := torrent.GetOrCreatePeerID(filepath.Join(dir, "peer-id"))
	if err != nil {
		t.Fatal(err)
	}
	// Production's config (PLAN D3), with only the lab's loopback overrides.
	// Without DHT the item store is never used.
	cc := torrent.ClientConfigFor(&cfg.Torrent, &torrent.ClientConfig{
		Storage:         pieces,
		PeerID:          peerID,
		PieceCompletion: completion,
	})
	swarm.Loopback(cc, "streamlab-leecher")
	client, err := atorrent.NewClient(cc)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { client.Close() })

	am := torrent.NewActivityManager(time.Duration(cfg.Torrent.IdleTimeout)*time.Second, cfg.Torrent.StartPaused)
	am.Start()
	t.Cleanup(am.Stop)

	speeds := torrent.NewSpeedSampler()
	store := torrent.NewMetainfoStore(filepath.Join(dir, "metainfo"))
	svc := torrent.NewService(client, am, labAddTimeout, readTimeout,
		torrent.WithMetainfoStore(store),
		torrent.WithPieceStorage(pieces),
		torrent.WithSpeedSampler(speeds),
	)
	stop := make(chan struct{})
	go speeds.Run(svc, stop)
	t.Cleanup(func() { close(stop) })
	t.Cleanup(func() { svc.Close() })

	sc := streaming.Config{
		HeaderPriorityBytes: cfg.Streaming.HeaderPriorityBytes,
		FooterPriorityBytes: cfg.Streaming.FooterPriorityBytes,
		ReadaheadBytes:      cfg.Streaming.ReadaheadBytes,
		UrgentBufferBytes:   cfg.Streaming.UrgentBufferBytes,
	}
	if sc.IsZero() {
		sc = streaming.DefaultConfig()
	}
	return &leecher{
		client:      client,
		svc:         svc,
		am:          am,
		store:       store,
		tracker:     streaming.NewReaderTracker(),
		streaming:   sc,
		readTimeout: readTimeout,
	}
}

// status describes the torrent for a failure message: the activity
// manager's state and anacrolix's client status (peers, pieces, requests).
func (l *leecher) status(hash string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "activity state %q, active %v\n", l.am.GetState(hash), l.svc.IsActive(hash))
	l.client.WriteStatus(&b)
	return b.String()
}

// open loads the magnet's torrent and opens one of its files, as a WebDAV
// request does. It also returns the anacrolix torrent, for diagnosis.
func (l *leecher) open(t *testing.T, magnet, filePath string) (*vfs.TorrentFile, *atorrent.Torrent) {
	t.Helper()
	info, err := l.svc.GetOrAddTorrent(magnet)
	if err != nil {
		t.Fatalf("GetOrAddTorrent: %v", err)
	}
	handle, err := l.svc.GetFile(info.InfoHash, filePath)
	if err != nil {
		t.Fatalf("GetFile(%q): %v", filePath, err)
	}
	return vfs.NewTorrentFile(
		handle,
		path.Base(filePath),
		info.InfoHash,
		l.readTimeout,
		l.am.MarkActive,
		l.am.WaitForActivation,
		l.streaming,
		nil,
		l.tracker,
	), handle.Torrent()
}
