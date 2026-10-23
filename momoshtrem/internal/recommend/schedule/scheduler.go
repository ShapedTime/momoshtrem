// Package schedule runs the picks (PLAN §7 R2, #157): it queues runs on
// the schedule (rec_schedule, 03:00 server-local), on refresh and after
// Trakt's initial import, and executes them one at a time. Each run is one
// household member's (#245). It's also the picks' API service
// (api.Recommendations), the usage page's part (aisettings.RecsReporter)
// and the title pages' reasons (catalog.Reasons), reading through
// runs.Reader, each for the request's member (member.Require).
//
// Single-flight: the database holds at most one queued or running run per
// member (the partial unique index), so a refresh, a slot or the
// first-sync trigger during a member's run joins it; in memory one worker
// executes every member's runs, oldest queued first.
//
// The schedule: nightly or, weekly, Sunday at 03:00 server-local, or
// manual. Each slot queues a run for every member with Trakt connected, in
// an order that rotates by the slot (rotate, slotIndex), so a budget that
// runs out late in the month doesn't always skip the same member. A slot
// the server missed (it was down, and no run was queued since) is caught
// up about a minute after startup. Changing the settings reschedules at
// once (Reschedule, from the settings service). With rec_enabled off or
// without a key, nothing is queued.
package schedule

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/shapedtime/momoshtrem/internal/aicore"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/member"
	"github.com/shapedtime/momoshtrem/internal/recommend"
	"github.com/shapedtime/momoshtrem/internal/recommend/profile"
	"github.com/shapedtime/momoshtrem/internal/recommend/runs"
)

const (
	// CatchUpDelay is how long after startup a missed slot runs, so
	// startup isn't slowed.
	CatchUpDelay = time.Minute
	// maxSleep bounds a wait for the next slot, so a clock change or a
	// suspended host never pushes a slot far out.
	maxSleep = time.Hour
	// closeTimeout is how long Close waits for the run in flight to stop.
	closeTimeout = 15 * time.Second
	// forgetTimeout is how long ForgetMember waits for the removed
	// member's run to stop.
	forgetTimeout = 10 * time.Second
	// restartedMessage is a run's error when startup finds it active.
	restartedMessage = "momoshtrem restarted during the run."
	// staleMargin is how long past runs.Timeout a run another process (or a
	// lost final write) left running counts as stale, and staleMessage its
	// error then.
	staleMargin  = 10 * time.Minute
	staleMessage = "The run stopped without finishing."
)

// Clock is the scheduler's time; tests drive a fake one.
type Clock interface {
	Now() time.Time
	// After fires once d has passed.
	After(d time.Duration) <-chan time.Time
}

type realClock struct{}

func (realClock) Now() time.Time                         { return time.Now() }
func (realClock) After(d time.Duration) <-chan time.Time { return time.After(d) }

// Store queues runs and reads what the schedule needs
// (library.RecRunRepository).
type Store interface {
	QueueRecRun(ctx context.Context, memberID int64, kind, model string) (id int64, queued bool, err error)
	ActiveRecRuns(ctx context.Context) ([]library.RecRun, error)
	NextQueuedRecRun(ctx context.Context) (*library.RecRun, error)
	LatestSucceededRecRun(ctx context.Context, memberID int64) (*library.RecRun, error)
	LatestRecRunCreatedAt(ctx context.Context) (at time.Time, ok bool, err error)
	FailActiveRecRuns(ctx context.Context, message string) (int64, error)
	FailRecRun(ctx context.Context, id int64, message string, progress json.RawMessage) error
}

var _ Store = (*library.RecRunRepository)(nil)

// Executor executes a queued run to its end (runs.Runner).
type Executor interface {
	Execute(ctx context.Context, run library.RecRun) (runs.Outcome, error)
}

// History says whether a member's run can go ahead (their
// profile.Builder).
type History interface {
	CheckHistory(ctx context.Context) (profile.Readiness, error)
}

// Members are the household's members the schedule queues runs for.
type Members interface {
	// IDs are every member's, by id (member.Service).
	IDs(ctx context.Context) ([]int64, error)
	// Eligible reports whether the member's Trakt is connected: without
	// it a run of theirs is skipped (no_history), so no slot queues one.
	Eligible(memberID int64) bool
}

