// Package compare is streamlab compare (docs/streamlab/PLAN.md §6
// "Compare", D5, D6, D8): two runs of the same scenarios, base and head,
// judged into one verdict an agent can trust: pass, regression,
// improvement, inconclusive or fail.
//
//   - fail: head has an integrity error, a panic, a race, a watchdog trip
//     or a leak (a correctness check fails), a repetition that hung or
//     crashed, or a check that didn't fail in base fails in head (on every
//     head rep, or significantly more than in base).
//   - Exact metrics: any change on any rep is reported, its direction
//     from the metric's better and the means.
//   - Count metrics: a median change past max(10%, 3×CV, a floor), the CV
//     from calibrate's noise.json for this machine or DefaultCV.
//   - Timing metrics: a median change past max(15%, a floor per metric)
//     with Mann-Whitney U p < 0.05, at least 5 reps a side. Less is
//     inconclusive.
//
// Known issues (D6) keep their meaning: a correctness check a known issue
// explains in head (an xfail) doesn't fail, and an intermittent issue
// (#195) that fired over its rate fails only when it fired significantly
// more often in head than in base (Fisher's exact test), since the base
// side ran under the same load. Otherwise it's inconclusive: a loaded
// machine makes #195 fire on both sides. xfail → xpass is reported as a
// known issue that appears fixed.
//
// Two machines are refused without Force; race builds get no timing
// verdicts (D8); sides that ran under very different load turn timing
// and count verdicts inconclusive.
//
// One comparison tests every timing and count metric of every scenario,
// a few hundred tests, so a handful pass p < 0.05 by chance. A single
// Compare is therefore a screen: Confirm judges the metrics it flagged
// again on fresh repetitions (streamlab ab runs them), and Unconfirmed
// makes what nothing confirmed inconclusive.
package compare

import (
	"cmp"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/shapedtime/momoshtrem/internal/streamlab/known"
	"github.com/shapedtime/momoshtrem/internal/streamlab/report"
)

// Schema names compare.json's format.
const Schema = "streamlab.compare/v1"

// Verdict is a comparison's outcome.
type Verdict string

const (
	Pass         Verdict = "pass"
	Improvement  Verdict = "improvement"
	Inconclusive Verdict = "inconclusive"
	Regression   Verdict = "regression"
	Fail         Verdict = "fail"
)

// rank orders verdicts: the worst wins.
func (v Verdict) rank() int {
	switch v {
	case Improvement:
		return 1
	case Inconclusive:
		return 2
	case Regression:
		return 3
	case Fail:
		return 4
	}
	return 0
}

// Exit codes (PLAN §6).
const (
	ExitPass         = 0
	ExitFail         = 1
	ExitInconclusive = 2
	ExitTool         = 3
)

// ExitCode is the verdict's exit code: 0 pass or improvement, 1 fail or
// regression, 2 inconclusive.
func (v Verdict) ExitCode() int {
	switch v {
	case Fail, Regression:
		return ExitFail
	case Inconclusive:
		return ExitInconclusive
	}
	return ExitPass
}

// ErrMachines is a comparison of two machines' runs without Force.
var ErrMachines = errors.New("the sides ran on different machines")

// ErrParams is a comparison of scenarios run with different params
// without Force.
var ErrParams = errors.New("the sides ran scenarios with different params")

// Options configure a comparison.
type Options struct {
	// Force compares across machines (and params): timings and counts
	// then give no verdict, only exact metrics and checks.
	Force bool
	// Noise is this machine's calibration; nil: DefaultCV.
	Noise *Calibration
	// NoiseSource says where Noise came from, for the report.
	NoiseSource string
}

// Result is compare.json.
type Result struct {
	Schema  string  `json:"schema"`
	Verdict Verdict `json:"verdict"`
	// Why is the verdict's reason on one line: what decided it.
	Why  string   `json:"why"`
	Base SideInfo `json:"base"`
	Head SideInfo `json:"head"`
	// Timing says whether timings could get a verdict, and why not.
	Timing Judged `json:"timing"`
	// Counts says whether counts could.
	Counts Judged    `json:"counts"`
	Noise  NoiseInfo `json:"noise"`
	// LoadSkew says why timing and count changes are inconclusive, when
	// the sides ran under very different load.
	LoadSkew string `json:"load_skew,omitempty"`
	// Scenarios are those both sides ran, in head's order.
	Scenarios []Scenario `json:"scenarios"`
	// Intermittent is each intermittent known issue over every scenario
	// both ran: how often it fired on each side.
	Intermittent []Issue `json:"intermittent"`
	// Errors are scenarios a side couldn't run (a tool or environment
	// error): the CLI exits 3 when there's one.
	Errors []string `json:"errors,omitempty"`
	Notes  []string `json:"notes,omitempty"`
	// Confirmed is set once Confirm judged the flagged metrics again on
	// fresh repetitions.
	Confirmed *Confirmation `json:"confirmed,omitempty"`
}

// Confirmation is what Confirm used.
type Confirmation struct {
	// Scenarios are those judged again.
	Scenarios []string `json:"scenarios"`
	Base      SideInfo `json:"base"`
	Head      SideInfo `json:"head"`
}

// SideInfo describes one side.
type SideInfo struct {
	Path    string `json:"path"`
	Commit  string `json:"commit,omitempty"`
	Dirty   bool   `json:"dirty"`
	Branch  string `json:"branch,omitempty"`
	Machine string `json:"machine"`
	Race    bool   `json:"race"`
	// Reps is the most measured repetitions of a scenario.
	Reps int `json:"reps"`
	// Load is the 1-minute load average per CPU over the measured reps.
	Load LoadStats `json:"load"`
}

