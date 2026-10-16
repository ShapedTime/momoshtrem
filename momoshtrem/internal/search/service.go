package search

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/service"
	"github.com/shapedtime/momoshtrem/internal/torznab"
)

// Cache and progress timings (PLAN §6 "Search" › Progress, Cache).
const (
	// cacheTTL is how long a search's results are read without searching
	// again, from when it started.
	cacheTTL = 10 * time.Minute
	// refreshEvery: refresh=1 starts a new search at most this often per
	// plan.
	refreshEvery = 10 * time.Second
	// maxPlans caps the cache; the oldest search goes first.
	maxPlans = 200
	// defaultWait is how long the GET that starts a search waits for it.
	defaultWait = 2 * time.Second
	// defaultRunTimeout bounds a whole search, fallback included.
	defaultRunTimeout = 60 * time.Second
)

// TitleSource builds a search's title context. *service.TitleContexts
// implements it.
type TitleSource interface {
	Get(ctx context.Context, mediaType library.JobMediaType, tmdbID int) (*service.TitleContext, error)
}

// ProfileSource reads the quality profile. *library.SettingsRepository
// implements it.
type ProfileSource interface {
	QualityProfile(ctx context.Context) (library.QualityProfile, error)
}

// Jackett is what a search asks of Jackett. *torznab.Client implements it.
type Jackett interface {
	Indexers(ctx context.Context) ([]torznab.Indexer, error)
	Fanout(ctx context.Context, reqs []torznab.Request, opts torznab.FanoutOptions) []torznab.Response
	Download(ctx context.Context, link string) (magnet string, torrent []byte, err error)
	SafeError(err error) string
}

var (
	_ TitleSource   = (*service.TitleContexts)(nil)
	_ ProfileSource = (*library.SettingsRepository)(nil)
	_ Jackett       = (*torznab.Client)(nil)
)

// Deps are a Service's dependencies. Titles, Jackett and Profiles are
// required.
type Deps struct {
	Titles   TitleSource
	Jackett  Jackett
	Profiles ProfileSource
	Now      func() time.Time // nil: time.Now
	Log      *slog.Logger     // nil: slog.Default()
	// Wait is how long the GET that starts a search waits for it (0: 2 s),
	// and RunTimeout bounds a whole search (0: 60 s).
	Wait, RunTimeout time.Duration
}

// Service is torrent search (PLAN §6 "Search"): GET /search/torrents and
// resolving a release to a magnet. Searches run detached from requests,
// one per plan at a time, and their results are cached for 10 minutes;
// every read applies the current quality profile, filters, sort and
// paging to what has arrived.
type Service struct {
	titles     TitleSource
	jackett    Jackett
	profiles   ProfileSource
	now        func() time.Time
	log        *slog.Logger
	wait       time.Duration
	runTimeout time.Duration

	mu      sync.Mutex
	entries map[string]*entry // by plan key

	index *releaseIndex
}

// New creates a Service.
func New(d Deps) *Service {
	s := &Service{
		titles:     d.Titles,
		jackett:    d.Jackett,
		profiles:   d.Profiles,
		now:        d.Now,
		log:        d.Log,
		wait:       d.Wait,
		runTimeout: d.RunTimeout,
		entries:    map[string]*entry{},
		index:      newReleaseIndex(),
	}
	if s.now == nil {
		s.now = time.Now
	}
	if s.log == nil {
		s.log = slog.Default()
	}
	s.log = s.log.With("component", "torrent-search")
	if s.wait <= 0 {
		s.wait = defaultWait
	}
	if s.runTimeout <= 0 {
		s.runTimeout = defaultRunTimeout
	}
	return s
}

// Search answers GET /search/torrents. Its errors: ErrBadRequest (400),
// service.ErrTitleNotFound (404), library.ErrTMDBFetch (502
// tmdb_unavailable), ErrJackettUnavailable (502), and the caller's own
// cancellation.
//
// The plan's search starts unless it's cached or running. The GET that
// starts it answers when it completes or after Deps.Wait; one that finds
// it running answers at once, with what has arrived (complete is false).
func (s *Service) Search(ctx context.Context, req SearchRequest) (*SearchResponse, error) {
	mt, err := req.validate()
	if err != nil {
		return nil, err
	}
	tc, err := s.titles.Get(ctx, mt, req.TMDBID)
	if err != nil {
		return nil, err
	}
	plan, err := Plan(tc, req.planRequest())
	if err != nil {
		return nil, err
	}

	e, started := s.entryFor(plan, tc, req.Refresh)
	if started {
		timer := time.NewTimer(s.wait)
		select {
		case <-e.done:
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		}
		timer.Stop()
	}

	now := s.now()
	snap := e.snapshot(now)
	if snap.err != nil {
		return nil, snap.err
	}
	var warnings []string
	profile, err := s.profiles.QualityProfile(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		s.log.Warn("Quality profile unreadable; ranking with the defaults", "error", err)
		profile = library.DefaultQualityProfile()
		warnings = append(warnings, "Your quality profile couldn't be read, so results are ranked with the default one.")
	}
	return read(snap, tc, req, profile, now, warnings), nil
}

// entryFor returns the plan's cache entry, starting a search when there's
// none, it's older than cacheTTL, it failed, or refresh asks for one and
// the current one is at least refreshEvery old. started reports a new
// search.
func (s *Service) entryFor(plan *QueryPlan, tc *service.TitleContext, refresh bool) (e *entry, started bool) {
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()

	if cur := s.entries[plan.Key]; cur != nil {
		age := now.Sub(cur.startedAt)
		running, failed := cur.state()
		switch {
		case refresh && age >= refreshEvery:
		case failed && !running:
		case running || age < cacheTTL:
			return cur, false
		}
		cur.cancel()
		delete(s.entries, plan.Key)
	}

	e = newEntry(plan, tc, now, s.runTimeout)
	s.entries[plan.Key] = e
	s.evictLocked(now)
	go s.run(e)
	return e, true
}

// evictLocked drops expired searches, then the oldest ones while the cache
// is over maxPlans. Dropping a running search cancels it.
func (s *Service) evictLocked(now time.Time) {
	for key, e := range s.entries {
		if running, _ := e.state(); !running && now.Sub(e.startedAt) >= cacheTTL {
			delete(s.entries, key)
		}
	}
	for len(s.entries) > maxPlans {
		var oldest *entry
		for _, e := range s.entries {
			if oldest == nil || e.startedAt.Before(oldest.startedAt) {
				oldest = e
			}
		}
		oldest.cancel()
		delete(s.entries, oldest.key)
	}
}
