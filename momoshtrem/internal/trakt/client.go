// Package trakt is the read-only Trakt integration (PLAN §6 "Trakt (B5)"):
// the API client, and the connection (Auth) with its device-code connect,
// encrypted rotating tokens, reauth and disconnect. Nothing is ever
// written to Trakt.
//
// Tokens travel only in headers and request bodies, never in a URL, and
// no error carries one, or the client secret.
package trakt

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/time/rate"
)

const (
	// DefaultAuthURL serves the OAuth endpoints: device code and token,
	// refresh and revoke.
	DefaultAuthURL = "https://auth.trakt.tv"
	// DefaultAPIURL serves everything else.
	DefaultAPIURL = "https://api.trakt.tv"
	// PageLimit is the most items Trakt answers per page; Paginate always
	// asks for it.
	PageLimit = 250

	// oobRedirectURI is the redirect_uri of a device-code app's refresh.
	oobRedirectURI = "urn:ietf:wg:oauth:2.0:oob"

	defaultTimeout = 15 * time.Second
	maxBody        = 32 << 20

	// A 429 is tried at most 5 times in all, a 5xx retried 3 times.
	maxRateLimitedTries = 5
	maxServerRetries    = 3
	// Retry-After is clamped to [1 s, 5 min]; without one (Cloudflare's
	// 429s) the wait is full jitter over backoffBase·2^(n-1).
	minRetryAfter = time.Second
	maxRetryAfter = 5 * time.Minute
	backoffBase   = time.Second

	// Trakt allows 500 authed GETs per 5 minutes and a POST a second.
	// getBurst + getsPer5Min stays under 500 in any 5-minute window.
	getsPer5Min  = 430
	getBurst     = 40
	postInterval = time.Second
)

// Clock is the time the client and the connection run on; tests use a
// fake one.
type Clock interface {
	Now() time.Time
	// Sleep waits d, or answers ctx's error when it ends first.
	Sleep(ctx context.Context, d time.Duration) error
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

func (realClock) Sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// Client talks to Trakt for one app (client id and secret). It is safe for
// concurrent use.
type Client struct {
	clientID     string
	clientSecret string
	userAgent    string
	authURL      string
	apiURL       string
	http         *http.Client
	clock        Clock
	gets         *rate.Limiter // nil: unlimited
	posts        *rate.Limiter // nil: unlimited
}

// Option configures a Client.
type Option func(*Client)

// WithBaseURLs points the client at other hosts (tests use trakttest).
func WithBaseURLs(authURL, apiURL string) Option {
	return func(c *Client) {
		c.authURL = strings.TrimRight(authURL, "/")
		c.apiURL = strings.TrimRight(apiURL, "/")
	}
}

// WithHTTPClient replaces the default HTTP client (15 s timeout).
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) { c.http = h }
}

// WithClock replaces real time for the retry waits.
func WithClock(clock Clock) Option {
	return func(c *Client) { c.clock = clock }
}

// WithLimiters replaces the rate limits of GETs and of everything else;
// nil is unlimited (tests).
func WithLimiters(gets, posts *rate.Limiter) Option {
	return func(c *Client) { c.gets, c.posts = gets, posts }
}

