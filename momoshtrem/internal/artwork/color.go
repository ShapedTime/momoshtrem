// Package artwork derives values from TMDB artwork, such as the ambient
// colour behind a title's hero.
package artwork

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg" // TMDB serves JPEG backdrops
	_ "image/png"  // and the odd PNG
	"io"
	"log/slog"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"
)

// DefaultBaseURL serves TMDB backdrops 300 px wide: small enough to fetch
// inline, big enough to sample.
const DefaultBaseURL = "https://image.tmdb.org/t/p/w300"

const (
	// defaultBudget is how long Color waits inline before answering null and
	// letting the computation finish in the background.
	defaultBudget = 1500 * time.Millisecond
	// computeTimeout bounds one background fetch, decode and store.
	computeTimeout = 15 * time.Second
	// failureTTL is how long a failed fetch is remembered before retrying.
	failureTTL = time.Hour
	// maxImageBytes guards against a runaway response.
	maxImageBytes = 4 << 20

	// The sampling grid: 32×18 points, the backdrop's 16:9.
	gridW, gridH = 32, 18
)

// Cache stores JSON payloads by key; a zero expiresAt never expires.
// library.TMDBCacheRepository implements it.
type Cache interface {
	Get(ctx context.Context, key string) (payload []byte, expiresAt time.Time, found bool, err error)
	Set(ctx context.Context, key string, payload []byte, expiresAt time.Time) error
}

// Option configures an Extractor.
type Option func(*Extractor)

// WithBaseURL fetches images from another server (tests use httptest).
func WithBaseURL(u string) Option {
	return func(e *Extractor) { e.baseURL = strings.TrimRight(u, "/") }
}

// WithHTTPClient replaces the default HTTP client.
func WithHTTPClient(c *http.Client) Option {
	return func(e *Extractor) { e.http = c }
}

// WithBudget sets how long Color waits inline.
func WithBudget(d time.Duration) Option {
	return func(e *Extractor) { e.budget = d }
}

// Extractor computes a title's hero colour from its backdrop and caches it
// forever under "color:w300{path}". A computation that outlives the inline
// budget finishes in the background (one per key); Close stops them.
type Extractor struct {
	cache   Cache
	http    *http.Client
	baseURL string
	budget  time.Duration
	now     func() time.Time
	log     *slog.Logger

	ctx    context.Context // Background work; cancelled by Close
	cancel context.CancelFunc
	wg     sync.WaitGroup

	mu      sync.Mutex
	closed  bool
	flights map[string]*flight
}

// flight is one computation in progress; done closes when hex is set.
type flight struct {
	done chan struct{}
	hex  *string
}

// New creates an Extractor backed by cache.
func New(cache Cache, opts ...Option) *Extractor {
	ctx, cancel := context.WithCancel(context.Background())
	e := &Extractor{
		cache:   cache,
		http:    &http.Client{Timeout: 10 * time.Second},
		baseURL: DefaultBaseURL,
		budget:  defaultBudget,
		now:     time.Now,
		log:     slog.With("component", "artwork"),
		ctx:     ctx,
		cancel:  cancel,
		flights: make(map[string]*flight),
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// CacheKey is the tmdb_cache key of a backdrop's colour.
func CacheKey(backdropPath string) string {
	return "color:w300" + backdropPath
}

// Color returns the hero colour of a TMDB backdrop ("/abc.jpg") as #rrggbb,
// or nil when there's no backdrop, the image can't be read, or the colour
// isn't ready within the inline budget (the next request gets it).
func (e *Extractor) Color(ctx context.Context, backdropPath string) *string {
	if backdropPath == "" {
		return nil
	}
	key := CacheKey(backdropPath)

	payload, expiresAt, found, err := e.cache.Get(ctx, key)
	switch {
	case err != nil:
		e.log.Warn("Colour cache read failed", "key", key, "error", err)
	case found && (expiresAt.IsZero() || e.now().Before(expiresAt)):
		var hex *string
		if err := json.Unmarshal(payload, &hex); err == nil {
			return hex
		}
		e.log.Warn("Undecodable colour cache entry; recomputing", "key", key)
	}

	f := e.start(key, backdropPath)
	if f == nil {
		return nil // shutting down
	}
	timer := time.NewTimer(e.budget)
	defer timer.Stop()
	select {
	case <-f.done:
		return f.hex
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return nil
	}
}

// Close stops background computations and waits for them to return.
func (e *Extractor) Close() {
	e.mu.Lock()
	e.closed = true
	e.mu.Unlock()
	e.cancel()
	e.wg.Wait()
}

// start joins the computation for key, starting it if none is running.
func (e *Extractor) start(key, backdropPath string) *flight {
	e.mu.Lock()
	defer e.mu.Unlock()
	if f, ok := e.flights[key]; ok {
		return f
	}
	if e.closed {
		return nil
	}

	f := &flight{done: make(chan struct{})}
	e.flights[key] = f
	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		f.hex = e.compute(key, backdropPath)
		e.mu.Lock()
		delete(e.flights, key)
		e.mu.Unlock()
		close(f.done)
	}()
	return f
}

// compute fetches and samples the backdrop and stores the result: the colour
// forever, a failure for failureTTL.
func (e *Extractor) compute(key, backdropPath string) *string {
	ctx, cancel := context.WithTimeout(e.ctx, computeTimeout)
	defer cancel()

	img, err := e.fetch(ctx, backdropPath)
	var hex *string
	expires := time.Time{} // never
	if err != nil {
		if ctx.Err() != nil && e.ctx.Err() != nil {
			return nil // shutting down: don't remember a failure
		}
		e.log.Warn("Failed to fetch backdrop for its colour", "path", backdropPath, "error", err)
		expires = e.now().Add(failureTTL)
	} else {
		c := DominantColor(img)
		hex = &c
	}

	payload, _ := json.Marshal(hex)
	if err := e.cache.Set(ctx, key, payload, expires); err != nil {
		e.log.Warn("Colour cache write failed", "key", key, "error", err)
	}
	return hex
}

func (e *Extractor) fetch(ctx context.Context, backdropPath string) (image.Image, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, e.baseURL+backdropPath, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build request: %w", err)
	}
	resp, err := e.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}
	img, _, err := image.Decode(io.LimitReader(resp.Body, maxImageBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to decode image: %w", err)
	}
	return img, nil
}

