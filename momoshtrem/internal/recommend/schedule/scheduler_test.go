package schedule

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/shapedtime/momoshtrem/internal/aicore"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/member"
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

// fakeStore keeps runs in memory, one active a member at a time.
type fakeStore struct {
	mu     sync.Mutex
	runs   []library.RecRun
	nextID int64
	at     func() time.Time
}

func active(r library.RecRun) bool {
	return r.Status == recommend.StatusQueued || r.Status == recommend.StatusRunning
}

func (s *fakeStore) QueueRecRun(_ context.Context, memberID int64, kind, model string) (int64, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range s.runs {
		if r.MemberID == memberID && active(r) {
			return r.ID, false, nil
		}
	}
	s.nextID++
	s.runs = append(s.runs, library.RecRun{ID: s.nextID, MemberID: memberID, Kind: kind, Model: model,
		Status: recommend.StatusQueued, CreatedAt: s.at()})
	return s.nextID, true, nil
}

// find is the newest run pred holds for.
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

// byAge is every run pred holds for, oldest queued first.
func (s *fakeStore) byAge(pred func(library.RecRun) bool) []library.RecRun {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []library.RecRun
	for _, r := range s.runs {
		if pred(r) {
			out = append(out, r)
		}
	}
	slices.SortStableFunc(out, func(a, b library.RecRun) int {
		if c := a.CreatedAt.Compare(b.CreatedAt); c != 0 {
			return c
		}
		return int(a.ID - b.ID)
	})
	return out
}

func (s *fakeStore) ActiveRecRuns(context.Context) ([]library.RecRun, error) {
	return s.byAge(active), nil
}

func (s *fakeStore) NextQueuedRecRun(context.Context) (*library.RecRun, error) {
	queued := s.byAge(func(r library.RecRun) bool { return r.Status == recommend.StatusQueued })
	if len(queued) == 0 {
		return nil, nil
	}
	return &queued[0], nil
}

func (s *fakeStore) ActiveRecRun(_ context.Context, memberID int64) (*library.RecRun, error) {
	return s.find(func(r library.RecRun) bool { return r.MemberID == memberID && active(r) }), nil
}

func (s *fakeStore) LatestSucceededRecRun(_ context.Context, memberID int64) (*library.RecRun, error) {
	return s.find(func(r library.RecRun) bool { return r.MemberID == memberID && r.Status == recommend.StatusSucceeded }), nil
}

func (s *fakeStore) LatestFinishedRecRun(_ context.Context, memberID int64) (*library.RecRun, error) {
	return s.find(func(r library.RecRun) bool { return r.MemberID == memberID && !active(r) }), nil
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
		if active(s.runs[i]) {
			s.runs[i].Status, s.runs[i].Error = recommend.StatusFailed, &msg
			n++
		}
	}
	return n, nil
}

func (s *fakeStore) FailRecRun(_ context.Context, id int64, msg string, _ json.RawMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.runs {
		if s.runs[i].ID == id && active(s.runs[i]) {
			s.runs[i].Status, s.runs[i].Error = recommend.StatusFailed, &msg
			return nil
		}
	}
	return library.ErrRecRunNotActive
}

// add stores a run as another process would.
func (s *fakeStore) add(r library.RecRun) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	r.ID = s.nextID
	s.runs = append(s.runs, r)
	return r.ID
}

func (s *fakeStore) set(id int64, status string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.runs {
		if s.runs[i].ID == id {
			s.runs[i].Status = status
		}
	}
}

// get is run id, the zero run once it's gone.
func (s *fakeStore) get(id int64) library.RecRun {
	if r := s.find(func(r library.RecRun) bool { return r.ID == id }); r != nil {
		return *r
	}
	return library.RecRun{}
}

// remove deletes runs, as a removed member's cascade does.
func (s *fakeStore) remove(pred func(library.RecRun) bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.runs = slices.DeleteFunc(s.runs, pred)
}

func (s *fakeStore) all() []library.RecRun {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]library.RecRun(nil), s.runs...)
}

