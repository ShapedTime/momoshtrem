package api

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/shapedtime/momoshtrem/internal/airdate"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/metadata"
	"github.com/shapedtime/momoshtrem/internal/streaming"
	"github.com/shapedtime/momoshtrem/internal/subtitle"
	"github.com/shapedtime/momoshtrem/internal/tmdb"
	"github.com/shapedtime/momoshtrem/internal/torrent"
	"github.com/shapedtime/momoshtrem/internal/vfs"
)

// Server represents the REST API server
type Server struct {
	router          *gin.Engine
	movieRepo       *library.MovieRepository
	showRepo        *library.ShowRepository
	assignmentRepo  *library.AssignmentRepository
	torrentService  torrent.Service      // Optional: nil until Stage 2 implementation
	treeUpdater     vfs.TreeUpdater      // Optional: updates VFS tree on assignment changes
	subtitleService *subtitle.Service    // Optional: subtitle search/download service
	subtitleItems   subtitleItemResolver // Loads movies and episodes for the subtitle routes
	airDateSync     *airdate.SyncService // Optional: air date sync service

	readerTracker *streaming.ReaderTracker // Optional: tracks active readers for debug endpoint

	opts Options
}

// Options holds settings that must be known before routes are registered:
// Gin middleware added after route registration doesn't apply to them.
type Options struct {
	// APIToken is required in the X-Momoshtrem-Token header on every /api
	// route. Empty disables the check.
	APIToken string
	// CORSAllowedOrigins are the browser origins allowed to call the API.
	// Empty sends no CORS headers.
	CORSAllowedOrigins []string
	// Status serves GET /api/status. Nil makes the endpoint return 503.
	Status StatusReporter
	// DB is pinged by GET /healthz. Nil makes the endpoint return 503.
	DB Pinger
	// Settings is the app_settings store, for the settings routes (B4, R2).
	Settings *library.SettingsRepository
	// Metadata adds and refreshes movies and shows from TMDB. Nil makes
	// those routes return 503.
	Metadata *metadata.Syncer
	// TMDB is the cached TMDB client, for routes that read TMDB data
	// directly. Title pages and episode groups go through Catalog.
	TMDB *tmdb.Cached
	// Catalog serves the title pages, show settings and GET /library. Nil
	// makes those routes return 503.
	Catalog Catalog
	// Assignments runs the assignment jobs: assigning torrents, review and
	// apply. Nil makes those routes return 503.
	Assignments AssignmentJobs
	// Review serves the mapping review: proposal edits, the episode
	// picker, manual overrides and torrent files. Nil makes those routes
	// return 503.
	Review AssignmentReview
	// HDRezka searches HDRezka and assigns its streams. Nil (HDRezka off)
	// makes those routes return 503.
	HDRezka HDRezka
	// Removals deletes torrents and titles and unassigns files, cleaning
	// up after them. Nil makes those routes return 503.
	Removals Removals
}

// NewServer creates a new API server
func NewServer(
	movieRepo *library.MovieRepository,
	showRepo *library.ShowRepository,
	assignmentRepo *library.AssignmentRepository,
	torrentService torrent.Service, // Can be nil until Stage 2
	treeUpdater vfs.TreeUpdater, // Optional: updates VFS tree on assignment changes
	opts Options,
) *Server {
	gin.SetMode(gin.ReleaseMode)

	s := &Server{
		router:         gin.New(),
		movieRepo:      movieRepo,
		showRepo:       showRepo,
		assignmentRepo: assignmentRepo,
		torrentService: torrentService,
		treeUpdater:    treeUpdater,
		opts:           opts,
	}
	s.subtitleItems = &libraryItemResolver{movies: movieRepo, shows: showRepo, assignments: assignmentRepo}

	s.setupMiddleware()
	s.setupRoutes()

	return s
}

// SetSubtitleService configures subtitle support
func (s *Server) SetSubtitleService(svc *subtitle.Service) {
	s.subtitleService = svc
	slog.Info("Subtitle service configured")
}

