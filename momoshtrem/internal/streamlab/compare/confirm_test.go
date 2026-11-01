package compare

import (
	"slices"
	"strings"
	"testing"

	"github.com/shapedtime/momoshtrem/internal/streamlab/report"
)

func timings(base, head []float64) (*fixture, *fixture) {
	return newFixture(len(base)).timing("activation_wait", base...), newFixture(len(head)).timing("activation_wait", head...)
}

func judged(t *testing.T, base, head []float64) *Result {
	t.Helper()
	b, h := timings(base, head)
	return judge(t, b, h, Options{})
}

// A screen's regression holds only when each run shows it on its own,
// one-sided the way it went (the screen ≤ 2/252, the fresh reps ≤ 1/252),
// and both runs' reps together agree.
func TestConfirmReplication(t *testing.T) {
	head := []float64{1500, 1501, 1502, 1503, 1504}
	// Base's slowest rep at top: none of head's reps under it (U = 0,
	// one-sided 1/252), one (U = 1, 2/252), two (U = 2, 4/252: two-sided
	// 0.032, a screen's regression all the same).
	base := func(top float64) []float64 { return []float64{1000, 1001, 1002, 1003, top} }
	for _, tc := range []struct {
		name          string
		screen, fresh []float64 // base's values; head's are head
		want          Verdict
		note          string
	}{
		{"both separated", base(1004), base(1004), Regression, "one-sided 0.004 (≤ 0.004)"},
		{"the screen one out of order", base(1500.5), base(1004), Regression, "one-sided 0.008 (≤ 0.008)"},
		{"the screen two out of order", base(1501.5), base(1004), Inconclusive, "didn't hold up"},
		{"the fresh reps one out of order", base(1004), base(1500.5), Inconclusive, "didn't hold up"},
		{"the fresh reps two out of order", base(1004), base(1501.5), Inconclusive, "didn't hold up"},
	} {
		screen := judged(t, tc.screen, head)
		if m := metric(screen, "activation_wait"); m.Status != Worse {
			t.Fatalf("%s: the screen: %+v", tc.name, m)
		}
		r := Confirm(screen, judged(t, tc.fresh, head), Options{})
		if m := metric(r, "activation_wait"); r.Verdict != tc.want || !strings.Contains(m.Note, tc.note) {
			t.Errorf("%s: %s %+v", tc.name, r.Verdict, m)
		}
	}
	// The fresh reps alone are fully separated, and so is the screen, but
	// both runs' reps together overlap: the screen's 100s against the
	// fresh 1000s.
	r := Confirm(judged(t, steady(100, 5), steady(400, 5)), judged(t, steady(1000, 5), steady(1300, 5)), Options{})
	if m := metric(r, "activation_wait"); r.Verdict != Inconclusive || !strings.Contains(m.Note, "both inconclusive: didn't hold up") {
		t.Errorf("pooled disagrees: %s %+v", r.Verdict, m)
	}
	// An improvement replicates the same way, the other direction.
	if r := Confirm(judged(t, steady(1500, 5), steady(1000, 5)), judged(t, steady(1500, 5), steady(1000, 5)), Options{}); r.Verdict != Improvement {
		t.Errorf("improvement: %s: %s", r.Verdict, r.Why)
	}
}

// Skewed load in the confirmation makes it inconclusive, as in a screen.
func TestConfirmSkew(t *testing.T) {
	b, h := timings(steady(1000, 5), steady(1500, 5))
	h.load = 3.0
	again := judge(t, b, h, Options{})
	if r := Confirm(judged(t, steady(1000, 5), steady(1500, 5)), again, Options{}); r.Verdict != Inconclusive || r.Confirmed == nil {
		t.Errorf("skewed confirmation: %s: %s", r.Verdict, r.Why)
	}
	// #195 within its rate in the screen (5 of 20), over it in fresh reps
	// that ran under skewed load (10 of 20): 15 of 40 over both runs would
	// fail, but the skew excuses it.
	play := func(k int, load float64) *fixture {
		f := newFixture(20)
		f.load = load
		s, known := report.Pass, ""
		switch {
		case k == 0:
		case overRate(tally{fired: k, reps: 20}, 0.25):
			s, known = report.Fail, "#195"
		default:
			s, known = report.XFail, "#195"
		}
		return f.check("play-ok", s, known, firing("#195", k, 20)...)
	}
	screen := judge(t, play(0, 0.4), play(5, 0.4), Options{})
	if r := Confirm(screen, judge(t, play(0, 0.4), play(10, 3.0), Options{}), Options{}); r.Verdict != Inconclusive || !strings.Contains(r.Why, "under skewed load") {
		t.Errorf("an issue under skewed fresh reps: %s: %s", r.Verdict, r.Why)
	}
}

