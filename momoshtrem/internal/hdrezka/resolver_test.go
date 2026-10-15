package hdrezka

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeFetcher answers Movie and Episode with stream (or err), counting
// calls. A non-nil gate holds every call until it's closed.
type fakeFetcher struct {
	mu     sync.Mutex
	stream *Stream
	err    error
	gate   chan struct{}
	calls  atomic.Int32

	gotRef        Ref
	gotTranslator Translator
	gotEpisode    [3]int // translator, season, episode
}

func (f *fakeFetcher) Movie(ctx context.Context, ref Ref, t Translator) (*Stream, error) {
	f.mu.Lock()
	f.gotRef, f.gotTranslator = ref, t
	f.mu.Unlock()
	return f.answer(ctx)
}

func (f *fakeFetcher) Episode(ctx context.Context, ref Ref, translatorID, season, episode int) (*Stream, error) {
	f.mu.Lock()
	f.gotRef, f.gotEpisode = ref, [3]int{translatorID, season, episode}
	f.mu.Unlock()
	return f.answer(ctx)
}

func (f *fakeFetcher) answer(ctx context.Context) (*Stream, error) {
	f.calls.Add(1)
	if f.gate != nil {
		select {
		case <-f.gate:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.stream, f.err
}

// signedLink is a CDN link that expires at the given Moscow hour.
func signedLink(expiry time.Time, name string) string {
	return "https://cdn.example/stream/0be3bfb771ec26f0:" + expiry.In(expiryZone).Format("2006010215") + ":dlFOU2k3VzR6/" + name + ".mp4"
}

func streamWith(links ...string) *Stream {
	s := &Stream{}
	for i, l := range links {
		s.Qualities = append(s.Qualities, Quality{Label: []string{"720p", "1080p", "480p"}[i%3], MP4: l})
	}
	return s
}

func TestLinkTTL(t *testing.T) {
	now := time.Date(2026, 10, 16, 9, 0, 0, 0, time.UTC)
	tests := []struct {
		name   string
		stream *Stream
		want   time.Duration
	}{
		{"no expiry in the link: 4 h", streamWith("https://cdn.example/a.mp4"), maxLinkTTL},
		{"expires in 20 h: 4 h", streamWith(signedLink(now.Add(20*time.Hour), "a")), maxLinkTTL},
		{"expires in 3 h: an hour before", streamWith(signedLink(now.Add(3*time.Hour), "a")), 2 * time.Hour},
		{"the earliest link wins", streamWith(signedLink(now.Add(20*time.Hour), "a"), signedLink(now.Add(4*time.Hour), "b")), 3 * time.Hour},
		{"expiring within the hour: a minute", streamWith(signedLink(now.Add(30*time.Minute), "a")), minLinkTTL},
		{"already expired: a minute", streamWith(signedLink(now.Add(-5*time.Hour), "a")), minLinkTTL},
		{"no qualities: 4 h", &Stream{}, maxLinkTTL},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := linkTTL(tt.stream, now); got != tt.want {
				t.Errorf("linkTTL = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLinkExpiry_ReadsMoscowHour(t *testing.T) {
	got, ok := linkExpiry("https://edge.example/0be3bfb771ec26f0:2026101618:dlFOU2k3VzR6/x/y.mp4")
	want := time.Date(2026, 10, 16, 15, 0, 0, 0, time.UTC) // 18h MSK
	if !ok || !got.Equal(want) {
		t.Errorf("linkExpiry = %v %v, want %v", got, ok, want)
	}
	for _, link := range []string{"https://cdn.example/stream/a.mp4", "https://x/abc:2026101618:t/a.mp4", "https://x/0be3bfb771ec26f0:2026139918:t/a.mp4"} {
		if _, ok := linkExpiry(link); ok {
			t.Errorf("linkExpiry(%q) found an expiry", link)
		}
	}
}

func newTestResolver(f *fakeFetcher, now *time.Time) *Resolver {
	r := NewResolver(f)
	r.now = func() time.Time { return *now }
	return r
}

func TestResolver_CachesUntilTheTTL(t *testing.T) {
	now := time.Date(2026, 10, 16, 9, 0, 0, 0, time.UTC)
	f := &fakeFetcher{stream: streamWith(signedLink(now.Add(3*time.Hour), "a"))}
	r := newTestResolver(f, &now)
	key := StreamKey{PostID: 2259, Path: "/films/fiction/2259-interstellar-2014.html", TranslatorID: 238}

	for range 3 {
		if _, err := r.Resolve(context.Background(), key); err != nil {
			t.Fatal(err)
		}
	}
	if n := f.calls.Load(); n != 1 {
		t.Fatalf("fetches = %d, want 1", n)
	}

	// Another path for the same stream is the same stream.
	key2 := key
	key2.Path = "/films/fiction/2259-other-slug.html"
	if _, err := r.Resolve(context.Background(), key2); err != nil || f.calls.Load() != 1 {
		t.Fatalf("the Referer path changed the identity: %d fetches, %v", f.calls.Load(), err)
	}

	now = now.Add(2*time.Hour - time.Second) // the link expires in 3 h: cached for 2 h
	r.Resolve(context.Background(), key)
	if n := f.calls.Load(); n != 1 {
		t.Fatalf("fetches before the TTL = %d, want 1", n)
	}
	now = now.Add(2 * time.Second)
	r.Resolve(context.Background(), key)
	if n := f.calls.Load(); n != 2 {
		t.Fatalf("fetches after the TTL = %d, want 2", n)
	}
}

func TestResolver_KeysByEveryField(t *testing.T) {
	now := time.Now()
	f := &fakeFetcher{stream: streamWith("https://cdn.example/a.mp4")}
	r := newTestResolver(f, &now)
	base := StreamKey{PostID: 646, TranslatorID: 238, Season: 1, Episode: 1}
	keys := []StreamKey{base}
	for _, change := range []func(*StreamKey){
		func(k *StreamKey) { k.PostID = 647 },
		func(k *StreamKey) { k.TranslatorID = 56 },
		func(k *StreamKey) { k.Camrip = true },
		func(k *StreamKey) { k.Ads = true },
		func(k *StreamKey) { k.Director = true },
		func(k *StreamKey) { k.Season = 2 },
		func(k *StreamKey) { k.Episode = 2 },
	} {
		k := base
		change(&k)
		keys = append(keys, k)
	}
	for _, k := range keys {
		if _, err := r.Resolve(context.Background(), k); err != nil {
			t.Fatal(err)
		}
	}
	if n := int(f.calls.Load()); n != len(keys) {
		t.Errorf("fetches = %d, want %d (one per stream)", n, len(keys))
	}
}

func TestResolver_Singleflight(t *testing.T) {
	now := time.Now()
	f := &fakeFetcher{stream: streamWith("https://cdn.example/a.mp4"), gate: make(chan struct{})}
	r := newTestResolver(f, &now)
	key := StreamKey{PostID: 646, TranslatorID: 238, Season: 1, Episode: 2}

	const callers = 20
	var wg sync.WaitGroup
	results := make([]*Stream, callers)
	errs := make([]error, callers)
	for i := range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i], errs[i] = r.Resolve(context.Background(), key)
		}()
	}
	time.Sleep(20 * time.Millisecond) // let them queue behind the first
	close(f.gate)
	wg.Wait()

	if n := f.calls.Load(); n != 1 {
		t.Errorf("fetches = %d, want 1", n)
	}
	for i := range callers {
		if errs[i] != nil || results[i] != results[0] {
			t.Errorf("caller %d: %p %v, want the shared stream", i, results[i], errs[i])
		}
	}
}

func TestResolver_InvalidateFetchesAgain(t *testing.T) {
	now := time.Now()
	f := &fakeFetcher{stream: streamWith("https://cdn.example/a.mp4")}
	r := newTestResolver(f, &now)
	key := StreamKey{PostID: 2259, TranslatorID: 238}

	r.Resolve(context.Background(), key)
	r.Invalidate(StreamKey{PostID: 2259, TranslatorID: 56}) // another stream
	r.Resolve(context.Background(), key)
	if n := f.calls.Load(); n != 1 {
		t.Fatalf("fetches = %d, want 1", n)
	}
	r.Invalidate(key)
	r.Resolve(context.Background(), key)
	if n := f.calls.Load(); n != 2 {
		t.Errorf("fetches after Invalidate = %d, want 2", n)
	}
}

func TestResolver_ErrorsAreNotCached(t *testing.T) {
	now := time.Now()
	f := &fakeFetcher{err: ErrChallenge}
	r := newTestResolver(f, &now)
	key := StreamKey{PostID: 2259, TranslatorID: 238}

	if _, err := r.Resolve(context.Background(), key); !errors.Is(err, ErrChallenge) {
		t.Fatalf("Resolve = %v, want ErrChallenge", err)
	}
	f.mu.Lock()
	f.err, f.stream = nil, streamWith("https://cdn.example/a.mp4")
	f.mu.Unlock()
	if _, err := r.Resolve(context.Background(), key); err != nil {
		t.Fatalf("Resolve after the error: %v", err)
	}
	if n := f.calls.Load(); n != 2 {
		t.Errorf("fetches = %d, want 2", n)
	}
}

func TestResolver_FetchOutlivesACanceledCaller(t *testing.T) {
	now := time.Now()
	f := &fakeFetcher{stream: streamWith("https://cdn.example/a.mp4"), gate: make(chan struct{})}
	r := newTestResolver(f, &now)
	key := StreamKey{PostID: 2259, TranslatorID: 238}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := r.Resolve(ctx, key)
		done <- err
	}()
	for f.calls.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled caller got %v", err)
	}
	close(f.gate)
	if _, err := r.Resolve(context.Background(), key); err != nil {
		t.Fatal(err)
	}
	if n := f.calls.Load(); n != 1 {
		t.Errorf("fetches = %d, want 1: the first fetch should finish and be cached", n)
	}
}

