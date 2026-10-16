package sessions

import (
	"context"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/shapedtime/momoshtrem/internal/library"
)

type clock struct{ t time.Time }

func (c *clock) now() time.Time          { return c.t }
func (c *clock) advance(d time.Duration) { c.t = c.t.Add(d) }
func newClock() *clock                   { return &clock{t: time.Date(2026, 10, 16, 21, 0, 0, 0, time.UTC)} }
func ptr[T any](v T) *T                  { return &v }
func get(addr, ua string) context.Context {
	return WithRequest(context.Background(), Request{Method: "GET", UserAgent: ua, RemoteAddr: addr})
}

const (
	infuse = "Infuse-Direct/7.7 (AppleTV)"
	mb     = 1 << 20
)

// episode is S01E05 of a show, 1000 MB, in a torrent whose pieces are
// buffered 100 MB past any offset.
var episode = File{
	Path: "/TV Shows/Shogun (2024)/Season 01/Shogun - S01E05 - Broken to the Fist.mkv", Name: "Shogun - S01E05 - Broken to the Fist.mkv",
	Size: 1000 * mb, Source: SourceTorrent, InfoHash: "7b20c4e18d93a6f2b0e5d71c4a8f3e926b5d11fa",
	TorrentPath: "Shogun.S01/Shogun.S01E05.mkv", Item: &ItemRef{Type: library.ItemTypeEpisode, ID: 55},
	BufferedAhead: func(int64) int64 { return 100 * mb },
}

// read reads n bytes at off through h.
func read(h *Handle, off int64, n int) {
	h.Begin()
	h.End(off, n)
}

type fakeItems struct {
	items []library.StreamItem
	err   error
	calls int
}

func (f *fakeItems) StreamItems(_ context.Context, movieIDs, episodeIDs []int64) ([]library.StreamItem, error) {
	f.calls++
	return f.items, f.err
}

func TestOpen_OnlyGETsAreTracked(t *testing.T) {
	r := NewRegistry()
	if h := r.Open(context.Background(), episode); h != nil {
		t.Error("a context without a request: want no handle")
	}
	propfind := WithRequest(context.Background(), Request{Method: "PROPFIND", UserAgent: infuse, RemoteAddr: "10.0.0.5"})
	if h := r.Open(propfind, episode); h != nil {
		t.Error("PROPFIND: want no handle")
	}
	head := WithRequest(context.Background(), Request{Method: "HEAD", UserAgent: infuse, RemoteAddr: "10.0.0.5"})
	if h := r.Open(head, episode); h != nil {
		t.Error("HEAD: want no handle")
	}
	// A nil handle is safe to use.
	var h *Handle
	h.Begin()
	h.End(0, 10)
	h.Close()
	var nilRegistry *Registry
	if nilRegistry.Open(get("10.0.0.5", infuse), episode) != nil {
		t.Error("a nil registry: want no handle")
	}

	// Opens that never read (a listing) leave nothing behind.
	for range 100 {
		r.Open(get("10.0.0.5", infuse), episode).Close()
	}
	if len(r.sessions) != 0 || r.Count() != 0 {
		t.Errorf("sessions after opens without reads: %d", len(r.sessions))
	}
}