// Intermittent issues are judged on both runs' firings: a rise the
// screen couldn't tell (2 of 5) and the fresh reps show (5 of 5) fails;
// a rise within the rate needs the fresh reps to show it on their own.
func TestConfirmIssues(t *testing.T) {
	play := func(k, n int) *fixture {
		s, known := report.Pass, ""
		switch {
		case k == 0:
		case overRate(tally{fired: k, reps: n}, 0.25):
			s, known = report.Fail, "#195"
		default:
			s, known = report.XFail, "#195"
		}
		return newFixture(n).check("play-ok", s, known, firing("#195", k, n)...)
	}
	screen := judge(t, play(0, 5), play(2, 5), Options{})
	if screen.Verdict != Inconclusive || !slices.Equal(ToConfirm(screen), []string{"cold-start"}) {
		t.Fatalf("the screen: %s: %s; ToConfirm %v", screen.Verdict, screen.Why, ToConfirm(screen))
	}
	if r := Confirm(screen, judge(t, play(0, 5), play(5, 5), Options{}), Options{}); r.Verdict != Fail || !strings.Contains(r.Why, "7 of both runs' 10 head reps") {
		t.Errorf("worse on fresh reps: %s: %s", r.Verdict, r.Why)
	}
	screen = judge(t, play(0, 5), play(2, 5), Options{})
	if r := Confirm(screen, judge(t, play(0, 5), play(0, 5), Options{}), Options{}); r.Verdict != Pass {
		t.Errorf("clean fresh reps: %s: %s", r.Verdict, r.Why)
	}
	// Within the rate: 5 of 20 against none is a screen's regression; the
	// fresh reps' 2 of 20 make 7 of 40 over both, still significant, but
	// not on the fresh reps alone.
	screen = judge(t, play(0, 20), play(5, 20), Options{})
	if screen.Verdict != Regression {
		t.Fatalf("within its rate: %s: %s", screen.Verdict, screen.Why)
	}
	if r := Confirm(screen, judge(t, play(0, 20), play(2, 20), Options{}), Options{}); r.Verdict != Inconclusive || !strings.Contains(r.Why, "not on the fresh reps on their own") {
		t.Errorf("not on the fresh reps: %s: %s", r.Verdict, r.Why)
	}
	screen = judge(t, play(0, 20), play(5, 20), Options{})
	if r := Confirm(screen, judge(t, play(0, 20), play(5, 20), Options{}), Options{}); r.Verdict != Regression {
		t.Errorf("on the fresh reps too: %s: %s", r.Verdict, r.Why)
	}
}

// A single compare is a screen: its statistical regressions and
// improvements are inconclusive (J1); checks, exact metrics and broken
// reps keep their verdicts.
func TestUnconfirmed(t *testing.T) {
	for _, tc := range []struct {
		name string
		r    *Result
		want Verdict
	}{
		{"timing regression", judged(t, steady(1000, 5), steady(1500, 5)), Inconclusive},
		{"timing improvement", judged(t, steady(1500, 5), steady(1000, 5)), Inconclusive},
		{"no change", judged(t, steady(1000, 5), steady(1001, 5)), Pass},
		{"exact", judge(t, newFixture(1).metric("leaked_goroutines", report.Exact, "goroutines", report.Lower, 0),
			newFixture(1).metric("leaked_goroutines", report.Exact, "goroutines", report.Lower, 2), Options{}), Regression},
		{"check", judge(t, newFixture(1), newFixture(1).check("integrity", report.Fail, "", "fail"), Options{}), Fail},
	} {
		u := Unconfirmed(tc.r)
		if u.Verdict != tc.want {
			t.Errorf("%s: %s: %s", tc.name, u.Verdict, u.Why)
		}
		screen := strings.Contains(strings.Join(u.Notes, " "), "A single compare is a screen")
		if want := tc.want == Inconclusive; screen != want {
			t.Errorf("%s: the screen's note %v: %v", tc.name, screen, u.Notes)
		}
	}
	r := Unconfirmed(judged(t, steady(1000, 5), steady(1500, 5)))
	if m := metric(r, "activation_wait"); m.Status != Unsure || !strings.Contains(r.Why, "a single compare is a screen: confirm with ab") {
		t.Errorf("the metric: %+v: %s", m, r.Why)
	}
	// A confirmed result has nothing left to turn.
	c := Confirm(judged(t, steady(1000, 5), steady(1500, 5)), judged(t, steady(1000, 5), steady(1500, 5)), Options{})
	if u := Unconfirmed(c); u.Verdict != Regression {
		t.Errorf("confirmed: %s: %s", u.Verdict, u.Why)
	}
}

