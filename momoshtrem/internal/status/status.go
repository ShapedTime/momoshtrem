// Package status assembles the system health report served at /api/status.
package status

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

const (
	// DefaultProbeTimeout bounds each dependency check.
	DefaultProbeTimeout = 3 * time.Second
	// CacheTTL is how long a report is reused before checks run again.
	CacheTTL = 30 * time.Second
	// RefreshMinInterval throttles ?refresh=1: a forced check only runs when
	// the cached report is at least this old.
	RefreshMinInterval = 5 * time.Second
)

// Prober checks that an external dependency is reachable and accepts our
// credentials. It returns nil when healthy. Error messages are shown to the
// user, so they must never contain secrets.
type Prober func(ctx context.Context) error

// JackettProbe is a Prober that also lists Jackett's indexers with their
// latest search outcome.
type JackettProbe func(ctx context.Context) ([]IndexerStatus, error)

// Database is the subset of the library DB the report needs.
type Database interface {
	PingContext(ctx context.Context) error
	SizeBytes(ctx context.Context) (int64, error)
}

// WebDAVInfo describes how clients reach the WebDAV server.
type WebDAVInfo struct {
	URL         string // Public URL; empty falls back to http://localhost:<port>/
	Port        int
	AuthEnabled bool
	Username    string // Reported only when AuthEnabled
}

// Options configures a Service. Nil probers mean "not configured".
type Options struct {
	Version   string
	StartedAt time.Time

	DB     Database
	WebDAV WebDAVInfo
	// ActiveReaders returns the number of open streaming readers. Optional.
	ActiveReaders func() int

	Jackett JackettProbe // nil when JACKETT_URL or JACKETT_API_KEY is unset
	TMDB    Prober       // nil when TMDB_API_KEY is unset
	HDRezka Prober       // nil when HDREZKA_ENABLED is off

	// TraktConfigured: Trakt's client id and a valid SECRET_KEY.
	TraktConfigured bool
	// TraktConnected reports whether Trakt's state is connected (B5), live
	// rather than cached. Optional.
	TraktConnected          func() bool
	AIConfigured            bool
	OpenSubtitlesConfigured bool
	// AICost is what the AI features have cost this month (ai_usage).
	// Optional: without it, or when it fails, mtd_cost_usd is null.
	AICost func(ctx context.Context) (float64, error)

	// ProbeTimeout bounds each check; zero means DefaultProbeTimeout.
	ProbeTimeout time.Duration
	// Now overrides the clock in tests.
	Now func() time.Time
}

// Report is the /api/status response body. It never contains secrets.
type Report struct {
	Version       string              `json:"version"`
	UptimeSeconds int64               `json:"uptime_seconds"`
	CheckedAt     string              `json:"checked_at"` // RFC 3339, when the checks ran
	DB            DBStatus            `json:"db"`
	WebDAV        WebDAVStatus        `json:"webdav"`
	Jackett       JackettStatus       `json:"jackett"`
	TMDB          ProbeStatus         `json:"tmdb"`
	Trakt         TraktStatus         `json:"trakt"`
	AI            AIStatus            `json:"ai"`
	OpenSubtitles OpenSubtitlesStatus `json:"opensubtitles"`
	HDRezka       ProbeStatus         `json:"hdrezka"`
}

// DBStatus reports Postgres health. SizeBytes is null when unknown.
type DBStatus struct {
	OK        bool   `json:"ok"`
	SizeBytes *int64 `json:"size_bytes"`
	Error     string `json:"error,omitempty"`
}

// WebDAVStatus tells the user what to enter in Infuse.
type WebDAVStatus struct {
	URL           string `json:"url"`
	Port          int    `json:"port"`
	AuthEnabled   bool   `json:"auth_enabled"`
	Username      string `json:"username,omitempty"`
	ActiveReaders *int   `json:"active_readers,omitempty"`
}

// ProbeStatus reports an external HTTP dependency. LatencyMS is set only when
// the probe succeeded.
type ProbeStatus struct {
	Configured bool   `json:"configured"`
	Reachable  bool   `json:"reachable"`
	LatencyMS  *int64 `json:"latency_ms,omitempty"`
	Error      string `json:"error,omitempty"`
}