func TestSessions_GroupOnePlayback(t *testing.T) {
	c := newClock()
	r := NewRegistry(WithClock(c.now))

	// Infuse: the header, the index at the end, then playback, each its own
	// Range GET from another port of the same Apple TV.
	head := r.Open(get("10.0.0.5", infuse), episode)
	read(head, 0, 256<<10)
	head.Close()
	c.advance(200 * time.Millisecond)
	tail := r.Open(get("10.0.0.5", infuse), episode)
	read(tail, 990*mb, 512<<10)
	tail.Close()
	c.advance(200 * time.Millisecond)
	play := r.Open(get("10.0.0.5", infuse), episode)
	for i := range 10 {
		read(play, int64(i)*mb, mb)
		c.advance(100 * time.Millisecond)
	}

	// Another device on the same file, and the same device on another file.
	vlc := r.Open(get("10.0.0.5", "VLC/3.0.20 LibVLC/3.0.20"), episode)
	read(vlc, 0, mb)
	other := episode
	other.Path, other.Name = "/Movies/Dune (2021)/Dune (2021).mkv", "Dune (2021).mkv"
	movie := r.Open(get("10.0.0.5", infuse), other)
	read(movie, 0, mb)

	got := r.Sessions(context.Background())
	if len(got) != 3 || r.Count() != 3 {
		t.Fatalf("sessions = %d (count %d), want 3", len(got), r.Count())
	}
	var shogun *Session
	for i := range got {
		if got[i].FileName == episode.Name && deref(got[i].Client) == ClientInfuse {
			shogun = &got[i]
		}
	}
	if shogun == nil {
		t.Fatalf("no Infuse session on the episode: %+v", got)
	}
	if shogun.OffsetBytes != 10*mb || !shogun.StartedAt.Equal(newClock().t) || deref(shogun.Device) != "Apple TV" {
		t.Errorf("Infuse's session = %+v", *shogun)
	}
	if shogun.Source != SourceTorrent || deref(shogun.InfoHash) != episode.InfoHash || deref(shogun.FilePath) != episode.TorrentPath {
		t.Errorf("torrent fields = %v %v %v", shogun.Source, shogun.InfoHash, shogun.FilePath)
	}
	if got[0].LastReadAt.Before(got[1].LastReadAt) || got[1].LastReadAt.Before(got[2].LastReadAt) {
		t.Error("want the most recently read first")
	}
}

func TestSessions_Sniffs(t *testing.T) {
	tests := []struct {
		name    string
		bytes   int
		openFor time.Duration
		closed  bool
		want    bool
	}{
		{"small and closed at once: a sniff", 10 << 10, 500 * time.Millisecond, true, false},
		{"just under 64 KB, closed within 2 s", SniffBytes - 1, 1900 * time.Millisecond, true, false},
		{"64 KB, closed at once", SniffBytes, 100 * time.Millisecond, true, true},
		{"small, still open after 2 s (a slow torrent)", 10 << 10, 2 * time.Second, false, true},
		{"small, closed after 3 s", 10 << 10, 3 * time.Second, true, true},
		{"small, still open within 2 s: not yet", 10 << 10, time.Second, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newClock()
			r := NewRegistry(WithClock(c.now))
			h := r.Open(get("10.0.0.5", infuse), episode)
			read(h, 0, tt.bytes)
			c.advance(tt.openFor)
			if tt.closed {
				h.Close()
			}
			if got := r.Count() == 1; got != tt.want {
				t.Errorf("session shows = %v, want %v", got, tt.want)
			}
			if !tt.want && tt.closed && len(r.sessions) != 0 {
				t.Error("a sniff's session is still kept")
			}
		})
	}

	// A sniff, then playback: one session, which starts with the playback.
	c := newClock()
	r := NewRegistry(WithClock(c.now))
	sniff := r.Open(get("10.0.0.5", infuse), episode)
	read(sniff, 0, 512)
	sniff.Close()
	c.advance(time.Second)
	play := r.Open(get("10.0.0.5", infuse), episode)
	read(play, 0, mb)
	got := r.Sessions(context.Background())
	if len(got) != 1 || !got[0].StartedAt.Equal(c.t) {
		t.Errorf("sessions = %+v, want one from the playback", got)
	}
}

