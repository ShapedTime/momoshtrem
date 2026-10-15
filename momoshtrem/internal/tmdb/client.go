package tmdb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.org/x/time/rate"
)

// DefaultBaseURL is TMDB's v3 API.
const DefaultBaseURL = "https://api.themoviedb.org/3"

const (
	defaultTimeout       = 10 * time.Second
	defaultAttempts      = 3
	defaultBackoff       = 500 * time.Millisecond
	defaultMaxRetryAfter = 30 * time.Second

	// Lists TMDB returns in full are trimmed to keep cached payloads small
	// (a long anime's aggregate credits run to thousands of entries).
	maxCredits  = 40
	maxListItem = 20
)

var (
	// ErrNotFound means TMDB has no item with the requested ID.
	ErrNotFound = errors.New("not found")
	// ErrUnauthorized means TMDB rejected the API key.
	ErrUnauthorized = errors.New("TMDB rejected the API key (HTTP 401)")
	// ErrNoAPIKey is returned without a request when no key is configured.
	ErrNoAPIKey = errors.New("TMDB API key not configured")
)

// StatusError is an HTTP status TMDB answered with that has no sentinel.
type StatusError struct {
	StatusCode int
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("unexpected status: %d", e.StatusCode)
}

// Client is a TMDB API client. It retries transient failures, honours
// Retry-After on 429 and can rate-limit itself. Every error is free of the
// request URL, which carries the API key.
type Client struct {
	apiKey        string
	baseURL       string
	httpClient    *http.Client
	language      string
	limiter       *rate.Limiter // nil: unlimited
	attempts      int
	backoff       time.Duration
	maxRetryAfter time.Duration

	// sleep waits between attempts; tests replace it.
	sleep func(ctx context.Context, d time.Duration) error
}

// Option configures a Client.
type Option func(*Client)

// WithBaseURL points the client at another server (tests use httptest).
func WithBaseURL(u string) Option {
	return func(c *Client) { c.baseURL = strings.TrimRight(u, "/") }
}

// WithHTTPClient replaces the default HTTP client (10 s timeout).
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) { c.httpClient = h }
}

// WithLanguage sets the language parameter sent with every request, e.g.
// "en-US". Empty sends none (TMDB's default, en-US).
func WithLanguage(lang string) Option {
	return func(c *Client) { c.language = lang }
}

// WithRateLimit caps requests per second. rps <= 0 disables the limit.
func WithRateLimit(rps float64) Option {
	return func(c *Client) {
		if rps <= 0 {
			c.limiter = nil
			return
		}
		burst := int(rps)
		if burst < 1 {
			burst = 1
		}
		c.limiter = rate.NewLimiter(rate.Limit(rps), burst)
	}
}

// WithRetries sets how many attempts a request gets (1 means no retry) and
// the base delay of the exponential backoff between them.
func WithRetries(attempts int, backoff time.Duration) Option {
	return func(c *Client) {
		if attempts < 1 {
			attempts = 1
		}
		c.attempts = attempts
		c.backoff = backoff
	}
}