// LoadStats sums up the reps' loads per CPU.
type LoadStats struct {
	Reps   int     `json:"reps"`
	Min    float64 `json:"min"`
	Median float64 `json:"median"`
	Max    float64 `json:"max"`
}

func newLoadStats(v []float64) LoadStats {
	if len(v) == 0 {
		return LoadStats{}
	}
	return LoadStats{Reps: len(v), Min: slices.Min(v), Median: report.Quantile(v, 0.5), Max: slices.Max(v)}
}

// Judged says whether a kind of metric could get a verdict.
type Judged struct {
	Judged bool   `json:"judged"`
	Why    string `json:"why,omitempty"`
}

// NoiseInfo is where the count noise came from.
type NoiseInfo struct {
	// Source is noise.json's path, or "defaults".
	Source string `json:"source"`
	// Calibrated is the calibration's commit, date and reps, when there's
	// one.
	Calibrated string  `json:"calibrated,omitempty"`
	DefaultCV  float64 `json:"default_cv"`
}

// Scenario is one scenario's comparison.
type Scenario struct {
	Name    string  `json:"name"`
	Verdict Verdict `json:"verdict"`
	// Why is what decided its verdict.
	Why []string `json:"why,omitempty"`
	// Broken are head's repetitions that hung or crashed: fails.
	Broken       []string `json:"broken,omitempty"`
	Checks       []Check  `json:"checks"`
	Metrics      []Metric `json:"metrics"`
	Intermittent []Issue  `json:"intermittent,omitempty"`
	// Errors say a side couldn't run it.
	Errors []string `json:"errors,omitempty"`
	Notes  []string `json:"notes,omitempty"`
	// Again is the confirmation's view of its checks, hangs and
	// intermittent issues: a fail seen there counts.
	Again *Again `json:"again,omitempty"`
	// BaseOK and HeadOK are each side's measured reps that ran to the
	// end: what a metric only some reps record is measured against.
	BaseOK int `json:"base_ok_reps"`
	HeadOK int `json:"head_ok_reps"`
}

// Again is a scenario's confirmation run, beyond its metrics.
type Again struct {
	Verdict Verdict  `json:"verdict"`
	Why     []string `json:"why,omitempty"`
}

// Check is one check on both sides.
type Check struct {
	ID        string        `json:"id"`
	Base      report.Status `json:"base"`
	BaseKnown string        `json:"base_known,omitempty"`
	Head      report.Status `json:"head"`
	HeadKnown string        `json:"head_known,omitempty"`
	Verdict   Verdict       `json:"verdict"`
	Note      string        `json:"note,omitempty"`
	// HeadFailed of HeadReps, and BaseFailed of BaseReps, are the reps
	// whose failure carries no known signature (Check.Reps "fail"), of
	// those that ran it.
	HeadFailed int `json:"head_failed,omitempty"`
	HeadReps   int `json:"head_reps,omitempty"`
	BaseFailed int `json:"base_failed,omitempty"`
	BaseReps   int `json:"base_reps,omitempty"`
}

// unexplained counts the reps whose failure the runner's verdict on the
// check doesn't explain, of those that ran it: a plain "fail", or
// "fail:<issues>" when the check's Known doesn't name every one of them.
// A check the runner failed with no known issue (Known "") counts every
// failing rep, whatever signature it carries: the runner found it doesn't
// explain the check there (a deterministic issue outside the scenario's
// guards, one that doesn't apply on this host). An intermittent issue's
// firings are judgeIssue's to count.
func unexplained(c report.Check) (failed, ran int) {
	named := known.IDs(c.Known)
	for _, s := range c.Reps {
		if s == string(report.NA) {
			continue
		}
		ran++
		ids, signed := strings.CutPrefix(s, string(report.Fail)+":")
		if s == string(report.Fail) || (signed && !allNamed(known.IDs(ids), named)) {
			failed++
		}
	}
	return failed, ran
}

func allNamed(ids, named []string) bool {
	for _, id := range ids {
		if !slices.Contains(named, id) {
			return false
		}
	}
	return len(ids) > 0
}

// MetricStatus is what compare made of a metric.
type MetricStatus string

const (
	// Same: no change past the metric's threshold.
	Same MetricStatus = "same"
	// Changed: a change of a metric with no better direction, or a
	// change a known issue explains: reported only.
	Changed MetricStatus = "changed"
	Worse   MetricStatus = "regression"
	Better  MetricStatus = "improvement"
	// Unsure: a change past the threshold that statistics or the
	// conditions can't back.
	Unsure MetricStatus = "inconclusive"
	// Unjudged: the metric couldn't get a verdict (a race build, other
	// machines); reported only.
	Unjudged MetricStatus = "unjudged"
)

// flagged reports whether a status asks for a confirmation.
func (s MetricStatus) flagged() bool { return s == Worse || s == Better || s == Unsure }

