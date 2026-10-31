package compare

import (
	"fmt"
	"slices"

	"github.com/shapedtime/momoshtrem/internal/streamlab/known"
	"github.com/shapedtime/momoshtrem/internal/streamlab/report"
)

// A screen's timing or count change is confirmed when each run shows it
// on its own, one-sided, the way it went: the screen at ScreenAlpha, the
// fresh repetitions at ConfirmAlpha (and both runs' reps together past
// the threshold).
//
// At 5 reps against 5 the exact one-sided p-values are k/252: 1/252 when
// every head rep is past every base rep, 2/252 with one pair out of order.
// A screen tests a few hundred metrics, so on an unchanged tree some are
// significant by chance, and each replicates by chance with probability
// P(screen) × P(fresh). With the screen at p < 0.05 and the fresh reps at
// 2/252 that was 3.5% of unchanged abs in the null simulation
// (null_test.go: 22 scenarios × 15 independent, noisy metrics), and with
// the fresh reps at 1/252 still 2.1%; with the screen at 2/252 too it's
// about 1% (TestNullFalseVerdicts holds it to 2%). Holm over the flagged
// metrics can't do it at 5 reps: with six flags at a family-wise 2% it
// asks for p ≤ 0.0033, below the 1/252 that 5 reps can reach, so a real
// regression that moves six metrics (356e74d) could confirm none. A change
// the screen showed less strongly is inconclusive: add reps.
const (
	ScreenAlpha  = 2.0 / 252
	ConfirmAlpha = 1.0 / 252
)

// ToConfirm are the scenarios of a screen whose verdict rests on
// statistics: a timing or count metric past its threshold, a check that
// failed on some reps (judgeFlake), or an intermittent issue judged by
// its firings. A fail doesn't need confirming.
func ToConfirm(r *Result) []string {
	var out []string
	for _, sc := range r.Scenarios {
		if sc.Verdict == Fail {
			continue
		}
		need := false
		for _, m := range sc.Metrics {
			need = need || (m.Kind != report.Exact && m.Status.flagged())
		}
		for _, is := range sc.Intermittent {
			need = need || is.Verdict == Inconclusive || is.Verdict == Regression
		}
		for _, ch := range sc.Checks {
			need = need || ch.Verdict == Inconclusive
		}
		if need {
			out = append(out, sc.Name)
		}
	}
	return out
}

// Confirm judges a screen's statistical verdicts again with again, a
// comparison of fresh repetitions of the scenarios ToConfirm named:
//   - A timing or count metric the screen flagged keeps a regression or
//     an improvement only when each run shows it on its own the same way
//     (one-sided p ≤ ScreenAlpha in the screen, ≤ ConfirmAlpha in the fresh
//     repetitions, both past the threshold), and the repetitions of both
//     together back it. A change both runs together put within its
//     threshold is noise (the same); anything else is inconclusive.
//   - Intermittent issues, and checks that failed on some head reps with
//     no known signature (judgeFlake), are judged on both runs' reps
//     together.
//   - A fail in again (a broken repetition, a check) counts as one.
//
// opts are the comparisons'. The screen's result is updated and returned.
func Confirm(screen, again *Result, opts Options) *Result {
	c := conditions{timing: screen.Timing, counts: screen.Counts, skew: screen.LoadSkew, noise: opts.Noise}
	if c.skew == "" {
		c.skew = again.LoadSkew
	}
	if !again.Timing.Judged {
		c.timing = again.Timing
	}
	conf := &Confirmation{Base: again.Base, Head: again.Head}
	for i := range screen.Scenarios {
		sc := &screen.Scenarios[i]
		j := slices.IndexFunc(again.Scenarios, func(a Scenario) bool { return a.Name == sc.Name })
		if j < 0 {
			continue
		}
		ag := again.Scenarios[j]
		conf.Scenarios = append(conf.Scenarios, sc.Name)
		for k := range sc.Metrics {
			m := &sc.Metrics[k]
			if m.Kind == report.Exact || !m.Status.flagged() {
				continue
			}
			*m = confirmMetric(*sc, *m, ag, c)
		}
		sc.Intermittent = mergeIssues(sc.Intermittent, ag.Intermittent, c)
		merged := mergeChecks(sc.Checks, ag.Checks)
		// The fresh run's own fails: a hang, a check, an error.
		rest := ag
		rest.Metrics, rest.Intermittent, rest.Again = nil, nil, nil
		rest.Checks = slices.DeleteFunc(slices.Clone(ag.Checks), func(ch Check) bool { return merged[ch.ID] })
		rest.settle()
		if rest.Verdict.rank() >= Inconclusive.rank() {
			sc.Again = &Again{Verdict: rest.Verdict, Why: rest.Why}
		}
		sc.settle()
	}
	screen.Confirmed = conf
	screen.Errors = append(screen.Errors, again.Errors...)
	// What wasn't confirmed (a fail scenario's metrics) stays a screen's.
	unconfirmed(screen)
	screen.settle()
	return screen
}

