package tmdb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const baseURL = "https://api.themoviedb.org/3"

// ErrNotFound means TMDB has no item with the requested ID.
var ErrNotFound = errors.New("not found")

// Client is a TMDB API client
type Client struct {
	apiKey     string
	httpClient *http.Client
}

// NewClient creates a new TMDB client
func NewClient(apiKey string) *Client {
	return &Client{
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Movie represents a movie from TMDB
type Movie struct {
	ID          int    `json:"id"`
	Title       string `json:"title"`
	ReleaseDate string `json:"release_date"`
	Overview    string `json:"overview"`
	PosterPath  string `json:"poster_path"`
}

// Year extracts the year from the release date
func (m *Movie) Year() int {
	if len(m.ReleaseDate) < 4 {
		return 0
	}
	var year int
	fmt.Sscanf(m.ReleaseDate[:4], "%d", &year)
	return year
}

// Show represents a TV show from TMDB
type Show struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	FirstAirDate string `json:"first_air_date"`
	Overview     string `json:"overview"`
	PosterPath   string `json:"poster_path"`
}

// Year extracts the year from the first air date
func (s *Show) Year() int {
	if len(s.FirstAirDate) < 4 {
		return 0
	}
	var year int
	fmt.Sscanf(s.FirstAirDate[:4], "%d", &year)
	return year
}

// Season represents a TV season from TMDB
type Season struct {
	ID           int       `json:"id"`
	SeasonNumber int       `json:"season_number"`
	Name         string    `json:"name"`
	Episodes     []Episode `json:"episodes"`
}

// Episode represents a TV episode from TMDB
type Episode struct {
	ID            int    `json:"id"`
	EpisodeNumber int    `json:"episode_number"`
	Name          string `json:"name"`
	Overview      string `json:"overview"`
	AirDate       string `json:"air_date"`
}

// ShowDetails represents detailed show info including seasons
type ShowDetails struct {
	Show
	Seasons []struct {
		ID           int `json:"id"`
		SeasonNumber int `json:"season_number"`
		EpisodeCount int `json:"episode_count"`
	} `json:"seasons"`
}

// GetMovie fetches movie details by TMDB ID
func (c *Client) GetMovie(id int) (*Movie, error) {
	endpoint := fmt.Sprintf("%s/movie/%d", baseURL, id)

	movie := &Movie{}
	if err := c.get(endpoint, movie); err != nil {
		return nil, err
	}

	return movie, nil
}

// GetShow fetches TV show details by TMDB ID
func (c *Client) GetShow(id int) (*Show, error) {
	endpoint := fmt.Sprintf("%s/tv/%d", baseURL, id)

	show := &Show{}
	if err := c.get(endpoint, show); err != nil {
		return nil, err
	}

	return show, nil
}

// GetShowDetails fetches detailed TV show info including seasons
func (c *Client) GetShowDetails(id int) (*ShowDetails, error) {
	endpoint := fmt.Sprintf("%s/tv/%d", baseURL, id)

	details := &ShowDetails{}
	if err := c.get(endpoint, details); err != nil {
		return nil, err
	}

	return details, nil
}

// GetSeason fetches season details including episodes
func (c *Client) GetSeason(showID int, seasonNumber int) (*Season, error) {
	endpoint := fmt.Sprintf("%s/tv/%d/season/%d", baseURL, showID, seasonNumber)

	season := &Season{}
	if err := c.get(endpoint, season); err != nil {
		return nil, err
	}

	return season, nil
}

// SearchMovies searches for movies by title
func (c *Client) SearchMovies(query string) ([]Movie, error) {
	endpoint := fmt.Sprintf("%s/search/movie?query=%s", baseURL, url.QueryEscape(query))

	var result struct {
		Results []Movie `json:"results"`
	}
	if err := c.get(endpoint, &result); err != nil {
		return nil, err
	}

	return result.Results, nil
}

// SearchShows searches for TV shows by title
func (c *Client) SearchShows(query string) ([]Show, error) {
	endpoint := fmt.Sprintf("%s/search/tv?query=%s", baseURL, url.QueryEscape(query))

	var result struct {
		Results []Show `json:"results"`
	}
	if err := c.get(endpoint, &result); err != nil {
		return nil, err
	}

	return result.Results, nil
}

// Ping checks that TMDB is reachable and accepts the API key.
func (c *Client) Ping(ctx context.Context) error {
	return c.getContext(ctx, baseURL+"/configuration", nil)
}

// get performs a GET request and decodes the response
func (c *Client) get(endpoint string, v interface{}) error {
	return c.getContext(context.Background(), endpoint, v)
}

// getContext performs a GET request and decodes the response into v.
// A nil v discards the body.
func (c *Client) getContext(ctx context.Context, endpoint string, v interface{}) error {
	// Add API key
	u, err := url.Parse(endpoint)
	if err != nil {
		return fmt.Errorf("invalid endpoint: %w", err)
	}

	q := u.Query()
	q.Set("api_key", c.apiKey)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return fmt.Errorf("failed to build request: %w", withoutURL(err))
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", withoutURL(err))
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return ErrNotFound
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("TMDB rejected the API key (HTTP 401)")
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	if v == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}

	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	return nil
}

// withoutURL strips the request URL from transport errors, because the URL
// carries the API key and these errors end up in logs and API responses.
func withoutURL(err error) error {
	var uerr *url.Error
	if errors.As(err, &uerr) {
		return uerr.Err
	}
	return err
}
