package opensubtitles

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultBaseURL = "https://api.opensubtitles.com/api/v1"
	userAgent      = "momoshtrem v1.0"

	// HTTP timeouts
	defaultHTTPTimeout = 30 * time.Second

	// Token management
	tokenValidDuration   = 24 * time.Hour // Token validity period
	tokenRefreshDuration = 23 * time.Hour // Refresh before expiry
)

// Client is an OpenSubtitles API client
type Client struct {
	apiKey     string
	username   string
	password   string
	baseURL    string
	httpClient *http.Client

	// Token management
	mu       sync.RWMutex
	token    string
	tokenExp time.Time
}

var _ SubtitleFetcher = (*Client)(nil)

// Option configures a Client.
type Option func(*Client)

// WithBaseURL points the client at another API root (tests).
func WithBaseURL(u string) Option {
	return func(c *Client) { c.baseURL = strings.TrimSuffix(u, "/") }
}

// WithHTTPClient replaces the default HTTP client.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) { c.httpClient = hc }
}

// NewClient creates a new OpenSubtitles client
func NewClient(apiKey, username, password string, opts ...Option) *Client {
	c := &Client{
		apiKey:   apiKey,
		username: username,
		password: password,
		baseURL:  defaultBaseURL,
		httpClient: &http.Client{
			Timeout: defaultHTTPTimeout,
		},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// IsConfigured returns true if the client has an API key configured
func (c *Client) IsConfigured() bool {
	return c.apiKey != ""
}

// Search searches for subtitles matching the given parameters
func (c *Client) Search(ctx context.Context, params SearchParams) (*SearchResponse, error) {
	if !c.IsConfigured() {
		return nil, fmt.Errorf("OpenSubtitles API key not configured")
	}

	endpoint := fmt.Sprintf("%s/subtitles?%s", c.baseURL, searchQuery(params).Encode())

	var result SearchResponse
	if err := c.get(ctx, endpoint, &result); err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}

	return &result, nil
}

// searchQuery builds the /subtitles query. The API redirects requests whose
// parameters aren't sorted and lowercase; url.Values.Encode sorts the keys,
// and languages are lowercased and sorted here.
func searchQuery(params SearchParams) url.Values {
	query := url.Values{}

	if params.Type == "episode" {
		query.Set("type", "episode")
		if params.ParentTMDBID > 0 {
			query.Set("parent_tmdb_id", strconv.Itoa(params.ParentTMDBID))
		}
		// Season 0 holds specials, so it's sent whenever an episode is.
		if params.SeasonNumber > 0 || params.EpisodeNumber > 0 {
			query.Set("season_number", strconv.Itoa(params.SeasonNumber))
		}
		if params.EpisodeNumber > 0 {
			query.Set("episode_number", strconv.Itoa(params.EpisodeNumber))
		}
	} else {
		query.Set("type", "movie")
		if params.TMDBID > 0 {
			query.Set("tmdb_id", strconv.Itoa(params.TMDBID))
		}
	}

	if len(params.Languages) > 0 {
		langs := make([]string, 0, len(params.Languages))
		for _, l := range params.Languages {
			if l = strings.ToLower(strings.TrimSpace(l)); l != "" && !slices.Contains(langs, l) {
				langs = append(langs, l)
			}
		}
		sort.Strings(langs)
		query.Set("languages", strings.Join(langs, ","))
	}

	return query
}

// Download downloads a subtitle file by file ID
// Returns the subtitle content as bytes and the filename
func (c *Client) Download(ctx context.Context, fileID int) ([]byte, string, error) {
	if !c.IsConfigured() {
		return nil, "", fmt.Errorf("OpenSubtitles API key not configured")
	}

	// Ensure we have a valid token for downloads
	if err := c.ensureToken(ctx); err != nil {
		return nil, "", fmt.Errorf("failed to authenticate: %w", err)
	}

	// Request download link
	endpoint := fmt.Sprintf("%s/download", c.baseURL)
	reqBody := DownloadRequest{FileID: fileID}

	var downloadResp DownloadResponse
	if err := c.post(ctx, endpoint, reqBody, &downloadResp, true); err != nil {
		return nil, "", fmt.Errorf("download request failed: %w", err)
	}

	if downloadResp.Link == "" {
		return nil, "", fmt.Errorf("no download link in response")
	}

	// Download the actual subtitle file
	req, err := http.NewRequestWithContext(ctx, "GET", downloadResp.Link, nil)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create download request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("failed to download file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("download returned status %d", resp.StatusCode)
	}

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read subtitle content: %w", err)
	}

	return content, downloadResp.FileName, nil
}

// ensureToken ensures we have a valid authentication token
func (c *Client) ensureToken(ctx context.Context) error {
	c.mu.RLock()
	if c.token != "" && time.Now().Before(c.tokenExp) {
		c.mu.RUnlock()
		return nil
	}
	c.mu.RUnlock()

	// Need to login
	return c.login(ctx)
}

// login authenticates and obtains a token
func (c *Client) login(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Double-check after acquiring write lock
	if c.token != "" && time.Now().Before(c.tokenExp) {
		return nil
	}

	// If no credentials, use API key only (limited downloads)
	if c.username == "" || c.password == "" {
		// For anonymous use, we don't need a token
		// The API key alone allows limited downloads
		c.token = "anonymous"
		c.tokenExp = time.Now().Add(tokenValidDuration)
		return nil
	}

	endpoint := fmt.Sprintf("%s/login", c.baseURL)
	reqBody := LoginRequest{
		Username: c.username,
		Password: c.password,
	}

	var loginResp LoginResponse
	if err := c.post(ctx, endpoint, reqBody, &loginResp, false); err != nil {
		return fmt.Errorf("login failed: %w", err)
	}

	if loginResp.Token == "" {
		return fmt.Errorf("no token in login response")
	}

	c.token = loginResp.Token
	c.tokenExp = time.Now().Add(tokenRefreshDuration)

	return nil
}

// get performs a GET request
func (c *Client) get(ctx context.Context, endpoint string, result interface{}) error {
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	c.setHeaders(req, false)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	return c.handleResponse(resp, result)
}

// post performs a POST request
func (c *Client) post(ctx context.Context, endpoint string, body interface{}, result interface{}, useToken bool) error {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(jsonBody))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	c.setHeaders(req, useToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	return c.handleResponse(resp, result)
}

// setHeaders sets common headers
func (c *Client) setHeaders(req *http.Request, useToken bool) {
	req.Header.Set("Api-Key", c.apiKey)
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")

	if useToken {
		c.mu.RLock()
		token := c.token
		c.mu.RUnlock()

		if token != "" && token != "anonymous" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
	}
}

// handleResponse processes the HTTP response
func (c *Client) handleResponse(resp *http.Response, result interface{}) error {
	if resp.StatusCode == http.StatusUnauthorized {
		// Clear token to force re-login
		c.mu.Lock()
		c.token = ""
		c.tokenExp = time.Time{}
		c.mu.Unlock()
		return fmt.Errorf("unauthorized - invalid API key or token")
	}

	if resp.StatusCode == http.StatusTooManyRequests {
		return fmt.Errorf("rate limited - too many requests")
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	return nil
}