// Deps are what a Scheduler needs.
type Deps struct {
	Store    Store
	Runner   Executor
	Reader   *runs.Reader
	Settings runs.Settings
	Budget   aicore.Budget
	// History is a member's history check (refresh's no_history).
	History func(memberID int64) History
	Members Members
	// Configured says whether ANTHROPIC_API_KEY is set: without it nothing
	// is queued and refresh answers recommend.ErrUnavailable.
	Configured bool
	Clock      Clock // nil: real time
}

// Scheduler queues and executes the picks' runs.
type Scheduler struct {
	d   Deps
	log *slog.Logger

	wake chan struct{} // the settings changed: plan again
	work chan struct{} // a run was queued

	mu      sync.Mutex
	started bool
	catchUp time.Time  // a missed slot's runs, pending
	current *execution // the run the worker executes, nil when idle
	cancel  context.CancelFunc
	done    chan struct{}
}

// execution is the run the worker executes: ForgetMember cancels it.
type execution struct {
	run, member int64
	cancel      context.CancelFunc
	done        chan struct{} // closed once Execute returns
}

// New creates a Scheduler; Start runs it.
func New(d Deps) *Scheduler {
	if d.Clock == nil {
		d.Clock = realClock{}
	}
	return &Scheduler{
		d: d, log: slog.With("component", "recs-schedule"),
		wake: make(chan struct{}, 1), work: make(chan struct{}, 1),
	}
}

// Start marks the runs a crash left active failed, then, with a key, runs
// the schedule and the worker.
func (s *Scheduler) Start(ctx context.Context) error {
	if n, err := s.d.Store.FailActiveRecRuns(ctx, restartedMessage); err != nil {
		return err
	} else if n > 0 {
		s.log.Warn("Marked picks runs left active by a restart failed", "runs", n)
	}
	if !s.d.Configured {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started {
		return nil
	}
	s.started = true
	runCtx, cancel := context.WithCancel(context.Background())
	s.cancel, s.done = cancel, make(chan struct{})
	if s.missedSlot(ctx) {
		s.catchUp = s.d.Clock.Now().Add(CatchUpDelay)
		s.log.Info("A scheduled picks run was missed; catching up shortly", "at", s.catchUp)
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); s.loop(runCtx) }()
	go func() { defer wg.Done(); s.worker(runCtx) }()
	go func() { wg.Wait(); close(s.done) }()
	return nil
}

// Close stops the schedule and the run in flight (it ends failed: "momoshtrem
// stopped"), waiting for it a while.
func (s *Scheduler) Close() {
	s.mu.Lock()
	cancel, done := s.cancel, s.done
	s.mu.Unlock()
	if cancel == nil {
		return
	}
	cancel()
	select {
	case <-done:
	case <-time.After(closeTimeout):
		s.log.Warn("The picks run didn't stop in time")
	}
}

// Reschedule plans the schedule again: the settings changed.
func (s *Scheduler) Reschedule() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// ForgetMember stops the member's run in flight and waits a while for it
// (member.Service.OnRemoving): the member is being removed, and their
// runs, queued ones too, go with them.
func (s *Scheduler) ForgetMember(_ context.Context, memberID int64) {
	s.mu.Lock()
	x := s.current
	s.mu.Unlock()
	if x == nil || x.member != memberID {
		return
	}
	s.log.Info("Stopping the picks run of a member being removed", "run", x.run, "member", memberID)
	x.cancel()
	select {
	case <-x.done:
	case <-time.After(forgetTimeout):
		s.log.Warn("The removed member's picks run didn't stop in time", "run", x.run, "member", memberID)
	}
}

// kick wakes the worker: a run was queued.
func (s *Scheduler) kick() {
	select {
	case s.work <- struct{}{}:
	default:
	}
}

