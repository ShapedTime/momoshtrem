package tmdb

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const testKey = "tmdb-secret-key"

// fixtureServer serves testdata/<name>.json for the paths in routes and
// records every request.
type fixtureServer struct {
	*httptest.Server
	mu       sync.Mutex
	requests []*http.Request
}

func newFixtureServer(t *testing.T, routes map[string]string) *fixtureServer {
	t.Helper()
	fs := &fixtureServer{}
	fs.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fs.mu.Lock()
		fs.requests = append(fs.requests, r)
		fs.mu.Unlock()

		name, ok := routes[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		body, err := os.ReadFile(filepath.Join("testdata", name+".json"))
		if err != nil {
			t.Errorf("read fixture %s: %v", name, err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(fs.Close)
	return fs
}

func (fs *fixtureServer) lastRequest(t *testing.T) *http.Request {
	t.Helper()
	fs.mu.Lock()
	defer fs.mu.Unlock()
	if len(fs.requests) == 0 {
		t.Fatal("no request reached the server")
	}
	return fs.requests[len(fs.requests)-1]
}

// statusServer answers each request with the next status in statuses (the
// last one repeats) and counts requests.
func statusServer(t *testing.T, statuses []int, headers map[string]string) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := int(calls.Add(1)) - 1
		if n >= len(statuses) {
			n = len(statuses) - 1
		}
		for k, v := range headers {
			w.Header().Set(k, v)
		}
		w.WriteHeader(statuses[n])
		_, _ = w.Write([]byte(`{"id": 1, "images": {}}`))
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

// recordSleeps replaces the client's sleep with one that records the delays
// and returns at once.
func recordSleeps(c *Client) *[]time.Duration {
	var slept []time.Duration
	c.sleep = func(_ context.Context, d time.Duration) error {
		slept = append(slept, d)
		return nil
	}
	return &slept
}

func TestClient_Status(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		wantErr error // nil means success
	}{
		{"ok", http.StatusOK, nil},
		{"unauthorized", http.StatusUnauthorized, ErrUnauthorized},
		{"not found", http.StatusNotFound, ErrNotFound},
		{"server error", http.StatusInternalServerError, &StatusError{StatusCode: 500}},
		{"teapot", http.StatusTeapot, &StatusError{StatusCode: 418}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotKey string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotKey = r.URL.Query().Get("api_key")
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(`{"images":{}}`))
			}))
			defer srv.Close()

			c := NewClient(testKey, WithBaseURL(srv.URL), WithRetries(1, 0))
			err := c.Ping(context.Background())

			if gotKey != testKey {
				t.Errorf("api_key = %q, want %q", gotKey, testKey)
			}
			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("want success, got %v", err)
				}
				return
			}
			var wantStatus *StatusError
			if errors.As(tt.wantErr, &wantStatus) {
				var got *StatusError
				if !errors.As(err, &got) || got.StatusCode != wantStatus.StatusCode {
					t.Fatalf("error = %v, want status %d", err, wantStatus.StatusCode)
				}
				return
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestClient_UnauthorizedMessage(t *testing.T) {
	srv, _ := statusServer(t, []int{http.StatusUnauthorized}, nil)
	err := NewClient(testKey, WithBaseURL(srv.URL)).Ping(context.Background())
	if err == nil || !strings.Contains(err.Error(), "rejected the API key") {
		t.Fatalf("error = %v, want it to mention the rejected key", err)
	}
}

func TestClient_TransportErrorHidesKey(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close() // connection refused from here on

	err := NewClient(testKey, WithBaseURL(url)).Ping(context.Background())
	if err == nil {
		t.Fatal("want an error from a closed server")
	}
	if strings.Contains(err.Error(), testKey) {
		t.Errorf("error leaks the API key: %v", err)
	}
}

func TestClient_NoAPIKeyMakesNoRequest(t *testing.T) {
	srv, calls := statusServer(t, []int{http.StatusOK}, nil)
	_, err := NewClient("", WithBaseURL(srv.URL)).MovieDetails(context.Background(), 1)
	if !errors.Is(err, ErrNoAPIKey) {
		t.Fatalf("error = %v, want ErrNoAPIKey", err)
	}
	if calls.Load() != 0 {
		t.Errorf("requests = %d, want 0", calls.Load())
	}
}

func TestClient_Retries(t *testing.T) {
	tests := []struct {
		name       string
		statuses   []int
		retryAfter string // Retry-After header; "" sends none
		wantCalls  int32
		wantErr    bool
		wantSleeps int
		firstSleep time.Duration // 0: any backoff delay
	}{
		{"5xx then success", []int{502, 503, 200}, "", 3, false, 2, 0},
		{"gives up after three attempts", []int{500}, "", 3, true, 2, 0},
		{"401 is not retried", []int{401, 200}, "", 1, true, 0, 0},
		{"404 is not retried", []int{404, 200}, "", 1, true, 0, 0},
		{"400 is not retried", []int{400, 200}, "", 1, true, 0, 0},
		{"429 honours Retry-After seconds", []int{429, 200}, "7", 2, false, 1, 7 * time.Second},
		{"429 without Retry-After backs off", []int{429, 200}, "", 2, false, 1, 0},
		{"429 asking too long is not retried", []int{429, 200}, "3600", 1, true, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			headers := map[string]string{}
			if tt.retryAfter != "" {
				headers["Retry-After"] = tt.retryAfter
			}
			srv, calls := statusServer(t, tt.statuses, headers)
			c := NewClient(testKey, WithBaseURL(srv.URL), WithRetries(3, 100*time.Millisecond))
			slept := recordSleeps(c)

			_, err := c.Season(context.Background(), 1, 1)

			if got := calls.Load(); got != tt.wantCalls {
				t.Errorf("requests = %d, want %d", got, tt.wantCalls)
			}
			if (err != nil) != tt.wantErr {
				t.Errorf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if len(*slept) != tt.wantSleeps {
				t.Fatalf("sleeps = %v, want %d of them", *slept, tt.wantSleeps)
			}
			for i, d := range *slept {
				if d <= 0 {
					t.Errorf("sleep %d = %v, want > 0", i, d)
				}
			}
			if tt.firstSleep != 0 && (*slept)[0] != tt.firstSleep {
				t.Errorf("first sleep = %v, want %v", (*slept)[0], tt.firstSleep)
			}
		})
	}
}

func TestClient_RetriesTimeouts(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			time.Sleep(300 * time.Millisecond) // longer than the client timeout
		}
		_, _ = w.Write([]byte(`{"id": 1}`))
	}))
	defer srv.Close()

	c := NewClient(testKey, WithBaseURL(srv.URL), WithHTTPClient(&http.Client{Timeout: 50 * time.Millisecond}))
	recordSleeps(c)

	season, err := c.Season(context.Background(), 1, 1)
	if err != nil {
		t.Fatalf("Season: %v", err)
	}
	if season.ID != 1 || calls.Load() != 2 {
		t.Errorf("season %d after %d requests, want 1 after 2", season.ID, calls.Load())
	}
}

