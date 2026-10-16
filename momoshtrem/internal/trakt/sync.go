package trakt

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/tmdb"
)

// The sync's schedule (PLAN §7 B5).
const (
	firstSyncDelay = time.Minute      // the first check after startup
	syncInterval   = 30 * time.Minute // a last_activities check
	reconcileAfter = 24 * time.Hour   // a reconcile instead, once the last one is older
	calendarMaxAge = 12 * time.Hour   // an incremental sync refreshes an older calendar
	nudgeAfter     = 5 * time.Minute  // Nudge syncs once the last check is older
	stopTimeout    = 10 * time.Second // Stop waits this long for the running sync
)

// SyncAuth is the connection the sync reads Trakt with. *Auth implements
// it: Get refreshes after a 401 and retries once, a second 401 is
// ErrReauth.
type SyncAuth interface {
	Getter
	Token(ctx context.Context) (Token, error)
	Load(ctx context.Context) error
	Connected() bool
}

var _ SyncAuth = (*Auth)(nil)

// SyncStore is where the sync writes. Every write names the connection
// (Token.Generation) and is refused once it's replaced. *library.TraktSyncStore
// implements it.
type SyncStore interface {
	State(ctx context.Context) (library.TraktSyncState, error)
	LatestPlays(ctx context.Context) (movie, episode time.Time, err error)
	ReplaceHistory(ctx context.Context, gen time.Time, plays []library.TraktPlayRow) error
	UpsertHistory(ctx context.Context, gen time.Time, plays []library.TraktPlayRow) error
	ReplaceRatings(ctx context.Context, gen time.Time, types []string, ratings []library.TraktRatingRow) error
	ReplaceWatchlist(ctx context.Context, gen time.Time, items []library.TraktWatchlistRow) error
	ReplaceHidden(ctx context.Context, gen time.Time, shows []library.TraktHiddenRow) error
	ReplaceShowProgress(ctx context.Context, gen time.Time, shows []library.TraktShowProgressRow) error
	ReplaceCalendar(ctx context.Context, gen time.Time, entries []library.TraktCalendarRow, fetchedAt time.Time) error
	ArtworkWants(ctx context.Context) (library.TraktArtworkWants, error)
	SetArtwork(ctx context.Context, gen time.Time, a library.TraktArtwork) error
	FinishSync(ctx context.Context, gen time.Time, d library.TraktSyncDone) error
	SaveRun(ctx context.Context, gen time.Time, run json.RawMessage) error
}

var _ SyncStore = (*library.TraktSyncStore)(nil)

// Artwork is TMDB, for the details step. *tmdb.Cached implements it.
type Artwork interface {
	MovieDetails(ctx context.Context, id int) (*tmdb.MovieDetails, error)
	ShowDetails(ctx context.Context, id int) (*tmdb.ShowDetails, error)
	Season(ctx context.Context, showID, seasonNumber int, showStatus string) (*tmdb.Season, error)
}

var _ Artwork = (*tmdb.Cached)(nil)

// SyncDeps are what a Syncer needs.
type SyncDeps struct {
	Auth    SyncAuth
	Store   SyncStore
	Artwork Artwork // nil: no details step
	Clock   Clock   // nil: real time
}

// Syncer imports the Trakt account (PLAN §6 "Trakt (B5)", #132): the
// initial import after a connect, then a last_activities check every 30
// minutes that fetches only what changed, and a reconcile once the last
// one is a day old. One sync runs at a time; a trigger during one queues
// exactly one more. It is safe for concurrent use.
type Syncer struct {
	auth  SyncAuth
	store SyncStore
	art   Artwork
	clock Clock
	log   *slog.Logger

	wg sync.WaitGroup

	mu        sync.Mutex
	stopped   bool
	stopLoop  context.CancelFunc
	nextAt    time.Time // the next scheduled check
	running   bool      // a sync is planned or running
	queued    bool      // another sync follows it
	initial   bool      // the next sync is an initial import (a connect)
	resume    *plan     // a failed import, picked up by the next sync
	cancel    context.CancelFunc
	done      chan struct{}
	seq       int     // the running sync's; StopSync moves it on
	run       SyncRun // the running sync, else the last; zero: read the stored one
	lastCheck time.Time

	onImported func(context.Context) // SetOnImported
}

// SetOnImported installs a function called when an initial import has
// stored everything, before the sync reports it idle: the picks queue
// their first run there (#157), so For You finds it waiting.
func (s *Syncer) SetOnImported(fn func(context.Context)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onImported = fn
}

// NewSyncer creates the sync. Plug it in with Auth.SetHooks(s.Hooks()),
// then Start it.
func NewSyncer(d SyncDeps) *Syncer {
	s := &Syncer{auth: d.Auth, store: d.Store, art: d.Artwork, clock: d.Clock, log: slog.With("component", "trakt-sync")}
	if s.clock == nil {
		s.clock = realClock{}
	}
	return s
}