// Metric is one metric on both sides.
type Metric struct {
	Name   string        `json:"name"`
	Kind   report.Kind   `json:"kind"`
	Unit   string        `json:"unit"`
	Better report.Better `json:"better"`
	Base   Sample        `json:"base"`
	Head   Sample        `json:"head"`
	// Delta is the change from base to head: head's mean less base's for
	// an exact metric (any rep that moved is a change); for counts and
	// timings the Hodges-Lehmann shift, the median of every head − base
	// difference.
	Delta float64 `json:"delta"`
	// DeltaPct is Delta over base's median, in percent; nil when base's
	// is 0.
	DeltaPct *float64 `json:"delta_pct,omitempty"`
	// Threshold is the absolute change past which it counts, and Rule
	// how it was set.
	Threshold float64 `json:"threshold"`
	Rule      string  `json:"rule,omitempty"`
	// P is Mann-Whitney U's two-sided p-value, for timings and counts.
	P      *float64     `json:"p,omitempty"`
	Status MetricStatus `json:"status"`
	Note   string       `json:"note,omitempty"`
	// Screen is the first comparison's status, once Confirm judged it
	// again.
	Screen MetricStatus `json:"screen,omitempty"`
}

// Sample is a metric's values on one side.
type Sample struct {
	N      int       `json:"n"`
	Median float64   `json:"median"`
	Values []float64 `json:"values"`
}

// Issue is an intermittent known issue's firings on both sides.
type Issue struct {
	ID        string   `json:"id"`
	BaseFired int      `json:"base_fired"`
	BaseReps  int      `json:"base_reps"`
	HeadFired int      `json:"head_fired"`
	HeadReps  int      `json:"head_reps"`
	P         float64  `json:"p"`
	Verdict   Verdict  `json:"verdict"`
	Note      string   `json:"note,omitempty"`
	Checks    []string `json:"checks,omitempty"`
	// OnCorrectness: in head it fired on a correctness check (#225 on
	// quiescence), so skewed load doesn't excuse it.
	OnCorrectness bool `json:"on_correctness,omitempty"`
	// Confirmed: judged on the confirmation's reps too (Confirm).
	Confirmed bool `json:"confirmed,omitempty"`
}

// conditions are what every scenario's judgement shares.
type conditions struct {
	timing, counts Judged
	// skew is why timings and counts are inconclusive, when the sides ran
	// under very different load.
	skew  string
	noise *Calibration
}

// Compare judges head against base. The error is a comparison it refuses
// (ErrMachines, ErrParams) or can't make (no scenario in common).
func Compare(base, head *Side, opts Options) (*Result, error) {
	if len(base.Reports) == 0 || len(head.Reports) == 0 {
		return nil, errors.New("compare: a side has no report")
	}
	res := &Result{Schema: Schema, Base: sideInfo(base), Head: sideInfo(head),
		Noise: NoiseInfo{Source: "defaults", DefaultCV: DefaultCV}}
	if opts.Noise != nil {
		res.Noise.Source = cmp.Or(opts.NoiseSource, "noise.json")
		res.Noise.Calibrated = fmt.Sprintf("%s, %s, %d reps", shortCommit(opts.Noise.Commit), opts.Noise.Date.Format("2006-01-02"), opts.Noise.Reps)
	}
	c, notes, err := conditionsFor(base, head, opts)
	if err != nil {
		return nil, err
	}
	res.Notes = notes
	res.Timing, res.Counts, res.LoadSkew = c.timing, c.counts, c.skew

	pairs, notes, err := pair(base, head, opts.Force)
	if err != nil {
		return nil, err
	}
	res.Notes = append(res.Notes, notes...)
	for _, p := range pairs {
		sc := compareScenario(p.base, p.head, c)
		res.Errors = append(res.Errors, prefixed(sc.Name, sc.Errors)...)
		res.Scenarios = append(res.Scenarios, sc)
	}
	// Each intermittent issue over the whole run: a rate that's
	// significantly higher in head across every scenario is a regression
	// even where each scenario's own reps can't tell.
	if len(pairs) > 1 {
		res.Intermittent = runIssues(pairs, c)
	}
	res.settle()
	return res, nil
}

// conditionsFor checks the sides can be compared and sets what the
// verdicts may rest on: machines, race builds, load, noise.
func conditionsFor(base, head *Side, opts Options) (conditions, []string, error) {
	c := conditions{timing: Judged{Judged: true}, counts: Judged{Judged: true}, noise: opts.Noise}
	var notes []string
	bm, hm := base.machines(), head.machines()
	if len(bm) > 1 || len(hm) > 1 || bm[0] != hm[0] {
		why := fmt.Sprintf("base ran on %s, head on %s", strings.Join(bm, " + "), strings.Join(hm, " + "))
		if !opts.Force {
			return c, nil, fmt.Errorf("%w: %s (--force compares checks and exact metrics only)", ErrMachines, why)
		}
		c.timing = Judged{Why: "forced across machines: " + why}
		c.counts = c.timing
		notes = append(notes, "Forced across machines ("+why+"): timings and counts are reported, not judged.")
	}
	if base.race() || head.race() {
		c.timing = Judged{Why: "a race build (PLAN D8): -race distorts timings"}
		notes = append(notes, "A race build: timings are reported, not judged (PLAN D8).")
	}
	bl, hl := newLoadStats(base.loads()), newLoadStats(head.loads())
	if bl.Reps > 0 && hl.Reps > 0 {
		if skew, why := loadSkew(bl.Median, hl.Median); skew {
			c.skew = why
			notes = append(notes, "Load skew: "+why+". Timing and count changes are inconclusive; rerun when the machine is steadier.")
		}
	} else {
		notes = append(notes, "A side recorded no load average: load skew can't be checked.")
	}
	for _, s := range []struct {
		name string
		l    LoadStats
	}{{"base", bl}, {"head", hl}} {
		if s.l.Reps > 0 && s.l.Median > HighLoad {
			notes = append(notes, fmt.Sprintf("High load on %s: %.2f per CPU (median of %d reps). Timings are noisier, so verdicts lean inconclusive.",
				s.name, s.l.Median, s.l.Reps))
		}
	}
	if opts.Noise == nil {
		notes = append(notes, fmt.Sprintf("No calibration for this machine (streamlab calibrate): count noise is assumed at CV %.2f.", DefaultCV))
	}
	return c, notes, nil
}

