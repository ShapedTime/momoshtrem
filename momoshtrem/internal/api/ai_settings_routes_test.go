package api

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/shapedtime/momoshtrem/internal/recommend"
)

// fakeAISettings keeps the settings and applies patches like the store.
type fakeAISettings struct {
	err      error
	settings recommend.Settings
	patches  []recommend.SettingsPatch
}

func (f *fakeAISettings) AISettings(context.Context) (recommend.Settings, error) {
	return f.settings, f.err
}

func (f *fakeAISettings) UpdateAISettings(_ context.Context, p recommend.SettingsPatch) (recommend.Settings, error) {
	f.patches = append(f.patches, p)
	if p.RecEnabled != nil {
		f.settings.RecEnabled = *p.RecEnabled
	}
	if p.RecModel != nil {
		f.settings.RecModel = *p.RecModel
	}
	if p.RecSchedule != nil {
		f.settings.RecSchedule = *p.RecSchedule
	}
	if p.SetBudget {
		f.settings.MonthlyBudgetUSD = p.MonthlyBudgetUSD
	}
	return f.settings, f.err
}

func (f *fakeAISettings) AIUsage(context.Context) (*recommend.UsageReport, error) {
	// Nil lists and map: the handler makes them [] and a key per model
	return &recommend.UsageReport{AskDailyLimit: recommend.AskDailyLimit}, f.err
}

var _ AISettings = (*fakeAISettings)(nil)

// freshAISettings are a fresh install's (PLAN §6); the store owns the real
// defaults (library.DefaultAISettings, #154).
func freshAISettings() recommend.Settings {
	budget := 15.0
	return recommend.Settings{
		RecEnabled:              true,
		RecModel:                recommend.ModelOpus,
		RecSchedule:             recommend.ScheduleNightly,
		MonthlyBudgetUSD:        &budget,
		ClassifierEnabled:       true,
		ClassifierModel:         recommend.ModelHaiku,
		AutoApplyHighConfidence: true,
	}
}

func TestAISettingsRoutes(t *testing.T) {
	f := &fakeAISettings{settings: freshAISettings()}
	s := newTestServer(Options{AISettings: f})

	rec := doBody(t, s, "GET", "/api/settings/ai", "")
	want := `{"rec_enabled":true,"rec_model":"claude-opus-5-5","rec_schedule":"nightly","monthly_budget_usd":15,` +
		`"classifier_enabled":true,"classifier_model":"claude-haiku-5-5","auto_apply_high_confidence":true}`
	if rec.Code != 200 || rec.Body.String() != want {
		t.Errorf("GET = %d %s, want %s", rec.Code, rec.Body, want)
	}

	// Partial: what's sent, nothing else; unknown fields are ignored
	rec = doBody(t, s, "PUT", "/api/settings/ai", `{"rec_schedule":"weekly","monthly_budget_usd":null,"rec_schedule_cron":"0 4 * * *"}`)
	var saved recommend.Settings
	if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &saved) != nil || saved.RecSchedule != "weekly" || saved.MonthlyBudgetUSD != nil || saved.RecModel != recommend.ModelOpus {
		t.Errorf("PUT = %d %s", rec.Code, rec.Body)
	}
	if p := f.patches[0]; !p.SetBudget || p.MonthlyBudgetUSD != nil || *p.RecSchedule != "weekly" || p.RecModel != nil || p.RecEnabled != nil {
		t.Errorf("patch = %+v", p)
	}

	rec = doBody(t, s, "GET", "/api/settings/ai/usage", "")
	var usage struct {
		ByFeature []recommend.FeatureUsage   `json:"by_feature"`
		Estimates map[string]json.RawMessage `json:"estimates"`
	}
	if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &usage) != nil || usage.ByFeature == nil ||
		string(usage.Estimates["run_usd"]) != `{"claude-opus-5-5":null,"claude-sonnet-5-5":null}` {
		t.Errorf("usage = %d %s", rec.Code, rec.Body)
	}
}

func TestParseAISettingsPatch(t *testing.T) {
	for _, tt := range []struct {
		body  string
		field string // "" when valid
	}{
		{`{}`, ""},
		{`{"rec_enabled":false,"classifier_enabled":true,"auto_apply_high_confidence":false}`, ""},
		{`{"rec_model":"claude-sonnet-5-5","classifier_model":"claude-opus-5-5","rec_schedule":"manual"}`, ""},
		{`{"monthly_budget_usd":0}`, ""},
		{`{"monthly_budget_usd":12.5}`, ""},
		{`{"monthly_budget_usd":null}`, ""},
		{`{"rec_enabled":"yes"}`, "rec_enabled"},
		{`{"rec_enabled":null}`, "rec_enabled"},
		{`{"rec_model":"claude-haiku-5-5"}`, "rec_model"},
		{`{"rec_schedule":"0 4 * * *"}`, "rec_schedule"},
		{`{"monthly_budget_usd":-1}`, "monthly_budget_usd"},
		{`{"monthly_budget_usd":"15"}`, "monthly_budget_usd"},
		{`{"classifier_model":"claude-fable-5-1"}`, "classifier_model"},
		{`{"auto_apply_high_confidence":1}`, "auto_apply_high_confidence"},
		// The first bad field, in the documented order
		{`{"classifier_model":"x","rec_model":"y"}`, "rec_model"},
	} {
		_, err := parseAISettingsPatch([]byte(tt.body))
		var field *recommend.FieldError
		switch {
		case tt.field == "" && err != nil:
			t.Errorf("%s: %v", tt.body, err)
		case tt.field != "" && (!errors.As(err, &field) || field.Field != tt.field):
			t.Errorf("%s: %v, want a FieldError on %s", tt.body, err, tt.field)
		}
	}
	for _, body := range []string{`[]`, `"x"`, `null`, `not json`} {
		if _, err := parseAISettingsPatch([]byte(body)); !errors.Is(err, errNotAnObject) {
			t.Errorf("%s: %v, want errNotAnObject", body, err)
		}
	}
}

func TestAISettingsRoutes_Errors(t *testing.T) {
	s := newTestServer(Options{AISettings: &fakeAISettings{settings: freshAISettings()}})
	rec := doBody(t, s, "PUT", "/api/settings/ai", `{"rec_model":"gpt-5"}`)
	if e := errorOf(t, rec.Body.Bytes()); rec.Code != 422 || e.Code != CodeValidationError || e.Field != "rec_model" || e.Message != "rec_model must be claude-opus-5-5 or claude-sonnet-5-5" {
		t.Errorf("422 = %d %s", rec.Code, rec.Body)
	}
	if rec := doBody(t, s, "PUT", "/api/settings/ai", `[1]`); rec.Code != 400 {
		t.Errorf("not an object = %d, want 400", rec.Code)
	}

	s = newTestServer(Options{AISettings: &fakeAISettings{err: errors.New("db down")}})
	for _, path := range []string{"/api/settings/ai", "/api/settings/ai/usage"} {
		if rec := doBody(t, s, "GET", path, ""); rec.Code != 500 {
			t.Errorf("%s = %d, want 500", path, rec.Code)
		}
	}
}
