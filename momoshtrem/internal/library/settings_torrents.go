package library

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// TorrentSettingsKey is the app_settings key of the torrent settings, one
// JSON document: {"paused": {"<info hash>": "<RFC 3339 time>"}}, the
// torrents the user paused and when (PLAN §6 "Torrents and activity
// (B6)"). Every write is one atomic jsonb statement, so concurrent pauses
// and resumes never lose each other's changes.
const TorrentSettingsKey = "torrents"

// TorrentPauses returns the stored pauses: info hash → when it was paused.
// A time that doesn't parse is the zero time: the torrent is still paused.
func (r *SettingsRepository) TorrentPauses(ctx context.Context) (map[string]time.Time, error) {
	var raw []byte
	err := r.db.QueryRowContext(ctx, `
		SELECT value->'paused' FROM app_settings
		WHERE key = $1 AND jsonb_typeof(value->'paused') = 'object'
	`, TorrentSettingsKey).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return map[string]time.Time{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read the torrent pauses: %w", err)
	}
	var stored map[string]any
	if err := json.Unmarshal(raw, &stored); err != nil {
		return nil, fmt.Errorf("failed to decode the torrent pauses: %w", err)
	}
	pauses := make(map[string]time.Time, len(stored))
	for hash, v := range stored {
		var at time.Time
		if s, ok := v.(string); ok {
			at, _ = time.Parse(time.RFC3339Nano, s)
		}
		pauses[hash] = at
	}
	return pauses, nil
}

// PauseTorrents stores pauses (info hash → when), in one statement. A
// torrent already paused keeps its first time.
func (r *SettingsRepository) PauseTorrents(ctx context.Context, pauses map[string]time.Time) error {
	if len(pauses) == 0 {
		return nil
	}
	stamps := make(map[string]string, len(pauses))
	for hash, at := range pauses {
		stamps[hash] = at.UTC().Format(time.RFC3339Nano)
	}
	add, err := json.Marshal(stamps)
	if err != nil {
		return fmt.Errorf("failed to encode the torrent pauses: %w", err)
	}
	// The stored object goes on the right of ||, so its times win.
	_, err = r.db.ExecContext(ctx, `
		INSERT INTO app_settings (key, value, updated_at)
		VALUES ($1, jsonb_build_object('paused', $2::jsonb), NOW())
		ON CONFLICT (key) DO UPDATE SET
			value = jsonb_set(
				CASE WHEN jsonb_typeof(app_settings.value) = 'object' THEN app_settings.value ELSE '{}'::jsonb END,
				'{paused}',
				$2::jsonb || CASE WHEN jsonb_typeof(app_settings.value->'paused') = 'object'
				                  THEN app_settings.value->'paused' ELSE '{}'::jsonb END),
			updated_at = NOW()
	`, TorrentSettingsKey, string(add))
	if err != nil {
		return fmt.Errorf("failed to store the torrent pauses: %w", err)
	}
	return nil
}

// ResumeTorrents removes the torrents' pauses, in one statement. Hashes
// that aren't paused are ignored.
func (r *SettingsRepository) ResumeTorrents(ctx context.Context, hashes []string) error {
	if len(hashes) == 0 {
		return nil
	}
	_, err := r.db.ExecContext(ctx, `
		UPDATE app_settings SET value = jsonb_set(value, '{paused}', (value->'paused') - $2::text[]), updated_at = NOW()
		WHERE key = $1 AND jsonb_typeof(value->'paused') = 'object'
	`, TorrentSettingsKey, hashes)
	if err != nil {
		return fmt.Errorf("failed to clear the torrent pauses: %w", err)
	}
	return nil
}
