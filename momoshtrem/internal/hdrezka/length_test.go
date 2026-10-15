package hdrezka

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

const fileSize = 3322740623

// cdnServer fakes the CDN: head and get answer HEAD and GET, and every
// request's method and Range header are recorded.
type cdnServer struct {
	*httptest.Server
	mu   sync.Mutex
	seen []string // "HEAD", "GET bytes=0-0"
}

func newCDNServer(t *testing.T, head, get http.HandlerFunc) *cdnServer {
	t.Helper()
	s := &cdnServer{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.seen = append(s.seen, strings.TrimSpace(r.Method+" "+r.Header.Get("Range")))
		s.mu.Unlock()
		if r.Method == http.MethodHead {
			head(w, r)
			return
		}
		get(w, r)
	}))
	t.Cleanup(s.Close)
	return s
}

func lengthClient() (*Client, *[]int) {
	c := NewClient(WithRetries(3, 0))
	var sleeps []int
	c.sleep = func(context.Context, time.Duration) error {
		sleeps = append(sleeps, 1)
		return nil
	}
	return c, &sleeps
}

func withLength(n int64) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", strconv.FormatInt(n, 10))
		w.WriteHeader(http.StatusOK)
	}
}

func withStatus(code int) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(code) }
}

func withContentRange(cr string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Range", cr)
		w.Header().Set("Content-Length", "1")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte{0})
	}
}

func TestContentLength(t *testing.T) {
	tests := []struct {
		name     string
		head     http.HandlerFunc
		get      http.HandlerFunc
		want     int64
		wantErr  error
		wantSeen []string
	}{
		{
			"HEAD has the length",
			withLength(fileSize), withStatus(http.StatusTeapot),
			fileSize, nil, []string{"HEAD"},
		},
		{
			"HEAD without a length falls back to Range",
			withLength(0), withContentRange("bytes 0-0/3322740623"),
			fileSize, nil, []string{"HEAD", "GET bytes=0-0"},
		},
		{
			"HEAD not allowed falls back to Range",
			withStatus(http.StatusMethodNotAllowed), withContentRange("bytes 0-0/3322740623"),
			fileSize, nil, []string{"HEAD", "GET bytes=0-0"},
		},
		{
			"a 200 that ignores Range is an error",
			withLength(0), withLength(fileSize),
			0, ErrRangeIgnored, []string{"HEAD", "GET bytes=0-0"},
		},
		{
			"an unknown total is an error",
			withLength(0), withContentRange("bytes 0-0/*"),
			0, nil, []string{"HEAD", "GET bytes=0-0"},
		},
		{
			"403 on HEAD: the link expired",
			withStatus(http.StatusForbidden), withStatus(http.StatusTeapot),
			0, ErrLinkExpired, []string{"HEAD"},
		},
		{
			"410 on the Range request: the link expired",
			withLength(0), withStatus(http.StatusGone),
			0, ErrLinkExpired, []string{"HEAD", "GET bytes=0-0"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newCDNServer(t, tt.head, tt.get)
			c, _ := lengthClient()

			got, err := c.ContentLength(context.Background(), srv.URL+"/signed-token/file.mp4")

			wantErr := tt.wantErr != nil || tt.want == 0
			switch {
			case !wantErr && err != nil:
				t.Fatalf("ContentLength: %v", err)
			case wantErr && err == nil:
				t.Fatalf("ContentLength = %d, want an error", got)
			case tt.wantErr != nil && !errors.Is(err, tt.wantErr):
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("size = %d, want %d", got, tt.want)
			}
			if !slices.Equal(srv.seen, tt.wantSeen) {
				t.Errorf("requests = %v, want %v", srv.seen, tt.wantSeen)
			}
			if err != nil && strings.Contains(err.Error(), "signed-token") {
				t.Errorf("error leaks the link: %v", err)
			}
		})
	}
}

func TestContentLength_FollowsRedirect(t *testing.T) {
	// The CDN redirects stream.voidboost.one to an edge host.
	edge := newCDNServer(t, withLength(fileSize), withStatus(http.StatusTeapot))
	origin := newCDNServer(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, edge.URL+"/edge/file.mp4", http.StatusFound)
	}, withStatus(http.StatusTeapot))
	c, _ := lengthClient()

	got, err := c.ContentLength(context.Background(), origin.URL+"/signed/file.mp4")
	if err != nil {
		t.Fatalf("ContentLength: %v", err)
	}
	if got != fileSize {
		t.Errorf("size = %d, want %d", got, fileSize)
	}
	if !slices.Equal(edge.seen, []string{"HEAD"}) {
		t.Errorf("edge saw %v, want a HEAD", edge.seen)
	}
}

func TestContentLength_RetriesServerErrors(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	srv := newCDNServer(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls++
		n := calls
		mu.Unlock()
		if n == 1 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		withLength(fileSize)(w, r)
	}, withStatus(http.StatusTeapot))
	c, sleeps := lengthClient()

	got, err := c.ContentLength(context.Background(), srv.URL+"/file.mp4")
	if err != nil || got != fileSize {
		t.Fatalf("ContentLength = %d, %v", got, err)
	}
	if len(*sleeps) != 1 {
		t.Errorf("sleeps = %d, want 1", len(*sleeps))
	}
}

func TestContentLength_TransportErrorHidesLink(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	link := srv.URL + "/0be3bfb771ec26f0:2026101618:dlFOU2k3VzR6/file.mp4"
	srv.Close() // connection refused from here on

	c, _ := lengthClient()
	_, err := c.ContentLength(context.Background(), link)
	if err == nil {
		t.Fatal("want an error from a closed server")
	}
	if strings.Contains(err.Error(), "dlFOU2k3VzR6") || strings.Contains(err.Error(), "file.mp4") {
		t.Errorf("error leaks the signed link: %v", err)
	}
}

func TestParseContentRange(t *testing.T) {
	tests := []struct {
		in                string
		start, end, total int64
		wantErr           bool
	}{
		{"bytes 0-0/3322740623", 0, 0, 3322740623, false},
		{"bytes 1661370311-1661371334/3322740623", 1661370311, 1661371334, 3322740623, false},
		{"bytes 0-1023/*", 0, 1023, -1, false},
		{" bytes 0-0/10 ", 0, 0, 10, false},
		{"bytes */3322740623", 0, 0, 0, true},
		{"bytes 5-4/10", 0, 0, 0, true},
		{"bytes 0-10/10", 0, 0, 0, true},
		{"items 0-1/2", 0, 0, 0, true},
		{"bytes 0-1", 0, 0, 0, true},
		{"", 0, 0, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			start, end, total, err := ParseContentRange(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && (start != tt.start || end != tt.end || total != tt.total) {
				t.Errorf("got %d-%d/%d, want %d-%d/%d", start, end, total, tt.start, tt.end, tt.total)
			}
		})
	}
}
