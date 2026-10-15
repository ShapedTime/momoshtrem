package vfs_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/shapedtime/momoshtrem/internal/config"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/vfs"
	"github.com/shapedtime/momoshtrem/internal/webdav"
)

// recordingSource is a remote source over in-memory files, recording each
// open. With fail set, every open fails.
type recordingSource struct {
	files map[string][]byte // by the stream's file path
	fail  bool

	mu    sync.Mutex
	opens []int64
}

func (s *recordingSource) OpenRemote(_ context.Context, a *library.TorrentAssignment, off int64) (io.ReadCloser, error) {
	s.mu.Lock()
	s.opens = append(s.opens, off)
	s.mu.Unlock()
	if s.fail {
		return nil, errors.New("the CDN is down")
	}
	return io.NopCloser(bytes.NewReader(s.files[a.Stream.FilePath()][off:])), nil
}

func (s *recordingSource) calls() []int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]int64(nil), s.opens...)
}

func hdrezkaRow(ref library.StreamRef, size int64) *library.TorrentAssignment {
	return &library.TorrentAssignment{Kind: library.AssignmentKindHDRezka, Stream: &ref,
		FilePath: ref.FilePath(), FileSize: size, IsActive: true}
}

// newWebDAV serves a library with an HDRezka movie and episode over WebDAV
// (no torrent client), streaming from src.
func newWebDAV(t *testing.T, src *recordingSource) (*httptest.Server, []byte) {
	t.Helper()
	movie := library.StreamRef{Provider: "hdrezka", Path: "/films/fiction/2259-interstellar-2014.html", PostID: 2259,
		TranslatorID: 238, TranslatorName: "Оригинал (+субтитры)", Quality: "1080p"}
	episode := library.StreamRef{Provider: "hdrezka", Path: "/series/thriller/646-vo-vse-tyazhkie-2008-latest.html", PostID: 646,
		TranslatorID: 238, Season: 1, Episode: 1, Quality: "1080p"}

	data := make([]byte, 3<<20)
	for i := range data {
		data[i] = byte(i*13 + i/7919)
	}
	src.files = map[string][]byte{movie.FilePath(): data, episode.FilePath(): data[:1<<20]}

	fs := vfs.NewTestFS(
		[]vfs.TestMovie{{Title: "Interstellar", Year: 2014, Assignment: hdrezkaRow(movie, int64(len(data)))}},
		[]vfs.TestEpisode{{Show: "Breaking Bad", Year: 2008, Season: 1, Episode: 1, Name: "Pilot",
			Assignment: hdrezkaRow(episode, 1<<20)}},
	)
	fs.SetRemoteSource(src, 0)
	srv := httptest.NewServer(webdav.NewServer(fs, config.WebDAVAuthConfig{}).Handler())
	t.Cleanup(srv.Close)
	return srv, data
}

