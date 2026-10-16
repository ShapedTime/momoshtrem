package schedule

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/shapedtime/momoshtrem/internal/aicore"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/recommend"
	"github.com/shapedtime/momoshtrem/internal/recommend/profile"
	"github.com/shapedtime/momoshtrem/internal/recommend/runs"
)

// fakeClock is a clock tests move by hand.
type fakeClock struct {
	mu      sync.Mutex
	now     time.Time
	calls   int // After calls: the loop waiting
	waiters []waiter
}

type waiter struct {
	at time.Time
	ch chan time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) After(d time.Duration) <-chan time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls++
	ch := make(chan time.Time, 1)
	if d <= 0 {
		ch <- c.now
		return ch
	}
	c.waiters = append(c.waiters, waiter{at: c.now.Add(d), ch: ch})
	return ch
}

// Advance moves the clock, firing what's due.
func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
	kept := c.waiters[:0]
	for _, w := range c.waiters {
		if !w.at.After(c.now) {
			w.ch <- c.now
		} else {
			kept = append(kept, w)
		}
	}
	c.waiters = kept
}

func (c *fakeClock) afterCalls() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}

// fakeStore keeps runs in memory, one active at a time.
type fakeStore struct {
	mu   sync.Mutex
	runs []library.RecRun
	at   func() time.Time
}

func (s *fakeStore) QueueRecRun(_ context.Context, kind, model string) (int64, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range s.runs {
		if r.Status == recommend.StatusQueued || r.Status == recommend.StatusRunning {
			return r.ID, false, nil
		}
	}
	id := int64(len(s.runs) + 1)
	s.runs = append(s.runs, library.RecRun{ID: id, Kind: kind, Model: model, Status: recommend.StatusQueued, CreatedAt: s.at()})
	return id, true, nil
}

func (s *fakeStore) find(pred func(library.RecRun) bool) *library.RecRun {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := len(s.runs) - 1; i >= 0; i-- {
		if pred(s.runs[i]) {
			r := s.runs[i]
			return &r
		}
	}
	return nil
}

func (s *fakeStore) ActiveRecRun(context.Context) (*library.RecRun, error) {
	return s.find(func(r library.RecRun) bool {
		return r.Status == recommend.StatusQueued || r.Status == recommend.StatusRunning
	}), nil
}

func (s *fakeStore) LatestSucceededRecRun(context.Context) (*library.RecRun, error) {
	return s.find(func(r library.RecRun) bool { return r.Status == recommend.StatusSucceeded }), nil
}

func (s *fakeStore) LatestFinishedRecRun(context.Context) (*library.RecRun, error) {
	return s.find(func(r library.RecRun) bool {
		return r.Status != recommend.StatusQueued && r.Status != recommend.StatusRunning
	}), nil
}

func (s *fakeStore) LatestRecRunCreatedAt(context.Context) (time.Time, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.runs) == 0 {
		return time.Time{}, false, nil
	}
	return s.runs[len(s.runs)-1].CreatedAt, true, nil
}

func (s *fakeStore) FailActiveRecRuns(_ context.Context, msg string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := int64(0)
	for i := range s.runs {
		if s.runs[i].Status == recommend.StatusQueued || s.runs[i].Status == recommend.StatusRunning {
			s.runs[i].Status, s.runs[i].Error = recommend.StatusFailed, &msg
			n++
		}
	}
	return n, nil
}

func (s *fakeStore) set(id int64, status string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.runs[id-1].Status = status
}

func (s *fakeStore) all() []library.RecRun {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]library.RecRun(nil), s.runs...)
}

// The runs.ReadStore the usage page reads.
func (s *fakeStore) GetRecRun(context.Context, int64) (*library.RecRun, error)          { return nil, nil }
func (s *fakeStore) ListRecRuns(context.Context, int) ([]library.RecRun, error)         { return nil, nil }
func (s *fakeStore) RecRunShelves(context.Context, int64) ([]library.RecShelfRow, error) { return nil, nil }
func (s *fakeStore) RecRunHeroPicks(context.Context, int64) ([]library.RecHeroPickRow, error) {
	return nil, nil
}
func (s *fakeStore) RecommendationReason(context.Context, library.TitleRef) (*string, error) {
	return nil, nil
}
func (s *fakeStore) RecRunCostMeans(context.Context, int) (map[string]float64, error) { return nil, nil }

