package cdn

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/shapedtime/momoshtrem/internal/hdrezka"
	"github.com/shapedtime/momoshtrem/internal/streamlab/media"
)

// The CDN is tested through production's hdrezka.Streamer, which the lab
// wires it to. Nothing is timed tightly: the machine may be loaded.

type fixture struct {
	cdn    *CDN
	links  *Links
	s      *hdrezka.Streamer
	file   *File
	layout *media.Layout
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	l := media.MustNew(media.SingleFile("Interstellar.2014.mp4", 7, 64<<10, 3<<20+77, media.MP4))
	c := Start()
	t.Cleanup(c.Close)
	links := NewLinks(c)
	f := c.Add(l, 0, hdrezka.StreamKey{PostID: 2259, Path: "/films/2259.html", TranslatorID: 238}, "1080p")
	return &fixture{cdn: c, links: links, s: hdrezka.NewStreamer(links), file: f, layout: l}
}

func (fx *fixture) hfile() hdrezka.File {
	// Another Referer, the same stream: the resolver keys on the stream.
	key := fx.file.Key
	key.Path = "/films/other-mirror.html"
	return hdrezka.File{Key: key, Quality: fx.file.Quality}
}

func (fx *fixture) open(t *testing.T, off int64) io.ReadCloser {
	t.Helper()
	body, err := fx.s.Open(context.Background(), fx.hfile(), off, fx.file.Size)
	if err != nil {
		t.Fatalf("Open at %d: %v", off, err)
	}
	return body
}

// check reads n bytes of body (all of it when n < 0) and compares them
// with the file from off.
func (fx *fixture) check(t *testing.T, body io.Reader, off int64, n int64) {
	t.Helper()
	if n < 0 {
		n = fx.file.Size - off
	}
	got := make([]byte, n)
	if _, err := io.ReadFull(body, got); err != nil {
		t.Fatalf("reading %d bytes at %d: %v", n, off, err)
	}
	want := make([]byte, n)
	if _, err := fx.layout.ReadAt(want, off); err != nil && err != io.EOF {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("%d bytes at %d don't match the media", n, off)
	}
}