// Refresh queues a manual run for the context's member, or answers their
// active one (PLAN §6, in this order): ErrUnavailable without a key,
// ErrDisabled while rec_enabled is off, their active run joined,
// *LimitError once the budget is spent, ErrNoHistory without their Trakt
// or enough loved titles.
func (s *Scheduler) Refresh(ctx context.Context) (int64, error) {
	if !s.d.Configured {
		return 0, recommend.ErrUnavailable
	}
	memberID, err := member.Require(ctx)
	if err != nil {
		return 0, err
	}
	settings, err := s.d.Settings.AISettings(ctx)
	if err != nil {
		return 0, err
	}
	if !settings.RecEnabled {
		return 0, recommend.ErrDisabled
	}
	active, err := s.activeFor(ctx, memberID)
	if err != nil {
		return 0, err
	}
	if active != nil {
		return active.ID, nil
	}
	if err := s.d.Budget.Check(ctx); err != nil {
		var budget *aicore.BudgetError
		if errors.As(err, &budget) {
			return 0, &recommend.LimitError{Code: recommend.CodeBudgetExceeded, ResetsAt: budget.ResetsAt}
		}
		return 0, err
	}
	ready, err := s.d.History(memberID).CheckHistory(ctx)
	if err != nil {
		return 0, err
	}
	if !ready.Enough() {
		return 0, recommend.ErrNoHistory
	}
	id, queued, err := s.d.Store.QueueRecRun(ctx, memberID, recommend.KindManual, settings.RecModel)
	if err != nil {
		return 0, err
	}
	if queued {
		s.log.Info("Picks run queued", "run", id, "member", memberID, "kind", recommend.KindManual, "model", settings.RecModel)
	}
	s.kick()
	return id, nil
}

// FirstSync queues the member's first_sync run when their Trakt initial
// import finishes and they have no picks yet. The member's Trakt sync
// calls it (trakt.Registry.SetOnImported) before it reports the import
// idle, so For You finds the run waiting. Without a key, with rec_enabled
// off, or once a run of theirs has succeeded, it does nothing; their run
// already active is joined.
func (s *Scheduler) FirstSync(ctx context.Context, memberID int64) {
	if !s.d.Configured {
		return
	}
	settings, err := s.d.Settings.AISettings(ctx)
	if err != nil {
		s.log.Warn("First picks run not queued: couldn't read the AI settings", "member", memberID, "error", err)
		return
	}
	if !settings.RecEnabled {
		return
	}
	if run, err := s.d.Store.LatestSucceededRecRun(ctx, memberID); err != nil || run != nil {
		if err != nil {
			s.log.Warn("First picks run not queued: couldn't read the runs", "member", memberID, "error", err)
		}
		return
	}
	if _, err := s.reap(ctx); err != nil {
		s.log.Warn("Couldn't check the active picks runs", "error", err)
	}
	id, queued, err := s.d.Store.QueueRecRun(ctx, memberID, recommend.KindFirstSync, settings.RecModel)
	if err != nil {
		s.log.Warn("Failed to queue the first picks run", "member", memberID, "error", err)
		return
	}
	if queued {
		s.log.Info("Picks run queued", "run", id, "member", memberID, "kind", recommend.KindFirstSync, "model", settings.RecModel)
	}
	s.kick()
}

// NextRunAt is the schedule's next run for the context's member: a
// pending catch-up, else the next slot. Nil when manual, with rec_enabled
// off, without a key, or while the member's Trakt isn't connected (no
// slot queues a run of theirs).
func (s *Scheduler) NextRunAt(ctx context.Context) (*time.Time, error) {
	if !s.d.Configured {
		return nil, nil
	}
	memberID, err := member.Require(ctx)
	if err != nil {
		return nil, err
	}
	if !s.d.Members.Eligible(memberID) {
		return nil, nil
	}
	settings, err := s.d.Settings.AISettings(ctx)
	if err != nil {
		return nil, err
	}
	if !settings.RecEnabled {
		return nil, nil
	}
	next, ok := NextSlot(settings.RecSchedule, s.d.Clock.Now())
	if !ok {
		return nil, nil // manual: a pending catch-up won't run either
	}
	s.mu.Lock()
	catchUp := s.catchUp
	s.mu.Unlock()
	if !catchUp.IsZero() {
		return &catchUp, nil
	}
	return &next, nil
}

// missedSlot reports whether the schedule's last slot went by with no run
// queued since, anyone's, the server down: a catch-up is due. Never before
// the first run (a fresh install has missed nothing).
func (s *Scheduler) missedSlot(ctx context.Context) bool {
	settings, err := s.d.Settings.AISettings(ctx)
	if err != nil || !settings.RecEnabled {
		return false
	}
	last, ok := LastSlot(settings.RecSchedule, s.d.Clock.Now())
	if !ok {
		return false
	}
	created, any, err := s.d.Store.LatestRecRunCreatedAt(ctx)
	if err != nil {
		s.log.Warn("Couldn't check for a missed picks run", "error", err)
		return false
	}
	return any && created.Before(last)
}