// fakeExecutor runs a run by marking it succeeded; hold blocks it until
// released.
type fakeExecutor struct {
	store *fakeStore
	hold  chan struct{}
	mu    sync.Mutex
	ran   []library.RecRun
}

func (e *fakeExecutor) Execute(ctx context.Context, run library.RecRun) (runs.Outcome, error) {
	e.store.set(run.ID, recommend.StatusRunning)
	e.mu.Lock()
	e.ran = append(e.ran, run)
	e.mu.Unlock()
	if e.hold != nil {
		select {
		case <-e.hold:
		case <-ctx.Done():
			e.store.set(run.ID, recommend.StatusFailed)
			return runs.Outcome{Status: recommend.StatusFailed}, nil
		}
	}
	e.store.set(run.ID, recommend.StatusSucceeded)
	return runs.Outcome{Status: recommend.StatusSucceeded}, nil
}

func (e *fakeExecutor) count() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.ran)
}

type settingsBox struct {
	mu sync.Mutex
	s  library.AISettings
}

func (b *settingsBox) AISettings(context.Context) (library.AISettings, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.s, nil
}

func (b *settingsBox) update(fn func(*library.AISettings)) {
	b.mu.Lock()
	defer b.mu.Unlock()
	fn(&b.s)
}

type budget struct{ err error }

func (b budget) Check(context.Context) error { return b.err }

type history struct{ ready profile.Readiness }

func (h history) CheckHistory(context.Context) (profile.Readiness, error) { return h.ready, nil }

type env struct {
	clock    *fakeClock
	store    *fakeStore
	exec     *fakeExecutor
	settings *settingsBox
	s        *Scheduler
}

// friday0200 is Friday 16 October 2026, 02:00, server-local.
var friday0200 = time.Date(2026, 10, 16, 2, 0, 0, 0, time.UTC)

func newEnv(t *testing.T, start time.Time, configured bool, prior ...library.RecRun) *env {
	t.Helper()
	e := &env{clock: &fakeClock{now: start}, settings: &settingsBox{s: library.DefaultAISettings()}}
	e.store = &fakeStore{runs: prior, at: e.clock.Now}
	e.exec = &fakeExecutor{store: e.store}
	e.s = New(Deps{
		Store: e.store, Runner: e.exec, Settings: e.settings, Budget: budget{},
		History:    history{ready: profile.Readiness{Connected: true, Counts: profile.Counts{Loved: 9}}},
		Reader:     runs.NewReader(runs.ReaderDeps{Store: e.store, Settings: e.settings, Configured: configured, Now: e.clock.Now}),
		Configured: configured, Clock: e.clock,
	})
	t.Cleanup(e.s.Close)
	return e
}

func (e *env) start(t *testing.T) {
	t.Helper()
	if err := e.s.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
}

// waitUntil polls cond for a while.
func waitUntil(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(time.Millisecond)
	}
}

// waitWaiting waits for the loop's n-th wait.
func (e *env) waitWaiting(t *testing.T, n int) {
	t.Helper()
	waitUntil(t, "the schedule to wait", func() bool { return e.clock.afterCalls() >= n })
}


func TestSlots(t *testing.T) {
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Skip("no tzdata")
	}
	at := func(y int, m time.Month, d, h, min int) time.Time { return time.Date(y, m, d, h, min, 0, 0, berlin) }
	for _, tt := range []struct {
		schedule   string
		now        time.Time
		next, last time.Time
	}{
		{"nightly", at(2026, 10, 16, 2, 59), at(2026, 10, 16, 3, 0), at(2026, 10, 15, 3, 0)},
		{"nightly", at(2026, 10, 16, 3, 0), at(2026, 10, 17, 3, 0), at(2026, 10, 16, 3, 0)},
		{"nightly", at(2026, 10, 31, 23, 0), at(2026, 11, 1, 3, 0), at(2026, 10, 31, 3, 0)},
		// Clocks go back at 03:00 on 25 October: still one 03:00 slot.
		{"nightly", at(2026, 10, 24, 12, 0), at(2026, 10, 25, 3, 0), at(2026, 10, 24, 3, 0)},
		// Friday → Sunday.
		{"weekly", at(2026, 10, 16, 12, 0), at(2026, 10, 18, 3, 0), at(2026, 10, 11, 3, 0)},
		{"weekly", at(2026, 10, 18, 2, 0), at(2026, 10, 18, 3, 0), at(2026, 10, 11, 3, 0)},
		{"weekly", at(2026, 10, 18, 3, 0), at(2026, 10, 25, 3, 0), at(2026, 10, 18, 3, 0)},
	} {
		next, ok := NextSlot(tt.schedule, tt.now)
		if !ok || !next.Equal(tt.next) {
			t.Errorf("NextSlot(%s, %v) = %v, want %v", tt.schedule, tt.now, next, tt.next)
		}
		last, ok := LastSlot(tt.schedule, tt.now)
		if !ok || !last.Equal(tt.last) {
			t.Errorf("LastSlot(%s, %v) = %v, want %v", tt.schedule, tt.now, last, tt.last)
		}
		if next.Hour() != SlotHour || last.Hour() != SlotHour {
			t.Errorf("%s at %v: slots at %v and %v", tt.schedule, tt.now, next, last)
		}
	}
	if _, ok := NextSlot("manual", friday0200); ok {
		t.Error("manual has a next slot")
	}
	if _, ok := LastSlot("manual", friday0200); ok {
		t.Error("manual has a last slot")
	}
}

