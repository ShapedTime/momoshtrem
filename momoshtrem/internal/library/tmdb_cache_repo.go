package library

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// TMDBCacheRepository stores TMDB responses (and values derived from them,
// such as artwork colours) in tmdb_cache. Expired rows are kept: a stale
// payload beats an error when TMDB is down.
type TMDBCacheRepository struct {
	db *DB
}

// NewTMDBCacheRepository creates a new TMDB cache repository.
func NewTMDBCacheRepository(db *DB) *TMDBCacheRepository {
	return &TMDBCacheRepository{db: db}
}

// Get returns the JSON payload stored under key and when it expires (the
// zero time when it never does). found is false when there is no row.
func (r *TMDBCacheRepository) Get(ctx context.Context, key string) (payload []byte, expiresAt time.Time, found bool, err error) {
	var expires sql.NullTime
	err = r.db.QueryRowContext(ctx,
		`SELECT payload, expires_at FROM tmdb_cache WHERE key = $1`, key,
	).Scan(&payload, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, time.Time{}, false, nil
	}
	if err != nil {
		return nil, time.Time{}, false, fmt.Errorf("failed to read tmdb cache %q: %w", key, err)
	}
	return payload, expires.Time, true, nil
}

// Set stores a JSON payload under key. A zero expiresAt never expires.
func (r *TMDBCacheRepository) Set(ctx context.Context, key string, payload []byte, expiresAt time.Time) error {
	var expires sql.NullTime
	if !expiresAt.IsZero() {
		expires = sql.NullTime{Time: expiresAt, Valid: true}
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO tmdb_cache (key, payload, fetched_at, expires_at) VALUES ($1, $2::jsonb, NOW(), $3)
		ON CONFLICT (key) DO UPDATE SET
			payload = EXCLUDED.payload, fetched_at = EXCLUDED.fetched_at, expires_at = EXCLUDED.expires_at
	`, key, string(payload), expires)
	if err != nil {
		return fmt.Errorf("failed to write tmdb cache %q: %w", key, err)
	}
	return nil
}
