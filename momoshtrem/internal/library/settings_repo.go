package library

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

// ErrInvalidSetting is returned for an empty key or a value that isn't JSON.
var ErrInvalidSetting = errors.New("invalid setting")

// SettingsRepository stores application settings as JSON documents in
// app_settings, one row per key.
type SettingsRepository struct {
	db *DB
}

// NewSettingsRepository creates a new settings repository
func NewSettingsRepository(db *DB) *SettingsRepository {
	return &SettingsRepository{db: db}
}

// Get returns the raw JSON stored under key. found is false when the key has
// never been set.
func (r *SettingsRepository) Get(ctx context.Context, key string) (value json.RawMessage, found bool, err error) {
	if key == "" {
		return nil, false, fmt.Errorf("%w: empty key", ErrInvalidSetting)
	}

	var raw []byte
	err = r.db.QueryRowContext(ctx,
		`SELECT value FROM app_settings WHERE key = $1`, key,
	).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("failed to get setting %q: %w", key, err)
	}

	return json.RawMessage(raw), true, nil
}

// Set stores value under key, replacing any previous value.
func (r *SettingsRepository) Set(ctx context.Context, key string, value json.RawMessage) error {
	if key == "" {
		return fmt.Errorf("%w: empty key", ErrInvalidSetting)
	}
	if !json.Valid(value) {
		return fmt.Errorf("%w: value for %q is not valid JSON", ErrInvalidSetting, key)
	}

	_, err := r.db.ExecContext(ctx, `
		INSERT INTO app_settings (key, value, updated_at) VALUES ($1, $2::jsonb, NOW())
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()
	`, key, string(value))
	if err != nil {
		return fmt.Errorf("failed to set setting %q: %w", key, err)
	}
	return nil
}

// Delete removes key. Deleting a key that doesn't exist is not an error, so
// callers can use it to reset a setting to its default.
func (r *SettingsRepository) Delete(ctx context.Context, key string) error {
	if key == "" {
		return fmt.Errorf("%w: empty key", ErrInvalidSetting)
	}

	if _, err := r.db.ExecContext(ctx, `DELETE FROM app_settings WHERE key = $1`, key); err != nil {
		return fmt.Errorf("failed to delete setting %q: %w", key, err)
	}
	return nil
}

// GetJSON decodes the value stored under key into dst. found is false (and
// dst untouched) when the key has never been set.
func (r *SettingsRepository) GetJSON(ctx context.Context, key string, dst any) (found bool, err error) {
	raw, found, err := r.Get(ctx, key)
	if err != nil || !found {
		return found, err
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		return true, fmt.Errorf("failed to decode setting %q: %w", key, err)
	}
	return true, nil
}

// SetJSON encodes v and stores it under key.
func (r *SettingsRepository) SetJSON(ctx context.Context, key string, v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("failed to encode setting %q: %w", key, err)
	}
	return r.Set(ctx, key, raw)
}
