package api

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/shapedtime/momoshtrem/internal/aicore"
	"github.com/shapedtime/momoshtrem/internal/aisettings"
	"github.com/shapedtime/momoshtrem/internal/library"
)

// /settings/ai end to end: the routes over the AI core's settings service
// and Postgres (#154). A partial PUT changes only what it sends, a bad
// value is 422 naming the field and changes nothing, and the usage page
// sums this server-local month's ai_usage.
func TestAISettings_Integration(t *testing.T) {
	db := openRerunDB(t)
	store := library.NewAIStore(db)
	s := newTestServer(Options{AISettings: aisettings.New(library.NewSettingsRepository(db), store, true)})

	defaults := `{"rec_enabled":true,"rec_model":"claude-opus-5-5","rec_schedule":"nightly","monthly_budget_usd":15,` +
		`"classifier_enabled":true,"classifier_model":"claude-haiku-5-5","auto_apply_high_confidence":true}`
	if rec := doBody(t, s, "GET", "/api/settings/ai", ""); rec.Code != 200 || rec.Body.String() != defaults {
		t.Fatalf("GET = %d %s, want %s", rec.Code, rec.Body, defaults)
	}

	// Partial: what's sent, nothing else; unknown fields are ignored.
	rec := doBody(t, s, "PUT", "/api/settings/ai", `{"rec_schedule":"weekly","classifier_enabled":false,"rec_schedule_cron":"0 4 * * *"}`)
	want := `{"rec_enabled":true,"rec_model":"claude-opus-5-5","rec_schedule":"weekly","monthly_budget_usd":15,` +
		`"classifier_enabled":false,"classifier_model":"claude-haiku-5-5","auto_apply_high_confidence":true}`
	if rec.Code != 200 || rec.Body.String() != want {
		t.Fatalf("PUT = %d %s, want %s", rec.Code, rec.Body, want)
	}
	rec = doBody(t, s, "PUT", "/api/settings/ai", `{"monthly_budget_usd":null}`)
	want = `{"rec_enabled":true,"rec_model":"claude-opus-5-5","rec_schedule":"weekly","monthly_budget_usd":null,` +
		`"classifier_enabled":false,"classifier_model":"claude-haiku-5-5","auto_apply_high_confidence":true}`
	if rec.Code != 200 || rec.Body.String() != want {
		t.Fatalf("PUT null budget = %d %s", rec.Code, rec.Body)
	}

	// A bad value is 422 naming the first bad field, and changes nothing.
	rec = doBody(t, s, "PUT", "/api/settings/ai", `{"rec_enabled":false,"monthly_budget_usd":-3,"classifier_model":"gpt"}`)
	if e := errorOf(t, rec.Body.Bytes()); rec.Code != 422 || e.Code != CodeValidationError || e.Field != "monthly_budget_usd" {
		t.Errorf("422 = %d %s", rec.Code, rec.Body)
	}
	if rec := doBody(t, s, "PUT", "/api/settings/ai", `"weekly"`); rec.Code != 400 {
		t.Errorf("not an object = %d", rec.Code)
	}
	if rec := doBody(t, s, "GET", "/api/settings/ai", ""); rec.Code != 200 || rec.Body.String() != want {
		t.Errorf("after the rejected PUTs = %s, want %s", rec.Body, want)
	}

	// The classifier reads the same document.
	got, err := library.NewSettingsRepository(db).AISettings(context.Background())
	if err != nil || got.ClassifierEnabled || got.MonthlyBudgetUSD != nil {
		t.Errorf("classifier's view = %+v, %v", got, err)
	}

	// Usage: this month's rows, by feature, against the budget.
	ledger := aicore.NewLedger(store, library.NewSettingsRepository(db))
	for _, r := range []struct {
		feature string
		usd     float64
	}{{library.AIFeatureRecs, 0.36}, {library.AIFeatureClassifier, 0.0007}, {library.AIFeatureClassifier, 0.0003}} {
		if err := ledger.Record(context.Background(), r.feature, 1, aicore.Usage{Model: "claude-opus-5-5", CostUSD: r.usd}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO ai_usage (feature, model, cost_usd, created_at) VALUES ('ask', 'claude-opus-5-5', 5, NOW() - INTERVAL '40 days')`); err != nil {
		t.Fatal(err)
	}
	if rec := doBody(t, s, "PUT", "/api/settings/ai", `{"monthly_budget_usd":0.25}`); rec.Code != 200 {
		t.Fatalf("PUT budget = %d %s", rec.Code, rec.Body)
	}
	rec = doBody(t, s, "GET", "/api/settings/ai/usage", "")
	var usage map[string]json.RawMessage
	if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &usage) != nil {
		t.Fatalf("usage = %d %s", rec.Code, rec.Body)
	}
	start, next := aicore.MonthBounds(time.Now())
	checks := map[string]string{
		"configured":      `true`,
		"month":           `"` + start.Format("2006-01") + `"`,
		"resets_at":       string(mustMarshal(t, next)),
		"total_usd":       `0.361`,
		"budget_usd":      `0.25`,
		"budget_reached":  `true`,
		"by_feature":      `[{"feature":"recs","cost_usd":0.36,"requests":1},{"feature":"ask","cost_usd":0,"requests":0},{"feature":"classifier","cost_usd":0.001,"requests":2}]`,
		"recs":            `{"last_run":null,"active_run":null,"next_run_at":null}`,
		"estimates":       `{"run_usd":{"claude-opus-5-5":null,"claude-sonnet-5-5":null},"ask_usd":null}`,
		"asks_today":      `0`,
		"ask_daily_limit": `30`,
	}
	for field, want := range checks {
		if string(usage[field]) != want {
			t.Errorf("usage.%s = %s, want %s", field, usage[field], want)
		}
	}
	// The budget check agrees.
	if err := ledger.Check(context.Background()); err == nil {
		t.Error("the ledger allowed a call over the budget")
	}
}

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