// JackettStatus is the Jackett probe plus each configured indexer's health.
// Indexers is omitted when Jackett isn't configured or isn't reachable.
type JackettStatus struct {
	ProbeStatus
	Indexers []IndexerStatus `json:"indexers,omitzero"`
}

// IndexerStatus is one Jackett indexer's latest search outcome since
// startup: ok, timeout, error, or unknown before any search.
type IndexerStatus struct {
	Name   string  `json:"name"`
	Status string  `json:"status"`
	MS     *int64  `json:"ms"`
	Since  *string `json:"since"` // RFC 3339: when the current timeout or error streak began
	Error  string  `json:"error,omitempty"`
}

// TraktStatus reports the Trakt integration: configured (client id and
// secret and a valid SECRET_KEY) and connected (the state is connected).
type TraktStatus struct {
	Configured bool `json:"configured"`
	Connected  bool `json:"connected"`
}

// AIStatus reports the Anthropic integration. MTDCostUSD is what every AI
// feature has cost this month; null when it can't be read.
type AIStatus struct {
	Configured bool     `json:"configured"`
	MTDCostUSD *float64 `json:"mtd_cost_usd"`
}

// OpenSubtitlesStatus reports whether subtitle download is available.
type OpenSubtitlesStatus struct {
	Configured bool `json:"configured"`
}

// Service builds reports, caching probe results so the endpoint is cheap to
// poll and can't be used to hammer Jackett or TMDB.
type Service struct {
	opts Options
	now  func() time.Time

	group singleflight.Group

	mu       sync.Mutex
	cached   *Report
	cachedAt time.Time
}

// NewService creates a status service.
func NewService(opts Options) *Service {
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	if opts.StartedAt.IsZero() {
		opts.StartedAt = now()
	}
	if opts.ProbeTimeout <= 0 {
		opts.ProbeTimeout = DefaultProbeTimeout
	}
	return &Service{opts: opts, now: now}
}

// Report returns the current status. Probe results are cached for CacheTTL;
// refresh bypasses the cache unless the cached report is younger than
// RefreshMinInterval. A report with the database down is only kept for
// RefreshMinInterval, so an outage and the recovery both show up quickly.
// Concurrent callers share one in-flight check. Uptime and active readers
// are always live.
func (s *Service) Report(ctx context.Context, refresh bool) Report {
	s.mu.Lock()
	cached, cachedAt := s.cached, s.cachedAt
	s.mu.Unlock()

	maxAge := CacheTTL
	if refresh || (cached != nil && !cached.DB.OK) {
		maxAge = RefreshMinInterval
	}

	var report Report
	if cached != nil && s.now().Sub(cachedAt) < maxAge {
		report = *cached
	} else {
		// Detach from the caller: other callers may be waiting on this check.
		checkCtx := context.WithoutCancel(ctx)
		v, _, _ := s.group.Do("status", func() (any, error) {
			// A check may have finished between our cache read and Do.
			s.mu.Lock()
			if s.cached != nil && s.now().Sub(s.cachedAt) < RefreshMinInterval {
				r := *s.cached
				s.mu.Unlock()
				return r, nil
			}
			s.mu.Unlock()

			r := s.check(checkCtx)
			s.mu.Lock()
			s.cached, s.cachedAt = &r, s.now()
			s.mu.Unlock()
			return r, nil
		})
		report = v.(Report)
	}

	return s.withLiveFields(report)
}

// withLiveFields fills values that are cheap and should never be stale.
func (s *Service) withLiveFields(r Report) Report {
	r.UptimeSeconds = int64(s.now().Sub(s.opts.StartedAt).Seconds())
	if s.opts.TraktConnected != nil {
		r.Trakt.Connected = s.opts.TraktConnected()
	}
	if s.opts.ActiveReaders != nil {
		n := s.opts.ActiveReaders()
		r.WebDAV.ActiveReaders = &n
	}
	return r
}