// ToConfirm names the scenarios whose verdict rests on statistics, and
// no fail.
func TestToConfirm(t *testing.T) {
	scs := []Scenario{
		{Name: "timing", Verdict: Regression, Metrics: []Metric{{Kind: report.Timing, Status: Worse}}},
		{Name: "count", Verdict: Inconclusive, Metrics: []Metric{{Kind: report.Count, Status: Unsure}}},
		{Name: "exact", Verdict: Regression, Metrics: []Metric{{Kind: report.Exact, Status: Worse}}},
		{Name: "issue", Verdict: Regression, Intermittent: []Issue{{ID: "#195", Verdict: Regression}}},
		{Name: "issue-over", Verdict: Inconclusive, Intermittent: []Issue{{ID: "#195", Verdict: Inconclusive}}},
		{Name: "flaky", Verdict: Inconclusive, Checks: []Check{{ID: "play-ok", Verdict: Inconclusive}}},
		{Name: "failed", Verdict: Fail, Metrics: []Metric{{Kind: report.Timing, Status: Worse}}},
		{Name: "same", Verdict: Pass, Metrics: []Metric{{Kind: report.Timing, Status: Same}}},
	}
	want := []string{"timing", "count", "issue", "issue-over", "flaky"}
	if got := ToConfirm(&Result{Scenarios: scs}); !slices.Equal(got, want) {
		t.Errorf("ToConfirm = %v, want %v", got, want)
	}
}

// What head didn't reach stays inconclusive through the confirmation: a
// metric head measured in neither run, and one head measured on 2 of 5
// reps in both (its best reps against all of base's).
func TestConfirmWhatHeadDidntReach(t *testing.T) {
	missing := func() *Result {
		b := newFixture(5).timing("time_to_playable", steady(500, 5)...).timing("ttfb", steady(100, 5)...)
		return judge(t, b, newFixture(5).timing("ttfb", steady(100, 5)...), Options{})
	}
	r := Confirm(missing(), missing(), Options{})
	if m := metric(r, "time_to_playable"); r.Verdict != Inconclusive || m.Status != Unsure || !strings.Contains(m.Note, "head never got there") {
		t.Errorf("never measured: %s %+v", r.Verdict, m)
	}
	survivors := func(head ...float64) *Result {
		return judge(t, newFixture(5).timing("time_to_playable", 500, 520, 540, 560, 580), newFixture(5).timing("time_to_playable", head...), Options{})
	}
	r = Confirm(survivors(505, 515), survivors(505, 515), Options{})
	if m := metric(r, "time_to_playable"); r.Verdict != Inconclusive || m.Status != Unsure || !strings.Contains(m.Note, "4 of 10 head reps and 10 of 10 base") {
		t.Errorf("survivors: %s %+v", r.Verdict, m)
	}
	// Head reached it on every fresh rep, at base's times: 7 of 10 over
	// both runs is still fewer than base's 10.
	r = Confirm(survivors(505, 515), survivors(505, 515, 525, 545, 565), Options{})
	if m := metric(r, "time_to_playable"); r.Verdict != Inconclusive || m.Status != Unsure {
		t.Errorf("survivors, then every rep: %s %+v", r.Verdict, m)
	}
}

// #225 (a leak, on quiescence) rising within its rate in the screen and
// over it in fresh reps that ran under skewed load: the skew excuses no
// leak, so both runs together fail.
func TestConfirmLeakUnderSkew(t *testing.T) {
	quiescence := func(k int, load float64) *fixture {
		f := newFixture(20)
		f.load = load
		s, known := report.Pass, ""
		switch {
		case k == 0:
		case overRate(tally{fired: k, reps: 20}, 0.25):
			s, known = report.Fail, "#225"
		default:
			s, known = report.XFail, "#225"
		}
		return f.check("quiescence", s, known, firing("#225", k, 20)...)
	}
	screen := judge(t, quiescence(0, 0.4), quiescence(5, 0.4), Options{})
	if screen.Verdict != Regression {
		t.Fatalf("the screen: %s: %s", screen.Verdict, screen.Why)
	}
	if r := Confirm(screen, judge(t, quiescence(0, 0.4), quiescence(10, 3.0), Options{}), Options{}); r.Verdict != Fail || !strings.Contains(r.Why, "15 of both runs' 40 head reps") {
		t.Errorf("a leak under skewed fresh reps: %s: %s", r.Verdict, r.Why)
	}
}