func davRequest(t *testing.T, method, u string, header map[string]string) (*http.Response, []byte) {
	t.Helper()
	req, err := http.NewRequest(method, u, nil)
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range header {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp, body
}

const moviePath = "/Movies/Interstellar (2014)/Interstellar (2014).mp4"

func escape(p string) string { return (&url.URL{Path: p}).EscapedPath() }

// A PROPFIND opens every file it lists: an HDRezka file must open without
// touching the CDN, even when the CDN is down.
func TestWebDAV_PropfindMakesNoUpstreamCalls(t *testing.T) {
	src := &recordingSource{fail: true}
	srv, data := newWebDAV(t, src)

	for _, dir := range []string{"/Movies/", "/Movies/Interstellar (2014)/", "/TV Shows/Breaking Bad (2008)/Season 01/"} {
		resp, body := davRequest(t, "PROPFIND", srv.URL+escape(dir), map[string]string{"Depth": "1"})
		if resp.StatusCode != http.StatusMultiStatus {
			t.Fatalf("PROPFIND %s = %d: %s", dir, resp.StatusCode, body)
		}
		if dir == "/Movies/Interstellar (2014)/" {
			for _, want := range []string{"Interstellar%20%282014%29.mp4", "<D:getcontentlength>" + strconv.Itoa(len(data)) + "</D:getcontentlength>", "video/mp4"} {
				if !strings.Contains(string(body), want) {
					t.Errorf("PROPFIND %s lacks %s:\n%s", dir, want, body)
				}
			}
		}
		if dir == "/TV Shows/Breaking Bad (2008)/Season 01/" && !strings.Contains(string(body), "S01E01") {
			t.Errorf("PROPFIND %s lacks the episode:\n%s", dir, body)
		}
	}
	resp, _ := davRequest(t, "PROPFIND", srv.URL+escape(moviePath), map[string]string{"Depth": "0"})
	if resp.StatusCode != http.StatusMultiStatus {
		t.Errorf("PROPFIND of the file = %d", resp.StatusCode)
	}
	if calls := src.calls(); len(calls) != 0 {
		t.Errorf("upstream calls = %v, want none", calls)
	}
}

func TestWebDAV_RangeGetMakesOneUpstreamCall(t *testing.T) {
	src := &recordingSource{}
	srv, data := newWebDAV(t, src)

	const from, to = 1_000_000, 1_001_023
	resp, body := davRequest(t, http.MethodGet, srv.URL+escape(moviePath), map[string]string{"Range": "bytes=1000000-1001023"})
	if resp.StatusCode != http.StatusPartialContent {
		t.Fatalf("GET = %d", resp.StatusCode)
	}
	if !bytes.Equal(body, data[from:to+1]) {
		t.Errorf("body = %d bytes, not bytes %d-%d", len(body), from, to)
	}
	if got := resp.Header.Get("Content-Range"); got != "bytes 1000000-1001023/"+strconv.Itoa(len(data)) {
		t.Errorf("Content-Range = %q", got)
	}
	if calls := src.calls(); len(calls) != 1 || calls[0] != from {
		t.Errorf("upstream calls = %v, want one from %d", calls, from)
	}

	// The last bytes, as a player reads the index.
	tail := len(data) - 5000
	resp, body = davRequest(t, http.MethodGet, srv.URL+escape(moviePath), map[string]string{"Range": "bytes=" + strconv.Itoa(tail) + "-"})
	if resp.StatusCode != http.StatusPartialContent || !bytes.Equal(body, data[tail:]) {
		t.Errorf("GET of the tail = %d, %d bytes", resp.StatusCode, len(body))
	}
	if calls := src.calls(); len(calls) != 2 || calls[1] != int64(tail) {
		t.Errorf("upstream calls = %v", calls)
	}
}

func TestWebDAV_HeadMakesNoUpstreamCalls(t *testing.T) {
	src := &recordingSource{}
	srv, data := newWebDAV(t, src)

	resp, _ := davRequest(t, http.MethodHead, srv.URL+escape(moviePath), nil)
	if resp.StatusCode != http.StatusOK || resp.ContentLength != int64(len(data)) {
		t.Errorf("HEAD = %d, Content-Length %d; want 200, %d", resp.StatusCode, resp.ContentLength, len(data))
	}
	if ct := resp.Header.Get("Content-Type"); ct != "video/mp4" {
		t.Errorf("Content-Type = %q", ct)
	}
	if calls := src.calls(); len(calls) != 0 {
		t.Errorf("upstream calls = %v, want none", calls)
	}
}

func TestWebDAV_WholeEpisode(t *testing.T) {
	src := &recordingSource{}
	srv, data := newWebDAV(t, src)

	resp, body := davRequest(t, http.MethodGet, srv.URL+escape("/TV Shows/Breaking Bad (2008)/Season 01/Breaking Bad - S01E01 - Pilot.mp4"), nil)
	if resp.StatusCode != http.StatusOK || !bytes.Equal(body, data[:1<<20]) {
		t.Errorf("GET = %d, %d bytes", resp.StatusCode, len(body))
	}
	if calls := src.calls(); len(calls) != 1 || calls[0] != 0 {
		t.Errorf("upstream calls = %v", calls)
	}
}

// With HDRezka off the file still lists; reading it fails.
func TestWebDAV_NoRemoteSource(t *testing.T) {
	ref := library.StreamRef{Provider: "hdrezka", PostID: 2259, TranslatorID: 238, Quality: "1080p"}
	fs := vfs.NewTestFS([]vfs.TestMovie{{Title: "Interstellar", Year: 2014, Assignment: hdrezkaRow(ref, 5000)}}, nil)
	srv := httptest.NewServer(webdav.NewServer(fs, config.WebDAVAuthConfig{}).Handler())
	defer srv.Close()

	resp, _ := davRequest(t, "PROPFIND", srv.URL+escape("/Movies/Interstellar (2014)/"), map[string]string{"Depth": "1"})
	if resp.StatusCode != http.StatusMultiStatus {
		t.Errorf("PROPFIND = %d", resp.StatusCode)
	}
	f, err := fs.Open(moviePath)
	if err != nil {
		t.Fatalf("Open = %v", err)
	}
	defer f.Close()
	if _, err := f.ReadAt(make([]byte, 10), 0); !errors.Is(err, vfs.ErrRemoteUnavailable) {
		t.Errorf("ReadAt = %v, want ErrRemoteUnavailable", err)
	}
}