// Hooks are the connection's hooks: a connect starts the initial import,
// a disconnect stops the sync, and POST /integrations/trakt/sync and GET
// /integrations/trakt reach it.
func (s *Syncer) Hooks() Hooks {
	return Hooks{
		OnConnected: func() { s.goTrigger(true) },
		StopSync:    s.StopSync,
		SyncNow:     s.SyncNow,
		SyncStatus:  s.Status,
	}
}

// Start runs the schedule: a check a minute after startup, then every 30
// minutes, each a reconcile once the last one is over a day old.
func (s *Syncer) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped || s.stopLoop != nil {
		cancel()
		return
	}
	s.stopLoop = cancel
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.loop(ctx)
	}()
}

func (s *Syncer) loop(ctx context.Context) {
	wait := firstSyncDelay
	for {
		s.mu.Lock()
		s.nextAt = s.clock.Now().Add(wait)
		s.mu.Unlock()
		if s.clock.Sleep(ctx, wait) != nil {
			return
		}
		if err := s.trigger(ctx, false); err != nil && !errors.Is(err, ErrNotConnected) && ctx.Err() == nil {
			s.log.Error("Failed to start the scheduled Trakt sync", "error", err)
		}
		wait = syncInterval
	}
}

// Stop ends the schedule and the running sync, waiting for it at most 10
// s (shutdown).
func (s *Syncer) Stop() {
	s.mu.Lock()
	s.stopped, s.queued = true, false
	stopLoop, cancel := s.stopLoop, s.cancel
	s.mu.Unlock()
	if stopLoop != nil {
		stopLoop()
	}
	if cancel != nil {
		cancel()
	}
	stopped := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-time.After(stopTimeout):
		s.log.Warn("The Trakt sync didn't stop in time")
	}
}

// StopSync cancels the running sync and anything queued, and waits for it
// until ctx ends (a disconnect, which purges next). What the sync reports
// goes back to never synced.
func (s *Syncer) StopSync(ctx context.Context) {
	s.mu.Lock()
	s.queued, s.initial, s.resume = false, false, nil
	s.seq++
	cancel, done := s.cancel, s.done
	s.mu.Unlock()
	if cancel != nil {
		cancel()
		select {
		case <-done:
		case <-ctx.Done():
			s.log.Warn("The Trakt sync didn't stop in time; its writes are refused after the purge")
		}
	}
	s.mu.Lock()
	s.run, s.lastCheck = SyncRun{}, time.Time{}
	s.mu.Unlock()
}

// SyncNow starts a sync, or queues one after the running one (POST
// /integrations/trakt/sync): incremental, a reconcile when the last one is
// a day old, or a failed import picked up again.
func (s *Syncer) SyncNow(ctx context.Context) error {
	return s.trigger(ctx, false)
}

// Nudge syncs in the background when the last check is over 5 minutes old
// (GET /me/up-next).
func (s *Syncer) Nudge() {
	s.mu.Lock()
	now := s.clock.Now()
	due := !s.running && now.Sub(s.lastCheck) >= nudgeAfter
	if due {
		s.lastCheck = now // one nudge at a time
	}
	s.mu.Unlock()
	if due {
		s.goTrigger(false)
	}
}

// Status is the sync's part of GET /integrations/trakt.
func (s *Syncer) Status(ctx context.Context) (SyncStatus, error) {
	st, err := s.store.State(ctx)
	if err != nil {
		return SyncStatus{}, err
	}
	s.mu.Lock()
	run, next := s.run, s.nextAt
	s.mu.Unlock()
	if run.Status == "" && st.LastRun != nil {
		if err := json.Unmarshal(st.LastRun, &run); err != nil {
			s.log.Warn("Unreadable stored Trakt sync run", "error", err)
			run = SyncRun{}
		}
	}
	out := SyncStatus{Run: run}
	if !st.LastSyncAt.IsZero() {
		out.LastSyncAt = ptr(st.LastSyncAt)
	}
	if !next.IsZero() {
		out.NextSyncAt = ptr(next)
	}
	return out, nil
}

// goTrigger triggers a sync without waiting (a connect, a nudge).
func (s *Syncer) goTrigger(initial bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped {
		return
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		if err := s.trigger(context.Background(), initial); err != nil && !errors.Is(err, ErrNotConnected) {
			s.log.Error("Failed to start a Trakt sync", "error", err)
		}
	}()
}

