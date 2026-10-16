// Package stack runs the system under test in-process (docs/streamlab/
// PLAN.md §5 "Stack"): the leecher, the torrent service, the VFS and the
// WebDAV server, wired as cmd/momoshtrem/main.go wires them, serving lab
// media from a static library on 127.0.0.1.
//
// It differs from production only where the lab must (PLAN D1, D3): the
// client's loopback overrides (swarm.Loopback), a static tree in place of
// the database's, a temp folder, metrics always on in a registry of its
// own, a sessions clock the checks can skew, and the lab middleware
// outside the WebDAV handler.
package stack

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	atorrent "github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/anacrolix/torrent/storage"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/shapedtime/momoshtrem/internal/config"
	"github.com/shapedtime/momoshtrem/internal/metrics"
	"github.com/shapedtime/momoshtrem/internal/sessions"
	"github.com/shapedtime/momoshtrem/internal/streaming"
	"github.com/shapedtime/momoshtrem/internal/streamlab/swarm"
	"github.com/shapedtime/momoshtrem/internal/torrent"
	"github.com/shapedtime/momoshtrem/internal/vfs"
	"github.com/shapedtime/momoshtrem/internal/webdav"
)

// shutdownTimeout bounds Close's graceful shutdown of the WebDAV server
// before it drops the connections left.
const shutdownTimeout = 5 * time.Second

// Options configures a stack.
type Options struct {
	// Dir holds the leecher's state: the piece cache, piece completion,
	// peer ID and stored metainfo. Empty: a temp folder Close removes.
	// Prefer that to t.TempDir(): an anacrolix piece hasher can still mark
	// a piece complete, moving its file in the cache, just after the
	// client closes, and Close retries until its folder is gone.
	Dir string
	// ConfigPath is a YAML file (prod.yaml, say) laid over
	// config.DefaultConfig() by config.LoadWithoutDatabase, environment
	// variables included, as production loads its own. Empty: the
	// defaults alone.
	ConfigPath string
	// Tune changes the config before the stack is built: the lab's shorter
	// read timeout, say. The fingerprint is of the result.
	Tune func(*config.Config)
	// IdleCheckInterval is how often the activity manager looks for idle
	// torrents (SetCheckInterval); 0 keeps production's 30 s.
	IdleCheckInterval time.Duration
	// Movies and Episodes are the library: lab media in the static tree.
	Movies   []Movie
	Episodes []Episode
	// Stored is metainfo saved in the leecher's store before the stack
	// serves: those torrents load at once, as most production torrents
	// do. A torrent not here gets its info dict from its seeder (metadata
	// from peers).
	Stored []*metainfo.MetaInfo
}

// stacks numbers the stacks of the process.
var stacks atomic.Int64

// Stack is the system under test, serving WebDAV at URL. Its fields are
// the handles the checks read; don't replace them.
type Stack struct {
	// ID names the stack in the process ("s1"): its requests' IDs start
	// with it.
	ID string
	// URL is the WebDAV root: http://127.0.0.1:port, no trailing slash.
	URL string

	// Config is the effective config, and Fingerprint names its torrent
	// and streaming settings (the parts the streaming path reads).
	Config      *config.Config
	Fingerprint string
	Streaming   streaming.Config
	ReadTimeout time.Duration

	// Registry holds the stack's metrics, and only them: a registry per
	// stack, never the global one.
	Registry *prometheus.Registry
	Metrics  *metrics.Metrics
	Readers  *streaming.ReaderTracker
	Sessions *sessions.Registry
	// Clock is the sessions registry's clock.
	Clock    *Clock
	Service  torrent.Service
	Client   *atorrent.Client
	Activity *torrent.ActivityManager // nil when idle mode is off, as in main.go
	FS       *vfs.LibraryFS
	Requests *RequestLog

	hashes     []string
	server     *http.Server
	errs       *errorLog
	stopSpeeds chan struct{}
	completion storage.PieceCompletion
	removeDir  string // the temp folder Start made; "" when the caller gave one
	closeOnce  sync.Once
	closeErr   error
}

