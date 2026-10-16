package aicore

import (
	"context"
	"fmt"
	"time"

	"github.com/shapedtime/momoshtrem/internal/library"
)

// Recorder stores what each API call cost: one ai_usage row per call,
// under a feature (library.AIFeature*) and the job, run or turn it was for
// (ref; 0 for none).
type Recorder interface {
	Record(ctx context.Context, feature string, ref int64, u Usage) error
}

// Budget guards the monthly AI budget every feature shares. Check returns
// an error wrapping ErrBudgetExceeded (a *BudgetError) once this month's
// spend has reached monthly_budget_usd. It's checked before each run, Ask
// turn and classifier AI run, so one in flight can overshoot the budget by
// its own cost.
type Budget interface {
	Check(ctx context.Context) error
}

// UsageStore is where usage lands (library.AIStore).
type UsageStore interface {
	RecordUsage(ctx context.Context, u library.AIUsage) error
	// CostSince is every feature's spend since a time.
	CostSince(ctx context.Context, since time.Time) (float64, error)
}

// BudgetSettings reads monthly_budget_usd (library.SettingsRepository).
type BudgetSettings interface {
	AISettings(ctx context.Context) (library.AISettings, error)
}

// Ledger records usage in ai_usage and checks the monthly budget against
// it. The month is server-local (time.Local, the service's TZ).
type Ledger struct {
	store    UsageStore
	settings BudgetSettings
	now      func() time.Time
}

var (
	_ Recorder = (*Ledger)(nil)
	_ Budget   = (*Ledger)(nil)
)

// NewLedger creates a ledger over the usage store and the AI settings.
func NewLedger(store UsageStore, settings BudgetSettings) *Ledger {
	return &Ledger{store: store, settings: settings, now: time.Now}
}

// Record stores one API call's usage. ai_usage has no cache-write column,
// so cache writes count as input tokens; the cost has them at their own
// rate.
func (l *Ledger) Record(ctx context.Context, feature string, ref int64, u Usage) error {
	var refID *int64
	if ref != 0 {
		refID = &ref
	}
	return l.store.RecordUsage(ctx, library.AIUsage{
		Feature: feature, Model: u.Model,
		InputTokens: int(u.Input + u.CacheWrite), OutputTokens: int(u.Output),
		CacheReadTokens: int(u.CacheRead), WebSearches: int(u.WebSearches),
		CostUSD: u.CostUSD, RefID: refID,
	})
}

// Check fails with a *BudgetError once this month's spend reaches
// monthly_budget_usd. Without a budget (an explicit null) there's no limit;
// when the settings or the spend can't be read, the call is refused.
func (l *Ledger) Check(ctx context.Context) error {
	settings, err := l.settings.AISettings(ctx)
	if err != nil {
		return fmt.Errorf("failed to read the AI budget: %w", err)
	}
	if settings.MonthlyBudgetUSD == nil {
		return nil
	}
	start, next := MonthBounds(l.now())
	spent, err := l.store.CostSince(ctx, start)
	if err != nil {
		return err
	}
	if budget := *settings.MonthlyBudgetUSD; spent >= budget {
		return &BudgetError{SpentUSD: spent, BudgetUSD: budget, ResetsAt: next}
	}
	return nil
}

// MonthToDate is every AI feature's spend this server-local month
// (/status's ai.mtd_cost_usd).
func (l *Ledger) MonthToDate(ctx context.Context) (float64, error) {
	start, _ := MonthBounds(l.now())
	return l.store.CostSince(ctx, start)
}