// NewClient creates a TMDB client. An empty apiKey makes every call return
// ErrNoAPIKey.
func NewClient(apiKey string, opts ...Option) *Client {
	c := &Client{
		apiKey:        apiKey,
		baseURL:       DefaultBaseURL,
		httpClient:    &http.Client{Timeout: defaultTimeout},
		attempts:      defaultAttempts,
		backoff:       defaultBackoff,
		maxRetryAfter: defaultMaxRetryAfter,
		sleep:         sleepContext,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Language returns the language sent with requests ("" for TMDB's default).
func (c *Client) Language() string { return c.language }

// MovieDetails fetches a movie with external IDs, keywords, credits, videos,
// recommendations and similar titles in one request.
func (c *Client) MovieDetails(ctx context.Context, id int) (*MovieDetails, error) {
	params := url.Values{"append_to_response": {"external_ids,keywords,credits,videos,recommendations,similar"}}
	c.addVideoLanguage(params)

	m := &MovieDetails{}
	if err := c.get(ctx, fmt.Sprintf("/movie/%d", id), params, m, c.attempts); err != nil {
		return nil, err
	}
	m.Credits = trimCredits(m.Credits)
	m.Recommendations.Results = trimList(m.Recommendations.Results)
	m.Similar.Results = trimList(m.Similar.Results)
	return m, nil
}

// ShowDetails fetches a show with external IDs, keywords, credits, aggregate
// credits, videos, recommendations and similar titles in one request.
func (c *Client) ShowDetails(ctx context.Context, id int) (*ShowDetails, error) {
	params := url.Values{"append_to_response": {"external_ids,keywords,credits,aggregate_credits,videos,recommendations,similar"}}
	c.addVideoLanguage(params)

	s := &ShowDetails{}
	if err := c.get(ctx, fmt.Sprintf("/tv/%d", id), params, s, c.attempts); err != nil {
		return nil, err
	}
	s.Credits = trimCredits(s.Credits)
	s.AggregateCredits = trimAggregateCredits(s.AggregateCredits)
	s.Recommendations.Results = trimList(s.Recommendations.Results)
	s.Similar.Results = trimList(s.Similar.Results)
	return s, nil
}

// Season fetches one season with its episodes. Season 0 holds specials.
func (c *Client) Season(ctx context.Context, showID, seasonNumber int) (*Season, error) {
	s := &Season{}
	if err := c.get(ctx, fmt.Sprintf("/tv/%d/season/%d", showID, seasonNumber), nil, s, c.attempts); err != nil {
		return nil, err
	}
	return s, nil
}

// EpisodeGroups lists a show's alternative episode orderings.
func (c *Client) EpisodeGroups(ctx context.Context, showID int) ([]EpisodeGroupSummary, error) {
	var resp struct {
		Results []EpisodeGroupSummary `json:"results"`
	}
	if err := c.get(ctx, fmt.Sprintf("/tv/%d/episode_groups", showID), nil, &resp, c.attempts); err != nil {
		return nil, err
	}
	return resp.Results, nil
}

// ShowAlternativeTitles lists a show's names in other countries and
// scripts. The response doesn't depend on the client's language.
func (c *Client) ShowAlternativeTitles(ctx context.Context, showID int) ([]AlternativeTitle, error) {
	var resp struct {
		Results []AlternativeTitle `json:"results"`
	}
	if err := c.get(ctx, fmt.Sprintf("/tv/%d/alternative_titles", showID), nil, &resp, c.attempts); err != nil {
		return nil, err
	}
	return resp.Results, nil
}

// EpisodeGroup fetches one episode group with its episodes.
func (c *Client) EpisodeGroup(ctx context.Context, id string) (*EpisodeGroup, error) {
	if id == "" {
		return nil, ErrNotFound
	}
	g := &EpisodeGroup{}
	if err := c.get(ctx, "/tv/episode_group/"+url.PathEscape(id), nil, g, c.attempts); err != nil {
		return nil, err
	}
	return g, nil
}

// Ping checks that TMDB is reachable and accepts the API key. It doesn't
// retry: /api/status wants a quick answer.
func (c *Client) Ping(ctx context.Context) error {
	return c.get(ctx, "/configuration", nil, nil, 1)
}

// addVideoLanguage asks for videos in the client's language and those with
// no language, instead of the language alone (anime trailers are often
// untagged).
func (c *Client) addVideoLanguage(params url.Values) {
	if c.language == "" {
		return
	}
	lang, _, _ := strings.Cut(c.language, "-")
	params.Set("include_video_language", lang+",null")
}

// get requests path and decodes the JSON body into v (nil discards it),
// making up to attempts tries.
func (c *Client) get(ctx context.Context, path string, params url.Values, v any, attempts int) error {
	if c.apiKey == "" {
		return ErrNoAPIKey
	}

	q := url.Values{}
	for k, vs := range params {
		q[k] = vs
	}
	q.Set("api_key", c.apiKey)
	if c.language != "" {
		q.Set("language", c.language)
	}
	endpoint := c.baseURL + path + "?" + q.Encode()

	for attempt := 1; ; attempt++ {
		if c.limiter != nil {
			if err := c.limiter.Wait(ctx); err != nil {
				return fmt.Errorf("rate limiter: %w", err)
			}
		}

		retry, wait, err := c.do(ctx, endpoint, v)
		if err == nil {
			return nil
		}
		if !retry || attempt >= attempts || ctx.Err() != nil {
			return err
		}
		if wait <= 0 {
			wait = backoffDelay(c.backoff, attempt)
		}
		if err := c.sleep(ctx, wait); err != nil {
			return err
		}
	}
}

// do makes one request. retry reports whether the failure is transient; wait
// is how long the server asked us to wait (0 when it didn't say).
func (c *Client) do(ctx context.Context, endpoint string, v any) (retry bool, wait time.Duration, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return false, 0, fmt.Errorf("failed to build request: %w", withoutURL(err))
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		err = withoutURL(err)
		return isTimeout(err) && ctx.Err() == nil, 0, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusOK:
		if v == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			return false, 0, nil
		}
		if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
			err = withoutURL(err)
			return isTimeout(err) && ctx.Err() == nil, 0, fmt.Errorf("failed to decode response: %w", err)
		}
		return false, 0, nil
	case resp.StatusCode == http.StatusNotFound:
		return false, 0, ErrNotFound
	case resp.StatusCode == http.StatusUnauthorized:
		return false, 0, ErrUnauthorized
	case resp.StatusCode == http.StatusTooManyRequests:
		wait := parseRetryAfter(resp.Header.Get("Retry-After"), time.Now())
		if wait > c.maxRetryAfter {
			return false, 0, &StatusError{StatusCode: resp.StatusCode}
		}
		return true, wait, &StatusError{StatusCode: resp.StatusCode}
	case resp.StatusCode >= 500:
		return true, 0, &StatusError{StatusCode: resp.StatusCode}
	default:
		return false, 0, &StatusError{StatusCode: resp.StatusCode}
	}
}

