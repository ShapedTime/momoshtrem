package artwork

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"math"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func solid(w, h int, c color.Color) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, c)
		}
	}
	return img
}

func TestDominantColor(t *testing.T) {
	grey := color.RGBA{128, 128, 128, 255}

	// A grey frame with a saturated blue band across a quarter of it.
	band := solid(300, 169, grey)
	for y := 0; y < 169; y++ {
		for x := 0; x < 75; x++ {
			band.Set(x, y, color.RGBA{20, 40, 220, 255})
		}
	}

	// Mostly black with faint red noise: chroma weighting must not let the
	// near-black pixels swamp the image (HSL saturation would call them
	// fully saturated).
	noisy := solid(320, 180, color.RGBA{0, 0, 0, 255})
	for y := 0; y < 180; y += 2 {
		for x := 0; x < 320; x += 2 {
			noisy.Set(x, y, color.RGBA{6, 0, 0, 255})
		}
	}
	for y := 0; y < 180; y++ {
		for x := 160; x < 320; x++ {
			noisy.Set(x, y, color.RGBA{30, 160, 60, 255})
		}
	}

	tests := []struct {
		name string
		img  image.Image
		want string
	}{
		{"pure red stays red", solid(300, 169, color.RGBA{255, 0, 0, 255}), "#ff0000"},
		{"dark navy is lifted to lightness 0.30", solid(300, 169, color.RGBA{0, 0, 51, 255}), "#000099"},
		{"pale pink is darkened to lightness 0.50", solid(300, 169, color.RGBA{255, 200, 200, 255}), "#ff0000"},
		{"white gets the minimum saturation", solid(300, 169, color.White), "#9f6060"},
		{"grey frame takes the saturated band's hue", band, "#1428dc"},
		{"green half wins over black noise", noisy, "#1ea03c"},
		{"empty image", image.NewRGBA(image.Rect(0, 0, 0, 0)), "#603939"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DominantColor(tt.img); got != tt.want {
				t.Errorf("DominantColor = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestHSLRoundTrip(t *testing.T) {
	for _, c := range [][3]float64{{1, 0, 0}, {0.2, 0.4, 0.6}, {0.9, 0.9, 0.1}, {0.5, 0.5, 0.5}, {0, 0, 0}} {
		h, s, l := rgbToHSL(c[0], c[1], c[2])
		r, g, b := hslToRGB(h, s, l)
		if math.Abs(r-c[0]) > 1e-9 || math.Abs(g-c[1]) > 1e-9 || math.Abs(b-c[2]) > 1e-9 {
			t.Errorf("round trip %v -> (%v %v %v) -> (%v %v %v)", c, h, s, l, r, g, b)
		}
	}
}

// memCache is an in-memory Cache.
type memCache struct {
	mu      sync.Mutex
	entries map[string]memEntry
	gets    atomic.Int32
}

type memEntry struct {
	payload []byte
	expires time.Time
}

func newMemCache() *memCache { return &memCache{entries: map[string]memEntry{}} }

func (m *memCache) Get(_ context.Context, key string) ([]byte, time.Time, bool, error) {
	m.gets.Add(1)
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.entries[key]
	return e.payload, e.expires, ok, nil
}

func (m *memCache) Set(_ context.Context, key string, payload []byte, expires time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries[key] = memEntry{payload: append([]byte(nil), payload...), expires: expires}
	return nil
}

func (m *memCache) entry(key string) (memEntry, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.entries[key]
	return e, ok
}

func jpegBytes(t *testing.T, c color.Color) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, solid(300, 169, c), &jpeg.Options{Quality: 95}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// imageServer serves a JPEG at /w300/good.jpg after delay, 404 elsewhere,
// and counts requests.
type imageServer struct {
	*httptest.Server
	hits  atomic.Int32
	delay time.Duration
}

func newImageServer(t *testing.T, body []byte, delay time.Duration) *imageServer {
	s := &imageServer{delay: delay}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.hits.Add(1)
		if s.delay > 0 {
			select {
			case <-time.After(s.delay):
			case <-r.Context().Done():
				return
			}
		}
		if r.URL.Path != "/w300/good.jpg" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write(body)
	}))
	t.Cleanup(s.Close)
	return s
}

func TestExtractor_ComputesThenCaches(t *testing.T) {
	srv := newImageServer(t, jpegBytes(t, color.RGBA{0, 0, 200, 255}), 0)
	cache := newMemCache()
	e := New(cache, WithBaseURL(srv.URL+"/w300"))
	defer e.Close()

	first := e.Color(context.Background(), "/good.jpg")
	if first == nil || *first != "#0100c8" {
		t.Fatalf("first Color = %s, want #0100c8", deref(first))
	}
	entry, ok := cache.entry("color:w300/good.jpg")
	if !ok || !entry.expires.IsZero() || string(entry.payload) != `"#0100c8"` {
		t.Errorf("cache entry = %+v (found %v), want the colour with no expiry", entry, ok)
	}

	second := e.Color(context.Background(), "/good.jpg")
	if second == nil || *second != *first {
		t.Errorf("second Color = %s, want %s", deref(second), *first)
	}
	if hits := srv.hits.Load(); hits != 1 {
		t.Errorf("image fetched %d times, want 1", hits)
	}
	if e.Color(context.Background(), "") != nil {
		t.Error("no backdrop should have no colour")
	}
}

func TestExtractor_BudgetThenBackground(t *testing.T) {
	srv := newImageServer(t, jpegBytes(t, color.RGBA{200, 0, 0, 255}), 150*time.Millisecond)
	cache := newMemCache()
	e := New(cache, WithBaseURL(srv.URL+"/w300"), WithBudget(20*time.Millisecond))
	defer e.Close()

	// Concurrent callers past the budget all get null and share one fetch.
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if got := e.Color(context.Background(), "/good.jpg"); got != nil {
				t.Errorf("Color within budget = %s, want nil", *got)
			}
		}()
	}
	wg.Wait()

	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, ok := cache.entry("color:w300/good.jpg"); ok {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("background computation never stored the colour")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if got := e.Color(context.Background(), "/good.jpg"); got == nil || *got != "#c80100" {
		t.Errorf("Color after background = %s, want #c80100", deref(got))
	}
	if hits := srv.hits.Load(); hits != 1 {
		t.Errorf("image fetched %d times, want 1 (one flight per key)", hits)
	}
}

