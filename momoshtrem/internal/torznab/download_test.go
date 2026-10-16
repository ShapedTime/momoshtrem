package torznab

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const (
	testTorrent = "d8:announce35:udp://tracker.opentrackr.org:1337/4:infod4:name4:testee"
	testMagnet  = "magnet:?xt=urn:btih:1f1ad4ac357b893b17c7a3554f292775b6f816f6&dn=test"
)

// jackettLink is a download link as Jackett writes it, pointing at the host
// that answered the search, which isn't necessarily the configured one.
func jackettLink(host string) string {
	return "http://" + host + "/dl/torrentgalaxyclone/?jackett_apikey=" + testKey + "&path=REDACTED&file=Frieren+S01E01"
}

func TestDownload_RebasesOntoTheConfiguredJackett(t *testing.T) {
	var foreign atomic.Int32
	elsewhere := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { foreign.Add(1) }))
	defer elsewhere.Close()

	j := newFakeJackett(t)
	var gotPath, gotQuery string
	j.dl = func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery = r.URL.Path, r.URL.RawQuery
		w.Header().Set("Content-Type", "application/x-bittorrent")
		_, _ = w.Write([]byte(testTorrent))
	}
	c := New(j.URL, testKey)

	// The link names another host (Jackett writes the host the search came
	// in on); only its path and query are used.
	magnet, torrent, err := c.Download(context.Background(), jackettLink(strings.TrimPrefix(elsewhere.URL, "http://")))
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if magnet != "" || string(torrent) != testTorrent {
		t.Errorf("Download = %q, %q", magnet, torrent)
	}
	if gotPath != "/dl/torrentgalaxyclone/" || gotQuery != "jackett_apikey="+testKey+"&path=REDACTED&file=Frieren+S01E01" {
		t.Errorf("request = %s?%s", gotPath, gotQuery)
	}
	if n := foreign.Load(); n != 0 {
		t.Errorf("the link's own host got %d requests", n)
	}
}

func TestDownload_BasePath(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(testTorrent))
	}))
	defer srv.Close()
	c := New(srv.URL+"/jackett", testKey)

	if _, _, err := c.Download(context.Background(), "http://jackett.test/jackett/dl/x/?file=a"); err != nil {
		t.Fatalf("Download: %v", err)
	}
	if gotPath != "/jackett/dl/x/" {
		t.Errorf("path = %q", gotPath)
	}
	if _, _, err := c.Download(context.Background(), "http://jackett.test/dl/x/?file=a"); !errors.Is(err, ErrInvalidLink) {
		t.Errorf("a link outside the base path: err = %v, want ErrInvalidLink", err)
	}
}

func TestDownload_Redirects(t *testing.T) {
	var followed atomic.Int32
	elsewhere := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { followed.Add(1) }))
	defer elsewhere.Close()

	tests := []struct {
		name       string
		location   string
		wantMagnet string
		wantErr    error
	}{
		{"to a magnet link", testMagnet, testMagnet, nil},
		{"to another host is refused", elsewhere.URL + "/file.torrent", "", ErrRedirect},
		{"to Jackett itself is refused too", "/dl/other/", "", ErrRedirect},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			j := newFakeJackett(t)
			j.dl = func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r, tt.location, http.StatusFound)
			}
			magnet, torrent, err := New(j.URL, testKey).Download(context.Background(), jackettLink("jackett.test"))
			if !errors.Is(err, tt.wantErr) || magnet != tt.wantMagnet || torrent != nil {
				t.Fatalf("Download = %q, %d bytes, %v; want %q, %v", magnet, len(torrent), err, tt.wantMagnet, tt.wantErr)
			}
		})
	}
	if n := followed.Load(); n != 0 {
		t.Errorf("a redirect was followed %d times", n)
	}
}

func TestDownload_Body(t *testing.T) {
	tests := []struct {
		name        string
		status      int
		body        []byte
		wantMagnet  string
		wantTorrent bool
		wantErr     func(error) bool
	}{
		{"exactly 5 MB", 200, bytes.Repeat([]byte("x"), maxTorrent), "", true, nil},
		{"over 5 MB", 200, bytes.Repeat([]byte("x"), maxTorrent+1), "", false,
			func(err error) bool { return errors.Is(err, ErrTooLarge) }},
		{"a magnet as text", 200, []byte(testMagnet + "\n"), testMagnet, false, nil},
		{"a Torznab error document", 200, fixture(t, "error_indexer.xml"), "", false,
			func(err error) bool { var e *Error; return errors.As(err, &e) && e.Code == 900 }},
		{"an HTTP error", 500, []byte("boom"), "", false,
			func(err error) bool { var e *HTTPError; return errors.As(err, &e) && e.StatusCode == 500 }},
		{"empty", 200, nil, "", false, func(err error) bool { return err != nil }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			j := newFakeJackett(t)
			j.dl = func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write(tt.body)
			}
			magnet, torrent, err := New(j.URL, testKey).Download(context.Background(), jackettLink("jackett.test"))
			if tt.wantErr != nil {
				if !tt.wantErr(err) {
					t.Fatalf("err = %v (%T)", err, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Download: %v", err)
			}
			if magnet != tt.wantMagnet || (torrent != nil) != tt.wantTorrent {
				t.Errorf("Download = %q, %d bytes", magnet, len(torrent))
			}
		})
	}
}

func TestDownload_InvalidLinks(t *testing.T) {
	j := newFakeJackett(t)
	c := New(j.URL, testKey)
	for _, link := range []string{
		"",
		"http://jackett.test/api/v2.0/server/config?apikey=x", // not the /dl/ proxy
		"http://jackett.test/dl/../api/v2.0/server/config",
		"file:///etc/passwd",
		"http://[::1",
	} {
		if _, _, err := c.Download(context.Background(), link); !errors.Is(err, ErrInvalidLink) {
			t.Errorf("Download(%q): err = %v, want ErrInvalidLink", link, err)
		}
	}
	if n := len(j.requests); n != 0 {
		t.Errorf("%d requests for invalid links", n)
	}
	// A magnet link needs no request.
	if magnet, _, err := c.Download(context.Background(), testMagnet); err != nil || magnet != testMagnet {
		t.Errorf("Download(magnet) = %q, %v", magnet, err)
	}
}

func TestDownload_Timeout(t *testing.T) {
	j := newFakeJackett(t)
	j.dl = func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }
	c := New(j.URL, testKey, WithTimeouts(0, 50*time.Millisecond))

	if _, _, err := c.Download(context.Background(), jackettLink("jackett.test")); !errors.Is(err, ErrTimeout) {
		t.Fatalf("err = %v, want ErrTimeout", err)
	}
}
