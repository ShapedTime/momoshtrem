package ask_test

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/shapedtime/momoshtrem/internal/aicore"
	"github.com/shapedtime/momoshtrem/internal/aicore/fake"
	"github.com/shapedtime/momoshtrem/internal/api"
	"github.com/shapedtime/momoshtrem/internal/catalog"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/member"
	"github.com/shapedtime/momoshtrem/internal/recommend/ask"
	"github.com/shapedtime/momoshtrem/internal/recommend/candidates"
	"github.com/shapedtime/momoshtrem/internal/recommend/profile"
	"github.com/shapedtime/momoshtrem/internal/recommend/tools"
	"github.com/shapedtime/momoshtrem/internal/tmdb"
)

// harness is Ask behind the real API routes, on the fake Anthropic API,
// for a household of two: Me (1, the default without a header, whose
// taste is testProfile) and Sam (2, samProfile).
type harness struct {
	t        *testing.T
	api      *fake.Server
	store    ask.Store
	svc      *ask.Service
	members  *member.Service
	server   *httptest.Server
	budget   *budget
	titles   *titles // Me's; titlesOf has Sam's
	recorder *recorder
	upstream *watchTransport

	mu     sync.Mutex
	others map[int64]*titles
	// built counts each member's ForMember calls, and profiles their
	// taste profile's builds.
	built, profiles map[int64]int
}

type harnessOpts struct {
	noKey        bool
	model        string
	writeTimeout time.Duration
}

func newHarness(t *testing.T, store ask.Store, script ...fake.Response) *harness {
	return newHarnessWith(t, store, harnessOpts{}, script...)
}

func newHarnessWith(t *testing.T, store ask.Store, o harnessOpts, script ...fake.Response) *harness {
	t.Helper()
	h := &harness{t: t, api: fake.New(t, script...), store: store, budget: &budget{}, titles: &titles{},
		recorder: &recorder{}, upstream: newWatchTransport(),
		others: map[int64]*titles{}, built: map[int64]int{}, profiles: map[int64]int{}}
	deps := ask.Deps{
		Budget:       h.budget,
		Store:        store,
		Settings:     settings{model: o.model},
		Library:      libraryStates{{MediaType: "tv", TMDBID: 1001}: {LibraryID: 7, AvailabilityStage: catalog.Stage("ready")}},
		TMDB:         testTMDB,
		ForMember:    h.forMember,
		WriteTimeout: o.writeTimeout,
	}
	if !o.noKey {
		opts := append(h.api.Options(), option.WithHTTPClient(&http.Client{Transport: h.upstream}))
		deps.Runner = aicore.New(aicore.Config{APIKey: "test-key", Recorder: h.recorder, Options: opts})
	}
	h.svc = ask.New(deps)
	h.members = member.NewService(newHousehold())
	h.members.OnRemoving(h.svc.ForgetMember)
	h.server = httptest.NewServer(api.NewServer(nil, nil, nil, nil, nil, api.Options{Ask: h.svc, Members: h.members}).Handler())
	t.Cleanup(func() {
		h.svc.Close()
		h.server.Close()
	})
	return h
}

// forMember is the member's parts, as wireAsk builds them: their titles,
// and R1's tools over a tools.Cached of their own.
func (h *harness) forMember(memberID int64) ask.Member {
	h.mu.Lock()
	h.built[memberID]++
	h.mu.Unlock()
	titles := h.titlesOf(memberID)
	inputs := tools.NewCached(func(context.Context) (*profile.Profile, error) {
		h.mu.Lock()
		defer h.mu.Unlock()
		h.profiles[memberID]++
		if memberID == 2 {
			return samProfile, nil
		}
		return testProfile, nil
	}, func(context.Context, *profile.Profile) (*candidates.Pool, error) { return testPool(), nil }, time.Hour)
	return ask.Member{Titles: titles, Tools: tools.All(tools.Deps{Inputs: inputs, TMDB: testTMDB, Titles: titles})}
}

