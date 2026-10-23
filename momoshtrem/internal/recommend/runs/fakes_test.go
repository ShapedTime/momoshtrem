package runs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/shapedtime/momoshtrem/internal/aicore"
	"github.com/shapedtime/momoshtrem/internal/aicore/fake"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/recommend/agent"
	"github.com/shapedtime/momoshtrem/internal/recommend/candidates"
	"github.com/shapedtime/momoshtrem/internal/recommend/profile"
	"github.com/shapedtime/momoshtrem/internal/recommend/tools"
	"github.com/shapedtime/momoshtrem/internal/tmdb"
)

// now is the tests' server-local time: Friday 16 October 2026, 03:00.
var now = time.Date(2026, 10, 16, 3, 0, 0, 0, time.UTC)

// fakeStore records what a run writes.
type fakeStore struct {
	mu        sync.Mutex
	started   bool
	profileID int64
	progress  []Progress
	syncs     int
	published bool
	shelves   []library.RecShelfRow
	picks     []library.RecHeroPickRow
	final     Progress
	failed    string
	skipped   string
	pruned    int
	// prunedSince is the last prune's month start.
	prunedSince time.Time
	// failWrites fails that many final writes; writes counts them.
	failWrites, writes int
}

func (s *fakeStore) StartRecRun(_ context.Context, _ int64, progress json.RawMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.started = true
	s.progress = append(s.progress, DecodeProgress(progress))
	return nil
}

func (s *fakeStore) SetRecRunProfile(_ context.Context, _, profileID int64) error {
	s.profileID = profileID
	return nil
}

func (s *fakeStore) SetRecRunProgress(_ context.Context, _ int64, progress json.RawMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.progress = append(s.progress, DecodeProgress(progress))
	return nil
}

func (s *fakeStore) SyncRecRunUsage(context.Context, int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.syncs++
	return nil
}

// flaky fails a final write while failWrites lasts, and counts the tries.
func (s *fakeStore) flaky(ctx context.Context) error {
	s.writes++
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if s.failWrites > 0 {
		s.failWrites--
		return errors.New("db down")
	}
	return nil
}

func (s *fakeStore) PublishRecRun(ctx context.Context, _ int64, progress json.RawMessage, shelves []library.RecShelfRow, picks []library.RecHeroPickRow) error {
	if err := s.flaky(ctx); err != nil {
		return err
	}
	s.published, s.shelves, s.picks, s.final = true, shelves, picks, DecodeProgress(progress)
	return nil
}

func (s *fakeStore) FailRecRun(ctx context.Context, _ int64, message string, progress json.RawMessage) error {
	if err := s.flaky(ctx); err != nil {
		return err
	}
	s.failed = message
	if progress != nil {
		s.final = DecodeProgress(progress)
	}
	return nil
}

func (s *fakeStore) SkipRecRun(_ context.Context, _ int64, reason string) error {
	s.skipped = reason
	return nil
}

func (s *fakeStore) PruneRecRuns(_ context.Context, _ int, monthStart time.Time) (int64, error) {
	s.pruned++
	s.prunedSince = monthStart
	return 0, nil
}

type fakeSettings struct{ s library.AISettings }

func (f fakeSettings) AISettings(context.Context) (library.AISettings, error) { return f.s, nil }

type fakeBudget struct{ err error }

func (f fakeBudget) Check(context.Context) error { return f.err }

// fakeProfiles answers a small profile; Save stores it as profile 7.
type fakeProfiles struct {
	ready profile.Readiness
	saved int
}

func (f *fakeProfiles) CheckHistory(context.Context) (profile.Readiness, error) { return f.ready, nil }

func (f *fakeProfiles) Build(context.Context) (*profile.Profile, error) {
	return &profile.Profile{
		Counts: profile.Counts{Loved: 7, Completed: 5, Plays: 2184, Rated: 312},
		Genres: []profile.Weighted{{ID: 18, Name: "Drama", W: 0.6}},
		Loved:  []profile.Title{{MediaType: "tv", TMDBID: 95396, Title: "Severance", Year: 2022, Note: "rated 10"}},
	}, nil
}

func (f *fakeProfiles) Save(_ context.Context, p *profile.Profile) (*profile.Stored, error) {
	f.saved++
	return &profile.Stored{ID: 7, Profile: p}, nil
}

// fakePools answers a pool of n movies, 1001 onwards.
type fakePools struct{ n int }

func (f fakePools) Build(context.Context, *profile.Profile) (*candidates.Pool, error) {
	pool := &candidates.Pool{Stats: candidates.Stats{Considered: f.n + 23, Excluded: 23, Kept: f.n}}
	for i := range f.n {
		id := 1001 + i
		pool.Items = append(pool.Items, candidates.Candidate{MediaType: "movie", TMDBID: id, Title: movieTitle(id),
			Why: []string{"rec:Severance (2022)"}})
	}
	return pool, nil
}

func movieTitle(id int) string { return fmt.Sprintf("Movie %d", id) }

// fakeTitles answers TitleStates: watched marks a movie watched, feedback
// gives a signal.
type fakeTitles struct {
	watched  map[int]bool
	feedback map[library.TitleRef]string
	calls    int
}

