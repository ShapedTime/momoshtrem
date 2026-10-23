package library

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/shapedtime/momoshtrem/internal/member"
)

// A run's life in the database (#157): queued once (a second queue joins
// it), started, its progress and usage written as it goes, published with
// its shelves and hero picks in one transaction; the reads /home, the
// title pages and the usage page make.
func TestRecRunRepository(t *testing.T) {
	db := openSchemaDB(t)
	repo := NewRecRunRepository(db)
	ctx := context.Background()

	id, queued, err := repo.QueueRecRun(ctx, 1, "first_sync", "claude-opus-5-5")
	if err != nil || !queued {
		t.Fatalf("Queue = %d, %v, %v", id, queued, err)
	}
	again, queued, err := repo.QueueRecRun(ctx, 1, "manual", "claude-sonnet-5-5")
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
	active, err := repo.ActiveRecRun(ctx, 1)
	if err != nil || active == nil || active.ID != id || active.MemberID != 1 || active.Status != RecRunRunning || active.StartedAt == nil {
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

	latest, err := repo.LatestSucceededRecRun(ctx, 1)
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
		r, err := repo.RecommendationReason(ctx, 1, TitleRef{mt, id})
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
	if err := feedback.Set(ctx, 1, TitleRef{"movie", 2}, "like", "shelf"); err != nil {
		t.Fatal(err)
	}
	if err := feedback.Set(ctx, 1, TitleRef{"tv", 1}, "not_interested", "shelf"); err != nil {
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
	failed, _, err := repo.QueueRecRun(ctx, 1, "scheduled", "claude-sonnet-5-5")
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.StartRecRun(ctx, failed, nil); err != nil {
		t.Fatal(err)
	}
	if err := repo.FailRecRun(ctx, failed, "Claude declined to write the picks.", nil); err != nil {
		t.Fatal(err)
	}
	skipped, _, err := repo.QueueRecRun(ctx, 1, "scheduled", "claude-opus-5-5")
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.SkipRecRun(ctx, skipped, "budget_exceeded"); err != nil {
		t.Fatal(err)
	}
	last, err := repo.LatestFinishedRecRun(ctx, 1)
	if err != nil || last.ID != skipped || *last.SkipReason != "budget_exceeded" || last.StartedAt != nil {
		t.Errorf("latest finished = %+v, %v", last, err)
	}
	if latest, _ := repo.LatestSucceededRecRun(ctx, 1); latest.ID != id {
		t.Errorf("latest succeeded = %d, want %d", latest.ID, id)
	}
	if run, _ := repo.GetRecRun(ctx, failed); run.Status != RecRunFailed || *run.Error != "Claude declined to write the picks." {
		t.Errorf("failed run = %+v", run)
	}
	if active, err := repo.ActiveRecRun(ctx, 1); active != nil || err != nil {
		t.Errorf("active = %+v, %v", active, err)
	}
	list, err := repo.ListRecRuns(ctx, 1, 2)
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

	// Run 1 succeeds; then eight more runs fail. They all finished last
	// month.
	first, _, _ := repo.QueueRecRun(ctx, 1, "manual", "claude-opus-5-5")
	if err := repo.StartRecRun(ctx, first, nil); err != nil {
		t.Fatal(err)
	}
	if err := repo.PublishRecRun(ctx, first, nil, []RecShelfRow{{Slug: "a", Kind: "other", Title: "A",
		Items: []RecItemRow{{TitleRef: TitleRef{"movie", 1}, Reason: "x"}}}}, nil); err != nil {
		t.Fatal(err)
	}
	var ids []int64
	for range 8 {
		id, _, err := repo.QueueRecRun(ctx, 1, "scheduled", "claude-opus-5-5")
		if err != nil {
			t.Fatal(err)
		}
		if err := repo.FailRecRun(ctx, id, "nope", nil); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	running, _, _ := repo.QueueRecRun(ctx, 1, "manual", "claude-opus-5-5")
	if err := repo.StartRecRun(ctx, running, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE rec_runs SET finished_at = finished_at - INTERVAL '40 days'`); err != nil {
		t.Fatal(err)
	}

	n, err := repo.PruneRecRuns(ctx, 3, time.Now().AddDate(0, 0, -1))
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

	// Startup with a key fails the running run only: another member's
	// queued one waits for the worker. Without a key, it's failed too.
	if _, err := NewMemberRepository(db).Create(ctx, "Sam", ""); err != nil {
		t.Fatal(err)
	}
	waiting, _, err := repo.QueueRecRun(ctx, 2, "scheduled", "claude-opus-5-5")
	if err != nil {
		t.Fatal(err)
	}
	failed, err := repo.FailRunningRecRuns(ctx, "momoshtrem restarted during the run.")
	if err != nil || failed != 1 {
		t.Fatalf("failed running = %d, %v", failed, err)
	}
	run, _ := repo.GetRecRun(ctx, running)
	if run.Status != RecRunFailed || *run.Error != "momoshtrem restarted during the run." || run.FinishedAt == nil {
		t.Errorf("run left running = %+v", run)
	}
	if run, _ := repo.GetRecRun(ctx, waiting); run.Status != RecRunQueued {
		t.Errorf("the queued run after FailRunningRecRuns = %+v, want it queued", run)
	}
	if failed, err := repo.FailActiveRecRuns(ctx, "momoshtrem restarted during the run."); err != nil || failed != 1 {
		t.Fatalf("failed active = %d, %v", failed, err)
	}
	if run, _ := repo.GetRecRun(ctx, waiting); run.Status != RecRunFailed {
		t.Errorf("the queued run after FailActiveRecRuns = %+v, want it failed", run)
	}
	if id, queued, err := repo.QueueRecRun(ctx, 1, "manual", "claude-opus-5-5"); err != nil || !queued {
		t.Errorf("queue after = %d, %v, %v", id, queued, err)
	}
}

// Two members' runs (#245): one active each at once, the oldest queued
// next whoever's it is, and each member's reads, reason and feedback
// their own. A member who's gone can't queue.
func TestRecRunRepository_PerMember(t *testing.T) {
	db := openSchemaDB(t)
	repo := NewRecRunRepository(db)
	ctx := context.Background()
	if sam, err := NewMemberRepository(db).Create(ctx, "Sam", ""); err != nil || sam.ID != 2 {
		t.Fatalf("Create(Sam) = %+v, %v", sam, err)
	}

	mine, queued, err := repo.QueueRecRun(ctx, 1, "manual", "claude-opus-5-5")
	if err != nil || !queued {
		t.Fatalf("member 1's queue = %d, %v, %v", mine, queued, err)
	}
	theirs, queued, err := repo.QueueRecRun(ctx, 2, "scheduled", "claude-opus-5-5")
	if err != nil || !queued || theirs == mine {
		t.Fatalf("member 2's queue beside it = %d, %v, %v", theirs, queued, err)
	}
	if again, queued, err := repo.QueueRecRun(ctx, 2, "manual", "claude-sonnet-5-5"); err != nil || queued || again != theirs {
		t.Errorf("member 2's second queue = %d, %v, %v; want it to join %d", again, queued, err, theirs)
	}
	for id, want := range map[int64]int64{1: mine, 2: theirs} {
		if active, err := repo.ActiveRecRun(ctx, id); err != nil || active == nil || active.ID != want || active.MemberID != id {
			t.Errorf("member %d's active = %+v, %v; want %d", id, active, err, want)
		}
	}
	// The oldest queued goes first, whatever the ids say.
	if _, err := db.Exec(`UPDATE rec_runs SET created_at = created_at - INTERVAL '1 minute' WHERE id = $1`, theirs); err != nil {
		t.Fatal(err)
	}
	if next, err := repo.NextQueuedRecRun(ctx); err != nil || next == nil || next.ID != theirs || next.MemberID != 2 {
		t.Errorf("next queued = %+v, %v; want member 2's older run", next, err)
	}
	active, err := repo.ActiveRecRuns(ctx)
	if err != nil || len(active) != 2 || active[0].ID != theirs || active[1].ID != mine {
		t.Errorf("active runs = %+v, %v; want member 2's, then member 1's", active, err)
	}
	if err := repo.StartRecRun(ctx, theirs, nil); err != nil {
		t.Fatal(err)
	}
	if next, err := repo.NextQueuedRecRun(ctx); err != nil || next == nil || next.ID != mine {
		t.Errorf("next queued once member 2's runs = %+v, %v", next, err)
	}

	// Each publishes; each member's reads are their own.
	shelf := func(id int, reason string) []RecShelfRow {
		return []RecShelfRow{{Slug: "a", Kind: "other", Title: "A", Items: []RecItemRow{{TitleRef: TitleRef{"movie", id}, Reason: reason}}}}
	}
	if err := repo.PublishRecRun(ctx, theirs, nil, shelf(20, "Sam's reason"), nil); err != nil {
		t.Fatal(err)
	}
	if err := repo.StartRecRun(ctx, mine, nil); err != nil {
		t.Fatal(err)
	}
	if err := repo.PublishRecRun(ctx, mine, nil, shelf(10, "My reason"), nil); err != nil {
		t.Fatal(err)
	}
	for id, want := range map[int64]int64{1: mine, 2: theirs} {
		if run, err := repo.LatestSucceededRecRun(ctx, id); err != nil || run == nil || run.ID != want {
			t.Errorf("member %d's latest succeeded = %+v, %v; want %d", id, run, err, want)
		}
		if run, err := repo.LatestFinishedRecRun(ctx, id); err != nil || run == nil || run.ID != want {
			t.Errorf("member %d's latest finished = %+v, %v; want %d", id, run, err, want)
		}
		if list, err := repo.ListRecRuns(ctx, id, 10); err != nil || len(list) != 1 || list[0].ID != want {
			t.Errorf("member %d's runs = %+v, %v; want only %d", id, list, err, want)
		}
	}
	if run, err := repo.GetRecRun(ctx, theirs); err != nil || run.MemberID != 2 {
		t.Errorf("GetRecRun = %+v, %v; want member 2's", run, err)
	}
	reason := func(memberID int64, tmdbID int) string {
		t.Helper()
		r, err := repo.RecommendationReason(ctx, memberID, TitleRef{"movie", tmdbID})
		if err != nil {
			t.Fatal(err)
		}
		if r == nil {
			return "<nil>"
		}
		return *r
	}
	if r1, r2 := reason(1, 10), reason(2, 10); r1 != "My reason" || r2 != "<nil>" {
		t.Errorf("movie 10's reason = %q for member 1, %q for member 2; want only member 1's", r1, r2)
	}
	// Feedback is the member's: member 1 turning Sam's pick down hides
	// nothing of Sam's.
	feedback := NewRecFeedbackRepository(db)
	if err := feedback.Set(ctx, 1, TitleRef{"movie", 20}, "dislike", "title"); err != nil {
		t.Fatal(err)
	}
	if r := reason(2, 20); r != "Sam's reason" {
		t.Errorf("member 2's reason after member 1's dislike = %q", r)
	}
	if err := feedback.Set(ctx, 2, TitleRef{"movie", 20}, "seen", "title"); err != nil {
		t.Fatal(err)
	}
	if r := reason(2, 20); r != "<nil>" {
		t.Errorf("member 2's reason after their own seen = %q", r)
	}

	// A member removed meanwhile.
	if _, _, err := repo.QueueRecRun(ctx, 99, "manual", "claude-opus-5-5"); !errors.Is(err, member.ErrUnknown) {
		t.Errorf("queue for no member = %v, want member.ErrUnknown", err)
	}
	if err := feedback.Set(ctx, 99, TitleRef{"movie", 20}, "like", "title"); !errors.Is(err, member.ErrUnknown) {
		t.Errorf("feedback for no member = %v, want member.ErrUnknown", err)
	}
}

// The prune keeps each member's newest runs and their newest succeeded
// one, and every run that finished this budget month; never an active
// run.
func TestRecRunRepository_PrunePerMember(t *testing.T) {
	db := openSchemaDB(t)
	repo := NewRecRunRepository(db)
	ctx := context.Background()
	if _, err := NewMemberRepository(db).Create(ctx, "Sam", ""); err != nil {
		t.Fatal(err)
	}
	monthStart := time.Date(2026, 10, 1, 0, 0, 0, 0, time.Local)
	sept := func(day int) time.Time { return time.Date(2026, 9, day, 3, 30, 0, 0, time.Local) }
	// add stores a run that finished then (zero: it hasn't).
	add := func(memberID int64, status string, finished time.Time) int64 {
		t.Helper()
		var at *time.Time
		if !finished.IsZero() {
			at = &finished
		}
		var id int64
		if err := db.QueryRow(`INSERT INTO rec_runs (member_id, kind, status, model, finished_at) VALUES ($1, 'scheduled', $2, 'x', $3)
			RETURNING id`, memberID, status, at).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}

	mySucceeded := add(1, RecRunSucceeded, sept(1))
	thisMonth := add(1, RecRunFailed, time.Date(2026, 10, 2, 3, 30, 0, 0, time.Local))
	gone1, gone2 := add(1, RecRunFailed, sept(2)), add(1, RecRunFailed, sept(3))
	myNewest := []int64{add(1, RecRunFailed, sept(4)), add(1, RecRunSkipped, sept(5))}
	// Member 2's runs come last: by id alone, they'd be the newest.
	theirActive := add(2, RecRunRunning, time.Time{})
	theirSucceeded := add(2, RecRunSucceeded, sept(6)) // newer than member 1's succeeded
	theirNewest := []int64{add(2, RecRunFailed, sept(7)), add(2, RecRunFailed, sept(8))}

	n, err := repo.PruneRecRuns(ctx, 2, monthStart)
	if err != nil || n != 2 {
		t.Fatalf("pruned = %d, %v; want 2 (%d and %d)", n, err, gone1, gone2)
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
	want := []int64{mySucceeded, thisMonth, myNewest[0], myNewest[1], theirActive, theirSucceeded, theirNewest[0], theirNewest[1]}
	if !reflect.DeepEqual(left, want) {
		t.Errorf("left = %v, want %v", left, want)
	}
}