func TestClient_CancelledContextIsNotRetried(t *testing.T) {
	srv, calls := statusServer(t, []int{503}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	c := NewClient(testKey, WithBaseURL(srv.URL))
	c.sleep = func(context.Context, time.Duration) error {
		cancel()
		return context.Canceled
	}

	if _, err := c.Season(ctx, 1, 1); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if calls.Load() != 1 {
		t.Errorf("requests = %d, want 1", calls.Load())
	}
}

func TestParseRetryAfter(t *testing.T) {
	now := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		value string
		want  time.Duration
	}{
		{"", 0},
		{"5", 5 * time.Second},
		{" 2 ", 2 * time.Second},
		{"-3", 0},
		{"soon", 0},
		{now.Add(10 * time.Second).Format(http.TimeFormat), 10 * time.Second},
		{now.Add(-time.Minute).Format(http.TimeFormat), 0},
	}
	for _, tt := range tests {
		if got := parseRetryAfter(tt.value, now); got != tt.want {
			t.Errorf("parseRetryAfter(%q) = %v, want %v", tt.value, got, tt.want)
		}
	}
}

func TestBackoffDelay(t *testing.T) {
	base := 100 * time.Millisecond
	for attempt := 1; attempt <= 4; attempt++ {
		full := base << (attempt - 1)
		for i := 0; i < 50; i++ {
			d := backoffDelay(base, attempt)
			if d < full/2 || d > full {
				t.Fatalf("attempt %d: delay %v outside [%v, %v]", attempt, d, full/2, full)
			}
		}
	}
	if d := backoffDelay(0, 1); d != 0 {
		t.Errorf("zero base: delay %v, want 0", d)
	}
}

