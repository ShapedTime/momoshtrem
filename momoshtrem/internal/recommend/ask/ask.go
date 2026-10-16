// Package ask is Ask momoshtrem (PLAN §6 "Ask momoshtrem", §7 R3; #158):
// a conversation with the picks agent, streamed over SSE, in threads.
//
// A turn runs on the AI core's loop (aicore.Client.Run) with R1's tools,
// web search and fetch, and show_picks, the strict tool through which the
// agent shows its intro, its picks (each checked against what you've seen
// and turned down, resolved on TMDB and sent at once) and its refinement
// chips. Its progress becomes the stream's events: start, status (from the
// tools' block starts), text, item, then done or error.
//
// Threads are append-only. A follow-up replays the thread's stored API
// messages unchanged, on the model and prompt version the thread started
// with; a turn's messages are stored only when it ends done, in one
// transaction, so a cancelled or failed turn never replays (and never
// leaves an orphaned tool_use). One turn streams at a time, in any thread.
package ask

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/shapedtime/momoshtrem/internal/aicore"
	"github.com/shapedtime/momoshtrem/internal/catalog"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/recommend"
	"github.com/shapedtime/momoshtrem/internal/recommend/agent"
	"github.com/shapedtime/momoshtrem/internal/tmdb"
)

// Runner runs a tool loop (aicore.Client).
type Runner interface {
	Run(ctx context.Context, req aicore.Request) (*aicore.Result, error)
}

// Budget checks the monthly AI budget (aicore.Ledger): a *aicore.BudgetError
// once it's spent.
type Budget interface {
	Check(ctx context.Context) error
}

// Store keeps the threads (library.AskRepository).
type Store interface {
	Thread(ctx context.Context, id int64) (*library.AskThread, error)
	Threads(ctx context.Context, limit int) ([]library.AskThread, error)
	Turns(ctx context.Context, threadID int64) ([]library.AskTurn, error)
	Messages(ctx context.Context, threadID int64) ([]library.AskMessage, error)
	StartTurn(ctx context.Context, in library.NewAskTurn) (*library.AskThread, *library.AskTurn, error)
	CancelTurn(ctx context.Context, turnID int64) error
	EndTurn(ctx context.Context, end library.AskTurnEnd) (bool, error)
	AbandonTurn(ctx context.Context, turnID int64) error
	FailStreaming(ctx context.Context, code string, view []byte) (int64, error)
	TurnsSince(ctx context.Context, since time.Time) (int, error)
	MeanDoneCost(ctx context.Context, n int) (*float64, error)
}

// Settings reads rec_model, the model new threads start on
// (library.SettingsRepository).
type Settings interface {
	AISettings(ctx context.Context) (library.AISettings, error)
}

// Titles looks up what's known about titles: watched, rated, hidden,
// feedback, the watchlist (library.TasteRepository).
type Titles interface {
	TitleStates(ctx context.Context, refs []library.TitleRef) ([]library.TitleState, error)
}

// Library says where titles are in the library (catalog.Service).
type Library interface {
	LibraryStates(ctx context.Context, ids library.TMDBIDs) (map[catalog.TitleRef]*catalog.SimilarLibrary, error)
}

// TMDB describes a pick (tmdb.Cached).
type TMDB interface {
	MovieDetails(ctx context.Context, id int) (*tmdb.MovieDetails, error)
	ShowDetails(ctx context.Context, id int) (*tmdb.ShowDetails, error)
}

// Deps are what Ask is built from.
type Deps struct {
	// Runner is nil without ANTHROPIC_API_KEY: asking answers
	// recommend.ErrUnavailable, and threads still read.
	Runner   Runner
	Budget   Budget
	Store    Store
	Settings Settings
	Titles   Titles
	Library  Library
	TMDB     TMDB
	// Tools are R1's custom tools (tools.All over a tools.Cached), in
	// their fixed order; show_picks is added after them.
	Tools []agent.Tool
	// Now is the clock; time.Now by default. Days are server-local.
	Now func() time.Time
	// WriteTimeout bounds a turn's final writes, retries included;
	// DefaultWriteTimeout when 0.
	WriteTimeout time.Duration
}

// DefaultWriteTimeout bounds a turn's final writes, which run after its
// context may have ended, with their retries.
const DefaultWriteTimeout = 10 * time.Second

// Limits.
const (
	threadsListed = 50
	// estimateTurns is how many done turns ask_usd averages.
	estimateTurns = 20
	// maxTokens bounds one response: adaptive thinking and a short answer.
	maxTokens = 32000
	// closeWait is how long Close waits for the turns it cancelled to
	// write how they ended.
	closeWait = 5 * time.Second
)

