package hdrezka

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// cdnFile is a fake CDN serving data with http.ServeContent. Each request
// takes the next status from fail (0 serves the file), then serves.
type cdnFile struct {
	data []byte

	mu       sync.Mutex
	fail     []int
	requests int
	ranges   []string
	agents   []string
	// ignoreRange serves the whole file with 200 whatever the Range.
	ignoreRange bool
	// block holds the response until it's closed, after sending headers
	// and the first byte (blockAfterHeaders) or before anything.
	block             chan struct{}
	blockAfterHeaders bool
}

func (c *cdnFile) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c.mu.Lock()
	c.requests++
	c.ranges = append(c.ranges, r.Header.Get("Range"))
	c.agents = append(c.agents, r.Header.Get("User-Agent"))
	status := 0
	if len(c.fail) > 0 {
		status, c.fail = c.fail[0], c.fail[1:]
	}
	block, afterHeaders, ignore := c.block, c.blockAfterHeaders, c.ignoreRange
	c.mu.Unlock()

	if block != nil && !afterHeaders {
		select {
		case <-block:
		case <-r.Context().Done():
			return
		}
	}
	switch {
	case status != 0:
		w.WriteHeader(status)
	case block != nil:
		w.Header().Set("Content-Range", "bytes 0-"+strconv.Itoa(len(c.data)-1)+"/"+strconv.Itoa(len(c.data)))
		w.WriteHeader(http.StatusPartialContent)
		w.Write(c.data[:1])
		w.(http.Flusher).Flush()
		select {
		case <-block:
		case <-r.Context().Done():
		}
	case ignore:
		w.Header().Set("Content-Length", strconv.Itoa(len(c.data)))
		w.Write(c.data)
	default:
		http.ServeContent(w, r, "f.mp4", time.Time{}, bytes.NewReader(c.data))
	}
}

func (c *cdnFile) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.requests
}

// fakeLinks resolves every key to link, counting calls.
type fakeLinks struct {
	link        string
	err         error
	links       atomic.Int32
	invalidated atomic.Int32
}

func (l *fakeLinks) Link(ctx context.Context, key StreamKey, quality string) (string, error) {
	l.links.Add(1)
	return l.link, l.err
}

func (l *fakeLinks) Invalidate(StreamKey) { l.invalidated.Add(1) }

func testData(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(i*7 + i/251)
	}
	return b
}

// newCDN starts a fake CDN and a streamer pointed at it. The link carries a
// token that must never show up in errors.
func newCDN(t *testing.T, size int, opts ...StreamerOption) (*cdnFile, *fakeLinks, *Streamer, *[]time.Duration) {
	t.Helper()
	cdn := &cdnFile{data: testData(size)}
	srv := httptest.NewServer(cdn)
	t.Cleanup(srv.Close)
	links := &fakeLinks{link: srv.URL + "/0be3bfb771ec26f0:2026101618:SECRETTOKEN/file.mp4"}
	s := NewStreamer(links, opts...)
	var sleeps []time.Duration
	s.sleep = func(ctx context.Context, d time.Duration) error {
		sleeps = append(sleeps, d)
		return ctx.Err()
	}
	return cdn, links, s, &sleeps
}

var testFile = File{Key: StreamKey{PostID: 2259, TranslatorID: 238}, Quality: "1080p"}

func readAll(t *testing.T, rc io.ReadCloser) []byte {
	t.Helper()
	defer rc.Close()
	b, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return b
}

func TestStreamer_PartialContent(t *testing.T) {
	cdn, _, s, _ := newCDN(t, 100_000)
	size := int64(len(cdn.data))

	body, err := s.Open(context.Background(), testFile, 12_345, size)
	if err != nil {
		t.Fatal(err)
	}
	if got := readAll(t, body); !bytes.Equal(got, cdn.data[12_345:]) {
		t.Errorf("body = %d bytes, want bytes 12345- (%d)", len(got), size-12_345)
	}
	if cdn.ranges[0] != "bytes=12345-" {
		t.Errorf("Range = %q", cdn.ranges[0])
	}
	if cdn.agents[0] != DefaultUserAgent || strings.HasPrefix(cdn.agents[0], "Mozilla") {
		t.Errorf("User-Agent = %q", cdn.agents[0])
	}
}

