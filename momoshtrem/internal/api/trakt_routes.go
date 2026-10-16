package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/trakt"
)

// CodeTraktUnavailable is a 502: Trakt's device-code endpoint failed.
const CodeTraktUnavailable = "trakt_unavailable"

// Trakt is the Trakt integration (PLAN §6 "Trakt (B5)"). *trakt.Auth
// implements it.
type Trakt interface {
	Integration(ctx context.Context) (trakt.Integration, error)
	Connect(ctx context.Context) (trakt.DeviceCode, error)
	Disconnect(ctx context.Context) error
	SyncNow(ctx context.Context) (trakt.Integration, error)
	UpdateSettings(ctx context.Context, patch library.TraktSettingsPatch) (library.TraktSettings, error)
}

var _ Trakt = (*trakt.Auth)(nil)

// registerTraktRoutes adds /integrations/trakt. Without Options.Trakt they
// answer 503.
func (s *Server) registerTraktRoutes(api *gin.RouterGroup) {
	api.GET("/integrations/trakt", s.getTrakt)
	api.POST("/integrations/trakt/connect", s.connectTrakt)
	api.DELETE("/integrations/trakt", s.disconnectTrakt)
	api.PUT("/integrations/trakt/settings", s.putTraktSettings)
	api.POST("/integrations/trakt/sync", s.syncTrakt)
}

func (s *Server) traktReady(c *gin.Context) bool {
	if s.opts.Trakt == nil {
		errorResponse(c, http.StatusServiceUnavailable, "Trakt isn't available.")
		return false
	}
	return true
}

// GET /integrations/trakt
func (s *Server) getTrakt(c *gin.Context) {
	if !s.traktReady(c) {
		return
	}
	in, err := s.opts.Trakt.Integration(c.Request.Context())
	if err != nil {
		traktErrorResponse(c, err)
		return
	}
	c.JSON(http.StatusOK, in)
}

// POST /integrations/trakt/connect → 200 TraktConnect
func (s *Server) connectTrakt(c *gin.Context) {
	if !s.traktReady(c) {
		return
	}
	code, err := s.opts.Trakt.Connect(c.Request.Context())
	if err != nil {
		traktErrorResponse(c, err)
		return
	}
	c.JSON(http.StatusOK, code)
}

// DELETE /integrations/trakt → 204, idempotent
func (s *Server) disconnectTrakt(c *gin.Context) {
	if !s.traktReady(c) {
		return
	}
	if err := s.opts.Trakt.Disconnect(c.Request.Context()); err != nil {
		traktErrorResponse(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// POST /integrations/trakt/sync → 202 TraktIntegration
func (s *Server) syncTrakt(c *gin.Context) {
	if !s.traktReady(c) {
		return
	}
	in, err := s.opts.Trakt.SyncNow(c.Request.Context())
	if err != nil {
		traktErrorResponse(c, err)
		return
	}
	c.JSON(http.StatusAccepted, in)
}

// PUT /integrations/trakt/settings with any of {use_ratings,
// show_watchlist, include_paused} → 200 TraktSettings. Fields left out keep
// their value and unknown ones are ignored; a value that isn't a boolean
// is 400.
func (s *Server) putTraktSettings(c *gin.Context) {
	if !s.traktReady(c) {
		return
	}
	raw, err := io.ReadAll(io.LimitReader(c.Request.Body, 64<<10))
	if err != nil {
		errorResponse(c, http.StatusBadRequest, "Couldn't read the body.")
		return
	}
	patch, ok := parseTraktSettingsPatch(raw)
	if !ok {
		errorResponse(c, http.StatusBadRequest, "use_ratings, show_watchlist and include_paused must be true or false.")
		return
	}
	settings, err := s.opts.Trakt.UpdateSettings(c.Request.Context(), patch)
	if err != nil {
		traktErrorResponse(c, err)
		return
	}
	c.JSON(http.StatusOK, settings)
}

// parseTraktSettingsPatch reads a JSON object's known fields, each true or
// false (null isn't a boolean either).
func parseTraktSettingsPatch(raw []byte) (library.TraktSettingsPatch, bool) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return library.TraktSettingsPatch{}, false
	}
	var patch library.TraktSettingsPatch
	for name, dst := range map[string]**bool{
		"use_ratings":    &patch.UseRatings,
		"show_watchlist": &patch.ShowWatchlist,
		"include_paused": &patch.IncludePaused,
	} {
		v, ok := fields[name]
		if !ok {
			continue
		}
		switch string(v) {
		case "true", "false":
			b := string(v) == "true"
			*dst = &b
		default:
			return library.TraktSettingsPatch{}, false
		}
	}
	return patch, true
}

// traktErrorResponse maps the integration's errors. Nothing maps to 401 or
// 403: the web proxy reads those as a bad backend token.
func traktErrorResponse(c *gin.Context, err error) {
	switch {
	case errors.Is(err, trakt.ErrNotConfigured):
		errorResponse(c, http.StatusServiceUnavailable, "Trakt isn’t set up: set TRAKT_CLIENT_ID, TRAKT_CLIENT_SECRET and SECRET_KEY.")
	case errors.Is(err, trakt.ErrSyncUnavailable):
		errorResponse(c, http.StatusServiceUnavailable, "Trakt sync isn’t available yet.")
	case errors.Is(err, trakt.ErrConnected):
		errorResponse(c, http.StatusConflict, "Trakt is already connected.")
	case errors.Is(err, trakt.ErrNotConnected), errors.Is(err, trakt.ErrReauth):
		errorResponse(c, http.StatusConflict, "Connect Trakt first.")
	case errors.Is(err, trakt.ErrUnavailable):
		slog.Warn("Trakt's device-code endpoint failed", "error", err)
		msg := "Trakt didn’t answer. Try again in a minute."
		if se := (*trakt.StatusError)(nil); errors.As(err, &se) && se.StatusCode < 500 && se.StatusCode != http.StatusTooManyRequests {
			msg = fmt.Sprintf("Trakt refused momoshtrem’s app (HTTP %d): check TRAKT_CLIENT_ID and TRAKT_CLIENT_SECRET.", se.StatusCode)
		}
		errorCodeResponse(c, http.StatusBadGateway, CodeTraktUnavailable, msg)
	case errors.Is(err, context.Canceled):
		errorCodeResponse(c, StatusClientClosedRequest, CodeClientClosed, "The request was canceled")
	default:
		slog.Error("Trakt request failed", "path", c.Request.URL.Path, "error", err)
		errorResponse(c, http.StatusInternalServerError, "Something went wrong with Trakt.")
	}
}