type scenarioPair struct{ base, head *report.Report }

// pair matches the sides' scenarios by name, in head's order; params
// must match unless forced.
func pair(base, head *Side, force bool) ([]scenarioPair, []string, error) {
	var out []scenarioPair
	var notes, differ []string
	for _, hr := range head.Reports {
		br := base.report(hr.Scenario.Name)
		if br == nil {
			notes = append(notes, fmt.Sprintf("%s ran in head only: not compared.", hr.Scenario.Name))
			continue
		}
		if d := diffParams(br.Scenario.Params, hr.Scenario.Params); d != "" {
			differ = append(differ, hr.Scenario.Name+": "+d)
		}
		out = append(out, scenarioPair{br, hr})
	}
	for _, br := range base.Reports {
		if head.report(br.Scenario.Name) == nil {
			notes = append(notes, fmt.Sprintf("%s ran in base only: not compared.", br.Scenario.Name))
		}
	}
	if len(differ) > 0 {
		if !force {
			return nil, nil, fmt.Errorf("%w: %s", ErrParams, strings.Join(differ, "; "))
		}
		notes = append(notes, "Forced with different params: "+strings.Join(differ, "; "))
	}
	if len(out) == 0 {
		return nil, nil, errors.New("compare: the sides have no scenario in common")
	}
	return out, notes, nil
}

func sideInfo(s *Side) SideInfo {
	t := s.tool()
	reps := 0
	for _, r := range s.Reports {
		reps = max(reps, len(measuredReps(r)))
	}
	return SideInfo{Path: s.Path, Commit: t.Git.Commit, Dirty: t.Git.Dirty, Branch: t.Git.Branch,
		Machine: Machine(s.env()), Race: s.race(), Reps: reps, Load: newLoadStats(s.loads())}
}

func repError(r *report.Report) string {
	for _, rep := range r.Reps {
		if rep.Status == report.RepError && rep.Error != "" {
			return rep.Error
		}
	}
	return "a tool or environment error"
}

// diffParams says how two scenarios' params differ; "" when they don't.
func diffParams(a, b map[string]string) string {
	keys := map[string]bool{}
	for k := range a {
		keys[k] = true
	}
	for k := range b {
		keys[k] = true
	}
	sorted := make([]string, 0, len(keys))
	for k := range keys {
		sorted = append(sorted, k)
	}
	slices.Sort(sorted)
	var out []string
	for _, k := range sorted {
		if a[k] != b[k] {
			out = append(out, fmt.Sprintf("%s %q vs %q", k, a[k], b[k]))
		}
	}
	return strings.Join(out, ", ")
}

func prefixed(name string, s []string) []string {
	out := make([]string, len(s))
	for i, v := range s {
		out[i] = name + ": " + v
	}
	return out
}

// compareScenario compares one scenario's checks, metrics, intermittent
// issues and broken repetitions; settle gives its verdict.
func compareScenario(base, head *report.Report, c conditions) Scenario {
	sc := Scenario{Name: head.Scenario.Name}
	// Hangs and crashes in head, a warm-up's included, are fails: a
	// deadlock or a panic.
	for _, rep := range head.Reps {
		if rep.Status == report.RepTimeout || rep.Status == report.RepCrash {
			name := fmt.Sprintf("rep %d", rep.Rep)
			if rep.Warmup {
				name = "the warm-up"
			}
			sc.Broken = append(sc.Broken, fmt.Sprintf("head's %s %s: %s", name, rep.Status, rep.Error))
		}
	}
	if base.Verdict == report.VerdictError {
		sc.Errors = append(sc.Errors, "base couldn't run it: "+repError(base))
	}
	if head.Verdict == report.VerdictError {
		sc.Errors = append(sc.Errors, "head couldn't run it: "+repError(head))
	}

	baseIssues, headIssues := firings(base), firings(head)
	// Every intermittent issue whose checks ran, firing or not: Confirm
	// adds the tallies up.
	for _, is := range known.Issues {
		ht, hok := headIssues[is.ID]
		bt, bok := baseIssues[is.ID]
		if hok || bok {
			sc.Intermittent = append(sc.Intermittent, judgeIssue(is, bt, ht, c, "this scenario's"))
		}
	}

	for _, hc := range head.Checks {
		bc, inBase := findCheck(base.Checks, hc.ID)
		ch := Check{ID: hc.ID, Head: hc.Status, HeadKnown: hc.Known}
		ch.HeadFailed, ch.HeadReps = unexplained(hc)
		if inBase {
			ch.Base, ch.BaseKnown = bc.Status, bc.Known
			ch.BaseFailed, ch.BaseReps = unexplained(bc)
		}
		ch.Verdict, ch.Note = judgeCheck(ch, inBase, headIssues, baseIssues)
		sc.Checks = append(sc.Checks, ch)
	}
	// Checks base ran and head didn't: listed, and a correctness check
	// that went missing can't vouch for head.
	for _, bc := range base.Checks {
		if _, ok := findCheck(head.Checks, bc.ID); ok {
			continue
		}
		ch := Check{ID: bc.ID, Base: bc.Status, BaseKnown: bc.Known, Verdict: Pass, Note: "base only: head didn't run it"}
		if correctness[bc.ID] {
			ch.Verdict, ch.Note = Inconclusive, "a correctness check ran in base only: head can't vouch for it"
		}
		sc.Checks = append(sc.Checks, ch)
	}

	bOK, hOK := okReps(base), okReps(head)
	sc.BaseOK, sc.HeadOK = bOK, hOK
	for _, n := range metricNames(base, head) {
		bm, inBase := base.Metrics[n]
		hm, inHead := head.Metrics[n]
		switch {
		case inBase && !inHead && bm.Better != report.None && !labOnly[n]:
			// What head didn't measure it didn't reach: a play that never
			// got playable has no time to playable.
			sc.Metrics = append(sc.Metrics, Metric{Name: n, Kind: bm.Kind, Unit: bm.Unit, Better: bm.Better, Base: sample(bm.Values),
				Status: Unsure, Note: fmt.Sprintf("measured on %d of %d base reps and none of head's: head never got there?", len(bm.Values), bOK)})
			continue
		case !inBase || !inHead:
			side := "base"
			if inHead {
				side = "head"
			}
			sc.Notes = append(sc.Notes, fmt.Sprintf("%s was measured in %s only.", n, side))
			continue
		}
		m := compareMetric(sc.Name, n, bm.Values, hm.Values, hm, c, head.Checks)
		if note := survivorBias(n, len(bm.Values), bOK, len(hm.Values), hOK); note != "" && m.Status != Worse {
			m.Status, m.Note = Unsure, note
		}
		sc.Metrics = append(sc.Metrics, m)
	}
	sc.settle()
	return sc
}

