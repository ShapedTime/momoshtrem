package tmdb

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// TMDB's lists, for recommendations' candidate pools and tools (R1, #153):
// discover, trending, upcoming, on the air, search and the genre lists.
// Every list item carries its media type ("movie" or "tv"), which TMDB
// itself sends only on trending.

// Media types.
const (
	MediaMovie = "movie"
	MediaTV    = "tv"
)

// List cache TTLs (Cached).
const (
	DiscoverTTL = 12 * time.Hour
	TrendingTTL = 6 * time.Hour
	UpcomingTTL = 12 * time.Hour
	OnTheAirTTL = 6 * time.Hour
	SearchTTL   = 12 * time.Hour
	GenresTTL   = 7 * 24 * time.Hour
)

// DiscoverQuery filters GET /discover/{movie|tv}. Zero fields don't filter.
type DiscoverQuery struct {
	Genres   []int // any of them
	Keywords []int // any of them
	// From and To bound the release date (movies) or the first air date
	// (shows), YYYY-MM-DD, both inclusive.
	From, To       string
	MinVoteAverage float64
	MinVoteCount   int
	Language       string // the original language, ISO 639-1
	SortBy         string // e.g. "vote_average.desc"; popularity.desc when empty
	Page           int    // 1 when 0
}

// params is the query as TMDB's parameters.
func (q DiscoverQuery) params(mediaType string) url.Values {
	p := url.Values{"include_adult": {"false"}, "sort_by": {"popularity.desc"}, "page": {"1"}}
	if len(q.Genres) > 0 {
		p.Set("with_genres", joinInts(q.Genres, "|"))
	}
	if len(q.Keywords) > 0 {
		p.Set("with_keywords", joinInts(q.Keywords, "|"))
	}
	date := "primary_release_date"
	if mediaType == MediaTV {
		date = "first_air_date"
		p.Set("include_null_first_air_dates", "false")
	} else {
		p.Set("include_video", "false")
	}
	if q.From != "" {
		p.Set(date+".gte", q.From)
	}
	if q.To != "" {
		p.Set(date+".lte", q.To)
	}
	if q.MinVoteAverage > 0 {
		p.Set("vote_average.gte", strconv.FormatFloat(q.MinVoteAverage, 'f', -1, 64))
	}
	if q.MinVoteCount > 0 {
		p.Set("vote_count.gte", strconv.Itoa(q.MinVoteCount))
	}
	if q.Language != "" {
		p.Set("with_original_language", q.Language)
	}
	if q.SortBy != "" {
		p.Set("sort_by", q.SortBy)
	}
	if q.Page > 0 {
		p.Set("page", strconv.Itoa(q.Page))
	}
	return p
}

// Discover lists movies or shows matching q.
func (c *Client) Discover(ctx context.Context, mediaType string, q DiscoverQuery) (*ResultsPage, error) {
	if err := checkMediaType(mediaType); err != nil {
		return nil, err
	}
	return c.list(ctx, "/discover/"+mediaType, q.params(mediaType), mediaType)
}

// Trending lists the movies or shows trending this "day" or "week".
func (c *Client) Trending(ctx context.Context, mediaType, window string) (*ResultsPage, error) {
	if err := checkMediaType(mediaType); err != nil {
		return nil, err
	}
	if window != "day" && window != "week" {
		return nil, fmt.Errorf("unknown trending window %q", window)
	}
	return c.list(ctx, "/trending/"+mediaType+"/"+window, nil, mediaType)
}

// UpcomingMovies lists movies about to be released (page 1 when 0).
func (c *Client) UpcomingMovies(ctx context.Context, page int) (*ResultsPage, error) {
	return c.list(ctx, "/movie/upcoming", pageParams(page), MediaMovie)
}

// OnTheAir lists shows with an episode airing in the next seven days
// (page 1 when 0).
func (c *Client) OnTheAir(ctx context.Context, page int) (*ResultsPage, error) {
	return c.list(ctx, "/tv/on_the_air", pageParams(page), MediaTV)
}

// Search finds movies or shows by name; year (0: any) narrows it to the
// release year or the first air year.
func (c *Client) Search(ctx context.Context, mediaType, query string, year int) (*ResultsPage, error) {
	if err := checkMediaType(mediaType); err != nil {
		return nil, err
	}
	params := url.Values{"query": {strings.TrimSpace(query)}, "include_adult": {"false"}, "page": {"1"}}
	if year > 0 {
		if mediaType == MediaTV {
			params.Set("first_air_date_year", strconv.Itoa(year))
		} else {
			params.Set("year", strconv.Itoa(year))
		}
	}
	return c.list(ctx, "/search/"+mediaType, params, mediaType)
}

