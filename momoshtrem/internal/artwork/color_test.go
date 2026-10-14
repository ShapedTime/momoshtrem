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

func deref(s *string) string {
	if s == nil {
		return "<nil>"
	}
	return *s
}