// titlesOf is what the member watched, rated and turned down: h.titles for
// Me.
func (h *harness) titlesOf(memberID int64) *titles {
	if memberID == 1 {
		return h.titles
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.others[memberID] == nil {
		h.others[memberID] = &titles{}
	}
	return h.others[memberID]
}

// counts are a member's ForMember calls and profile builds.
func (h *harness) counts(memberID int64) (built, profiles int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.built[memberID], h.profiles[memberID]
}

// ---- Fixtures

var testProfile = &profile.Profile{
	Counts: profile.Counts{Loved: 4, Completed: 12, Plays: 2184},
	Loved: []profile.Title{
		{MediaType: "tv", TMDBID: 95396, Title: "Severance", Note: "rated 10"},
		{MediaType: "tv", TMDBID: 136315, Title: "The Bear", Note: "rated 9"},
		{MediaType: "tv", TMDBID: 97546, Title: "Ted Lasso", Note: "completed"},
		{MediaType: "movie", TMDBID: 545611, Title: "Everything Everywhere All at Once", Note: "rated 9"},
	},
}

// samProfile is the second member's taste: nothing of Me's.
var samProfile = &profile.Profile{
	Counts: profile.Counts{Loved: 2, Completed: 5, Plays: 311},
	Loved: []profile.Title{
		{MediaType: "tv", TMDBID: 209867, Title: "Frieren: Beyond Journey's End", Note: "rated 10"},
		{MediaType: "tv", TMDBID: 94605, Title: "Arcane", Note: "completed"},
	},
}

// testPool has 23 candidates: shows 1001–1023.
func testPool() *candidates.Pool {
	p := &candidates.Pool{Stats: candidates.Stats{Considered: 29, Excluded: 6, Kept: 23}}
	for i := 1; i <= 23; i++ {
		p.Items = append(p.Items, candidates.Candidate{MediaType: "tv", TMDBID: 1000 + i, Title: "Show", Why: []string{"rec:Severance"}})
	}
	return p
}

var testTMDB = fakeTMDB{
	movies: map[int]*tmdb.MovieDetails{
		2002: {ID: 2002, Title: "The Menu", ReleaseDate: "2022-11-17", Runtime: 107, PosterPath: "/menu.jpg", VoteAverage: 7.2, VoteCount: 4100},
		2003: {ID: 2003, Title: "Watched Film", ReleaseDate: "2020-01-01"},
		2099: {ID: 2099, Title: "Coming Up", ReleaseDate: "2099-01-16"},
	},
	shows: map[int]*tmdb.ShowDetails{
		1001: {ID: 1001, Name: "Mythic Quest", FirstAirDate: "2020-02-07", LastAirDate: "2025-03-26", Status: "Ended",
			PosterPath: "/mq.jpg", EpisodeRunTime: []int{30}, VoteAverage: 7.6, VoteCount: 512,
			Seasons: []tmdb.SeasonSummary{{SeasonNumber: 0}, {SeasonNumber: 1}, {SeasonNumber: 2}, {SeasonNumber: 3}, {SeasonNumber: 4}}},
		1002: {ID: 1002, Name: "Party Down", FirstAirDate: "2009-03-20", Status: "Returning Series",
			Seasons: []tmdb.SeasonSummary{{SeasonNumber: 1}, {SeasonNumber: 2}, {SeasonNumber: 3}}},
	},
}

// ---- The fake API's answers

func toolUse(id, name string, input any) fake.Block { return fake.ToolUse(id, name, input) }

func picksInput(intro string, refinements []string, items ...map[string]any) map[string]any {
	if items == nil {
		items = []map[string]any{}
	}
	if refinements == nil {
		refinements = []string{}
	}
	return map[string]any{"intro": intro, "items": items, "refinements": refinements}
}

// pick names the title as testTMDB has it, as the model would ("Unknown"
// for an id TMDB doesn't know); namedPick names it as given.
func pick(mediaType string, id int, tag, reason string) map[string]any {
	title, year := "Unknown", 0
	if mediaType == "movie" {
		if d, err := testTMDB.MovieDetails(context.Background(), id); err == nil {
			title, year = d.Title, testYear(d.ReleaseDate)
		}
	} else if d, err := testTMDB.ShowDetails(context.Background(), id); err == nil {
		title, year = d.Name, testYear(d.FirstAirDate)
	}
	return namedPick(mediaType, id, title, year, tag, reason)
}

// testYear is a TMDB date's year, 0 without one.
func testYear(date string) int {
	if len(date) < 4 {
		return 0
	}
	y, _ := strconv.Atoi(date[:4])
	return y
}

func namedPick(mediaType string, id int, title string, year int, tag, reason string) map[string]any {
	return map[string]any{"media_type": mediaType, "tmdb_id": id, "title": title, "year": year, "match_tag": tag, "reason": reason}
}

var anyCandidates = map[string]any{"media_type": "any", "genres": []string{}, "min_runtime": 0, "max_runtime": 0,
	"year_from": 0, "year_to": 0, "language": "", "anime": "either", "limit": 0}

// fullTurn is a fresh ask: the profile, a web search, the candidates,
// show_picks, and a closing line.
func fullTurn() []fake.Response {
	return []fake.Response{
		{Blocks: []fake.Block{fake.Thinking("sig_1"), toolUse("toolu_1", "get_taste_profile", map[string]any{})},
			Usage: fake.Usage{InputTokens: 3000, OutputTokens: 120}},
		{Blocks: []fake.Block{
			fake.ServerToolUse("srvtoolu_1", "web_search", map[string]any{"query": "new workplace comedies 2026"}),
			fake.WebSearchResult("srvtoolu_1", "https://example.com/new"),
			toolUse("toolu_2", "get_candidates", anyCandidates),
		}, Usage: fake.Usage{InputTokens: 500, CacheReadInputTokens: 3000, OutputTokens: 80, WebSearchRequests: 1}},
		{Blocks: []fake.Block{fake.Thinking("sig_3"), toolUse("toolu_3", "show_picks", picksInput(
			"Two comedies with Severance's office dread, only warmer.", []string{"Lighter", "Only movies", "Older"},
			pick("tv", 1001, "Workplace satire · lighter", "The same deadpan office absurdity as Severance, half the dread."),
			pick("movie", 2002, "Dark comedy · one night", "A sharp, contained satire like the ones you rated 9."),
		))}, Usage: fake.Usage{InputTokens: 900, CacheReadInputTokens: 3500, OutputTokens: 300}},
		{Blocks: []fake.Block{fake.Text("Enjoy these two.")}, Usage: fake.Usage{InputTokens: 300, CacheReadInputTokens: 4400, OutputTokens: 12}},
	}
}

// ---- The client

type sseEvent struct {
	Name string
	Data string
}

// stream is an open Ask stream.
type stream struct {
	resp   *http.Response
	r      *bufio.Reader
	cancel context.CancelFunc
	pings  int
}

// post sends POST /api/recommendations/ask, as the default member (no
// header); a 200 is an open stream.
func (h *harness) post(body string) (*stream, *http.Response) {
	h.t.Helper()
	return h.postAs(0, body)
}

// postAs posts as a member (0: no header).
func (h *harness) postAs(memberID int64, body string) (*stream, *http.Response) {
	h.t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, "POST", h.server.URL+"/api/recommendations/ask", strings.NewReader(body))
	if err != nil {
		h.t.Fatal(err)
	}
	asMember(req, memberID)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		cancel()
		h.t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		cancel()
		return nil, resp
	}
	s := &stream{resp: resp, r: bufio.NewReader(resp.Body), cancel: cancel}
	h.t.Cleanup(func() {
		cancel()
		resp.Body.Close()
	})
	return s, resp
}