// backoffDelay is base·2^(attempt-1), jittered to between half and all of it
// so parallel retries spread out.
func backoffDelay(base time.Duration, attempt int) time.Duration {
	d := base << (attempt - 1)
	if d <= 0 {
		return 0
	}
	half := d / 2
	return half + rand.N(half+1)
}

// parseRetryAfter reads a Retry-After header: delay seconds or an HTTP date.
// It returns 0 when the header is absent or unreadable.
func parseRetryAfter(value string, now time.Time) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if secs, err := strconv.Atoi(value); err == nil {
		if secs < 0 {
			return 0
		}
		return time.Duration(secs) * time.Second
	}
	if at, err := http.ParseTime(value); err == nil {
		if d := at.Sub(now); d > 0 {
			return d
		}
	}
	return 0
}

// isTimeout reports whether err is a timeout of this request (as opposed to
// the caller cancelling it).
func isTimeout(err error) bool {
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	return errors.Is(err, context.DeadlineExceeded)
}

func sleepContext(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
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

// trimCredits keeps the first cast members and the directors and writers.
func trimCredits(c Credits) Credits {
	if len(c.Cast) > maxCredits {
		c.Cast = c.Cast[:maxCredits]
	}
	crew := c.Crew[:0:0]
	for _, m := range c.Crew {
		if (m.Department == "Directing" || m.Department == "Writing") && len(crew) < maxCredits {
			crew = append(crew, m)
		}
	}
	c.Crew = crew
	return c
}

// trimAggregateCredits keeps the first cast members and the directors and
// writers.
func trimAggregateCredits(c AggregateCredits) AggregateCredits {
	if len(c.Cast) > maxCredits {
		c.Cast = c.Cast[:maxCredits]
	}
	crew := c.Crew[:0:0]
	for _, m := range c.Crew {
		if (m.Department == "Directing" || m.Department == "Writing") && len(crew) < maxCredits {
			crew = append(crew, m)
		}
	}
	c.Crew = crew
	return c
}

func trimList(items []ListItem) []ListItem {
	if len(items) > maxListItem {
		return items[:maxListItem]
	}
	return items
}
