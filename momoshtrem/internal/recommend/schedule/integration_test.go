package schedule

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/shapedtime/momoshtrem/internal/aicore"
	"github.com/shapedtime/momoshtrem/internal/aicore/fake"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/recommend"
	"github.com/shapedtime/momoshtrem/internal/recommend/agent"
	"github.com/shapedtime/momoshtrem/internal/recommend/candidates"
	"github.com/shapedtime/momoshtrem/internal/recommend/profile"
	"github.com/shapedtime/momoshtrem/internal/recommend/runs"
	"github.com/shapedtime/momoshtrem/internal/recommend/tools"
	"github.com/shapedtime/momoshtrem/internal/tmdb"
)

// openDB is a migrated private schema in MOMOSHTREM_TEST_DATABASE_URL.
func openDB(t *testing.T) *library.DB {
	t.Helper()
	url := os.Getenv("MOMOSHTREM_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("MOMOSHTREM_TEST_DATABASE_URL not set; skipping Postgres integration test")
	}
	base, err := library.NewDB(url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { base.Close() })
	schema := fmt.Sprintf("recs_%d", time.Now().UnixNano())
	if _, err := base.Exec(`CREATE SCHEMA ` + schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = base.Exec(`DROP SCHEMA ` + schema + ` CASCADE`) })
	sep := "?"
	if strings.Contains(url, "?") {
		sep = "&"
	}
	db, err := library.NewDB(url + sep + "search_path=" + schema)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

type connected struct{}

func (connected) Connected() bool { return true }

// movieDetails knows movies 1001–1999 as "Movie N".
type movieDetails struct{}

func (movieDetails) MovieDetails(_ context.Context, id int) (*tmdb.MovieDetails, error) {
	if id < 1001 || id > 1999 {
		return nil, tmdb.ErrNotFound
	}
	return &tmdb.MovieDetails{ID: id, Title: fmt.Sprintf("Movie %d", id), ReleaseDate: "2024-05-01", Runtime: 100,
		VoteAverage: 7, VoteCount: 100}, nil
}

func (movieDetails) ShowDetails(context.Context, int) (*tmdb.ShowDetails, error) {
	return nil, tmdb.ErrNotFound
}
func (movieDetails) Search(context.Context, string, string, int) (*tmdb.ResultsPage, error) {
	return &tmdb.ResultsPage{}, nil
}
func (movieDetails) GenreList(context.Context, string) ([]tmdb.Genre, error) { return nil, nil }

type pool struct{}

func (pool) Build(context.Context, *profile.Profile) (*candidates.Pool, error) {
	p := &candidates.Pool{Stats: candidates.Stats{Considered: 40, Kept: 30}}
	for id := 1001; id <= 1030; id++ {
		p.Items = append(p.Items, candidates.Candidate{MediaType: "movie", TMDBID: id, Title: fmt.Sprintf("Movie %d", id), Why: []string{"kw"}})
	}
	return p, nil
}

func shelfInput(slug, reason string, from int) map[string]any {
	items := []map[string]any{}
	for id := from; id < from+7; id++ {
		items = append(items, map[string]any{"media_type": "movie", "tmdb_id": id, "title": fmt.Sprintf("Movie %d", id),
			"reason": fmt.Sprintf("%s: %d", reason, id)})
	}
	return map[string]any{"slug": slug, "kind": "other", "title": "Shelf " + slug, "subtitle": "", "items": items}
}

// runScript is one run on the fake API: four shelves of movies 1001–1028
// with reason's reasons, three hero picks, then the end.
func runScript(reason string) []fake.Response {
	var blocks []fake.Block
	for i, slug := range []string{"a", "b", "c", "d"} {
		blocks = append(blocks, fake.ToolUse("toolu_"+slug, runs.NameSubmitShelf, shelfInput(slug, reason, 1001+7*i)))
	}
	return []fake.Response{
		{Blocks: blocks, Usage: fake.Usage{InputTokens: 8000, CacheCreationInputTokens: 6000, OutputTokens: 1500}},
		{Blocks: []fake.Block{fake.ToolUse("toolu_h", runs.NameSubmitHeroPicks, map[string]any{"picks": []map[string]any{
			{"media_type": "movie", "tmdb_id": 1002, "title": "Movie 1002", "kicker": "Tonight", "reason": "r"},
			{"media_type": "movie", "tmdb_id": 1009, "title": "Movie 1009", "kicker": "Tonight", "reason": "r"},
			{"media_type": "movie", "tmdb_id": 1016, "title": "Movie 1016", "kicker": "Tonight", "reason": "r"},
		}})}, Usage: fake.Usage{CacheReadInputTokens: 14000, InputTokens: 900, OutputTokens: 300}},
		{Blocks: []fake.Block{fake.Text("Saved.")}, Usage: fake.Usage{CacheReadInputTokens: 15000, OutputTokens: 10}},
	}
}