// turnCaps bound one turn: it ends with limit_reached past them. There's
// no token cap: tokens count cache reads, so a long thread's replays would
// trip one on a cheap turn; the cost cap bounds what they spend instead.
// Once show_picks has shown a pick, one call past a cap may end the turn
// (aicore.Request.AllowFinal), so a shown answer isn't turned into an
// error.
var turnCaps = aicore.Caps{Calls: 12, ToolCalls: 24, CostUSD: 1.0}

// Service implements api.Ask, and aisettings.AskReporter for the usage
// page.
type Service struct {
	d   Deps
	log *slog.Logger

	writeTimeout time.Duration

	mu      sync.Mutex
	busy    bool // a turn holds the one slot
	closed  bool
	running map[*turn]struct{}
	// unended are turns whose end couldn't be written: their rows may
	// still say streaming, which answers ask_busy, so the next ask ends
	// them (heal).
	unended map[int64]bool
	wg      sync.WaitGroup
}

// New creates the service.
func New(d Deps) *Service {
	if d.Now == nil {
		d.Now = time.Now
	}
	timeout := d.WriteTimeout
	if timeout <= 0 {
		timeout = DefaultWriteTimeout
	}
	return &Service{d: d, log: slog.With("component", "ask"), writeTimeout: timeout,
		running: map[*turn]struct{}{}, unended: map[int64]bool{}}
}

// Recover ends the turns a crash left streaming as errors (internal), at
// startup, before any turn runs. It returns how many.
func (s *Service) Recover(ctx context.Context) (int64, error) {
	v, err := view{Error: &recommend.TurnError{Code: recommend.AskErrorInternal,
		Message: "momoshtrem restarted before Noir finished. Try again."}}.encode()
	if err != nil {
		return 0, err
	}
	return s.d.Store.FailStreaming(ctx, recommend.AskErrorInternal, v)
}

// Threads lists the threads, the newest updated first (at most 50).
func (s *Service) Threads(ctx context.Context) ([]recommend.ThreadSummary, error) {
	rows, err := s.d.Store.Threads(ctx, threadsListed)
	if err != nil {
		return nil, err
	}
	out := make([]recommend.ThreadSummary, len(rows))
	for i, t := range rows {
		out[i] = summaryOf(t)
	}
	return out, nil
}

func summaryOf(t library.AskThread) recommend.ThreadSummary {
	return recommend.ThreadSummary{ID: t.ID, Title: t.Title, CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt, TurnCount: t.TurnCount}
}

// Thread is one thread with its turns, library, watchlist and feedback
// read now; titles with negative feedback are left out.
func (s *Service) Thread(ctx context.Context, id int64) (*recommend.Thread, error) {
	t, err := s.d.Store.Thread(ctx, id)
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, recommend.ErrNotFound
	}
	turns, err := s.d.Store.Turns(ctx, id)
	if err != nil {
		return nil, err
	}
	views := make([]*view, len(turns))
	var refs []library.TitleRef
	seen := map[library.TitleRef]bool{}
	for i, turn := range turns {
		if views[i], err = decodeView(turn.View); err != nil {
			return nil, err
		}
		if views[i] == nil {
			continue
		}
		for _, it := range views[i].Items {
			if ref := it.ref(); !seen[ref] {
				seen[ref] = true
				refs = append(refs, ref)
			}
		}
	}
	l, err := readLive(ctx, s.d.Titles, s.d.Library, refs)
	if err != nil {
		return nil, err
	}
	out := &recommend.Thread{ThreadSummary: summaryOf(*t), Model: t.Model, Turns: make([]recommend.Turn, len(turns))}
	for i, turn := range turns {
		out.Turns[i] = turnOf(turn, views[i], l)
	}
	return out, nil
}

// AskEstimate is the mean cost of the last 20 done turns; nil without any.
func (s *Service) AskEstimate(ctx context.Context) (*float64, error) {
	return s.d.Store.MeanDoneCost(ctx, estimateTurns)
}

// AsksToday counts the turns started since dayStart, cancelled and failed
// ones included.
func (s *Service) AsksToday(ctx context.Context, dayStart time.Time) (int, error) {
	return s.d.Store.TurnsSince(ctx, dayStart)
}

