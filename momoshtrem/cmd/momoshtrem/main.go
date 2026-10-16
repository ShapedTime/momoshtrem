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

	"github.com/shapedtime/momoshtrem/internal/activity"
	"github.com/shapedtime/momoshtrem/internal/airdate"
	"github.com/shapedtime/momoshtrem/internal/api"
	"github.com/shapedtime/momoshtrem/internal/artwork"
	"github.com/shapedtime/momoshtrem/internal/catalog"
	"github.com/shapedtime/momoshtrem/internal/config"
	"github.com/shapedtime/momoshtrem/internal/hdrezka"
	"github.com/shapedtime/momoshtrem/internal/identify"
	"github.com/shapedtime/momoshtrem/internal/identify/ai"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/metadata"
	"github.com/shapedtime/momoshtrem/internal/metrics"
	"github.com/shapedtime/momoshtrem/internal/opensubtitles"
	"github.com/shapedtime/momoshtrem/internal/search"
	"github.com/shapedtime/momoshtrem/internal/service"
	"github.com/shapedtime/momoshtrem/internal/sessions"
	"github.com/shapedtime/momoshtrem/internal/status"
	"github.com/shapedtime/momoshtrem/internal/streaming"
	"github.com/shapedtime/momoshtrem/internal/subtitle"
	"github.com/shapedtime/momoshtrem/internal/tmdb"
	"github.com/shapedtime/momoshtrem/internal/torrent"
	"github.com/shapedtime/momoshtrem/internal/torznab"
	"github.com/shapedtime/momoshtrem/internal/vfs"
	"github.com/shapedtime/momoshtrem/internal/webdav"
)

// version is set at build time with -ldflags "-X main.version=<version>".
var version = "dev"

// backfillCommand is the subcommand that writes TMDB metadata onto an
// existing library.
const backfillCommand = "backfill-metadata"

// probeCommand is the subcommand that checks HDRezka end to end from this
// host: search, title, streams and Range requests on the CDN.
const probeCommand = "hdrezka-probe"

// tmdbCacheGrace is how long expired TMDB cache entries are kept for
// stale-on-error before startup prunes them.
const tmdbCacheGrace = 30 * 24 * time.Hour

// hdrezkaReadTimeout bounds opening an HDRezka file on the CDN and each
// stretch of a read that receives nothing. The CDN answers in well under a
// second; a stalled body is reopened once at its position.
const hdrezkaReadTimeout = 30 * time.Second

