package hdrezka

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// recorded is what the test server saw of a request.
type recorded struct {
	method, path string
	query, form  url.Values
	header       http.Header
}

// siteServer answers every request with handler and records it.
type siteServer struct {
	*httptest.Server
	mu       sync.Mutex
	requests []recorded
}

func newSiteServer(t *testing.T, handler func(w http.ResponseWriter, r *http.Request)) *siteServer {
	t.Helper()
	s := &siteServer{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse form: %v", err)
		}
		s.mu.Lock()
		s.requests = append(s.requests, recorded{r.Method, r.URL.Path, r.URL.Query(), r.PostForm, r.Header.Clone()})
		s.mu.Unlock()
		handler(w, r)
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *siteServer) only(t *testing.T) recorded {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.requests) != 1 {
		t.Fatalf("server saw %d requests, want 1", len(s.requests))
	}
	return s.requests[0]
}

// serve answers with a fixture.
func serve(t *testing.T, name string) func(http.ResponseWriter, *http.Request) {
	body := fixture(t, name)
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(body)
	}
}

// testClient talks to srv without a rate limit, records its sleeps and
// stamps CDN requests with a fixed time.
func testClient(srv *siteServer, opts ...Option) (*Client, *[]time.Duration) {
	c := NewClient(append([]Option{WithBaseURL(srv.URL), WithRateLimit(0, 0)}, opts...)...)
	var slept []time.Duration
	c.sleep = func(_ context.Context, d time.Duration) error {
		slept = append(slept, d)
		return nil
	}
	c.now = func() time.Time { return time.UnixMilli(1792094252123) }
	return c, &slept
}

func TestDefaultUserAgentIsNotABrowser(t *testing.T) {
	// Anubis challenges user agents that start with Mozilla.
	if strings.HasPrefix(DefaultUserAgent, "Mozilla") || DefaultUserAgent == "" {
		t.Errorf("DefaultUserAgent = %q", DefaultUserAgent)
	}
}

func TestClient_QuickSearchRequest(t *testing.T) {
	srv := newSiteServer(t, serve(t, "quick_search_interstellar.html"))
	c, _ := testClient(srv)

	results, err := c.QuickSearch(context.Background(), "interstellar 2014")
	if err != nil {
		t.Fatalf("QuickSearch: %v", err)
	}
	if len(results) != 5 || results[0].PostID != 2259 {
		t.Errorf("results = %+v", results)
	}
	r := srv.only(t)
	if r.method != http.MethodPost || r.path != "/engine/ajax/search.php" {
		t.Errorf("request = %s %s", r.method, r.path)
	}
	if got := r.form.Get("q"); got != "interstellar 2014" {
		t.Errorf("q = %q", got)
	}
	if got := r.header.Get("X-Requested-With"); got != "XMLHttpRequest" {
		t.Errorf("X-Requested-With = %q", got)
	}
	if got := r.header.Get("User-Agent"); got != DefaultUserAgent {
		t.Errorf("User-Agent = %q", got)
	}
}

func TestClient_SearchRequest(t *testing.T) {
	srv := newSiteServer(t, serve(t, "search_interstellar.html"))
	c, _ := testClient(srv)

	if _, err := c.Search(context.Background(), "interstellar"); err != nil {
		t.Fatalf("Search: %v", err)
	}
	r := srv.only(t)
	if r.method != http.MethodGet || r.path != "/search/" {
		t.Errorf("request = %s %s", r.method, r.path)
	}
	want := url.Values{"do": {"search"}, "subaction": {"search"}, "q": {"interstellar"}}
	if r.query.Encode() != want.Encode() {
		t.Errorf("query = %v, want %v", r.query, want)
	}
}

func TestClient_Title(t *testing.T) {
	srv := newSiteServer(t, serve(t, "film_interstellar.html"))
	c, _ := testClient(srv)

	// A URL on another mirror: only its path is fetched, from our base URL.
	title, err := c.Title(context.Background(), "https://hdrezka.me/films/fiction/2259-interstellar-2014.html")
	if err != nil {
		t.Fatalf("Title: %v", err)
	}
	if title.PostID != 2259 || title.Path != "/films/fiction/2259-interstellar-2014.html" {
		t.Errorf("title = %+v", title)
	}
	if r := srv.only(t); r.path != "/films/fiction/2259-interstellar-2014.html" {
		t.Errorf("fetched %s", r.path)
	}
}

func TestClient_TitleRejectsOtherPaths(t *testing.T) {
	srv := newSiteServer(t, serve(t, "film_interstellar.html"))
	c, _ := testClient(srv)
	if _, err := c.Title(context.Background(), "https://evil.example/admin"); !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("error = %v, want ErrInvalidPath", err)
	}
	if len(srv.requests) != 0 {
		t.Errorf("made %d requests, want 0", len(srv.requests))
	}
}

var interstellarRef = Ref{PostID: 2259, Path: "/films/fiction/2259-interstellar-2014.html"}