// seedLoved gives the member five loved movies, named prefix One…Five,
// and a play of each of extra.
func seedLoved(t *testing.T, db *library.DB, memberID int64, prefix string, extra ...int) {
	t.Helper()
	for i, name := range []string{"One", "Two", "Three", "Four", "Five"} {
		id := 500 + int(memberID)*10 + i
		if _, err := db.Exec(`INSERT INTO trakt_history (member_id, id, media_type, tmdb_id, trakt_id, title, year, watched_at)
			VALUES ($1, $2, 'movie', $3, $4, $5, 2020, NOW() - make_interval(days => $6))`,
			memberID, memberID*100+int64(i), id, id, prefix+" "+name, 10*(i+1)); err != nil {
			t.Fatal(err)
		}
	}
	for i, id := range extra {
		if _, err := db.Exec(`INSERT INTO trakt_history (member_id, id, media_type, tmdb_id, trakt_id, title, year, watched_at)
			VALUES ($1, $2, 'movie', $3, $4, $5, 2024, NOW() - INTERVAL '1 day')`,
			memberID, memberID*100+50+int64(i), id, id, fmt.Sprintf("Movie %d", id)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO trakt_watched_movies (member_id, tmdb_id, plays, last_watched_at)
		SELECT member_id, tmdb_id, 1, watched_at FROM trakt_history WHERE member_id = $1`, memberID); err != nil {
		t.Fatal(err)
	}
}

// newIntegration is the scheduler over Postgres and the fake API, every
// member's parts bound to them as wire_recs builds them, and every member
// connected.
func newIntegration(t *testing.T, db *library.DB, srv *fake.Server, members Members) (*Scheduler, *library.RecRunRepository) {
	t.Helper()
	settings := library.NewSettingsRepository(db)
	ledger := aicore.NewLedger(library.NewAIStore(db), settings)
	repo := library.NewRecRunRepository(db)
	builder := func(memberID int64) (*profile.Builder, *library.TasteRepository) {
		taste := library.NewTasteRepository(db).ForMember(memberID)
		return profile.New(profile.Deps{Store: taste, Profiles: library.NewTasteProfileRepository(db).ForMember(memberID),
			Settings: library.NewMemberRepository(db).TraktSettingsFor(memberID), Trakt: connected{}}), taste
	}
	runner := runs.NewRunner(runs.Deps{
		AI:     aicore.New(aicore.Config{APIKey: "test-key", Recorder: ledger, Options: srv.Options()}),
		Budget: ledger, Settings: settings,
		Members: func(memberID int64) runs.Member {
			profiles, taste := builder(memberID)
			return runs.Member{Profiles: profiles, Pools: pool{}, Titles: taste, Tools: func(in tools.Inputs) []agent.Tool {
				return tools.All(tools.Deps{Inputs: in, TMDB: movieDetails{}, Titles: taste})
			}}
		},
		TMDB: movieDetails{}, Store: repo,
	})
	s := New(Deps{
		Store: repo, Runner: runner, Settings: settings, Budget: ledger, Configured: true, Members: members,
		History: func(memberID int64) History {
			profiles, _ := builder(memberID)
			return profiles
		},
		Reader: runs.NewReader(runs.ReaderDeps{Store: repo, Settings: settings, Configured: true,
			Titles: func(memberID int64) runs.Titles { return library.NewTasteRepository(db).ForMember(memberID) }}),
	})
	if err := s.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return s, repo
}

// waitDone waits for a run to finish.
func waitDone(t *testing.T, repo *library.RecRunRepository, id int64) {
	t.Helper()
	waitUntil(t, fmt.Sprintf("run %d", id), func() bool {
		run, _ := repo.GetRecRun(context.Background(), id)
		return run != nil && run.Status != recommend.StatusQueued && run.Status != recommend.StatusRunning
	})
}

// A refresh end to end over Postgres and the fake API: the run is queued,
// the worker executes it with R1's profile built from the database, the
// API usage lands in ai_usage and on the run, and /home serves its
// shelves. A title watched since the pool was built is dropped.
func TestRefresh_EndToEnd(t *testing.T) {
	db := openDB(t)
	seedLoved(t, db, 1, "Loved", 1001)
	srv := fake.New(t, runScript("Like the five films you finished")...)
	s, repo := newIntegration(t, db, srv, &fakeMembers{ids: []int64{1}, eligible: map[int64]bool{1: true}})
	ctx := as(1)

	id, err := s.Refresh(ctx)
	if err != nil {
		t.Fatal(err)
	}
	waitDone(t, repo, id)
	detail, err := s.Run(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Status != recommend.StatusSucceeded || detail.Kind != recommend.KindManual || detail.ShelvesDone != 4 ||
		detail.Usage.APICalls != 3 || detail.Usage.InputTokens != 8000+6000+900 || detail.Usage.CacheReadTokens != 29000 ||
		detail.CostUSD <= 0 || detail.CostUSD != detail.Usage.CostUSD {
		t.Fatalf("run = %+v", detail)
	}
	run, _ := repo.GetRecRun(ctx, id)
	if run.ProfileID == nil || run.MemberID != 1 {
		t.Errorf("run = %+v, want member 1's referencing its profile", run)
	}

	home, err := s.Home(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(home.Shelves) != 4 || len(home.HeroPicks) != 3 || home.Run.ID != id || home.ActiveRun != nil {
		t.Fatalf("home = %+v", home)
	}
	// Movie 1001 was watched: dropped from the first shelf.
	if first := home.Shelves[0].Items; len(first) != 6 || first[0].TMDBID != 1002 || first[0].Title != "Movie 1002" {
		t.Errorf("first shelf = %+v", first)
	}
	usage, err := s.RecsUsage(ctx)
	if err != nil || usage.LastRun.ID != id || usage.NextRunAt == nil {
		t.Errorf("usage = %+v, %v", usage, err)
	}
	est, err := s.RunEstimates(ctx)
	if err != nil || est["claude-opus-5-5"] == nil || *est["claude-opus-5-5"] != detail.CostUSD {
		t.Errorf("estimates = %v, %v", est, err)
	}
	reason, err := s.RecommendationReason(ctx, 1, "movie", 1002)
	if err != nil || reason == nil || *reason != "Like the five films you finished: 1002" {
		t.Errorf("reason = %v, %v", reason, err)
	}
}

// Two members end to end (#245): each refresh is that member's run, the
// worker runs them one after the other, each on its own history (member 1
// watched 1001, member 2 didn't), and each member's /home, runs, reason
// and usage are their own; another member's run is not found.
func TestTwoMembers_EndToEnd(t *testing.T) {
	db := openDB(t)
	if sam, err := library.NewMemberRepository(db).Create(context.Background(), "Sam", ""); err != nil || sam.ID != 2 {
		t.Fatalf("Create(Sam) = %+v, %v", sam, err)
	}
	seedLoved(t, db, 1, "Loved", 1001)
	seedLoved(t, db, 2, "Sam's")
	srv := fake.New(t, append(runScript("For member one"), runScript("For Sam")...)...)
	s, repo := newIntegration(t, db, srv, &fakeMembers{ids: []int64{1, 2}, eligible: map[int64]bool{1: true, 2: true}})
	one, sam := as(1), as(2)

	mine, err := s.Refresh(one)
	if err != nil {
		t.Fatal(err)
	}
	theirs, err := s.Refresh(sam)
	if err != nil || theirs == mine {
		t.Fatalf("Sam's refresh = %d, %v; want their own run", theirs, err)
	}
	if again, err := s.Refresh(sam); err != nil || again != theirs {
		t.Errorf("Sam's second refresh = %d, %v; want it to join %d", again, err, theirs)
	}
	waitDone(t, repo, mine)
	waitDone(t, repo, theirs)

	// Each run read its own member's history: Sam's first request has
	// Sam's loved titles and none of member 1's.
	requests := srv.Requests()
	if len(requests) != 6 {
		t.Fatalf("%d API calls, want 3 a run", len(requests))
	}
	if first := requests[0].Body; !bytes.Contains(first, []byte("Loved One")) || bytes.Contains(first, []byte("Sam's One")) {
		t.Error("member 1's run didn't read member 1's history alone")
	}
	if first := requests[3].Body; !bytes.Contains(first, []byte("Sam's One")) || bytes.Contains(first, []byte("Loved One")) {
		t.Error("Sam's run didn't read Sam's history alone")
	}

	for _, tt := range []struct {
		ctx      context.Context
		run      int64
		reason   string
		first    int // the first shelf's first title: 1001 is member 1's watched
		other    int64
		memberID int64
	}{
		{one, mine, "For member one", 1002, theirs, 1},
		{sam, theirs, "For Sam", 1001, mine, 2},
	} {
		home, err := s.Home(tt.ctx)
		if err != nil {
			t.Fatal(err)
		}
		if home.Run == nil || home.Run.ID != tt.run || home.ActiveRun != nil || len(home.Shelves) != 4 || len(home.HeroPicks) != 3 {
			t.Fatalf("member %d's home = %+v", tt.memberID, home)
		}
		if item := home.Shelves[0].Items[0]; item.TMDBID != tt.first || item.Reason != fmt.Sprintf("%s: %d", tt.reason, tt.first) {
			t.Errorf("member %d's first pick = %+v", tt.memberID, item)
		}
		list, err := s.Runs(tt.ctx, 10)
		if err != nil || len(list) != 1 || list[0].ID != tt.run {
			t.Errorf("member %d's runs = %+v, %v", tt.memberID, list, err)
		}
		if _, err := s.Run(tt.ctx, tt.other); !errors.Is(err, recommend.ErrNotFound) {
			t.Errorf("member %d reading the other's run = %v, want recommend.ErrNotFound", tt.memberID, err)
		}
		reason, err := s.RecommendationReason(tt.ctx, tt.memberID, "movie", 1003)
		if err != nil || reason == nil || *reason != tt.reason+": 1003" {
			t.Errorf("member %d's reason = %v, %v", tt.memberID, reason, err)
		}
		usage, err := s.RecsUsage(tt.ctx)
		if err != nil || usage.LastRun == nil || usage.LastRun.ID != tt.run {
			t.Errorf("member %d's usage = %+v, %v", tt.memberID, usage, err)
		}
	}
}