// DominantColor samples img on a 32×18 grid and averages the samples,
// weighting each by its saturation (chroma, so near-black noise doesn't
// dominate the way HSL saturation would let it). The result is clamped to
// lightness 0.30–0.50 and saturation ≥ 0.25, so it glows behind white text
// whatever the artwork, and returned as #rrggbb.
func DominantColor(img image.Image) string {
	b := img.Bounds()
	if b.Empty() {
		return hexColor(clampHSL(0, 0, 0))
	}

	var sumR, sumG, sumB, sumW float64
	var plainR, plainG, plainB float64
	n := 0
	for gy := 0; gy < gridH; gy++ {
		y := b.Min.Y + (2*gy+1)*b.Dy()/(2*gridH)
		for gx := 0; gx < gridW; gx++ {
			x := b.Min.X + (2*gx+1)*b.Dx()/(2*gridW)
			r16, g16, b16, _ := img.At(x, y).RGBA()
			r, g, bl := float64(r16)/0xffff, float64(g16)/0xffff, float64(b16)/0xffff
			w := math.Max(r, math.Max(g, bl)) - math.Min(r, math.Min(g, bl))
			sumR += r * w
			sumG += g * w
			sumB += bl * w
			sumW += w
			plainR += r
			plainG += g
			plainB += bl
			n++
		}
	}

	var r, g, bl float64
	if sumW > 1e-6 {
		r, g, bl = sumR/sumW, sumG/sumW, sumB/sumW
	} else { // greyscale: no saturation to weight by
		r, g, bl = plainR/float64(n), plainG/float64(n), plainB/float64(n)
	}

	h, s, l := rgbToHSL(r, g, bl)
	return hexColor(clampHSL(h, s, l))
}

// clampHSL applies the hero clamps and converts back to RGB.
func clampHSL(h, s, l float64) (r, g, b float64) {
	l = math.Min(math.Max(l, 0.30), 0.50)
	s = math.Max(s, 0.25)
	return hslToRGB(h, s, l)
}

func hexColor(r, g, b float64) string {
	return fmt.Sprintf("#%02x%02x%02x", to8(r), to8(g), to8(b))
}

func to8(v float64) int {
	return int(math.Round(math.Min(math.Max(v, 0), 1) * 255))
}

// rgbToHSL converts RGB in 0–1 to hue (0–360), saturation and lightness
// (0–1).
func rgbToHSL(r, g, b float64) (h, s, l float64) {
	maxC := math.Max(r, math.Max(g, b))
	minC := math.Min(r, math.Min(g, b))
	l = (maxC + minC) / 2
	d := maxC - minC
	if d == 0 {
		return 0, 0, l
	}
	s = d / (1 - math.Abs(2*l-1))
	switch maxC {
	case r:
		h = math.Mod((g-b)/d, 6)
	case g:
		h = (b-r)/d + 2
	default:
		h = (r-g)/d + 4
	}
	h *= 60
	if h < 0 {
		h += 360
	}
	return h, s, l
}

// hslToRGB converts hue (0–360), saturation and lightness (0–1) to RGB.
func hslToRGB(h, s, l float64) (r, g, b float64) {
	c := (1 - math.Abs(2*l-1)) * s
	x := c * (1 - math.Abs(math.Mod(h/60, 2)-1))
	m := l - c/2
	switch {
	case h < 60:
		r, g, b = c, x, 0
	case h < 120:
		r, g, b = x, c, 0
	case h < 180:
		r, g, b = 0, c, x
	case h < 240:
		r, g, b = 0, x, c
	case h < 300:
		r, g, b = x, 0, c
	default:
		r, g, b = c, 0, x
	}
	return r + m, g + m, b + m
}