// GenreList lists TMDB's movie or show genres, with their ids.
func (c *Client) GenreList(ctx context.Context, mediaType string) ([]Genre, error) {
	if err := checkMediaType(mediaType); err != nil {
		return nil, err
	}
	var resp struct {
		Genres []Genre `json:"genres"`
	}
	if err := c.get(ctx, "/genre/"+mediaType+"/list", nil, &resp, c.attempts); err != nil {
		return nil, err
	}
	return resp.Genres, nil
}

// list fetches one page and marks its items with mediaType.
func (c *Client) list(ctx context.Context, path string, params url.Values, mediaType string) (*ResultsPage, error) {
	page := &ResultsPage{}
	if err := c.get(ctx, path, params, page, c.attempts); err != nil {
		return nil, err
	}
	for i := range page.Results {
		page.Results[i].MediaType = mediaType
	}
	return page, nil
}

func checkMediaType(mediaType string) error {
	if mediaType != MediaMovie && mediaType != MediaTV {
		return fmt.Errorf("unknown media type %q", mediaType)
	}
	return nil
}

func pageParams(page int) url.Values {
	if page < 1 {
		page = 1
	}
	return url.Values{"page": {strconv.Itoa(page)}}
}

func joinInts(ids []int, sep string) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = strconv.Itoa(id)
	}
	return strings.Join(parts, sep)
}

// Discover returns a discover page, cached for DiscoverTTL.
func (c *Cached) Discover(ctx context.Context, mediaType string, q DiscoverQuery) (*ResultsPage, error) {
	if err := checkMediaType(mediaType); err != nil {
		return nil, err
	}
	return cachedGet(ctx, c, c.key("discover/"+mediaType+"?"+q.params(mediaType).Encode()), DiscoverTTL,
		func(ctx context.Context) (*ResultsPage, error) { return c.client.Discover(ctx, mediaType, q) })
}

// Trending returns the trending page, cached for TrendingTTL.
func (c *Cached) Trending(ctx context.Context, mediaType, window string) (*ResultsPage, error) {
	return cachedGet(ctx, c, c.key("trending/"+mediaType+"/"+window), TrendingTTL,
		func(ctx context.Context) (*ResultsPage, error) { return c.client.Trending(ctx, mediaType, window) })
}

// UpcomingMovies returns upcoming movies, cached for UpcomingTTL.
func (c *Cached) UpcomingMovies(ctx context.Context, page int) (*ResultsPage, error) {
	return cachedGet(ctx, c, c.key("movie/upcoming?"+pageParams(page).Encode()), UpcomingTTL,
		func(ctx context.Context) (*ResultsPage, error) { return c.client.UpcomingMovies(ctx, page) })
}

// OnTheAir returns shows on the air, cached for OnTheAirTTL.
func (c *Cached) OnTheAir(ctx context.Context, page int) (*ResultsPage, error) {
	return cachedGet(ctx, c, c.key("tv/on_the_air?"+pageParams(page).Encode()), OnTheAirTTL,
		func(ctx context.Context) (*ResultsPage, error) { return c.client.OnTheAir(ctx, page) })
}

// Search returns search results, cached for SearchTTL.
func (c *Cached) Search(ctx context.Context, mediaType, query string, year int) (*ResultsPage, error) {
	if err := checkMediaType(mediaType); err != nil {
		return nil, err
	}
	key := url.Values{"query": {strings.ToLower(strings.TrimSpace(query))}, "year": {strconv.Itoa(year)}}
	return cachedGet(ctx, c, c.key("search/"+mediaType+"?"+key.Encode()), SearchTTL,
		func(ctx context.Context) (*ResultsPage, error) { return c.client.Search(ctx, mediaType, query, year) })
}

// genreList wraps the genres so cachedGet can work with a pointer.
type genreList struct {
	Genres []Genre `json:"genres"`
}

// GenreList returns the genre list, cached for GenresTTL.
func (c *Cached) GenreList(ctx context.Context, mediaType string) ([]Genre, error) {
	list, err := cachedGet(ctx, c, c.key("genre/"+mediaType+"/list"), GenresTTL,
		func(ctx context.Context) (*genreList, error) {
			genres, err := c.client.GenreList(ctx, mediaType)
			if err != nil {
				return nil, err
			}
			return &genreList{Genres: genres}, nil
		})
	if err != nil {
		return nil, err
	}
	return list.Genres, nil
}