// The rest of the runs.ReadStore the usage page reads.
func (s *fakeStore) GetRecRun(_ context.Context, id int64) (*library.RecRun, error) {
	return s.find(func(r library.RecRun) bool { return r.ID == id }), nil
}
func (s *fakeStore) ListRecRuns(context.Context, int64, int) ([]library.RecRun, error) {
	return nil, nil
}
func (s *fakeStore) RecRunShelves(context.Context, int64) ([]library.RecShelfRow, error) {
	return nil, nil
}
func (s *fakeStore) RecRunHeroPicks(context.Context, int64) ([]library.RecHeroPickRow, error) {
	return nil, nil
}
func (s *fakeStore) RecommendationReason(context.Context, int64, library.TitleRef) (*string, error) {
	return nil, nil
}
func (s *fakeStore) RecRunCostMeans(context.Context, int) (map[string]float64, error) {
	return nil, nil
}

// fakeExecutor runs a run by marking it succeeded; hold blocks it until
// released (or its context ends: failed), and a run in gone finds its row
// deleted under it (its member removed).
type fakeExecutor struct {
	store *fakeStore
	hold  chan struct{}
	mu    sync.Mutex
	ran   []library.RecRun
	gone  map[int64]bool
}

func (e *fakeExecutor) Execute(ctx context.Context, run library.RecRun) (runs.Outcome, error) {
	e.mu.Lock()
	gone := e.gone[run.ID]
	e.mu.Unlock()
	if gone {
		e.store.remove(func(r library.RecRun) bool { return r.ID == run.ID })
		return runs.Outcome{Status: recommend.StatusFailed}, fmt.Errorf("run %d: %w", run.ID, library.ErrRecRunNotActive)
	}
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

// members are whose runs it executed, in order.
func (e *fakeExecutor) members() []int64 {
	e.mu.Lock()
	defer e.mu.Unlock()
	var out []int64
	for _, r := range e.ran {
		out = append(out, r.MemberID)
	}
	return out
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

// histories are each member's history check: enough by default.
type histories struct {
	mu  sync.Mutex
	set map[int64]profile.Readiness
}

func (h *histories) of(memberID int64) History {
	h.mu.Lock()
	defer h.mu.Unlock()
	if ready, ok := h.set[memberID]; ok {
		return history{ready: ready}
	}
	return history{ready: profile.Readiness{Connected: true, Counts: profile.Counts{Loved: 9}}}
}

func (h *histories) put(memberID int64, ready profile.Readiness) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.set[memberID] = ready
}

// fakeMembers are the household, and who has Trakt connected.
type fakeMembers struct {
	mu       sync.Mutex
	ids      []int64
	eligible map[int64]bool
}

func (m *fakeMembers) IDs(context.Context) ([]int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return slices.Clone(m.ids), nil
}

func (m *fakeMembers) Eligible(id int64) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.eligible[id]
}

// set makes the household ids, the eligible ones with Trakt connected.
func (m *fakeMembers) set(ids []int64, eligible ...int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ids, m.eligible = ids, map[int64]bool{}
	for _, id := range eligible {
		m.eligible[id] = true
	}
}

type env struct {
	clock    *fakeClock
	store    *fakeStore
	exec     *fakeExecutor
	settings *settingsBox
	members  *fakeMembers
	history  *histories
	s        *Scheduler
}

// friday0200 is Friday 16 October 2026, 02:00, server-local.
var friday0200 = time.Date(2026, 10, 16, 2, 0, 0, 0, time.UTC)

// as is a request for the member.
func as(memberID int64) context.Context { return member.WithID(context.Background(), memberID) }

