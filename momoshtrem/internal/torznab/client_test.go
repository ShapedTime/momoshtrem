package torznab

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

const testKey = "jackett-secret-key"

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return b
}

// fakeJackett answers the indexer list from a fixture and each indexer's
// searches from its handler, checking the API key, and records requests.
type fakeJackett struct {
	*httptest.Server
	mu       sync.Mutex
	requests []*url.URL
	indexers []byte                      // the t=indexers answer
	search   map[string]http.HandlerFunc // by indexer id
	dl       http.HandlerFunc            // /dl/…
}

func newFakeJackett(t *testing.T) *fakeJackett {
	t.Helper()
	j := &fakeJackett{indexers: fixture(t, "indexers.xml"), search: map[string]http.HandlerFunc{}}
	apiKeyError := fixture(t, "error_apikey.xml")
	j.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		j.mu.Lock()
		j.requests = append(j.requests, r.URL)
		indexers, dl := j.indexers, j.dl
		j.mu.Unlock()

		if strings.HasPrefix(r.URL.Path, "/dl/") && dl != nil {
			dl(w, r)
			return
		}
		if r.URL.Query().Get("apikey") != testKey {
			_, _ = w.Write(apiKeyError) // Jackett answers a bad key with HTTP 200
			return
		}
		id, ok := strings.CutPrefix(r.URL.Path, "/api/v2.0/indexers/")
		id, ok2 := strings.CutSuffix(id, "/results/torznab/api")
		if !ok || !ok2 {
			http.NotFound(w, r)
			return
		}
		if id == "all" && r.URL.Query().Get("t") == "indexers" {
			_, _ = w.Write(indexers)
			return
		}
		j.mu.Lock()
		h := j.search[id]
		j.mu.Unlock()
		if h == nil {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write(fixture(t, "error_unknown_indexer.json"))
			return
		}
		h(w, r)
	}))
	t.Cleanup(j.Close)
	return j
}

func (j *fakeJackett) handle(id string, h http.HandlerFunc) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.search[id] = h
}

// searches returns the requests to indexer id's Torznab endpoint.
func (j *fakeJackett) searches(id string) []url.Values {
	j.mu.Lock()
	defer j.mu.Unlock()
	var out []url.Values
	for _, u := range j.requests {
		if u.Path == apiPath(id) {
			out = append(out, u.Query())
		}
	}
	return out
}

func serveFixture(t *testing.T, name string) http.HandlerFunc {
	body := fixture(t, name)
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write(body)
	}
}