// Unconfirmed is a screen's result as a verdict of its own: what rests
// on statistics no fresh repetitions confirmed is inconclusive. A single
// compare (compare base head, ab --confirm=false, an ab folder without
// confirm/) runs a few hundred tests, and on an unchanged tree a few of
// them pass p < 0.05 by chance: as a regression it would be false nearly
// every time. Checks, exact metrics, broken repetitions and intermittent
// issues over their rate keep their verdicts.
func Unconfirmed(screen *Result) *Result {
	if unconfirmed(screen) {
		screen.Notes = append(screen.Notes, "A single compare is a screen: timing and count changes, and intermittent issues within their rate, "+
			"are inconclusive until ab confirms them on fresh reps.")
	}
	for i := range screen.Scenarios {
		screen.Scenarios[i].settle()
	}
	screen.settle()
	return screen
}

// unconfirmed turns every statistical regression or improvement nothing
// confirmed into inconclusive, and reports whether there was one. A
// run-wide rise within an issue's rate is never confirmed (the
// confirmation reruns scenarios, not the run): it's inconclusive too.
func unconfirmed(r *Result) bool {
	changed := false
	note := "a single compare is a screen: confirm with ab"
	for i := range r.Scenarios {
		sc := &r.Scenarios[i]
		for k := range sc.Metrics {
			m := &sc.Metrics[k]
			if m.Kind != report.Exact && m.Screen == "" && (m.Status == Worse || m.Status == Better) {
				m.Status, m.Note, changed = Unsure, note, true
			}
		}
		for k := range sc.Intermittent {
			if is := &sc.Intermittent[k]; is.Verdict == Regression && !is.Confirmed {
				is.Verdict, is.Note, changed = Inconclusive, is.Note+": "+note, true
			}
		}
		sc.settle()
	}
	for k := range r.Intermittent {
		if is := &r.Intermittent[k]; is.Verdict == Regression {
			is.Verdict, is.Note, changed = Inconclusive, is.Note+": a run-wide rise is the scenarios' to confirm", true
		}
	}
	return changed
}

// confirmMetric judges a flagged metric of scenario sc again with ag, the
// scenario's confirmation: again's own status, and both runs' values
// together. What head didn't reach stays inconclusive, as in a screen.
func confirmMetric(sc Scenario, m Metric, ag Scenario, c conditions) Metric {
	i := slices.IndexFunc(ag.Metrics, func(a Metric) bool { return a.Name == m.Name })
	if i < 0 {
		m.Screen, m.Status = m.Status, Unsure
		m.Note = "the confirmation didn't measure it"
		return m
	}
	a := ag.Metrics[i]
	def := report.Metric{Kind: m.Kind, Unit: m.Unit, Better: m.Better}
	both := compareMetric(sc.Name, m.Name, concat(m.Base.Values, a.Base.Values), concat(m.Head.Values, a.Head.Values), def, c, nil)
	both.Screen = m.Status
	if m.Head.N == 0 || a.Head.N == 0 {
		// A run whose head never measured it (a play that never got
		// playable): no values to pass on.
		both.Status = Unsure
		both.Note = fmt.Sprintf("head measured it on %d of the screen's %d reps and %d of the fresh %d, base on %d and %d: head never got there?",
			m.Head.N, sc.HeadOK, a.Head.N, ag.HeadOK, m.Base.N, a.Base.N)
		return both
	}
	if note := survivorBias(m.Name, both.Base.N, sc.BaseOK+ag.BaseOK, both.Head.N, sc.HeadOK+ag.HeadOK); note != "" && both.Status != Worse {
		both.Status, both.Note = Unsure, "over both runs, "+note
		return both
	}
	// Each run's own one-sided p, the way the screen's change went.
	screen, fresh := oneSided(m.Base.Values, m.Head.Values, m.Delta), oneSided(a.Base.Values, a.Head.Values, m.Delta)
	// A replication: each run significant the same way on its own, and
	// both runs together too. A screen that wasn't significant makes the
	// fresh reps a second screen, not a replication: one of them passing
	// by chance stays inconclusive.
	confirmed := (m.Status == Worse || m.Status == Better) && a.Status == m.Status && both.Status == m.Status &&
		screen <= ScreenAlpha*(1+1e-9) && fresh <= ConfirmAlpha*(1+1e-9)
	note := fmt.Sprintf("screen %s%s, one-sided %s (≤ %s); confirmation %s%s, one-sided %s (≤ %s); both %s",
		m.Status, pText(m.P), PText(screen), PText(ScreenAlpha), a.Status, pText(a.P), PText(fresh), PText(ConfirmAlpha), both.Status)
	switch {
	case confirmed:
		both.Note = note
	case both.Status == Same || both.Status == Changed:
		// Over both runs' reps the change is within the threshold: the
		// screen's was noise.
		both.Note = note + ": within the threshold over both runs"
	case both.Status == Unjudged:
		both.Note = note + ": " + both.Note
	default:
		both.Status = Unsure
		both.Note = note + ": didn't hold up on fresh reps, add reps"
	}
	return both
}