// Start builds the stack and serves WebDAV on 127.0.0.1 on a free port.
// Close it when done.
//
// The wiring is cmd/momoshtrem/main.go's, step for step: InitStorage, the
// client from torrent.ClientConfigFor, the activity manager, the metainfo
// store, the torrent service and its speed sampler, then the VFS with the
// service, the reader tracker, the sessions and the metrics, and the
// WebDAV server with the same metrics on an http.Server with no timeouts.
// The DHT item store is left out: without DHT it's never used.
func Start(opts Options) (_ *Stack, err error) {
	id := fmt.Sprintf("s%d", stacks.Add(1))
	s := &Stack{ID: id, Requests: &RequestLog{prefix: id + "."}, errs: &errorLog{}}
	defer func() {
		if err != nil {
			s.Close()
		}
	}()

	dir := opts.Dir
	if dir == "" {
		if dir, err = os.MkdirTemp("", "streamlab-stack-*"); err != nil {
			return nil, fmt.Errorf("stack: %w", err)
		}
		s.removeDir = dir
	}
	cfg, err := loadConfig(opts, dir)
	if err != nil {
		return nil, err
	}
	movies, episodes, hashes, err := staticLibrary(opts.Movies, opts.Episodes)
	if err != nil {
		return nil, err
	}
	s.Config, s.hashes = cfg, hashes
	s.Streaming = streamingConfig(cfg)
	s.Fingerprint = fingerprint(cfg.Torrent, s.Streaming)
	s.ReadTimeout = time.Duration(cfg.Torrent.ReadTimeout) * time.Second
	addTimeout := time.Duration(cfg.Torrent.AddTimeout) * time.Second

	// Torrent storage: the filecache LRU production uses.
	pieces, _, completion, err := torrent.InitStorage(cfg.Torrent.MetadataFolder, cfg.Torrent.GlobalCacheSize)
	if err != nil {
		return nil, fmt.Errorf("stack: torrent storage: %w", err)
	}
	s.completion = completion
	peerID, err := torrent.GetOrCreatePeerID(filepath.Join(cfg.Torrent.MetadataFolder, "peer-id"))
	if err != nil {
		return nil, fmt.Errorf("stack: peer ID: %w", err)
	}

	// Production's client config, with only the lab's loopback overrides
	// (PLAN D3).
	cc := torrent.ClientConfigFor(&cfg.Torrent, &torrent.ClientConfig{
		Storage:         pieces,
		PeerID:          peerID,
		PieceCompletion: completion,
	})
	swarm.Loopback(cc, "streamlab-leecher")
	if s.Client, err = atorrent.NewClient(cc); err != nil {
		return nil, fmt.Errorf("stack: torrent client: %w", err)
	}

	if cfg.Torrent.IdleEnabled {
		s.Activity = torrent.NewActivityManager(time.Duration(cfg.Torrent.IdleTimeout)*time.Second, cfg.Torrent.StartPaused)
		if opts.IdleCheckInterval > 0 {
			s.Activity.SetCheckInterval(opts.IdleCheckInterval)
		}
		s.Activity.Start()
	}

	speeds := torrent.NewSpeedSampler()
	store := torrent.NewMetainfoStore(filepath.Join(cfg.Torrent.MetadataFolder, "metainfo"))
	for _, mi := range opts.Stored {
		if err := store.Save(mi.HashInfoBytes().HexString(), mi); err != nil {
			return nil, fmt.Errorf("stack: storing metainfo: %w", err)
		}
	}
	s.Service = torrent.NewService(s.Client, s.Activity, addTimeout, s.ReadTimeout,
		torrent.WithMetainfoStore(store),
		torrent.WithPieceStorage(pieces),
		torrent.WithSpeedSampler(speeds),
	)
	s.stopSpeeds = make(chan struct{})
	go speeds.Run(s.Service, s.stopSpeeds)

	s.Readers = streaming.NewReaderTracker()
	s.FS = vfs.NewStaticFS(movies, episodes)
	var activity func(string)
	var activation func(string, time.Duration) error
	if s.Activity != nil {
		activity, activation = s.Activity.MarkActive, s.Activity.WaitForActivation
	}
	s.FS.SetTorrentService(s.Service, s.ReadTimeout, activity, activation, s.Streaming)
	s.FS.SetReaderTracker(s.Readers)
	// Production resolves each session's item from the database
	// (WithItems); the lab has none, so items stay null.
	s.Clock = &Clock{}
	s.Sessions = sessions.NewRegistry(sessions.WithClock(s.Clock.Now))
	s.FS.SetSessions(s.Sessions)
	// Production sets metrics only with METRICS_ENABLED; the checks read
	// them, so the lab always does. The lab serves no /metrics: no Go,
	// process or torrent collectors.
	s.Registry = prometheus.NewRegistry()
	s.Metrics = metrics.New(s.Registry)
	s.FS.SetMetrics(s.Metrics)

	webdav.ValidateConfig(cfg.Server.WebDAVAuth)
	ws := webdav.NewServer(s.FS, cfg.Server.WebDAVAuth)
	ws.SetMetrics(s.Metrics) // before Handler: without it requests_inflight stays 0

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("stack: listening: %w", err)
	}
	// No timeouts, as in production.
	s.server = &http.Server{
		Handler:  middleware(s.Requests, ws.Handler()),
		ErrorLog: log.New(s.errs, "", 0),
	}
	s.URL = "http://" + ln.Addr().String()
	go s.server.Serve(ln) // returns ErrServerClosed on Close
	return s, nil
}

