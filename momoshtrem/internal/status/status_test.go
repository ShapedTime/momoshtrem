package status

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeClock is a manually advanced clock.
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func newFakeClock() *fakeClock {
	return &fakeClock{t: time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

// fakeDB implements Database.
type fakeDB struct {
	pingErr error
	sizeErr error
	size    int64
	block   bool // ping waits for the context to end
	pings   atomic.Int32
}

func (f *fakeDB) PingContext(ctx context.Context) error {
	f.pings.Add(1)
	if f.block {
		<-ctx.Done()
		return ctx.Err()
	}
	return f.pingErr
}

func (f *fakeDB) SizeBytes(context.Context) (int64, error) {
	return f.size, f.sizeErr
}

// countingProber returns err and counts calls.
func countingProber(err error, calls *atomic.Int32) Prober {
	return func(context.Context) error {
		calls.Add(1)
		return err
	}
}

// jackettProbe turns p into a Jackett probe that lists indexers on success.
func jackettProbe(p Prober, indexers ...IndexerStatus) JackettProbe {
	return func(ctx context.Context) ([]IndexerStatus, error) {
		if err := p(ctx); err != nil {
			return nil, err
		}
		return indexers, nil
	}
}

func TestReport_AllConfigured(t *testing.T) {
	clock := newFakeClock()
	started := clock.Now().Add(-90 * time.Second)
	var jackettCalls, tmdbCalls atomic.Int32
	readers := 2

	svc := NewService(Options{
		Version:   "1.2.3",
		StartedAt: started,
		DB:        &fakeDB{size: 4096},
		WebDAV: WebDAVInfo{
			URL: "https://momo.example:36911/", Port: 36911,
			AuthEnabled: true, Username: "infuse",
		},
		ActiveReaders:           func() int { return readers },
		Jackett:                 jackettProbe(countingProber(nil, &jackettCalls), IndexerStatus{Name: "EZTV", Status: "unknown"}),
		TMDB:                    countingProber(errors.New("TMDB rejected the API key (HTTP 401)"), &tmdbCalls),
		HDRezka:                 func(context.Context) error { return errors.New("HDRezka asks for a browser check") },
		TraktConfigured:         true,
		AIConfigured:            true,
		OpenSubtitlesConfigured: true,
		Now:                     clock.Now,
	})

	r := svc.Report(context.Background(), false)

	if r.Version != "1.2.3" {
		t.Errorf("version = %q", r.Version)
	}
	if r.UptimeSeconds != 90 {
		t.Errorf("uptime_seconds = %d, want 90", r.UptimeSeconds)
	}
	if r.CheckedAt != "2026-10-14T12:00:00Z" {
		t.Errorf("checked_at = %q", r.CheckedAt)
	}
	if !r.DB.OK || r.DB.SizeBytes == nil || *r.DB.SizeBytes != 4096 {
		t.Errorf("db = %+v", r.DB)
	}
	if r.WebDAV.URL != "https://momo.example:36911/" || r.WebDAV.Username != "infuse" || !r.WebDAV.AuthEnabled {
		t.Errorf("webdav = %+v", r.WebDAV)
	}
	if r.WebDAV.ActiveReaders == nil || *r.WebDAV.ActiveReaders != 2 {
		t.Errorf("active_readers = %v, want 2", r.WebDAV.ActiveReaders)
	}
	if !r.Jackett.Configured || !r.Jackett.Reachable || r.Jackett.LatencyMS == nil || r.Jackett.Error != "" ||
		len(r.Jackett.Indexers) != 1 || r.Jackett.Indexers[0].Name != "EZTV" {
		t.Errorf("jackett = %+v", r.Jackett)
	}
	if !r.TMDB.Configured || r.TMDB.Reachable || r.TMDB.LatencyMS != nil || !strings.Contains(r.TMDB.Error, "401") {
		t.Errorf("tmdb = %+v", r.TMDB)
	}
	if !r.Trakt.Configured || r.Trakt.Connected {
		t.Errorf("trakt = %+v", r.Trakt)
	}
	if !r.AI.Configured || r.AI.MTDCostUSD != nil {
		t.Errorf("ai = %+v", r.AI)
	}
	if !r.OpenSubtitles.Configured {
		t.Errorf("opensubtitles = %+v", r.OpenSubtitles)
	}
	if h := r.HDRezka; !h.Configured || h.Reachable || h.LatencyMS != nil || h.Error != "HDRezka asks for a browser check" {
		t.Errorf("hdrezka = %+v", r.HDRezka)
	}
}

// ai.mtd_cost_usd is this month's AI cost, rounded to a hundredth of a
// cent; null when it can't be read.
func TestReport_AICost(t *testing.T) {
	r := NewService(Options{DB: &fakeDB{}, AIConfigured: true, AICost: func(context.Context) (float64, error) {
		return 0.0123456, nil
	}}).Report(context.Background(), false)
	if r.AI.MTDCostUSD == nil || *r.AI.MTDCostUSD != 0.0123 {
		t.Errorf("ai = %+v", r.AI)
	}

	r = NewService(Options{DB: &fakeDB{}, AIConfigured: true, AICost: func(context.Context) (float64, error) {
		return 0, errors.New("db down")
	}}).Report(context.Background(), false)
	if r.AI.MTDCostUSD != nil {
		t.Errorf("a failed query = %v, want null", *r.AI.MTDCostUSD)
	}
}

func TestReport_Unconfigured(t *testing.T) {
	svc := NewService(Options{
		Version: "dev",
		DB:      &fakeDB{},
		WebDAV:  WebDAVInfo{Port: 36911, AuthEnabled: false, Username: "ignored"},
	})

	r := svc.Report(context.Background(), false)

	if r.Jackett.Configured || r.Jackett.Reachable || r.Jackett.LatencyMS != nil {
		t.Errorf("jackett = %+v, want unconfigured", r.Jackett)
	}
	if r.TMDB.Configured || r.TMDB.Reachable || r.TMDB.LatencyMS != nil {
		t.Errorf("tmdb = %+v, want unconfigured", r.TMDB)
	}
	if r.HDRezka.Configured || r.HDRezka.Reachable || r.HDRezka.LatencyMS != nil {
		t.Errorf("hdrezka = %+v, want unconfigured", r.HDRezka)
	}
	if r.Trakt.Configured || r.AI.Configured || r.OpenSubtitles.Configured {
		t.Errorf("trakt/ai/opensubtitles should be unconfigured: %+v %+v %+v", r.Trakt, r.AI, r.OpenSubtitles)
	}
	if r.WebDAV.URL != "http://localhost:36911/" {
		t.Errorf("webdav.url = %q, want localhost fallback", r.WebDAV.URL)
	}
	if r.WebDAV.Username != "" {
		t.Errorf("username must be omitted when auth is disabled, got %q", r.WebDAV.Username)
	}
	if r.WebDAV.ActiveReaders != nil {
		t.Errorf("active_readers = %v, want nil without a tracker", *r.WebDAV.ActiveReaders)
	}

	// JSON shape: omitted and null fields.
	raw, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil {
		t.Fatal(err)
	}
	section := func(name string) map[string]any {
		var m map[string]any
		if err := json.Unmarshal(top[name], &m); err != nil {
			t.Fatalf("section %s: %v", name, err)
		}
		return m
	}
	if _, ok := section("jackett")["latency_ms"]; ok {
		t.Error("jackett.latency_ms should be omitted when not probed")
	}
	if _, ok := section("jackett")["indexers"]; ok {
		t.Error("jackett.indexers should be omitted when Jackett isn't configured")
	}
	if h := section("hdrezka"); h["configured"] != false || h["reachable"] != false {
		t.Errorf("hdrezka = %v", h)
	}
	if v, ok := section("ai")["mtd_cost_usd"]; !ok || v != nil {
		t.Errorf("ai.mtd_cost_usd should be present and null, got %v (present %v)", v, ok)
	}
	if _, ok := section("webdav")["username"]; ok {
		t.Error("webdav.username should be omitted when auth is disabled")
	}
}

// jackett.indexers matches PLAN §6 "System": present only with a
// reachable Jackett; ms and since are null when unknown, error only when
// failing.
func TestReport_JackettIndexersJSON(t *testing.T) {
	ms, since := int64(8000), "2026-10-16T10:00:00Z"
	tests := []struct {
		name  string
		probe JackettProbe
		want  string // the jackett section without latency_ms; "" skips the comparison
	}{
		{
			name: "reachable",
			probe: jackettProbe(func(context.Context) error { return nil },
				IndexerStatus{Name: "The Pirate Bay", Status: "unknown"},
				IndexerStatus{Name: "1337x", Status: "timeout", MS: &ms, Since: &since, Error: "timed out after 8s"}),
			want: `{"configured":true,"reachable":true,"indexers":[` +
				`{"name":"The Pirate Bay","status":"unknown","ms":null,"since":null},` +
				`{"name":"1337x","status":"timeout","ms":8000,"since":"2026-10-16T10:00:00Z","error":"timed out after 8s"}]}`,
		},
		{
			name:  "reachable with no indexers",
			probe: jackettProbe(func(context.Context) error { return nil }),
			want:  `{"configured":true,"reachable":true,"indexers":[]}`,
		},
		{
			name:  "unreachable",
			probe: jackettProbe(func(context.Context) error { return errors.New("jackett error: Invalid API Key") }),
			want:  `{"configured":true,"reachable":false,"error":"jackett error: Invalid API Key"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewService(Options{DB: &fakeDB{}, Jackett: tt.probe}).Report(context.Background(), false)
			r.Jackett.LatencyMS = nil
			raw, err := json.Marshal(r)
			if err != nil {
				t.Fatal(err)
			}
			var top map[string]json.RawMessage
			if err := json.Unmarshal(raw, &top); err != nil {
				t.Fatal(err)
			}
			if got := string(top["jackett"]); got != tt.want {
				t.Errorf("jackett =\n%s\nwant\n%s", got, tt.want)
			}
		})
	}
}

func TestReport_DBFailures(t *testing.T) {
	tests := []struct {
		name      string
		db        *fakeDB
		wantOK    bool
		wantSize  bool
		wantError string
	}{
		{"ping fails", &fakeDB{pingErr: errors.New("dial tcp postgres:5432: password=hunter2")}, false, false, "database unreachable"},
		{"ping times out", &fakeDB{pingErr: context.DeadlineExceeded}, false, false, "timed out"},
		{"size fails", &fakeDB{sizeErr: errors.New("boom")}, true, false, "size query failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewService(Options{DB: tt.db}).Report(context.Background(), false)
			if r.DB.OK != tt.wantOK {
				t.Errorf("ok = %v, want %v", r.DB.OK, tt.wantOK)
			}
			if (r.DB.SizeBytes != nil) != tt.wantSize {
				t.Errorf("size_bytes = %v", r.DB.SizeBytes)
			}
			if !strings.Contains(r.DB.Error, tt.wantError) {
				t.Errorf("error = %q, want it to contain %q", r.DB.Error, tt.wantError)
			}
			if strings.Contains(r.DB.Error, "hunter2") {
				t.Errorf("driver error leaked into the report: %q", r.DB.Error)
			}
		})
	}
}

func TestReport_ProbesTimeOutConcurrently(t *testing.T) {
	blocked := func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	}
	const timeout = 100 * time.Millisecond
	svc := NewService(Options{
		DB:           &fakeDB{block: true},
		Jackett:      jackettProbe(blocked),
		TMDB:         blocked,
		ProbeTimeout: timeout,
	})

	start := time.Now()
	r := svc.Report(context.Background(), false)
	elapsed := time.Since(start)

	for name, p := range map[string]ProbeStatus{"jackett": r.Jackett.ProbeStatus, "tmdb": r.TMDB} {
		if !p.Configured || p.Reachable || !strings.Contains(p.Error, "timed out") {
			t.Errorf("%s = %+v, want a timeout error", name, p)
		}
	}
	if r.DB.OK || !strings.Contains(r.DB.Error, "timed out") {
		t.Errorf("db = %+v, want a timeout error", r.DB)
	}
	// Run in parallel: three blocked checks take about one timeout, not the
	// three of a serial run. The bound leaves a full timeout of slack for
	// -race and loaded CI.
	if elapsed >= 2*timeout {
		t.Errorf("checks took %s, want < %s (checks should run concurrently)", elapsed, 2*timeout)
	}
}

func TestReport_CachingAndRefreshThrottle(t *testing.T) {
	clock := newFakeClock()
	db := &fakeDB{}
	var jackettCalls atomic.Int32
	svc := NewService(Options{
		DB:      db,
		Jackett: jackettProbe(countingProber(nil, &jackettCalls)),
		Now:     clock.Now,
	})
	ctx := context.Background()

	check := func(name string, refresh bool, want int32) {
		t.Helper()
		svc.Report(ctx, refresh)
		if got := jackettCalls.Load(); got != want {
			t.Fatalf("%s: probe calls = %d, want %d", name, got, want)
		}
		if got := db.pings.Load(); got != want {
			t.Fatalf("%s: db pings = %d, want %d", name, got, want)
		}
	}

	check("first call runs the checks", false, 1)

	clock.Advance(2 * time.Second)
	check("cached within TTL", false, 1)
	check("refresh 2s after a check is throttled", true, 1)

	clock.Advance(4 * time.Second) // report is now 6s old
	check("refresh 6s after a check bypasses the cache", true, 2)
	check("immediate second refresh is throttled", true, 2)

	clock.Advance(CacheTTL - time.Second)
	check("cached just under TTL", false, 2)

	clock.Advance(time.Second)
	check("cache expires at TTL", false, 3)
}

func TestReport_DBDownCachedBriefly(t *testing.T) {
	clock := newFakeClock()
	db := &fakeDB{pingErr: errors.New("connection refused")}
	svc := NewService(Options{DB: db, Now: clock.Now})
	ctx := context.Background()

	if r := svc.Report(ctx, false); r.DB.OK {
		t.Fatalf("db = %+v, want down", r.DB)
	}

	clock.Advance(RefreshMinInterval - time.Second)
	svc.Report(ctx, false)
	if got := db.pings.Load(); got != 1 {
		t.Fatalf("db pings = %d, want 1 (a fresh outage report is still cached)", got)
	}

	// The database comes back: it shows after RefreshMinInterval, not CacheTTL.
	db.pingErr = nil
	clock.Advance(time.Second)
	if r := svc.Report(ctx, false); !r.DB.OK {
		t.Fatalf("db = %+v, want recovered after %s", r.DB, RefreshMinInterval)
	}

	// A healthy report goes back to the normal TTL.
	clock.Advance(RefreshMinInterval)
	svc.Report(ctx, false)
	if got := db.pings.Load(); got != 2 {
		t.Errorf("db pings = %d, want 2 (healthy report cached for CacheTTL)", got)
	}
}

func TestReport_LiveFieldsNotCached(t *testing.T) {
	clock := newFakeClock()
	readers := 1
	svc := NewService(Options{
		DB:            &fakeDB{},
		StartedAt:     clock.Now(),
		ActiveReaders: func() int { return readers },
		Now:           clock.Now,
	})

	first := svc.Report(context.Background(), false)
	clock.Advance(10 * time.Second)
	readers = 3
	second := svc.Report(context.Background(), false)

	if first.CheckedAt != second.CheckedAt {
		t.Errorf("checked_at changed within TTL: %q → %q", first.CheckedAt, second.CheckedAt)
	}
	if second.UptimeSeconds != 10 {
		t.Errorf("uptime_seconds = %d, want 10 (live)", second.UptimeSeconds)
	}
	if *second.WebDAV.ActiveReaders != 3 {
		t.Errorf("active_readers = %d, want 3 (live)", *second.WebDAV.ActiveReaders)
	}
}

func TestReport_ConcurrentCallersShareOneCheck(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{}, 1)
	var calls atomic.Int32
	slow := func(ctx context.Context) error {
		calls.Add(1)
		select {
		case started <- struct{}{}:
		default:
		}
		<-release
		return nil
	}
	svc := NewService(Options{DB: &fakeDB{}, Jackett: jackettProbe(slow)})

	const n = 20
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); svc.Report(context.Background(), false) }()
	<-started // the first check is in flight

	for i := 0; i < n-1; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); svc.Report(context.Background(), true) }()
	}
	time.Sleep(20 * time.Millisecond) // let the others reach singleflight
	close(release)
	wg.Wait()

	if got := calls.Load(); got != 1 {
		t.Errorf("probe ran %d times for %d concurrent callers, want 1", got, n)
	}
}

func TestReport_CallerCancellationDoesNotAbortSharedCheck(t *testing.T) {
	var sawCancel atomic.Bool
	probe := func(ctx context.Context) error {
		time.Sleep(10 * time.Millisecond)
		if ctx.Err() != nil && !errors.Is(ctx.Err(), context.DeadlineExceeded) {
			sawCancel.Store(true)
		}
		return ctx.Err()
	}
	svc := NewService(Options{DB: &fakeDB{}, TMDB: probe})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r := svc.Report(ctx, false)

	if sawCancel.Load() || !r.TMDB.Reachable {
		t.Errorf("caller cancellation leaked into the shared check: %+v", r.TMDB)
	}
}