// survivorBias says why a metric only the reps that got there record,
// measured on bN of base's bOK reps and hN of head's hOK, can't pass for
// the same: on fewer of head's reps than base's, it compares head's best
// reps with all of base's. "" when it can. Head worse even so is worse.
func survivorBias(name string, bN, bOK, hN, hOK int) string {
	if !survivors[name] || bOK == 0 || hOK == 0 || hN*bOK >= bN*hOK {
		return ""
	}
	return fmt.Sprintf("measured on %d of %d head reps and %d of %d base: the rest never got there", hN, hOK, bN, bOK)
}

// metricNames are both sides' metrics: exact first, then counts, then
// timings, by name.
func metricNames(base, head *report.Report) []string {
	names := map[string]report.Kind{}
	for n, m := range base.Metrics {
		names[n] = m.Kind
	}
	for n, m := range head.Metrics {
		names[n] = m.Kind
	}
	out := make([]string, 0, len(names))
	for n := range names {
		out = append(out, n)
	}
	slices.SortFunc(out, func(a, b string) int {
		return cmp.Or(cmp.Compare(kindRank(names[a]), kindRank(names[b])), strings.Compare(a, b))
	})
	return out
}

func kindRank(k report.Kind) int {
	switch k {
	case report.Exact:
		return 0
	case report.Count:
		return 1
	}
	return 2
}

// settle gives the scenario its verdict and why: the worst of its broken
// repetitions, checks, intermittent issues, metrics, errors and the
// confirmation's own.
func (sc *Scenario) settle() {
	sc.Verdict, sc.Why = Pass, nil
	add := func(v Verdict, why string) {
		if v.rank() > sc.Verdict.rank() {
			sc.Verdict, sc.Why = v, nil
		}
		if v == sc.Verdict && v != Pass {
			sc.Why = append(sc.Why, why)
		}
	}
	for _, b := range sc.Broken {
		add(Fail, b)
	}
	for _, is := range sc.Intermittent {
		add(is.Verdict, is.Note)
	}
	for _, ch := range sc.Checks {
		add(ch.Verdict, fmt.Sprintf("check %s: %s", ch.ID, ch.Note))
	}
	for _, m := range sc.Metrics {
		switch m.Status {
		case Worse:
			add(Regression, metricWhy(m))
		case Better:
			add(Improvement, metricWhy(m))
		case Unsure:
			add(Inconclusive, metricWhy(m))
		}
	}
	for _, e := range sc.Errors {
		add(Inconclusive, e)
	}
	if sc.Again != nil {
		for _, w := range sc.Again.Why {
			add(sc.Again.Verdict, "confirmation: "+w)
		}
	}
}

// settle gives the result its verdict and why: the worst scenario's and
// run-wide issue's, with their reasons.
func (r *Result) settle() {
	r.Verdict = Pass
	for _, sc := range r.Scenarios {
		if sc.Verdict.rank() > r.Verdict.rank() {
			r.Verdict = sc.Verdict
		}
	}
	for _, is := range r.Intermittent {
		if is.Verdict.rank() > r.Verdict.rank() {
			r.Verdict = is.Verdict
		}
	}
	var why []string
	for _, sc := range r.Scenarios {
		if sc.Verdict == r.Verdict && sc.Verdict != Pass {
			why = append(why, sc.Name+": "+strings.Join(sc.Why, "; "))
		}
	}
	for _, is := range r.Intermittent {
		if is.Verdict == r.Verdict && is.Verdict != Pass {
			why = append(why, "over the run: "+is.Note)
		}
	}
	if len(why) == 0 {
		why = append(why, "no check got worse and no metric changed past its threshold")
	}
	r.Why = strings.Join(why, " | ")
}

func findCheck(cs []report.Check, id string) (report.Check, bool) {
	for _, c := range cs {
		if c.ID == id {
			return c, true
		}
	}
	return report.Check{}, false
}