// next_run_at follows the settings at once, and is null when manual, off
// or not configured.
func TestNextRunAt(t *testing.T) {
	e := newEnv(t, friday0200, true)
	ctx := context.Background()
	if next, _ := e.s.NextRunAt(ctx); next == nil || !next.Equal(friday0200.Add(time.Hour)) {
		t.Errorf("nightly = %v", next)
	}
	e.settings.update(func(s *library.AISettings) { s.RecSchedule = recommend.ScheduleWeekly })
	if next, _ := e.s.NextRunAt(ctx); next == nil || !next.Equal(time.Date(2026, 10, 18, 3, 0, 0, 0, time.UTC)) {
		t.Errorf("weekly = %v", next)
	}
	e.settings.update(func(s *library.AISettings) { s.RecSchedule = recommend.ScheduleManual })
	if next, _ := e.s.NextRunAt(ctx); next != nil {
		t.Errorf("manual = %v", next)
	}
	e.settings.update(func(s *library.AISettings) { s.RecSchedule, s.RecEnabled = recommend.ScheduleNightly, false })
	if next, _ := e.s.NextRunAt(ctx); next != nil {
		t.Errorf("off = %v", next)
	}
	if next, _ := newEnv(t, friday0200, false).s.NextRunAt(ctx); next != nil {
		t.Errorf("not configured = %v", next)
	}

	usage, err := e.s.RecsUsage(ctx)
	if err != nil || usage.NextRunAt != nil || usage.LastRun != nil || usage.ActiveRun != nil {
		t.Errorf("usage = %+v, %v", usage, err)
	}
}

// At 03:00 a scheduled run is queued and executed; then the next night's.
func TestSchedule_Slot(t *testing.T) {
	prior := library.RecRun{ID: 1, Kind: "scheduled", Status: "succeeded", Model: "claude-opus-5-5",
		CreatedAt: friday0200.Add(-23 * time.Hour)}
	e := newEnv(t, friday0200, true, prior)
	e.start(t)
	e.waitWaiting(t, 1)
	if e.exec.count() != 0 {
		t.Fatal("a run before the slot")
	}
	e.clock.Advance(59 * time.Minute)
	if e.exec.count() != 0 {
		t.Fatal("a run before the slot")
	}
	e.clock.Advance(time.Minute)
	waitUntil(t, "the 03:00 run", func() bool { return e.exec.count() == 1 })
	if run := e.store.all()[1]; run.Kind != recommend.KindScheduled || run.Model != "claude-opus-5-5" || run.Status != "succeeded" {
		t.Errorf("run = %+v", run)
	}
	usage, _ := e.s.RecsUsage(context.Background())
	if usage.NextRunAt == nil || !usage.NextRunAt.Equal(time.Date(2026, 10, 17, 3, 0, 0, 0, time.UTC)) || usage.LastRun.ID != 2 {
		t.Errorf("usage after = %+v", usage)
	}
	e.waitWaiting(t, 2)
	e.clock.Advance(24 * time.Hour)
	waitUntil(t, "the next night's run", func() bool { return e.exec.count() == 2 })
}

