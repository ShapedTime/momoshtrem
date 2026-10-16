// Package torznab talks to Jackett over Torznab: the configured indexers and
// their caps, one search request per indexer, per-indexer health, a small
// fan-out for running many searches, and fetching a result's .torrent.
//
// The API key travels in every request URL and in the download links Jackett
// returns, so no error, log line or returned string carries a request URL or
// a link: transport errors lose their URL, and Jackett's own error text loses
// any URL or key it contains.
package torznab

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

const (
	// DefaultSearchTimeout bounds a search request. An indexer behind
	// Cloudflare answers only after FlareSolverr's challenge (about 12 s),
	// Jackett sends one challenge at a time, and 1337x then reads 4 pages
	// 3 s apart. A search with cold cookies had requests take 59 s (#106).
	DefaultSearchTimeout = 90 * time.Second
	// DefaultDownloadTimeout bounds a .torrent download. 1337x's goes
	// through Cloudflare too, and resolve answers within one request, under
	// the web proxy's 30 s.
	DefaultDownloadTimeout = 25 * time.Second
	// IndexersTimeout bounds the indexer-list request, which Jackett answers
	// from its own config.
	IndexersTimeout = 8 * time.Second
	// IndexersTTL is how long Indexers reuses the configured-indexer list.
	IndexersTTL = 10 * time.Minute

	// maxBody caps a Torznab response: 100 results are about 250 KB, and
	// the indexer list of 7 indexers about 150 KB.
	maxBody = 8 << 20
	// maxTorrent caps a .torrent download.
	maxTorrent = 5 << 20
	// maxErrorText caps Jackett's error text in returned errors.
	maxErrorText = 200
)

// apiPath is the Torznab endpoint of one indexer, or of "all". Indexer ids
// come from Jackett's own list.
func apiPath(indexer string) string {
	return "/api/v2.0/indexers/" + indexer + "/results/torznab/api"
}

// Client talks to one Jackett instance. It is safe for concurrent use.
type Client struct {
	base    *url.URL // nil when the base URL is invalid
	apiKey  string
	http    *http.Client
	noRedir *http.Client // http, without following redirects (Download)
	log     *slog.Logger
	now     func() time.Time

	searchTimeout   time.Duration
	downloadTimeout time.Duration

	health *healthTracker

	group     singleflight.Group // one indexer-list fetch at a time
	mu        sync.Mutex
	indexers  []Indexer
	fetchedAt time.Time
}

// Option configures a Client.
type Option func(*Client)

// WithHTTPClient replaces the default HTTP client. Its Timeout should be
// zero or longer than the request timeouts, which the client applies per
// request through the context. h itself isn't modified.
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) { c.http = h }
}

// WithTimeouts sets the per-request timeouts: search for searches (default
// 90 s), download for .torrent files (default 25 s). Zero keeps the
// default. The indexer list always has IndexersTimeout.
func WithTimeouts(search, download time.Duration) Option {
	return func(c *Client) {
		if search > 0 {
			c.searchTimeout = search
		}
		if download > 0 {
			c.downloadTimeout = download
		}
	}
}

// WithClock replaces time.Now for the indexer cache, health timestamps and
// elapsed times.
func WithClock(now func() time.Time) Option {
	return func(c *Client) { c.now = now }
}

// WithLogger replaces slog.Default().
func WithLogger(l *slog.Logger) Option {
	return func(c *Client) { c.log = l }
}

// New creates a client for the Jackett at baseURL (e.g. http://jackett:9117).
// An invalid baseURL doesn't fail here: every request returns
// ErrInvalidBaseURL, so /status can say what's wrong.
func New(baseURL, apiKey string, opts ...Option) *Client {
	c := &Client{
		apiKey:          apiKey,
		http:            &http.Client{},
		log:             slog.Default(),
		now:             time.Now,
		searchTimeout:   DefaultSearchTimeout,
		downloadTimeout: DefaultDownloadTimeout,
		health:          newHealthTracker(),
	}
	if u, err := url.Parse(strings.TrimRight(strings.TrimSpace(baseURL), "/")); err == nil &&
		(u.Scheme == "http" || u.Scheme == "https") && u.Host != "" {
		u.RawQuery, u.Fragment = "", ""
		c.base = u
	}
	for _, opt := range opts {
		opt(c)
	}
	noRedir := *c.http
	noRedir.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	c.noRedir = &noRedir
	return c
}