// judgeCheck judges a check's change from base to head.
func judgeCheck(ch Check, inBase bool, headIssues, baseIssues map[string]tally) (Verdict, string) {
	b, h := ch.Base, ch.Head
	if h == report.Fail {
		// An intermittent issue that fired over its rate: its firings are
		// judged against base's (judgeIssue).
		if is, ok := known.Lookup(ch.HeadKnown); ok && is.Intermittent() {
			ht, bt := headIssues[is.ID], baseIssues[is.ID]
			return Pass, fmt.Sprintf("fails in head as %s fired over its rate (head %d of %d reps, base %d of %d): judged as an intermittent issue",
				is.ID, ht.fired, ht.reps, bt.fired, bt.reps)
		}
		switch {
		case correctness[ch.ID] && inBase && b == report.Fail:
			return Fail, "a correctness check fails in head (it fails in base too)"
		case correctness[ch.ID]:
			return Fail, "a correctness check fails in head: " + statusText(b, ch.BaseKnown) + " → fail"
		case !inBase:
			return Fail, "fails in head, and base didn't run it"
		}
		return judgeFlake(ch, statusText(b, ch.BaseKnown)+" → fail")
	}
	if !inBase {
		return Pass, "head only"
	}
	switch {
	case b == report.XFail && h == report.XPass:
		return Improvement, fmt.Sprintf("known issue %s appears fixed (xfail → xpass): remove it from the registry", ch.BaseKnown)
	case b == report.Fail && (h == report.Pass || h == report.XPass) && Intermittent(ch.BaseKnown):
		// An intermittent issue base fired over its rate isn't head's fix:
		// judgeIssue judges its firings.
		return Pass, fmt.Sprintf("fail → %s: base failed as %s fired over its rate: judged as an intermittent issue", h, ch.BaseKnown)
	case b == report.Fail && (h == report.Pass || h == report.XPass):
		// Base's flake isn't head's fix: judged by the reps, as judgeFlake
		// judges head's.
		if ch.BaseFailed > 0 && ch.BaseFailed < ch.BaseReps {
			if p := FisherGreater(ch.HeadFailed, ch.HeadReps, ch.BaseFailed, ch.BaseReps); p >= Alpha {
				return Pass, fmt.Sprintf("fail → %s: base failed with no known signature on %d of %d reps only (Fisher p %s): a flake?",
					h, ch.BaseFailed, ch.BaseReps, PText(p))
			}
		}
		return Improvement, "fail → " + string(h) + ": fixed"
	case b == report.Fail && h == report.XFail:
		return Improvement, "fail → xfail (" + ch.HeadKnown + ")"
	case h == report.XFail && b != report.XFail:
		if is, ok := known.Lookup(ch.HeadKnown); ok && !is.Intermittent() {
			return Regression, fmt.Sprintf("known issue %s is back: %s → xfail", is.ID, statusText(b, ch.BaseKnown))
		}
		return Pass, ch.HeadKnown + " fired in head within its rate"
	case b == report.XFail && h == report.Pass:
		return Pass, ch.BaseKnown + " didn't fire in head"
	case correctness[ch.ID] && h == report.NA && b != report.NA:
		return Inconclusive, statusText(b, ch.BaseKnown) + " → n/a: a correctness check couldn't tell in head, so it can't vouch for it"
	case b != h:
		return Pass, string(b) + " → " + string(h)
	}
	return Pass, ""
}

// judgeFlake judges a check head fails, outside the correctness checks,
// by the reps whose failure nothing explains (HeadFailed against
// BaseFailed), whether base failed it too or not:
//   - on every head rep where base didn't fail every rep: a fail;
//   - on significantly more head reps than base's (Fisher's exact test):
//     a fail;
//   - when base failed too, as often or more: both sides' (pass);
//   - else inconclusive. The lab has rare failures no signature explains
//     yet (a body broken after its first chunk, a window lowered outside
//     F13's pieces), on base and head alike: one such rep can't tell a
//     regression from the lab's flake, and ab's confirmation adds reps.
//
// So a base flake (one rep, or #195 over its rate) can't hide a head
// that fails every rep.
func judgeFlake(ch Check, change string) (Verdict, string) {
	bothFail := ch.Base == report.Fail
	if ch.HeadFailed == 0 || ch.HeadReps == 0 {
		if bothFail {
			return Pass, "fails on both sides (not this change's)"
		}
		return Fail, change
	}
	p := FisherGreater(ch.BaseFailed, ch.BaseReps, ch.HeadFailed, ch.HeadReps)
	counts := fmt.Sprintf("fails with no known signature on %d of %d head reps and %d of %d base (Fisher p %s)",
		ch.HeadFailed, ch.HeadReps, ch.BaseFailed, ch.BaseReps, PText(p))
	switch {
	case ch.HeadFailed == ch.HeadReps && (ch.BaseReps == 0 || ch.BaseFailed < ch.BaseReps):
		if !bothFail {
			return Fail, change
		}
		return Fail, change + ": " + counts + ": on every head rep"
	case p < Alpha:
		return Fail, change + ": " + counts
	case bothFail && ch.BaseReps > 0 && ch.HeadFailed*ch.BaseReps <= ch.BaseFailed*ch.HeadReps:
		return Pass, "fails on both sides about as often (not this change's): " + counts
	}
	return Inconclusive, change + ": " + counts + ": a flake or a regression? add reps"
}

func statusText(s report.Status, known string) string {
	if s == "" {
		return "absent"
	}
	if known != "" {
		return string(s) + " (" + known + ")"
	}
	return string(s)
}

