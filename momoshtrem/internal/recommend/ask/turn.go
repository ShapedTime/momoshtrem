package ask

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/shapedtime/momoshtrem/internal/aicore"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/recommend"
)

// turn is one ask in flight. Its hooks, show_picks and its end run on the
// turn's goroutine, in order; only the cancel watcher runs beside them, and
// it touches nothing but the outcome and the slot.
type turn struct {
	s       *Service
	thread  library.AskThread
	turnID  int64
	message string
	refine  bool
	started time.Time
	user    aicore.Message
	req     aicore.Request

	ctx    context.Context
	cancel context.CancelFunc
	events chan recommend.Event
	stop   chan struct{} // closed when the run returns

	// What the turn showed, for its view.
	tracker     *tracker
	intro       string
	items       []viewItem
	refinements []string
	// text is the current response's text, held until the response ends:
	// only a final answer's text is shown (a note before a tool call isn't).
	text strings.Builder

	mu       sync.Mutex
	outcome  string // the turn's status once decided; "" while it runs
	released sync.Once
}

// run streams the turn, then ends it.
func (t *turn) run() {
	defer t.s.untrack(t)
	defer close(t.events)
	defer t.cancel()
	go t.watch()

	t.send(recommend.Event{Name: recommend.EventStart, Data: recommend.StartEvent{
		ThreadID: t.thread.ID, TurnID: t.turnID, Title: t.thread.Title}})
	res, err := t.s.d.Runner.Run(t.ctx, t.req)
	close(t.stop)
	t.end(res, err)
}

// watch cancels the turn as soon as its context ends, before the run has
// stopped and recorded what it spent: the slot is free at once, so a
// refinement sent right after Stop doesn't meet ask_busy.
func (t *turn) watch() {
	select {
	case <-t.ctx.Done():
		if t.decide(library.AskTurnCancelled) {
			t.cancelNow()
		}
	case <-t.stop:
	}
}

// decide settles the turn's outcome once; it reports whether this call
// settled it (else the outcome stays what was decided first).
func (t *turn) decide(outcome string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.outcome != "" {
		return false
	}
	t.outcome = outcome
	return true
}

func (t *turn) decided() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.outcome
}

// cancelNow marks the turn cancelled in the database and frees the slot.
func (t *turn) cancelNow() {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(t.ctx), writeTimeout)
	defer cancel()
	if err := t.s.d.Store.CancelTurn(ctx, t.turnID); err != nil {
		t.s.log.Error("Ask: failed to mark a turn cancelled", "turn", t.turnID, "error", err)
	}
	t.release()
}

func (t *turn) release() { t.released.Do(t.s.release) }

// end writes how the turn ended, frees the slot, and only then sends done
// or error: the web closes the stream as soon as it reads either, and a
// follow-up must find the turn ended. A context cancelled after the turn
// was decided done changes nothing.
func (t *turn) end(res *aicore.Result, err error) {
	if res == nil {
		res = &aicore.Result{}
	}
	outcome, code, message := library.AskTurnDone, "", ""
	switch {
	case err == nil:
	case t.ctx.Err() != nil:
		outcome = library.AskTurnCancelled
	default:
		outcome, code, message = library.AskTurnError, errorCode(err), errorMessage(err)
	}
	if t.decide(outcome) {
		if outcome == library.AskTurnCancelled {
			t.cancelNow()
		}
	} else {
		outcome = t.decided() // cancelled by the watcher first
	}

	elapsed := time.Since(t.started).Milliseconds()
	v := view{Intro: t.intro, Items: t.items, Refinements: t.refinements, ElapsedMS: &elapsed}
	end := library.AskTurnEnd{TurnID: t.turnID, Status: outcome, ErrorCode: code, CostUSD: res.Usage.CostUSD}
	if outcome == library.AskTurnDone {
		for _, step := range t.tracker.finish() {
			t.send(recommend.Event{Name: recommend.EventStatus, Data: step})
		}
		end.Messages = encodeMessages(append([]aicore.Message{t.user}, res.Messages...))
	}
	v.Steps = t.tracker.ended()
	if outcome == library.AskTurnError {
		v.Error = &recommend.TurnError{Code: code, Message: message}
		t.s.log.Warn("Ask turn failed", "turn", t.turnID, "code", code, "error", err)
	}

	if ok := t.write(v, end); !ok && outcome == library.AskTurnDone {
		// Its messages aren't stored, so it can't count as done.
		outcome, code = library.AskTurnError, recommend.AskErrorInternal
		message = "Noir's answer couldn't be saved. Try again."
		v.Error = &recommend.TurnError{Code: code, Message: message}
		t.write(v, library.AskTurnEnd{TurnID: t.turnID, Status: outcome, ErrorCode: code, CostUSD: res.Usage.CostUSD})
	}
	t.release()
	t.s.log.Info("Ask turn ended", "thread", t.thread.ID, "turn", t.turnID, "status", outcome, "refine", t.refine,
		"picks", len(t.items), "calls", res.Usage.Calls, "tool_calls", res.ToolCalls, "cost_usd", res.Usage.CostUSD, "elapsed_ms", elapsed)

	switch outcome {
	case library.AskTurnDone:
		t.send(recommend.Event{Name: recommend.EventDone, Data: recommend.DoneEvent{
			ThreadID: t.thread.ID, TurnID: t.turnID, ElapsedMS: elapsed, Refinements: orEmpty(t.refinements),
			Usage: recommend.TurnUsage{InputTokens: res.Usage.Prompt(), OutputTokens: res.Usage.Output,
				WebSearches: int(res.Usage.WebSearches), CostUSD: res.Usage.CostUSD},
		}})
	case library.AskTurnError:
		t.send(recommend.Event{Name: recommend.EventError, Data: recommend.TurnError{Code: code, Message: message}})
	}
}

