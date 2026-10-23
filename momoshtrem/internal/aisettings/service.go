// Package aisettings serves the AI settings and their usage (PLAN §6 "AI
// settings and usage"; the AI core, #154): GET/PUT /settings/ai on
// app_settings key "ai", shared with the classifier, and GET
// /settings/ai/usage from ai_usage, for the server-local month. Both work
// without ANTHROPIC_API_KEY.
//
// The usage page's picks and Ask parts come from reporters the slices that
// own them set: the picks (R2, #157) with SetRecs, Ask (R3, #158) with
// SetAsk. Until then recs and the estimates are null and asks_today is 0.
package aisettings

import (
	"context"
	"log/slog"
	"math"
	"sync"
	"time"

	"github.com/shapedtime/momoshtrem/internal/aicore"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/recommend"
)

// SettingsStore reads and writes the "ai" document
// (library.SettingsRepository).
type SettingsStore interface {
	AISettings(ctx context.Context) (library.AISettings, error)
	UpdateAISettings(ctx context.Context, patch library.AISettingsPatch) (library.AISettings, error)
}

// UsageStore sums ai_usage (library.AIStore): by feature, and the picks
// and Ask by household member.
type UsageStore interface {
	FeatureTotalsSince(ctx context.Context, since time.Time) ([]library.AIFeatureTotal, error)
	MemberSpendSince(ctx context.Context, since time.Time) ([]library.AIMemberTotal, error)
}

// RecsReporter is the picks' part of the usage page (R2, #157).
type RecsReporter interface {
	// RecsUsage is the context's member's newest finished run (succeeded,
	// failed or skipped), their queued or running run, and the schedule's
	// next run (nil when manual, off, not configured, or without their
	// Trakt).
	RecsUsage(ctx context.Context) (recommend.RecsUsage, error)
	// RunEstimates is, per rec model, the mean cost of its last 5
	// succeeded runs; a model without any is nil or missing.
	RunEstimates(ctx context.Context) (map[string]*float64, error)
}

// AskReporter is Ask's part of the usage page (R3, #158).
type AskReporter interface {
	// AskEstimate is the mean cost of the last 20 done turns; nil
	// without any.
	AskEstimate(ctx context.Context) (*float64, error)
	// AsksToday counts the context's member's turns started since
	// dayStart, the server-local midnight: cancelled and failed ones too.
	AsksToday(ctx context.Context, dayStart time.Time) (int, error)
}

// Service implements api.AISettings.
type Service struct {
	settings   SettingsStore
	usage      UsageStore
	configured bool
	now        func() time.Time
	log        *slog.Logger

	mu       sync.RWMutex
	recs     RecsReporter
	ask      AskReporter
	onChange func()
}

// New serves the settings and the usage; configured says whether
// ANTHROPIC_API_KEY is set.
func New(settings SettingsStore, usage UsageStore, configured bool) *Service {
	return &Service{
		settings: settings, usage: usage, configured: configured, now: time.Now,
		log: slog.With("component", "ai-settings"),
	}
}

// SetRecs installs the picks' reporter (#157).
func (s *Service) SetRecs(r RecsReporter) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.recs = r
}

// SetOnChange installs a function called after each successful update:
// the picks' scheduler plans again at once (#157).
func (s *Service) SetOnChange(fn func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onChange = fn
}

// SetAsk installs Ask's reporter (#158).
func (s *Service) SetAsk(a AskReporter) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ask = a
}

// AISettings reads the document over its defaults.
func (s *Service) AISettings(ctx context.Context) (recommend.Settings, error) {
	settings, err := s.settings.AISettings(ctx)
	if err != nil {
		return recommend.Settings{}, err
	}
	return toWire(settings), nil
}

// UpdateAISettings applies the fields set, in one jsonb || statement, and
// answers every setting. The API has checked the values.
func (s *Service) UpdateAISettings(ctx context.Context, p recommend.SettingsPatch) (recommend.Settings, error) {
	settings, err := s.settings.UpdateAISettings(ctx, library.AISettingsPatch{
		RecEnabled:              p.RecEnabled,
		RecModel:                p.RecModel,
		RecSchedule:             p.RecSchedule,
		SetBudget:               p.SetBudget,
		MonthlyBudgetUSD:        p.MonthlyBudgetUSD,
		ClassifierEnabled:       p.ClassifierEnabled,
		ClassifierModel:         p.ClassifierModel,
		AutoApplyHighConfidence: p.AutoApplyHighConfidence,
	})
	if err != nil {
		return recommend.Settings{}, err
	}
	s.mu.RLock()
	onChange := s.onChange
	s.mu.RUnlock()
	if onChange != nil {
		onChange()
	}
	return toWire(settings), nil
}