// compareMetric judges one metric from both sides' values; def gives its
// kind, unit and better.
func compareMetric(scenario, name string, bv, hv []float64, def report.Metric, c conditions, headChecks []report.Check) Metric {
	m := Metric{Name: name, Kind: def.Kind, Unit: def.Unit, Better: def.Better,
		Base: sample(bv), Head: sample(hv), Status: Same}
	if m.Base.N == 0 || m.Head.N == 0 {
		m.Status, m.Note = Unjudged, "no values on a side"
		return m
	}
	if m.Kind == report.Exact {
		// An exact metric holds one value over the reps: any rep that
		// moved is a change, its direction the means'.
		m.Delta = mean(hv) - mean(bv)
	} else {
		// Counts and timings shift by the Hodges-Lehmann estimate, the
		// Mann-Whitney test's own: robust to reps in two clusters.
		m.Delta = HodgesLehmann(bv, hv)
	}
	if m.Base.Median != 0 {
		pct := m.Delta / math.Abs(m.Base.Median) * 100
		m.DeltaPct = &pct
	}
	base := math.Abs(m.Base.Median)
	bad, directed := worse(m.Better, m.Delta)
	if labOnly[name] {
		m.Note = "the lab's own cost: reported, never a verdict"
	}
	moved := func() MetricStatus {
		switch {
		case !directed, labOnly[name]:
			return Changed
		case bad:
			return Worse
		}
		return Better
	}

	switch m.Kind {
	case report.Exact:
		m.Rule = "any change on any rep"
		if slices.Equal(sortedCopy(bv), sortedCopy(hv)) {
			return m
		}
		m.Status = moved()
		if m.Delta == 0 {
			// The values moved both ways and the means agree: reported.
			m.Status, m.Note = Changed, "the values differ between reps, but not on the whole"
		}
		if check, ok := exactChecks[name]; ok && m.Status == Worse {
			if hc, ok := findCheck(headChecks, check); ok && (hc.Status == report.Fail || (hc.Status == report.XFail && Intermittent(hc.Known))) {
				// A failing check fails the comparison already, and an
				// intermittent issue (#225) moves the value on the reps it
				// fires. A deterministic one's value is stable: a change
				// is its own.
				m.Status = Changed
				m.Note = fmt.Sprintf("%s is %s in head", check, statusText(hc.Status, hc.Known))
			}
		}
		return m

	case report.Count:
		cv, calibrated := c.noise.CVFor(scenario, name)
		src := "calibrated"
		if !calibrated {
			cv, src = DefaultCV, "default"
		}
		floor := CountFloor(m.Unit)
		m.Threshold = math.Max(math.Max(CountRel*base, CVTimes*cv*base), floor)
		m.Rule = fmt.Sprintf("max(10%%, 3×CV %.3f %s, %s %s)", cv, src, report.Num(floor), m.Unit)
		if m.Base.N >= 2 && m.Head.N >= 2 {
			p := MannWhitney(bv, hv)
			m.P = &p
		}
		if math.Abs(m.Delta) <= m.Threshold {
			return m
		}
		m.Status = moved()
		switch {
		case !c.counts.Judged:
			m.Status, m.Note = Unjudged, c.counts.Why
		case m.Status == Changed:
		case m.Base.N < MinTimingReps || m.Head.N < MinTimingReps:
			// The lab's counts move with timing more than PLAN D5 hoped (a
			// waste ratio over a few bytes served, updates per MiB), so a
			// count gets a verdict on as many reps as a timing does.
			m.Status, m.Note = Unsure, fmt.Sprintf("needs %d reps a side (base %d, head %d): add reps", MinTimingReps, m.Base.N, m.Head.N)
		case *m.P >= Alpha:
			m.Status, m.Note = Unsure, fmt.Sprintf("past the threshold but not significant (p %s): add reps", PText(*m.P))
		case c.skew != "":
			m.Status, m.Note = Unsure, c.skew
		}
		return m
	}

	// Timing.
	floor := TimingFloor(name, m.Unit)
	m.Threshold = math.Max(TimingRel*base, floor)
	m.Rule = fmt.Sprintf("max(15%%, %s %s), p < %.2f, ≥%d reps a side", report.Num(floor), m.Unit, Alpha, MinTimingReps)
	if m.Base.N >= 2 && m.Head.N >= 2 {
		p := MannWhitney(bv, hv)
		m.P = &p
	}
	if math.Abs(m.Delta) <= m.Threshold {
		return m
	}
	m.Status = moved()
	switch {
	case !c.timing.Judged:
		m.Status, m.Note = Unjudged, c.timing.Why
	case m.Status == Changed:
	case m.Base.N < MinTimingReps || m.Head.N < MinTimingReps:
		m.Status, m.Note = Unsure, fmt.Sprintf("needs %d reps a side (base %d, head %d): add reps", MinTimingReps, m.Base.N, m.Head.N)
	case *m.P >= Alpha:
		m.Status, m.Note = Unsure, fmt.Sprintf("past the threshold but not significant (p %s): add reps", PText(*m.P))
	case c.skew != "":
		m.Status, m.Note = Unsure, c.skew
	}
	return m
}

func sample(v []float64) Sample {
	return Sample{N: len(v), Median: report.Quantile(v, 0.5), Values: v}
}

func sortedCopy(v []float64) []float64 {
	s := slices.Clone(v)
	slices.Sort(s)
	return s
}

