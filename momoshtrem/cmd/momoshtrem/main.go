package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"

	"github.com/shapedtime/momoshtrem/internal/airdate"
	"github.com/shapedtime/momoshtrem/internal/api"
	"github.com/shapedtime/momoshtrem/internal/config"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/metadata"
	"github.com/shapedtime/momoshtrem/internal/metrics"
	"github.com/shapedtime/momoshtrem/internal/opensubtitles"
	"github.com/shapedtime/momoshtrem/internal/status"
	"github.com/shapedtime/momoshtrem/internal/streaming"
	"github.com/shapedtime/momoshtrem/internal/subtitle"
	"github.com/shapedtime/momoshtrem/internal/tmdb"
	"github.com/shapedtime/momoshtrem/internal/torrent"
	"github.com/shapedtime/momoshtrem/internal/vfs"
	"github.com/shapedtime/momoshtrem/internal/webdav"
)

// version is set at build time with -ldflags "-X main.version=<version>".
var version = "dev"

// backfillCommand is the subcommand that writes TMDB metadata onto an
// existing library.
const backfillCommand = "backfill-metadata"

func main() {
	startedAt := time.Now()

	// Parse command line flags
	configPath := flag.String("config", "config.yaml", "Path to configuration file")
	flag.Usage = func() {
		out := flag.CommandLine.Output()
		fmt.Fprintf(out, "Usage: %s [-config path] [command]\n\n", os.Args[0])
		fmt.Fprintf(out, "Without a command, runs the server. Commands:\n")
		fmt.Fprintf(out, "  %s  write TMDB metadata onto movies and shows added before it existed\n", backfillCommand)
		fmt.Fprintf(out, "                     (%s -h for its flags)\n\nFlags:\n", backfillCommand)
		flag.PrintDefaults()
	}
	flag.Parse()

	command := flag.Arg(0)
	var backfill backfillFlags
	switch command {
	case "":
	case backfillCommand:
		var code int
		var ok bool
		if backfill, code, ok = parseBackfillFlags(flag.Args()[1:]); !ok {
			os.Exit(code)
		}
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", command)
		flag.Usage()
		os.Exit(2)
	}

	// Setup structured logging
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	slog.Info("Starting momoshtrem", "version", version, "config", *configPath, "command", command)

	// Load configuration (defaults → config file if present → env)
	cfg, err := config.Load(*configPath)
	if err != nil {
		slog.Error("Failed to load config", "error", err)
		os.Exit(1)
	}

	// Initialize database (and run migrations)
	db, err := library.NewDB(cfg.Database.URL)
	if err != nil {
		slog.Error("Failed to open database", "error", err)
		os.Exit(1)
	}
	defer db.Close()
	slog.Info("Database initialized",
		"driver", "postgres",
		"host", dsnHost(cfg.Database.URL),
		"from_env", os.Getenv("DATABASE_URL") != "",
	)

	// Subcommands branch off here, before torrent storage: the running
	// server holds its bolt locks, so `docker compose run` must not touch it.
	if command == backfillCommand {
		code := runBackfill(cfg, db, backfill)
		db.Close()
		os.Exit(code)
	}

	apiOpts := api.Options{
		APIToken:           cfg.Server.APIToken,
		CORSAllowedOrigins: cfg.Server.CORSAllowedOrigins,
	}
	api.ValidateOptions(apiOpts)

	// Ensure required directories exist
	if err := cfg.EnsureDirectories(); err != nil {
		slog.Error("Failed to create directories", "error", err)
		os.Exit(1)
	}

	// Initialize repositories
	movieRepo := library.NewMovieRepository(db)
	showRepo := library.NewShowRepository(db)
	assignmentRepo := library.NewAssignmentRepository(db)
	syncMetaRepo := library.NewSyncMetadataRepository(db)
	settingsRepo := library.NewSettingsRepository(db)
	metadataRepo := library.NewMetadataRepository(db)

	// Initialize TMDB: the client, and the Postgres-backed cache every
	// metadata reader goes through. Without a key, calls fail with
	// tmdb.ErrNoAPIKey (503 from the API).
	if cfg.TMDB.APIKey == "" {
		slog.Warn("TMDB API key not configured, some features will be unavailable")
	}
	tmdbClient := tmdb.NewClient(cfg.TMDB.APIKey,
		tmdb.WithLanguage(cfg.TMDB.Language),
		tmdb.WithRateLimit(float64(cfg.TMDB.RPS)),
	)
	cachedTMDB := tmdb.NewCached(tmdbClient, library.NewTMDBCacheRepository(db))
	slog.Info("TMDB client initialized", "language", cfg.TMDB.Language, "rps", cfg.TMDB.RPS)

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

	// Create reader tracker for debug visualization
	readerTracker := streaming.NewReaderTracker()

	// Initialize VFS (event-driven updates, no periodic rebuilds)
	libraryFS := vfs.NewLibraryFS(movieRepo, showRepo, assignmentRepo, cfg.VFS.TreeTTL)
	if cfg.VFS.CacheDir != "" {
		libraryFS.SetCacheDir(cfg.VFS.CacheDir)
	}
	slog.Info("VFS initialized", "cache_dir", cfg.VFS.CacheDir)

	// Wire torrent service into VFS with streaming optimization
	var activityCallback func(string)
	var activationCallback func(string, time.Duration) error
	if activityManager != nil {
		activityCallback = activityManager.MarkActive
		activationCallback = activityManager.WaitForActivation
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
		activationCallback,
		streamingCfg,
	)
	libraryFS.SetReaderTracker(readerTracker)

	// Initialize Prometheus metrics (optional)
	var metricsServer *metrics.Server
	var streamingMetrics *metrics.Metrics
	if cfg.Metrics.Enabled {
		reg := prometheus.NewRegistry()
		reg.MustRegister(
			collectors.NewGoCollector(),
			collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		)

		streamingMetrics = metrics.New(reg)
		libraryFS.SetMetrics(streamingMetrics)

		torrentCollector := metrics.NewTorrentCollector(torrentService, activityManager, streamingCfg, cfg.Torrent.MaxUnverifiedMB)
		reg.MustRegister(torrentCollector)

		metricsServer = metrics.NewServer(cfg.Metrics.Port, reg)
		go func() {
			if err := metricsServer.Start(); err != nil {
				slog.Error("Metrics server error", "error", err)
			}
		}()
		slog.Info("Prometheus metrics enabled", "port", cfg.Metrics.Port)
	}

	// System status: probes behind small interfaces, cached and throttled
	webdavPublicURL := cfg.Server.WebDAVPublicURL
	if webdavPublicURL == "" {
		webdavPublicURL = fmt.Sprintf("http://localhost:%d/", cfg.Server.WebDAVPort)
	}
	statusOpts := status.Options{
		Version:   version,
		StartedAt: startedAt,
		DB:        db,
		WebDAV: status.WebDAVInfo{
			URL:         webdavPublicURL,
			Port:        cfg.Server.WebDAVPort,
			AuthEnabled: cfg.Server.WebDAVAuth.Enabled,
			Username:    cfg.Server.WebDAVAuth.Username,
		},
		ActiveReaders:           readerTracker.Count,
		TraktConfigured:         cfg.Trakt.ClientID != "" && cfg.Trakt.ClientSecret != "",
		AIConfigured:            cfg.AI.AnthropicAPIKey != "",
		OpenSubtitlesConfigured: cfg.OpenSubtitles.APIKey != "",
	}
	if cfg.Jackett.URL != "" && cfg.Jackett.APIKey != "" {
		statusOpts.Jackett = status.JackettProber(&http.Client{}, cfg.Jackett.URL, cfg.Jackett.APIKey)
	}
	if cfg.TMDB.APIKey != "" {
		statusOpts.TMDB = tmdbClient.Ping
	}
	apiOpts.Status = status.NewService(statusOpts)
	apiOpts.Settings = settingsRepo
	apiOpts.TMDB = cachedTMDB
	// Adds and refreshes library metadata; rebuilds the WebDAV tree when a
	// refresh renames an assigned episode.
	apiOpts.Metadata = metadata.NewSyncer(cachedTMDB, movieRepo, showRepo, metadataRepo,
		metadata.WithTreeInvalidator(libraryFS))

	// Initialize servers with torrent service and tree updater
	apiServer := api.NewServer(movieRepo, showRepo, assignmentRepo, torrentService, libraryFS, apiOpts)
	apiServer.SetReaderTracker(readerTracker)

	// Initialize air date sync service
	var airDateSync *airdate.SyncService
	if cfg.AirDateSync.Enabled && cfg.TMDB.APIKey != "" {
		airDateSync = airdate.NewSyncService(cfg.AirDateSync, showRepo, syncMetaRepo, cachedTMDB)
		airDateSync.Start()
		apiServer.SetAirDateSyncService(airDateSync)
		slog.Info("Air date sync service initialized",
			"interval_hours", cfg.AirDateSync.SyncIntervalHours,
			"lookback_days", cfg.AirDateSync.LookbackDays,
		)
	} else {
		slog.Warn("Air date sync service disabled (requires TMDB API key)")
	}

	// Initialize subtitle repository (always, for VFS to show existing subtitles)
	subtitleRepo := subtitle.NewRepository(db.DB)
	libraryFS.SetSubtitleRepository(subtitleRepo)

	// Initialize subtitle service (optional, for downloading new subtitles)
	if cfg.OpenSubtitles.APIKey != "" {
		osClient := opensubtitles.NewClient(
			cfg.OpenSubtitles.APIKey,
			cfg.OpenSubtitles.Username,
			cfg.OpenSubtitles.Password,
		)
		subtitleService := subtitle.NewService(osClient, subtitleRepo, cfg.Subtitles.DownloadPath)
		apiServer.SetSubtitleService(subtitleService)
		slog.Info("Subtitle service initialized with OpenSubtitles client")
	} else {
		slog.Warn("OpenSubtitles API key not configured, subtitle download unavailable")
	}

	// Validate WebDAV auth config and create server
	webdav.ValidateConfig(cfg.Server.WebDAVAuth)
	webdavServer := webdav.NewServer(libraryFS, cfg.Server.WebDAVAuth)
	if streamingMetrics != nil {
		webdavServer.SetMetrics(streamingMetrics)
	}

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
		"version", version,
		"api_url", fmt.Sprintf("http://localhost:%d/api", cfg.Server.HTTPPort),
		"api_token_required", cfg.Server.APIToken != "",
		"webdav_url", webdavPublicURL,
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

	// Shutdown metrics server first (so collectors don't access closing services)
	if metricsServer != nil {
		if err := metricsServer.Shutdown(ctx); err != nil {
			slog.Error("Metrics server shutdown error", "error", err)
		}
	}

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

	// Stop air date sync service
	if airDateSync != nil {
		airDateSync.Stop()
	}

	// Close torrent service
	if err := torrentService.Close(); err != nil {
		slog.Error("Torrent service close error", "error", err)
	}

	// Close torrent client
	torrentClient.Close()

	slog.Info("momoshtrem stopped")
}

// dsnHost returns the host of a postgres:// URL for logging, without the
// credentials. Key/value DSNs are reported as "unknown".
func dsnHost(dsn string) string {
	u, err := url.Parse(dsn)
	if err != nil || u.Host == "" {
		return "unknown"
	}
	return u.Host
}
