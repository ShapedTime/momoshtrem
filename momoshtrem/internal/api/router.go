package api

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/shapedtime/momoshtrem/internal/airdate"
	"github.com/shapedtime/momoshtrem/internal/identify"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/metadata"
	"github.com/shapedtime/momoshtrem/internal/service"
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
	torrentService  torrent.Service // Optional: nil until Stage 2 implementation
	identifier      *identify.Identifier
	treeUpdater     vfs.TreeUpdater      // Optional: updates VFS tree on assignment changes
	subtitleService *subtitle.Service    // Optional: subtitle search/download service
	airDateSync     *airdate.SyncService // Optional: air date sync service

	readerTracker *streaming.ReaderTracker // Optional: tracks active readers for debug endpoint

	opts Options

	// Business logic services
	showAssignmentService *service.ShowAssignmentService
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
	// Settings is the app_settings store, for the settings routes (B4, R2).
	Settings *library.SettingsRepository
	// Metadata adds and refreshes movies and shows from TMDB. Nil makes
	// those routes return 503.
	Metadata *metadata.Syncer
	// TMDB is the cached TMDB client, for routes that read TMDB data
	// directly (title pages, episode groups).
	TMDB *tmdb.Cached
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

	identifier := identify.NewIdentifier(nil)

	s := &Server{
		router:         gin.New(),
		movieRepo:      movieRepo,
		showRepo:       showRepo,
		assignmentRepo: assignmentRepo,
		torrentService: torrentService,
		identifier:     identifier,
		treeUpdater:    treeUpdater,
		opts:           opts,
	}

	// Build assignment service with optional dependencies
	var assignmentOpts []service.AssignmentServiceOption
	if treeUpdater != nil {
		assignmentOpts = append(assignmentOpts, service.WithTreeUpdater(treeUpdater))
	}
	s.showAssignmentService = service.NewShowAssignmentService(
		showRepo,
		assignmentRepo,
		torrentService,
		identifier,
		assignmentOpts...,
	)

	s.setupMiddleware()
	s.setupRoutes()

	return s
}

// SetSubtitleService configures subtitle support
func (s *Server) SetSubtitleService(svc *subtitle.Service) {
	s.subtitleService = svc
	// Configure subtitle creator on assignment service
	if s.showAssignmentService != nil {
		s.showAssignmentService.SetSubtitleCreator(svc)
	}
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
	api := s.router.Group("/api")

	// Movies
	api.GET("/movies", s.listMovies)
	api.POST("/movies", s.createMovie)
	api.GET("/movies/:id", s.getMovie)
	api.DELETE("/movies/:id", s.deleteMovie)
	api.POST("/movies/:id/refresh", s.refreshMovie)              // Re-fetch metadata from TMDB
	api.POST("/movies/:id/assign-torrent", s.assignMovieTorrent) // Auto-detect movie file
	api.DELETE("/movies/:id/assign", s.unassignMovieTorrent)

	// Shows
	api.GET("/shows", s.listShows)
	api.POST("/shows", s.createShow)
	api.GET("/shows/:id", s.getShow)
	api.DELETE("/shows/:id", s.deleteShow)
	api.POST("/shows/:id/assign-torrent", s.assignShowTorrent) // Auto-detect episodes
	api.POST("/shows/:id/refresh", s.refreshShow)              // Re-fetch seasons/episodes from TMDB
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

	// Subtitles
	api.GET("/subtitles/search", s.searchSubtitles)
	api.POST("/subtitles/download", s.downloadSubtitle)
	api.DELETE("/subtitles/:id", s.deleteSubtitle)
	api.GET("/movies/:id/subtitles", s.getMovieSubtitles)
	api.GET("/episodes/:id/subtitles", s.getEpisodeSubtitles)

	// Status
	api.GET("/status", s.getStatus)
}

// Handler returns the HTTP handler
func (s *Server) Handler() http.Handler {
	return s.router
}
