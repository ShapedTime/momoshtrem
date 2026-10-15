package vfs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/shapedtime/momoshtrem/internal/hdrezka"
)

// upstream is a fake CDN: http.ServeContent over data, each request
// recorded by the offset it starts at. Links carry a version: a stale one
// gets 404, like an expired signed link.
type upstream struct {
	data []byte
	srv  *httptest.Server

	mu      sync.Mutex
	starts  []int64
	version int
	// cutAfter aborts the next response after that many bytes; with
	// expireOnCut, its link expires at the same time.
	cutAfter    int
	expireOnCut bool
	// stall sends the headers and the first byte, then nothing.
	stall bool
}

func newUpstream(t *testing.T, size int) *upstream {
	t.Helper()
	u := &upstream{data: make([]byte, size)}
	for i := range u.data {
		u.data[i] = byte(i*31 + i/509)
	}
	u.srv = httptest.NewServer(u)
	t.Cleanup(u.srv.Close)
	return u
}

func (u *upstream) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start, _ := strconv.ParseInt(strings.TrimSuffix(strings.TrimPrefix(r.Header.Get("Range"), "bytes="), "-"), 10, 64)
	version, _ := strconv.Atoi(strings.TrimPrefix(strings.Split(r.URL.Path, "/")[1], "v"))

	u.mu.Lock()
	u.starts = append(u.starts, start)
	stale := version != u.version
	cut := u.cutAfter
	u.cutAfter = 0
	if cut > 0 && u.expireOnCut {
		u.version++
	}
	stall := u.stall
	u.mu.Unlock()

	switch {
	case stale:
		w.WriteHeader(http.StatusNotFound)
	case stall:
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, len(u.data)-1, len(u.data)))
		w.WriteHeader(http.StatusPartialContent)
		w.Write(u.data[start : start+1])
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	case cut > 0:
		http.ServeContent(&cutWriter{ResponseWriter: w, left: cut}, r, "f.mp4", time.Time{}, bytes.NewReader(u.data))
	default:
		http.ServeContent(w, r, "f.mp4", time.Time{}, bytes.NewReader(u.data))
	}
}

func (u *upstream) requests() []int64 {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([]int64(nil), u.starts...)
}

// cutWriter drops the connection after left bytes of body.
type cutWriter struct {
	http.ResponseWriter
	left int
}

func (c *cutWriter) Write(p []byte) (int, error) {
	if len(p) >= c.left {
		c.ResponseWriter.Write(p[:c.left])
		c.ResponseWriter.(http.Flusher).Flush()
		panic(http.ErrAbortHandler)
	}
	c.left -= len(p)
	return c.ResponseWriter.Write(p)
}

// versionedLinks hands out the upstream's link at the version it last
// resolved; Invalidate resolves the current one.
type versionedLinks struct {
	u           *upstream
	mu          sync.Mutex
	version     int
	invalidated int
}

func (l *versionedLinks) Link(context.Context, hdrezka.StreamKey, string) (string, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return fmt.Sprintf("%s/v%d/file.mp4", l.u.srv.URL, l.version), nil
}

func (l *versionedLinks) Invalidate(hdrezka.StreamKey) {
	l.u.mu.Lock()
	current := l.u.version
	l.u.mu.Unlock()
	l.mu.Lock()
	l.version = current
	l.invalidated++
	l.mu.Unlock()
}

// remoteOver is a RemoteFile streamed from u through hdrezka.Streamer, the
// opener HDRezka files use, claiming size bytes.
func remoteOver(u *upstream, size int64, readTimeout time.Duration) (*RemoteFile, *versionedLinks) {
	links := &versionedLinks{u: u}
	streamer := hdrezka.NewStreamer(links)
	open := func(ctx context.Context, off int64) (io.ReadCloser, error) {
		return streamer.Open(ctx, hdrezka.File{Quality: "1080p"}, off, size)
	}
	return NewRemoteFile("Interstellar (2014).mp4", size, open, readTimeout), links
}

// readRange reads [from, to) with ReadAt in chunk-sized calls, the way
// WebDAV's GET does.
func readRange(t *testing.T, f *RemoteFile, from, to int64, chunk int) []byte {
	t.Helper()
	var out []byte
	buf := make([]byte, chunk)
	for off := from; off < to; {
		n := int(min(int64(chunk), to-off))
		got, err := f.ReadAt(buf[:n], off)
		if err != nil && !(err == io.EOF && off+int64(got) == f.Size()) {
			t.Fatalf("ReadAt(%d bytes at %d) = %d, %v", n, off, got, err)
		}
		out = append(out, buf[:got]...)
		off += int64(got)
	}
	return out
}

