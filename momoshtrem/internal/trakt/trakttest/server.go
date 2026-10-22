// Package trakttest is a fake Trakt for tests. One server stands in for
// both hosts (auth.trakt.tv and api.trakt.tv): the device-code flow,
// single-use refresh tokens that rotate, revoke, /users/settings,
// paginated lists, scripted replies (429 with or without Retry-After, 5xx)
// and a log of every request.
//
// Each token belongs to the user SetUser named when it was granted (a
// refresh keeps the user), so several accounts can be connected at once:
// /users/settings answers the token's user, and ListFor serves a list to
// one user only.
//
// Like Trakt it refuses requests without a User-Agent or without the app's
// trakt-api-key and trakt-api-version headers (403), and authed API paths
// answer 401 for a token that isn't current: replaced by a refresh,
// revoked or expired.
package trakttest

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// The app and the device code the server knows.
const (
	ClientID        = "fake-client-id"
	ClientSecret    = "fake-client-secret"
	DeviceCode      = "fake-device-code"
	UserCode        = "A1B2C3D4"
	VerificationURL = "https://trakt.tv/activate"
)

// Reply is a scripted answer (Queue).
type Reply struct {
	Status int
	Header map[string]string // e.g. {"Retry-After": "7"}
	Body   string            // JSON
}

// Request is one request the server received.
type Request struct {
	Method string
	Path   string
	Query  url.Values
	Header http.Header
	Body   map[string]string // a JSON body's string fields
}

// Server is the fake. Its zero configuration approves the device code on
// the first poll, for user Teymur (slug teymur), with 7-day tokens.
type Server struct {
	*httptest.Server

	mu            sync.Mutex
	now           func() time.Time
	interval      int
	expiresIn     int
	deviceAnswers []int
	deviceUsed    bool
	lifetime      time.Duration
	user          user // SetUser's: the next grants'
	seq           int
	access        map[string]time.Time // current access tokens and when they expire
	refresh       map[string]bool      // issued refresh tokens: true until used or revoked
	pairs         map[string]string
	owners        map[string]user // every token granted, access and refresh, and its user
	revoked       []string
	queued        map[string][]Reply
	lists         map[string][]json.RawMessage
	userLists     map[string]map[string][]json.RawMessage // ListFor's: slug → path → items
	handlers      map[string]http.HandlerFunc
	log           []Request
}

// user is a Trakt account.
type user struct{ username, slug string }

// New starts a fake Trakt, closed when the test ends.
func New(t testing.TB) *Server {
	s := &Server{
		now:           time.Now,
		interval:      5,
		expiresIn:     600,
		deviceAnswers: []int{http.StatusOK},
		lifetime:      7 * 24 * time.Hour,
		user:          user{username: "Teymur", slug: "teymur"},
		access:        map[string]time.Time{},
		refresh:       map[string]bool{},
		pairs:         map[string]string{},
		owners:        map[string]user{},
		queued:        map[string][]Reply{},
		lists:         map[string][]json.RawMessage{},
		userLists:     map[string]map[string][]json.RawMessage{},
		handlers:      map[string]http.HandlerFunc{},
	}
	s.Server = httptest.NewServer(http.HandlerFunc(s.serve))
	t.Cleanup(s.Close)
	return s
}

// SetClock replaces time.Now for the tokens' created_at.
func (s *Server) SetClock(now func() time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.now = now
}

// SetDeviceCode sets the interval and expires_in (seconds) the device code
// answers with.
func (s *Server) SetDeviceCode(interval, expiresIn int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.interval, s.expiresIn = interval, expiresIn
}

// SetDeviceAnswers sets the statuses POST /oauth/device/token answers in
// turn, the last one repeating: 400 pending, 404, 409, 410, 418, 429, and
// 200, which grants tokens (the code is used after that: 409).
func (s *Server) SetDeviceAnswers(statuses ...int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deviceAnswers = statuses
	s.deviceUsed = false
}

// SetUser sets the account the tokens granted from now on belong to (the
// user who approves the device code); tokens granted before keep theirs.
func (s *Server) SetUser(username, slug string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.user = user{username: username, slug: slug}
}

// SetTokenLifetime sets the expires_in of the tokens granted from now on.
func (s *Server) SetTokenLifetime(d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lifetime = d
}

// Grant issues a token pair without the device flow, to SetUser's user.
func (s *Server) Grant() (access, refresh string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	set := s.grantLocked(s.user)
	return set["access_token"].(string), set["refresh_token"].(string)
}