func TestResolver_MovieAndEpisode(t *testing.T) {
	now := time.Now()
	f := &fakeFetcher{stream: streamWith("https://cdn.example/a.mp4")}
	r := newTestResolver(f, &now)

	movie := StreamKey{PostID: 2259, Path: "/films/x/2259-a.html", TranslatorID: 238, Camrip: true, Director: true}
	r.Resolve(context.Background(), movie)
	want := Translator{ID: 238, Camrip: true, Director: true}
	if f.gotTranslator != want || f.gotRef != (Ref{PostID: 2259, Path: "/films/x/2259-a.html"}) {
		t.Errorf("Movie got %+v %+v, want %+v", f.gotRef, f.gotTranslator, want)
	}

	episode := StreamKey{PostID: 646, Path: "/series/x/646-a.html", TranslatorID: 238, Season: 2, Episode: 5}
	r.Resolve(context.Background(), episode)
	if f.gotEpisode != [3]int{238, 2, 5} || f.gotRef.PostID != 646 {
		t.Errorf("Episode got %+v %v", f.gotRef, f.gotEpisode)
	}
}

func TestResolver_Link(t *testing.T) {
	now := time.Now()
	f := &fakeFetcher{stream: &Stream{Qualities: []Quality{
		{Label: "720p", Resolution: 720, MP4: "https://cdn.example/720.mp4"},
		{Label: "1080p", Resolution: 1080, MP4: "https://cdn.example/1080.mp4"},
		{Label: "1080p Ultra", Resolution: 1080, Premium: true, MP4: "https://cdn.example/teaser.mp4"},
		{Label: "4K", Resolution: 2160, HLS: "https://cdn.example/4k.m3u8"},
	}}}
	r := newTestResolver(f, &now)
	key := StreamKey{PostID: 2259, TranslatorID: 238}

	tests := []struct {
		quality string
		want    string
		err     error
	}{
		{"1080p", "https://cdn.example/1080.mp4", nil},
		{"720P", "https://cdn.example/720.mp4", nil},
		{"480p", "", ErrNoQuality},
		{"1080p Ultra", "", ErrPremium},
		{"4K", "", ErrNoMP4},
	}
	for _, tt := range tests {
		got, err := r.Link(context.Background(), key, tt.quality)
		if got != tt.want || !errors.Is(err, tt.err) || (tt.err == nil && err != nil) {
			t.Errorf("Link(%q) = %q, %v; want %q, %v", tt.quality, got, err, tt.want, tt.err)
		}
	}

	f.mu.Lock()
	f.err = ErrPremium
	f.mu.Unlock()
	r.Invalidate(key)
	if _, err := r.Link(context.Background(), key, "1080p"); !errors.Is(err, ErrPremium) {
		t.Errorf("Link of a premium translation = %v", err)
	}
}