// settled waits until the CDN has no open body.
func (fx *fixture) settled(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for fx.cdn.Open() > 0 {
		if time.Now().After(deadline) {
			t.Fatalf("%d CDN bodies still open", fx.cdn.Open())
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestCDN_ServesTheMedia(t *testing.T) {
	fx := newFixture(t)
	for _, off := range []int64{0, 1 << 20, fx.file.Size - 100} {
		body := fx.open(t, off)
		fx.check(t, body, off, -1)
		body.Close()
	}
	fx.settled(t)
	if st := fx.links.Stats(); st.Links != 3 || st.Resolves != 1 || st.Invalidations != 0 {
		t.Errorf("link stats %+v: want 3 links, 1 resolve (then cached)", st)
	}
	if st := fx.cdn.Stats(); st.Requests != 3 || st.Bodies != 3 || st.Open != 0 {
		t.Errorf("CDN stats %+v", st)
	}
	if _, err := fx.s.Open(context.Background(), hdrezka.File{Key: fx.file.Key, Quality: "720p"}, 0, 10); !errors.Is(err, hdrezka.ErrNoQuality) {
		t.Errorf("another quality: %v, want ErrNoQuality", err)
	}
}

// An expired link is refused; the streamer resolves a fresh one once and
// carries on.
func TestCDN_Expire(t *testing.T) {
	for _, status := range []int{http.StatusForbidden, http.StatusNotFound, http.StatusGone} {
		fx := newFixture(t)
		fx.open(t, 0).Close()
		fx.cdn.Expire(status)
		body := fx.open(t, 1000)
		fx.check(t, body, 1000, 1000)
		body.Close()
		if st := fx.links.Stats(); st.Resolves != 2 || st.Invalidations != 1 {
			t.Errorf("%d: link stats %+v, want a refresh", status, st)
		}
		if st := fx.cdn.Stats(); st.Refused != 1 {
			t.Errorf("%d: CDN stats %+v, want one refused", status, st)
		}
	}
}

// A 503 is retried once after the streamer's backoff; two in a row fail
// the open.
func TestCDN_Unavailable(t *testing.T) {
	fx := newFixture(t)
	fx.cdn.Unavailable(1)
	start := time.Now()
	body := fx.open(t, 0)
	if took := time.Since(start); took < 900*time.Millisecond {
		t.Errorf("the retry came after %v, before the streamer's 1 s backoff", took)
	}
	fx.check(t, body, 0, 100)
	body.Close()

	fx.cdn.Unavailable(2)
	_, err := fx.s.Open(context.Background(), fx.hfile(), 0, fx.file.Size)
	var se *hdrezka.StatusError
	if !errors.As(err, &se) || se.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("after two 503s: %v, want a 503 StatusError", err)
	}
	if st := fx.cdn.Stats(); st.Unavailable != 3 {
		t.Errorf("CDN stats %+v, want three 503s", st)
	}
}

// A stalled body sends nothing until its client closes it; a dropped one
// ends early. Either way the CDN's body ends.
func TestCDN_StallAndDrop(t *testing.T) {
	fx := newFixture(t)
	body := fx.open(t, 0)
	fx.check(t, body, 0, 64<<10)
	if n := fx.cdn.Stall(); n != 1 {
		t.Fatalf("stalled %d bodies, want 1", n)
	}
	read := make(chan error, 1)
	go func() {
		_, err := io.Copy(io.Discard, body)
		read <- err
	}()
	select {
	case err := <-read:
		t.Fatalf("a stalled body's read returned: %v", err)
	case <-time.After(200 * time.Millisecond):
	}
	body.Close()
	select {
	case <-read:
	case <-time.After(5 * time.Second):
		t.Fatal("closing a stalled body didn't end its read")
	}
	fx.settled(t)

	body = fx.open(t, 0)
	fx.check(t, body, 0, 64<<10)
	if n := fx.cdn.Drop(); n != 1 {
		t.Fatalf("dropped %d bodies, want 1", n)
	}
	if _, err := io.Copy(io.Discard, body); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Errorf("a dropped body read %v, want an unexpected EOF", err)
	}
	body.Close()
	fx.settled(t)
	if st := fx.cdn.Stats(); st.Stalled != 1 || st.Dropped != 1 || st.PeakOpen != 1 {
		t.Errorf("CDN stats %+v", st)
	}
}

// A slow body sends at its rate at most. A floor only.
func TestCDN_SetRate(t *testing.T) {
	fx := newFixture(t)
	const bps = 512 << 10
	fx.cdn.SetRate(bps)
	body := fx.open(t, 0)
	defer body.Close()
	start := time.Now()
	n := int64(512 << 10)
	fx.check(t, body, 0, n)
	if floor := time.Duration(float64(n-bps/10) / bps * 0.8 * float64(time.Second)); time.Since(start) < floor {
		t.Errorf("%d bytes in %v at %d B/s", n, time.Since(start), bps)
	}
}

// FreeSlots sees the streamer's 8 slots, the ones open bodies hold, and
// makes no CDN request.
func TestFreeSlots(t *testing.T) {
	fx := newFixture(t)
	if n := fx.links.FreeSlots(fx.s, StreamerSlots+1, 300*time.Millisecond); n != StreamerSlots {
		t.Fatalf("a fresh streamer has %d slots, want %d: did hdrezka's defaultMaxBodies change?", n, StreamerSlots)
	}
	a, b := fx.open(t, 0), fx.open(t, 100)
	if n := fx.links.FreeSlots(fx.s, StreamerSlots, 300*time.Millisecond); n != StreamerSlots-2 {
		t.Errorf("with 2 bodies open: %d free slots, want %d", n, StreamerSlots-2)
	}
	a.Close()
	b.Close()
	start := time.Now()
	if n := fx.links.FreeSlots(fx.s, StreamerSlots, 5*time.Second); n != StreamerSlots {
		t.Errorf("after closing them: %d free slots, want %d", n, StreamerSlots)
	}
	if took := time.Since(start); took > 4*time.Second {
		t.Errorf("a probe of free slots took %v: it shouldn't wait", took)
	}
	if st := fx.cdn.Stats(); st.Requests != 2 {
		t.Errorf("the probes reached the CDN: %d requests, want the 2 bodies'", st.Requests)
	}
	if st := fx.links.Stats(); st.Links != 2 {
		t.Errorf("the probes counted as links: %+v", st)
	}
}