// endpoint is path under the base URL, with query and the API key.
func (c *Client) endpoint(path string, query url.Values) (string, error) {
	if c.base == nil {
		return "", ErrInvalidBaseURL
	}
	u := *c.base
	u.Path = c.base.Path + path
	u.RawPath = ""
	q := url.Values{}
	for k, v := range query {
		q[k] = v
	}
	q.Set("apikey", c.apiKey)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// get fetches a Torznab endpoint within timeout and returns the body of a
// 2xx answer that isn't a Torznab error document.
func (c *Client) get(ctx context.Context, path string, query url.Values, timeout time.Duration) ([]byte, error) {
	endpoint, err := c.endpoint(path, query)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, ErrInvalidBaseURL
	}
	start := time.Now()
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, c.transportError(ctx, err, start)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", c.transportError(ctx, err, start))
	}
	if len(body) > maxBody {
		return nil, errors.New("jackett response is over 8 MB")
	}

	// Jackett reports a bad API key (HTTP 200) and a failed indexer (HTTP
	// 400) as a Torznab error document.
	if e, ok := c.torznabError(body); ok {
		return nil, e
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, &HTTPError{StatusCode: resp.StatusCode, Message: c.jsonErrorText(body)}
	}
	return body, nil
}

// transportError turns a failed request into an error without its URL,
// mapping this request's timeout to ErrTimeout. ctx is the request's
// context; a cancelled parent comes back as context.Canceled.
func (c *Client) transportError(ctx context.Context, err error, start time.Time) error {
	err = withoutURL(err)
	if errors.Is(ctx.Err(), context.Canceled) {
		return context.Canceled
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || isTimeout(err) {
		if deadline, ok := ctx.Deadline(); ok {
			return fmt.Errorf("%w after %s", ErrTimeout, roundDuration(deadline.Sub(start)))
		}
		return ErrTimeout
	}
	return fmt.Errorf("jackett request failed: %s", c.clean(err.Error()))
}

// torznabError reads a Torznab error document such as
// <error code="100" description="Invalid API Key"/>.
func (c *Client) torznabError(body []byte) (*Error, bool) {
	var doc struct {
		XMLName     xml.Name
		Code        int    `xml:"code,attr"`
		Description string `xml:"description,attr"`
	}
	if err := xml.Unmarshal(body, &doc); err != nil || doc.XMLName.Local != "error" {
		return nil, false
	}
	desc := c.clean(doc.Description)
	if desc == "" {
		desc = fmt.Sprintf("code %d", doc.Code)
	}
	return &Error{Code: doc.Code, Description: desc}, true
}

// jsonErrorText reads the message of Jackett's JSON error answers, e.g. a
// 500 {"result":"error","error":"Unknown indexer: x"}.
func (c *Client) jsonErrorText(body []byte) string {
	var doc struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(body, &doc) != nil {
		return ""
	}
	return c.clean(doc.Error)
}

var (
	// urlPattern matches anything URL-shaped in Jackett's error text.
	urlPattern = regexp.MustCompile(`[a-zA-Z][a-zA-Z0-9+.\-]*://\S+|magnet:\?\S+`)
	// exceptionPrefix matches the .NET exception names Jackett puts before
	// an indexer's message: "Jackett.Common.IndexerException: Exception (1337x): ".
	exceptionPrefix = regexp.MustCompile(`^(?:[\w.]+Exception: )+(?:Exception \([^)]*\): )?`)
)

// clean makes text from Jackett safe to show: the first line, without
// exception names, URLs or the API key, at most 200 characters.
func (c *Client) clean(s string) string {
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		s = s[:i]
	}
	s = exceptionPrefix.ReplaceAllString(strings.TrimSpace(s), "")
	s = urlPattern.ReplaceAllString(s, "[link]")
	if c.apiKey != "" {
		s = strings.ReplaceAll(s, c.apiKey, "[key]")
	}
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > maxErrorText {
		s = string(r[:maxErrorText]) + "…"
	}
	return s
}

// xmlProblem describes an XML decoding error without quoting the body: a
// syntax error's text can echo part of a URL that carries the API key.
func xmlProblem(err error) string {
	var syntax *xml.SyntaxError
	if errors.As(err, &syntax) {
		return fmt.Sprintf("XML syntax error on line %d", syntax.Line)
	}
	return "unexpected document"
}

// SafeError is err's text, cleaned like Jackett's: fit for an API response
// or a log line.
func (c *Client) SafeError(err error) string {
	if err == nil {
		return ""
	}
	return c.clean(err.Error())
}

// roundDuration rounds to seconds, or to milliseconds under a second.
func roundDuration(d time.Duration) time.Duration {
	if d >= time.Second {
		return d.Round(time.Second)
	}
	return d.Round(time.Millisecond)
}

// isTimeout reports whether err is a network timeout.
func isTimeout(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

// withoutURL strips the request URL from transport errors, because the URL
// carries the API key.
func withoutURL(err error) error {
	var uerr *url.Error
	if errors.As(err, &uerr) {
		return uerr.Err
	}
	return err
}