func TestRemoteFile_OpenDoesNoIO(t *testing.T) {
	u := newUpstream(t, 1000)
	f, _ := remoteOver(u, 1000, time.Second)
	if f.Name() != "Interstellar (2014).mp4" || f.Size() != 1000 || f.IsDir() {
		t.Errorf("file = %q %d %v", f.Name(), f.Size(), f.IsDir())
	}
	if fi, err := f.Stat(); err != nil || fi.Size() != 1000 || fi.Name() != f.Name() {
		t.Errorf("Stat = %v, %v", fi, err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if n := len(u.requests()); n != 0 {
		t.Errorf("requests = %d, want 0", n)
	}
}

func TestRemoteFile_SequentialReadsUseOneBody(t *testing.T) {
	const size = 1 << 20
	u := newUpstream(t, size)
	f, _ := remoteOver(u, size, time.Second)
	defer f.Close()

	got := readRange(t, f, 0, size, 32<<10)
	if !bytes.Equal(got, u.data) {
		t.Fatal("bytes differ")
	}
	if r := u.requests(); len(r) != 1 || r[0] != 0 {
		t.Errorf("requests = %v, want one from 0", r)
	}
}

func TestRemoteFile_ShortSkipDiscards(t *testing.T) {
	const size = 2 << 20
	u := newUpstream(t, size)
	f, _ := remoteOver(u, size, time.Second)
	defer f.Close()

	a := readRange(t, f, 0, 32<<10, 32<<10)
	skipTo := int64(32<<10 + 200<<10) // 200 KiB ahead
	b := readRange(t, f, skipTo, skipTo+64<<10, 32<<10)
	if !bytes.Equal(a, u.data[:32<<10]) || !bytes.Equal(b, u.data[skipTo:skipTo+64<<10]) {
		t.Fatal("bytes differ")
	}
	if r := u.requests(); len(r) != 1 {
		t.Errorf("requests = %v, want the skip to reuse the body", r)
	}
}

func TestRemoteFile_JumpsReopen(t *testing.T) {
	const size = 8 << 20
	u := newUpstream(t, size)
	f, _ := remoteOver(u, size, time.Second)
	defer f.Close()

	readRange(t, f, 0, 32<<10, 32<<10)
	far := int64(5 << 20)
	if got := readRange(t, f, far, far+32<<10, 32<<10); !bytes.Equal(got, u.data[far:far+32<<10]) {
		t.Fatal("bytes after the jump differ")
	}
	back := int64(100)
	if got := readRange(t, f, back, back+1000, 1000); !bytes.Equal(got, u.data[back:back+1000]) {
		t.Fatal("bytes after jumping back differ")
	}
	tail := int64(size - 1000)
	readRange(t, f, tail, size, 32<<10) // a player reading the index at the end
	if r := u.requests(); len(r) != 4 || r[1] != far || r[2] != back || r[3] != tail {
		t.Errorf("requests start at %v, want [0 %d %d %d]", r, far, back, tail)
	}
}

// The CDN drops the connection midway and the link expires with it: the
// read reopens where it stopped, the streamer resolves a fresh link on the
// 404, and the bytes come out whole.
func TestRemoteFile_MidStreamFailureRefreshesAndResumes(t *testing.T) {
	const size = 1 << 20
	u := newUpstream(t, size)
	u.cutAfter, u.expireOnCut = 300_000, true
	f, links := remoteOver(u, size, time.Second)
	defer f.Close()

	got := readRange(t, f, 0, size, 32<<10)
	if !bytes.Equal(got, u.data) {
		t.Fatal("bytes differ after the resume")
	}
	r := u.requests()
	if len(r) != 3 || r[0] != 0 || r[1] != r[2] || r[1] < 32<<10*8 || r[1] > 300_000 {
		t.Errorf("requests start at %v, want 0, then twice at the break (404, then the fresh link)", r)
	}
	if links.invalidated != 1 {
		t.Errorf("refreshes = %d, want 1", links.invalidated)
	}
}

func TestRemoteFile_ReadTimeout(t *testing.T) {
	u := newUpstream(t, 100_000)
	u.stall = true
	f, _ := remoteOver(u, 100_000, 100*time.Millisecond)
	defer f.Close()

	start := time.Now()
	n, err := f.ReadAt(make([]byte, 32<<10), 0)
	if !errors.Is(err, ErrReadTimeout) {
		t.Fatalf("ReadAt = %d, %v; want ErrReadTimeout", n, err)
	}
	if n != 2 { // the byte before each stall
		t.Errorf("n = %d, want 2", n)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("took %v", elapsed)
	}
	if r := u.requests(); len(r) != 2 || r[1] != 1 {
		t.Errorf("requests = %v, want the stall retried once from byte 1", r)
	}
}

func TestRemoteFile_SizeMismatch(t *testing.T) {
	u := newUpstream(t, 100_000)
	f, _ := remoteOver(u, 100_001, time.Second)
	defer f.Close()
	if _, err := f.ReadAt(make([]byte, 100), 0); !errors.Is(err, hdrezka.ErrSizeMismatch) {
		t.Errorf("ReadAt = %v, want ErrSizeMismatch", err)
	}
}

func TestRemoteFile_EOF(t *testing.T) {
	const size = 100_000
	u := newUpstream(t, size)
	f, _ := remoteOver(u, size, time.Second)
	defer f.Close()

	for _, off := range []int64{size, size + 10} {
		if n, err := f.ReadAt(make([]byte, 10), off); n != 0 || err != io.EOF {
			t.Errorf("ReadAt(%d) = %d, %v; want 0, EOF", off, n, err)
		}
	}
	if len(u.requests()) != 0 {
		t.Errorf("reads past the end opened %v", u.requests())
	}

	buf := make([]byte, 32<<10)
	n, err := f.ReadAt(buf, size-10)
	if n != 10 || err != io.EOF || !bytes.Equal(buf[:10], u.data[size-10:]) {
		t.Errorf("ReadAt(last 10) = %d, %v", n, err)
	}
	n, err = f.ReadAt(buf[:10], size-20)
	if n != 10 || err != nil || !bytes.Equal(buf[:10], u.data[size-20:size-10]) {
		t.Errorf("ReadAt(10 before the last 10) = %d, %v", n, err)
	}
	if _, err := f.ReadAt(buf, -1); err == nil {
		t.Error("a negative offset read")
	}
}

func TestRemoteFile_Read(t *testing.T) {
	const size = 300_000
	u := newUpstream(t, size)
	f, _ := remoteOver(u, size, time.Second)
	defer f.Close()

	got, err := io.ReadAll(f)
	if err != nil || !bytes.Equal(got, u.data) {
		t.Fatalf("ReadAll = %d bytes, %v", len(got), err)
	}
	if len(u.requests()) != 1 {
		t.Errorf("requests = %v", u.requests())
	}
}

// fakeBody is a body over a reader that records Close.
type fakeBody struct {
	io.Reader
	closed int
}

func (b *fakeBody) Close() error { b.closed++; return nil }

func TestRemoteFile_Close(t *testing.T) {
	var bodies []*fakeBody
	open := func(ctx context.Context, off int64) (io.ReadCloser, error) {
		b := &fakeBody{Reader: bytes.NewReader(make([]byte, 1000-off))}
		bodies = append(bodies, b)
		return b, nil
	}
	f := NewRemoteFile("f.mp4", 1000, open, time.Second)
	f.ReadAt(make([]byte, 10), 500)
	f.ReadAt(make([]byte, 10), 0) // a jump back closes the first body
	if len(bodies) != 2 || bodies[0].closed != 1 || bodies[1].closed != 0 {
		t.Fatalf("bodies %d, closed %d %d", len(bodies), bodies[0].closed, bodies[1].closed)
	}
	f.Close()
	f.Close()
	if bodies[1].closed != 1 {
		t.Errorf("the open body closed %d times, want 1", bodies[1].closed)
	}
	if _, err := f.ReadAt(make([]byte, 10), 0); !errors.Is(err, os.ErrClosed) {
		t.Errorf("ReadAt after Close = %v", err)
	}
	if len(bodies) != 2 {
		t.Error("a read after Close opened a body")
	}
}

func TestRemoteFile_OpenErrorsAreReturned(t *testing.T) {
	calls := 0
	open := func(ctx context.Context, off int64) (io.ReadCloser, error) {
		calls++
		if _, ok := ctx.Deadline(); !ok {
			t.Error("the open has no deadline")
		}
		return nil, hdrezka.ErrPremium
	}
	f := NewRemoteFile("f.mp4", 1000, open, time.Second)
	for range 2 {
		if _, err := f.ReadAt(make([]byte, 10), 0); !errors.Is(err, hdrezka.ErrPremium) {
			t.Errorf("ReadAt = %v", err)
		}
	}
	if calls != 2 {
		t.Errorf("opens = %d, want one per read, without retrying", calls)
	}
}
