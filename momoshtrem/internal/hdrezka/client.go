// Package hdrezka is a small client for HDRezka (rezka.ag and its mirrors):
// search, title pages with their translations, seasons and episodes, and the
// CDN links of a movie or an episode, resolved to a progressive MP4.
//
// The protocol follows n0madic/go-hdrezka
// (https://github.com/n0madic/go-hdrezka, MIT). This client adds contexts, a
// rate limit, retries, status checks, anti-bot detection and typed errors,
// and keeps the signed CDN URLs out of errors.
package hdrezka

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"

	"golang.org/x/time/rate"
)

const (
	// DefaultBaseURL is the main mirror.
	DefaultBaseURL = "https://rezka.ag"
	// DefaultUserAgent names the client. It must not start with "Mozilla":
	// the site's Anubis proxy challenges browser user agents only.
	DefaultUserAgent = "momoshtrem/1.0 (+https://github.com/ShapedTime/momoshtrem)"
)

const (
	defaultTimeout  = 20 * time.Second
	defaultAttempts = 3
	defaultBackoff  = 500 * time.Millisecond
	defaultRPS      = 4
	defaultBurst    = 8

	// maxBody caps a site response; a title page is about 150 KB.
	maxBody = 4 << 20
)

// Client talks to one HDRezka mirror. Site requests share a rate limit and a
// cookie jar and are retried on 5xx, 429 and timeouts; CDN requests
// (ContentLength) are retried but not rate-limited. No error carries a
// request URL, because CDN URLs are signed.
type Client struct {
	baseURL    string
	httpClient *http.Client
	userAgent  string
	limiter    *rate.Limiter // site requests only; nil: unlimited
	attempts   int
	backoff    time.Duration

	// sleep waits between attempts and now stamps CDN requests; tests
	// replace them.
	sleep func(ctx context.Context, d time.Duration) error
	now   func() time.Time
}

// Option configures a Client.
type Option func(*Client)

// WithBaseURL points the client at another mirror, or at httptest.
func WithBaseURL(u string) Option {
	return func(c *Client) { c.baseURL = strings.TrimRight(u, "/") }
}

// WithHTTPClient replaces the default HTTP client (20 s timeout). The client
// gets its own cookie jar when h has none; h itself isn't modified.
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) { c.httpClient = h }
}

// WithUserAgent replaces DefaultUserAgent.
func WithUserAgent(ua string) Option {
	return func(c *Client) { c.userAgent = ua }
}

