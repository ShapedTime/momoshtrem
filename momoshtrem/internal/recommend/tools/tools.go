// Package tools is the picks agent's custom tools (PLAN §7 R1, #153):
// get_taste_profile, get_candidates, tmdb_search, tmdb_details and
// check_seen, each an agent.Tool with a strict schema and compact JSON
// results. Runs (R2, #157) and Ask (R3, #158) both use them: a run builds
// its profile and pool before it starts and passes them as Fixed; Ask
// passes a Cached that builds them on first use.
package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/recommend/agent"
	"github.com/shapedtime/momoshtrem/internal/recommend/candidates"
	"github.com/shapedtime/momoshtrem/internal/recommend/profile"
	"github.com/shapedtime/momoshtrem/internal/tmdb"
)

// Inputs are the taste profile and candidate pool the tools read.
type Inputs interface {
	Profile(ctx context.Context) (*profile.Profile, error)
	Pool(ctx context.Context) (*candidates.Pool, error)
}

// TMDB is what tmdb_search and tmdb_details ask (tmdb.Cached).
type TMDB interface {
	Search(ctx context.Context, mediaType, query string, year int) (*tmdb.ResultsPage, error)
	MovieDetails(ctx context.Context, id int) (*tmdb.MovieDetails, error)
	ShowDetails(ctx context.Context, id int) (*tmdb.ShowDetails, error)
	GenreList(ctx context.Context, mediaType string) ([]tmdb.Genre, error)
}

// Titles looks up what's known about titles (library.TasteRepository).
type Titles interface {
	TitleStates(ctx context.Context, refs []library.TitleRef) ([]library.TitleState, error)
}

// Deps are what the tools read.
type Deps struct {
	Inputs Inputs
	TMDB   TMDB
	Titles Titles
}

// All returns the five tools, always in this order (their definitions sit
// in the cached prompt prefix).
func All(d Deps) []agent.Tool {
	return []agent.Tool{
		NewGetTasteProfile(d.Inputs),
		NewGetCandidates(d.Inputs, d.Titles),
		NewTMDBSearch(d.TMDB, d.Titles),
		NewTMDBDetails(d.TMDB, d.Titles),
		NewCheckSeen(d.Titles),
	}
}

// Fixed are a run's inputs, built before it starts.
type Fixed struct {
	P *profile.Profile
	C *candidates.Pool
}

// Profile returns the profile.
func (f Fixed) Profile(context.Context) (*profile.Profile, error) { return f.P, nil }

// Pool returns the pool.
func (f Fixed) Pool(context.Context) (*candidates.Pool, error) { return f.C, nil }

// Cached builds the inputs on first use and again once they're older than
// its TTL (Ask, between runs). A build is shared by concurrent callers.
type Cached struct {
	buildProfile func(context.Context) (*profile.Profile, error)
	buildPool    func(context.Context, *profile.Profile) (*candidates.Pool, error)
	ttl          time.Duration
	now          func() time.Time

	mu        sync.Mutex
	profile   *profile.Profile
	profileAt time.Time
	pool      *candidates.Pool
	poolAt    time.Time
}

// NewCached creates a Cached: buildProfile is, say, profile.Builder's
// Current; buildPool candidates.Builder's Build.
func NewCached(buildProfile func(context.Context) (*profile.Profile, error),
	buildPool func(context.Context, *profile.Profile) (*candidates.Pool, error), ttl time.Duration) *Cached {
	return &Cached{buildProfile: buildProfile, buildPool: buildPool, ttl: ttl, now: time.Now}
}

// Profile returns the profile, building it when it's missing or stale.
func (c *Cached) Profile(ctx context.Context) (*profile.Profile, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.profileLocked(ctx)
}

func (c *Cached) profileLocked(ctx context.Context) (*profile.Profile, error) {
	if c.profile != nil && c.now().Sub(c.profileAt) < c.ttl {
		return c.profile, nil
	}
	p, err := c.buildProfile(ctx)
	if err != nil {
		return nil, err
	}
	c.profile, c.profileAt = p, c.now()
	return p, nil
}

