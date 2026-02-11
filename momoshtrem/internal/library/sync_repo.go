package library

import (
	"database/sql"
	"fmt"
	"log/slog"
	"time"
)

// SyncMetadataRepository handles sync metadata database operations
type SyncMetadataRepository struct {
	db *DB
}

// NewSyncMetadataRepository creates a new sync metadata repository
func NewSyncMetadataRepository(db *DB) *SyncMetadataRepository {
	return &SyncMetadataRepository{db: db}
}

// GetLastSyncTime retrieves the last sync time for a given key
func (r *SyncMetadataRepository) GetLastSyncTime(key string) (time.Time, error) {
	var value string
	err := r.db.QueryRow(
		`SELECT value FROM sync_metadata WHERE key = $1`,
		"last_"+key,
	).Scan(&value)

	if err == sql.ErrNoRows || value == "" {
		return time.Time{}, nil // Never synced
	}
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to get last sync time: %w", err)
	}

	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		// Gracefully handle corrupt timestamps - log warning and treat as never synced
		slog.Warn("Failed to parse sync timestamp, treating as never synced",
			"key", key,
			"value", value,
			"error", err,
		)
		return time.Time{}, nil
	}

	return t, nil
}

// SetLastSyncTime updates the last sync time for a given key
func (r *SyncMetadataRepository) SetLastSyncTime(key string, t time.Time) error {
	_, err := r.db.Exec(`
		INSERT INTO sync_metadata (key, value, updated_at) VALUES ($1, $2, NOW())
		ON CONFLICT(key) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()
	`, "last_"+key, t.Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("failed to set last sync time: %w", err)
	}
	return nil
}

// GetValue retrieves a generic value from sync metadata
func (r *SyncMetadataRepository) GetValue(key string) (string, error) {
	var value string
	err := r.db.QueryRow(
		`SELECT value FROM sync_metadata WHERE key = $1`,
		key,
	).Scan(&value)

	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("failed to get sync metadata value: %w", err)
	}

	return value, nil
}

// SetValue sets a generic value in sync metadata
func (r *SyncMetadataRepository) SetValue(key, value string) error {
	_, err := r.db.Exec(`
		INSERT INTO sync_metadata (key, value, updated_at) VALUES ($1, $2, NOW())
		ON CONFLICT(key) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()
	`, key, value)
	if err != nil {
		return fmt.Errorf("failed to set sync metadata value: %w", err)
	}
	return nil
}
