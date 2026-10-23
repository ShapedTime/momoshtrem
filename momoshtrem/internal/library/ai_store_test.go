package library

import (
	"context"
	"math"
	"testing"
	"time"
)

func TestAIStoreCache(t *testing.T) {
	db := openSchemaDB(t)
	s := NewAIStore(db)
	ctx := context.Background()

	if _, ok, err := s.CacheGet(ctx, "classifier:missing"); ok || err != nil {
		t.Fatalf("missing = %v, %v", ok, err)
	}
	if err := s.CachePut(ctx, AIFeatureClassifier, "classifier:a", []byte(`[{"path":"a.mkv"}]`), time.Hour); err != nil {
		t.Fatal(err)
	}
	payload, ok, err := s.CacheGet(ctx, "classifier:a")
	if err != nil || !ok {
		t.Fatalf("get = %v, %v", ok, err)
	}
	assertJSONEqual(t, payload, `[{"path": "a.mkv"}]`)

	// A put replaces; an expired entry is a miss and is pruned.
	if err := s.CachePut(ctx, AIFeatureClassifier, "classifier:a", []byte(`[]`), 0); err != nil {
		t.Fatal(err)
	}
	if payload, _, _ := s.CacheGet(ctx, "classifier:a"); string(payload) != "[]" {
		t.Errorf("after a second put = %s", payload)
	}
	if err := s.CachePut(ctx, AIFeatureClassifier, "classifier:old", []byte(`[]`), time.Millisecond); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	if _, ok, err := s.CacheGet(ctx, "classifier:old"); ok || err != nil {
		t.Errorf("expired = %v, %v", ok, err)
	}
	if n, err := s.DeleteExpiredCache(ctx); n != 1 || err != nil {
		t.Errorf("pruned %d, %v; want 1", n, err)
	}
	if _, ok, _ := s.CacheGet(ctx, "classifier:a"); !ok {
		t.Error("pruning removed an entry that doesn't expire")
	}
}

func TestAIStoreUsage(t *testing.T) {
	db := openSchemaDB(t)
	s := NewAIStore(db)
	ctx := context.Background()

	// The period's start comes from Go (the server-local month), never
	// from the database's clock or zone.
	since := time.Now().Add(-time.Hour)
	if cost, err := s.CostSince(ctx, since); cost != 0 || err != nil {
		t.Fatalf("empty period = %v, %v", cost, err)
	}
	if totals, err := s.FeatureTotalsSince(ctx, since); len(totals) != 0 || err != nil {
		t.Fatalf("empty period by feature = %v, %v", totals, err)
	}
	ref := int64(42)
	for _, u := range []AIUsage{
		{Feature: AIFeatureClassifier, Model: "claude-haiku-5-5", InputTokens: 1000, OutputTokens: 200, CacheReadTokens: 2000, CostUSD: 0.00022, RefID: &ref},
		{Feature: AIFeatureRecs, Model: "claude-opus-5-5", InputTokens: 10, WebSearches: 3, CostUSD: 0.5},
		{Feature: AIFeatureRecs, Model: "claude-opus-5-5", InputTokens: 10, CostUSD: 0.25},
	} {
		if err := s.RecordUsage(ctx, u); err != nil {
			t.Fatal(err)
		}
	}
	// Usage before the period doesn't count.
	if _, err := db.Exec(`INSERT INTO ai_usage (feature, model, cost_usd, created_at)
		VALUES ('classifier', 'claude-haiku-5-5', 9, $1::timestamptz - INTERVAL '1 second')`, since); err != nil {
		t.Fatal(err)
	}
	cost, err := s.CostSince(ctx, since)
	if err != nil || math.Abs(cost-0.75022) > 1e-9 {
		t.Errorf("cost since = %v, %v; want 0.75022", cost, err)
	}
	totals, err := s.FeatureTotalsSince(ctx, since)
	if err != nil || len(totals) != 2 ||
		totals[0].Feature != AIFeatureClassifier || math.Abs(totals[0].CostUSD-0.00022) > 1e-9 || totals[0].Requests != 1 ||
		totals[1].Feature != AIFeatureRecs || math.Abs(totals[1].CostUSD-0.75) > 1e-9 || totals[1].Requests != 2 {
		t.Errorf("by feature = %+v, %v", totals, err)
	}
	var searches int
	if err := db.QueryRow(`SELECT SUM(web_searches) FROM ai_usage WHERE feature = 'recs'`).Scan(&searches); err != nil || searches != 3 {
		t.Errorf("web searches = %d, %v", searches, err)
	}

	var got AIUsage
	var refID *int64
	if err := db.QueryRow(`SELECT feature, model, input_tokens, output_tokens, cache_read_tokens, ref_id FROM ai_usage WHERE feature = 'classifier' AND ref_id IS NOT NULL`).
		Scan(&got.Feature, &got.Model, &got.InputTokens, &got.OutputTokens, &got.CacheReadTokens, &refID); err != nil {
		t.Fatal(err)
	}
	if got.Model != "claude-haiku-5-5" || got.InputTokens != 1000 || got.OutputTokens != 200 || got.CacheReadTokens != 2000 || refID == nil || *refID != 42 {
		t.Errorf("row = %+v ref %v", got, refID)
	}
}