// fakeClock is a manually advanced clock.
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func newFakeClock() *fakeClock {
	return &fakeClock{t: time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

func TestIndexers_ParsesCaps(t *testing.T) {
	j := newFakeJackett(t)
	c := New(j.URL+"/", testKey) // a trailing slash must not produce "//api"

	list, err := c.Indexers(context.Background())
	if err != nil {
		t.Fatalf("Indexers: %v", err)
	}
	var ids []string
	for _, ix := range list {
		ids = append(ids, ix.ID)
	}
	want := []string{"rutracker", "torrentgalaxyclone", "1337x", "thepiratebay", "limetorrents", "therarbg", "eztv"}
	if !slices.Equal(ids, want) {
		t.Fatalf("ids = %v, want %v", ids, want)
	}

	q := j.searches("all")[0]
	if q.Get("t") != "indexers" || q.Get("configured") != "true" || q.Get("apikey") != testKey {
		t.Errorf("request query = %v", q)
	}

	byID := map[string]Indexer{}
	for _, ix := range list {
		byID[ix.ID] = ix
	}
	ru := byID["rutracker"]
	if ru.Title != "RuTracker.org" || ru.Type != "semi-private" || ru.Caps.LimitMax != 100 {
		t.Errorf("rutracker = %+v", ru)
	}
	if !slices.Equal(ru.Caps.TVSearch.Params, []string{"q", "season", "ep"}) || !ru.Caps.TVSearch.Available {
		t.Errorf("rutracker tv-search = %+v", ru.Caps.TVSearch)
	}
	// Top-level, subcategory and the indexer's own categories.
	for _, id := range []int{2000, 5000, 5070, 2045} {
		if !ru.Caps.HasCategory(id) {
			t.Errorf("rutracker lacks category %d", id)
		}
	}
	if ru.Caps.HasCategory(6000) {
		t.Error("rutracker declares no 6000 category")
	}
	if !byID["torrentgalaxyclone"].Caps.Supports(ModeMovie, "imdbid") || byID["torrentgalaxyclone"].Caps.Supports(ModeTV, "imdbid") {
		t.Errorf("torrentgalaxyclone caps = %+v", byID["torrentgalaxyclone"].Caps)
	}
	eztv := byID["eztv"]
	if eztv.Caps.MovieSearch.Available || eztv.Caps.Supports(ModeMovie, "q") {
		t.Errorf("eztv movie-search = %+v, want unavailable", eztv.Caps.MovieSearch)
	}
	if !slices.Equal(eztv.Caps.Categories, []int{5000}) {
		t.Errorf("eztv categories = %v", eztv.Caps.Categories)
	}
}

func TestIndexers_CachedForTTL(t *testing.T) {
	j := newFakeJackett(t)
	clock := newFakeClock()
	c := New(j.URL, testKey, WithClock(clock.Now))
	ctx := context.Background()

	check := func(name string, want int) {
		t.Helper()
		if got := len(j.searches("all")); got != want {
			t.Fatalf("%s: %d list requests, want %d", name, got, want)
		}
	}
	if _, err := c.Indexers(ctx); err != nil {
		t.Fatal(err)
	}
	check("first call fetches", 1)
	clock.Advance(IndexersTTL - time.Second)
	_, _ = c.Indexers(ctx)
	check("cached within the TTL", 1)
	_, _ = c.FetchIndexers(ctx)
	check("FetchIndexers always asks", 2)
	clock.Advance(IndexersTTL)
	_, _ = c.Indexers(ctx)
	check("expired after the TTL", 3)
}

// deadlineRecorder is a transport that records how long each kind of
// request had left when it was sent: "list" (t=indexers), "download" (/dl/)
// or "search".
type deadlineRecorder struct {
	mu   sync.Mutex
	left map[string]time.Duration
}

func (d *deadlineRecorder) RoundTrip(r *http.Request) (*http.Response, error) {
	kind := "search"
	switch {
	case r.URL.Query().Get("t") == "indexers":
		kind = "list"
	case strings.HasPrefix(r.URL.Path, "/dl/"):
		kind = "download"
	}
	if deadline, ok := r.Context().Deadline(); ok {
		d.mu.Lock()
		d.left[kind] = time.Until(deadline)
		d.mu.Unlock()
	}
	return http.DefaultTransport.RoundTrip(r)
}

// A search waits out FlareSolverr's challenge, the indexer list doesn't
// (#106).
func TestTimeouts_PerRequestKind(t *testing.T) {
	tests := []struct {
		name                   string
		opts                   []Option
		list, search, download time.Duration
	}{
		{"defaults", nil, IndexersTimeout, DefaultSearchTimeout, DefaultDownloadTimeout},
		{"WithTimeouts leaves the list alone", []Option{WithTimeouts(2*time.Second, 3*time.Second)},
			IndexersTimeout, 2 * time.Second, 3 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			j := newFakeJackett(t)
			j.handle("thepiratebay", serveFixture(t, "search_thepiratebay.xml"))
			j.dl = func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, testMagnet, http.StatusFound) }
			rec := &deadlineRecorder{left: map[string]time.Duration{}}
			c := New(j.URL, testKey, append([]Option{WithHTTPClient(&http.Client{Transport: rec})}, tt.opts...)...)

			ctx := context.Background()
			if _, err := c.Search(ctx, "thepiratebay", Query{Q: "Frieren"}); err != nil {
				t.Fatal(err)
			}
			if _, _, err := c.Download(ctx, jackettLink("jackett.test")); err != nil {
				t.Fatal(err)
			}
			for kind, want := range map[string]time.Duration{"list": tt.list, "search": tt.search, "download": tt.download} {
				if got, ok := rec.left[kind]; !ok || got <= want-time.Second || got > want {
					t.Errorf("%s request had %v left, want %v", kind, got, want)
				}
			}
		})
	}
}

