package api

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/tmdb"
)

// Server represents the REST API server
type Server struct {
	router         *gin.Engine
	movieRepo      *library.MovieRepository
	showRepo       *library.ShowRepository
	assignmentRepo *library.AssignmentRepository
	tmdbClient     *tmdb.Client
}

// NewServer creates a new API server
func NewServer(
	movieRepo *library.MovieRepository,
	showRepo *library.ShowRepository,
	assignmentRepo *library.AssignmentRepository,
	tmdbClient *tmdb.Client,
) *Server {
	gin.SetMode(gin.ReleaseMode)

	s := &Server{
		router:         gin.New(),
		movieRepo:      movieRepo,
		showRepo:       showRepo,
		assignmentRepo: assignmentRepo,
		tmdbClient:     tmdbClient,
	}

	s.setupMiddleware()
	s.setupRoutes()

	return s
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
	api.POST("/movies/:id/assign", s.assignMovieTorrent)
	api.DELETE("/movies/:id/assign", s.unassignMovieTorrent)

	// Shows
	api.GET("/shows", s.listShows)
	api.POST("/shows", s.createShow)
	api.GET("/shows/:id", s.getShow)
	api.DELETE("/shows/:id", s.deleteShow)

	// Episodes
	api.POST("/episodes/:id/assign", s.assignEpisodeTorrent)
	api.DELETE("/episodes/:id/assign", s.unassignEpisodeTorrent)

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
