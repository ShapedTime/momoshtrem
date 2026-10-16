package tmdb

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

// listServer is a fake TMDB that answers every list path with two items
// and records the requests.
type listServer struct {
	*httptest.Server
	mu       sync.Mutex
	requests []*url.URL
}

func newListServer(t *testing.T) *listServer {
	t.Helper()
	ls := &listServer{}
	ls.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ls.mu.Lock()
		ls.requests = append(ls.requests, r.URL)
		ls.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if strings.HasPrefix(r.URL.Path, "/genre/") {
			_, _ = w.Write([]byte(`{"genres": [{"id": 18, "name": "Drama"}, {"id": 16, "name": "Animation"}]}`))
			return
		}
		if r.URL.Path == "/discover/tv" && r.URL.Query().Get("page") == "9" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"page": 1, "total_pages": 3, "total_results": 41, "results": [
			{"id": 95396, "name": "Severance", "original_language": "en", "origin_country": ["US"], "first_air_date": "2022-02-17",
			 "genre_ids": [18, 9648], "vote_average": 8.4, "vote_count": 2100, "popularity": 210.5, "poster_path": "/sev.jpg"},
			{"id": 693134, "title": "Dune: Part Two", "media_type": "movie", "original_language": "en", "release_date": "2024-02-27",
			 "genre_ids": [878, 12], "vote_average": 8.1, "vote_count": 6000}
		]}`))
	}))
	t.Cleanup(ls.Close)
	return ls
}

func (ls *listServer) last(t *testing.T) *url.URL {
	t.Helper()
	ls.mu.Lock()
	defer ls.mu.Unlock()
	if len(ls.requests) == 0 {
		t.Fatal("no request")
	}
	return ls.requests[len(ls.requests)-1]
}

func (ls *listServer) count() int {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	return len(ls.requests)
}

func TestClient_Lists(t *testing.T) {
	ls := newListServer(t)
	c := NewClient(testKey, WithBaseURL(ls.URL), WithLanguage("en-US"), WithRetries(1, 0))
	ctx := context.Background()

	tests := []struct {
		name      string
		call      func() (*ResultsPage, error)
		path      string
		params    map[string]string
		mediaType string
	}{
		{"discover movies", func() (*ResultsPage, error) {
			return c.Discover(ctx, MediaMovie, DiscoverQuery{Genres: []int{18, 9648}, Keywords: []int{210024}, From: "2023-01-01",
				To: "2026-10-16", MinVoteAverage: 7.5, MinVoteCount: 200, Language: "ja", SortBy: "vote_average.desc", Page: 2})
		}, "/discover/movie", map[string]string{
			"with_genres": "18|9648", "with_keywords": "210024", "primary_release_date.gte": "2023-01-01",
			"primary_release_date.lte": "2026-10-16", "vote_average.gte": "7.5", "vote_count.gte": "200",
			"with_original_language": "ja", "sort_by": "vote_average.desc", "page": "2", "include_adult": "false",
			"include_video": "false", "api_key": testKey, "language": "en-US",
		}, MediaMovie},
		{"discover shows, defaults", func() (*ResultsPage, error) {
			return c.Discover(ctx, MediaTV, DiscoverQuery{From: "2026-07-18"})
		}, "/discover/tv", map[string]string{
			"first_air_date.gte": "2026-07-18", "sort_by": "popularity.desc", "page": "1", "include_adult": "false",
			"include_null_first_air_dates": "false", "with_genres": "", "primary_release_date.gte": "",
		}, MediaTV},
		{"trending", func() (*ResultsPage, error) { return c.Trending(ctx, MediaTV, "week") }, "/trending/tv/week", nil, MediaTV},
		{"upcoming", func() (*ResultsPage, error) { return c.UpcomingMovies(ctx, 0) }, "/movie/upcoming", map[string]string{"page": "1"}, MediaMovie},
		{"on the air", func() (*ResultsPage, error) { return c.OnTheAir(ctx, 2) }, "/tv/on_the_air", map[string]string{"page": "2"}, MediaTV},
		{"search movies by year", func() (*ResultsPage, error) { return c.Search(ctx, MediaMovie, "dune part two", 2024) },
			"/search/movie", map[string]string{"query": "dune part two", "year": "2024", "include_adult": "false"}, MediaMovie},
		{"search shows by year", func() (*ResultsPage, error) { return c.Search(ctx, MediaTV, "severance", 2022) },
			"/search/tv", map[string]string{"query": "severance", "first_air_date_year": "2022", "year": ""}, MediaTV},
		{"search any year", func() (*ResultsPage, error) { return c.Search(ctx, MediaTV, "severance", 0) },
			"/search/tv", map[string]string{"first_air_date_year": ""}, MediaTV},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			page, err := tt.call()
			if err != nil {
				t.Fatal(err)
			}
			u := ls.last(t)
			if u.Path != tt.path {
				t.Errorf("path = %s, want %s", u.Path, tt.path)
			}
			for k, want := range tt.params {
				if got := u.Query().Get(k); got != want {
					t.Errorf("%s = %q, want %q", k, got, want)
				}
			}
			if page.TotalResults != 41 || len(page.Results) != 2 {
				t.Fatalf("page = %+v", page)
			}
			// Every item carries the list's media type, whatever TMDB said
			for _, item := range page.Results {
				if item.MediaType != tt.mediaType {
					t.Errorf("item %d media type = %q, want %q", item.ID, item.MediaType, tt.mediaType)
				}
			}
			sev := page.Results[0]
			if sev.DisplayTitle() != "Severance" || sev.Year() != 2022 || len(sev.OriginCountry) != 1 || sev.GenreIDs[1] != 9648 {
				t.Errorf("first item = %+v", sev)
			}
		})
	}

	genres, err := c.GenreList(ctx, MediaTV)
	if err != nil || len(genres) != 2 || genres[0].Name != "Drama" || ls.last(t).Path != "/genre/tv/list" {
		t.Errorf("genres = %+v, %v", genres, err)
	}

	before := ls.count()
	for _, bad := range []func() error{
		func() error { _, err := c.Discover(ctx, "show", DiscoverQuery{}); return err },
		func() error { _, err := c.Trending(ctx, MediaMovie, "month"); return err },
		func() error { _, err := c.Search(ctx, "person", "x", 0); return err },
		func() error { _, err := c.GenreList(ctx, ""); return err },
	} {
		if bad() == nil {
			t.Error("a bad argument was accepted")
		}
	}
	if ls.count() != before {
		t.Error("a bad argument reached TMDB")
	}
	if _, err := c.Discover(ctx, MediaTV, DiscoverQuery{Page: 9}); err != ErrNotFound {
		t.Errorf("404 = %v, want ErrNotFound", err)
	}
}

func TestCached_Lists(t *testing.T) {
	ls := newListServer(t)
	client := NewClient(testKey, WithBaseURL(ls.URL), WithLanguage("en-US"), WithRetries(1, 0))
	cache := newMemCache()
	c := NewCached(client, cache)
	now := time.Date(2026, 10, 16, 3, 0, 0, 0, time.UTC)
	c.now = func() time.Time { return now }
	ctx := context.Background()

	calls := []struct {
		name string
		call func() error
		key  string
		ttl  time.Duration
	}{
		{"discover", func() error {
			_, err := c.Discover(ctx, MediaMovie, DiscoverQuery{Keywords: []int{9715, 4344}, From: "2025-01-01"})
			return err
		}, "tmdb:v1:en-US:discover/movie?include_adult=false&include_video=false&page=1&primary_release_date.gte=2025-01-01&sort_by=popularity.desc&with_keywords=9715%7C4344", DiscoverTTL},
		{"trending", func() error { _, err := c.Trending(ctx, MediaMovie, "week"); return err }, "tmdb:v1:en-US:trending/movie/week", TrendingTTL},
		{"upcoming", func() error { _, err := c.UpcomingMovies(ctx, 1); return err }, "tmdb:v1:en-US:movie/upcoming?page=1", UpcomingTTL},
		{"on the air", func() error { _, err := c.OnTheAir(ctx, 1); return err }, "tmdb:v1:en-US:tv/on_the_air?page=1", OnTheAirTTL},
		{"search", func() error { _, err := c.Search(ctx, MediaTV, " Severance ", 2022); return err }, "tmdb:v1:en-US:search/tv?query=severance&year=2022", SearchTTL},
		{"genres", func() error { _, err := c.GenreList(ctx, MediaMovie); return err }, "tmdb:v1:en-US:genre/movie/list", GenresTTL},
	}
	for _, tt := range calls {
		t.Run(tt.name, func(t *testing.T) {
			before := ls.count()
			if err := tt.call(); err != nil {
				t.Fatal(err)
			}
			if err := tt.call(); err != nil {
				t.Fatal(err)
			}
			if got := ls.count() - before; got != 1 {
				t.Errorf("requests = %d, want 1 (the second call is a cache hit)", got)
			}
			entry, ok := cache.entry(tt.key)
			if !ok {
				t.Fatalf("no cache entry %s", tt.key)
			}
			if want := now.Add(tt.ttl); !entry.expiresAt.Equal(want) {
				t.Errorf("expires %v, want %v", entry.expiresAt, want)
			}
		})
	}

	// A cached page keeps its media types
	page, err := c.Trending(ctx, MediaMovie, "week")
	if err != nil || page.Results[0].MediaType != MediaMovie {
		t.Errorf("cached trending = %+v, %v", page, err)
	}
	// Another search is another entry; the same one differently cased isn't
	before := ls.count()
	_, _ = c.Search(ctx, MediaTV, "SEVERANCE", 2022)
	_, _ = c.Search(ctx, MediaTV, "severance", 0)
	if got := ls.count() - before; got != 1 {
		t.Errorf("requests = %d, want 1", got)
	}
}

func TestShowDetails_EpisodeRuntime(t *testing.T) {
	tests := []struct {
		name string
		show ShowDetails
		want int
	}{
		{"episode_run_time", ShowDetails{EpisodeRunTime: []int{0, 52}, LastEpisodeToAir: &EpisodeRef{Runtime: 60}}, 52},
		{"the last episode's when TMDB's list is empty", ShowDetails{LastEpisodeToAir: &EpisodeRef{Runtime: 58}}, 58},
		{"unknown", ShowDetails{}, 0},
	}
	for _, tt := range tests {
		if got := tt.show.EpisodeRuntime(); got != tt.want {
			t.Errorf("%s: %d, want %d", tt.name, got, tt.want)
		}
	}
}