// trigger starts a sync, or queues one more behind the running one.
// ErrNotConnected unless connected.
func (s *Syncer) trigger(ctx context.Context, initial bool) error {
	if err := s.auth.Load(ctx); err != nil {
		return err
	}
	if !s.auth.Connected() {
		return ErrNotConnected
	}
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return nil
	}
	if initial {
		s.initial, s.resume = true, nil
	}
	if s.running {
		s.queued = true
		s.mu.Unlock()
		return nil
	}
	s.running = true
	s.mu.Unlock()
	return s.start(ctx)
}

// start plans a sync from the stored state and runs it; running is set,
// so the state read is the last sync's.
func (s *Syncer) start(ctx context.Context) error {
	st, err := s.store.State(ctx)
	s.mu.Lock()
	defer s.mu.Unlock()
	if err != nil || s.stopped {
		s.running = false
		return err
	}
	p := s.planLocked(st)
	runCtx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	s.cancel, s.done = cancel, done
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer close(done)
		defer cancel()
		s.finish(s.execute(runCtx, p))

		s.mu.Lock()
		again := s.queued && !s.stopped
		s.queued = false
		s.running = again
		if !again {
			s.cancel, s.done = nil, nil
		}
		s.mu.Unlock()
		if again {
			if err := s.start(context.Background()); err != nil {
				s.log.Error("Failed to start the queued Trakt sync", "error", err)
			}
		}
	}()
	return nil
}

// planLocked decides what the next sync does: an initial import after a
// connect or while none has finished, a failed import picked up again, a
// reconcile once the last is a day old, else an incremental sync.
func (s *Syncer) planLocked(st library.TraktSyncState) plan {
	now := s.clock.Now()
	var p plan
	switch {
	case s.initial:
		p = plan{kind: SyncInitial}
	case s.resume != nil:
		p = *s.resume
	case st.Activities == nil:
		p = plan{kind: SyncInitial}
	case now.Sub(st.LastReconcileAt) > reconcileAfter:
		p = plan{kind: SyncReconcile}
	default:
		p = plan{kind: SyncIncremental}
	}
	s.initial, s.resume = false, nil
	s.seq++
	p.seq, p.prev = s.seq, st
	s.run = SyncRun{
		Status: SyncSyncing, Kind: ptr(p.kind), Step: ptr(fullSteps[p.from]),
		Progress: p.progress, StartedAt: ptr(now),
	}
	s.lastCheck = now
	return p
}

// update changes the running sync's report, unless StopSync moved on.
func (s *Syncer) update(seq int, fn func(*SyncRun)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.seq == seq {
		fn(&s.run)
	}
}

// finish records how a sync ended. A stopped one isn't recorded, and one
// whose connection went leaves nothing: a disconnect purged it.
func (s *Syncer) finish(r *run, err error) {
	if r.ctx.Err() != nil {
		return
	}
	now := s.clock.Now()
	s.mu.Lock()
	if s.seq != r.plan.seq {
		s.mu.Unlock()
		return
	}
	out := s.run
	switch {
	case err == nil:
		out = r.result
	case errors.Is(err, ErrNotConnected), errors.Is(err, ErrNotConfigured), errors.Is(err, library.ErrTraktConnectionReplaced):
		s.run = SyncRun{}
		s.mu.Unlock()
		s.log.Info("Trakt sync stopped: the connection is gone", "kind", r.plan.kind)
		return
	case errors.Is(err, ErrReauth):
		// Not the sync's error: the connection reports reauth.
		out.Status, out.Step, out.FinishedAt = SyncIdle, nil, ptr(now)
	default:
		out.Status, out.FinishedAt = SyncError, ptr(now)
		out.Error = ptr(syncErrorMessage(r.step, err))
		if r.plan.kind != SyncIncremental {
			s.resume = &plan{kind: r.plan.kind, from: stepIndex(r.step), snapshot: r.activities, progress: out.Progress}
		}
	}
	s.run = out
	s.mu.Unlock()

	switch {
	case err == nil:
		s.log.Info("Trakt sync finished", "kind", r.plan.kind, "plays", out.Progress.Plays,
			"ratings", out.Progress.Ratings, "watchlist", out.Progress.Watchlist, "shows", out.Progress.Shows,
			"duration", now.Sub(*out.StartedAt).Round(time.Millisecond))
		return
	case errors.Is(err, ErrReauth):
		s.log.Warn("Trakt sync stopped: connect Trakt again", "kind", r.plan.kind, "step", r.step)
	default:
		s.log.Error("Trakt sync failed", "kind", r.plan.kind, "step", r.step, "error", err)
	}
	if r.gen.IsZero() {
		return
	}
	raw, err := json.Marshal(out)
	if err == nil {
		err = s.store.SaveRun(r.ctx, r.gen, raw)
	}
	if err != nil && !errors.Is(err, library.ErrTraktConnectionReplaced) {
		s.log.Warn("Failed to store the Trakt sync run", "error", err)
	}
}