// loop queues the scheduled runs at each slot, and the catch-up.
func (s *Scheduler) loop(ctx context.Context) {
	var (
		fired time.Time // the last slot queued
		known *library.AISettings
	)
	for {
		now := s.d.Clock.Now()
		wait := maxSleep
		settings, err := s.d.Settings.AISettings(ctx)
		switch {
		case err == nil:
			known = &settings
		case ctx.Err() != nil:
			return
		default:
			// Plan on what was read last, and read again soon.
			s.log.Warn("Couldn't read the AI settings; trying again in a minute", "error", err)
			wait = time.Minute
			if known != nil {
				settings = *known
			}
		}
		next, scheduled := NextSlot(settings.RecSchedule, now)
		scheduled = scheduled && settings.RecEnabled
		s.mu.Lock()
		if err == nil && !scheduled && !s.catchUp.IsZero() {
			// Manual now, or off: the missed slot isn't run.
			s.catchUp = time.Time{}
			s.log.Info("Missed picks run dropped: the schedule is manual or picks are off")
		}
		catchUp := s.catchUp
		s.mu.Unlock()
		if _, err := s.reap(ctx); err != nil && ctx.Err() == nil {
			s.log.Warn("Couldn't check the active picks runs", "error", err)
		}

		if scheduled {
			wait = min(wait, next.Sub(now))
		}
		if !catchUp.IsZero() {
			wait = min(wait, max(catchUp.Sub(now), 0))
		}
		select {
		case <-ctx.Done():
			return
		case <-s.wake:
			continue
		case <-s.d.Clock.After(wait):
		}

		now = s.d.Clock.Now()
		if !catchUp.IsZero() && !now.Before(catchUp) {
			s.mu.Lock()
			s.catchUp = time.Time{}
			s.mu.Unlock()
			last, _ := LastSlot(settings.RecSchedule, now)
			s.queueScheduled(ctx, last)
			continue
		}
		if scheduled && !now.Before(next) && next.After(fired) {
			fired = next
			s.queueScheduled(ctx, next)
		}
	}
}

// queueScheduled queues the slot's scheduled runs: one for every member
// with Trakt connected, in the slot's rotated order (the worker runs them
// oldest first), unless picks are off or the schedule is manual. A
// member's run already active is joined.
func (s *Scheduler) queueScheduled(ctx context.Context, slot time.Time) {
	settings, err := s.d.Settings.AISettings(ctx)
	if err != nil {
		s.log.Warn("Scheduled picks runs not queued: couldn't read the AI settings", "error", err)
		return
	}
	if _, ok := NextSlot(settings.RecSchedule, s.d.Clock.Now()); !ok || !settings.RecEnabled {
		return
	}
	ids, err := s.d.Members.IDs(ctx)
	if err != nil {
		if ctx.Err() == nil {
			s.log.Error("Scheduled picks runs not queued: couldn't read the household's members", "error", err)
		}
		return
	}
	var eligible []int64
	for _, id := range ids {
		if s.d.Members.Eligible(id) {
			eligible = append(eligible, id)
		}
	}
	if _, err := s.reap(ctx); err != nil {
		s.log.Warn("Couldn't check the active picks runs", "error", err)
	}
	for _, id := range rotate(eligible, slotIndex(settings.RecSchedule, slot)) {
		runID, queued, err := s.d.Store.QueueRecRun(ctx, id, recommend.KindScheduled, settings.RecModel)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			s.log.Error("Failed to queue a scheduled picks run", "member", id, "error", err)
			continue
		}
		if queued {
			s.log.Info("Picks run queued", "run", runID, "member", id, "kind", recommend.KindScheduled, "model", settings.RecModel)
		}
	}
	s.kick()
}

// worker executes queued runs one at a time, every member's, oldest
// first. A run another process is running (recommend-run) is left to it.
func (s *Scheduler) worker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.work:
		}
		for ctx.Err() == nil {
			run, err := s.d.Store.NextQueuedRecRun(ctx)
			if err != nil {
				if ctx.Err() == nil {
					s.log.Error("Couldn't read the queued picks runs", "error", err)
				}
				break
			}
			if run == nil {
				break
			}
			if err := s.execute(ctx, *run); err != nil {
				if errors.Is(err, library.ErrRecRunNotActive) {
					// Gone with its member (removed during the run), or
					// finished by another process: on to the next.
					continue
				}
				// Left active: reap fails it once it's stale.
				s.log.Error("Failed to record the picks run", "run", run.ID, "member", run.MemberID, "error", err)
				break
			}
		}
	}
}

