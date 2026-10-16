package trakt

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/time/rate"

	"github.com/shapedtime/momoshtrem/internal/trakt/trakttest"
)

// fakeClock is time that moves only when something sleeps: Sleep records
// the wait, adds it to Now and returns at once. With block set, Sleep
// waits for its context instead (a poller that never polls).
type fakeClock struct {
	mu    sync.Mutex
	now   time.Time
	slept []time.Duration
	block bool
}

func newFakeClock() *fakeClock {
	return &fakeClock{now: time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Sleep(ctx context.Context, d time.Duration) error {
	c.mu.Lock()
	block := c.block
	c.mu.Unlock()
	if block {
		<-ctx.Done()
		return ctx.Err()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
	c.slept = append(c.slept, d)
	return nil
}

// Block makes Sleep wait for its context (true) or return at once.
func (c *fakeClock) Block(block bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.block = block
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func (c *fakeClock) Sleeps() []time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]time.Duration(nil), c.slept...)
}

func newTestClient(srv *trakttest.Server, clock Clock) *Client {
	return NewClient(trakttest.ClientID, trakttest.ClientSecret, "1.2.3",
		WithBaseURLs(srv.URL, srv.URL), WithClock(clock), WithLimiters(nil, nil))
}

// tokenGetter is a Getter over the client with a fixed token.
type tokenGetter struct {
	c     *Client
	token string
}

func (g tokenGetter) Get(ctx context.Context, path string, query url.Values, v any) (http.Header, error) {
	return g.c.Get(ctx, g.token, path, query, v)
}

func TestClient_Headers(t *testing.T) {
	srv := trakttest.New(t)
	c := newTestClient(srv, newFakeClock())
	ctx := context.Background()
	access, refresh := srv.Grant()

	user, err := c.UserSettings(ctx, access)
	if err != nil {
		t.Fatal(err)
	}
	if user.User.Username != "Teymur" || user.User.IDs.Slug != "teymur" {
		t.Errorf("user = %+v", user.User)
	}
	if _, err := c.RefreshToken(ctx, refresh); err != nil {
		t.Fatal(err)
	}

	reqs := srv.Requests()
	if len(reqs) != 2 {
		t.Fatalf("requests = %+v", reqs)
	}
	for _, r := range reqs {
		for name, want := range map[string]string{
			"Content-Type": "application/json", "User-Agent": "momoshtrem/1.2.3",
			"trakt-api-key": trakttest.ClientID, "trakt-api-version": "2",
		} {
			if got := r.Header.Get(name); got != want {
				t.Errorf("%s %s: %s = %q, want %q", r.Method, r.Path, name, got, want)
			}
		}
	}
	if got := reqs[0].Header.Get("Authorization"); got != "Bearer "+access {
		t.Errorf("GET Authorization = %q", got)
	}
	if got := reqs[1].Header.Get("Authorization"); got != "" {
		t.Errorf("the refresh sent Authorization %q", got)
	}
	if b := reqs[1].Body; b["grant_type"] != "refresh_token" || b["redirect_uri"] != "urn:ietf:wg:oauth:2.0:oob" ||
		b["refresh_token"] != refresh || b["client_secret"] != trakttest.ClientSecret {
		t.Errorf("refresh body = %v", b)
	}
}

func TestClient_DefaultLimitsStayUnderTrakts(t *testing.T) {
	c := NewClient("id", "secret", "dev")
	window := 5 * time.Minute
	if most := float64(c.gets.Burst()) + float64(c.gets.Limit())*window.Seconds(); most > 500 {
		t.Errorf("up to %.0f GETs in 5 minutes, want at most 500", most)
	}
	if c.posts.Limit() != rate.Every(time.Second) || c.posts.Burst() != 1 {
		t.Errorf("POSTs: %v a second, burst %d; want 1 and 1", c.posts.Limit(), c.posts.Burst())
	}
}

func TestClient_RetryAfter(t *testing.T) {
	tests := []struct {
		header string
		want   time.Duration
	}{
		{"7", 7 * time.Second},
		{"0", time.Second},       // clamped up
		{"900", 5 * time.Minute}, // clamped down
		{"Fri, 16 Oct 2026 12:00:30 GMT", 30 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.header, func(t *testing.T) {
			srv := trakttest.New(t)
			clock := newFakeClock()
			c := newTestClient(srv, clock)
			access, _ := srv.Grant()
			srv.Queue("/users/settings", trakttest.Reply{Status: 429, Header: map[string]string{"Retry-After": tt.header}})

			if _, err := c.UserSettings(context.Background(), access); err != nil {
				t.Fatal(err)
			}
			if got := clock.Sleeps(); len(got) != 1 || got[0] != tt.want {
				t.Errorf("sleeps = %v, want [%v]", got, tt.want)
			}
		})
	}
}

// Cloudflare's 429s carry no Retry-After: full jitter, 5 tries in all.
func TestClient_RateLimitedWithoutHeaders(t *testing.T) {
	srv := trakttest.New(t)
	clock := newFakeClock()
	c := newTestClient(srv, clock)
	access, _ := srv.Grant()
	for range 5 {
		srv.Queue("/users/settings", trakttest.Reply{Status: 429})
	}

	_, err := c.UserSettings(context.Background(), access)
	var se *StatusError
	if !errors.As(err, &se) || se.StatusCode != 429 {
		t.Fatalf("error = %v, want a 429", err)
	}
	if n := srv.Count("/users/settings"); n != 5 {
		t.Errorf("requests = %d, want 5", n)
	}
	sleeps := clock.Sleeps()
	if len(sleeps) != 4 {
		t.Fatalf("sleeps = %v, want 4", sleeps)
	}
	for i, d := range sleeps {
		if ceiling := time.Second << i; d < 0 || d >= ceiling {
			t.Errorf("sleep %d = %v, want in [0, %v)", i+1, d, ceiling)
		}
	}
}

func TestClient_ServerErrors(t *testing.T) {
	t.Run("retried 3 times", func(t *testing.T) {
		srv := trakttest.New(t)
		c := newTestClient(srv, newFakeClock())
		access, _ := srv.Grant()
		for range 4 {
			srv.Queue("/users/settings", trakttest.Reply{Status: 502})
		}
		_, err := c.UserSettings(context.Background(), access)
		if se := (*StatusError)(nil); !errors.As(err, &se) || se.StatusCode != 502 {
			t.Errorf("error = %v, want a 502", err)
		}
		if n := srv.Count("/users/settings"); n != 4 {
			t.Errorf("requests = %d, want 4", n)
		}
	})
	t.Run("then it answers", func(t *testing.T) {
		srv := trakttest.New(t)
		c := newTestClient(srv, newFakeClock())
		access, _ := srv.Grant()
		srv.Queue("/users/settings", trakttest.Reply{Status: 500}, trakttest.Reply{Status: 503}, trakttest.Reply{Status: 504})
		if _, err := c.UserSettings(context.Background(), access); err != nil {
			t.Errorf("error = %v", err)
		}
	})
	t.Run("401 is not retried", func(t *testing.T) {
		srv := trakttest.New(t)
		c := newTestClient(srv, newFakeClock())
		_, err := c.UserSettings(context.Background(), "not-a-token")
		if !errors.Is(err, ErrUnauthorized) || srv.Count("/users/settings") != 1 {
			t.Errorf("error = %v after %d requests, want ErrUnauthorized after 1", err, srv.Count("/users/settings"))
		}
	})
}

func TestClient_DeviceToken(t *testing.T) {
	tests := []struct {
		status int
		want   error
	}{
		{400, ErrAuthorizationPending},
		{404, ErrInvalidDeviceCode},
		{409, ErrDeviceCodeUsed},
		{410, ErrDeviceCodeExpired},
		{418, ErrDeviceCodeDenied},
		{429, ErrSlowDown},
		{200, nil},
	}
	for _, tt := range tests {
		t.Run(strconv.Itoa(tt.status), func(t *testing.T) {
			srv := trakttest.New(t)
			c := newTestClient(srv, newFakeClock())
			srv.SetDeviceAnswers(tt.status)
			set, err := c.DeviceToken(context.Background(), trakttest.DeviceCode)
			if !errors.Is(err, tt.want) || (tt.want == nil && set.AccessToken == "") {
				t.Errorf("DeviceToken = %+v, %v; want %v", set, err, tt.want)
			}
			if n := srv.Count("/oauth/device/token"); n != 1 {
				t.Errorf("requests = %d, want 1 (polling isn't retried)", n)
			}
		})
	}
}

// Refresh tokens are single-use and rotate; no error carries a token or
// the client secret.
func TestClient_RefreshRotates(t *testing.T) {
	srv := trakttest.New(t)
	c := newTestClient(srv, newFakeClock())
	ctx := context.Background()
	access, refresh := srv.Grant()

	set, err := c.RefreshToken(ctx, refresh)
	if err != nil {
		t.Fatal(err)
	}
	if set.AccessToken == access || set.RefreshToken == refresh || set.ExpiresIn != 7*24*3600 {
		t.Errorf("refreshed = %+v", set)
	}
	_, err = c.RefreshToken(ctx, refresh)
	if !errors.Is(err, ErrInvalidGrant) {
		t.Fatalf("reusing the refresh token = %v, want ErrInvalidGrant", err)
	}
	if _, err := c.UserSettings(ctx, access); !errors.Is(err, ErrUnauthorized) {
		t.Errorf("the replaced access token = %v, want ErrUnauthorized", err)
	}

	srv.Close()
	_, netErr := c.RefreshToken(ctx, set.RefreshToken)
	_, getErr := c.UserSettings(ctx, set.AccessToken)
	for _, err := range []error{err, netErr, getErr} {
		for _, secret := range []string{refresh, set.RefreshToken, set.AccessToken, trakttest.ClientSecret} {
			if err == nil || strings.Contains(err.Error(), secret) {
				t.Errorf("error %v carries %q", err, secret)
			}
		}
	}
}

// Trakt stopped issuing secrets to new apps: without one, the OAuth calls
// send the client id alone, and work.
func TestClient_WithoutSecret(t *testing.T) {
	srv := trakttest.New(t)
	c := NewClient(trakttest.ClientID, "", "1.2.3",
		WithBaseURLs(srv.URL, srv.URL), WithClock(newFakeClock()), WithLimiters(nil, nil))
	ctx := context.Background()

	set, err := c.DeviceToken(ctx, trakttest.DeviceCode)
	if err != nil {
		t.Fatal(err)
	}
	next, err := c.RefreshToken(ctx, set.RefreshToken)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.RevokeToken(ctx, next.AccessToken); err != nil {
		t.Fatal(err)
	}
	for _, r := range srv.Requests() {
		if _, sent := r.Body["client_secret"]; sent || r.Body["client_id"] != trakttest.ClientID {
			t.Errorf("%s %s body = %v, want the client id without a secret", r.Method, r.Path, r.Body)
		}
	}
}

func TestPaginate(t *testing.T) {
	items := func(n int) []int {
		out := make([]int, n)
		for i := range out {
			out[i] = i
		}
		return out
	}
	tests := []struct {
		name      string
		pages     [][]int           // what each page answers
		headers   map[string]string // on every page
		wantPages int               // requests
	}{
		{"the page count", [][]int{items(250), items(250), items(250)}, map[string]string{"X-Pagination-Page-Count": "2"}, 2},
		{"a short page", [][]int{items(250), items(3), items(250)}, nil, 2},
		{"an empty page", [][]int{items(250), items(250), {}}, nil, 3},
		{"not the item count", [][]int{items(250), items(250), items(1)}, map[string]string{"X-Pagination-Page-Count": "3", "X-Pagination-Item-Count": "1"}, 3},
		{"Trakt's own limit", [][]int{items(100), items(100), items(40)}, map[string]string{"X-Pagination-Limit": "100"}, 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := trakttest.New(t)
			c := newTestClient(srv, newFakeClock())
			access, _ := srv.Grant()
			srv.Handle("/sync/history/movies", func(w http.ResponseWriter, r *http.Request) {
				n, _ := strconv.Atoi(r.URL.Query().Get("page"))
				for k, v := range tt.headers {
					w.Header().Set(k, v)
				}
				page := []int{}
				if n >= 1 && n <= len(tt.pages) {
					page = tt.pages[n-1]
				}
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, strings.ReplaceAll(fmt.Sprint(page), " ", ","))
			})

			got := 0
			err := Paginate(context.Background(), tokenGetter{c, access}, "/sync/history/movies", url.Values{"start_at": {"2026-10-01T00:00:00Z"}},
				func(page []int) error { got += len(page); return nil })
			if err != nil {
				t.Fatal(err)
			}
			reqs := srv.Requests()
			if len(reqs) != tt.wantPages {
				t.Fatalf("requests = %d, want %d", len(reqs), tt.wantPages)
			}
			want := 0
			for i, r := range reqs {
				q := r.Query
				if q.Get("page") != strconv.Itoa(i+1) || q.Get("limit") != "250" || q.Get("start_at") == "" {
					t.Errorf("request %d query = %v", i+1, q)
				}
				want += len(tt.pages[i])
			}
			if got != want {
				t.Errorf("items = %d, want %d", got, want)
			}
		})
	}
}

// The fake's own lists page like Trakt's.
func TestPaginate_FakeList(t *testing.T) {
	srv := trakttest.New(t)
	c := newTestClient(srv, newFakeClock())
	access, _ := srv.Grant()
	type item struct {
		ID int `json:"id"`
	}
	var list []item
	for i := range 600 {
		list = append(list, item{ID: i})
	}
	srv.List("/sync/watchlist", list)

	var got []item
	if err := Paginate(context.Background(), tokenGetter{c, access}, "/sync/watchlist", nil, func(page []item) error {
		got = append(got, page...)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(got) != 600 || got[599].ID != 599 || srv.Count("/sync/watchlist") != 3 {
		t.Errorf("%d items after %d requests, want 600 after 3", len(got), srv.Count("/sync/watchlist"))
	}
}
