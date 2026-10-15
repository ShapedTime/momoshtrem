package library

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// AI features: ai_usage.feature and ai_cache.feature.
const (
	AIFeatureClassifier = "classifier"
	AIFeatureRecs       = "recs"
	AIFeatureAsk        = "ask"
)

// AIStore keeps what the AI features answered (ai_cache) and what they
// cost (ai_usage). Every feature shares it.
type AIStore struct {
	db *DB
}

// NewAIStore creates an AI store.
func NewAIStore(db *DB) *AIStore {
	return &AIStore{db: db}
}

// CacheGet returns the cached payload under key, unless it expired.
func (s *AIStore) CacheGet(ctx context.Context, key string) ([]byte, bool, error) {
	var payload []byte
	err := s.db.QueryRowContext(ctx, `
		SELECT payload FROM ai_cache WHERE key = $1 AND (expires_at IS NULL OR expires_at > NOW())
	`, key).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("failed to read the AI cache: %w", err)
	}
	return payload, true, nil
}

// CachePut stores a JSON payload under key for ttl (0: no expiry),
// replacing any earlier one.
func (s *AIStore) CachePut(ctx context.Context, feature, key string, payload []byte, ttl time.Duration) error {
	var expires any
	if ttl > 0 {
		expires = time.Now().Add(ttl)
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO ai_cache (key, feature, payload, created_at, expires_at)
		VALUES ($1, $2, $3::jsonb, NOW(), $4)
		ON CONFLICT (key) DO UPDATE
		SET feature = EXCLUDED.feature, payload = EXCLUDED.payload, created_at = NOW(), expires_at = EXCLUDED.expires_at
	`, key, feature, string(payload), expires)
	if err != nil {
		return fmt.Errorf("failed to write the AI cache: %w", err)
	}
	return nil
}

// DeleteExpiredCache removes expired cache entries.
func (s *AIStore) DeleteExpiredCache(ctx context.Context) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM ai_cache WHERE expires_at <= NOW()`)
	if err != nil {
		return 0, fmt.Errorf("failed to prune the AI cache: %w", err)
	}
	return res.RowsAffected()
}

// AIUsage is one API call's usage. InputTokens are the prompt tokens not
// read from the cache (cache writes included); CostUSD prices them all.
type AIUsage struct {
	Feature         string
	Model           string
	InputTokens     int
	OutputTokens    int
	CacheReadTokens int
	WebSearches     int
	CostUSD         float64
	// RefID is the job or run the call was for; nil for none.
	RefID *int64
}

// RecordUsage stores a call's usage.
func (s *AIStore) RecordUsage(ctx context.Context, u AIUsage) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO ai_usage (feature, model, input_tokens, output_tokens, cache_read_tokens, web_searches, cost_usd, ref_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, u.Feature, u.Model, u.InputTokens, u.OutputTokens, u.CacheReadTokens, u.WebSearches, u.CostUSD, u.RefID)
	if err != nil {
		return fmt.Errorf("failed to record AI usage: %w", err)
	}
	return nil
}

// MonthToDateCost is what every AI feature has cost since the start of
// the database's current month.
func (s *AIStore) MonthToDateCost(ctx context.Context) (float64, error) {
	var cost float64
	err := s.db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(cost_usd), 0)::float8 FROM ai_usage WHERE created_at >= date_trunc('month', NOW())
	`).Scan(&cost)
	if err != nil {
		return 0, fmt.Errorf("failed to sum AI usage: %w", err)
	}
	return cost, nil
}