// The picks and Ask by member: every current member by id, zeros
// included, each call found through its run or turn. The classifier's
// isn't anyone's, nor a call whose run or turn went with a removed member;
// both still count in the totals.
func TestAIStoreMemberSpend(t *testing.T) {
	db := openSchemaDB(t)
	s := NewAIStore(db)
	ctx := context.Background()
	since := time.Now().Add(-time.Hour)
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := db.Exec(query, args...); err != nil {
			t.Fatalf("%.60s…: %v", query, err)
		}
	}

	// Only Me: one row, at zero.
	got, err := s.MemberSpendSince(ctx, since)
	if err != nil || len(got) != 1 || got[0] != (AIMemberTotal{MemberID: 1, Name: "Me"}) {
		t.Fatalf("one member, no spend = %+v, %v", got, err)
	}

	exec(`INSERT INTO members (id, name, color) VALUES (2, 'Sam', 'rose'), (3, 'Ari', 'lime'), (4, 'Gone', 'sand')`)
	// Run 1 is Me's and turn 1 Sam's: the same ref_id, told apart by the
	// feature.
	exec(`INSERT INTO rec_runs (id, member_id, kind, status, model) VALUES (1, 1, 'manual', 'succeeded', 'x'),
		(2, 2, 'scheduled', 'succeeded', 'x'), (3, 4, 'manual', 'succeeded', 'x')`)
	exec(`INSERT INTO ask_threads (id, member_id, title, model, prompt_version) VALUES (1, 2, 'T', 'x', 'ask-v2')`)
	exec(`INSERT INTO ask_turns (id, member_id, thread_id, position, message, status) VALUES (1, 2, 1, 0, 'm', 'done')`)
	ref := func(id int64) *int64 { return &id }
	for _, u := range []AIUsage{
		{Feature: AIFeatureRecs, CostUSD: 0.5, RefID: ref(1)},
		{Feature: AIFeatureRecs, CostUSD: 0.25, RefID: ref(1)},
		{Feature: AIFeatureAsk, CostUSD: 0.02, RefID: ref(1)},
		{Feature: AIFeatureRecs, CostUSD: 0.1, RefID: ref(2)},
		{Feature: AIFeatureClassifier, CostUSD: 0.003, RefID: ref(1)}, // a job, the household's
		{Feature: AIFeatureAsk, CostUSD: 0.04, RefID: ref(99)},        // a turn that's gone
		{Feature: AIFeatureRecs, CostUSD: 0.3, RefID: ref(3)},         // Gone's run
	} {
		u.Model = "claude-opus-5-5"
		if err := s.RecordUsage(ctx, u); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO ai_usage (feature, model, cost_usd, ref_id, created_at)
		VALUES ('recs', 'claude-opus-5-5', 9, 1, $1::timestamptz - INTERVAL '1 second')`, since) // last month
	exec(`DELETE FROM members WHERE id = 4`)

	got, err = s.MemberSpendSince(ctx, since)
	if err != nil || len(got) != 3 {
		t.Fatalf("by member = %+v, %v", got, err)
	}
	for i, want := range []AIMemberTotal{
		{MemberID: 1, Name: "Me", CostUSD: 0.75, Requests: 2},
		{MemberID: 2, Name: "Sam", CostUSD: 0.12, Requests: 2},
		{MemberID: 3, Name: "Ari"},
	} {
		if g := got[i]; g.MemberID != want.MemberID || g.Name != want.Name || math.Abs(g.CostUSD-want.CostUSD) > 1e-9 || g.Requests != want.Requests {
			t.Errorf("by member[%d] = %+v, want %+v", i, g, want)
		}
	}
	if total, err := s.CostSince(ctx, since); err != nil || math.Abs(total-1.213) > 1e-9 {
		t.Errorf("total = %v, %v; want 1.213, the unsplit spend included", total, err)
	}
}