func (f *fakeTitles) TitleStates(_ context.Context, refs []library.TitleRef) ([]library.TitleState, error) {
	f.calls++
	out := make([]library.TitleState, len(refs))
	for i, ref := range refs {
		out[i] = library.TitleState{TitleRef: ref, Feedback: f.feedback[ref]}
		if f.watched[ref.TMDBID] {
			out[i].Plays, out[i].Started = 1, true
		}
	}
	return out, nil
}

// fakeTMDB knows movies 1001–1999 ("Movie N", released 2024) and show 2001
// ("Show 2001", returning, its next episode in November).
type fakeTMDB struct{}

func (fakeTMDB) MovieDetails(_ context.Context, id int) (*tmdb.MovieDetails, error) {
	if id < 1001 || id > 1999 {
		return nil, tmdb.ErrNotFound
	}
	release := "2024-05-01"
	if id == 1999 {
		release = "2027-01-16"
	}
	return &tmdb.MovieDetails{ID: id, Title: movieTitle(id), OriginalTitle: movieTitle(id), ReleaseDate: release,
		Runtime: 101, PosterPath: "/p.jpg", VoteAverage: 7.4, VoteCount: 900}, nil
}

func (fakeTMDB) ShowDetails(_ context.Context, id int) (*tmdb.ShowDetails, error) {
	if id != 2001 {
		return nil, tmdb.ErrNotFound
	}
	return &tmdb.ShowDetails{ID: id, Name: "Show 2001", OriginalName: "Show 2001", FirstAirDate: "2022-02-18",
		Status: "Returning Series", EpisodeRunTime: []int{50}, Seasons: []tmdb.SeasonSummary{{SeasonNumber: 0}, {SeasonNumber: 1}, {SeasonNumber: 2}},
		NextEpisodeToAir: &tmdb.EpisodeRef{AirDate: "2026-11-20"}}, nil
}

func (fakeTMDB) Search(context.Context, string, string, int) (*tmdb.ResultsPage, error) {
	return &tmdb.ResultsPage{}, nil
}

func (fakeTMDB) GenreList(context.Context, string) ([]tmdb.Genre, error) { return nil, nil }

// harness is a Runner over fakes and the fake Anthropic API. Its member's
// parts are the fakes; members records whose parts were asked for.
type harness struct {
	srv      *fake.Server
	store    *fakeStore
	titles   *fakeTitles
	profiles *fakeProfiles
	runner   *Runner
	members  []int64
}

func newHarness(t *testing.T, script ...fake.Response) *harness {
	t.Helper()
	h := &harness{
		srv:      fake.New(t, script...),
		store:    &fakeStore{},
		titles:   &fakeTitles{watched: map[int]bool{1001: true}},
		profiles: &fakeProfiles{ready: profile.Readiness{Connected: true, Counts: profile.Counts{Loved: 7}}},
	}
	settings := library.DefaultAISettings()
	h.runner = NewRunner(Deps{
		AI:       aicore.New(aicore.Config{APIKey: "test-key", Options: h.srv.Options()}),
		Budget:   fakeBudget{},
		Settings: fakeSettings{settings},
		Members:  h.member,
		TMDB:     fakeTMDB{},
		Store:    h.store,
		Now:      func() time.Time { return now },
	})
	return h
}

func (h *harness) member(id int64) Member {
	h.members = append(h.members, id)
	return Member{
		Profiles: h.profiles,
		Pools:    fakePools{n: 60},
		Titles:   h.titles,
		Tools: func(in tools.Inputs) []agent.Tool {
			return tools.All(tools.Deps{Inputs: in, TMDB: fakeTMDB{}, Titles: h.titles})
		},
	}
}

// execute runs member 2's run 42.
func (h *harness) execute(t *testing.T) Outcome {
	t.Helper()
	out, err := h.runner.Execute(context.Background(),
		library.RecRun{ID: 42, MemberID: 2, Kind: "manual", Status: "queued", Model: "claude-opus-5-5"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	return out
}

// item is a submitted title.
func item(id int, reason string) map[string]any {
	return map[string]any{"media_type": "movie", "tmdb_id": id, "title": movieTitle(id), "year": 2024, "reason": reason}
}

// shelfInput is a submit_shelf input of movies from..from+n-1.
func shelfOf(slug, kind string, from, n int) map[string]any {
	items := []map[string]any{}
	for id := from; id < from+n; id++ {
		items = append(items, item(id, fmt.Sprintf("Because you loved Severance: %d", id)))
	}
	return map[string]any{"slug": slug, "kind": kind, "title": "Shelf " + slug, "subtitle": "", "items": items}
}

func heroOf(ids ...int) map[string]any {
	picks := []map[string]any{}
	for _, id := range ids {
		picks = append(picks, map[string]any{"media_type": "movie", "tmdb_id": id, "title": movieTitle(id), "year": 2024,
			"kicker": "Tonight's pick", "reason": "A quiet, strange workplace story like Severance."})
	}
	return map[string]any{"picks": picks}
}