// AccessToken is the newest access token, "" when there's none.
func (s *Server) AccessToken() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.seq == 0 {
		return ""
	}
	return fmt.Sprintf("access-%d", s.seq)
}

// RevokeAll makes every token invalid, as if the user revoked the app on
// trakt.tv: API calls answer 401 and refreshes invalid_grant.
func (s *Server) RevokeAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	clear(s.access)
	clear(s.refresh)
}

// Revoked are the tokens POST /oauth/revoke received.
func (s *Server) Revoked() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.revoked...)
}

// Queue scripts the next answers to path (any method), served before
// anything else, one per request.
func (s *Server) Queue(path string, replies ...Reply) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.queued[path] = append(s.queued[path], replies...)
}

// List serves items (a slice) at path as an authed, paginated list: page
// and limit (default 10, at most 250) with the X-Pagination headers. It's
// every user's list, but for a user ListFor gave their own.
func (s *Server) List(path string, items any) {
	raw := marshalItems(items)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lists[path] = raw
}

// ListFor serves items at path as List does, to the user with slug only.
func (s *Server) ListFor(slug, path string, items any) {
	raw := marshalItems(items)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.userLists[slug] == nil {
		s.userLists[slug] = map[string][]json.RawMessage{}
	}
	s.userLists[slug][path] = raw
}

func marshalItems(items any) []json.RawMessage {
	v := reflect.ValueOf(items)
	raw := make([]json.RawMessage, v.Len())
	for i := range raw {
		b, err := json.Marshal(v.Index(i).Interface())
		if err != nil {
			panic(err)
		}
		raw[i] = b
	}
	return raw
}

// Handle serves path with h once the token checks out.
func (s *Server) Handle(path string, h http.HandlerFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handlers[path] = h
}

// Requests is every request so far, in order.
func (s *Server) Requests() []Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Request(nil), s.log...)
}

// Count is how many requests path received.
func (s *Server) Count(path string) int {
	n := 0
	for _, r := range s.Requests() {
		if r.Path == path {
			n++
		}
	}
	return n
}

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	body := map[string]string{}
	var fields map[string]any
	if json.Unmarshal(raw, &fields) == nil {
		for k, v := range fields {
			if str, ok := v.(string); ok {
				body[k] = str
			}
		}
	}

	s.mu.Lock()
	s.log = append(s.log, Request{Method: r.Method, Path: r.URL.Path, Query: r.URL.Query(), Header: r.Header.Clone(), Body: body})
	if q := s.queued[r.URL.Path]; len(q) > 0 {
		s.queued[r.URL.Path] = q[1:]
		s.mu.Unlock()
		for k, v := range q[0].Header {
			w.Header().Set(k, v)
		}
		reply(w, q[0].Status, q[0].Body)
		return
	}
	s.mu.Unlock()

	ua := r.Header.Get("User-Agent")
	if ua == "" || strings.HasPrefix(ua, "Go-http-client") || r.Header.Get("trakt-api-key") != ClientID || r.Header.Get("trakt-api-version") != "2" {
		reply(w, http.StatusForbidden, `{"error":"forbidden"}`)
		return
	}

	switch r.URL.Path {
	case "/oauth/device/code":
		s.deviceCode(w, body)
	case "/oauth/device/token":
		s.deviceToken(w, body)
	case "/oauth/token":
		s.refreshToken(w, body)
	case "/oauth/revoke":
		s.revoke(w, body)
	default:
		s.api(w, r)
	}
}

