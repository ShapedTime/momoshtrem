package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/shapedtime/momoshtrem/internal/recommend"
)

// Recommendations serves the picks (PLAN §6 "Recommendations (R1–R3)"):
// /recommendations/home, refresh and runs (R2, #157). Once wired it's
// always set, with or without ANTHROPIC_API_KEY: /home then answers
// configured: false and refresh recommend.ErrUnavailable.
type Recommendations interface {
	// Home is the newest succeeded run's shelves and the day's hero picks,
	// with the active run; feedback, library and watchlist joined.
	Home(ctx context.Context) (*recommend.Home, error)
	// Refresh queues a manual run, or answers the active one. Errors:
	// recommend.ErrUnavailable, ErrDisabled, ErrNoHistory, *LimitError.
	Refresh(ctx context.Context) (runID int64, err error)
	// Runs lists the newest runs first.
	Runs(ctx context.Context, limit int) ([]recommend.RunSummary, error)
	// Run is one run with its progress and usage; recommend.ErrNotFound
	// for an unknown one.
	Run(ctx context.Context, id int64) (*recommend.RunDetail, error)
}

// RecFeedback stores feedback on titles (R1, #153), apart from the picks
// so it can land before them. It works without a key.
type RecFeedback interface {
	// SetFeedback upserts a title's signal; the newest wins.
	SetFeedback(ctx context.Context, f recommend.Feedback) error
	// ClearFeedback removes it; idempotent.
	ClearFeedback(ctx context.Context, mediaType string, tmdbID int) error
}

// RecDebug builds R1's inputs for the dev-only GET
// /recommendations/debug/profile: the taste profile and the candidate
// pool as a run would see them now, stored nowhere.
type RecDebug interface {
	DebugProfile(ctx context.Context) (*recommend.DebugReport, error)
}

const (
	defaultRunsLimit = 10
	maxRunsLimit     = 50
)

var (
	feedbackSignals = map[string]bool{recommend.SignalLike: true, recommend.SignalDislike: true, recommend.SignalNotInterested: true, recommend.SignalSeen: true}
	feedbackSources = map[string]bool{recommend.SourceShelf: true, recommend.SourceHero: true, recommend.SourceAsk: true, recommend.SourceTitle: true}
)

// registerRecommendationRoutes adds /recommendations (but Ask's, in
// ask_routes.go). Without Options.Recommendations the picks' routes
// answer 503, and without Options.RecFeedback the feedback routes; the
// debug route exists only with Options.RecDebug.
func (s *Server) registerRecommendationRoutes(api *gin.RouterGroup) {
	api.GET("/recommendations/home", s.getRecommendationsHome)
	api.POST("/recommendations/refresh", s.refreshRecommendations)
	api.GET("/recommendations/runs", s.listRecommendationRuns)
	api.GET("/recommendations/runs/:id", s.getRecommendationRun)
	api.POST("/recommendations/feedback", s.postRecommendationFeedback)
	api.DELETE("/recommendations/feedback/:media_type/:tmdb_id", s.deleteRecommendationFeedback)
	if s.opts.RecDebug != nil {
		api.GET("/recommendations/debug/profile", s.getRecommendationsDebugProfile)
	}
}

func (s *Server) recsReady(c *gin.Context) bool {
	if s.opts.Recommendations == nil {
		errorResponse(c, http.StatusServiceUnavailable, "Recommendations aren't available")
		return false
	}
	return true
}

func (s *Server) feedbackReady(c *gin.Context) bool {
	if s.opts.RecFeedback == nil {
		errorResponse(c, http.StatusServiceUnavailable, "Feedback isn't available")
		return false
	}
	return true
}

// GET /recommendations/home → 200 Home
func (s *Server) getRecommendationsHome(c *gin.Context) {
	if !s.recsReady(c) {
		return
	}
	home, err := s.opts.Recommendations.Home(c.Request.Context())
	if err == nil && home == nil {
		home = &recommend.Home{}
	}
	if err != nil {
		recsErrorResponse(c, err)
		return
	}
	c.JSON(http.StatusOK, emptyHomeSlices(*home))
}

// POST /recommendations/refresh → 202 {run_id}
func (s *Server) refreshRecommendations(c *gin.Context) {
	if !s.recsReady(c) {
		return
	}
	id, err := s.opts.Recommendations.Refresh(c.Request.Context())
	if err != nil {
		recsErrorResponse(c, err)
		return
	}
	c.JSON(http.StatusAccepted, recommend.RunAccepted{RunID: id})
}

// GET /recommendations/runs?limit= (1–50, default 10) → {items}
func (s *Server) listRecommendationRuns(c *gin.Context) {
	limit, ok := queryRange(c, "limit", defaultRunsLimit, maxRunsLimit)
	if !ok || !s.recsReady(c) {
		return
	}
	runs, err := s.opts.Recommendations.Runs(c.Request.Context(), limit)
	if err != nil {
		recsErrorResponse(c, err)
		return
	}
	if runs == nil {
		runs = []recommend.RunSummary{}
	}
	c.JSON(http.StatusOK, gin.H{"items": runs})
}