func TestExtractor_FailureIsRememberedBriefly(t *testing.T) {
	srv := newImageServer(t, nil, 0)
	cache := newMemCache()
	now := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	e := New(cache, WithBaseURL(srv.URL+"/w300"))
	e.now = func() time.Time { return now }
	defer e.Close()

	if got := e.Color(context.Background(), "/missing.jpg"); got != nil {
		t.Fatalf("Color of a 404 = %s, want nil", *got)
	}
	entry, ok := cache.entry("color:w300/missing.jpg")
	if !ok || string(entry.payload) != "null" || !entry.expires.Equal(now.Add(failureTTL)) {
		t.Fatalf("failure entry = %+v (found %v), want null until %v", entry, ok, now.Add(failureTTL))
	}

	_ = e.Color(context.Background(), "/missing.jpg")
	if hits := srv.hits.Load(); hits != 1 {
		t.Errorf("fetched %d times within the failure TTL, want 1", hits)
	}
	if e.hostDown() {
		t.Error("a 404 is an answer: it must not mark the host down")
	}

	now = now.Add(failureTTL + time.Minute)
	_ = e.Color(context.Background(), "/missing.jpg")
	if hits := srv.hits.Load(); hits != 2 {
		t.Errorf("fetched %d times after the failure TTL, want 2", hits)
	}
}

func TestExtractor_CloseStopsBackgroundWork(t *testing.T) {
	srv := newImageServer(t, jpegBytes(t, color.White), time.Minute)
	cache := newMemCache()
	e := New(cache, WithBaseURL(srv.URL+"/w300"), WithBudget(10*time.Millisecond))

	if got := e.Color(context.Background(), "/good.jpg"); got != nil {
		t.Fatalf("Color = %s, want nil", *got)
	}

	done := make(chan struct{})
	go func() {
		e.Close()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Close didn't stop the background fetch")
	}
	if _, ok := cache.entry("color:w300/good.jpg"); ok {
		t.Error("a fetch cancelled by shutdown must not be cached as a failure")
	}
	if got := e.Color(context.Background(), "/good.jpg"); got != nil {
		t.Errorf("Color after Close = %s, want nil", *got)
	}
}

// fakeClock is a settable clock, safe across goroutines.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func (e *Extractor) hostDown() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.now().Before(e.downUntil)
}

func (e *Extractor) probeRunning() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.probing
}

// timed returns Color's answer and how long it took.
func timed(e *Extractor, ctx context.Context, path string) (*string, time.Duration) {
	start := time.Now()
	got := e.Color(ctx, path)
	return got, time.Since(start)
}

