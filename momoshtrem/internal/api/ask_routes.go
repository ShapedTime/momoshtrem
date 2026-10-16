package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"

	"github.com/shapedtime/momoshtrem/internal/recommend"
)

// Ask serves Ask momoshtrem (PLAN §6 "Recommendations (R1–R3)", R3, #158):
// the threads, and a turn streamed over SSE.
type Ask interface {
	// Threads lists the threads, the newest updated first (at most 50).
	Threads(ctx context.Context) ([]recommend.ThreadSummary, error)
	// Thread is one thread and its turns, feedback, library and watchlist
	// joined; recommend.ErrNotFound for an unknown one.
	Thread(ctx context.Context, id int64) (*recommend.Thread, error)
	// Ask runs the checks that answer before the stream, in PLAN §6's
	// order (recommend.ErrUnavailable, ErrNotFound, ErrThreadFull,
	// ErrAskBusy, *LimitError), then starts the turn. Its events arrive on
	// the channel, start first and done or error last; then it's closed.
	// The turn runs under ctx: when the client goes away ctx is cancelled,
	// and Ask must mark the turn cancelled, record what it spent, and
	// never block sending once ctx is done. Shutdown does the same: the
	// server's RegisterOnShutdown hook (cmd/momoshtrem) cancels every turn
	// in flight and closes its channel, since http.Server.Shutdown doesn't
	// cancel request contexts and a stream would otherwise hold it.
	Ask(ctx context.Context, req recommend.AskRequest) (<-chan recommend.Event, error)
}

// askPingInterval is how often an idle stream gets a ": ping" comment, so
// proxies don't time it out. Tests shorten it.
var askPingInterval = 15 * time.Second

// registerAskRoutes adds Ask and its threads. Without Options.Ask they
// answer 503.
func (s *Server) registerAskRoutes(api *gin.RouterGroup) {
	api.GET("/recommendations/threads", s.listAskThreads)
	api.GET("/recommendations/threads/:id", s.getAskThread)
	api.POST("/recommendations/ask", s.postAsk)
}

func (s *Server) askReady(c *gin.Context) bool {
	if s.opts.Ask == nil {
		errorResponse(c, http.StatusServiceUnavailable, "Ask momoshtrem isn't available")
		return false
	}
	return true
}

// GET /recommendations/threads → {items}
func (s *Server) listAskThreads(c *gin.Context) {
	if !s.askReady(c) {
		return
	}
	threads, err := s.opts.Ask.Threads(c.Request.Context())
	if err != nil {
		recsErrorResponse(c, err)
		return
	}
	if threads == nil {
		threads = []recommend.ThreadSummary{}
	}
	c.JSON(http.StatusOK, gin.H{"items": threads})
}

// GET /recommendations/threads/{id} → Thread
func (s *Server) getAskThread(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok || !s.askReady(c) {
		return
	}
	thread, err := s.opts.Ask.Thread(c.Request.Context(), id)
	if err == nil && thread == nil {
		err = recommend.ErrNotFound
	}
	if err != nil {
		recsErrorResponse(c, err)
		return
	}
	c.JSON(http.StatusOK, emptyThreadSlices(*thread))
}

// POST /recommendations/ask {thread_id?, message} → JSON errors, then
// 200 text/event-stream.
func (s *Server) postAsk(c *gin.Context) {
	raw, err := io.ReadAll(io.LimitReader(c.Request.Body, 64<<10))
	if err != nil {
		errorResponse(c, http.StatusBadRequest, "Couldn't read the body.")
		return
	}
	req, ok := parseAskRequest(raw)
	if !ok {
		errorResponse(c, http.StatusBadRequest, "Send a message of 1–500 characters, and thread_id to follow up.")
		return
	}
	if !s.askReady(c) {
		return
	}
	ctx := c.Request.Context()
	events, err := s.opts.Ask.Ask(ctx, req)
	if err != nil {
		recsErrorResponse(c, err)
		return
	}
	streamAskEvents(ctx, c, events)
}

// parseAskRequest reads the body: a message of 1–500 characters once
// trimmed, and an optional positive thread_id (null is none).
func parseAskRequest(raw []byte) (recommend.AskRequest, bool) {
	var body struct {
		ThreadID *int64 `json:"thread_id"`
		Message  string `json:"message"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return recommend.AskRequest{}, false
	}
	message := strings.TrimSpace(body.Message)
	if message == "" || utf8.RuneCountInString(message) > recommend.AskMessageMaxChars || (body.ThreadID != nil && *body.ThreadID <= 0) {
		return recommend.AskRequest{}, false
	}
	return recommend.AskRequest{ThreadID: body.ThreadID, Message: message}, true
}

// streamAskEvents writes the turn's events as Server-Sent Events, flushing
// each, with a ": ping" comment every askPingInterval, until the channel
// closes or the client goes away.
func streamAskEvents(ctx context.Context, c *gin.Context, events <-chan recommend.Event) {
	h := c.Writer.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache, no-transform")
	h.Set("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)
	c.Writer.WriteHeaderNow()
	c.Writer.Flush()

	ping := time.NewTicker(askPingInterval)
	defer ping.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ping.C:
			if _, err := io.WriteString(c.Writer, ": ping\n\n"); err != nil {
				return
			}
			c.Writer.Flush()
		case event, open := <-events:
			if !open {
				return
			}
			data, err := json.Marshal(emptyEventSlices(event).Data)
			if err != nil {
				slog.Error("Couldn't encode an Ask event", "event", event.Name, "error", err)
				continue
			}
			if _, err := fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", event.Name, data); err != nil {
				return
			}
			c.Writer.Flush()
		}
	}
}
