package api

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// healthzPath is the container liveness check. It sits outside /api, so it
// needs no token.
const healthzPath = "/healthz"

// healthzTimeout bounds the database ping, so a hung database fails the check
// instead of stalling it.
const healthzTimeout = 2 * time.Second

// Pinger checks that the database is reachable.
type Pinger interface {
	PingContext(ctx context.Context) error
}

// getHealthz handles GET /healthz: 200 when the database answers a ping, 503
// otherwise. Docker's healthcheck calls it, so it does no other work.
func (s *Server) getHealthz(c *gin.Context) {
	if s.opts.DB == nil {
		errorResponse(c, http.StatusServiceUnavailable, "database not configured")
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), healthzTimeout)
	defer cancel()

	if err := s.opts.DB.PingContext(ctx); err != nil {
		// Driver errors can echo connection details, so keep them in the log.
		slog.Warn("Health check: database ping failed", "error", err)
		errorResponse(c, http.StatusServiceUnavailable, "database unreachable")
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