func (s *Server) deviceCode(w http.ResponseWriter, body map[string]string) {
	if body["client_id"] != ClientID {
		reply(w, http.StatusUnauthorized, `{"error":"invalid_client"}`)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deviceUsed = false
	writeJSON(w, http.StatusOK, map[string]any{
		"device_code": DeviceCode, "user_code": UserCode, "verification_url": VerificationURL,
		"expires_in": s.expiresIn, "interval": s.interval,
	})
}

// validApp: the client id, and the secret when one is sent (Trakt calls it
// optional, and stopped issuing secrets to new apps).
func validApp(body map[string]string) bool {
	secret, sent := body["client_secret"]
	return body["client_id"] == ClientID && (!sent || secret == ClientSecret)
}

func (s *Server) deviceToken(w http.ResponseWriter, body map[string]string) {
	if !validApp(body) {
		reply(w, http.StatusUnauthorized, `{"error":"invalid_client"}`)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if body["code"] != DeviceCode {
		reply(w, http.StatusNotFound, "")
		return
	}
	if s.deviceUsed {
		reply(w, http.StatusConflict, "")
		return
	}
	status := s.deviceAnswers[0]
	if len(s.deviceAnswers) > 1 {
		s.deviceAnswers = s.deviceAnswers[1:]
	}
	if status != http.StatusOK {
		reply(w, status, "")
		return
	}
	s.deviceUsed = true
	writeJSON(w, http.StatusOK, s.grantLocked(s.user))
}

func (s *Server) refreshToken(w http.ResponseWriter, body map[string]string) {
	if !validApp(body) {
		reply(w, http.StatusUnauthorized, `{"error":"invalid_client"}`)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	old := body["refresh_token"]
	if body["grant_type"] != "refresh_token" || body["redirect_uri"] != "urn:ietf:wg:oauth:2.0:oob" || !s.refresh[old] {
		reply(w, http.StatusBadRequest, `{"error":"invalid_grant","error_description":"The provided authorization grant is invalid, expired, revoked, or was issued to another client."}`)
		return
	}
	// Single-use: the old pair is dead from here on.
	s.refresh[old] = false
	delete(s.access, s.pairs[old])
	writeJSON(w, http.StatusOK, s.grantLocked(s.owners[old]))
}

func (s *Server) revoke(w http.ResponseWriter, body map[string]string) {
	if !validApp(body) {
		reply(w, http.StatusUnauthorized, `{"error":"invalid_client"}`)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.revoked = append(s.revoked, body["token"])
	delete(s.access, body["token"])
	for refresh, access := range s.pairs {
		if access == body["token"] {
			s.refresh[refresh] = false
		}
	}
	reply(w, http.StatusOK, "{}")
}

// grantLocked issues a new pair to u.
func (s *Server) grantLocked(u user) map[string]any {
	s.seq++
	access, refresh := fmt.Sprintf("access-%d", s.seq), fmt.Sprintf("refresh-%d", s.seq)
	s.access[access] = s.now().Add(s.lifetime)
	s.refresh[refresh] = true
	s.pairs[refresh] = access
	s.owners[access], s.owners[refresh] = u, u
	return map[string]any{
		"access_token": access, "token_type": "bearer", "expires_in": int64(s.lifetime / time.Second),
		"refresh_token": refresh, "scope": "public", "created_at": s.now().Unix(),
	}
}

func (s *Server) api(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	token, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	expires, ok := s.access[token]
	valid := ok && s.now().Before(expires)
	owner := s.owners[token]
	items, isList := s.userLists[owner.slug][r.URL.Path]
	if !isList {
		items, isList = s.lists[r.URL.Path]
	}
	handler := s.handlers[r.URL.Path]
	s.mu.Unlock()

	if !valid {
		reply(w, http.StatusUnauthorized, "")
		return
	}
	switch {
	case handler != nil:
		handler(w, r)
	case isList:
		page(w, r, items)
	case r.URL.Path == "/users/settings":
		writeJSON(w, http.StatusOK, map[string]any{
			"user": map[string]any{
				"username": owner.username, "private": false, "name": owner.username, "vip": false, "vip_ep": false,
				"ids": map[string]any{"slug": owner.slug},
			},
			"account": map[string]any{"timezone": "Europe/Berlin", "time_24hr": true},
		})
	default:
		reply(w, http.StatusNotFound, "")
	}
}

// page answers one page of a list with Trakt's pagination headers.
func page(w http.ResponseWriter, r *http.Request, items []json.RawMessage) {
	n, err := strconv.Atoi(r.URL.Query().Get("page"))
	if err != nil || n < 1 {
		n = 1
	}
	limit, err := strconv.Atoi(r.URL.Query().Get("limit"))
	if err != nil || limit < 1 {
		limit = 10
	}
	limit = min(limit, 250)
	pages := max((len(items)+limit-1)/limit, 1)
	from := min((n-1)*limit, len(items))
	to := min(from+limit, len(items))
	w.Header().Set("X-Pagination-Page", strconv.Itoa(n))
	w.Header().Set("X-Pagination-Limit", strconv.Itoa(limit))
	w.Header().Set("X-Pagination-Page-Count", strconv.Itoa(pages))
	w.Header().Set("X-Pagination-Item-Count", strconv.Itoa(len(items)))
	writeJSON(w, http.StatusOK, items[from:to])
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	reply(w, status, string(b))
}

func reply(w http.ResponseWriter, status int, body string) {
	if body != "" {
		w.Header().Set("Content-Type", "application/json")
	}
	w.WriteHeader(status)
	_, _ = io.WriteString(w, body)
}
