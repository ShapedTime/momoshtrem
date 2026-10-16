package aisettings

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/recommend"
)

type fakeSettings struct {
	settings library.AISettings
	patches  []library.AISettingsPatch
	err      error
}

func (f *fakeSettings) AISettings(context.Context) (library.AISettings, error) {
	return f.settings, f.err
}

func (f *fakeSettings) UpdateAISettings(_ context.Context, p library.AISettingsPatch) (library.AISettings, error) {
	f.patches = append(f.patches, p)
	if p.RecSchedule != nil {
		f.settings.RecSchedule = *p.RecSchedule
	}
	if p.SetBudget {
		f.settings.MonthlyBudgetUSD = p.MonthlyBudgetUSD
	}
	return f.settings, f.err
}

type fakeUsage struct {
	totals []library.AIFeatureTotal
	since  time.Time
	err    error
}

func (f *fakeUsage) FeatureTotalsSince(_ context.Context, since time.Time) ([]library.AIFeatureTotal, error) {
	f.since = since
	return f.totals, f.err
}

type fakeRecs struct {
	usage recommend.RecsUsage
	est   map[string]*float64
	err   error
}

func (f fakeRecs) RecsUsage(context.Context) (recommend.RecsUsage, error)    { return f.usage, f.err }
func (f fakeRecs) RunEstimates(context.Context) (map[string]*float64, error) { return f.est, f.err }

type fakeAsk struct {
	est      *float64
	today    int
	dayStart time.Time
	err      error
}

func (f *fakeAsk) AskEstimate(context.Context) (*float64, error) { return f.est, f.err }
func (f *fakeAsk) AsksToday(_ context.Context, dayStart time.Time) (int, error) {
	f.dayStart = dayStart
	return f.today, f.err
}

func ptr[T any](v T) *T { return &v }

func TestSettings(t *testing.T) {
	store := &fakeSettings{settings: library.DefaultAISettings()}
	s := New(store, &fakeUsage{}, true)
	got, err := s.AISettings(context.Background())
	want := recommend.Settings{
		RecEnabled: true, RecModel: recommend.ModelOpus, RecSchedule: recommend.ScheduleNightly, MonthlyBudgetUSD: ptr(15.0),
		ClassifierEnabled: true, ClassifierModel: recommend.ModelHaiku, AutoApplyHighConfidence: true,
	}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("AISettings = %+v, %v", got, err)
	}

	patch := recommend.SettingsPatch{
		RecEnabled: ptr(false), RecModel: ptr("claude-sonnet-5-5"), RecSchedule: ptr("weekly"), SetBudget: true,
		ClassifierEnabled: ptr(false), ClassifierModel: ptr("claude-opus-5-5"), AutoApplyHighConfidence: ptr(false),
	}
	got, err = s.UpdateAISettings(context.Background(), patch)
	if err != nil || got.RecSchedule != "weekly" || got.MonthlyBudgetUSD != nil {
		t.Errorf("UpdateAISettings = %+v, %v", got, err)
	}
	wantPatch := library.AISettingsPatch{
		RecEnabled: patch.RecEnabled, RecModel: patch.RecModel, RecSchedule: patch.RecSchedule, SetBudget: true,
		ClassifierEnabled: patch.ClassifierEnabled, ClassifierModel: patch.ClassifierModel, AutoApplyHighConfidence: patch.AutoApplyHighConfidence,
	}
	if !reflect.DeepEqual(store.patches, []library.AISettingsPatch{wantPatch}) {
		t.Errorf("patch = %+v", store.patches)
	}

	store.err = errors.New("db down")
	if _, err := s.AISettings(context.Background()); err == nil {
		t.Error("a store error was lost")
	}
}

