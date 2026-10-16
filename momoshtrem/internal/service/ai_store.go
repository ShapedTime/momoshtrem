package service

import (
	"context"
	"time"

	"github.com/shapedtime/momoshtrem/internal/aicore"
	"github.com/shapedtime/momoshtrem/internal/identify"
	"github.com/shapedtime/momoshtrem/internal/identify/ai"
	"github.com/shapedtime/momoshtrem/internal/library"
)

// ClassifierAIStore gives the AI resolver its Postgres side: answers cached
// in ai_cache, and the AI core's ledger (aicore.Ledger) for every call's
// usage in ai_usage (against its job) and the monthly budget every AI
// feature shares, on the server-local month.
type ClassifierAIStore struct {
	store  *library.AIStore
	ledger *aicore.Ledger
}

var (
	_ ai.Cache         = (*ClassifierAIStore)(nil)
	_ ai.UsageRecorder = (*ClassifierAIStore)(nil)
	_ ai.Budget        = (*ClassifierAIStore)(nil)
)

// NewClassifierAIStore creates the resolver's store.
func NewClassifierAIStore(store *library.AIStore, settings *library.SettingsRepository) *ClassifierAIStore {
	return &ClassifierAIStore{store: store, ledger: aicore.NewLedger(store, settings)}
}

// Get reads a cached answer.
func (c *ClassifierAIStore) Get(ctx context.Context, key string) ([]byte, bool, error) {
	return c.store.CacheGet(ctx, key)
}

// Put caches an answer.
func (c *ClassifierAIStore) Put(ctx context.Context, key string, payload []byte, ttl time.Duration) error {
	return c.store.CachePut(ctx, library.AIFeatureClassifier, key, payload, ttl)
}

// Record stores a call's usage as a classifier row.
func (c *ClassifierAIStore) Record(ctx context.Context, u identify.Usage, ref int64) error {
	return c.ledger.Record(ctx, library.AIFeatureClassifier, ref, aicore.Usage{
		Model: u.Model,
		Tokens: aicore.Tokens{
			Input: int64(u.InputTokens), CacheWrite: int64(u.CacheWriteTokens),
			CacheRead: int64(u.CacheReadTokens), Output: int64(u.OutputTokens),
		},
		CostUSD: u.CostUSD,
		Calls:   1,
	})
}

// Check fails with ai.ErrBudgetExceeded (an *aicore.BudgetError) once
// this month's spend reaches monthly_budget_usd ($15 unless set; an
// explicit null is no cap). When the spend can't be read, the call is
// refused.
func (c *ClassifierAIStore) Check(ctx context.Context) error {
	return c.ledger.Check(ctx)
}
