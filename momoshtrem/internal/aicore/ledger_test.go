package aicore

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shapedtime/momoshtrem/internal/library"
)

type fakeStore struct {
	rows  []library.AIUsage
	since time.Time
	spent float64
	err   error
}

func (f *fakeStore) RecordUsage(_ context.Context, u library.AIUsage) error {
	f.rows = append(f.rows, u)
	return f.err
}

func (f *fakeStore) CostSince(_ context.Context, since time.Time) (float64, error) {
	f.since = since
	return f.spent, f.err
}

type fakeSettings struct {
	settings library.AISettings
	err      error
}

func (f fakeSettings) AISettings(context.Context) (library.AISettings, error) {
	return f.settings, f.err
}

func budget(usd float64) library.AISettings {
	s := library.DefaultAISettings()
	s.MonthlyBudgetUSD = &usd
	return s
}

func TestLedger_Check(t *testing.T) {
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Skip("no tzdata:", err)
	}
	now := time.Date(2026, 10, 31, 23, 30, 0, 0, berlin) // already November in UTC
	ctx := context.Background()
	newLedger := func(store *fakeStore, s fakeSettings) *Ledger {
		l := NewLedger(store, s)
		l.now = func() time.Time { return now }
		return l
	}

	// The default is $15 a month, on the server-local month.
	store := &fakeStore{spent: 14.99}
	l := newLedger(store, fakeSettings{settings: library.DefaultAISettings()})
	if err := l.Check(ctx); err != nil {
		t.Errorf("under the default budget: %v", err)
	}
	if want := time.Date(2026, 10, 1, 0, 0, 0, 0, berlin); !store.since.Equal(want) {
		t.Errorf("month starts %v, want %v", store.since, want)
	}

	store.spent = 15
	err = l.Check(ctx)
	var be *BudgetError
	if !errors.As(err, &be) || !errors.Is(err, ErrBudgetExceeded) || be.BudgetUSD != 15 || be.SpentUSD != 15 ||
		!be.ResetsAt.Equal(time.Date(2026, 11, 1, 0, 0, 0, 0, berlin)) {
		t.Fatalf("at the budget: %v (%+v)", err, be)
	}
	if err.Error() != "this month's AI budget is spent: $15.00 of $15.00" {
		t.Errorf("message = %q", err.Error())
	}

	// An explicit null is no cap; a zero budget allows nothing.
	none := library.DefaultAISettings()
	none.MonthlyBudgetUSD = nil
	store.spent = 1000
	if err := newLedger(store, fakeSettings{settings: none}).Check(ctx); err != nil {
		t.Errorf("no cap: %v", err)
	}
	store.spent = 0
	if err := newLedger(store, fakeSettings{settings: budget(0)}).Check(ctx); !errors.Is(err, ErrBudgetExceeded) {
		t.Errorf("a zero budget: %v", err)
	}

	// Unreadable settings or spend refuse the call.
	if err := newLedger(store, fakeSettings{err: errors.New("db down")}).Check(ctx); err == nil {
		t.Error("unreadable settings allowed a call")
	}
	if err := newLedger(&fakeStore{err: errors.New("db down")}, fakeSettings{settings: budget(5)}).Check(ctx); err == nil {
		t.Error("an unreadable spend allowed a call")
	}

	// MonthToDate is the same month.
	store = &fakeStore{spent: 2.5}
	if got, err := newLedger(store, fakeSettings{}).MonthToDate(ctx); got != 2.5 || err != nil || !store.since.Equal(time.Date(2026, 10, 1, 0, 0, 0, 0, berlin)) {
		t.Errorf("MonthToDate = %v, %v since %v", got, err, store.since)
	}
}

func TestLedger_Record(t *testing.T) {
	store := &fakeStore{}
	l := NewLedger(store, fakeSettings{})
	u := Usage{Model: "claude-opus-5-5", Tokens: Tokens{Input: 100, CacheWrite: 50, CacheRead: 2000, Output: 30}, WebSearches: 2, CostUSD: 0.6}
	if err := l.Record(context.Background(), library.AIFeatureRecs, 9, u); err != nil {
		t.Fatal(err)
	}
	if err := l.Record(context.Background(), library.AIFeatureAsk, 0, u); err != nil {
		t.Fatal(err)
	}
	got := store.rows[0]
	if got.Feature != "recs" || got.Model != "claude-opus-5-5" || got.InputTokens != 150 || got.CacheReadTokens != 2000 ||
		got.OutputTokens != 30 || got.WebSearches != 2 || got.CostUSD != 0.6 || got.RefID == nil || *got.RefID != 9 {
		t.Errorf("row = %+v", got)
	}
	if store.rows[1].RefID != nil {
		t.Errorf("ref 0 stored as %v", *store.rows[1].RefID)
	}
}

func TestBounds(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip("no tzdata:", err)
	}
	for _, tt := range []struct {
		now                              time.Time
		monthStart, monthNext, day, next time.Time
	}{
		{
			now:        time.Date(2026, 12, 31, 23, 59, 0, 0, ny),
			monthStart: time.Date(2026, 12, 1, 0, 0, 0, 0, ny), monthNext: time.Date(2027, 1, 1, 0, 0, 0, 0, ny),
			day: time.Date(2026, 12, 31, 0, 0, 0, 0, ny), next: time.Date(2027, 1, 1, 0, 0, 0, 0, ny),
		},
		{ // the day clocks fall back is 25 hours long
			now:        time.Date(2026, 11, 1, 12, 0, 0, 0, ny),
			monthStart: time.Date(2026, 11, 1, 0, 0, 0, 0, ny), monthNext: time.Date(2026, 12, 1, 0, 0, 0, 0, ny),
			day: time.Date(2026, 11, 1, 0, 0, 0, 0, ny), next: time.Date(2026, 11, 2, 0, 0, 0, 0, ny),
		},
	} {
		start, next := MonthBounds(tt.now)
		if !start.Equal(tt.monthStart) || !next.Equal(tt.monthNext) {
			t.Errorf("MonthBounds(%v) = %v, %v", tt.now, start, next)
		}
		day, dayNext := DayBounds(tt.now)
		if !day.Equal(tt.day) || !dayNext.Equal(tt.next) {
			t.Errorf("DayBounds(%v) = %v, %v", tt.now, day, dayNext)
		}
	}
	if _, next := DayBounds(time.Date(2026, 11, 1, 12, 0, 0, 0, ny)); next.Sub(time.Date(2026, 11, 1, 0, 0, 0, 0, ny)) != 25*time.Hour {
		t.Error("the fall-back day isn't 25 hours")
	}
}