func TestUsage(t *testing.T) {
	tokyo, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		t.Skip("no tzdata:", err)
	}
	now := time.Date(2026, 11, 1, 1, 30, 0, 0, tokyo) // still October in UTC
	ctx := context.Background()
	newService := func(settings library.AISettings, usage *fakeUsage) *Service {
		s := New(&fakeSettings{settings: settings}, usage, true)
		s.now = func() time.Time { return now }
		return s
	}

	// Without the picks and Ask wired: all three features, nulls and zeros.
	usage := &fakeUsage{totals: []library.AIFeatureTotal{
		{Feature: "classifier", CostUSD: 0.1 + 0.2, Requests: 12},
		{Feature: "recs", CostUSD: 14.7, Requests: 30},
	}}
	s := newService(library.DefaultAISettings(), usage)
	got, err := s.AIUsage(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := &recommend.UsageReport{
		Configured: true, Month: "2026-11", ResetsAt: time.Date(2026, 12, 1, 0, 0, 0, 0, tokyo),
		TotalUSD: 15, BudgetUSD: ptr(15.0), BudgetReached: true,
		ByFeature: []recommend.FeatureUsage{
			{Feature: "recs", CostUSD: 14.7, Requests: 30},
			{Feature: "ask", CostUSD: 0, Requests: 0},
			{Feature: "classifier", CostUSD: 0.3, Requests: 12},
		},
		Estimates:     recommend.Estimates{RunUSD: map[string]*float64{}},
		AskDailyLimit: 30,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("usage =\n%+v\nwant\n%+v", got, want)
	}
	if !usage.since.Equal(time.Date(2026, 11, 1, 0, 0, 0, 0, tokyo)) {
		t.Errorf("month starts %v", usage.since)
	}

	// No cap: never reached.
	none := library.DefaultAISettings()
	none.MonthlyBudgetUSD = nil
	if got, _ := newService(none, usage).AIUsage(ctx); got.BudgetReached || got.BudgetUSD != nil {
		t.Errorf("no cap: reached %v, budget %v", got.BudgetReached, got.BudgetUSD)
	}

	// With the reporters: their parts, and today's asks since local midnight.
	last := &recommend.RunSummary{ID: 4, Kind: "scheduled", Status: "succeeded"}
	s.SetRecs(fakeRecs{
		usage: recommend.RecsUsage{LastRun: last},
		est:   map[string]*float64{recommend.ModelOpus: ptr(0.36)},
	})
	ask := &fakeAsk{est: ptr(0.02), today: 4}
	s.SetAsk(ask)
	got, err = s.AIUsage(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got.Recs.LastRun != last || *got.Estimates.RunUSD[recommend.ModelOpus] != 0.36 || *got.Estimates.AskUSD != 0.02 || got.AsksToday != 4 {
		t.Errorf("with reporters = %+v", got)
	}
	if !ask.dayStart.Equal(time.Date(2026, 11, 1, 0, 0, 0, 0, tokyo)) {
		t.Errorf("asks counted since %v", ask.dayStart)
	}

	// A failing reporter leaves its part null; the page still loads.
	s.SetRecs(fakeRecs{err: errors.New("boom")})
	s.SetAsk(&fakeAsk{err: errors.New("boom")})
	got, err = s.AIUsage(ctx)
	if err != nil || got.Recs.LastRun != nil || got.Estimates.AskUSD != nil || got.AsksToday != 0 {
		t.Errorf("failing reporters = %+v, %v", got, err)
	}

	// The store's errors fail the request.
	if _, err := newService(library.DefaultAISettings(), &fakeUsage{err: errors.New("db down")}).AIUsage(ctx); err == nil {
		t.Error("a usage error was lost")
	}
}

// A successful update calls the change hook (the picks reschedule, #157);
// a failed one doesn't.
func TestSetOnChange(t *testing.T) {
	store := &fakeSettings{settings: library.DefaultAISettings()}
	s := New(store, &fakeUsage{}, true)
	if _, err := s.UpdateAISettings(context.Background(), recommend.SettingsPatch{RecSchedule: ptr("weekly")}); err != nil {
		t.Fatal(err) // no hook yet
	}
	changes := 0
	s.SetOnChange(func() { changes++ })
	if _, err := s.UpdateAISettings(context.Background(), recommend.SettingsPatch{RecSchedule: ptr("manual")}); err != nil || changes != 1 {
		t.Errorf("update = %v, changes %d", err, changes)
	}
	store.err = errors.New("db down")
	if _, err := s.UpdateAISettings(context.Background(), recommend.SettingsPatch{RecSchedule: ptr("nightly")}); err == nil || changes != 1 {
		t.Errorf("failed update = %v, changes %d", err, changes)
	}
}