func TestClient_MovieDetails(t *testing.T) {
	srv := newFixtureServer(t, map[string]string{"/movie/157336": "movie_157336"})
	c := NewClient(testKey, WithBaseURL(srv.URL), WithLanguage("en-US"))

	m, err := c.MovieDetails(context.Background(), 157336)
	if err != nil {
		t.Fatalf("MovieDetails: %v", err)
	}

	q := srv.lastRequest(t).URL.Query()
	queryChecks := map[string]string{
		"append_to_response":     "external_ids,keywords,credits,videos,recommendations,similar",
		"language":               "en-US",
		"include_video_language": "en,null",
		"api_key":                testKey,
	}
	for k, want := range queryChecks {
		if got := q.Get(k); got != want {
			t.Errorf("query %s = %q, want %q", k, got, want)
		}
	}

	checks := []struct {
		name      string
		got, want any
	}{
		{"title", m.Title, "Interstellar"},
		{"year", m.Year(), 2014},
		{"runtime", m.Runtime, 169},
		{"imdb", m.ExternalIDs.IMDbID, "tt0816692"},
		{"genres", len(m.Genres), 3},
		{"keywords (movie shape)", len(m.Keywords.Keywords), 2},
		{"directors", strings.Join(m.Credits.Directors(), ","), "Christopher Nolan"},
		{"crew trimmed to directing and writing", len(m.Credits.Crew), 3},
		{"trailer prefers official trailer", m.Videos.TrailerKey(), "zSWdZVtXT7E"},
		{"recommendations trimmed", len(m.Recommendations.Results), maxListItem},
		{"recommendation title", m.Recommendations.Results[0].DisplayTitle(), "Inception"},
		{"similar", m.Similar.Results[0].Year(), 2015},
	}
	for _, c := range checks {
		if !reflect.DeepEqual(c.got, c.want) {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}
}

func TestClient_ShowDetails(t *testing.T) {
	srv := newFixtureServer(t, map[string]string{"/tv/209867": "tv_209867"})
	c := NewClient(testKey, WithBaseURL(srv.URL))

	s, err := c.ShowDetails(context.Background(), 209867)
	if err != nil {
		t.Fatalf("ShowDetails: %v", err)
	}

	q := srv.lastRequest(t).URL.Query()
	if got := q.Get("append_to_response"); got != "external_ids,keywords,credits,aggregate_credits,videos,recommendations,similar" {
		t.Errorf("append_to_response = %q", got)
	}
	if q.Has("language") || q.Has("include_video_language") {
		t.Errorf("no language configured, but the query has %v", q)
	}

	checks := []struct {
		name      string
		got, want any
	}{
		{"name", s.Name, "Frieren: Beyond Journey's End"},
		{"year", s.Year(), 2023},
		{"original language", s.OriginalLanguage, "ja"},
		{"origin country", s.OriginCountry, []string{"JP"}},
		{"status", s.Status, "Returning Series"},
		{"tvdb", s.ExternalIDs.TVDBID, 424536},
		{"imdb", s.ExternalIDs.IMDbID, "tt22248376"},
		{"keywords (TV shape)", s.Keywords.Keywords[0].ID, 210024},
		{"networks", s.Networks[0].Name, "Nippon TV"},
		{"episode run time", s.EpisodeRunTime, []int{24}},
		{"season numbers include specials", s.SeasonNumbers(), []int{0, 1}},
		{"season name", s.Seasons[0].Name, "Specials"},
		{"aggregate cast", s.AggregateCredits.Cast[1].Character(), "Fern (voice)"},
		{"aggregate crew trimmed", len(s.AggregateCredits.Crew), 1},
		{"next episode", s.NextEpisodeToAir == nil, true},
		{"last episode type", s.LastEpisodeToAir.EpisodeType, "finale"},
		{"trailer", s.Videos.TrailerKey(), "qgQ7YVO2ZeY"},
	}
	for _, c := range checks {
		if !reflect.DeepEqual(c.got, c.want) {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}
}

func TestClient_SeasonZero(t *testing.T) {
	srv := newFixtureServer(t, map[string]string{"/tv/209867/season/0": "tv_209867_season_0"})
	c := NewClient(testKey, WithBaseURL(srv.URL))

	season, err := c.Season(context.Background(), 209867, 0)
	if err != nil {
		t.Fatalf("Season: %v", err)
	}
	if season.SeasonNumber != 0 || len(season.Episodes) != 2 {
		t.Fatalf("season %d with %d episodes, want season 0 with 2", season.SeasonNumber, len(season.Episodes))
	}
	ep := season.Episodes[0]
	if ep.ID != 4700001 || ep.Runtime != 3 || ep.StillPath != "/sp1.jpg" || ep.EpisodeType != "standard" || ep.AirDate != "2023-10-06" {
		t.Errorf("episode = %+v", ep)
	}
	if second := season.Episodes[1]; second.AirDate != "" || second.Runtime != 0 {
		t.Errorf("null air date and runtime should decode as zero values, got %+v", second)
	}
}

func TestClient_EpisodeGroups(t *testing.T) {
	srv := newFixtureServer(t, map[string]string{
		"/tv/37854/episode_groups":                   "tv_37854_episode_groups",
		"/tv/episode_group/5b1f6ff4925141523f00d2bb": "episode_group_5b1f6ff4925141523f00d2bb",
	})
	c := NewClient(testKey, WithBaseURL(srv.URL))

	groups, err := c.EpisodeGroups(context.Background(), 37854)
	if err != nil {
		t.Fatalf("EpisodeGroups: %v", err)
	}
	if len(groups) != 3 || groups[0].Type != GroupAbsolute || groups[0].EpisodeCount != 1100 {
		t.Fatalf("groups = %+v", groups)
	}

	group, err := c.EpisodeGroup(context.Background(), "5b1f6ff4925141523f00d2bb")
	if err != nil {
		t.Fatalf("EpisodeGroup: %v", err)
	}
	if group.Type != GroupStoryArc || len(group.Groups) != 2 || group.Groups[0].Episodes[1].EpisodeNumber != 4 {
		t.Errorf("group = %+v", group)
	}

	if _, err := c.EpisodeGroup(context.Background(), "missing"); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing group: error = %v, want ErrNotFound", err)
	}
}

func TestClient_ShowAlternativeTitles(t *testing.T) {
	srv := newFixtureServer(t, map[string]string{
		"/tv/209867/alternative_titles": "tv_209867_alternative_titles",
	})
	c := NewClient(testKey, WithBaseURL(srv.URL))

	titles, err := c.ShowAlternativeTitles(context.Background(), 209867)
	if err != nil {
		t.Fatalf("ShowAlternativeTitles: %v", err)
	}
	want := AlternativeTitle{Country: "JP", Title: "Sousou no Frieren", Type: "Romaji"}
	if len(titles) != 3 || titles[0] != want {
		t.Errorf("titles = %+v", titles)
	}
	if _, err := c.ShowAlternativeTitles(context.Background(), 1); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown show: error = %v, want ErrNotFound", err)
	}
}