// WithRateLimit caps site requests per second, with bursts of up to burst.
// rps <= 0 disables the limit.
func WithRateLimit(rps float64, burst int) Option {
	return func(c *Client) {
		if rps <= 0 {
			c.limiter = nil
			return
		}
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

// NewClient creates a client for DefaultBaseURL, 4 site requests per second
// with bursts of 8, and 3 attempts per request.
func NewClient(opts ...Option) *Client {
	c := &Client{
		baseURL:    DefaultBaseURL,
		httpClient: &http.Client{Timeout: defaultTimeout},
		userAgent:  DefaultUserAgent,
		limiter:    rate.NewLimiter(defaultRPS, defaultBurst),
		attempts:   defaultAttempts,
		backoff:    defaultBackoff,
		sleep:      sleepContext,
		now:        time.Now,
	}
	for _, opt := range opts {
		opt(c)
	}
	if c.httpClient.Jar == nil {
		jar, _ := cookiejar.New(nil) // never fails without options
		hc := *c.httpClient
		hc.Jar = jar
		c.httpClient = &hc
	}
	return c
}

// BaseURL returns the mirror the client fetches from, without a trailing
// slash.
func (c *Client) BaseURL() string { return c.baseURL }

// UserAgent returns the User-Agent the client sends.
func (c *Client) UserAgent() string { return c.userAgent }

// Ping fetches the home page once, without retries or the rate limit: it
// checks the mirror answers with content rather than an anti-bot page
// (ErrChallenge).
func (c *Client) Ping(ctx context.Context) error {
	_, _, err := c.do(ctx, siteRequest{path: "/"})
	return err
}

// siteRequest is one request to the site, as opposed to its CDN.
type siteRequest struct {
	path    string     // under the base URL, with any query
	form    url.Values // non-nil: POST it form-urlencoded
	referer string     // "" sends none
	ajax    bool       // send X-Requested-With, like the site's own scripts
}

// fetch makes a site request, rate-limited and retried, and returns the
// body of a 200.
func (c *Client) fetch(ctx context.Context, r siteRequest) ([]byte, error) {
	var body []byte
	err := c.retry(ctx, func() (bool, error) {
		if c.limiter != nil {
			if err := c.limiter.Wait(ctx); err != nil {
				return false, fmt.Errorf("rate limiter: %w", err)
			}
		}
		var retry bool
		var err error
		body, retry, err = c.do(ctx, r)
		return retry, err
	})
	return body, err
}

// do makes one site request. retry reports whether the failure is transient.
func (c *Client) do(ctx context.Context, r siteRequest) (body []byte, retry bool, err error) {
	method, reqBody := http.MethodGet, io.Reader(nil)
	if r.form != nil {
		method, reqBody = http.MethodPost, strings.NewReader(r.form.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+r.path, reqBody)
	if err != nil {
		return nil, false, fmt.Errorf("failed to build request: %w", withoutURL(err))
	}
	req.Header.Set("User-Agent", c.userAgent)
	if r.form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	}
	if r.referer != "" {
		req.Header.Set("Referer", r.referer)
	}
	if r.ajax {
		req.Header.Set("X-Requested-With", "XMLHttpRequest")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		err = withoutURL(err)
		return nil, isTimeout(err) && ctx.Err() == nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err = io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		err = withoutURL(err)
		return nil, isTimeout(err) && ctx.Err() == nil, fmt.Errorf("failed to read response: %w", err)
	}
	// Anubis answers 200 and Cloudflare 403 or 503, so the body is checked
	// before the status.
	if isChallenge(body) {
		return nil, false, ErrChallenge
	}
	switch {
	case resp.StatusCode == http.StatusOK:
	case resp.StatusCode == http.StatusNotFound:
		return nil, false, ErrNotFound
	case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
		return nil, true, &StatusError{StatusCode: resp.StatusCode}
	default:
		return nil, false, &StatusError{StatusCode: resp.StatusCode}
	}
	if len(body) > maxBody {
		return nil, false, &ParseError{What: "response, which is over 4 MB"}
	}
	return body, false, nil
}

// retry calls attempt until it succeeds, fails for good or runs out of
// attempts, backing off in between.
func (c *Client) retry(ctx context.Context, attempt func() (retry bool, err error)) error {
	for n := 1; ; n++ {
		retry, err := attempt()
		if err == nil {
			return nil
		}
		if !retry || n >= c.attempts || ctx.Err() != nil {
			return err
		}
		if err := c.sleep(ctx, backoffDelay(c.backoff, n)); err != nil {
			return err
		}
	}
}

// challengeMarkers identify anti-bot pages: Anubis (in front of rezka.ag;
// it challenges browser user agents), Cloudflare and DDoS-Guard.
var challengeMarkers = [][]byte{
	[]byte(`id="anubis_challenge"`),
	[]byte(`/.within.website/x/cmd/anubis/`),
	[]byte(`<title>Just a moment...</title>`),
	[]byte(`<title>DDoS-Guard</title>`),
	[]byte(`<title>DDOS-GUARD</title>`),
}

func isChallenge(body []byte) bool {
	for _, m := range challengeMarkers {
		if bytes.Contains(body, m) {
			return true
		}
	}
	return false
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

// withoutURL strips the request URL from transport errors: CDN URLs are
// signed, and these errors end up in logs and API responses.
func withoutURL(err error) error {
	var uerr *url.Error
	if errors.As(err, &uerr) {
		return uerr.Err
	}
	return err
}