// Pool returns the pool, building it (and the profile it needs) when it's
// missing or stale.
func (c *Cached) Pool(ctx context.Context) (*candidates.Pool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.pool != nil && c.now().Sub(c.poolAt) < c.ttl {
		return c.pool, nil
	}
	p, err := c.profileLocked(ctx)
	if err != nil {
		return nil, err
	}
	pool, err := c.buildPool(ctx, p)
	if err != nil {
		return nil, err
	}
	c.pool, c.poolAt = pool, c.now()
	return pool, nil
}

// Invalidate drops what's cached, so the next call builds anew.
func (c *Cached) Invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.profile, c.pool = nil, nil
}

// tool is the shape every tool shares: a fixed name, description and
// schema, and a run function.
type tool struct {
	name, description string
	schema            json.RawMessage
	run               func(ctx context.Context, input json.RawMessage) (any, error)
}

func (t *tool) Name() string            { return t.name }
func (t *tool) Description() string     { return t.description }
func (t *tool) Schema() json.RawMessage { return t.schema }

// Run executes the tool and encodes its result compactly.
func (t *tool) Run(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	out, err := t.run(ctx, input)
	if err != nil {
		return nil, err
	}
	b, err := json.Marshal(out)
	if err != nil {
		return nil, fmt.Errorf("%s: failed to encode the result: %w", t.name, err)
	}
	return b, nil
}

// compact turns a schema written for people into the bytes sent, once.
func compact(schema string) json.RawMessage {
	var buf bytes.Buffer
	if err := json.Compact(&buf, []byte(schema)); err != nil {
		panic(fmt.Sprintf("invalid tool schema: %v", err))
	}
	return buf.Bytes()
}

// decode reads a tool's input. Strict tool use makes it match the schema,
// so a failure is the model's to see.
func decode(input json.RawMessage, v any) error {
	if len(bytes.TrimSpace(input)) == 0 {
		input = json.RawMessage(`{}`)
	}
	dec := json.NewDecoder(bytes.NewReader(input))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return &agent.ToolError{Message: "The input doesn't match the tool's schema: " + err.Error()}
	}
	return nil
}

// tmdbError is a TMDB failure the model can work around; a cancelled
// context ends the run instead.
func tmdbError(err error, what string) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	if errors.Is(err, tmdb.ErrNotFound) {
		return &agent.ToolError{Message: "TMDB has no " + what + "."}
	}
	return &agent.ToolError{Message: "TMDB didn't answer for " + what + "; go on without it."}
}

func validMediaType(mt string) bool { return mt == tmdb.MediaMovie || mt == tmdb.MediaTV }

// Mark is what's known about a title, as every tool's results mark it.
type Mark struct {
	// Excluded says why it can't be picked (watched, started, dropped on
	// Trakt, hidden on Trakt, disliked, not interested, marked seen).
	Excluded    string `json:"excluded,omitempty"`
	Rating      int    `json:"your_rating,omitempty"` // your Trakt rating
	Liked       bool   `json:"liked,omitempty"`
	InLibrary   bool   `json:"in_library,omitempty"`
	OnWatchlist bool   `json:"watchlist,omitempty"`
}

func markOf(s library.TitleState) Mark {
	st := Mark{Excluded: candidates.Exclusion(s), Liked: s.Feedback == "like", InLibrary: s.InLibrary, OnWatchlist: s.OnWatchlist}
	if s.Rating != nil {
		st.Rating = *s.Rating
	}
	return st
}

// states looks up refs' statuses by title.
func states(ctx context.Context, titles Titles, refs []library.TitleRef) (map[library.TitleRef]library.TitleState, error) {
	out := map[library.TitleRef]library.TitleState{}
	if len(refs) == 0 {
		return out, nil
	}
	list, err := titles.TitleStates(ctx, refs)
	if err != nil {
		return nil, err
	}
	for _, s := range list {
		out[s.TitleRef] = s
	}
	return out, nil
}

// clip cuts s to n characters.
func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
