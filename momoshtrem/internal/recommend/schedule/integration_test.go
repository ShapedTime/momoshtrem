package schedule

import (
	"context"
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

func shelfInput(slug string, from int) map[string]any {
	items := []map[string]any{}
	for id := from; id < from+7; id++ {
		items = append(items, map[string]any{"media_type": "movie", "tmdb_id": id, "title": fmt.Sprintf("Movie %d", id),
			"reason": fmt.Sprintf("Like the five films you finished: %d", id)})
	}
	return map[string]any{"slug": slug, "kind": "other", "title": "Shelf " + slug, "subtitle": "", "items": items}
}

// A refresh end to end over Postgres and the fake API: the run is queued,
// the worker executes it with R1's profile built from the database, the
// API usage lands in ai_usage and on the run, and /home serves its
// shelves. A title watched since the pool was built is dropped.
func TestRefresh_EndToEnd(t *testing.T) {
	db := openDB(t)
	ctx := context.Background()
	for _, q := range []string{
		`INSERT INTO trakt_history (member_id, id, media_type, tmdb_id, trakt_id, title, year, watched_at) VALUES
			(1, 1, 'movie', 501, 1, 'Loved One', 2020, NOW() - INTERVAL '10 days'),
			(1, 2, 'movie', 502, 2, 'Loved Two', 2021, NOW() - INTERVAL '20 days'),
			(1, 3, 'movie', 503, 3, 'Loved Three', 2019, NOW() - INTERVAL '30 days'),
			(1, 4, 'movie', 504, 4, 'Loved Four', 2018, NOW() - INTERVAL '40 days'),
			(1, 5, 'movie', 505, 5, 'Loved Five', 2022, NOW() - INTERVAL '50 days'),
			(1, 6, 'movie', 1001, 6, 'Movie 1001', 2024, NOW() - INTERVAL '1 day')`,
		`INSERT INTO trakt_watched_movies (member_id, tmdb_id, plays, last_watched_at)
			SELECT member_id, tmdb_id, 1, watched_at FROM trakt_history`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("%.50s…: %v", q, err)
		}
	}

	var script []fake.Response
	var blocks []fake.Block
	for i, slug := range []string{"a", "b", "c", "d"} {
		blocks = append(blocks, fake.ToolUse("toolu_"+slug, runs.NameSubmitShelf, shelfInput(slug, 1001+7*i)))
	}
	script = append(script,
		fake.Response{Blocks: blocks, Usage: fake.Usage{InputTokens: 8000, CacheCreationInputTokens: 6000, OutputTokens: 1500}},
		fake.Response{Blocks: []fake.Block{fake.ToolUse("toolu_h", runs.NameSubmitHeroPicks, map[string]any{"picks": []map[string]any{
			{"media_type": "movie", "tmdb_id": 1002, "title": "Movie 1002", "kicker": "Tonight", "reason": "r"},
			{"media_type": "movie", "tmdb_id": 1009, "title": "Movie 1009", "kicker": "Tonight", "reason": "r"},
			{"media_type": "movie", "tmdb_id": 1016, "title": "Movie 1016", "kicker": "Tonight", "reason": "r"},
		}})}, Usage: fake.Usage{CacheReadInputTokens: 14000, InputTokens: 900, OutputTokens: 300}},
		fake.Response{Blocks: []fake.Block{fake.Text("Saved.")}, Usage: fake.Usage{CacheReadInputTokens: 15000, OutputTokens: 10}},
	)
	srv := fake.New(t, script...)

	settings := library.NewSettingsRepository(db)
	aiStore := library.NewAIStore(db)
	ledger := aicore.NewLedger(aiStore, settings)
	taste := library.NewTasteRepository(db).ForMember(1)
	repo := library.NewRecRunRepository(db)
	profiles := profile.New(profile.Deps{Store: taste, Profiles: library.NewTasteProfileRepository(db),
		Settings: library.NewMemberRepository(db).TraktSettingsFor(1), Trakt: connected{}})
	runner := runs.NewRunner(runs.Deps{
		AI:     aicore.New(aicore.Config{APIKey: "test-key", Recorder: ledger, Options: srv.Options()}),
		Budget: ledger, Settings: settings, Profiles: profiles, Pools: pool{},
		Tools: func(in tools.Inputs) []agent.Tool {
			return tools.All(tools.Deps{Inputs: in, TMDB: movieDetails{}, Titles: taste})
		},
		TMDB: movieDetails{}, Titles: taste, Store: repo,
	})
	s := New(Deps{
		Store: repo, Runner: runner, Settings: settings, Budget: ledger, History: profiles, Configured: true,
		Reader: runs.NewReader(runs.ReaderDeps{Store: repo, Settings: settings, Titles: taste, Configured: true}),
	})
	if err := s.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)

	id, err := s.Refresh(ctx)
	if err != nil {
		t.Fatal(err)
	}
	waitUntil(t, "the run", func() bool {
		run, _ := repo.GetRecRun(ctx, id)
		return run != nil && run.Status != recommend.StatusQueued && run.Status != recommend.StatusRunning
	})
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
	if run.ProfileID == nil {
		t.Error("the run doesn't reference its profile")
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
	reason, err := s.RecommendationReason(ctx, "movie", 1002)
	if err != nil || reason == nil || *reason != "Like the five films you finished: 1002" {
		t.Errorf("reason = %v, %v", reason, err)
	}
}
