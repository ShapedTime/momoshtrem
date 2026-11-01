package compare

import (
	"testing"
	"time"

	"github.com/shapedtime/momoshtrem/internal/streamlab/report"
)

// Each threshold rule on both sides of its boundary.

// Timings: past max(15%, the floor), and Mann-Whitney's two-sided p <
// Alpha. At 5 against 5, two of head's reps below a base rep (U = 2) is
// p 0.032, three is 0.056.
func TestTimingBoundaries(t *testing.T) {
	timing := func(v ...float64) *fixture { return newFixture(len(v)).timing("activation_wait", v...) }
	head := timing(1500, 1501, 1502, 1503, 1504)
	for _, tc := range []struct {
		top  float64 // base's slowest rep
		want MetricStatus
	}{
		{1501.5, Worse},  // U = 2: p 0.032
		{1502.5, Unsure}, // U = 3: p 0.056
	} {
		r := judge(t, timing(1000, 1001, 1002, 1003, tc.top), head, Options{})
		if m := metric(r, "activation_wait"); m.Status != tc.want {
			t.Errorf("base's slowest at %v: %+v, want %s", tc.top, m, tc.want)
		}
	}
	// Base at 1000 on every rep: the threshold is 15%, 150 ms. Exactly
	// at it is the same; 140 under it, 160 past it.
	for shift, want := range map[float64]MetricStatus{140: Same, 150: Same, 160: Worse} {
		h := 1000 + shift
		r := judge(t, timing(1000, 1000, 1000, 1000, 1000), timing(h, h, h, h, h), Options{})
		if m := metric(r, "activation_wait"); m.Status != want || m.Threshold != 150 {
			t.Errorf("+%v ms: %+v, want %s", shift, m, want)
		}
	}
	// Under 667 ms the floor (100 ms) is the larger.
	r := judge(t, timing(steady(400, 5)...), timing(steady(495, 5)...), Options{})
	if m := metric(r, "activation_wait"); m.Status != Same || m.Threshold != 100 {
		t.Errorf("under the floor: %+v", m)
	}
}

// Counts: past max(10%, 3×CV, the unit's floor); the default CV (0.10)
// makes 3×CV the larger.
func TestCountBoundaries(t *testing.T) {
	count := func(unit string, v ...float64) *fixture {
		return newFixture(len(v)).metric("waste", report.Count, unit, report.Lower, v...)
	}
	all := func(v float64) []float64 { return []float64{v, v, v, v, v} }
	// The default CV: 3 × 0.10 × 100 = 30.
	for head, want := range map[float64]MetricStatus{120: Same, 130: Same, 131: Worse} {
		r := judge(t, count("reads", all(100)...), count("reads", all(head)...), Options{})
		if m := metric(r, "waste"); m.Status != want || !near(m.Threshold, 30, 1e-9) {
			t.Errorf("default CV, 100 → %v: %+v, want %s", head, m, want)
		}
	}
	// Calibrated quiet (CV 0.01): 10% is the larger.
	cal := &Calibration{Commit: "c", Date: time.Now(), Reps: 10, CV: map[string]map[string]float64{"cold-start": {"waste": 0.01}}}
	for head, want := range map[float64]MetricStatus{109: Same, 110: Same, 111: Worse} {
		r := judge(t, count("reads", all(100)...), count("reads", all(head)...), Options{Noise: cal})
		if m := metric(r, "waste"); m.Status != want || !near(m.Threshold, 10, 1e-9) {
			t.Errorf("calibrated, 100 → %v: %+v, want %s", head, m, want)
		}
	}
	// The unit's floor: a MiB for bytes.
	for head, want := range map[float64]MetricStatus{1000: Same, 2 << 20: Worse} {
		r := judge(t, count("bytes", all(0)...), count("bytes", all(head)...), Options{})
		if m := metric(r, "waste"); m.Status != want || m.Threshold != 1<<20 {
			t.Errorf("bytes, 0 → %v: %+v, want %s", head, m, want)
		}
	}
	// Under skewed load a count change is inconclusive, as a timing's is.
	h := count("reads", all(200)...)
	h.load = 3.0
	if m := metric(judge(t, count("reads", all(100)...), h, Options{}), "waste"); m.Status != Unsure {
		t.Errorf("skewed load: %+v", m)
	}
}

// Load skew needs both: more than 0.5 per CPU apart, and more than 1.5
// times.
func TestLoadSkew(t *testing.T) {
	for _, tc := range []struct {
		base, head float64
		want       bool
	}{
		{0.2, 1.0, true},
		{1.0, 0.2, true},
		{0.1, 0.55, false}, // 5.5 times, 0.45 apart
		{2.0, 2.6, false},  // 0.6 apart, 1.3 times
		{2.0, 3.1, true},
		{0, 0.6, true}, // near idle: the ratio against 0.1
		{0, 0.4, false},
	} {
		if got, _ := loadSkew(tc.base, tc.head); got != tc.want {
			t.Errorf("loadSkew(%v, %v) = %v", tc.base, tc.head, got)
		}
	}
}

// An intermittent issue is over its rate on every rep, or on two or more
// and more than its rate (25%) of them.
func TestOverRate(t *testing.T) {
	for _, tc := range []struct {
		fired, reps int
		want        bool
	}{
		{1, 1, true}, {1, 2, false}, {2, 8, false}, {3, 8, true}, {2, 4, true}, {0, 5, false}, {0, 0, false},
	} {
		if got := overRate(tally{fired: tc.fired, reps: tc.reps}, 0.25); got != tc.want {
			t.Errorf("%d of %d: %v", tc.fired, tc.reps, got)
		}
	}
}
