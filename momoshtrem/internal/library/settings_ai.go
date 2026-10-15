package library

import (
	"context"
	"encoding/json"
)

// AISettingsKey is the app_settings key of the AI settings: one JSON
// document shaped like GET/PUT /settings/ai (PLAN §6 "Recommendations"),
// which #9 serves. Fields it doesn't hold take their defaults.
const AISettingsKey = "ai"

// Default AI settings.
const (
	DefaultClassifierModel = "claude-haiku-5-5"
)

// AISettings are the AI settings the assignment jobs read (the classifier's
// share of the "ai" document).
type AISettings struct {
	// ClassifierEnabled lets the AI resolver look at the files the rules
	// aren't sure about.
	ClassifierEnabled bool   `json:"classifier_enabled"`
	ClassifierModel   string `json:"classifier_model"`
	// AutoApplyHighConfidence applies a classified job on its own when
	// everything in it is high confidence.
	AutoApplyHighConfidence bool `json:"auto_apply_high_confidence"`
	// MonthlyBudgetUSD caps what every AI feature may spend in a calendar
	// month; nil means no cap. Once it's spent, AI calls are skipped.
	MonthlyBudgetUSD *float64 `json:"monthly_budget_usd"`
}

// DefaultAISettings are the settings before anyone changes them.
func DefaultAISettings() AISettings {
	return AISettings{
		ClassifierEnabled:       true,
		ClassifierModel:         DefaultClassifierModel,
		AutoApplyHighConfidence: true,
	}
}

// AISettings reads the AI settings over their defaults: a field the stored
// document doesn't have keeps its default.
func (r *SettingsRepository) AISettings(ctx context.Context) (AISettings, error) {
	settings := DefaultAISettings()
	raw, found, err := r.Get(ctx, AISettingsKey)
	if err != nil || !found {
		return settings, err
	}
	// Unmarshal only overwrites the fields present in raw.
	if err := json.Unmarshal(raw, &settings); err != nil {
		return DefaultAISettings(), err
	}
	if settings.ClassifierModel == "" {
		settings.ClassifierModel = DefaultClassifierModel
	}
	return settings, nil
}