// open posts and fails unless the stream opens.
func (h *harness) open(body string) *stream {
	h.t.Helper()
	return h.openAs(0, body)
}

// openAs opens a stream as a member (0: no header).
func (h *harness) openAs(memberID int64, body string) *stream {
	h.t.Helper()
	s, resp := h.postAs(memberID, body)
	if s == nil {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		h.t.Fatalf("ask = %d %s, want a stream", resp.StatusCode, b)
	}
	return s
}

// asMember names the request's member in the header, as the web's proxy
// does (0: none, so the default member).
func asMember(req *http.Request, memberID int64) {
	if memberID > 0 {
		req.Header.Set(api.MemberHeader, strconv.FormatInt(memberID, 10))
	}
}

// memberCtx is a context for a member, as the API's middleware gives the
// service.
func memberCtx(memberID int64) context.Context { return member.WithID(context.Background(), memberID) }

// next reads the next event, skipping ": ping" comments; ok is false at
// the end of the stream.
func (s *stream) next() (sseEvent, bool) {
	var e sseEvent
	for {
		line, err := s.r.ReadString('\n')
		if err != nil {
			return e, false
		}
		line = strings.TrimRight(line, "\n")
		switch {
		case line == "":
			if e.Name != "" {
				return e, true
			}
		case strings.HasPrefix(line, ":"):
			s.pings++
		case strings.HasPrefix(line, "event: "):
			e.Name = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			e.Data = strings.TrimPrefix(line, "data: ")
		}
	}
}

