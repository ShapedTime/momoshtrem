package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/shapedtime/momoshtrem/internal/api"
	"github.com/shapedtime/momoshtrem/internal/config"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/streaming"
	"github.com/shapedtime/momoshtrem/internal/tmdb"
	"github.com/shapedtime/momoshtrem/internal/torrent"
	"github.com/shapedtime/momoshtrem/internal/vfs"
	"github.com/shapedtime/momoshtrem/internal/webdav"
)

func main() {
	// Parse command line flags
	configPath := flag.String("config", "config.yaml", "Path to configuration file")
	flag.Parse()

	// Setup structured logging
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	slog.SetDefault(logger)

	slog.Info("Starting momoshtrem", "config", *configPath)

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		slog.Error("Failed to load config", "error", err)
		os.Exit(1)
	}

	// Ensure required directories exist
	if err := cfg.EnsureDirectories(); err != nil {
		slog.Error("Failed to create directories", "error", err)
		os.Exit(1)
	}

	// Initialize database
	db, err := library.NewDB(cfg.Database.Path)
	if err != nil {
		slog.Error("Failed to open database", "error", err)
		os.Exit(1)
	}
	defer db.Close()
	slog.Info("Database initialized", "path", cfg.Database.Path)

	// Initialize repositories
	movieRepo := library.NewMovieRepository(db)
	showRepo := library.NewShowRepository(db)
	assignmentRepo := library.NewAssignmentRepository(db)

	// Initialize TMDB client
	var tmdbClient *tmdb.Client
	if cfg.TMDB.APIKey != "" {
		tmdbClient = tmdb.NewClient(cfg.TMDB.APIKey)
		slog.Info("TMDB client initialized")
	} else {
		slog.Warn("TMDB API key not configured, some features will be unavailable")
		tmdbClient = tmdb.NewClient("") // Empty client will fail on API calls
	}

	// Initialize torrent storage
	pieceStorage, _, pieceCompletion, err := torrent.InitStorage(
		cfg.Torrent.MetadataFolder,
		cfg.Torrent.GlobalCacheSize,
	)
	if err != nil {
		slog.Error("Failed to initialize torrent storage", "error", err)
		os.Exit(1)
	}
	defer pieceCompletion.Close()
	slog.Info("Torrent storage initialized",
		"metadata_folder", cfg.Torrent.MetadataFolder,
		"cache_size_mb", cfg.Torrent.GlobalCacheSize,
	)

	// Initialize DHT item store
	itemStore, err := torrent.NewItemStore(
		filepath.Join(cfg.Torrent.MetadataFolder, "dht-items"),
		2*time.Hour,
	)
	if err != nil {
		slog.Error("Failed to initialize DHT item store", "error", err)
		os.Exit(1)
	}
	defer itemStore.Close()

	// Get or create peer ID
	peerID, err := torrent.GetOrCreatePeerID(
		filepath.Join(cfg.Torrent.MetadataFolder, "peer-id"),
	)
	if err != nil {
		slog.Error("Failed to get peer ID", "error", err)
		os.Exit(1)
	}
	slog.Info("Peer ID initialized")

	// Create torrent client
	torrentClient, err := torrent.NewClient(&cfg.Torrent, &torrent.ClientConfig{
		Storage:         pieceStorage,
		ItemStore:       itemStore,
		PeerID:          peerID,
		PieceCompletion: pieceCompletion,
	})
	if err != nil {
		slog.Error("Failed to create torrent client", "error", err)
		os.Exit(1)
	}
	slog.Info("Torrent client created")

	// Create activity manager (if idle mode enabled)
	var activityManager *torrent.ActivityManager
	if cfg.Torrent.IdleEnabled {
		activityManager = torrent.NewActivityManager(
			time.Duration(cfg.Torrent.IdleTimeout)*time.Second,
			cfg.Torrent.StartPaused,
		)
		activityManager.Start()
		slog.Info("Activity manager started",
			"idle_timeout_seconds", cfg.Torrent.IdleTimeout,
			"start_paused", cfg.Torrent.StartPaused,
		)
	}

	// Create torrent service
	torrentService := torrent.NewService(
		torrentClient,
		activityManager,
		time.Duration(cfg.Torrent.AddTimeout)*time.Second,
		time.Duration(cfg.Torrent.ReadTimeout)*time.Second,
	)
	slog.Info("Torrent service initialized",
		"add_timeout_seconds", cfg.Torrent.AddTimeout,
		"read_timeout_seconds", cfg.Torrent.ReadTimeout,
	)

	// Initialize VFS
	libraryFS := vfs.NewLibraryFS(movieRepo, showRepo, assignmentRepo, cfg.VFS.TreeTTL)
	slog.Info("VFS initialized", "tree_ttl_seconds", cfg.VFS.TreeTTL)

	// Wire torrent service into VFS with streaming optimization
	var activityCallback func(string)
	if activityManager != nil {
		activityCallback = activityManager.MarkActive
	}

	// Create streaming config from application config
	streamingCfg := streaming.Config{
		HeaderPriorityBytes: cfg.Streaming.HeaderPriorityBytes,
		FooterPriorityBytes: cfg.Streaming.FooterPriorityBytes,
		ReadaheadBytes:      cfg.Streaming.ReadaheadBytes,
		UrgentBufferBytes:   cfg.Streaming.UrgentBufferBytes,
	}
	if streamingCfg.IsZero() {
		streamingCfg = streaming.DefaultConfig()
	}
	slog.Info("Streaming optimization configured",
		"header_priority_mb", streamingCfg.HeaderPriorityBytes/(1024*1024),
		"footer_priority_mb", streamingCfg.FooterPriorityBytes/(1024*1024),
		"readahead_mb", streamingCfg.ReadaheadBytes/(1024*1024),
	)

	libraryFS.SetTorrentService(
		torrentService,
		time.Duration(cfg.Torrent.ReadTimeout)*time.Second,
		activityCallback,
		streamingCfg,
	)

	// Initialize servers with torrent service and tree updater
	apiServer := api.NewServer(movieRepo, showRepo, assignmentRepo, tmdbClient, torrentService, libraryFS)

	// Validate WebDAV auth config and create server
	webdav.ValidateConfig(cfg.Server.WebDAVAuth)
	webdavServer := webdav.NewServer(libraryFS, cfg.Server.WebDAVAuth)

	// Start HTTP servers
	httpServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Server.HTTPPort),
		Handler: apiServer.Handler(),
	}

	webdavHTTPServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Server.WebDAVPort),
		Handler: webdavServer.Handler(),
	}

	// Start servers in goroutines
	go func() {
		slog.Info("Starting REST API server", "port", cfg.Server.HTTPPort)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("REST API server error", "error", err)
		}
	}()

	go func() {
		slog.Info("Starting WebDAV server", "port", cfg.Server.WebDAVPort)
		if err := webdavHTTPServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("WebDAV server error", "error", err)
		}
	}()

	slog.Info("momoshtrem is ready",
		"api_url", fmt.Sprintf("http://localhost:%d/api", cfg.Server.HTTPPort),
		"webdav_url", fmt.Sprintf("http://localhost:%d", cfg.Server.WebDAVPort),
		"webdav_auth_enabled", cfg.Server.WebDAVAuth.Enabled,
	)

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	sig := <-sigChan
	slog.Info("Received signal, shutting down", "signal", sig)

	// Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Shutdown HTTP servers
	if err := httpServer.Shutdown(ctx); err != nil {
		slog.Error("REST API server shutdown error", "error", err)
	}
	if err := webdavHTTPServer.Shutdown(ctx); err != nil {
		slog.Error("WebDAV server shutdown error", "error", err)
	}

	// Stop activity manager
	if activityManager != nil {
		activityManager.Stop()
	}

	// Close torrent service
	if err := torrentService.Close(); err != nil {
		slog.Error("Torrent service close error", "error", err)
	}

	// Close torrent client
	torrentClient.Close()

	slog.Info("momoshtrem stopped")
}
