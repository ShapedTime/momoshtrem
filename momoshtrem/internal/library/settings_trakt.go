package library

import (
	"context"
	"encoding/json"
	"fmt"
)

// TraktSettingsKey is the app_settings key of the Trakt settings: one JSON
// document shaped like PUT /integrations/trakt/settings answers (PLAN §6
// "Trakt (B5)"). Fields it doesn't hold take their defaults, and a
// disconnect keeps it.
const TraktSettingsKey = "trakt"

// TraktSettings are what momoshtrem takes from Trakt.
type TraktSettings struct {
	// UseRatings lets recommendations (#9) weigh Trakt ratings.
	UseRatings bool `json:"use_ratings"`
	// ShowWatchlist shows the watchlist on For You, and Up next's
	// watchlist starts.
	ShowWatchlist bool `json:"show_watchlist"`
	// IncludePaused keeps shows idle for over 180 days in Up next.
	IncludePaused bool `json:"include_paused"`
}

// DefaultTraktSettings are the settings before anyone changes them.
func DefaultTraktSettings() TraktSettings {
	return TraktSettings{UseRatings: true, ShowWatchlist: true}
}

// TraktSettingsPatch is a partial update: a nil field keeps its value.
type TraktSettingsPatch struct {
	UseRatings    *bool `json:"use_ratings,omitempty"`
	ShowWatchlist *bool `json:"show_watchlist,omitempty"`
	IncludePaused *bool `json:"include_paused,omitempty"`
}

// TraktSettings reads the Trakt settings over their defaults: a field the
// stored document doesn't have keeps its default. A document that isn't
// the settings gives the defaults and the error.
func (r *SettingsRepository) TraktSettings(ctx context.Context) (TraktSettings, error) {
	raw, found, err := r.Get(ctx, TraktSettingsKey)
	if err != nil || !found {
		return DefaultTraktSettings(), err
	}
	return decodeTraktSettings(raw)
}

// UpdateTraktSettings stores the fields patch sets and answers the
// settings it leaves. It's one statement (jsonb ||), so two updates of
// different fields never lose each other's change.
func (r *SettingsRepository) UpdateTraktSettings(ctx context.Context, patch TraktSettingsPatch) (TraktSettings, error) {
	set, err := json.Marshal(patch)
	if err != nil {
		return DefaultTraktSettings(), fmt.Errorf("failed to encode setting %q: %w", TraktSettingsKey, err)
	}
	var raw []byte
	err = r.db.QueryRowContext(ctx, `
		INSERT INTO app_settings (key, value, updated_at) VALUES ($1, $2::jsonb, NOW())
		ON CONFLICT (key) DO UPDATE SET
			value = CASE WHEN jsonb_typeof(app_settings.value) = 'object' THEN app_settings.value ELSE '{}'::jsonb END || EXCLUDED.value,
			updated_at = NOW()
		RETURNING value
	`, TraktSettingsKey, string(set)).Scan(&raw)
	if err != nil {
		return DefaultTraktSettings(), fmt.Errorf("failed to update setting %q: %w", TraktSettingsKey, err)
	}
	return decodeTraktSettings(raw)
}

func decodeTraktSettings(raw []byte) (TraktSettings, error) {
	settings := DefaultTraktSettings()
	// Unmarshal only overwrites the fields present in raw.
	if err := json.Unmarshal(raw, &settings); err != nil {
		return DefaultTraktSettings(), fmt.Errorf("failed to decode setting %q: %w", TraktSettingsKey, err)
	}
	return settings, nil
}
