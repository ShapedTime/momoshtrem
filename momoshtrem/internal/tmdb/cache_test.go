package tmdb

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// memCache is an in-memory Cache.
type memCache struct {
	mu      sync.Mutex
	entries map[string]memEntry
	getErr  error
}

type memEntry struct {
	payload   []byte
	expiresAt time.Time
}

func newMemCache() *memCache { return &memCache{entries: map[string]memEntry{}} }

func (m *memCache) Get(_ context.Context, key string) ([]byte, time.Time, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.getErr != nil {
		return nil, time.Time{}, false, m.getErr
	}
	e, ok := m.entries[key]
	return e.payload, e.expiresAt, ok, nil
}

func (m *memCache) Set(_ context.Context, key string, payload []byte, expiresAt time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries[key] = memEntry{payload: append([]byte(nil), payload...), expiresAt: expiresAt}
	return nil
}

func (m *memCache) entry(key string) (memEntry, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.entries[key]
	return e, ok
}

// upstream is a TMDB stand-in whose status and body can change mid-test.
type upstream struct {
	srv    *httptest.Server
	calls  atomic.Int32
	mu     sync.Mutex
	status int
	body   string
	delay  time.Duration
}

func newUpstream(t *testing.T) *upstream {
	t.Helper()
	u := &upstream{status: http.StatusOK, body: `{"id": 1, "name": "v1", "status": "Ended"}`}
	u.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u.calls.Add(1)
		u.mu.Lock()
		status, body, delay := u.status, u.body, u.delay
		u.mu.Unlock()
		time.Sleep(delay)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(u.srv.Close)
	return u
}

func (u *upstream) set(status int, body string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.status, u.body = status, body
}

// newTestCached returns a Cached over u with a controllable clock.
func newTestCached(u *upstream, cache Cache) (*Cached, *time.Time) {
	client := NewClient(testKey, WithBaseURL(u.srv.URL), WithLanguage("en-US"), WithRetries(1, 0))
	c := NewCached(client, cache)
	now := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	c.now = func() time.Time { return now }
	return c, &now
}

func TestSeasonTTL(t *testing.T) {
	tests := []struct {
		status string
		want   time.Duration
	}{
		{"Returning Series", SeasonTTLAiring},
		{"In Production", SeasonTTLAiring},
		{"Planned", SeasonTTLAiring},
		{"", SeasonTTLAiring},
		{"Ended", SeasonTTLEnded},
		{"Canceled", SeasonTTLEnded},
		{"Cancelled", SeasonTTLEnded},
	}
	for _, tt := range tests {
		if got := SeasonTTL(tt.status); got != tt.want {
			t.Errorf("SeasonTTL(%q) = %v, want %v", tt.status, got, tt.want)
		}
	}
	if SeasonTTLAiring != 24*time.Hour || SeasonTTLEnded != 30*24*time.Hour || DetailsTTL != 7*24*time.Hour {
		t.Errorf("TTLs changed: airing %v, ended %v, details %v", SeasonTTLAiring, SeasonTTLEnded, DetailsTTL)
	}
}

func TestCached_FreshHitSkipsUpstream(t *testing.T) {
	u := newUpstream(t)
	cache := newMemCache()
	c, now := newTestCached(u, cache)
	ctx := context.Background()

	first, err := c.ShowDetails(ctx, 1)
	if err != nil {
		t.Fatalf("first ShowDetails: %v", err)
	}
	e, ok := cache.entry("tmdb:v1:en-US:tv/1")
	if !ok {
		t.Fatalf("nothing cached; entries: %v", cache.entries)
	}
	if want := now.Add(DetailsTTL); !e.expiresAt.Equal(want) {
		t.Errorf("expires at %v, want %v", e.expiresAt, want)
	}

	u.set(http.StatusOK, `{"id": 1, "name": "v2"}`)
	*now = now.Add(DetailsTTL - time.Minute)
	second, err := c.ShowDetails(ctx, 1)
	if err != nil {
		t.Fatalf("second ShowDetails: %v", err)
	}
	if u.calls.Load() != 1 || second.Name != first.Name {
		t.Errorf("fresh entry: %d requests, name %q; want 1 request, %q", u.calls.Load(), second.Name, first.Name)
	}

	// Each caller gets its own copy.
	second.Name = "mutated"
	third, _ := c.ShowDetails(ctx, 1)
	if third.Name != "v1" {
		t.Errorf("cached value shared between callers: %q", third.Name)
	}
}

func TestCached_ExpiredEntryRefetches(t *testing.T) {
	u := newUpstream(t)
	c, now := newTestCached(u, newMemCache())
	ctx := context.Background()

	if _, err := c.ShowDetails(ctx, 1); err != nil {
		t.Fatal(err)
	}
	u.set(http.StatusOK, `{"id": 1, "name": "v2"}`)
	*now = now.Add(DetailsTTL + time.Second)

	got, err := c.ShowDetails(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "v2" || u.calls.Load() != 2 {
		t.Errorf("expired entry: name %q after %d requests, want v2 after 2", got.Name, u.calls.Load())
	}
}

func TestCached_StaleOnError(t *testing.T) {
	tests := []struct {
		name      string
		status    int
		wantStale bool
	}{
		{"server error serves stale", http.StatusInternalServerError, true},
		{"rate limited serves stale", http.StatusTooManyRequests, true},
		{"not found is not hidden", http.StatusNotFound, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := newUpstream(t)
			c, now := newTestCached(u, newMemCache())
			ctx := context.Background()

			if _, err := c.MovieDetails(ctx, 1); err != nil {
				t.Fatal(err)
			}
			*now = now.Add(DetailsTTL + time.Hour)
			u.set(tt.status, `{}`)

			got, err := c.MovieDetails(ctx, 1)
			if tt.wantStale {
				if err != nil || got.ID != 1 {
					t.Fatalf("want the stale entry, got (%v, %v)", got, err)
				}
				return
			}
			if !errors.Is(err, ErrNotFound) {
				t.Fatalf("error = %v, want ErrNotFound", err)
			}
		})
	}
}

