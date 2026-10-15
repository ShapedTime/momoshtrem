package service

import (
	"context"
	"fmt"
	"time"

	"github.com/shapedtime/momoshtrem/internal/identify"
	"github.com/shapedtime/momoshtrem/internal/identify/ai"
	"github.com/shapedtime/momoshtrem/internal/library"
)

// ClassifierAIStore gives the AI resolver its Postgres side: answers cached
// in ai_cache, every call's usage in ai_usage (against its job), and the
// monthly budget (monthly_budget_usd in the AI settings, over every AI
// feature's spend this month).
type ClassifierAIStore struct {
	store    *library.AIStore
	settings *library.SettingsRepository
}

var (
	_ ai.Cache         = (*ClassifierAIStore)(nil)
	_ ai.UsageRecorder = (*ClassifierAIStore)(nil)
	_ ai.Budget        = (*ClassifierAIStore)(nil)
)

// NewClassifierAIStore creates the resolver's store.
func NewClassifierAIStore(store *library.AIStore, settings *library.SettingsRepository) *ClassifierAIStore {
	return &ClassifierAIStore{store: store, settings: settings}
}

// Get reads a cached answer.
func (c *ClassifierAIStore) Get(ctx context.Context, key string) ([]byte, bool, error) {
	return c.store.CacheGet(ctx, key)
}

// Put caches an answer.
func (c *ClassifierAIStore) Put(ctx context.Context, key string, payload []byte, ttl time.Duration) error {
	return c.store.CachePut(ctx, library.AIFeatureClassifier, key, payload, ttl)
}

// Record stores a call's usage. ai_usage has no cache-write column, so
// cache writes count as input tokens; the cost has them at their own rate.
func (c *ClassifierAIStore) Record(ctx context.Context, u identify.Usage, ref int64) error {
	var refID *int64
	if ref != 0 {
		refID = &ref
	}
	return c.store.RecordUsage(ctx, library.AIUsage{
		Feature: library.AIFeatureClassifier, Model: u.Model,
		InputTokens: u.InputTokens + u.CacheWriteTokens, OutputTokens: u.OutputTokens,
		CacheReadTokens: u.CacheReadTokens, CostUSD: u.CostUSD, RefID: refID,
	})
}

// Check fails with ai.ErrBudgetExceeded once this month's spend reaches
// monthly_budget_usd. Without a budget there's no limit; when the spend
// can't be read, the call is refused.
func (c *ClassifierAIStore) Check(ctx context.Context) error {
	settings, err := c.settings.AISettings(ctx)
	if err != nil {
		return fmt.Errorf("failed to read the AI budget: %w", err)
	}
	if settings.MonthlyBudgetUSD == nil {
		return nil
	}
	spent, err := c.store.MonthToDateCost(ctx)
	if err != nil {
		return err
	}
	if budget := *settings.MonthlyBudgetUSD; spent >= budget {
		return fmt.Errorf("%w: $%.2f of $%.2f", ai.ErrBudgetExceeded, spent, budget)
	}
	return nil
}