// execute runs one run to its end, as the current execution:
// ForgetMember can cancel it.
func (s *Scheduler) execute(ctx context.Context, run library.RecRun) error {
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	x := &execution{run: run.ID, member: run.MemberID, cancel: cancel, done: make(chan struct{})}
	s.mu.Lock()
	s.current = x
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.current = nil
		s.mu.Unlock()
		close(x.done)
	}()
	_, err := s.d.Runner.Execute(runCtx, run)
	return err
}

// reap answers every member's active run, after failing the stale ones:
// running for longer than a run may (runs.Timeout and a margin) while this
// process isn't executing it, so its process died or lost its final
// write. Another process's live run (recommend-run) is younger and kept.
// A queued run this process isn't executing wakes the worker.
func (s *Scheduler) reap(ctx context.Context) ([]library.RecRun, error) {
	active, err := s.d.Store.ActiveRecRuns(ctx)
	if err != nil {
		return nil, err
	}
	var mine int64
	s.mu.Lock()
	if s.current != nil {
		mine = s.current.run
	}
	s.mu.Unlock()
	kept, queued := active[:0], false
	for _, run := range active {
		if run.ID != mine && run.Status == recommend.StatusQueued {
			queued = true
		}
		if run.ID == mine || run.Status == recommend.StatusQueued {
			kept = append(kept, run)
			continue
		}
		since := run.CreatedAt
		if run.StartedAt != nil {
			since = *run.StartedAt
		}
		if s.d.Clock.Now().Sub(since) <= runs.Timeout+staleMargin {
			kept = append(kept, run)
			continue
		}
		if err := s.d.Store.FailRecRun(ctx, run.ID, staleMessage, nil); err != nil && !errors.Is(err, library.ErrRecRunNotActive) {
			return nil, err
		}
		s.log.Warn("Marked a stale picks run failed", "run", run.ID, "member", run.MemberID, "started", since)
	}
	if queued {
		s.kick()
	}
	return kept, nil
}

// activeFor is the member's queued or running run, after reap; nil when
// there's none.
func (s *Scheduler) activeFor(ctx context.Context, memberID int64) (*library.RecRun, error) {
	active, err := s.reap(ctx)
	if err != nil {
		return nil, err
	}
	for i := range active {
		if active[i].MemberID == memberID {
			return &active[i], nil
		}
	}
	return nil, nil
}

// ---- The API and the usage page, for the context's member ----

// Home is GET /recommendations/home.
func (s *Scheduler) Home(ctx context.Context) (*recommend.Home, error) {
	memberID, err := member.Require(ctx)
	if err != nil {
		return nil, err
	}
	return s.d.Reader.Home(ctx, memberID)
}

// Runs lists the member's newest runs first.
func (s *Scheduler) Runs(ctx context.Context, limit int) ([]recommend.RunSummary, error) {
	memberID, err := member.Require(ctx)
	if err != nil {
		return nil, err
	}
	return s.d.Reader.Runs(ctx, memberID, limit)
}

// Run is one of the member's runs with its progress and usage;
// recommend.ErrNotFound for another member's.
func (s *Scheduler) Run(ctx context.Context, id int64) (*recommend.RunDetail, error) {
	memberID, err := member.Require(ctx)
	if err != nil {
		return nil, err
	}
	return s.d.Reader.Run(ctx, memberID, id)
}

// RecommendationReason is a title page's recommendation_reason for the
// member (catalog.Reasons).
func (s *Scheduler) RecommendationReason(ctx context.Context, memberID int64, mediaType string, tmdbID int) (*string, error) {
	return s.d.Reader.RecommendationReason(ctx, memberID, mediaType, tmdbID)
}

// RecsUsage is the usage page's picks part: the member's newest finished
// run, their active one, and the next scheduled run.
func (s *Scheduler) RecsUsage(ctx context.Context) (recommend.RecsUsage, error) {
	var out recommend.RecsUsage
	memberID, err := member.Require(ctx)
	if err != nil {
		return out, err
	}
	if out.LastRun, err = s.d.Reader.LastFinished(ctx, memberID); err != nil {
		return out, err
	}
	if out.ActiveRun, err = s.d.Reader.Active(ctx, memberID); err != nil {
		return out, err
	}
	out.NextRunAt, err = s.NextRunAt(ctx)
	return out, err
}

// RunEstimates is, per rec model, the mean cost of its last 5 succeeded
// runs, anyone's.
func (s *Scheduler) RunEstimates(ctx context.Context) (map[string]*float64, error) {
	return s.d.Reader.RunEstimates(ctx)
}