// SetAirDateSyncService configures air date sync support
func (s *Server) SetAirDateSyncService(svc *airdate.SyncService) {
	s.airDateSync = svc
	slog.Info("Air date sync service configured")
}

// SetReaderTracker configures reader tracking for the debug endpoint.
func (s *Server) SetReaderTracker(rt *streaming.ReaderTracker) {
	s.readerTracker = rt
}

func (s *Server) setupMiddleware() {
	// Recovery middleware. Gin logs the panic and stack; the client gets the
	// usual error body instead of an empty 500.
	s.router.Use(gin.CustomRecovery(func(c *gin.Context, _ any) {
		errorResponse(c, http.StatusInternalServerError, "internal error")
		c.Abort()
	}))

	// Logging middleware
	s.router.Use(func(c *gin.Context) {
		c.Next()
		// Docker polls the liveness check; only its failures are worth a line.
		if c.Request.URL.Path == healthzPath && c.Writer.Status() == http.StatusOK {
			return
		}
		slog.Info("API request",
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
		)
	})

	// CORS allowlist. Runs before the token check so preflights get a 204.
	if cors := corsMiddleware(s.opts.CORSAllowedOrigins); cors != nil {
		s.router.Use(cors)
	}

	// Shared-secret auth for /api
	if auth := tokenMiddleware(s.opts.APIToken); auth != nil {
		s.router.Use(auth)
	}

	// JSON errors for unknown routes. Global middleware also wraps NoRoute,
	// so unknown /api paths need the token too.
	s.router.NoRoute(func(c *gin.Context) {
		errorResponse(c, http.StatusNotFound, "route not found")
	})
}

func (s *Server) setupRoutes() {
	// Liveness, outside /api so the token check doesn't apply
	s.router.GET(healthzPath, s.getHealthz)

	api := s.router.Group("/api")

	// Movies
	api.GET("/movies", s.listMovies)
	api.POST("/movies", s.createMovie)
	api.GET("/movies/:id", s.getMovie)
	api.DELETE("/movies/:id", s.deleteMovie)
	api.POST("/movies/:id/refresh", s.refreshMovie) // Re-fetch metadata from TMDB
	api.DELETE("/movies/:id/assign", s.unassignMovieTorrent)

	// Shows
	api.GET("/shows", s.listShows)
	api.POST("/shows", s.createShow)
	api.GET("/shows/:id", s.getShow)
	api.DELETE("/shows/:id", s.deleteShow)
	api.POST("/shows/:id/refresh", s.refreshShow) // Re-fetch seasons/episodes from TMDB
	api.GET("/shows/recently-aired", s.getRecentlyAiredEpisodes)
	api.POST("/shows/sync-air-dates", s.triggerAirDateSync)

	// Episodes - only unassign, assignment is done via show-level API
	api.DELETE("/episodes/:id/assign", s.unassignEpisodeTorrent)

	// Torrents - torrent management
	api.GET("/torrents", s.listTorrents)
	api.GET("/torrents/:hash", s.getTorrent)
	api.DELETE("/torrents/:hash", s.deleteTorrent)
	api.POST("/torrents/:hash/pause", s.pauseTorrent)
	api.POST("/torrents/:hash/resume", s.resumeTorrent)
	api.GET("/torrents/:hash/debug", s.getTorrentDebug)

	// Subtitles: item-scoped under /movies/:id and /episodes/:id
	s.registerSubtitleRoutes(api)

	// Status
	api.GET("/status", s.getStatus)

	s.registerTitleRoutes(api)      // Title pages, seasons, show settings and episode groups
	s.registerLibraryRoutes(api)    // GET /library
	s.registerAssignmentRoutes(api) // Assign torrents (jobs), review and apply
	s.registerReviewRoutes(api)     // Proposal edits, episode picker, manual overrides, torrent files
	s.registerHDRezkaRoutes(api)    // HDRezka search, titles, streams and assigns
}

// Handler returns the HTTP handler
func (s *Server) Handler() http.Handler {
	return s.router
}