// oneSided is Mann-Whitney's one-sided p that head moved from base the way
// delta (a screen's change) went.
func oneSided(base, head []float64, delta float64) float64 {
	if delta < 0 {
		return MannWhitneyGreater(head, base)
	}
	return MannWhitneyGreater(base, head)
}

func pText(p *float64) string {
	if p == nil {
		return ""
	}
	return " (p " + PText(*p) + ")"
}

func concat(a, b []float64) []float64 {
	return append(slices.Clone(a), b...)
}

// mergeIssues judges each intermittent issue on both runs' firings.
func mergeIssues(first, again []Issue, c conditions) []Issue {
	var out []Issue
	seen := map[string]bool{}
	for _, list := range [][]Issue{first, again} {
		for _, i := range list {
			if seen[i.ID] {
				continue
			}
			seen[i.ID] = true
			is, ok := known.Lookup(i.ID)
			if !ok {
				out = append(out, i)
				continue
			}
			var bt, ht tally
			for _, l := range [][]Issue{first, again} {
				if k := slices.IndexFunc(l, func(x Issue) bool { return x.ID == i.ID }); k >= 0 {
					bt.fired += l[k].BaseFired
					bt.reps += l[k].BaseReps
					ht.fired += l[k].HeadFired
					ht.reps += l[k].HeadReps
					ht.checks = l[k].Checks
					ht.onCorrectness = ht.onCorrectness || l[k].OnCorrectness
				}
			}
			merged := judgeIssue(is, bt, ht, c, "both runs'")
			// A rise within the rate holds only when the fresh reps show
			// it on their own too.
			if merged.Verdict == Regression {
				k := slices.IndexFunc(again, func(x Issue) bool { return x.ID == i.ID })
				if k < 0 || (again[k].Verdict != Regression && again[k].Verdict != Fail) {
					merged.Verdict, merged.Note = Inconclusive, merged.Note+": not on the fresh reps on their own, add reps"
				}
			}
			merged.Confirmed = true
			out = append(out, merged)
		}
	}
	return out
}

// mergeChecks judges each check the screen found flaky (inconclusive) on
// both runs' reps together, and returns the IDs it judged.
func mergeChecks(screen, again []Check) map[string]bool {
	merged := map[string]bool{}
	for i := range screen {
		ch := &screen[i]
		if ch.Verdict != Inconclusive || ch.HeadReps == 0 {
			continue
		}
		j := slices.IndexFunc(again, func(a Check) bool { return a.ID == ch.ID })
		if j < 0 {
			continue
		}
		a := again[j]
		both := *ch
		both.HeadFailed += a.HeadFailed
		both.HeadReps += a.HeadReps
		both.BaseFailed += a.BaseFailed
		both.BaseReps += a.BaseReps
		both.Verdict, both.Note = judgeFlake(both, fmt.Sprintf("over both runs, %s → %s", statusText(ch.Base, ch.BaseKnown), statusText(ch.Head, ch.HeadKnown)))
		*ch = both
		merged[ch.ID] = true
	}
	return merged
}