// Torrents returns the info hashes of the library's torrents, each once.
func (s *Stack) Torrents() []string { return append([]string(nil), s.hashes...) }

// Torrent returns the leecher's torrent with the hash, when the client has
// it: loaded, or being loaded.
func (s *Stack) Torrent(hash string) (*atorrent.Torrent, bool) {
	var ih metainfo.Hash
	if err := ih.FromHexString(hash); err != nil {
		return nil, false
	}
	return s.Client.Torrent(ih)
}

// ServerErrors returns what the HTTP server logged: each handler panic it
// recovered is one entry ("http: panic serving …" and the stack).
func (s *Stack) ServerErrors() []string { return s.errs.entries() }

// Close shuts the stack down in main.go's order: the WebDAV server (its
// requests get shutdownTimeout to finish, then their connections are
// dropped), the activity manager and the speed sampler, the torrent
// service, the client and piece completion. It removes the temp folder
// Start made. It's safe to call more than once.
func (s *Stack) Close() error {
	s.closeOnce.Do(func() {
		var errs []error
		if s.server != nil {
			ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
			if err := s.server.Shutdown(ctx); err != nil {
				errs = append(errs, s.server.Close())
			}
			cancel()
		}
		if s.Activity != nil {
			s.Activity.Stop()
		}
		if s.stopSpeeds != nil {
			close(s.stopSpeeds)
		}
		if s.Service != nil {
			errs = append(errs, s.Service.Close())
		}
		if s.Client != nil {
			errs = append(errs, s.Client.Close()...)
		}
		if s.completion != nil {
			errs = append(errs, s.completion.Close())
		}
		if s.removeDir != "" {
			errs = append(errs, removeAll(s.removeDir))
		}
		s.closeErr = errors.Join(errs...)
	})
	return s.closeErr
}

// removeAll removes dir, retrying for a while: anacrolix's Client.Close
// returns before a piece hasher in flight has marked its piece complete,
// which moves the piece's file within the cache.
func removeAll(dir string) error {
	var err error
	for range 40 {
		if err = os.RemoveAll(dir); err == nil {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return err
}

// Clock is the sessions registry's clock: the wall clock plus a skew. The
// quiescence check skews it past sessions.Timeout to end the sessions
// whose reads are done, without waiting 30 s.
type Clock struct {
	skew atomic.Int64
}

// Now is the wall clock plus the skew.
func (c *Clock) Now() time.Time { return time.Now().Add(time.Duration(c.skew.Load())) }

// Skew moves the clock d ahead, for good.
func (c *Clock) Skew(d time.Duration) { c.skew.Add(int64(d)) }