func toWire(s library.AISettings) recommend.Settings {
	return recommend.Settings{
		RecEnabled:              s.RecEnabled,
		RecModel:                s.RecModel,
		RecSchedule:             s.RecSchedule,
		MonthlyBudgetUSD:        s.MonthlyBudgetUSD,
		ClassifierEnabled:       s.ClassifierEnabled,
		ClassifierModel:         s.ClassifierModel,
		AutoApplyHighConfidence: s.AutoApplyHighConfidence,
	}
}

// features are by_feature's entries, always all three, in this order.
var features = []string{library.AIFeatureRecs, library.AIFeatureAsk, library.AIFeatureClassifier}

// AIUsage is this server-local month's spend by feature against the
// budget, and the picks and Ask by member, with the picks' and Ask's
// parts (the context's member's) when their reporters are set. A reporter
// that fails leaves its part null (logged): the page still loads.
func (s *Service) AIUsage(ctx context.Context) (*recommend.UsageReport, error) {
	now := s.now()
	monthStart, next := aicore.MonthBounds(now)
	settings, err := s.settings.AISettings(ctx)
	if err != nil {
		return nil, err
	}
	totals, err := s.usage.FeatureTotalsSince(ctx, monthStart)
	if err != nil {
		return nil, err
	}
	members, err := s.usage.MemberSpendSince(ctx, monthStart)
	if err != nil {
		return nil, err
	}

	report := &recommend.UsageReport{
		Configured:    s.configured,
		Month:         monthStart.Format("2006-01"),
		ResetsAt:      next,
		BudgetUSD:     settings.MonthlyBudgetUSD,
		AskDailyLimit: recommend.AskDailyLimit,
		Estimates:     recommend.Estimates{RunUSD: map[string]*float64{}},
	}
	byFeature := map[string]library.AIFeatureTotal{}
	var total float64
	for _, t := range totals {
		byFeature[t.Feature] = t
		total += t.CostUSD
	}
	for _, f := range features {
		report.ByFeature = append(report.ByFeature, recommend.FeatureUsage{
			Feature: f, CostUSD: roundUSD(byFeature[f].CostUSD), Requests: byFeature[f].Requests,
		})
	}
	report.TotalUSD = roundUSD(total)
	report.BudgetReached = settings.MonthlyBudgetUSD != nil && total >= *settings.MonthlyBudgetUSD
	for _, m := range members {
		report.ByMember = append(report.ByMember, recommend.MemberUsage{
			MemberID: m.MemberID, Name: m.Name, CostUSD: roundUSD(m.CostUSD), Requests: m.Requests,
		})
	}

	s.mu.RLock()
	recs, ask := s.recs, s.ask
	s.mu.RUnlock()
	if recs != nil {
		if r, err := recs.RecsUsage(ctx); err != nil {
			s.log.Warn("Usage page: the picks' part failed", "error", err)
		} else {
			report.Recs = r
		}
		if est, err := recs.RunEstimates(ctx); err != nil {
			s.log.Warn("Usage page: run estimates failed", "error", err)
		} else {
			for model, usd := range est {
				report.Estimates.RunUSD[model] = usd
			}
		}
	}
	if ask != nil {
		if est, err := ask.AskEstimate(ctx); err != nil {
			s.log.Warn("Usage page: the Ask estimate failed", "error", err)
		} else {
			report.Estimates.AskUSD = est
		}
		dayStart, _ := aicore.DayBounds(now)
		if n, err := ask.AsksToday(ctx, dayStart); err != nil {
			s.log.Warn("Usage page: today's asks failed", "error", err)
		} else {
			report.AsksToday = n
		}
	}
	return report, nil
}

// roundUSD keeps dollars to a millionth (ai_usage's numeric sums come back
// as floats: 0.30000000000000004).
func roundUSD(v float64) float64 { return math.Round(v*1e6) / 1e6 }