// check runs every probe concurrently and assembles a report.
func (s *Service) check(ctx context.Context) Report {
	r := Report{
		Version:       s.opts.Version,
		CheckedAt:     s.now().UTC().Format(time.RFC3339),
		WebDAV:        s.webDAVStatus(),
		Trakt:         TraktStatus{Configured: s.opts.TraktConfigured},
		AI:            AIStatus{Configured: s.opts.AIConfigured},
		OpenSubtitles: OpenSubtitlesStatus{Configured: s.opts.OpenSubtitlesConfigured},
	}

	var wg sync.WaitGroup
	wg.Add(5)
	go func() { defer wg.Done(); r.DB = s.checkDB(ctx) }()
	go func() { defer wg.Done(); r.Jackett = s.checkJackett(ctx) }()
	go func() { defer wg.Done(); r.TMDB = s.runProbe(ctx, "tmdb", s.opts.TMDB) }()
	go func() { defer wg.Done(); r.HDRezka = s.runProbe(ctx, "hdrezka", s.opts.HDRezka) }()
	go func() { defer wg.Done(); r.AI.MTDCostUSD = s.aiCost(ctx) }()
	wg.Wait()

	return r
}

// aiCost reads this month's AI cost, rounded to a hundredth of a cent;
// nil when it isn't configured or fails.
func (s *Service) aiCost(ctx context.Context) *float64 {
	if s.opts.AICost == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, s.opts.ProbeTimeout)
	defer cancel()
	cost, err := s.opts.AICost(ctx)
	if err != nil {
		slog.Warn("Status: AI cost query failed", "error", err)
		return nil
	}
	cost = math.Round(cost*10_000) / 10_000
	return &cost
}

func (s *Service) webDAVStatus() WebDAVStatus {
	w := s.opts.WebDAV
	st := WebDAVStatus{
		URL:         w.URL,
		Port:        w.Port,
		AuthEnabled: w.AuthEnabled,
	}
	if st.URL == "" {
		st.URL = fmt.Sprintf("http://localhost:%d/", w.Port)
	}
	if w.AuthEnabled {
		st.Username = w.Username
	}
	return st
}

func (s *Service) checkDB(ctx context.Context) DBStatus {
	if s.opts.DB == nil {
		return DBStatus{Error: "database not configured"}
	}

	ctx, cancel := context.WithTimeout(ctx, s.opts.ProbeTimeout)
	defer cancel()

	if err := s.opts.DB.PingContext(ctx); err != nil {
		// Driver errors can echo connection details, so keep them in the log.
		slog.Warn("Status: database ping failed", "error", err)
		return DBStatus{Error: s.describeDBError(err, "database unreachable")}
	}

	st := DBStatus{OK: true}
	size, err := s.opts.DB.SizeBytes(ctx)
	if err != nil {
		slog.Warn("Status: database size query failed", "error", err)
		st.Error = s.describeDBError(err, "size query failed")
		return st
	}
	st.SizeBytes = &size
	return st
}

func (s *Service) describeDBError(err error, fallback string) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Sprintf("%s: timed out after %s", fallback, s.opts.ProbeTimeout)
	}
	return fallback
}

// checkJackett runs the Jackett probe. The indexers come only with a
// reachable Jackett, as an empty list when it has none.
func (s *Service) checkJackett(ctx context.Context) JackettStatus {
	if s.opts.Jackett == nil {
		return JackettStatus{}
	}
	var indexers []IndexerStatus
	st := JackettStatus{ProbeStatus: s.runProbe(ctx, "jackett", func(ctx context.Context) error {
		var err error
		indexers, err = s.opts.Jackett(ctx)
		return err
	})}
	if st.Reachable {
		st.Indexers = indexers
		if st.Indexers == nil {
			st.Indexers = []IndexerStatus{}
		}
	}
	return st
}

func (s *Service) runProbe(ctx context.Context, name string, probe Prober) ProbeStatus {
	if probe == nil {
		return ProbeStatus{}
	}

	ctx, cancel := context.WithTimeout(ctx, s.opts.ProbeTimeout)
	defer cancel()

	start := time.Now()
	err := probe(ctx)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil {
			err = fmt.Errorf("timed out after %s", s.opts.ProbeTimeout)
		}
		slog.Warn("Status: probe failed", "dependency", name, "error", err)
		return ProbeStatus{Configured: true, Error: truncate(err.Error(), 200)}
	}

	latency := time.Since(start).Milliseconds()
	return ProbeStatus{Configured: true, Reachable: true, LatencyMS: &latency}
}

func truncate(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= n {
		return string(r)
	}
	return string(r[:n]) + "…"
}