func main() {
	startedAt := time.Now()

	// Parse command line flags
	configPath := flag.String("config", "config.yaml", "Path to configuration file")
	flag.Usage = func() {
		out := flag.CommandLine.Output()
		fmt.Fprintf(out, "Usage: %s [-config path] [command]\n\n", os.Args[0])
		fmt.Fprintf(out, "Without a command, runs the server. Commands:\n")
		fmt.Fprintf(out, "  %s  write TMDB metadata onto movies and shows added before it existed\n", backfillCommand)
		fmt.Fprintf(out, "                     (%s -h for its flags)\n", backfillCommand)
		fmt.Fprintf(out, "  %s      check that HDRezka resolves a title to a seekable MP4 from this host\n", probeCommand)
		fmt.Fprintf(out, "                     (no database needed; %s -h for its flags)\n\nFlags:\n", probeCommand)
		flag.PrintDefaults()
	}
	flag.Parse()

	command := flag.Arg(0)
	var backfill backfillFlags
	var probe probeFlags
	switch command {
	case "":
	case backfillCommand:
		var code int
		var ok bool
		if backfill, code, ok = parseBackfillFlags(flag.Args()[1:]); !ok {
			os.Exit(code)
		}
	case probeCommand:
		var code int
		var ok bool
		if probe, code, ok = parseProbeFlags(flag.Args()[1:]); !ok {
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

	// Load configuration (defaults → config file if present → env). The
	// probe needs no database, so it runs without DATABASE_URL too.
	load := config.Load
	if command == probeCommand {
		load = config.LoadWithoutDatabase
	}
	cfg, err := load(*configPath)
	if err != nil {
		slog.Error("Failed to load config", "error", err)
		os.Exit(1)
	}

	// The probe branches off before the database: `docker compose run
	// --no-deps` works without Postgres. It ignores HDREZKA_ENABLED, since
	// it's what tells you whether to turn HDRezka on.
	if command == probeCommand {
		os.Exit(runProbe(cfg, probe))
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
		DB:                 db,
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
	aiStore := library.NewAIStore(db)
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
	tmdbCache := library.NewTMDBCacheRepository(db)
	cachedTMDB := tmdb.NewCached(tmdbClient, tmdbCache)
	slog.Info("TMDB client initialized", "language", cfg.TMDB.Language, "rps", cfg.TMDB.RPS)

	// Drop cache entries long expired. Recently expired ones stay: they are
	// served when TMDB is down.
	if pruned, err := tmdbCache.PruneExpired(context.Background(), tmdbCacheGrace); err != nil {
		slog.Warn("Failed to prune the TMDB cache", "error", err)
	} else if pruned > 0 {
		slog.Info("Pruned expired TMDB cache entries", "rows", pruned)
	}

	// Initialize torrent storage. The piece cache's fill and capacity go to
	// the Activity summary.
	pieceStorage, pieceCache, pieceCompletion, err := torrent.InitStorage(
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

	// Create torrent service. Each torrent's .torrent file is kept in
	// {metadata_folder}/metainfo, so it loads again without waiting for
	// peers, and its data can be deleted while it isn't loaded. The speed
	// sampler reads its byte counters every 2 s.
	speeds := torrent.NewSpeedSampler()
	metainfoStore := torrent.NewMetainfoStore(filepath.Join(cfg.Torrent.MetadataFolder, "metainfo"))
	torrentService := torrent.NewService(
		torrentClient,
		activityManager,
		time.Duration(cfg.Torrent.AddTimeout)*time.Second,
		time.Duration(cfg.Torrent.ReadTimeout)*time.Second,
		torrent.WithMetainfoStore(metainfoStore),
		torrent.WithPieceStorage(pieceStorage),
		torrent.WithSpeedSampler(speeds),
	)
	stopSpeeds := make(chan struct{})
	go speeds.Run(torrentService, stopSpeeds)
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

	// Streaming sessions: who plays what over WebDAV, from every video's
	// reads (Activity, /library's streaming and /status's active_readers).
	sessionRegistry := sessions.NewRegistry(sessions.WithItems(library.NewCatalogRepository(db)))
	libraryFS.SetSessions(sessionRegistry)

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
		ActiveReaders:           sessionRegistry.Count,
		TraktConfigured:         cfg.Trakt.ClientID != "" && cfg.Trakt.ClientSecret != "",
		AIConfigured:            cfg.AI.AnthropicAPIKey != "",
		AICost:                  aiStore.MonthToDateCost,
		OpenSubtitlesConfigured: cfg.OpenSubtitles.APIKey != "",
	}
	if cfg.TMDB.APIKey != "" {
		statusOpts.TMDB = tmdbClient.Ping
	}

	// Initialize subtitle repository (always, for VFS to show existing subtitles)
	subtitleRepo := subtitle.NewRepository(db.DB)
	libraryFS.SetSubtitleRepository(subtitleRepo)

	// Initialize subtitle service. Always: stored subtitles list and torrent
	// and HDRezka subtitles are recorded without OpenSubtitles; only search
	// and download need its key.
	var osFetcher opensubtitles.SubtitleFetcher
	if cfg.OpenSubtitles.APIKey != "" {
		osFetcher = opensubtitles.NewClient(
			cfg.OpenSubtitles.APIKey,
			cfg.OpenSubtitles.Username,
			cfg.OpenSubtitles.Password,
		)
		slog.Info("Subtitle service initialized with OpenSubtitles client")
	} else {
		slog.Warn("OpenSubtitles API key not configured, subtitle download unavailable")
	}
	subtitleService := subtitle.NewService(osFetcher, subtitleRepo, cfg.Subtitles.DownloadPath)

	// HDRezka, a second source of files: its routes, the status probe and
	// streaming its files. Off, its files still list over WebDAV, but
	// their reads fail.
	if cfg.HDRezka.Enabled {
		client := hdrezka.NewClient(hdrezka.WithBaseURL(cfg.HDRezka.URL))
		resolver := hdrezka.NewResolver(client)
		streamer := hdrezka.NewStreamer(resolver, hdrezka.WithStreamUserAgent(client.UserAgent()))
		libraryFS.SetRemoteSource(service.NewHDRezkaSource(streamer), hdrezkaReadTimeout)
		hdrezkaService := service.NewHDRezkaService(service.HDRezkaDeps{
			Site:        client,
			Resolver:    resolver,
			TMDB:        cachedTMDB,
			Movies:      movieRepo,
			Shows:       showRepo,
			Assignments: assignmentRepo,
			Tree:        libraryFS,
			Subtitles:   subtitleService,
		})
		apiOpts.HDRezka = hdrezkaService
		statusOpts.HDRezka = hdrezkaService.Ping
		slog.Info("HDRezka enabled", "mirror", client.BaseURL())
	}
	// Jackett over Torznab: the status probe and per-indexer health. Torrent
	// search (#6) reuses jackettClient; nil when Jackett isn't configured.
	var jackettClient *torznab.Client
	if cfg.Jackett.URL != "" && cfg.Jackett.APIKey != "" {
		jackettClient = torznab.New(cfg.Jackett.URL, cfg.Jackett.APIKey)
		statusOpts.Jackett = status.JackettProber(jackettClient)
		slog.Info("Jackett Torznab client configured")
	}
	// Torrent search (#6): Jackett's indexers, ranked by the quality
	// profile. It also resolves Inspect's release ids.
	if jackettClient != nil {
		searchService := search.New(search.Deps{
			Titles: service.NewTitleContexts(service.TitleContextDeps{
				Titles: library.NewCatalogRepository(db),
				Shows:  showRepo,
				TMDB:   cachedTMDB,
			}),
			Jackett:  jackettClient,
			Profiles: settingsRepo,
		})
		apiOpts.Search = searchService
		apiOpts.Releases = searchService
		slog.Info("Torrent search configured")
	}
	apiOpts.Status = status.NewService(statusOpts)
	apiOpts.Settings = settingsRepo
	apiOpts.TMDB = cachedTMDB
	// Adds and refreshes library metadata; rebuilds the WebDAV tree when a
	// refresh renames an assigned episode.
	syncer := metadata.NewSyncer(cachedTMDB, movieRepo, showRepo, metadataRepo,
		metadata.WithTreeInvalidator(libraryFS))
	apiOpts.Metadata = syncer
	// Title pages and /library: library rows, cached TMDB details, live
	// torrent state and hero colours (computed in the background, cached in
	// tmdb_cache).
	heroColors := artwork.New(tmdbCache)
	apiOpts.Catalog = catalog.New(catalog.Deps{
		Store:           library.NewCatalogRepository(db),
		TMDB:            cachedTMDB,
		Torrents:        torrentService,
		Sessions:        sessionRegistry,
		Colors:          heroColors,
		Settings:        syncer,
		WebDAVPublicURL: webdavPublicURL,
		HDRezkaURL:      cfg.HDRezka.URL,
	})

	// The AI resolver (Claude Haiku) looks at the files the rules aren't
	// sure about; without ANTHROPIC_API_KEY the jobs run on rules only.
	// Its answers are cached in ai_cache and its cost recorded in ai_usage,
	// against the monthly budget.
	var resolver identify.Resolver = identify.NoopResolver{}
	classifierAI := service.NewClassifierAIStore(aiStore, settingsRepo)
	if r, err := ai.New(ai.Config{
		APIKey: cfg.AI.AnthropicAPIKey, Cache: classifierAI, Usage: classifierAI, Budget: classifierAI,
	}); err != nil {
		slog.Error("AI resolver unavailable; episode mapping runs on rules only", "error", err)
	} else if r != nil {
		resolver = r
		slog.Info("AI resolver enabled", "default_model", r.Model())
	}

	// Assignment jobs: assigning a torrent queues a job; workers fetch its
	// file list, classify the files and apply them (or wait for review).
	jobService := service.NewJobService(service.JobDeps{
		Jobs:        library.NewAssignmentJobRepository(db),
		Shows:       showRepo,
		Movies:      movieRepo,
		Assignments: assignmentRepo,
		Settings:    settingsRepo,
		Torrents:    torrentService,
		Tree:        libraryFS,
		Subtitles:   subtitleService,
		Metadata:    syncer,
		TMDB:        cachedTMDB,
		Resolver:    resolver,
		AICache:     aiStore,
	})
	apiOpts.Assignments = jobService
	apiOpts.Review = jobService
	apiOpts.Inspect = jobService
	apiOpts.Magnets = library.NewAssignmentJobRepository(db) // Re-run mapping by info_hash

	// Removals: deleting torrents and titles and unassigning files, with
	// their subtitles, jobs and the torrents nothing uses any more.
	apiOpts.Removals = service.NewRemovalService(service.RemovalDeps{
		Jobs:        library.NewAssignmentJobRepository(db),
		Movies:      movieRepo,
		Shows:       showRepo,
		Assignments: assignmentRepo,
		Torrents:    torrentService,
		Subtitles:   subtitleService,
		Tree:        libraryFS,
		Pauses:      settingsRepo,
	})

	// Activity: the torrents the library knows merged with the client's,
	// their streaming sessions and detail, the summary, and pauses kept in
	// app_settings. The stored pauses apply before anything loads a torrent
	// (the workers and WebDAV start below).
	activityService := activity.New(activity.Deps{
		Torrents: torrentService,
		Sessions: sessionRegistry,
		Library:  library.NewAssignmentJobRepository(db),
		Pauses:   settingsRepo,
		Metainfo: metainfoStore,
		Speeds:   speeds,
		Cache:    pieceCache,
		DiskPath: cfg.Torrent.MetadataFolder,
	})
	if err := activityService.RestorePauses(context.Background()); err != nil {
		slog.Error("Failed to restore torrent pauses; paused torrents run until paused again", "error", err)
	}
	apiOpts.Activity = activityService

	// Initialize servers with torrent service and tree updater
	apiServer := api.NewServer(movieRepo, showRepo, assignmentRepo, torrentService, libraryFS, apiOpts)
	apiServer.SetSubtitleService(subtitleService)
	jobService.Start()

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

	// Stop background colour computations
	heroColors.Close()

	// Stop activity manager and the speed sampler
	if activityManager != nil {
		activityManager.Stop()
	}
	close(stopSpeeds)

	// Stop air date sync service
	if airDateSync != nil {
		airDateSync.Stop()
	}

	// Stop the assignment workers before the torrent service they use; a
	// job in flight resumes on the next start.
	jobService.Close()

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