// A slot missed while the server was down runs a minute after startup,
// once; none when a run was queued since, or on a fresh install.
func TestSchedule_CatchUp(t *testing.T) {
	friday1000 := friday0200.Add(8 * time.Hour)
	before := library.RecRun{ID: 1, Kind: "scheduled", Status: "succeeded", CreatedAt: friday0200.Add(-23 * time.Hour)}

	e := newEnv(t, friday1000, true, before)
	e.start(t)
	if next, _ := e.s.NextRunAt(context.Background()); next == nil || !next.Equal(friday1000.Add(CatchUpDelay)) {
		t.Errorf("next run = %v, want the catch-up a minute after startup", next)
	}
	e.waitWaiting(t, 1)
	e.clock.Advance(CatchUpDelay)
	waitUntil(t, "the catch-up run", func() bool { return e.exec.count() == 1 })
	if run := e.store.all()[1]; run.Kind != recommend.KindScheduled {
		t.Errorf("catch-up = %+v", run)
	}
	waitUntil(t, "the catch-up to clear", func() bool {
		next, _ := e.s.NextRunAt(context.Background())
		return next != nil && next.Equal(time.Date(2026, 10, 17, 3, 0, 0, 0, time.UTC))
	})

	since := library.RecRun{ID: 1, Kind: "manual", Status: "succeeded", CreatedAt: friday0200.Add(2 * time.Hour)}
	for name, env := range map[string]*env{
		"a run since":   newEnv(t, friday1000, true, since),
		"fresh install": newEnv(t, friday1000, true),
	} {
		env.start(t)
		if next, _ := env.s.NextRunAt(context.Background()); next == nil || !next.Equal(time.Date(2026, 10, 17, 3, 0, 0, 0, time.UTC)) {
			t.Errorf("%s: next run = %v, want tomorrow's slot", name, next)
		}
		env.waitWaiting(t, 1)
		env.clock.Advance(CatchUpDelay)
		time.Sleep(20 * time.Millisecond)
		if env.exec.count() != 0 {
			t.Errorf("%s: caught up", name)
		}
	}

	off := newEnv(t, friday1000, true, before)
	off.settings.update(func(s *library.AISettings) { s.RecEnabled = false })
	off.start(t)
	if off.s.catchUp != (time.Time{}) {
		t.Error("a catch-up with picks off")
	}
}

// Changing the settings reschedules at once: manual skips tonight's slot,
// and picks off queue nothing.
func TestSchedule_Reschedule(t *testing.T) {
	prior := library.RecRun{ID: 1, Status: "succeeded", CreatedAt: friday0200.Add(-23 * time.Hour)}
	e := newEnv(t, friday0200, true, prior)
	e.start(t)
	e.waitWaiting(t, 1)
	e.settings.update(func(s *library.AISettings) { s.RecSchedule = recommend.ScheduleManual })
	e.s.Reschedule()
	e.waitWaiting(t, 2)
	e.clock.Advance(2 * time.Hour) // 04:00: the manual plan's hour is up
	e.waitWaiting(t, 3)
	if e.exec.count() != 0 {
		t.Fatal("a manual schedule ran at 03:00")
	}

	// Back to nightly: the next slot is tomorrow's.
	e.settings.update(func(s *library.AISettings) { s.RecSchedule = recommend.ScheduleNightly })
	e.s.Reschedule()
	e.waitWaiting(t, 4)
	if next, _ := e.s.NextRunAt(context.Background()); !next.Equal(time.Date(2026, 10, 17, 3, 0, 0, 0, time.UTC)) {
		t.Errorf("next run = %v", next)
	}
	e.clock.Advance(23 * time.Hour)
	waitUntil(t, "tomorrow's run", func() bool { return e.exec.count() == 1 })
	e.waitWaiting(t, 5)

	// Picks off at the slot: nothing queued.
	e.settings.update(func(s *library.AISettings) { s.RecEnabled = false })
	e.s.Reschedule()
	e.waitWaiting(t, 6)
	e.clock.Advance(24 * time.Hour)
	e.waitWaiting(t, 7)
	if e.exec.count() != 1 || len(e.store.all()) != 2 {
		t.Errorf("runs with picks off = %d", len(e.store.all()))
	}
}