func TestClient_MovieRequest(t *testing.T) {
	srv := newSiteServer(t, serve(t, "cdn_get_movie.json"))
	c, _ := testClient(srv)

	stream, err := c.Movie(context.Background(), interstellarRef, Translator{ID: 238, Camrip: true, Director: true})
	if err != nil {
		t.Fatalf("Movie: %v", err)
	}
	r := srv.only(t)
	if r.method != http.MethodPost || r.path != "/ajax/get_cdn_series/" {
		t.Errorf("request = %s %s", r.method, r.path)
	}
	if got := r.query.Get("t"); got != "1792094252123" {
		t.Errorf("t = %q, want the time in milliseconds", got)
	}
	want := url.Values{
		"id": {"2259"}, "translator_id": {"238"},
		"is_camrip": {"1"}, "is_ads": {"0"}, "is_director": {"1"},
		"action": {"get_movie"},
	}
	if r.form.Encode() != want.Encode() {
		t.Errorf("form = %v, want %v", r.form, want)
	}
	checkCDNHeaders(t, r, srv.URL+interstellarRef.Path)

	if best, ok := stream.Best(); !ok || best.Label != "1080p" || best.MP4 != "https://cdn.example/stream/xdclm.mp4" {
		t.Errorf("best = %+v, %v", best, ok)
	}
	if len(stream.Subtitles) != 2 || stream.Subtitles[1].Code != "en" || stream.Legacy {
		t.Errorf("stream = %+v", stream)
	}
}

func TestClient_EpisodesRequest(t *testing.T) {
	srv := newSiteServer(t, serve(t, "cdn_get_episodes.json"))
	c, _ := testClient(srv)

	ref := Ref{PostID: 646, Path: "/series/thriller/646-vo-vse-tyazhkie-2008-latest.html"}
	seasons, err := c.Episodes(context.Background(), ref, 238)
	if err != nil {
		t.Fatalf("Episodes: %v", err)
	}
	r := srv.only(t)
	want := url.Values{"id": {"646"}, "translator_id": {"238"}, "action": {"get_episodes"}}
	if r.form.Encode() != want.Encode() {
		t.Errorf("form = %v, want %v", r.form, want)
	}
	checkCDNHeaders(t, r, srv.URL+ref.Path)
	if len(seasons) != 5 || seasons[0].Number != 1 || len(seasons[0].Episodes) != 7 || len(seasons[4].Episodes) != 16 {
		t.Errorf("seasons = %+v", seasons)
	}
}

func TestClient_EpisodeRequest(t *testing.T) {
	srv := newSiteServer(t, serve(t, "cdn_get_stream.json"))
	c, _ := testClient(srv)

	ref := Ref{PostID: 646, Path: "/series/thriller/646-vo-vse-tyazhkie-2008-latest.html"}
	stream, err := c.Episode(context.Background(), ref, 238, 1, 2)
	if err != nil {
		t.Fatalf("Episode: %v", err)
	}
	r := srv.only(t)
	want := url.Values{"id": {"646"}, "translator_id": {"238"}, "season": {"1"}, "episode": {"2"}, "action": {"get_stream"}}
	if r.form.Encode() != want.Encode() {
		t.Errorf("form = %v, want %v", r.form, want)
	}
	if len(stream.Qualities) != 7 || len(stream.Subtitles) != 3 || stream.Subtitles[1].Code != "uk" {
		t.Errorf("stream = %+v", stream)
	}
}

func checkCDNHeaders(t *testing.T, r recorded, referer string) {
	t.Helper()
	for name, want := range map[string]string{
		"X-Requested-With": "XMLHttpRequest",
		"Referer":          referer,
		"Content-Type":     "application/x-www-form-urlencoded; charset=UTF-8",
		"User-Agent":       DefaultUserAgent,
	} {
		if got := r.header.Get(name); got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
}

func TestClient_CDNAnswers(t *testing.T) {
	tests := []struct {
		name  string
		body  string
		check func(t *testing.T, err error)
	}{
		{"success=false", string(fixture(t, "cdn_failure.json")), func(t *testing.T, err error) {
			var cdnErr *CDNError
			if !errors.As(err, &cdnErr) || !strings.HasPrefix(cdnErr.Message, "Время сессии истекло") {
				t.Errorf("error = %v, want a CDNError with the site's message", err)
			}
		}},
		{"premium translation", string(fixture(t, "cdn_premium.json")), func(t *testing.T, err error) {
			if !errors.Is(err, ErrPremium) {
				t.Errorf("error = %v, want ErrPremium", err)
			}
		}},
		{"HLS only", `{"success":true,"premium_content":0,"url":"[720p]https:\/\/cdn.example\/a\/index.m3u8"}`, func(t *testing.T, err error) {
			if !errors.Is(err, ErrNoMP4) {
				t.Errorf("error = %v, want ErrNoMP4", err)
			}
		}},
		{"no stream", `{"success":true,"premium_content":0,"url":false}`, func(t *testing.T, err error) {
			if !errors.Is(err, ErrNotFound) {
				t.Errorf("error = %v, want ErrNotFound", err)
			}
		}},
		{"not JSON", `<html>maintenance</html>`, func(t *testing.T, err error) {
			var perr *ParseError
			if !errors.As(err, &perr) {
				t.Errorf("error = %v, want a ParseError", err)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newSiteServer(t, func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(tt.body))
			})
			c, _ := testClient(srv)
			_, err := c.Movie(context.Background(), interstellarRef, Translator{ID: 56})
			tt.check(t, err)
		})
	}
}