// all reads every event to the end.
func (s *stream) all() []sseEvent {
	var out []sseEvent
	for {
		e, ok := s.next()
		if !ok {
			return out
		}
		out = append(out, e)
	}
}

// until reads events up to and including the first that matches.
func (s *stream) until(t *testing.T, match func(sseEvent) bool) []sseEvent {
	t.Helper()
	var out []sseEvent
	for {
		e, ok := s.next()
		if !ok {
			t.Fatalf("the stream ended before the event waited for; got %v", out)
		}
		out = append(out, e)
		if match(e) {
			return out
		}
	}
}

func named(name string) func(sseEvent) bool { return func(e sseEvent) bool { return e.Name == name } }

// field reads one field of an event's data.
func field[T any](t *testing.T, e sseEvent, name string) T {
	t.Helper()
	var data map[string]json.RawMessage
	var v T
	if err := json.Unmarshal([]byte(e.Data), &data); err != nil {
		t.Fatalf("%s data %s: %v", e.Name, e.Data, err)
	}
	if err := json.Unmarshal(data[name], &v); err != nil {
		t.Fatalf("%s.%s = %s: %v", e.Name, name, data[name], err)
	}
	return v
}

// ---- Waiting on the store

// turnByID reads a turn from the store.
func turnByID(t *testing.T, store ask.Store, threadID, turnID int64) library.AskTurn {
	t.Helper()
	turns, err := store.Turns(context.Background(), threadID)
	if err != nil {
		t.Fatal(err)
	}
	for _, turn := range turns {
		if turn.ID == turnID {
			return turn
		}
	}
	t.Fatalf("turn %d isn't in thread %d", turnID, threadID)
	return library.AskTurn{}
}

// waitTurn waits until the turn has the status and a view.
func waitTurn(t *testing.T, store ask.Store, threadID, turnID int64, status string) library.AskTurn {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		turn := turnByID(t, store, threadID, turnID)
		if turn.Status == status && len(turn.View) > 0 {
			return turn
		}
		if time.Now().After(deadline) {
			t.Fatalf("turn %d is %s (view %s), want %s", turnID, turn.Status, turn.View, status)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// ---- The upstream connection

// watchTransport reports a request to the API whose context ended while
// its response was still streaming: the call was cut off.
type watchTransport struct {
	mu        sync.Mutex
	cancelled chan struct{}
	fired     bool
}

func newWatchTransport() *watchTransport { return &watchTransport{cancelled: make(chan struct{})} }

func (w *watchTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := http.DefaultTransport.RoundTrip(req)
	if err != nil {
		w.cut(req)
		return nil, err
	}
	body := &watchedBody{ReadCloser: resp.Body, done: make(chan struct{})}
	resp.Body = body
	go func() {
		select {
		case <-req.Context().Done():
		case <-body.done:
		}
		if !body.eof.Load() && req.Context().Err() != nil {
			w.cut(req)
		}
	}()
	return resp, nil
}

func (w *watchTransport) cut(req *http.Request) {
	if req.Context().Err() == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.fired {
		w.fired = true
		close(w.cancelled)
	}
}

// watchedBody is a response body that says when it's read to its end
// (eof) or closed.
type watchedBody struct {
	io.ReadCloser
	eof  atomic.Bool
	once sync.Once
	done chan struct{}
}

func (b *watchedBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if err == io.EOF {
		b.eof.Store(true)
		b.once.Do(func() { close(b.done) })
	}
	return n, err
}

func (b *watchedBody) Close() error {
	b.once.Do(func() { close(b.done) })
	return b.ReadCloser.Close()
}