// Refresh's checks, in the contract's order, then a manual run; a second
// refresh, the first-sync trigger and a slot all join the active run, and
// one worker executes it.
func TestRefresh_SingleFlight(t *testing.T) {
	ctx := context.Background()
	if _, err := newEnv(t, friday0200, false).s.Refresh(ctx); !errors.Is(err, recommend.ErrUnavailable) {
		t.Errorf("no key = %v", err)
	}

	e := newEnv(t, friday0200.Add(50*time.Minute), true)
	e.exec.hold = make(chan struct{})
	e.start(t)

	e.settings.update(func(s *library.AISettings) { s.RecEnabled = false })
	if _, err := e.s.Refresh(ctx); !errors.Is(err, recommend.ErrDisabled) {
		t.Errorf("off = %v", err)
	}
	e.settings.update(func(s *library.AISettings) { s.RecEnabled = true })

	resets := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	e.s.d.Budget = budget{err: &aicore.BudgetError{SpentUSD: 15, BudgetUSD: 15, ResetsAt: resets}}
	var limit *recommend.LimitError
	if _, err := e.s.Refresh(ctx); !errors.As(err, &limit) || limit.Code != recommend.CodeBudgetExceeded || !limit.ResetsAt.Equal(resets) {
		t.Errorf("budget = %v", err)
	}
	e.s.d.Budget = budget{}
	e.s.d.History = history{ready: profile.Readiness{Connected: true, Counts: profile.Counts{Loved: 4}}}
	if _, err := e.s.Refresh(ctx); !errors.Is(err, recommend.ErrNoHistory) {
		t.Errorf("history = %v", err)
	}
	e.s.d.History = history{ready: profile.Readiness{Connected: true, Counts: profile.Counts{Loved: 5}}}

	id, err := e.s.Refresh(ctx)
	if err != nil || id != 1 {
		t.Fatalf("refresh = %d, %v", id, err)
	}
	waitUntil(t, "the run to start", func() bool { return e.exec.count() == 1 })

	// Joined while it runs, whatever the budget says (the active run comes
	// before the budget check).
	e.s.d.Budget = budget{err: &aicore.BudgetError{ResetsAt: resets}}
	if again, err := e.s.Refresh(ctx); err != nil || again != id {
		t.Errorf("a second refresh = %d, %v; want it to join %d", again, err, id)
	}
	e.s.d.Budget = budget{}
	e.s.FirstSync(ctx)
	e.waitWaiting(t, 1)
	e.clock.Advance(10 * time.Minute) // past 03:00
	time.Sleep(20 * time.Millisecond)
	if all := e.store.all(); len(all) != 1 {
		t.Errorf("runs = %+v, want the one", all)
	}
	home, _ := e.s.RecsUsage(ctx)
	if home.ActiveRun == nil || home.ActiveRun.ID != id {
		t.Errorf("active = %+v", home.ActiveRun)
	}
	close(e.exec.hold)
	waitUntil(t, "the run to finish", func() bool { return e.store.all()[0].Status == recommend.StatusSucceeded })
	if e.exec.count() != 1 {
		t.Errorf("executed %d times", e.exec.count())
	}
}

// Trakt's initial import queues the first_sync run when nothing has
// succeeded yet; not with picks off, without a key, or once a run has
// succeeded.
func TestFirstSync(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t, friday0200, true)
	e.start(t)
	e.s.FirstSync(ctx)
	if all := e.store.all(); len(all) != 1 || all[0].Kind != recommend.KindFirstSync || all[0].Model != "claude-opus-5-5" {
		t.Fatalf("runs = %+v", all)
	}
	waitUntil(t, "the first run", func() bool { return e.exec.count() == 1 })
	waitUntil(t, "it to succeed", func() bool { return e.store.all()[0].Status == recommend.StatusSucceeded })
	e.s.FirstSync(ctx) // a reconnect's import: picks exist
	if n := len(e.store.all()); n != 1 {
		t.Errorf("runs after a second import = %d", n)
	}

	off := newEnv(t, friday0200, true)
	off.settings.update(func(s *library.AISettings) { s.RecEnabled = false })
	off.s.FirstSync(ctx)
	unconfigured := newEnv(t, friday0200, false)
	unconfigured.s.FirstSync(ctx)
	if len(off.store.all())+len(unconfigured.store.all()) != 0 {
		t.Error("a first run with picks off or without a key")
	}
}

// Startup fails what a crash left active, with or without a key; without
// one nothing runs.
func TestStart_FailsActiveRuns(t *testing.T) {
	for _, configured := range []bool{true, false} {
		left := library.RecRun{ID: 1, Status: "running", CreatedAt: friday0200}
		e := newEnv(t, friday0200, configured, left)
		e.start(t)
		run := e.store.all()[0]
		if run.Status != recommend.StatusFailed || *run.Error != restartedMessage {
			t.Errorf("configured %v: run = %+v", configured, run)
		}
		if !configured {
			time.Sleep(10 * time.Millisecond)
			if e.clock.afterCalls() != 0 {
				t.Error("the schedule runs without a key")
			}
		}
	}
}