func TestSessions_Expiry(t *testing.T) {
	c := newClock()
	r := NewRegistry(WithClock(c.now))
	h := r.Open(get("10.0.0.5", infuse), episode)
	read(h, 0, mb)
	first := r.Sessions(context.Background())[0].ID

	c.advance(Timeout - time.Second)
	if r.Count() != 1 {
		t.Fatal("ended before the timeout")
	}
	// A read in flight (waiting for pieces) keeps it going.
	h.Begin()
	c.advance(2 * Timeout)
	if r.Count() != 1 {
		t.Fatal("ended during a read")
	}
	h.End(mb, mb)
	c.advance(Timeout - time.Second)
	if r.Count() != 1 {
		t.Fatal("ended before the timeout after the read")
	}
	c.advance(time.Second)
	if n := r.Count(); n != 0 || len(r.sessions) != 0 {
		t.Fatalf("count %d (kept %d) 30 s after the last read, want 0", n, len(r.sessions))
	}

	// Infuse resumes on the open connection: a new session, shown at once.
	read(h, 2*mb, 4<<10)
	got := r.Sessions(context.Background())
	if len(got) != 1 || got[0].ID == first || !got[0].StartedAt.Equal(c.t) {
		t.Errorf("after resuming: %+v, want a new session", got)
	}
	h.Close()
	if r.Count() != 1 {
		t.Error("closing the last open ends the session early")
	}
}

func TestSessions_Rate(t *testing.T) {
	c := newClock()
	r := NewRegistry(WithClock(c.now))
	h := r.Open(get("10.0.0.5", infuse), episode)
	// 100 KB every 100 ms: 1 MB/s for 30 s.
	const rate = 1000 * 1024.0
	for i := range 300 {
		c.advance(100 * time.Millisecond)
		read(h, int64(i)*100<<10, 100<<10)
	}
	got := float64(r.Sessions(context.Background())[0].RateBps)
	if math.Abs(got-rate)/rate > 0.02 {
		t.Errorf("rate = %.0f, want about %.0f", got, rate)
	}
	// Five seconds without a read: it decays by e.
	c.advance(RateTimeConstant)
	decayed := float64(r.Sessions(context.Background())[0].RateBps)
	if want := got / math.E; math.Abs(decayed-want)/want > 0.01 {
		t.Errorf("decayed rate = %.0f, want about %.0f", decayed, want)
	}
}

func TestSessions_PositionAndBuffer(t *testing.T) {
	items := &fakeItems{items: []library.StreamItem{
		{ItemType: library.ItemTypeEpisode, ItemID: 55, MediaType: library.JobTV, TMDBID: 126308, LibraryID: 4, Title: "Shōgun",
			SeasonNumber: ptr(1), EpisodeNumber: ptr(5), EpisodeName: "Broken to the Fist", RuntimeMinutes: 50},
		{ItemType: library.ItemTypeMovie, ItemID: 9, MediaType: library.JobMovie, TMDBID: 438631, LibraryID: 9, Title: "Dune"},
	}}
	c := newClock()
	r := NewRegistry(WithClock(c.now), WithItems(items))

	ep := r.Open(get("10.0.0.5", infuse), episode)
	read(ep, 0, 500*mb) // offset 500 MB of 1000 MB
	got := r.Sessions(context.Background())[0]
	it := got.Item
	if it == nil || it.MediaType != "tv" || it.TMDBID != 126308 || it.LibraryID != 4 || it.Title != "Shōgun" ||
		deref(it.SeasonNumber) != 1 || deref(it.EpisodeNumber) != 5 || deref(it.EpisodeName) != "Broken to the Fist" {
		t.Fatalf("item = %+v", it)
	}
	// Half of 50 minutes; 100 MB of 1000 MB is 5 minutes ahead.
	if deref(got.PositionSeconds) != 1500 || deref(got.DurationSeconds) != 3000 {
		t.Errorf("position %v of %v, want 1500 of 3000", got.PositionSeconds, got.DurationSeconds)
	}
	if deref(got.BufferedAheadBytes) != 100*mb || deref(got.BufferedAheadSeconds) != 300 {
		t.Errorf("buffered %v bytes, %v s; want 100 MB, 300 s", got.BufferedAheadBytes, got.BufferedAheadSeconds)
	}

	// A movie without a runtime, from HDRezka: no position, no buffer.
	ep.Close()
	c.advance(Timeout)
	stream := File{Path: "/Movies/Dune (2021)/Dune (2021).mp4", Name: "Dune (2021).mp4", Size: 2000 * mb,
		Source: SourceHDRezka, Item: &ItemRef{Type: library.ItemTypeMovie, ID: 9}}
	h := r.Open(get("10.0.0.6", "VLC/3.0.20 LibVLC/3.0.20"), stream)
	read(h, 0, mb)
	all := r.Sessions(context.Background())
	if len(all) != 1 {
		t.Fatalf("sessions = %+v", all)
	}
	s := all[0]
	if s.Item == nil || s.Item.MediaType != "movie" || s.Item.SeasonNumber != nil || s.Item.EpisodeName != nil {
		t.Errorf("movie item = %+v", s.Item)
	}
	if s.PositionSeconds != nil || s.DurationSeconds != nil || s.BufferedAheadBytes != nil || s.BufferedAheadSeconds != nil ||
		s.InfoHash != nil || s.FilePath != nil || s.Source != SourceHDRezka {
		t.Errorf("an HDRezka movie without a runtime: %+v", s)
	}
}

