package api

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/shapedtime/momoshtrem/internal/airdate"
	"github.com/shapedtime/momoshtrem/internal/identify"
	"github.com/shapedtime/momoshtrem/internal/library"
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
	tmdbClient      *tmdb.Client
	torrentService  torrent.Service       // Optional: nil until Stage 2 implementation
	identifier      *identify.Identifier
	treeUpdater     vfs.TreeUpdater       // Optional: updates VFS tree on assignment changes
	subtitleService *subtitle.Service     // Optional: subtitle search/download service
	airDateSync     *airdate.SyncService  // Optional: air date sync service
}

// NewServer creates a new API server
func NewServer(
	movieRepo *library.MovieRepository,
	showRepo *library.ShowRepository,
	assignmentRepo *library.AssignmentRepository,
	tmdbClient *tmdb.Client,
	torrentService torrent.Service, // Can be nil until Stage 2
	treeUpdater vfs.TreeUpdater,    // Optional: updates VFS tree on assignment changes
) *Server {
	gin.SetMode(gin.ReleaseMode)

	s := &Server{
		router:         gin.New(),
		movieRepo:      movieRepo,
		showRepo:       showRepo,
		assignmentRepo: assignmentRepo,
		tmdbClient:     tmdbClient,
		torrentService: torrentService,
		identifier:     identify.NewIdentifier(nil),
		treeUpdater:    treeUpdater,
	}

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

func (s *Server) setupMiddleware() {
	// Recovery middleware
	s.router.Use(gin.Recovery())

	// Logging middleware
	s.router.Use(func(c *gin.Context) {
		c.Next()
		slog.Info("API request",
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
		)
	})

	// CORS for development
	s.router.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusOK)
			return
		}

		c.Next()
	})
}

func (s *Server) setupRoutes() {
	api := s.router.Group("/api")

	// Movies
	api.GET("/movies", s.listMovies)
	api.POST("/movies", s.createMovie)
	api.GET("/movies/:id", s.getMovie)
	api.DELETE("/movies/:id", s.deleteMovie)
	api.POST("/movies/:id/assign-torrent", s.assignMovieTorrent) // Auto-detect movie file
	api.DELETE("/movies/:id/assign", s.unassignMovieTorrent)

	// Shows
	api.GET("/shows", s.listShows)
	api.POST("/shows", s.createShow)
	api.GET("/shows/:id", s.getShow)
	api.DELETE("/shows/:id", s.deleteShow)
	api.POST("/shows/:id/assign-torrent", s.assignShowTorrent) // Auto-detect episodes
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

// Error response helper
func errorResponse(c *gin.Context, status int, message string) {
	c.JSON(status, gin.H{"error": message})
}