func TestClient_Challenge(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
	}{
		{"Anubis answers 200", http.StatusOK, string(fixture(t, "challenge.html"))},
		{"Cloudflare answers 503", http.StatusServiceUnavailable, `<!DOCTYPE html><html><head><title>Just a moment...</title></head></html>`},
		{"DDoS-Guard answers 403", http.StatusForbidden, `<html><head><title>DDoS-Guard</title></head></html>`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newSiteServer(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			})
			c, slept := testClient(srv)
			_, err := c.Title(context.Background(), interstellarRef.Path)
			if !errors.Is(err, ErrChallenge) {
				t.Fatalf("error = %v, want ErrChallenge", err)
			}
			if len(srv.requests) != 1 || len(*slept) != 0 {
				t.Errorf("%d requests and %d sleeps, want 1 and 0: a challenge isn't retried", len(srv.requests), len(*slept))
			}
		})
	}
}

func TestClient_Retries(t *testing.T) {
	tests := []struct {
		name      string
		statuses  []int
		wantCalls int32
		wantErr   error // nil means success
	}{
		{"5xx then success", []int{502, 503, 200}, 3, nil},
		{"429 then success", []int{429, 200}, 2, nil},
		{"gives up after three attempts", []int{500}, 3, &StatusError{StatusCode: 500}},
		{"404 is not retried", []int{404, 200}, 1, ErrNotFound},
		{"403 is not retried", []int{403, 200}, 1, &StatusError{StatusCode: 403}},
	}
	page := fixture(t, "quick_search_interstellar.html")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls atomic.Int32
			srv := newSiteServer(t, func(w http.ResponseWriter, _ *http.Request) {
				n := int(calls.Add(1)) - 1
				status := tt.statuses[min(n, len(tt.statuses)-1)]
				w.WriteHeader(status)
				if status == http.StatusOK {
					_, _ = w.Write(page)
				}
			})
			c, slept := testClient(srv, WithRetries(3, 100*time.Millisecond))

			_, err := c.QuickSearch(context.Background(), "interstellar")

			if got := calls.Load(); got != tt.wantCalls {
				t.Errorf("requests = %d, want %d", got, tt.wantCalls)
			}
			if len(*slept) != int(tt.wantCalls)-1 {
				t.Errorf("sleeps = %v, want %d", *slept, tt.wantCalls-1)
			}
			for i, d := range *slept {
				if d <= 0 || d > 100*time.Millisecond<<i {
					t.Errorf("sleep %d = %v, want backoff within (0, %v]", i, d, 100*time.Millisecond<<i)
				}
			}
			var wantStatus *StatusError
			switch {
			case tt.wantErr == nil:
				if err != nil {
					t.Errorf("error = %v, want success", err)
				}
			case errors.As(tt.wantErr, &wantStatus):
				var got *StatusError
				if !errors.As(err, &got) || got.StatusCode != wantStatus.StatusCode {
					t.Errorf("error = %v, want status %d", err, wantStatus.StatusCode)
				}
			case !errors.Is(err, tt.wantErr):
				t.Errorf("error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestClient_RetriesTimeouts(t *testing.T) {
	var calls atomic.Int32
	page := fixture(t, "quick_search_interstellar.html")
	srv := newSiteServer(t, func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			time.Sleep(300 * time.Millisecond) // longer than the client timeout
		}
		_, _ = w.Write(page)
	})
	c, _ := testClient(srv, WithHTTPClient(&http.Client{Timeout: 50 * time.Millisecond}))

	if _, err := c.QuickSearch(context.Background(), "interstellar"); err != nil {
		t.Fatalf("QuickSearch: %v", err)
	}
	if calls.Load() != 2 {
		t.Errorf("requests = %d, want 2", calls.Load())
	}
}

func TestClient_KeepsCookies(t *testing.T) {
	var sawCookie atomic.Bool
	srv := newSiteServer(t, func(w http.ResponseWriter, r *http.Request) {
		if _, err := r.Cookie("PHPSESSID"); err == nil {
			sawCookie.Store(true)
		}
		http.SetCookie(w, &http.Cookie{Name: "PHPSESSID", Value: "abc", Path: "/"})
		_, _ = w.Write(fixture(t, "quick_search_interstellar.html"))
	})
	c, _ := testClient(srv)
	for range 2 {
		if _, err := c.QuickSearch(context.Background(), "x"); err != nil {
			t.Fatal(err)
		}
	}
	if !sawCookie.Load() {
		t.Error("the second request didn't send the session cookie back")
	}
}
