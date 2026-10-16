package library

import (
	"context"
	"encoding/json"
	"reflect"
	"sync"
	"testing"
)

func floatPtr(f float64) *float64 { return &f }

// The settings' JSON is the contract's AiSettings; a patch sends only the
// fields it sets, a null budget included.
func TestAISettings_JSON(t *testing.T) {
	raw, err := json.Marshal(DefaultAISettings())
	if err != nil {
		t.Fatal(err)
	}
	assertJSONEqual(t, raw, `{"rec_enabled": true, "rec_model": "claude-opus-5-5", "rec_schedule": "nightly",
		"monthly_budget_usd": 15, "classifier_enabled": true, "classifier_model": "claude-haiku-5-5",
		"auto_apply_high_confidence": true}`)

	for _, tt := range []struct {
		patch AISettingsPatch
		want  string
	}{
		{AISettingsPatch{}, `{}`},
		{AISettingsPatch{RecSchedule: strPtr("weekly"), RecEnabled: boolPtr(false)}, `{"rec_schedule": "weekly", "rec_enabled": false}`},
		{AISettingsPatch{SetBudget: true}, `{"monthly_budget_usd": null}`},
		{AISettingsPatch{SetBudget: true, MonthlyBudgetUSD: floatPtr(7.5), ClassifierModel: strPtr("claude-opus-5-5")},
			`{"monthly_budget_usd": 7.5, "classifier_model": "claude-opus-5-5"}`},
		// A budget without SetBudget isn't sent.
		{AISettingsPatch{MonthlyBudgetUSD: floatPtr(3)}, `{}`},
	} {
		raw, err := json.Marshal(tt.patch)
		if err != nil {
			t.Fatal(err)
		}
		assertJSONEqual(t, raw, tt.want)
	}
}

func TestSettingsRepository_AISettings(t *testing.T) {
	db := openTestDB(t)
	repo := NewSettingsRepository(db)
	ctx := context.Background()

	// The settings have one fixed key: leave the database as it was.
	before, had, err := repo.Get(ctx, AISettingsKey)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if had {
			_ = repo.Set(ctx, AISettingsKey, before)
		} else {
			_ = repo.Delete(ctx, AISettingsKey)
		}
	})
	reset := func(t *testing.T) {
		t.Helper()
		if err := repo.Delete(ctx, AISettingsKey); err != nil {
			t.Fatal(err)
		}
	}
	read := func(t *testing.T) AISettings {
		t.Helper()
		got, err := repo.AISettings(ctx)
		if err != nil {
			t.Fatalf("AISettings: %v", err)
		}
		return got
	}

	t.Run("a fresh install has the defaults: a $15 budget", func(t *testing.T) {
		reset(t)
		if got := read(t); !reflect.DeepEqual(got, DefaultAISettings()) || *got.MonthlyBudgetUSD != 15 {
			t.Errorf("AISettings = %+v, want the defaults", got)
		}
	})

	t.Run("a stored document keeps the defaults it doesn't have", func(t *testing.T) {
		reset(t)
		// #5's document: the classifier's fields only, and one nothing reads.
		if err := repo.Set(ctx, AISettingsKey, json.RawMessage(`{"classifier_enabled": false, "unknown_field": "kept, never read"}`)); err != nil {
			t.Fatal(err)
		}
		want := DefaultAISettings()
		want.ClassifierEnabled = false
		if got := read(t); !reflect.DeepEqual(got, want) {
			t.Errorf("AISettings = %+v, want %+v", got, want)
		}
	})

	t.Run("an explicit null budget is no cap", func(t *testing.T) {
		reset(t)
		if err := repo.Set(ctx, AISettingsKey, json.RawMessage(`{"monthly_budget_usd": null}`)); err != nil {
			t.Fatal(err)
		}
		if got := read(t); got.MonthlyBudgetUSD != nil {
			t.Errorf("budget = %v, want no cap", *got.MonthlyBudgetUSD)
		}
	})

	t.Run("a patch changes only its fields", func(t *testing.T) {
		reset(t)
		got, err := repo.UpdateAISettings(ctx, AISettingsPatch{RecSchedule: strPtr("weekly"), SetBudget: true, MonthlyBudgetUSD: floatPtr(4.5)})
		if err != nil {
			t.Fatalf("UpdateAISettings: %v", err)
		}
		want := DefaultAISettings()
		want.RecSchedule, want.MonthlyBudgetUSD = "weekly", floatPtr(4.5)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("answer = %+v, want %+v", got, want)
		}
		got, err = repo.UpdateAISettings(ctx, AISettingsPatch{RecModel: strPtr("claude-sonnet-5-5"), SetBudget: true})
		if err != nil {
			t.Fatal(err)
		}
		want.RecModel, want.MonthlyBudgetUSD = "claude-sonnet-5-5", nil
		if !reflect.DeepEqual(got, want) || !reflect.DeepEqual(read(t), want) {
			t.Errorf("after a second patch = %+v, want %+v", got, want)
		}
		// Unrelated keys in the document survive.
		if _, err := db.Exec(`UPDATE app_settings SET value = value || '{"other": 1}' WHERE key = $1`, AISettingsKey); err != nil {
			t.Fatal(err)
		}
		if _, err := repo.UpdateAISettings(ctx, AISettingsPatch{AutoApplyHighConfidence: boolPtr(false)}); err != nil {
			t.Fatal(err)
		}
		raw, _, _ := repo.Get(ctx, AISettingsKey)
		assertJSONEqual(t, raw, `{"rec_schedule": "weekly", "monthly_budget_usd": null, "rec_model": "claude-sonnet-5-5",
			"other": 1, "auto_apply_high_confidence": false}`)
	})

	t.Run("a document that isn't an object is replaced by the patch", func(t *testing.T) {
		reset(t)
		if err := repo.Set(ctx, AISettingsKey, json.RawMessage(`[1]`)); err != nil {
			t.Fatal(err)
		}
		if _, err := repo.AISettings(ctx); err == nil {
			t.Error("a broken document read without an error")
		}
		got, err := repo.UpdateAISettings(ctx, AISettingsPatch{RecEnabled: boolPtr(false)})
		want := DefaultAISettings()
		want.RecEnabled = false
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("answer = %+v, %v", got, err)
		}
	})

	t.Run("concurrent patches of different fields both land", func(t *testing.T) {
		reset(t)
		var wg sync.WaitGroup
		for i := range 10 {
			wg.Add(2)
			go func() {
				defer wg.Done()
				_, _ = repo.UpdateAISettings(ctx, AISettingsPatch{RecEnabled: boolPtr(i%2 == 0)})
			}()
			go func() {
				defer wg.Done()
				_, _ = repo.UpdateAISettings(ctx, AISettingsPatch{ClassifierModel: strPtr("claude-sonnet-5-5")})
			}()
		}
		wg.Wait()
		if got := read(t); got.ClassifierModel != "claude-sonnet-5-5" {
			t.Errorf("classifier_model lost: %+v", got)
		}
	})
}