// Ask checks what answers before the stream, in PLAN §6's order (the body
// is the handler's): no key (ErrUnavailable), an unknown thread
// (ErrNotFound), a full one (ErrThreadFull), a turn streaming
// (ErrAskBusy), today's asks and the budget (*recommend.LimitError). Then
// it starts the turn, whose events arrive on the channel: start first,
// done or error last (none after a cancel), then it's closed. The turn
// runs under ctx: when it's cancelled (the client went away) the turn ends
// cancelled, with what it spent recorded, and frees the slot at once.
func (s *Service) Ask(ctx context.Context, req recommend.AskRequest) (<-chan recommend.Event, error) {
	if s.d.Runner == nil || s.isClosed() {
		return nil, recommend.ErrUnavailable
	}
	message := strings.TrimSpace(req.Message)
	var (
		thread *library.AskThread
		err    error
	)
	if req.ThreadID != nil {
		if thread, err = s.d.Store.Thread(ctx, *req.ThreadID); err != nil {
			return nil, err
		}
		if thread == nil {
			return nil, recommend.ErrNotFound
		}
		if thread.TurnCount >= recommend.ThreadMaxTurns {
			return nil, recommend.ErrThreadFull
		}
		if _, ok := prompts[thread.PromptVersion]; !ok {
			// Its system prompt and tools are gone with an older build: the
			// thread can't go on without editing its prefix.
			s.log.Warn("Ask thread started on a prompt this build doesn't have", "thread", thread.ID, "prompt_version", thread.PromptVersion)
			return nil, recommend.ErrThreadFull
		}
	}
	if !s.acquire() {
		return nil, recommend.ErrAskBusy
	}
	started := false
	defer func() {
		if !started {
			s.release()
		}
	}()
	s.heal(ctx)

	now := s.d.Now()
	dayStart, nextDay := aicore.DayBounds(now)
	asked, err := s.d.Store.TurnsSince(ctx, dayStart)
	if err != nil {
		return nil, err
	}
	if asked >= recommend.AskDailyLimit {
		return nil, &recommend.LimitError{Code: recommend.CodeAskLimit, ResetsAt: nextDay}
	}

	t := &turn{s: s, message: message, started: now, tracker: newTracker()}
	if err := s.prepare(ctx, t, thread); err != nil {
		return nil, err
	}
	if err := t.req.Validate(); err != nil {
		return nil, fmt.Errorf("ask: %w", err)
	}
	if err := s.d.Budget.Check(ctx); err != nil {
		var budget *aicore.BudgetError
		if errors.As(err, &budget) {
			return nil, &recommend.LimitError{Code: recommend.CodeBudgetExceeded, ResetsAt: budget.ResetsAt}
		}
		return nil, err
	}

	newTurn := library.NewAskTurn{Message: message}
	if thread != nil {
		newTurn.ThreadID = thread.ID
	} else {
		newTurn.Title, newTurn.Model, newTurn.PromptVersion = ThreadTitle(message), t.req.Model, PromptVersion
	}
	th, tu, err := s.d.Store.StartTurn(ctx, newTurn)
	if errors.Is(err, library.ErrAskTurnStreaming) {
		return nil, recommend.ErrAskBusy
	}
	if err != nil {
		return nil, err
	}
	t.thread, t.turnID = *th, tu.ID
	t.req.Ref = tu.ID
	t.ctx, t.cancel = context.WithCancel(ctx)
	t.events = make(chan recommend.Event, 16)
	t.stop = make(chan struct{})

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		t.cancel()
		s.endUnstarted(t)
		return nil, recommend.ErrUnavailable
	}
	s.running[t] = struct{}{}
	s.wg.Add(1)
	s.mu.Unlock()
	started = true
	go t.run()
	return t.events, nil
}

// prepare builds the turn's request: the thread's history replayed
// unchanged and the new user message, on the thread's model and prompt
// (a new thread's from rec_model and PromptVersion).
func (s *Service) prepare(ctx context.Context, t *turn, thread *library.AskThread) error {
	model, version := "", PromptVersion
	var history []aicore.Message
	if thread != nil {
		model, version = thread.Model, thread.PromptVersion
		stored, err := s.d.Store.Messages(ctx, thread.ID)
		if err != nil {
			return err
		}
		if history, err = decodeMessages(stored); err != nil {
			return fmt.Errorf("ask thread %d: %w", thread.ID, err)
		}
		turns, err := s.d.Store.Turns(ctx, thread.ID)
		if err != nil {
			return err
		}
		if t.refine, t.chips, err = isRefinement(t.message, turns); err != nil {
			return err
		}
	} else {
		settings, err := s.d.Settings.AISettings(ctx)
		if err != nil {
			return err
		}
		model = recommend.ModelOpus
		if slices.Contains(recommend.RecModels, settings.RecModel) {
			model = settings.RecModel
		}
	}
	p := prompts[version]
	t.user = userMessage(t.started, t.message, t.refine)
	t.req = aicore.Request{
		Model:                  model,
		System:                 p.system,
		Tools:                  append(slices.Clone(s.d.Tools), picksTool{run: t.showPicks}),
		WebSearchMaxUses:       p.webSearches,
		WebFetchMaxUses:        p.webFetches,
		Messages:               append(history, t.user),
		MaxTokens:              maxTokens,
		DropMismatchedThinking: true,
		Caps:                   turnCaps,
		AllowFinal:             func() bool { return len(t.items) > 0 },
		Hooks: aicore.Hooks{
			BlockStart: t.blockStart,
			Text:       t.textDelta,
			ToolStart:  t.toolStart,
			ToolResult: t.tracker.tally.toolResult,
			Call:       t.callEnded,
		},
		Feature: library.AIFeatureAsk,
	}
	return nil
}

