package api

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/metadata"
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
	// Inspect creates Inspect's dry runs (POST /search/torrents/inspect).
	// Nil makes the route return 503.
	Inspect Inspector
	// Releases resolves a search result's release_id for Inspect. Nil
	// answers 404 "search again" for a release_id; a magnet_uri still
	// works.
	Releases ReleaseResolver
	// Activity lists the torrents, sums them up, and pauses and resumes
	// them. Nil makes those routes return 503.
	Activity Activity
	// Search serves torrent search (GET /search/torrents and resolve). Nil
	// (no Jackett) makes those routes return 503.
	Search TorrentSearch
	// Magnets finds a magnet for a torrent the library knows, so the
	// assign routes take {info_hash} (Re-run mapping). Nil makes an assign
	// by info_hash return 503.
	Magnets TorrentMagnets
	// Trakt serves /integrations/trakt: connect, disconnect, settings and
	// sync (B5). Nil makes those routes return 503.
	Trakt Trakt
	// Me serves /me: Up next, new episodes, the watchlist and the history
	// (B5). Nil makes those routes return 503.
	Me MeLists
	// TraktNudge is nudged by GET /me/up-next (#132). Nil: reading Up next
	// never starts a sync.
	TraktNudge TraktNudger
	// Recommendations serves the picks: /recommendations/home, refresh and
	// runs (R2, #157; cmd/momoshtrem/wire_recs.go). Nil makes those routes
	// return 503.
	Recommendations Recommendations
	// RecFeedback serves /recommendations/feedback (R1, #153;
	// wire_recs.go). Nil makes those routes return 503.
	RecFeedback RecFeedback
	// RecDebug serves the dev-only GET /recommendations/debug/profile
	// (R1, #153), set only with ai.recs_debug (RECS_DEBUG). Nil leaves
	// the route unregistered (404).
	RecDebug RecDebug
	// Ask serves Ask momoshtrem over SSE and its threads (R3, #158;
	// wire_ask.go). Nil makes those routes return 503.
	Ask Ask
	// AISettings serves GET/PUT /settings/ai and /settings/ai/usage (the
	// AI core, #154; wire_ai.go). Nil makes those routes return 503.
	AISettings AISettings
	// Storage serves GET/PUT /settings/storage: the piece cache's limit
	// and the download caps (#77; wire_storage.go). Nil makes those routes
	// return 503.
	Storage Storage
	// Downloads serves /downloads: Keeps, smart picks and the estimate
	// (#77 P4; wire_downloads.go). Nil makes those routes return 503.
	Downloads Downloads
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

	// Episodes - only unassign, assignment is done via show-level API
	api.DELETE("/episodes/:id/assign", s.unassignEpisodeTorrent)

	// Torrents: the list, summary, pauses, deletes, legacy status and debug
	s.registerTorrentRoutes(api)

	// Subtitles: item-scoped under /movies/:id and /episodes/:id
	s.registerSubtitleRoutes(api)

	// Status
	api.GET("/status", s.getStatus)

	s.registerTitleRoutes(api)      // Title pages, seasons, show settings and episode groups
	s.registerLibraryRoutes(api)    // GET /library
	s.registerAssignmentRoutes(api) // Assign torrents (jobs), review and apply
	s.registerReviewRoutes(api)     // Proposal edits, episode picker, manual overrides, torrent files
	s.registerHDRezkaRoutes(api)    // HDRezka search, titles, streams and assigns
	s.registerQualityRoutes(api)    // GET and PUT /settings/quality-profile
	s.registerStorageRoutes(api)    // GET and PUT /settings/storage (#77)
	s.registerDownloadRoutes(api)   // Keeps, smart picks and the estimate (#77)
	s.registerInspectRoutes(api)    // Inspect: dry runs of a search result or magnet
	s.registerSearchRoutes(api)     // Torrent search and resolve
	s.registerTraktRoutes(api)      // Trakt connect, disconnect, settings and sync
	s.registerMeRoutes(api)         // Up next, new episodes, watchlist and history (B5)

	// AI recommendations and Ask momoshtrem (#9)
	s.registerRecommendationRoutes(api) // Picks: home, refresh, runs and feedback
	s.registerAskRoutes(api)            // Ask over SSE, and its threads
	s.registerAISettingsRoutes(api)     // GET/PUT /settings/ai and its usage
}

// Handler returns the HTTP handler
func (s *Server) Handler() http.Handler {
	return s.router
}
