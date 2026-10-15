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

	if cost, err := s.MonthToDateCost(ctx); cost != 0 || err != nil {
		t.Fatalf("empty month = %v, %v", cost, err)
	}
	ref := int64(42)
	for _, u := range []AIUsage{
		{Feature: AIFeatureClassifier, Model: "claude-haiku-5-5", InputTokens: 1000, OutputTokens: 200, CacheReadTokens: 2000, CostUSD: 0.00022, RefID: &ref},
		{Feature: AIFeatureRecs, Model: "claude-opus-5-5", InputTokens: 10, CostUSD: 0.5},
	} {
		if err := s.RecordUsage(ctx, u); err != nil {
			t.Fatal(err)
		}
	}
	// Last month's usage doesn't count.
	if _, err := db.Exec(`INSERT INTO ai_usage (feature, model, cost_usd, created_at)
		VALUES ('classifier', 'claude-haiku-5-5', 9, date_trunc('month', NOW()) - INTERVAL '1 day')`); err != nil {
		t.Fatal(err)
	}
	cost, err := s.MonthToDateCost(ctx)
	if err != nil || math.Abs(cost-0.50022) > 1e-9 {
		t.Errorf("month to date = %v, %v; want 0.50022", cost, err)
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
