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

// Retrying a turn's final writes: the first retry after retryFirst, then
// twice as long each time up to retryMax, until the write timeout ends.
const (
	retryFirst = 50 * time.Millisecond
	retryMax   = time.Second
)

// turn is one ask in flight. Its hooks, show_picks and its end run on the
// turn's goroutine, in order; only the cancel watcher runs beside them, and
// it touches nothing but the outcome, the store and the slot.
type turn struct {
	s *Service
	// member is whose turn it is, and parts their titles and tools.
	member  int64
	parts   Member
	thread  library.AskThread
	turnID  int64
	message string
	refine  bool
	started time.Time
	user    aicore.Message
	req     aicore.Request
	// chips are the refinements the thread's newest done turn offered: a
	// refinement that offers none keeps them.
	chips []string

	ctx    context.Context
	cancel context.CancelFunc
	events chan recommend.Event
	stop   chan struct{} // closed when the run returns
	done   chan struct{} // closed once the turn has ended (ForgetMember)

	// What the turn showed, for its view.
	tracker     *tracker
	intro       string
	items       []viewItem
	refinements []string
	// text is the current response's text, held until the response ends:
	// only a final answer's text is shown (a note before a tool call isn't).
	text strings.Builder
	// call is the API call whose blocks are streaming, and customTools how
	// many custom tool blocks it has started.
	call, customTools int

	mu       sync.Mutex
	outcome  string // the turn's status once decided; "" while it runs
	released sync.Once
}

// run streams the turn, then ends it.
func (t *turn) run() {
	defer close(t.done)
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

// cancelNow marks the turn cancelled in the database and frees the
// member's slot. When the store can't take it, the turn is left for their
// next ask to end (Service.heal): a row left streaming would answer them
// ask_busy.
func (t *turn) cancelNow() {
	if err := t.persist("cancel", func(ctx context.Context) error {
		return t.s.d.Store.CancelTurn(ctx, t.turnID)
	}); err != nil {
		t.s.stale(t.member, t.turnID)
	}
	t.release()
}

func (t *turn) release() { t.released.Do(func() { t.s.release(t.member) }) }

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
	if t.refine && len(t.refinements) == 0 {
		t.refinements = t.chips
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

	stored := t.write(v, end)
	if !stored && outcome == library.AskTurnDone {
		// Its messages aren't stored, so it can't count as done.
		outcome, code, message = library.AskTurnError, recommend.AskErrorInternal, "Noir's answer couldn't be saved. Try again."
		v.Error = &recommend.TurnError{Code: code, Message: message}
		stored = t.write(v, library.AskTurnEnd{TurnID: t.turnID, Status: outcome, ErrorCode: code, CostUSD: res.Usage.CostUSD})
	}
	if !stored && outcome != library.AskTurnCancelled {
		t.abandon()
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

// write stores the turn's end, retrying while the store fails; it reports
// whether the turn ended that way (false too when it had already ended
// otherwise).
func (t *turn) write(v view, end library.AskTurnEnd) bool {
	doc, err := v.encode()
	if err != nil {
		t.s.log.Error("Ask: failed to encode a turn's view", "turn", t.turnID, "error", err)
		return false
	}
	end.View = doc
	var ok bool
	if err := t.persist("end "+end.Status, func(ctx context.Context) error {
		var err error
		ok, err = t.s.d.Store.EndTurn(ctx, end)
		return err
	}); err != nil {
		return false
	}
	if !ok {
		t.s.log.Warn("Ask: the turn had already ended", "turn", t.turnID, "status", end.Status)
	}
	return ok
}

// abandon is the last resort when the turn's end can't be written: the
// least that frees the one-streaming index (status error, nothing else).
// If even that fails, the member's next ask tries again (Service.heal).
func (t *turn) abandon() {
	if err := t.persist("abandon", func(ctx context.Context) error {
		return t.s.d.Store.AbandonTurn(ctx, t.turnID)
	}); err != nil {
		t.s.stale(t.member, t.turnID)
	}
}

// persist runs a store write until it succeeds or the write timeout ends,
// backing off between tries, with a context of its own (the turn's may be
// gone). It returns the last error.
func (t *turn) persist(what string, write func(ctx context.Context) error) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(t.ctx), t.s.writeTimeout)
	defer cancel()
	delay := retryFirst
	for {
		err := write(ctx)
		if err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			t.s.log.Error("Ask: a turn's write failed", "turn", t.turnID, "write", what, "error", err)
			return err
		case <-time.After(delay):
			t.s.log.Warn("Ask: retrying a turn's write", "turn", t.turnID, "write", what, "error", err)
		}
		delay = min(2*delay, retryMax)
	}
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

// emitText adds to the intro and sends it. The intro is one paragraph.
func (t *turn) emitText(text string) {
	if t.intro != "" {
		text = " " + text
	}
	t.intro += text
	t.send(recommend.Event{Name: recommend.EventText, Data: recommend.TextEvent{Delta: text}})
}

// blockStart: a server tool's block starts its step (it has run by then);
// a custom tool's does only when it's the first in its response, with no
// other tool's answer pending (toolStart starts the rest). Text held
// before a tool block was a note between tool calls, never shown.
func (t *turn) blockStart(b aicore.Block) {
	if b.Call != t.call {
		t.call, t.customTools = b.Call, 0
	}
	switch b.Type {
	case "server_tool_use":
		t.text.Reset()
		t.tracker.tally.serverTool(b.Name)
		t.reach(stepOf(b.Name, t.refine))
	case "tool_use":
		t.text.Reset()
		if t.customTools == 0 {
			t.reach(stepOf(b.Name, t.refine))
		}
		t.customTools++
	}
}

// toolStart: a custom tool is about to run, after the ones before it in
// its response answered.
func (t *turn) toolStart(call aicore.ToolCall) { t.reach(stepOf(call.Name, t.refine)) }

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