func TestCached_NoCacheNoStale(t *testing.T) {
	u := newUpstream(t)
	u.set(http.StatusBadGateway, `{}`)
	c, _ := newTestCached(u, newMemCache())

	var status *StatusError
	if _, err := c.MovieDetails(context.Background(), 1); !errors.As(err, &status) || status.StatusCode != 502 {
		t.Fatalf("error = %v, want a 502 StatusError", err)
	}
}

func TestCached_Bypass(t *testing.T) {
	u := newUpstream(t)
	c, _ := newTestCached(u, newMemCache())
	ctx := context.Background()

	if _, err := c.ShowDetails(ctx, 1); err != nil {
		t.Fatal(err)
	}

	u.set(http.StatusOK, `{"id": 1, "name": "fresh"}`)
	got, err := c.ShowDetails(Bypass(ctx), 1)
	if err != nil || got.Name != "fresh" {
		t.Fatalf("bypass = (%v, %v), want the fresh value", got, err)
	}
	// The bypass result is cached for everyone else.
	if got, _ := c.ShowDetails(ctx, 1); got.Name != "fresh" || u.calls.Load() != 2 {
		t.Errorf("after bypass: %q after %d requests, want fresh after 2", got.Name, u.calls.Load())
	}

	// A refresh surfaces errors instead of hiding them behind the cache.
	u.set(http.StatusInternalServerError, `{}`)
	if _, err := c.ShowDetails(Bypass(ctx), 1); err == nil {
		t.Error("bypass with TMDB down: want an error")
	}
}

func TestCached_SingleflightSharesOneRequest(t *testing.T) {
	u := newUpstream(t)
	u.delay = 100 * time.Millisecond
	c, _ := newTestCached(u, newMemCache())

	var wg sync.WaitGroup
	errs := make(chan error, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := c.Season(context.Background(), 1, 2, "Ended")
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("Season: %v", err)
		}
	}
	if got := u.calls.Load(); got != 1 {
		t.Errorf("requests = %d, want 1", got)
	}
}

func TestCached_SeasonTTLFollowsStatus(t *testing.T) {
	u := newUpstream(t)
	cache := newMemCache()
	c, now := newTestCached(u, cache)
	ctx := context.Background()

	if _, err := c.Season(ctx, 7, 0, "Ended"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Season(ctx, 7, 1, "Returning Series"); err != nil {
		t.Fatal(err)
	}
	ended, _ := cache.entry("tmdb:v1:en-US:tv/7/season/0")
	airing, _ := cache.entry("tmdb:v1:en-US:tv/7/season/1")
	if !ended.expiresAt.Equal(now.Add(SeasonTTLEnded)) || !airing.expiresAt.Equal(now.Add(SeasonTTLAiring)) {
		t.Errorf("expiry: ended %v, airing %v", ended.expiresAt, airing.expiresAt)
	}
}

func TestCached_EpisodeGroups(t *testing.T) {
	u := newUpstream(t)
	u.set(http.StatusOK, `{"results": [{"id": "abc", "name": "Absolute", "type": 2, "episode_count": 3}]}`)
	cache := newMemCache()
	c, _ := newTestCached(u, cache)

	for i := 0; i < 2; i++ {
		groups, err := c.EpisodeGroups(context.Background(), 5)
		if err != nil || len(groups) != 1 || groups[0].ID != "abc" {
			t.Fatalf("EpisodeGroups = (%v, %v)", groups, err)
		}
	}
	if u.calls.Load() != 1 {
		t.Errorf("requests = %d, want 1", u.calls.Load())
	}
	if _, ok := cache.entry("tmdb:v1:en-US:tv/5/episode_groups"); !ok {
		t.Error("episode groups not cached")
	}
}

func TestCached_CacheReadErrorFallsThrough(t *testing.T) {
	u := newUpstream(t)
	cache := newMemCache()
	cache.getErr = errors.New("db down")
	c, _ := newTestCached(u, cache)

	if got, err := c.MovieDetails(context.Background(), 1); err != nil || got.ID != 1 {
		t.Fatalf("MovieDetails = (%v, %v), want the upstream value", got, err)
	}
}

func TestCached_UndecodableEntryRefetches(t *testing.T) {
	u := newUpstream(t)
	cache := newMemCache()
	_ = cache.Set(context.Background(), "tmdb:v1:en-US:movie/1", []byte(`{"id": "not a number"}`), time.Time{})
	c, _ := newTestCached(u, cache)

	if got, err := c.MovieDetails(context.Background(), 1); err != nil || got.ID != 1 {
		t.Fatalf("MovieDetails = (%v, %v), want the upstream value", got, err)
	}
}