// GET /recommendations/runs/{id} → RunDetail
func (s *Server) getRecommendationRun(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok || !s.recsReady(c) {
		return
	}
	run, err := s.opts.Recommendations.Run(c.Request.Context(), id)
	if err == nil && run == nil {
		err = recommend.ErrNotFound
	}
	if err != nil {
		recsErrorResponse(c, err)
		return
	}
	detail := *run
	detail.RunProgress = emptyProgressSlices(detail.RunProgress)
	c.JSON(http.StatusOK, detail)
}

// POST /recommendations/feedback {media_type, tmdb_id, signal, source} → 204
func (s *Server) postRecommendationFeedback(c *gin.Context) {
	if !s.feedbackReady(c) {
		return
	}
	raw, err := io.ReadAll(io.LimitReader(c.Request.Body, 64<<10))
	if err != nil {
		errorResponse(c, http.StatusBadRequest, "Couldn't read the body.")
		return
	}
	f, ok := parseFeedback(raw)
	if !ok {
		errorResponse(c, http.StatusBadRequest, "Send media_type (movie|tv), tmdb_id, signal (like|dislike|not_interested|seen) and source (shelf|hero|ask|title).")
		return
	}
	if err := s.opts.RecFeedback.SetFeedback(c.Request.Context(), f); err != nil {
		recsErrorResponse(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// DELETE /recommendations/feedback/{media_type}/{tmdb_id} → 204, idempotent
func (s *Server) deleteRecommendationFeedback(c *gin.Context) {
	mediaType := c.Param("media_type")
	tmdbID, err := strconv.Atoi(c.Param("tmdb_id"))
	if (mediaType != "movie" && mediaType != "tv") || err != nil || tmdbID <= 0 {
		errorResponse(c, http.StatusBadRequest, "Use /recommendations/feedback/{movie|tv}/{tmdb_id}.")
		return
	}
	if !s.feedbackReady(c) {
		return
	}
	if err := s.opts.RecFeedback.ClearFeedback(c.Request.Context(), mediaType, tmdbID); err != nil {
		recsErrorResponse(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// GET /recommendations/debug/profile → DebugReport (dev only: registered
// with ai.recs_debug)
func (s *Server) getRecommendationsDebugProfile(c *gin.Context) {
	report, err := s.opts.RecDebug.DebugProfile(c.Request.Context())
	if err != nil {
		recsErrorResponse(c, err)
		return
	}
	c.JSON(http.StatusOK, report)
}

// parseFeedback reads the body: every field present and known.
func parseFeedback(raw []byte) (recommend.Feedback, bool) {
	var f recommend.Feedback
	if err := json.Unmarshal(raw, &f); err != nil {
		return recommend.Feedback{}, false
	}
	ok := (f.MediaType == "movie" || f.MediaType == "tv") && f.TMDBID > 0 && feedbackSignals[f.Signal] && feedbackSources[f.Source]
	return f, ok
}

// recsErrorResponse maps the recommendations' and Ask's errors.
func recsErrorResponse(c *gin.Context, err error) {
	var limit *recommend.LimitError
	switch {
	case errors.As(err, &limit):
		msg := "This month’s AI budget is used up."
		if limit.Code == recommend.CodeAskLimit {
			msg = "That’s 30 asks today. Asks are back at midnight."
		}
		limitResponse(c, limit.Code, msg, limit.ResetsAt)
	case errors.Is(err, recommend.ErrUnavailable):
		errorResponse(c, http.StatusServiceUnavailable, "AI isn’t set up: set ANTHROPIC_API_KEY.")
	case errors.Is(err, recommend.ErrDisabled):
		errorCodeResponse(c, http.StatusConflict, CodeRecsDisabled, "Personal picks are off. Turn them on in Settings › AI & picks.")
	case errors.Is(err, recommend.ErrNoHistory):
		errorCodeResponse(c, http.StatusConflict, CodeNoHistory, "Picks need Trakt and at least 5 titles you loved or finished.")
	case errors.Is(err, recommend.ErrAskBusy):
		errorCodeResponse(c, http.StatusConflict, CodeAskBusy, "Noir is still answering. Wait for it, or stop it.")
	case errors.Is(err, recommend.ErrThreadFull):
		errorCodeResponse(c, http.StatusConflict, CodeThreadFull, "This conversation is full. Start a new one.")
	case errors.Is(err, recommend.ErrNotFound):
		errorResponse(c, http.StatusNotFound, "Not found")
	case errors.Is(err, context.Canceled):
		errorCodeResponse(c, StatusClientClosedRequest, CodeClientClosed, "The request was canceled")
	default:
		slog.Error("Recommendations request failed", "path", c.Request.URL.Path, "error", err)
		errorResponse(c, http.StatusInternalServerError, "Something went wrong with your picks.")
	}
}