func TestStreamer_WholeFileAtZero(t *testing.T) {
	cdn, _, s, _ := newCDN(t, 5000)
	cdn.ignoreRange = true
	size := int64(len(cdn.data))

	body, err := s.Open(context.Background(), testFile, 0, size)
	if err != nil {
		t.Fatalf("a 200 at byte 0 with the right length: %v", err)
	}
	if got := readAll(t, body); !bytes.Equal(got, cdn.data) {
		t.Error("body differs")
	}

	if _, err := s.Open(context.Background(), testFile, 10, size); !errors.Is(err, ErrRangeIgnored) {
		t.Errorf("a 200 for byte 10 = %v, want ErrRangeIgnored", err)
	}
	if _, err := s.Open(context.Background(), testFile, 0, size+1); !errors.Is(err, ErrSizeMismatch) {
		t.Errorf("a 200 of another length = %v, want ErrSizeMismatch", err)
	}
}

func TestStreamer_SizeMismatch(t *testing.T) {
	cdn, _, s, _ := newCDN(t, 5000)
	size := int64(len(cdn.data))

	for _, stored := range []int64{size + 1, size - 1} {
		_, err := s.Open(context.Background(), testFile, 100, stored)
		if !errors.Is(err, ErrSizeMismatch) {
			t.Errorf("stored %d: %v, want ErrSizeMismatch", stored, err)
		}
	}
	// The CDN's file ends before the offset: 416.
	_, err := s.Open(context.Background(), testFile, size+10, size+20)
	if !errors.Is(err, ErrSizeMismatch) {
		t.Errorf("past the CDN's end: %v, want ErrSizeMismatch", err)
	}
}

func TestStreamer_BadContentRange(t *testing.T) {
	var header string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Range", header)
		w.WriteHeader(http.StatusPartialContent)
	}))
	defer srv.Close()
	s := NewStreamer(&fakeLinks{link: srv.URL + "/f.mp4"})

	for _, h := range []string{"bytes 0-99/1000", "bytes 10-99/*", "garbage"} {
		header = h
		if _, err := s.Open(context.Background(), testFile, 10, 1000); err == nil {
			t.Errorf("Content-Range %q accepted for byte 10 of 1000", h)
		}
	}
}

func TestStreamer_RefreshesARefusedLinkOnce(t *testing.T) {
	for _, status := range []int{http.StatusForbidden, http.StatusNotFound, http.StatusGone} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			cdn, links, s, _ := newCDN(t, 5000)
			cdn.fail = []int{status}
			body, err := s.Open(context.Background(), testFile, 100, int64(len(cdn.data)))
			if err != nil {
				t.Fatal(err)
			}
			if got := readAll(t, body); !bytes.Equal(got, cdn.data[100:]) {
				t.Error("body differs after the refresh")
			}
			if links.invalidated.Load() != 1 || links.links.Load() != 2 || cdn.count() != 2 {
				t.Errorf("invalidated %d, links %d, requests %d; want 1, 2, 2",
					links.invalidated.Load(), links.links.Load(), cdn.count())
			}
		})
	}

	cdn, links, s, _ := newCDN(t, 5000)
	cdn.fail = []int{http.StatusForbidden, http.StatusForbidden, http.StatusForbidden}
	_, err := s.Open(context.Background(), testFile, 0, int64(len(cdn.data)))
	if !errors.Is(err, ErrLinkExpired) {
		t.Fatalf("refused twice = %v, want ErrLinkExpired", err)
	}
	if links.invalidated.Load() != 1 || cdn.count() != 2 {
		t.Errorf("invalidated %d, requests %d; want 1 refresh and 2 requests", links.invalidated.Load(), cdn.count())
	}
}

func TestStreamer_RetriesA5xxOnce(t *testing.T) {
	cdn, links, s, sleeps := newCDN(t, 5000)
	cdn.fail = []int{http.StatusServiceUnavailable}
	body, err := s.Open(context.Background(), testFile, 0, int64(len(cdn.data)))
	if err != nil {
		t.Fatal(err)
	}
	body.Close()
	if len(*sleeps) != 1 || (*sleeps)[0] != streamBackoff || links.invalidated.Load() != 0 {
		t.Errorf("sleeps %v, invalidated %d", *sleeps, links.invalidated.Load())
	}

	cdn, _, s, _ = newCDN(t, 5000)
	cdn.fail = []int{http.StatusTooManyRequests, http.StatusBadGateway, http.StatusBadGateway}
	_, err = s.Open(context.Background(), testFile, 0, int64(len(cdn.data)))
	var se *StatusError
	if !errors.As(err, &se) || se.StatusCode != http.StatusBadGateway || cdn.count() != 2 {
		t.Errorf("err = %v after %d requests, want a 502 after 2", err, cdn.count())
	}
}

func TestStreamer_ResolveErrorsPassThrough(t *testing.T) {
	_, links, s, _ := newCDN(t, 5000)
	links.err = ErrPremium
	if _, err := s.Open(context.Background(), testFile, 0, 5000); !errors.Is(err, ErrPremium) {
		t.Errorf("Open = %v, want ErrPremium", err)
	}
}

