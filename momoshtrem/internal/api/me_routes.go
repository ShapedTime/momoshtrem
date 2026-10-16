package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/shapedtime/momoshtrem/internal/me"
)

// MeLists serves the user's lists (PLAN §6 "Trakt (B5)"). *me.Service
// implements it.
type MeLists interface {
	UpNext(ctx context.Context) ([]me.UpNextItem, error)
	NewEpisodes(ctx context.Context, days int) ([]me.NewEpisodeItem, error)
	Watchlist(ctx context.Context, mediaType string) ([]me.WatchlistItem, error)
	History(ctx context.Context, page, pageSize int) (*me.HistoryPage, error)
}

var _ MeLists = (*me.Service)(nil)

// TraktNudger starts an incremental Trakt sync in the background when the
// last check is over 5 minutes old, and returns at once (#132). Reading Up
// next nudges it.
type TraktNudger interface {
	Nudge()
}

// registerMeRoutes adds /me. Every route answers 503 without Options.Me.
func (s *Server) registerMeRoutes(api *gin.RouterGroup) {
	api.GET("/me/up-next", s.getUpNext)
	api.GET("/me/new-episodes", s.getNewEpisodes)
	api.GET("/me/watchlist", s.getWatchlist)
	api.GET("/me/history", s.getHistory)
}

// meReady answers 503 when /me isn't configured.
func (s *Server) meReady(c *gin.Context) bool {
	if s.opts.Me == nil {
		errorResponse(c, http.StatusServiceUnavailable, "Your lists aren't configured")
		return false
	}
	return true
}

// GET /me/up-next → {items}
func (s *Server) getUpNext(c *gin.Context) {
	if !s.meReady(c) {
		return
	}
	if s.opts.TraktNudge != nil {
		s.opts.TraktNudge.Nudge()
	}
	items, err := s.opts.Me.UpNext(c.Request.Context())
	if err != nil {
		meErrorResponse(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

// GET /me/new-episodes?days=7 (1–30) → {items}
func (s *Server) getNewEpisodes(c *gin.Context) {
	days, ok := queryRange(c, "days", me.DefaultNewEpisodeDays, me.MaxNewEpisodeDays)
	if !ok || !s.meReady(c) {
		return
	}
	items, err := s.opts.Me.NewEpisodes(c.Request.Context(), days)
	if err != nil {
		meErrorResponse(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

// GET /me/watchlist?type=movie|tv → {items}
func (s *Server) getWatchlist(c *gin.Context) {
	mediaType := c.Query("type")
	if mediaType != "" && mediaType != "movie" && mediaType != "tv" {
		errorResponse(c, http.StatusBadRequest, "type must be movie or tv")
		return
	}
	if !s.meReady(c) {
		return
	}
	items, err := s.opts.Me.Watchlist(c.Request.Context(), mediaType)
	if err != nil {
		meErrorResponse(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

// GET /me/history?page=&page_size= (50, at most 200) → Paginated<HistoryItem>
func (s *Server) getHistory(c *gin.Context) {
	page, ok := queryInt(c, "page")
	if !ok {
		return
	}
	pageSize, ok := queryRange(c, "page_size", me.DefaultHistoryPageSize, me.MaxHistoryPageSize)
	if !ok || !s.meReady(c) {
		return
	}
	result, err := s.opts.Me.History(c.Request.Context(), max(page, 1), pageSize)
	if err != nil {
		meErrorResponse(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

// queryRange reads an optional whole number from 1 to most; fallback when
// it's absent. Anything else answers 400.
func queryRange(c *gin.Context, name string, fallback, most int) (int, bool) {
	raw := c.Query(name)
	if raw == "" {
		return fallback, true
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 || n > most {
		errorResponse(c, http.StatusBadRequest, name+" must be a whole number from 1 to "+strconv.Itoa(most))
		return 0, false
	}
	return n, true
}

// meErrorResponse maps a /me failure: the database's, or a canceled
// request.
func meErrorResponse(c *gin.Context, err error) {
	if errors.Is(err, context.Canceled) {
		errorCodeResponse(c, StatusClientClosedRequest, CodeClientClosed, "The request was canceled")
		return
	}
	slog.Error("Reading your lists failed", "path", c.Request.URL.Path, "error", err)
	errorResponse(c, http.StatusInternalServerError, "Couldn't read your lists")
}
