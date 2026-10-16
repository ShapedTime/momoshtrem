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