// metricWhy is a metric's change on one line: "ttfb 120 → 5130 ms
// (+5010, +4175%, p 0.008)".
func metricWhy(m Metric) string {
	s := fmt.Sprintf("%s %s → %s %s (%s", m.Name, report.Num(m.Base.Median), report.Num(m.Head.Median), m.Unit, signed(m.Delta))
	if m.DeltaPct != nil {
		s += fmt.Sprintf(", %+.0f%%", *m.DeltaPct)
	}
	if m.P != nil {
		s += ", p " + PText(*m.P)
	}
	s += ")"
	if m.Note != "" && m.Status == Unsure {
		s += ": " + m.Note
	}
	return s
}

func signed(v float64) string {
	if v > 0 {
		return "+" + report.Num(v)
	}
	return report.Num(v)
}

func shortCommit(c string) string {
	if len(c) > 12 {
		return c[:12]
	}
	return c
}

// tally is how often an intermittent issue fired over a scenario's
// measured reps that ran a check it explains.
type tally struct {
	fired, reps int
	checks      []string
	// onCorrectness: it fired on a correctness check (quiescence,
	// watchdog) on some rep.
	onCorrectness bool
}

// firings counts each intermittent issue's firings over a report's
// measured repetitions, from its checks' per-rep outcomes ("fail:<IDs>").
func firings(r *report.Report) map[string]tally {
	out := map[string]tally{}
	for _, is := range known.Issues {
		if !is.Intermittent() {
			continue
		}
		var t tally
		var ran, fired []bool
		for _, c := range r.Checks {
			if !slices.Contains(is.Checks, c.ID) {
				continue
			}
			t.checks = append(t.checks, c.ID)
			for i, s := range c.Reps {
				for len(ran) <= i {
					ran, fired = append(ran, false), append(fired, false)
				}
				if s == string(report.NA) {
					continue
				}
				ran[i] = true
				if ids, ok := strings.CutPrefix(s, string(report.Fail)+":"); ok && slices.Contains(known.IDs(ids), is.ID) {
					fired[i] = true
					t.onCorrectness = t.onCorrectness || correctness[c.ID]
				}
			}
		}
		for i := range ran {
			if ran[i] {
				t.reps++
			}
			if fired[i] {
				t.fired++
			}
		}
		if t.reps > 0 {
			out[is.ID] = t
		}
	}
	return out
}

// runIssues judges each intermittent issue over every scenario both sides
// ran.
func runIssues(pairs []scenarioPair, c conditions) []Issue {
	bs, hs := map[string]tally{}, map[string]tally{}
	for _, p := range pairs {
		for _, side := range []struct {
			r   *report.Report
			sum map[string]tally
		}{{p.base, bs}, {p.head, hs}} {
			for id, t := range firings(side.r) {
				o := side.sum[id]
				o.fired += t.fired
				o.reps += t.reps
				o.onCorrectness = o.onCorrectness || t.onCorrectness
				side.sum[id] = o
			}
		}
	}
	var out []Issue
	for _, is := range known.Issues {
		ht, hok := hs[is.ID]
		bt, bok := bs[is.ID]
		if !hok && !bok {
			continue
		}
		// The runner fails a check when an issue fires over its rate over
		// the whole run (known.Env.Run), even where the scenario's own
		// reps don't: judged here the same way.
		out = append(out, judgeIssue(is, bt, ht, c, "the run's"))
	}
	return out
}

// judgeIssue judges an intermittent issue's firings: a fail when it fired
// over its rate in head (so head's check fails) and significantly more
// often than in base; a regression when it stayed within its rate but
// fired significantly more often; inconclusive when it fired over its
// rate without that, since a loaded machine makes it fire on both sides.
func judgeIssue(is known.Issue, bt, ht tally, c conditions, scope string) Issue {
	p := FisherGreater(bt.fired, bt.reps, ht.fired, ht.reps)
	i := Issue{ID: is.ID, BaseFired: bt.fired, BaseReps: bt.reps, HeadFired: ht.fired, HeadReps: ht.reps, P: p,
		Verdict: Pass, Checks: ht.checks, OnCorrectness: ht.onCorrectness}
	counts := fmt.Sprintf("%s fired on %d of %s %d head reps and %d of %d base (Fisher p %s)", is.ID, ht.fired, scope, ht.reps, bt.fired, bt.reps, PText(p))
	over := overRate(ht, is.Rate())
	// Skewed load excuses a timing-like issue (#195 stalling a play), never
	// one a correctness check failed on (#225's leak, a watchdog trip).
	guard := c.skew != "" && !ht.onCorrectness
	switch {
	case ht.reps > 0 && ht.fired == ht.reps && (ht.onCorrectness || bt.fired < bt.reps):
		// Every rep on both sides, outside a correctness check, is the
		// machine's (over its rate below): as a plain fail on both is.
		i.Verdict = Fail
		i.Note = counts + ": on every head rep, which no known rate explains (D6)"
	case p < Alpha && over && !guard:
		i.Verdict = Fail
		i.Note = counts + ": over its known rate and significantly more than base, a regression that looks like " + is.ID
	case p < Alpha && !guard:
		i.Verdict = Regression
		i.Note = counts + ": within its known rate, but significantly more than base"
	case guard && (over || p < Alpha):
		i.Verdict = Inconclusive
		i.Note = counts + ": more than base, under skewed load: rerun"
	case over:
		i.Verdict = Inconclusive
		i.Note = counts + ": over its known rate, but not significantly more than base (load?): add reps"
	default:
		i.Note = counts
	}
	return i
}

// overRate is known.Tally's rule: every rep, or two or more and more than
// rate of them.
func overRate(t tally, rate float64) bool {
	return t.reps > 0 && (t.fired == t.reps || (t.fired >= 2 && float64(t.fired)/float64(t.reps) > rate))
}