// newEnv is a scheduler over fakes, for a household of member 1 with
// Trakt connected.
func newEnv(t *testing.T, start time.Time, configured bool, prior ...library.RecRun) *env {
	t.Helper()
	e := &env{clock: &fakeClock{now: start}, settings: &settingsBox{s: library.DefaultAISettings()},
		members: &fakeMembers{}, history: &histories{set: map[int64]profile.Readiness{}}}
	e.members.set([]int64{1}, 1)
	for i := range prior {
		if prior[i].MemberID == 0 {
			prior[i].MemberID = 1
		}
	}
	e.store = &fakeStore{runs: prior, nextID: int64(len(prior)), at: e.clock.Now}
	e.exec = &fakeExecutor{store: e.store, gone: map[int64]bool{}}
	e.s = New(Deps{
		Store: e.store, Runner: e.exec, Settings: e.settings, Budget: budget{},
		History:    e.history.of,
		Members:    e.members,
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

// next_run_at follows the settings at once, and is null when manual, off,
// not configured, or for a member without Trakt (no slot queues theirs).
func TestNextRunAt(t *testing.T) {
	e := newEnv(t, friday0200, true)
	ctx := as(1)
	if next, _ := e.s.NextRunAt(ctx); next == nil || !next.Equal(friday0200.Add(time.Hour)) {
		t.Errorf("nightly = %v", next)
	}
	e.members.set([]int64{1, 2}, 1)
	if next, err := e.s.NextRunAt(as(2)); next != nil || err != nil {
		t.Errorf("a member without Trakt = %v, %v", next, err)
	}
	if _, err := e.s.NextRunAt(context.Background()); !errors.Is(err, member.ErrNoMember) {
		t.Errorf("no member = %v, want member.ErrNoMember", err)
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

// At 03:00 a scheduled run is queued for every member with Trakt
// connected, one after the other in an order that moves on by one each
// night, and they're executed oldest first; a member without Trakt gets
// none. Then the next night's.
func TestSchedule_Slot(t *testing.T) {
	prior := library.RecRun{ID: 1, Kind: "scheduled", Status: "succeeded", Model: "claude-opus-5-5",
		CreatedAt: friday0200.Add(-23 * time.Hour)}
	e := newEnv(t, friday0200, true, prior)
	e.members.set([]int64{1, 2, 3}, 1, 3) // 2 has no Trakt
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
	waitUntil(t, "the 03:00 runs", func() bool { return e.exec.count() == 2 })
	// 16 October is day 20742 since the epoch: even, so 1 first.
	if got := e.exec.members(); !reflect.DeepEqual(got, []int64{1, 3}) {
		t.Errorf("tonight's runs were members %v, want 1 then 3", got)
	}
	for _, run := range e.store.all()[1:] {
		if run.Kind != recommend.KindScheduled || run.Model != "claude-opus-5-5" || run.Status != "succeeded" {
			t.Errorf("run = %+v", run)
		}
	}
	usage, _ := e.s.RecsUsage(as(3))
	if usage.NextRunAt == nil || !usage.NextRunAt.Equal(time.Date(2026, 10, 17, 3, 0, 0, 0, time.UTC)) ||
		usage.LastRun == nil || usage.LastRun.ID != 3 {
		t.Errorf("member 3's usage after = %+v", usage)
	}
	e.waitWaiting(t, 2)
	e.clock.Advance(24 * time.Hour)
	waitUntil(t, "the next night's runs", func() bool { return e.exec.count() == 4 })
	if got := e.exec.members()[2:]; !reflect.DeepEqual(got, []int64{3, 1}) {
		t.Errorf("the next night's runs were members %v, want 3 then 1", got)
	}
}

// A slot missed while the server was down runs a minute after startup,
// once, for every member with Trakt; none when a run was queued since, or
// on a fresh install.
func TestSchedule_CatchUp(t *testing.T) {
	friday1000 := friday0200.Add(8 * time.Hour)
	before := library.RecRun{ID: 1, Kind: "scheduled", Status: "succeeded", CreatedAt: friday0200.Add(-23 * time.Hour)}

	e := newEnv(t, friday1000, true, before)
	e.members.set([]int64{1, 2}, 1, 2)
	e.start(t)
	if next, _ := e.s.NextRunAt(as(1)); next == nil || !next.Equal(friday1000.Add(CatchUpDelay)) {
		t.Errorf("next run = %v, want the catch-up a minute after startup", next)
	}
	e.waitWaiting(t, 1)
	e.clock.Advance(CatchUpDelay)
	waitUntil(t, "the catch-up runs", func() bool { return e.exec.count() == 2 })
	// The missed slot's order: 16 October, 1 first.
	if got := e.exec.members(); !reflect.DeepEqual(got, []int64{1, 2}) {
		t.Errorf("caught up members %v, want 1 then 2", got)
	}
	for _, run := range e.store.all()[1:] {
		if run.Kind != recommend.KindScheduled {
			t.Errorf("catch-up = %+v", run)
		}
	}
	waitUntil(t, "the catch-up to clear", func() bool {
		next, _ := e.s.NextRunAt(as(2))
		return next != nil && next.Equal(time.Date(2026, 10, 17, 3, 0, 0, 0, time.UTC))
	})

	since := library.RecRun{ID: 1, Kind: "manual", Status: "succeeded", CreatedAt: friday0200.Add(2 * time.Hour)}
	for name, env := range map[string]*env{
		"a run since":   newEnv(t, friday1000, true, since),
		"fresh install": newEnv(t, friday1000, true),
	} {
		env.start(t)
		if next, _ := env.s.NextRunAt(as(1)); next == nil || !next.Equal(time.Date(2026, 10, 17, 3, 0, 0, 0, time.UTC)) {
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
	if next, _ := e.s.NextRunAt(as(1)); !next.Equal(time.Date(2026, 10, 17, 3, 0, 0, 0, time.UTC)) {
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
// refresh, the first-sync trigger and a slot all join the member's active
// run, and one worker executes it. Another member's refresh queues their
// own run, which waits its turn.
func TestRefresh_SingleFlight(t *testing.T) {
	if _, err := newEnv(t, friday0200, false).s.Refresh(as(1)); !errors.Is(err, recommend.ErrUnavailable) {
		t.Errorf("no key = %v", err)
	}

	e := newEnv(t, friday0200.Add(50*time.Minute), true)
	e.exec.hold = make(chan struct{})
	e.start(t)
	ctx := as(1)

	if _, err := e.s.Refresh(context.Background()); !errors.Is(err, member.ErrNoMember) {
		t.Errorf("no member = %v", err)
	}
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
	e.history.put(1, profile.Readiness{Connected: true, Counts: profile.Counts{Loved: 4}})
	if _, err := e.s.Refresh(ctx); !errors.Is(err, recommend.ErrNoHistory) {
		t.Errorf("history = %v", err)
	}
	e.history.put(1, profile.Readiness{Connected: true, Counts: profile.Counts{Loved: 5}})

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
	e.s.FirstSync(ctx, 1)
	e.waitWaiting(t, 1)
	e.clock.Advance(10 * time.Minute) // past 03:00
	time.Sleep(20 * time.Millisecond)
	if all := e.store.all(); len(all) != 1 {
		t.Errorf("runs = %+v, want the one", all)
	}
	usage, _ := e.s.RecsUsage(ctx)
	if usage.ActiveRun == nil || usage.ActiveRun.ID != id {
		t.Errorf("active = %+v", usage.ActiveRun)
	}

	// Member 2's refresh is their own run, queued behind member 1's.
	theirs, err := e.s.Refresh(as(2))
	if err != nil || theirs == id {
		t.Fatalf("member 2's refresh = %d, %v; want their own run", theirs, err)
	}
	if usage, _ := e.s.RecsUsage(as(2)); usage.ActiveRun == nil || usage.ActiveRun.ID != theirs || usage.ActiveRun.Status != "queued" {
		t.Errorf("member 2's active = %+v", usage.ActiveRun)
	}
	time.Sleep(20 * time.Millisecond)
	if e.exec.count() != 1 {
		t.Error("a second run started beside the first")
	}
	close(e.exec.hold)
	waitUntil(t, "both runs to finish", func() bool {
		return e.store.get(id).Status == recommend.StatusSucceeded && e.store.get(theirs).Status == recommend.StatusSucceeded
	})
	if got := e.exec.members(); !reflect.DeepEqual(got, []int64{1, 2}) {
		t.Errorf("executed members %v, want 1 then 2, once each", got)
	}
}

// A member's Trakt initial import queues their first_sync run while
// nothing of theirs has succeeded; not with picks off, without a key, or
// once a run of theirs has succeeded. Another member's picks don't count.
func TestFirstSync(t *testing.T) {
	e := newEnv(t, friday0200, true)
	e.start(t)
	e.s.FirstSync(as(1), 1)
	if all := e.store.all(); len(all) != 1 || all[0].Kind != recommend.KindFirstSync || all[0].Model != "claude-opus-5-5" ||
		all[0].MemberID != 1 {
		t.Fatalf("runs = %+v", all)
	}
	waitUntil(t, "the first run", func() bool { return e.exec.count() == 1 })
	waitUntil(t, "it to succeed", func() bool { return e.store.all()[0].Status == recommend.StatusSucceeded })
	e.s.FirstSync(context.Background(), 1) // a reconnect's import: picks exist
	if n := len(e.store.all()); n != 1 {
		t.Errorf("runs after a second import = %d", n)
	}
	e.s.FirstSync(context.Background(), 2) // another member's first import
	if all := e.store.all(); len(all) != 2 || all[1].MemberID != 2 || all[1].Kind != recommend.KindFirstSync {
		t.Errorf("runs after member 2's import = %+v", all)
	}

	off := newEnv(t, friday0200, true)
	off.settings.update(func(s *library.AISettings) { s.RecEnabled = false })
	off.s.FirstSync(context.Background(), 1)
	unconfigured := newEnv(t, friday0200, false)
	unconfigured.s.FirstSync(context.Background(), 1)
	if len(off.store.all())+len(unconfigured.store.all()) != 0 {
		t.Error("a first run with picks off or without a key")
	}
}

// A pending catch-up follows the schedule: switched to manual, it's
// dropped (next_run_at null) and never runs.
func TestSchedule_CatchUpDroppedOnManual(t *testing.T) {
	friday1000 := friday0200.Add(8 * time.Hour)
	before := library.RecRun{ID: 1, Kind: "scheduled", Status: "succeeded", CreatedAt: friday0200.Add(-23 * time.Hour)}
	e := newEnv(t, friday1000, true, before)
	e.start(t)
	e.waitWaiting(t, 1)
	if next, _ := e.s.NextRunAt(as(1)); next == nil || !next.Equal(friday1000.Add(CatchUpDelay)) {
		t.Fatalf("next run = %v, want the catch-up", next)
	}
	e.settings.update(func(s *library.AISettings) { s.RecSchedule = recommend.ScheduleManual })
	if next, _ := e.s.NextRunAt(as(1)); next != nil {
		t.Errorf("manual: next run = %v", next)
	}
	e.s.Reschedule()
	e.waitWaiting(t, 2)
	e.s.mu.Lock()
	pending := e.s.catchUp
	e.s.mu.Unlock()
	if !pending.IsZero() {
		t.Errorf("catch-up still pending at %v", pending)
	}
	e.clock.Advance(CatchUpDelay)
	time.Sleep(20 * time.Millisecond)
	if e.exec.count() != 0 || len(e.store.all()) != 1 {
		t.Errorf("a manual schedule caught up: %+v", e.store.all())
	}
	// Back to nightly: tomorrow's slot, not the dropped catch-up.
	e.settings.update(func(s *library.AISettings) { s.RecSchedule = recommend.ScheduleNightly })
	if next, _ := e.s.NextRunAt(as(1)); next == nil || !next.Equal(time.Date(2026, 10, 17, 3, 0, 0, 0, time.UTC)) {
		t.Errorf("nightly again: next run = %v", next)
	}
}

// A run left running by another process is failed once it's older than a
// run may be (runs.Timeout and a margin); a younger one (recommend-run in
// another process) is joined by its member, and only by them. Startup's
// sweep doesn't see either: they're written after it.
func TestReap(t *testing.T) {
	e := newEnv(t, friday0200, true)
	e.start(t)
	young := e.clock.Now().Add(-5 * time.Minute)
	id := e.store.add(library.RecRun{MemberID: 1, Kind: "manual", Status: "running", Model: "claude-opus-5-5", CreatedAt: young, StartedAt: &young})
	if got, err := e.s.Refresh(as(1)); err != nil || got != id {
		t.Fatalf("refresh during another process's run = %d, %v; want it joined", got, err)
	}
	theirs, err := e.s.Refresh(as(2))
	if err != nil || theirs == id {
		t.Fatalf("member 2's refresh = %d, %v; want their own run", theirs, err)
	}
	waitUntil(t, "member 2's run", func() bool { return e.store.get(theirs).Status == recommend.StatusSucceeded })
	e.clock.Advance(runs.Timeout + staleMargin)
	fresh, err := e.s.Refresh(as(1))
	if err != nil || fresh == id {
		t.Fatalf("refresh after it went stale = %d, %v", fresh, err)
	}
	stale := e.store.get(id)
	if stale.Status != recommend.StatusFailed || *stale.Error != staleMessage {
		t.Errorf("stale run = %+v", stale)
	}
	waitUntil(t, "the new run", func() bool { return e.exec.count() == 2 })

	// A queued run nobody runs (its process died before starting it) is
	// picked up by the worker.
	waitUntil(t, "the run to finish", func() bool { return e.store.get(fresh).Status == recommend.StatusSucceeded })
	queued := e.store.add(library.RecRun{MemberID: 1, Kind: "manual", Status: "queued", Model: "claude-opus-5-5", CreatedAt: e.clock.Now()})
	if got, _ := e.s.Refresh(as(1)); got != queued {
		t.Errorf("refresh = %d, want %d joined", got, queued)
	}
	waitUntil(t, "the orphaned run", func() bool { return e.exec.count() == 3 })
}

// Removing a member stops their run in flight before their rows go, and
// the worker goes on to the next member's; removing a member who isn't
// running leaves the run in flight be.
func TestForgetMember(t *testing.T) {
	e := newEnv(t, friday0200, true)
	e.exec.hold = make(chan struct{})
	e.start(t)
	first, _ := e.s.Refresh(as(1))
	second, _ := e.s.Refresh(as(2))
	third, _ := e.s.Refresh(as(3))
	waitUntil(t, "member 1's run", func() bool { return e.exec.count() == 1 })

	e.s.ForgetMember(context.Background(), 3)
	e.store.remove(func(r library.RecRun) bool { return r.MemberID == 3 }) // the cascade
	if run := e.store.get(first); run.Status != recommend.StatusRunning {
		t.Errorf("member 3's removal touched member 1's run: %+v", run)
	}

	e.s.ForgetMember(context.Background(), 1)
	if run := e.store.get(first); run.Status != recommend.StatusFailed {
		t.Errorf("member 1's run after ForgetMember = %+v, want it stopped", run)
	}
	e.store.remove(func(r library.RecRun) bool { return r.MemberID == 1 })
	waitUntil(t, "member 2's run", func() bool { return e.exec.count() == 2 })
	close(e.exec.hold)
	waitUntil(t, "member 2's run to finish", func() bool { return e.store.get(second).Status == recommend.StatusSucceeded })
	if got := e.exec.members(); !reflect.DeepEqual(got, []int64{1, 2}) || e.store.get(third).ID != 0 {
		t.Errorf("executed members %v (member 3's run %+v); want 1 then 2", got, e.store.get(third))
	}
	e.s.ForgetMember(context.Background(), 2) // nothing in flight: returns at once
}

// A run whose row is gone when the worker starts it (its member was
// removed: the cascade) doesn't stop the worker: the next queued run goes
// at once.
func TestWorker_MovesOnPastAGoneRun(t *testing.T) {
	e := newEnv(t, friday0200, true)
	e.start(t)
	e.waitWaiting(t, 1)
	gone := e.store.add(library.RecRun{MemberID: 1, Kind: "manual", Status: "queued", Model: "x", CreatedAt: friday0200})
	next := e.store.add(library.RecRun{MemberID: 2, Kind: "manual", Status: "queued", Model: "x", CreatedAt: friday0200})
	e.exec.mu.Lock()
	e.exec.gone[gone] = true
	e.exec.mu.Unlock()
	e.s.kick() // once: nothing else wakes the worker here
	waitUntil(t, "the next member's run", func() bool { return e.store.get(next).Status == recommend.StatusSucceeded })
	if e.store.get(gone).ID != 0 || !reflect.DeepEqual(e.exec.members(), []int64{2}) {
		t.Errorf("runs = %+v, executed %v", e.store.all(), e.exec.members())
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

// The API reads are the request's member's, and need one.
func TestReads_NeedAMember(t *testing.T) {
	e := newEnv(t, friday0200, true)
	ctx := context.Background()
	if _, err := e.s.Home(ctx); !errors.Is(err, member.ErrNoMember) {
		t.Errorf("Home = %v", err)
	}
	if _, err := e.s.Runs(ctx, 10); !errors.Is(err, member.ErrNoMember) {
		t.Errorf("Runs = %v", err)
	}
	if _, err := e.s.Run(ctx, 1); !errors.Is(err, member.ErrNoMember) {
		t.Errorf("Run = %v", err)
	}
	if _, err := e.s.RecsUsage(ctx); !errors.Is(err, member.ErrNoMember) {
		t.Errorf("RecsUsage = %v", err)
	}
	id := e.store.add(library.RecRun{MemberID: 1, Kind: "manual", Status: "succeeded", Model: "x", CreatedAt: friday0200})
	if run, err := e.s.Run(as(1), id); err != nil || run.ID != id {
		t.Errorf("member 1's run = %+v, %v", run, err)
	}
	if _, err := e.s.Run(as(2), id); !errors.Is(err, recommend.ErrNotFound) {
		t.Errorf("member 2 reading member 1's run = %v, want recommend.ErrNotFound", err)
	}
}