// isRefinement reports whether message is one of the chips the thread's
// newest done turn offered (a refinement re-ranks: rerank), and those
// chips (a refinement that offers none keeps them).
func isRefinement(message string, turns []library.AskTurn) (bool, []string, error) {
	for i := len(turns) - 1; i >= 0; i-- {
		if turns[i].Status != library.AskTurnDone {
			continue
		}
		v, err := decodeView(turns[i].View)
		if err != nil || v == nil {
			return false, nil, err
		}
		for _, r := range v.Refinements {
			if strings.EqualFold(strings.TrimSpace(r), message) {
				return true, v.Refinements, nil
			}
		}
		return false, nil, nil
	}
	return false, nil, nil
}

// decodeMessages turns stored messages back into the API's, each block's
// bytes exactly as stored.
func decodeMessages(stored []library.AskMessage) ([]aicore.Message, error) {
	out := make([]aicore.Message, len(stored))
	for i, m := range stored {
		out[i].Role = m.Role
		if err := json.Unmarshal(m.Content, &out[i].Content); err != nil {
			return nil, fmt.Errorf("stored message %d doesn't decode: %w", i, err)
		}
	}
	return out, nil
}

// encodeMessages is how messages are stored: each one's blocks as a JSON
// array of the raw blocks.
func encodeMessages(msgs []aicore.Message) []library.AskMessage {
	out := make([]library.AskMessage, len(msgs))
	for i, m := range msgs {
		var b strings.Builder
		b.WriteByte('[')
		for j, block := range m.Content {
			if j > 0 {
				b.WriteByte(',')
			}
			b.Write(block)
		}
		b.WriteByte(']')
		out[i] = library.AskMessage{Role: m.Role, Content: []byte(b.String())}
	}
	return out
}

// Close cancels every turn in flight (each ends cancelled, its usage
// recorded, its stream closed) and refuses new ones; it waits a little for
// them to write how they ended. The REST server's RegisterOnShutdown runs
// it, since http.Server.Shutdown doesn't cancel request contexts.
func (s *Service) Close() {
	s.mu.Lock()
	s.closed = true
	for t := range s.running {
		t.cancel()
	}
	s.mu.Unlock()
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(closeWait):
		s.log.Warn("Ask turns still ending at shutdown")
	}
}

func (s *Service) isClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

// acquire takes the one slot a streaming turn holds.
func (s *Service) acquire() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.busy {
		return false
	}
	s.busy = true
	return true
}

func (s *Service) release() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.busy = false
}

// stale remembers a turn whose end couldn't be written.
func (s *Service) stale(turnID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.unended[turnID] = true
}

// heal ends the turns whose end couldn't be written (their rows may still
// say streaming, and the one-streaming index would answer ask_busy until a
// restart). It runs holding the slot, so none of them is running.
func (s *Service) heal(ctx context.Context) {
	s.mu.Lock()
	var ids []int64
	for id := range s.unended {
		ids = append(ids, id)
	}
	s.mu.Unlock()
	for _, id := range ids {
		if err := s.d.Store.AbandonTurn(ctx, id); err != nil {
			s.log.Warn("Ask: still can't end a turn whose end failed", "turn", id, "error", err)
			continue
		}
		s.mu.Lock()
		delete(s.unended, id)
		s.mu.Unlock()
	}
}

func (s *Service) untrack(t *turn) {
	s.mu.Lock()
	delete(s.running, t)
	s.mu.Unlock()
	s.wg.Done()
}

// endUnstarted cancels a turn that was stored but never ran (shutdown
// raced it).
func (s *Service) endUnstarted(t *turn) {
	ctx, cancel := context.WithTimeout(context.Background(), s.writeTimeout)
	defer cancel()
	if err := s.d.Store.CancelTurn(ctx, t.turnID); err != nil {
		s.log.Warn("Ask: failed to cancel a turn at shutdown", "turn", t.turnID, "error", err)
		s.stale(t.turnID)
	}
}