func TestStreamer_ErrorsHideTheLink(t *testing.T) {
	cdn, links, s, _ := newCDN(t, 5000)
	cdn.fail = []int{http.StatusForbidden, http.StatusForbidden}
	_, err1 := s.Open(context.Background(), testFile, 0, 5000)

	srv := httptest.NewServer(http.NotFoundHandler())
	links.link = srv.URL + "/0be3bfb771ec26f0:2026101618:SECRETTOKEN/file.mp4"
	srv.Close() // connection refused
	_, err2 := s.Open(context.Background(), testFile, 0, 5000)

	for _, err := range []error{err1, err2} {
		if err == nil || strings.Contains(err.Error(), "SECRETTOKEN") || strings.Contains(err.Error(), "file.mp4") {
			t.Errorf("error leaks the link or is nil: %v", err)
		}
	}
}

func TestStreamer_BodyOutlivesTheOpenContext(t *testing.T) {
	cdn, _, s, _ := newCDN(t, 300_000)
	ctx, cancel := context.WithCancel(context.Background())
	body, err := s.Open(ctx, testFile, 0, int64(len(cdn.data)))
	cancel()
	if err != nil {
		t.Fatal(err)
	}
	if got := readAll(t, body); !bytes.Equal(got, cdn.data) {
		t.Error("the body broke when the open's context ended")
	}
}

func TestStreamer_ContextBoundsTheHeaders(t *testing.T) {
	cdn, _, s, _ := newCDN(t, 5000)
	cdn.block = make(chan struct{})
	defer close(cdn.block)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := s.Open(ctx, testFile, 0, 5000)
	if !errors.Is(err, context.DeadlineExceeded) || time.Since(start) > 2*time.Second {
		t.Errorf("Open = %v after %v, want the deadline", err, time.Since(start))
	}
}

func TestStreamer_CloseEndsABlockedRead(t *testing.T) {
	cdn, _, s, _ := newCDN(t, 5000)
	cdn.block, cdn.blockAfterHeaders = make(chan struct{}), true
	defer close(cdn.block)

	body, err := s.Open(context.Background(), testFile, 0, 5000)
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 10)
	if n, err := io.ReadAtLeast(body, buf, 1); n != 1 || err != nil {
		t.Fatalf("first byte: %d %v", n, err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := body.Read(buf)
		done <- err
	}()
	time.Sleep(20 * time.Millisecond)
	body.Close()
	select {
	case err := <-done:
		if err == nil {
			t.Error("Read after Close succeeded")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Close didn't end the blocked Read")
	}
	body.Close() // twice is fine
}

func TestStreamer_SemaphoreReleasedOnClose(t *testing.T) {
	cdn, _, s, _ := newCDN(t, 100_000, WithMaxBodies(2))
	size := int64(len(cdn.data))

	a, err := s.Open(context.Background(), testFile, 0, size)
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.Open(context.Background(), testFile, 10, size)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := s.Open(ctx, testFile, 20, size); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("a third body = %v, want to wait for a slot", err)
	}
	requests := cdn.count()

	a.Close()
	a.Close() // a second Close doesn't free a second slot
	c, err := s.Open(context.Background(), testFile, 30, size)
	if err != nil {
		t.Fatalf("after Close: %v", err)
	}
	ctx2, cancel2 := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel2()
	if _, err := s.Open(ctx2, testFile, 40, size); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("a double Close freed two slots: %v", err)
	}
	if cdn.count() != requests+1 {
		t.Errorf("requests = %d, want %d: waiting for a slot sends nothing", cdn.count(), requests+1)
	}
	b.Close()
	c.Close()
}

func TestStreamer_FailedOpenFreesItsSlot(t *testing.T) {
	cdn, _, s, _ := newCDN(t, 5000, WithMaxBodies(1))
	cdn.fail = []int{http.StatusTeapot}
	if _, err := s.Open(context.Background(), testFile, 0, 5000); err == nil {
		t.Fatal("want the 418")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	body, err := s.Open(ctx, testFile, 0, 5000)
	if err != nil {
		t.Fatalf("the failed open kept its slot: %v", err)
	}
	body.Close()
}

func TestStreamer_InvalidRange(t *testing.T) {
	_, _, s, _ := newCDN(t, 5000)
	for _, tt := range [][2]int64{{-1, 10}, {10, 10}, {0, 0}} {
		if _, err := s.Open(context.Background(), testFile, tt[0], tt[1]); err == nil {
			t.Errorf("Open(off %d, size %d) succeeded", tt[0], tt[1])
		}
	}
}
