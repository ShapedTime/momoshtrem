package compare

import (
	"fmt"
	"math"
	"math/rand/v2"
	"os"
	"slices"
	"testing"

	"github.com/shapedtime/momoshtrem/internal/streamlab/report"
)

// nullSide is one side of an unchanged tree's run, both sides drawn from
// one distribution: the gate's size (22 scenarios, 10 timings and 5
// counts each), reps a side, timings log-normal around 2 s, counts
// around 50.
func nullSide(rng *rand.Rand, names []string, reps int) *Side {
	s := &Side{Path: "null"}
	for _, name := range names {
		f := newFixture(reps)
		f.name = name
		for j := range 10 {
			v := make([]float64, reps)
			for k := range v {
				v[k] = 2000 * math.Exp(0.5*rng.NormFloat64())
			}
			f.timing(fmt.Sprintf("t%02d", j), v...)
		}
		for j := range 5 {
			v := make([]float64, reps)
			for k := range v {
				v[k] = math.Round(50 * math.Exp(0.3*rng.NormFloat64()))
			}
			f.metric(fmt.Sprintf("c%02d", j), report.Count, "reads", report.Lower, v...)
		}
		s.Reports = append(s.Reports, f.report())
	}
	return s
}

// An unchanged tree's ab, simulated: the screen, then the fresh reps of
// what it flagged, as ab runs them (5 a side). A regression or an
// improvement there is false; PLAN §10's "three unchanged runs in a row"
// needs it rare: at most 2% (ScreenAlpha and ConfirmAlpha give about
// 1.2% over 10,000 trials). The screen alone (compare, ab
// --confirm=false) is never one: Unconfirmed. STREAMLAB=1 runs 3,000
// trials and holds them to 2%; by default 100, held to 5%, a quick
// check that the rule didn't loosen a lot (the old one gave 9%).
func TestNullFalseVerdicts(t *testing.T) {
	trials, limit := 100, 0.05
	if os.Getenv("STREAMLAB") == "1" {
		trials, limit = 3000, 0.02
	}
	rng := rand.New(rand.NewPCG(1, 2))
	all := make([]string, 22)
	for i := range all {
		all[i] = fmt.Sprintf("sc-%02d", i)
	}
	screens, confirmed := map[Verdict]int{}, map[Verdict]int{}
	for range trials {
		r, err := Compare(nullSide(rng, all, 5), nullSide(rng, all, 5), Options{})
		if err != nil {
			t.Fatal(err)
		}
		names := ToConfirm(r)
		if len(names) == 0 {
			confirmed[Unconfirmed(r).Verdict]++
			continue
		}
		again, err := Compare(nullSide(rng, names, 5), nullSide(rng, names, 5), Options{})
		if err != nil {
			t.Fatal(err)
		}
		// The standalone screen, judged before Confirm updates it.
		screens[Unconfirmed(cloneResult(r)).Verdict]++
		confirmed[Confirm(r, again, Options{}).Verdict]++
	}
	false_ := confirmed[Regression] + confirmed[Improvement]
	rate := float64(false_) / float64(trials)
	t.Logf("null, 22 scenarios × (10 timings + 5 counts), 5 v 5, %d trials: ab %v, false %.1f%%; screens alone %v", trials, confirmed, 100*rate, screens)
	if rate > limit {
		t.Errorf("an unchanged tree's ab gives a regression or an improvement %.1f%% of the time (≤ %.0f%%)", 100*rate, 100*limit)
	}
	if screens[Regression]+screens[Improvement] > 0 || confirmed[Fail] > 0 {
		t.Errorf("a screen alone gave a statistical verdict, or a fail: %v %v", screens, confirmed)
	}
}

// cloneResult copies what Confirm and Unconfirmed change.
func cloneResult(r *Result) *Result {
	c := *r
	c.Scenarios = slices.Clone(r.Scenarios)
	for i := range c.Scenarios {
		c.Scenarios[i].Metrics = slices.Clone(r.Scenarios[i].Metrics)
		c.Scenarios[i].Intermittent = slices.Clone(r.Scenarios[i].Intermittent)
		c.Scenarios[i].Checks = slices.Clone(r.Scenarios[i].Checks)
	}
	c.Intermittent = slices.Clone(r.Intermittent)
	c.Notes = slices.Clone(r.Notes)
	return &c
}
