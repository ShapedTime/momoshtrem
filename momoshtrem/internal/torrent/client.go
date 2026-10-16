package torrent

import (
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/anacrolix/dht/v2"
	"github.com/anacrolix/dht/v2/bep44"
	tlog "github.com/anacrolix/log"
	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/storage"

	"github.com/shapedtime/momoshtrem/internal/config"
	"github.com/shapedtime/momoshtrem/internal/torrent/piececache"
)

// ClientConfig holds all components needed for client creation.
type ClientConfig struct {
	Storage         storage.ClientImpl
	ItemStore       bep44.Store
	PeerID          [20]byte
	PieceCompletion storage.PieceCompletion
}

// torrentLogHandler adapts slog for anacrolix/torrent's logger.
type torrentLogHandler struct {
	log *slog.Logger
}

func (h *torrentLogHandler) Handle(r tlog.Record) {
	level := slog.LevelDebug
	switch r.Level {
	case tlog.Critical, tlog.Error:
		level = slog.LevelError
	case tlog.Warning:
		level = slog.LevelWarn
	case tlog.Info:
		level = slog.LevelInfo
	case tlog.Debug:
		level = slog.LevelDebug
	}
	h.log.Log(nil, level, r.Msg.String())
}

// InitStorage creates the storage layer for torrents.
// Returns the storage implementation, piece cache, piece completion database, and any error.
//
// The piece cache is missinggo's filecache with pins (piececache). With
// nothing pinned it behaves as filecache did, trimming from the first write.
// To hold trims until pins are restored after a restart, call DeferTrim on
// it before the torrent client exists, then Ready once the pins are set.
func InitStorage(metadataFolder string, cacheSizeMB int64) (storage.ClientImpl, *piececache.Cache, storage.PieceCompletion, error) {
	// Create cache directory
	cacheDir := filepath.Join(metadataFolder, "cache")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return nil, nil, nil, err
	}

	// Create file cache for pieces
	fc, err := piececache.NewCache(cacheDir)
	if err != nil {
		return nil, nil, nil, err
	}

	// Set cache capacity (convert MB to bytes)
	fc.SetCapacity(cacheSizeMB * 1024 * 1024)

	// Create resource-based storage backed by file cache
	st := storage.NewResourcePieces(fc.AsResourceProvider())

	// Create piece completion tracking directory
	pcDir := filepath.Join(metadataFolder, "piece-completion")
	if err := os.MkdirAll(pcDir, 0755); err != nil {
		return nil, nil, nil, err
	}

	// Create BoltDB piece completion tracker
	pc, err := storage.NewBoltPieceCompletion(pcDir)
	if err != nil {
		return nil, nil, nil, err
	}

	slog.Info("torrent storage initialized",
		"cache_dir", cacheDir,
		"cache_size_mb", cacheSizeMB,
		"piece_completion_dir", pcDir,
	)

	return st, fc, pc, nil
}

// ClientConfigFor is the anacrolix config NewClient creates its client from:
// the defaults plus momoshtrem's settings. The streamlab lab builds its
// client from it too, so every client setting goes here, not in NewClient.
// It has no side effects: nothing listens until torrent.NewClient.
func ClientConfigFor(cfg *config.TorrentConfig, cc *ClientConfig) *torrent.ClientConfig {
	log := slog.With("component", "torrent-client")

	torrentCfg := torrent.NewDefaultClientConfig()
	torrentCfg.Seed = true
	torrentCfg.PeerID = string(cc.PeerID[:])
	torrentCfg.DefaultStorage = cc.Storage

	// BitTorrent port for TCP and uTP; 0 picks a free one.
	torrentCfg.ListenPort = cfg.ListenPort

	// Disable IPv6 for simpler networking
	torrentCfg.DisableIPv6 = true

	// Prevent duplicate peer connections that send redundant chunks
	torrentCfg.DropDuplicatePeerIds = cfg.DropDuplicatePeerIds

	// Cap in-flight unverified data to reduce wasted chunks when priorities change
	if cfg.MaxUnverifiedMB > 0 {
		torrentCfg.MaxUnverifiedBytes = cfg.MaxUnverifiedMB * 1024 * 1024
	}

	// Configure logging
	tl := tlog.NewLogger()
	tl.SetHandlers(&torrentLogHandler{log: log})
	torrentCfg.Logger = tl

	// Configure DHT server with item store
	torrentCfg.ConfigureAnacrolixDhtServer = func(dhtCfg *dht.ServerConfig) {
		dhtCfg.Store = cc.ItemStore
		dhtCfg.Exp = 2 * time.Hour
		dhtCfg.NoSecurity = false
	}

	return torrentCfg
}

// NewClient creates a new torrent client with the given configuration.
func NewClient(cfg *config.TorrentConfig, cc *ClientConfig) (*torrent.Client, error) {
	log := slog.With("component", "torrent-client")

	client, err := torrent.NewClient(ClientConfigFor(cfg, cc))
	if err != nil {
		return nil, err
	}

	log.Info("torrent client created",
		"listen_port", client.LocalPort(),
		"seeding", true,
		"ipv6_disabled", true,
		"drop_duplicate_peers", cfg.DropDuplicatePeerIds,
		"max_unverified_mb", cfg.MaxUnverifiedMB,
	)

	return client, nil
}
