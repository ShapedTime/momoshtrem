package library

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"
)

// A run's life in the database (#157): queued once (a second queue joins
// it), started, its progress and usage written as it goes, published with
// its shelves and hero picks in one transaction; the reads /home, the
// title pages and the usage page make.
func TestRecRunRepository(t *testing.T) {
	db := openSchemaDB(t)
	repo := NewRecRunRepository(db)
	ctx := context.Background()

	id, queued, err := repo.QueueRecRun(ctx, "first_sync", "claude-opus-5-5")
	if err != nil || !queued {
		t.Fatalf("Queue = %d, %v, %v", id, queued, err)
	}
	again, queued, err := repo.QueueRecRun(ctx, "manual", "claude-sonnet-5-5")
	if err != nil || queued || again != id {
		t.Fatalf("a second queue = %d, %v, %v; want it to join %d", again, queued, err, id)
	}
	if err := repo.SkipRecRun(ctx, 999, "no_history"); !errors.Is(err, ErrRecRunNotActive) {
		t.Errorf("skip an unknown run = %v", err)
	}

	if err := repo.StartRecRun(ctx, id, json.RawMessage(`{"steps":[{"step":"history","state":"active","detail":null}],"shelves_done":0,"shelves_target":5}`)); err != nil {
		t.Fatal(err)
	}
	if err := repo.StartRecRun(ctx, id, json.RawMessage(`{}`)); !errors.Is(err, ErrRecRunNotActive) {
		t.Errorf("start twice = %v", err)
	}
	if err := repo.SetRecRunProgress(ctx, id, json.RawMessage(`{"steps":[],"shelves_done":2,"shelves_target":5}`)); err != nil {
		t.Fatal(err)
	}
	active, err := repo.ActiveRecRun(ctx)
	if err != nil || active == nil || active.ID != id || active.Status != RecRunRunning || active.StartedAt == nil {
		t.Fatalf("active = %+v, %v", active, err)
	}

	// Usage: the run's ai_usage rows, summed; other runs' and features'
	// aren't counted.
	store := NewAIStore(db)
	ref, other := id, id+100
	for _, u := range []AIUsage{
		{Feature: AIFeatureRecs, Model: "claude-opus-5-5", InputTokens: 1000, OutputTokens: 200, CacheReadTokens: 0, WebSearches: 1, CostUSD: 0.05, RefID: &ref},
		{Feature: AIFeatureRecs, Model: "claude-opus-5-5", InputTokens: 300, OutputTokens: 400, CacheReadTokens: 9000, CostUSD: 0.0123, RefID: &ref},
		{Feature: AIFeatureRecs, Model: "claude-opus-5-5", InputTokens: 7, CostUSD: 1, RefID: &other},
		{Feature: AIFeatureAsk, Model: "claude-opus-5-5", InputTokens: 7, CostUSD: 1, RefID: &ref},
	} {
		if err := store.RecordUsage(ctx, u); err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.SyncRecRunUsage(ctx, id); err != nil {
		t.Fatal(err)
	}
	got, err := repo.GetRecRun(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.APICalls != 2 || got.InputTokens != 1300 || got.OutputTokens != 600 || got.CacheReadTokens != 9000 ||
		got.WebSearches != 1 || got.CostUSD != 0.0623 {
		t.Errorf("usage = %+v", got)
	}

	sub := "Workplace mysteries"
	shelves := []RecShelfRow{
		{Slug: "because-severance", Kind: "because", Title: "Because you loved Severance", Subtitle: &sub, Items: []RecItemRow{
			{TitleRef: TitleRef{"tv", 1}, Reason: "Shelf reason for 1", Meta: json.RawMessage(`{"title":"One"}`), Signals: json.RawMessage(`{"pool":true}`)},
			{TitleRef: TitleRef{"movie", 2}, Reason: "Shelf reason for 2", Meta: json.RawMessage(`{"title":"Two"}`)},
		}},
		{Slug: "gems", Kind: "gems", Title: "Hidden gems", Items: []RecItemRow{
			{TitleRef: TitleRef{"movie", 3}, Reason: "Gem", Meta: json.RawMessage(`{"title":"Three"}`)},
			{TitleRef: TitleRef{"tv", 1}, Reason: "Second shelf's reason for 1", Meta: json.RawMessage(`{"title":"One"}`)},
		}},
	}
	picks := []RecHeroPickRow{
		{TitleRef: TitleRef{"movie", 4}, Kicker: "Tonight", Reason: "Hero reason for 4", Meta: json.RawMessage(`{"title":"Four"}`)},
		{TitleRef: TitleRef{"tv", 1}, Kicker: "Also", Reason: "Hero reason for 1"},
	}
	done := json.RawMessage(`{"steps":[{"step":"write","state":"done","detail":"2 of 5 shelves"}],"shelves_done":2,"shelves_target":5}`)
	if err := repo.PublishRecRun(ctx, id, done, shelves, picks); err != nil {
		t.Fatal(err)
	}
	if err := repo.PublishRecRun(ctx, id, done, nil, nil); !errors.Is(err, ErrRecRunNotActive) {
		t.Errorf("publish twice = %v", err)
	}

	latest, err := repo.LatestSucceededRecRun(ctx)
	if err != nil || latest == nil || latest.ID != id || latest.FinishedAt == nil || latest.Error != nil {
		t.Fatalf("latest succeeded = %+v, %v", latest, err)
	}
	var progress map[string]any
	if err := json.Unmarshal(latest.Progress, &progress); err != nil || progress["shelves_done"] != 2.0 {
		t.Errorf("progress = %s", latest.Progress)
	}
	readShelves, err := repo.RecRunShelves(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	shelves[0].Items[1].Signals = nil // not read back
	shelves[0].Items[0].Signals = nil
	if len(readShelves) != 2 || readShelves[0].Slug != "because-severance" || *readShelves[0].Subtitle != sub ||
		len(readShelves[0].Items) != 2 || readShelves[0].Items[1].TMDBID != 2 || readShelves[1].Subtitle != nil ||
		readShelves[1].Items[0].Reason != "Gem" {
		t.Errorf("shelves = %+v", readShelves)
	}
	readPicks, err := repo.RecRunHeroPicks(ctx, id)
	if err != nil || len(readPicks) != 2 || readPicks[0].TMDBID != 4 || string(readPicks[1].Meta) != "{}" {
		t.Errorf("hero picks = %+v, %v", readPicks, err)
	}
	var signals string
	if err := db.QueryRow(`SELECT signals::text FROM rec_items WHERE tmdb_id = 1 AND reason = 'Shelf reason for 1'`).Scan(&signals); err != nil || signals != `{"pool": true}` {
		t.Errorf("signals = %s, %v", signals, err)
	}

	// The title page's reason: the first shelf's, else a hero pick's; none
	// after negative feedback, or for a title the run didn't pick.
	reason := func(mt string, id int) string {
		t.Helper()
		r, err := repo.RecommendationReason(ctx, TitleRef{mt, id})
		if err != nil {
			t.Fatal(err)
		}
		if r == nil {
			return "<nil>"
		}
		return *r
	}
	if r := reason("tv", 1); r != "Shelf reason for 1" {
		t.Errorf("reason tv 1 = %q", r)
	}
	if r := reason("movie", 4); r != "Hero reason for 4" {
		t.Errorf("reason movie 4 = %q", r)
	}
	if r := reason("movie", 1); r != "<nil>" {
		t.Errorf("reason movie 1 = %q", r)
	}
	feedback := NewRecFeedbackRepository(db)
	if err := feedback.Set(ctx, TitleRef{"movie", 2}, "like", "shelf"); err != nil {
		t.Fatal(err)
	}
	if err := feedback.Set(ctx, TitleRef{"tv", 1}, "not_interested", "shelf"); err != nil {
		t.Fatal(err)
	}
	if r := reason("movie", 2); r != "Shelf reason for 2" {
		t.Errorf("a liked title's reason = %q", r)
	}
	if r := reason("tv", 1); r != "<nil>" {
		t.Errorf("after not_interested = %q", r)
	}

	// The next run fails, then one is skipped: the newest finished is the
	// skipped one, the newest succeeded still the first.
	failed, _, err := repo.QueueRecRun(ctx, "scheduled", "claude-sonnet-5-5")
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.StartRecRun(ctx, failed, nil); err != nil {
		t.Fatal(err)
	}
	if err := repo.FailRecRun(ctx, failed, "Claude declined to write the picks.", nil); err != nil {
		t.Fatal(err)
	}
	skipped, _, err := repo.QueueRecRun(ctx, "scheduled", "claude-opus-5-5")
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.SkipRecRun(ctx, skipped, "budget_exceeded"); err != nil {
		t.Fatal(err)
	}
	last, err := repo.LatestFinishedRecRun(ctx)
	if err != nil || last.ID != skipped || *last.SkipReason != "budget_exceeded" || last.StartedAt != nil {
		t.Errorf("latest finished = %+v, %v", last, err)
	}
	if latest, _ := repo.LatestSucceededRecRun(ctx); latest.ID != id {
		t.Errorf("latest succeeded = %d, want %d", latest.ID, id)
	}
	if run, _ := repo.GetRecRun(ctx, failed); run.Status != RecRunFailed || *run.Error != "Claude declined to write the picks." {
		t.Errorf("failed run = %+v", run)
	}
	if active, err := repo.ActiveRecRun(ctx); active != nil || err != nil {
		t.Errorf("active = %+v, %v", active, err)
	}
	list, err := repo.ListRecRuns(ctx, 2)
	if err != nil || len(list) != 2 || list[0].ID != skipped || list[1].ID != failed {
		t.Errorf("list = %+v, %v", list, err)
	}
	created, ok, err := repo.LatestRecRunCreatedAt(ctx)
	if err != nil || !ok || time.Since(created) > time.Minute {
		t.Errorf("latest created = %v, %v, %v", created, ok, err)
	}

	means, err := repo.RecRunCostMeans(ctx, 5)
	if err != nil || !reflect.DeepEqual(means, map[string]float64{"claude-opus-5-5": 0.0623}) {
		t.Errorf("cost means = %v, %v", means, err)
	}
}

// Startup fails what a crash left active; pruning keeps the newest runs and
// the newest succeeded one, whatever its age.
func TestRecRunRepository_FailActiveAndPrune(t *testing.T) {
	db := openSchemaDB(t)
	repo := NewRecRunRepository(db)
	ctx := context.Background()

	if created, ok, err := repo.LatestRecRunCreatedAt(ctx); ok || err != nil {
		t.Errorf("no runs: latest created = %v, %v, %v", created, ok, err)
	}

	// Run 1 succeeds; then eight more runs fail.
	first, _, _ := repo.QueueRecRun(ctx, "manual", "claude-opus-5-5")
	if err := repo.StartRecRun(ctx, first, nil); err != nil {
		t.Fatal(err)
	}
	if err := repo.PublishRecRun(ctx, first, nil, []RecShelfRow{{Slug: "a", Kind: "other", Title: "A",
		Items: []RecItemRow{{TitleRef: TitleRef{"movie", 1}, Reason: "x"}}}}, nil); err != nil {
		t.Fatal(err)
	}
	var ids []int64
	for range 8 {
		id, _, err := repo.QueueRecRun(ctx, "scheduled", "claude-opus-5-5")
		if err != nil {
			t.Fatal(err)
		}
		if err := repo.FailRecRun(ctx, id, "nope", nil); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	running, _, _ := repo.QueueRecRun(ctx, "manual", "claude-opus-5-5")
	if err := repo.StartRecRun(ctx, running, nil); err != nil {
		t.Fatal(err)
	}

	n, err := repo.PruneRecRuns(ctx, 3)
	if err != nil || n != 6 {
		t.Fatalf("pruned = %d, %v; want 6 (the newest 3 and the succeeded one stay)", n, err)
	}
	var left []int64
	rows, err := db.Query(`SELECT id FROM rec_runs ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var id int64
		_ = rows.Scan(&id)
		left = append(left, id)
	}
	rows.Close()
	if want := []int64{first, ids[6], ids[7], running}; !reflect.DeepEqual(left, want) {
		t.Errorf("left = %v, want %v", left, want)
	}
	var shelves int
	if err := db.QueryRow(`SELECT COUNT(*) FROM rec_shelves`).Scan(&shelves); err != nil || shelves != 1 {
		t.Errorf("shelves = %d, %v", shelves, err)
	}

	failed, err := repo.FailActiveRecRuns(ctx, "momoshtrem restarted during the run.")
	if err != nil || failed != 1 {
		t.Fatalf("failed active = %d, %v", failed, err)
	}
	run, _ := repo.GetRecRun(ctx, running)
	if run.Status != RecRunFailed || *run.Error != "momoshtrem restarted during the run." || run.FinishedAt == nil {
		t.Errorf("run left running = %+v", run)
	}
	if id, queued, err := repo.QueueRecRun(ctx, "manual", "claude-opus-5-5"); err != nil || !queued {
		t.Errorf("queue after = %d, %v, %v", id, queued, err)
	}
}
