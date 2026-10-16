package library

import (
	"context"
	"encoding/json"
	"fmt"
)

// AISettingsKey is the app_settings key of the AI settings: one JSON
// document shaped like GET/PUT /settings/ai (PLAN §6 "Recommendations
// (R1–R3)"). Fields it doesn't hold take their defaults.
const AISettingsKey = "ai"

// Default AI settings.
const (
	DefaultClassifierModel = "claude-haiku-5-5"
	DefaultRecModel        = "claude-opus-5-5"
	DefaultRecSchedule     = "nightly"
	// DefaultMonthlyBudgetUSD is a fresh install's budget (the user's
	// decision, #9): a document without monthly_budget_usd has it, and an
	// explicit null means no cap.
	DefaultMonthlyBudgetUSD = 15.0
)

// AISettings is the "ai" document: the picks' settings (#9), the monthly
// budget every AI feature shares, and the classifier's settings (#5). The
// JSON is GET /settings/ai's.
type AISettings struct {
	// RecEnabled is the master switch for picks: off, no runs, shelves,
	// hero picks or title-page reasons. Ask is unaffected.
	RecEnabled bool `json:"rec_enabled"`
	// RecModel runs the picks and new Ask threads (claude-opus-5-5 or
	// claude-sonnet-5-5).
	RecModel string `json:"rec_model"`
	// RecSchedule is nightly, weekly or manual: 03:00 server-local,
	// weekly on Sunday.
	RecSchedule string `json:"rec_schedule"`
	// MonthlyBudgetUSD caps what every AI feature may spend in a
	// server-local calendar month; nil means no cap. Once it's spent, AI
	// calls are skipped.
	MonthlyBudgetUSD *float64 `json:"monthly_budget_usd"`
	// ClassifierEnabled lets the AI resolver look at the files the rules
	// aren't sure about.
	ClassifierEnabled bool   `json:"classifier_enabled"`
	ClassifierModel   string `json:"classifier_model"`
	// AutoApplyHighConfidence applies a classified job on its own when
	// everything in it is high confidence.
	AutoApplyHighConfidence bool `json:"auto_apply_high_confidence"`
}

// DefaultAISettings are the settings before anyone changes them.
func DefaultAISettings() AISettings {
	budget := DefaultMonthlyBudgetUSD
	return AISettings{
		RecEnabled:              true,
		RecModel:                DefaultRecModel,
		RecSchedule:             DefaultRecSchedule,
		MonthlyBudgetUSD:        &budget,
		ClassifierEnabled:       true,
		ClassifierModel:         DefaultClassifierModel,
		AutoApplyHighConfidence: true,
	}
}

// AISettingsPatch is a partial update: a nil field keeps its value.
// MonthlyBudgetUSD needs two levels: SetBudget says it's sent, and a nil
// MonthlyBudgetUSD with it means no cap.
type AISettingsPatch struct {
	RecEnabled              *bool
	RecModel                *string
	RecSchedule             *string
	SetBudget               bool
	MonthlyBudgetUSD        *float64
	ClassifierEnabled       *bool
	ClassifierModel         *string
	AutoApplyHighConfidence *bool
}

// MarshalJSON writes the fields the patch sets, and only those: the
// object UpdateAISettings merges into the document.
func (p AISettingsPatch) MarshalJSON() ([]byte, error) {
	set := map[string]any{}
	put := func(name string, ok bool, v any) {
		if ok {
			set[name] = v
		}
	}
	put("rec_enabled", p.RecEnabled != nil, p.RecEnabled)
	put("rec_model", p.RecModel != nil, p.RecModel)
	put("rec_schedule", p.RecSchedule != nil, p.RecSchedule)
	put("monthly_budget_usd", p.SetBudget, p.MonthlyBudgetUSD)
	put("classifier_enabled", p.ClassifierEnabled != nil, p.ClassifierEnabled)
	put("classifier_model", p.ClassifierModel != nil, p.ClassifierModel)
	put("auto_apply_high_confidence", p.AutoApplyHighConfidence != nil, p.AutoApplyHighConfidence)
	return json.Marshal(set)
}

// AISettings reads the AI settings over their defaults: a field the stored
// document doesn't have keeps its default. A document that isn't the
// settings gives the defaults and the error.
func (r *SettingsRepository) AISettings(ctx context.Context) (AISettings, error) {
	raw, found, err := r.Get(ctx, AISettingsKey)
	if err != nil || !found {
		return DefaultAISettings(), err
	}
	return decodeAISettings(raw)
}

// UpdateAISettings stores the fields patch sets and answers every setting.
// It's one statement (jsonb ||), so two updates of different fields never
// lose each other's change. Values are the caller's to check.
func (r *SettingsRepository) UpdateAISettings(ctx context.Context, patch AISettingsPatch) (AISettings, error) {
	set, err := json.Marshal(patch)
	if err != nil {
		return DefaultAISettings(), fmt.Errorf("failed to encode setting %q: %w", AISettingsKey, err)
	}
	var raw []byte
	err = r.db.QueryRowContext(ctx, `
		INSERT INTO app_settings (key, value, updated_at) VALUES ($1, $2::jsonb, NOW())
		ON CONFLICT (key) DO UPDATE SET
			value = CASE WHEN jsonb_typeof(app_settings.value) = 'object' THEN app_settings.value ELSE '{}'::jsonb END || EXCLUDED.value,
			updated_at = NOW()
		RETURNING value
	`, AISettingsKey, string(set)).Scan(&raw)
	if err != nil {
		return DefaultAISettings(), fmt.Errorf("failed to update setting %q: %w", AISettingsKey, err)
	}
	return decodeAISettings(raw)
}

func decodeAISettings(raw []byte) (AISettings, error) {
	settings := DefaultAISettings()
	// Unmarshal only overwrites the fields present in raw; an explicit null
	// budget sets it to nil (no cap).
	if err := json.Unmarshal(raw, &settings); err != nil {
		return DefaultAISettings(), fmt.Errorf("failed to decode setting %q: %w", AISettingsKey, err)
	}
	if settings.ClassifierModel == "" {
		settings.ClassifierModel = DefaultClassifierModel
	}
	if settings.RecModel == "" {
		settings.RecModel = DefaultRecModel
	}
	if settings.RecSchedule == "" {
		settings.RecSchedule = DefaultRecSchedule
	}
	return settings, nil
}