// NewClient creates a client for the app clientID. version goes in the
// User-Agent (momoshtrem/<version>), which Trakt requires.
func NewClient(clientID, clientSecret, version string, opts ...Option) *Client {
	c := &Client{
		clientID:     clientID,
		clientSecret: clientSecret,
		userAgent:    "momoshtrem/" + version,
		authURL:      DefaultAuthURL,
		apiURL:       DefaultAPIURL,
		http:         &http.Client{Timeout: defaultTimeout},
		clock:        realClock{},
		gets:         rate.NewLimiter(rate.Every(5*time.Minute/getsPer5Min), getBurst),
		posts:        rate.NewLimiter(rate.Every(postInterval), 1),
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// DeviceCode starts a device-code flow (POST /oauth/device/code).
func (c *Client) DeviceCode(ctx context.Context) (*DeviceAuthorization, error) {
	var out DeviceAuthorization
	_, err := c.send(ctx, request{
		method: http.MethodPost, base: c.authURL, path: "/oauth/device/code",
		body: map[string]string{"client_id": c.clientID},
	}, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// DeviceToken asks once whether the user approved the code (POST
// /oauth/device/token). Besides the tokens it answers
// ErrAuthorizationPending, ErrSlowDown, ErrInvalidDeviceCode,
// ErrDeviceCodeUsed, ErrDeviceCodeExpired or ErrDeviceCodeDenied. It never
// retries: the caller polls every interval.
func (c *Client) DeviceToken(ctx context.Context, deviceCode string) (*TokenSet, error) {
	var out TokenSet
	_, err := c.send(ctx, request{
		method: http.MethodPost, base: c.authURL, path: "/oauth/device/token", once: true,
		body: map[string]string{"code": deviceCode, "client_id": c.clientID, "client_secret": c.clientSecret},
	}, &out)
	if se := (*StatusError)(nil); errors.As(err, &se) {
		switch se.StatusCode {
		case http.StatusBadRequest:
			return nil, ErrAuthorizationPending
		case http.StatusNotFound:
			return nil, ErrInvalidDeviceCode
		case http.StatusConflict:
			return nil, ErrDeviceCodeUsed
		case http.StatusGone:
			return nil, ErrDeviceCodeExpired
		case http.StatusTeapot:
			return nil, ErrDeviceCodeDenied
		case http.StatusTooManyRequests:
			return nil, ErrSlowDown
		}
	}
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// RefreshToken trades the refresh token for a new pair (POST /oauth/token).
// Refresh tokens are single-use: once this succeeds the old pair is dead,
// so store the new one before using it. A 400 or 401 is ErrInvalidGrant.
func (c *Client) RefreshToken(ctx context.Context, refreshToken string) (*TokenSet, error) {
	var out TokenSet
	_, err := c.send(ctx, request{
		method: http.MethodPost, base: c.authURL, path: "/oauth/token",
		body: map[string]string{
			"refresh_token": refreshToken, "client_id": c.clientID, "client_secret": c.clientSecret,
			"redirect_uri": oobRedirectURI, "grant_type": "refresh_token",
		},
	}, &out)
	if se := (*StatusError)(nil); errors.As(err, &se) && (se.StatusCode == http.StatusBadRequest || se.StatusCode == http.StatusUnauthorized) {
		return nil, fmt.Errorf("%w: %v", ErrInvalidGrant, se)
	}
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// RevokeToken revokes an access token, and the connection with it (POST
// /oauth/revoke).
func (c *Client) RevokeToken(ctx context.Context, token string) error {
	_, err := c.send(ctx, request{
		method: http.MethodPost, base: c.authURL, path: "/oauth/revoke",
		body: map[string]string{"token": token, "client_id": c.clientID, "client_secret": c.clientSecret},
	}, nil)
	return err
}

// UserSettings reads the account the token belongs to (GET /users/settings).
func (c *Client) UserSettings(ctx context.Context, token string) (*UserSettings, error) {
	var out UserSettings
	if _, err := c.Get(ctx, token, "/users/settings", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Get GETs an API path with the access token, decodes the JSON body into v
// (nil discards it) and answers the response headers (pagination). A 401
// is ErrUnauthorized.
func (c *Client) Get(ctx context.Context, token, path string, query url.Values, v any) (http.Header, error) {
	return c.send(ctx, request{method: http.MethodGet, base: c.apiURL, path: path, query: query, token: token}, v)
}

// Getter GETs an API path and decodes its JSON into v: *Auth, with the
// connection's token.
type Getter interface {
	Get(ctx context.Context, path string, query url.Values, v any) (http.Header, error)
}

// Paginate GETs path page by page, PageLimit items each, and hands every
// page's items to fn. It stops after X-Pagination-Page-Count pages, a page
// shorter than X-Pagination-Limit, or an empty page;
// X-Pagination-Item-Count is wrong on some endpoints, so it isn't read.
func Paginate[T any](ctx context.Context, g Getter, path string, query url.Values, fn func([]T) error) error {
	for page := 1; ; page++ {
		q := url.Values{}
		for k, v := range query {
			q[k] = v
		}
		q.Set("page", strconv.Itoa(page))
		q.Set("limit", strconv.Itoa(PageLimit))

		var items []T
		header, err := g.Get(ctx, path, q, &items)
		if err != nil {
			return err
		}
		if len(items) == 0 {
			return nil
		}
		if err := fn(items); err != nil {
			return err
		}
		limit := PageLimit
		if n, err := strconv.Atoi(header.Get("X-Pagination-Limit")); err == nil && n > 0 {
			limit = n
		}
		pages, err := strconv.Atoi(header.Get("X-Pagination-Page-Count"))
		if (err == nil && page >= pages) || len(items) < limit {
			return nil
		}
	}
}

// request is one call to Trakt.
type request struct {
	method string
	base   string
	path   string
	query  url.Values
	token  string // the Bearer token; empty for the OAuth calls
	body   any    // sent as JSON
	once   bool   // no retries (device-code polling)
}

// send makes the request, retrying a 429 (Retry-After, else full-jitter
// backoff) and a 5xx. A transport error isn't retried: a POST may have
// landed, and a refresh token is single-use. Non-2xx answers are
// *StatusError.
func (c *Client) send(ctx context.Context, r request, v any) (http.Header, error) {
	var payload []byte
	if r.body != nil {
		var err error
		if payload, err = json.Marshal(r.body); err != nil {
			return nil, fmt.Errorf("failed to encode the request: %w", err)
		}
	}
	endpoint := r.base + r.path
	if len(r.query) > 0 {
		endpoint += "?" + r.query.Encode()
	}
	limiter := c.posts
	if r.method == http.MethodGet {
		limiter = c.gets
	}

	rateLimited, serverErrors := 0, 0
	for {
		if limiter != nil {
			if err := limiter.Wait(ctx); err != nil {
				return nil, fmt.Errorf("trakt rate limiter: %w", err)
			}
		}
		header, err := c.attempt(ctx, r, endpoint, payload, v)
		var se *StatusError
		if !errors.As(err, &se) || r.once {
			return header, err
		}
		var wait time.Duration
		switch {
		case se.StatusCode == http.StatusTooManyRequests && rateLimited+1 < maxRateLimitedTries:
			rateLimited++
			if wait = retryAfter(header.Get("Retry-After"), c.clock.Now()); wait == 0 {
				wait = fullJitter(rateLimited)
			}
		case se.StatusCode >= 500 && serverErrors < maxServerRetries:
			serverErrors++
			wait = fullJitter(serverErrors)
		default:
			return header, err
		}
		if err := c.clock.Sleep(ctx, wait); err != nil {
			return nil, err
		}
	}
}

// attempt makes one request and decodes a 2xx into v.
func (c *Client) attempt(ctx context.Context, r request, endpoint string, payload []byte, v any) (http.Header, error) {
	var body io.Reader
	if payload != nil {
		body = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, r.method, endpoint, body)
	if err != nil {
		return nil, fmt.Errorf("failed to build the request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("trakt-api-key", c.clientID)
	req.Header.Set("trakt-api-version", "2")
	if r.token != "" {
		req.Header.Set("Authorization", "Bearer "+r.token)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("trakt %s %s: %w", r.method, r.path, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return resp.Header, fmt.Errorf("trakt %s %s: failed to read the response: %w", r.method, r.path, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return resp.Header, &StatusError{StatusCode: resp.StatusCode, Code: oauthErrorCode(data)}
	}
	if v != nil && len(bytes.TrimSpace(data)) > 0 {
		if err := json.Unmarshal(data, v); err != nil {
			return resp.Header, fmt.Errorf("trakt %s %s: failed to decode the response: %w", r.method, r.path, err)
		}
	}
	return resp.Header, nil
}

// errorCode keeps an OAuth error code, e.g. invalid_grant, and nothing else.
var errorCode = regexp.MustCompile(`^[a-z_]{1,40}$`)

// oauthErrorCode is the "error" of an OAuth error body, when it's a plain
// code.
func oauthErrorCode(body []byte) string {
	var e struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(body, &e) != nil || !errorCode.MatchString(e.Error) {
		return ""
	}
	return e.Error
}

// retryAfter reads Retry-After (seconds or an HTTP date), clamped to
// [minRetryAfter, maxRetryAfter]; 0 when there's none.
func retryAfter(value string, now time.Time) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	var d time.Duration
	if secs, err := strconv.Atoi(value); err == nil {
		d = time.Duration(secs) * time.Second
	} else if at, err := http.ParseTime(value); err == nil {
		d = at.Sub(now)
	} else {
		return 0
	}
	return min(max(d, minRetryAfter), maxRetryAfter)
}

// fullJitter is a random wait in [0, backoffBase·2^(n-1)).
func fullJitter(n int) time.Duration {
	ceiling := min(backoffBase<<(n-1), maxRetryAfter)
	return rand.N(ceiling)
}