// A caller that joins a flight already running waits only what's left of
// the budget, and not at all once it's spent.
func TestExtractor_JoinedFlightWaitsOnlyTheRest(t *testing.T) {
	srv := newImageServer(t, jpegBytes(t, color.White), time.Minute) // never in time
	clock := &fakeClock{now: time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)}
	e := New(newMemCache(), WithBaseURL(srv.URL+"/w300"), WithBudget(time.Second))
	e.now = clock.Now
	defer e.Close()

	// The first caller's request ends early; its flight carries on.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if got := e.Color(ctx, "/good.jpg"); got != nil {
		t.Fatalf("first Color = %s, want nil", *got)
	}

	clock.Advance(900 * time.Millisecond)
	got, took := timed(e, context.Background(), "/good.jpg")
	if got != nil || took < 90*time.Millisecond || took > 700*time.Millisecond {
		t.Errorf("joined 900 ms in: %v after %v, want nil after ~100 ms", deref(got), took)
	}

	clock.Advance(time.Second)
	got, took = timed(e, context.Background(), "/good.jpg")
	if got != nil || took > 50*time.Millisecond {
		t.Errorf("joined past the budget: %v after %v, want nil at once", deref(got), took)
	}
	if hits := srv.hits.Load(); hits != 1 {
		t.Errorf("image requested %d times, want 1 (one flight)", hits)
	}
}

// After a transport failure Color stops waiting for the host: it answers
// null at once and sends one probe at a time until the host answers.
func TestExtractor_HostDown(t *testing.T) {
	body := jpegBytes(t, color.RGBA{0, 0, 200, 255})
	var stalled atomic.Bool
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if stalled.Load() {
			<-r.Context().Done() // the client times out
			return
		}
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	cache := newMemCache()
	clock := &fakeClock{now: time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)}
	e := New(cache, WithBaseURL(srv.URL), WithBudget(2*time.Second),
		WithHTTPClient(&http.Client{Timeout: 50 * time.Millisecond}))
	e.now = clock.Now
	defer e.Close()
	waitForProbe := func() {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for e.probeRunning() {
			if time.Now().After(deadline) {
				t.Fatal("probe never finished")
			}
			time.Sleep(5 * time.Millisecond)
		}
	}

	stalled.Store(true)
	if got := e.Color(context.Background(), "/a.jpg"); got != nil {
		t.Fatalf("Color with the host stalled = %s, want nil", *got)
	}
	if !e.hostDown() {
		t.Fatal("a timeout should mark the host down")
	}
	if entry, ok := cache.entry("color:w300/a.jpg"); !ok || string(entry.payload) != "null" {
		t.Errorf("failure entry = %+v (found %v), want null", entry, ok)
	}

	// Down: no waiting, and one probe however many titles ask.
	before := hits.Load()
	for _, p := range []string{"/b.jpg", "/c.jpg", "/d.jpg"} {
		if got, took := timed(e, context.Background(), p); got != nil || took > 30*time.Millisecond {
			t.Errorf("Color(%s) while down = %v after %v, want nil at once", p, deref(got), took)
		}
	}
	waitForProbe()
	if n := hits.Load() - before; n != 1 {
		t.Errorf("requests while down = %d, want 1 probe", n)
	}
	if entry, ok := cache.entry("color:w300/b.jpg"); !ok || string(entry.payload) != "null" {
		t.Errorf("failed probe entry = %+v (found %v), want null", entry, ok)
	}
	if !e.hostDown() {
		t.Error("a failed probe keeps the host down")
	}

	// The host is back: the next probe succeeds and lifts the breaker.
	stalled.Store(false)
	if got := e.Color(context.Background(), "/c.jpg"); got != nil {
		t.Errorf("Color while still down = %s, want nil", *got)
	}
	waitForProbe()
	if e.hostDown() {
		t.Fatal("a successful probe should mark the host up")
	}
	if got := e.Color(context.Background(), "/c.jpg"); got == nil || *got != "#0100c8" {
		t.Errorf("probed colour = %v, want #0100c8 from the cache", deref(got))
	}
	if got := e.Color(context.Background(), "/d.jpg"); got == nil || *got != "#0100c8" {
		t.Errorf("Color once up = %v, want it computed inline", deref(got))
	}

	// Without a successful probe the breaker still lifts after 5 minutes.
	stalled.Store(true)
	_ = e.Color(context.Background(), "/e.jpg")
	if !e.hostDown() {
		t.Fatal("host should be down again")
	}
	clock.Advance(hostDownFor + time.Second)
	if e.hostDown() {
		t.Error("the breaker should lift after hostDownFor")
	}
}

func deref(s *string) string {
	if s == nil {
		return "<nil>"
	}
	return *s
}