// write stores the turn's end, with a context of its own (the turn's may
// be gone); false when it wasn't stored.
func (t *turn) write(v view, end library.AskTurnEnd) bool {
	doc, err := v.encode()
	if err != nil {
		t.s.log.Error("Ask: failed to encode a turn's view", "turn", t.turnID, "error", err)
		return false
	}
	end.View = doc
	ctx, cancel := context.WithTimeout(context.WithoutCancel(t.ctx), writeTimeout)
	defer cancel()
	ok, err := t.s.d.Store.EndTurn(ctx, end)
	if err != nil {
		t.s.log.Error("Ask: failed to store how a turn ended", "turn", t.turnID, "status", end.Status, "error", err)
		return false
	}
	if !ok {
		t.s.log.Warn("Ask: the turn had already ended", "turn", t.turnID, "status", end.Status)
	}
	return ok
}

// send sends an event unless the turn's context is done: the client has
// gone, and the loop must never block on it.
func (t *turn) send(e recommend.Event) {
	select {
	case t.events <- e:
	case <-t.ctx.Done():
	}
}

// reach starts a step (status events).
func (t *turn) reach(step string) {
	for _, s := range t.tracker.reach(step) {
		t.send(recommend.Event{Name: recommend.EventStatus, Data: s})
	}
}

// emitText adds to the intro and sends it.
func (t *turn) emitText(text string) {
	if t.intro != "" {
		text = "\n\n" + text
	}
	t.intro += text
	t.send(recommend.Event{Name: recommend.EventText, Data: recommend.TextEvent{Delta: text}})
}

// blockStart: a tool's block starts its step. Text held before it was a
// note between tool calls, never shown.
func (t *turn) blockStart(b aicore.Block) {
	switch b.Type {
	case "tool_use", "server_tool_use":
		t.text.Reset()
		if b.Type == "server_tool_use" {
			t.tracker.tally.serverTool(b.Name)
		}
		t.reach(stepOf(b.Name, t.refine))
	}
}

func (t *turn) textDelta(delta string) { t.text.WriteString(delta) }

// callEnded: a response that ends the turn shows its text as the answer,
// unless show_picks already showed one (then it's a closing line).
func (t *turn) callEnded(info aicore.CallInfo) {
	text := strings.TrimSpace(t.text.String())
	t.text.Reset()
	if info.Err != nil || info.StopReason != "end_turn" || text == "" || len(t.items) > 0 {
		return
	}
	t.reach(recommend.StepWrite)
	t.emitText(text)
}

// errorCode is the error event's code for a run's error.
func errorCode(err error) string {
	var api *aicore.APIError
	switch {
	case errors.Is(err, aicore.ErrRefused):
		return recommend.AskErrorRefused
	case errors.Is(err, aicore.ErrTruncated), errors.Is(err, aicore.ErrCapReached):
		return recommend.AskErrorLimitReached
	case errors.As(err, &api), errors.Is(err, aicore.ErrEmptyAnswer):
		return recommend.AskErrorUpstream
	}
	return recommend.AskErrorInternal
}

// errorMessage is what the user reads about it.
func errorMessage(err error) string {
	switch errorCode(err) {
	case recommend.AskErrorRefused:
		return "Claude declined this one. Try asking another way."
	case recommend.AskErrorLimitReached:
		return "This ask grew past Noir's limit for one answer. Try a narrower ask."
	case recommend.AskErrorUpstream:
		return "The Anthropic API didn't answer. Try again in a minute."
	}
	return "Something went wrong while Noir was answering. Try again."
}
