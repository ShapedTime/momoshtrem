package api

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/shapedtime/momoshtrem/internal/status"
)

// StatusReporter builds the system status report.
type StatusReporter interface {
	Report(ctx context.Context, refresh bool) status.Report
}

// getStatus handles GET /api/status[?refresh=1].
// It returns 200 even when a probe fails (the body says what's wrong) and
// 503 only when the database is down.
func (s *Server) getStatus(c *gin.Context) {
	if s.opts.Status == nil {
		errorResponse(c, http.StatusServiceUnavailable, "status reporting is not configured")
		return
	}

	refresh := c.Query("refresh") == "1" || c.Query("refresh") == "true"
	report := s.opts.Status.Report(c.Request.Context(), refresh)

	code := http.StatusOK
	if !report.DB.OK {
		code = http.StatusServiceUnavailable
	}
	c.JSON(code, report)
}