func TestFetchIndexers_SharedRequestAndCallerDeadline(t *testing.T) {
	release := make(chan struct{})
	body := fixture(t, "indexers.xml")
	var mu sync.Mutex
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		requests++
		mu.Unlock()
		<-release
		_, _ = w.Write(body)
	}))
	defer srv.Close()
	c := New(srv.URL, testKey)

	// A caller with a short deadline stops waiting; the shared request
	// carries on for the others.
	short, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := c.FetchIndexers(short); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("short caller err = %v, want deadline exceeded", err)
	}

	var wg sync.WaitGroup
	for range 5 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if list, err := c.FetchIndexers(context.Background()); err != nil || len(list) != 7 {
				t.Errorf("FetchIndexers = %d, %v", len(list), err)
			}
		}()
	}
	time.Sleep(20 * time.Millisecond) // let them join the request in flight
	close(release)
	wg.Wait()
	mu.Lock()
	defer mu.Unlock()
	if requests != 1 {
		t.Errorf("%d list requests, want 1 shared", requests)
	}
}

func TestIndexers_Errors(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		check   func(error) bool
		wantMsg string
	}{
		{"torznab error document", 200, string(fixture(t, "error_apikey.xml")),
			func(err error) bool { var e *Error; return errors.As(err, &e) && e.Code == 100 }, "jackett error: Invalid API Key"},
		{"unauthorized status", 401, "nope",
			func(err error) bool { var e *HTTPError; return errors.As(err, &e) && e.StatusCode == 401 }, "jackett rejected the API key (HTTP 401)"},
		{"JSON error answer", 500, string(fixture(t, "error_unknown_indexer.json")),
			func(err error) bool { var e *HTTPError; return errors.As(err, &e) && e.StatusCode == 500 },
			"jackett answered HTTP 500: Unknown indexer: nosuchindexer"},
		{"server error", 502, "<html>bad gateway</html>",
			func(err error) bool { var e *HTTPError; return errors.As(err, &e) }, "unexpected response (HTTP 502)"},
		{"not XML", 200, "<html>login</html>",
			func(err error) bool { return err != nil }, "jackett indexer list is not Torznab XML"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer srv.Close()

			_, err := New(srv.URL, testKey).Indexers(context.Background())
			if err == nil || !tt.check(err) {
				t.Fatalf("err = %v (%T)", err, err)
			}
			if !strings.HasPrefix(err.Error(), tt.wantMsg) {
				t.Errorf("message = %q, want prefix %q", err.Error(), tt.wantMsg)
			}
		})
	}
}

func TestNew_InvalidBaseURL(t *testing.T) {
	for _, base := range []string{"", "jackett:9117", "ftp://jackett", "http://"} {
		_, err := New(base, testKey).Indexers(context.Background())
		if !errors.Is(err, ErrInvalidBaseURL) {
			t.Errorf("New(%q): err = %v, want ErrInvalidBaseURL", base, err)
		}
	}
}

func TestIndexers_BasePath(t *testing.T) {
	var gotPath string
	body := fixture(t, "indexers.xml")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	if _, err := New(srv.URL+"/jackett/", testKey).Indexers(context.Background()); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/jackett/api/v2.0/indexers/all/results/torznab/api" {
		t.Errorf("path = %q", gotPath)
	}
}