func TestSessions_ItemsAreCached(t *testing.T) {
	items := &fakeItems{items: []library.StreamItem{{ItemType: library.ItemTypeEpisode, ItemID: 55, MediaType: library.JobTV, Title: "Shōgun"}}}
	c := newClock()
	r := NewRegistry(WithClock(c.now), WithItems(items))
	h := r.Open(get("10.0.0.5", infuse), episode)
	for range 3 {
		read(h, 0, mb)
		r.Sessions(context.Background())
		c.advance(time.Second)
	}
	if items.calls != 1 {
		t.Errorf("resolver calls = %d, want 1 (cached)", items.calls)
	}
	c.advance(itemTTL)
	read(h, 0, mb)
	r.Sessions(context.Background())
	if items.calls != 2 {
		t.Errorf("resolver calls = %d after the TTL, want 2", items.calls)
	}

	// A file the library doesn't know (no item), and a failing resolver.
	items.err = errors.New("connection refused")
	c.advance(itemTTL)
	read(h, 0, mb)
	if got := r.Sessions(context.Background()); got[0].Item != nil || got[0].PositionSeconds != nil {
		t.Errorf("with the resolver failing: %+v", got[0])
	}
	items.err = nil
	calls := items.calls
	unknown := episode
	unknown.Item, unknown.Path, unknown.Name = nil, "/TV Shows/x.mkv", "x.mkv"
	read(r.Open(get("10.0.0.5", infuse), unknown), 0, mb)
	for _, s := range r.Sessions(context.Background()) {
		if s.FileName == unknown.Name && s.Item != nil {
			t.Errorf("a file without an item: %+v", s.Item)
		}
	}
	if items.calls != calls+1 {
		t.Errorf("resolver calls = %d, want only the failed item asked again", items.calls-calls)
	}
}

func TestMiddleware(t *testing.T) {
	var got Request
	h := Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, _ = RequestFrom(r.Context())
	}))
	req := httptest.NewRequest("GET", "/TV%20Shows/", nil)
	req.RemoteAddr = "192.168.1.40:52344"
	req.Header.Set("User-Agent", infuse)
	h.ServeHTTP(httptest.NewRecorder(), req)
	if got != (Request{Method: "GET", UserAgent: infuse, RemoteAddr: "192.168.1.40"}) {
		t.Errorf("request = %+v", got)
	}
	req.RemoteAddr = "[fe80::1]:52344"
	h.ServeHTTP(httptest.NewRecorder(), req)
	if got.RemoteAddr != "fe80::1" {
		t.Errorf("IPv6 address = %q", got.RemoteAddr)
	}
}
