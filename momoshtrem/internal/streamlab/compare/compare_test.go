package compare

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/shapedtime/momoshtrem/internal/streamlab/report"
)

// fixture builds a scenario's report over n measured reps: every rep on
// one machine at load (per CPU), its metrics' values and checks as given.
type fixture struct {
	name    string
	n       int
	load    float64
	race    bool
	host    string
	metrics map[string]report.Metric
	checks  []report.Check
	broken  report.RepStatus
}

func newFixture(n int) *fixture {
	f := &fixture{name: "cold-start", n: n, load: 0.4, host: "mac", metrics: map[string]report.Metric{}}
	ok := make([]string, n)
	for i := range ok {
		ok[i] = "pass"
	}
	for _, id := range []string{"script", "integrity", "http", "play-ok", "quiescence"} {
		f.checks = append(f.checks, report.Check{ID: id, Status: report.Pass, Reps: ok})
	}
	return f
}

func (f *fixture) metric(name string, kind report.Kind, unit string, better report.Better, values ...float64) *fixture {
	f.metrics[name] = report.NewMetric(report.MetricDef{Name: name, Kind: kind, Unit: unit, Better: better}, values)
	return f
}

func (f *fixture) timing(name string, values ...float64) *fixture {
	return f.metric(name, report.Timing, "ms", report.Lower, values...)
}

func (f *fixture) check(id string, s report.Status, known string, reps ...string) *fixture {
	for i := range f.checks {
		if f.checks[i].ID == id {
			f.checks[i] = report.Check{ID: id, Status: s, Known: known, Reps: reps}
			return f
		}
	}
	f.checks = append(f.checks, report.Check{ID: id, Status: s, Known: known, Reps: reps})
	return f
}

func (f *fixture) report() *report.Report {
	env := report.Env{Host: f.host, OS: "darwin", Arch: "arm64", CPU: "M1", NumCPU: 10, Race: f.race, Load: []float64{f.load * 10, 1, 1}}
	r := &report.Report{Schema: report.Schema, Env: env, Verdict: report.VerdictPass, Metrics: f.metrics, Checks: f.checks,
		Tool:     report.Tool{Git: report.Git{Commit: "abc"}},
		Scenario: report.Scenario{Name: f.name, Params: map[string]string{"seed": "1"}}}
	for i := range f.n {
		e := env
		r.Reps = append(r.Reps, report.Rep{Rep: i + 1, Status: report.RepOK, Env: &e})
	}
	if f.broken != "" {
		r.Reps = append(r.Reps, report.Rep{Rep: f.n + 1, Status: f.broken, Error: "stuck"})
		r.Verdict = report.VerdictFail
	}
	for _, c := range f.checks {
		if c.Status == report.Fail {
			r.Verdict = report.VerdictFail
		}
	}
	return r
}

func side(fs ...*fixture) *Side {
	s := &Side{Path: "test"}
	for _, f := range fs {
		s.Reports = append(s.Reports, f.report())
	}
	return s
}

func judge(t *testing.T, base, head *fixture, opts Options) *Result {
	t.Helper()
	r, err := Compare(side(base), side(head), opts)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func metric(r *Result, name string) Metric {
	for _, sc := range r.Scenarios {
		for _, m := range sc.Metrics {
			if m.Name == name {
				return m
			}
		}
	}
	return Metric{}
}

func steady(v float64, n int) []float64 {
	out := make([]float64, n)
	for i := range out {
		out[i] = v + float64(i) // a little spread, no ties
	}
	return out
}

func TestUnchangedPasses(t *testing.T) {
	base := newFixture(5).timing("ttfb", 100, 140, 90, 120, 110).metric("reads", report.Count, "reads", report.None, 50, 52, 49, 51, 50)
	head := newFixture(5).timing("ttfb", 115, 95, 130, 105, 125).metric("reads", report.Count, "reads", report.None, 51, 50, 50, 52, 49)
	r := judge(t, base, head, Options{})
	if r.Verdict != Pass || r.Verdict.ExitCode() != ExitPass {
		t.Fatalf("verdict %s: %s", r.Verdict, r.Why)
	}
	if !strings.HasPrefix(r.Markdown(), "**PASS**") {
		t.Errorf("compare.md doesn't start with its verdict:\n%s", r.Markdown())
	}
	if !strings.Contains(strings.Join(r.Notes, " "), "No calibration") {
		t.Errorf("an uncalibrated compare should say so: %v", r.Notes)
	}
}

// A timing regression needs the threshold, significance and 5 reps.
func TestTimingRegression(t *testing.T) {
	base := newFixture(5).timing("activation_wait", steady(100, 5)...)
	head := newFixture(5).timing("activation_wait", steady(5000, 5)...)
	r := judge(t, base, head, Options{})
	m := metric(r, "activation_wait")
	if r.Verdict != Regression || m.Status != Worse || m.P == nil || *m.P >= Alpha {
		t.Fatalf("verdict %s, metric %+v", r.Verdict, m)
	}
	if !strings.Contains(r.Why, "activation_wait 102 → 5002 ms") {
		t.Errorf("why doesn't name the metric: %s", r.Why)
	}
	// Faster is an improvement.
	if r := judge(t, head, base, Options{}); r.Verdict != Improvement || r.Verdict.ExitCode() != ExitPass {
		t.Errorf("reversed: %s: %s", r.Verdict, r.Why)
	}
	// Too few reps: inconclusive, add reps.
	r = judge(t, newFixture(3).timing("activation_wait", steady(100, 3)...), newFixture(3).timing("activation_wait", steady(5000, 3)...), Options{})
	if r.Verdict != Inconclusive || r.Verdict.ExitCode() != ExitInconclusive || !strings.Contains(r.Why, "add reps") {
		t.Errorf("3 reps: %s: %s", r.Verdict, r.Why)
	}
	// Past the threshold, but the reps overlap: not significant.
	r = judge(t, newFixture(5).timing("ttfb", 100, 300, 500, 700, 900), newFixture(5).timing("ttfb", 400, 600, 800, 1000, 1200), Options{})
	if m := metric(r, "ttfb"); r.Verdict != Inconclusive || m.Status != Unsure || m.Delta != 300 {
		t.Errorf("overlapping: %s %+v", r.Verdict, m)
	}
	// Reps in two clusters, mixed alike on both sides: the medians differ
	// a lot, the Hodges-Lehmann shift doesn't. The same.
	r = judge(t, newFixture(5).timing("ttfb", 100, 2000, 110, 2100, 105), newFixture(5).timing("ttfb", 2000, 120, 2050, 115, 2200), Options{})
	if m := metric(r, "ttfb"); r.Verdict != Pass || m.Status != Same {
		t.Errorf("two clusters: %s %+v", r.Verdict, m)
	}
	// Significant but under the floor (100 ms): the same.
	r = judge(t, newFixture(5).timing("ttfb", steady(20, 5)...), newFixture(5).timing("ttfb", steady(60, 5)...), Options{})
	if r.Verdict != Pass {
		t.Errorf("under the floor: %s: %s", r.Verdict, r.Why)
	}
}

// The lab's own cost is reported, never a verdict.
func TestLabOnly(t *testing.T) {
	over := func(f *fixture, v ...float64) *fixture {
		return f.metric("observe_overhead", report.Timing, "%", report.Lower, v...)
	}
	r := judge(t, over(newFixture(5), 0.20, 0.21, 0.22, 0.20, 0.21), over(newFixture(5), 0.40, 0.41, 0.42, 0.40, 0.43), Options{})
	if m := metric(r, "observe_overhead"); r.Verdict != Pass || m.Status != Changed {
		t.Errorf("%s %+v", r.Verdict, m)
	}
}

// Race builds and other machines give no timing verdict; skewed load
// makes one inconclusive.
func TestTimingConditions(t *testing.T) {
	base := newFixture(5).timing("ttfb", steady(100, 5)...)
	head := newFixture(5).timing("ttfb", steady(5000, 5)...)
	head.race = true
	r := judge(t, base, head, Options{})
	if r.Verdict != Pass || r.Timing.Judged || metric(r, "ttfb").Status != Unjudged {
		t.Errorf("race build: %s %+v %+v", r.Verdict, r.Timing, metric(r, "ttfb"))
	}

	head.race, head.host = false, "linux-box"
	if _, err := Compare(side(base), side(head), Options{}); !errors.Is(err, ErrMachines) {
		t.Errorf("two machines: %v", err)
	}
	r, err := Compare(side(base), side(head), Options{Force: true})
	if err != nil || r.Verdict != Pass || metric(r, "ttfb").Status != Unjudged {
		t.Errorf("forced: %v %+v", err, r)
	}

	head.host, head.load = "mac", 3.0
	r = judge(t, base, head, Options{})
	if r.Verdict != Inconclusive || r.LoadSkew == "" || !strings.Contains(metric(r, "ttfb").Note, "different load") {
		t.Errorf("load skew: %s %q %+v", r.Verdict, r.LoadSkew, metric(r, "ttfb"))
	}

	head.load = 0.4
	headParams := side(head)
	headParams.Reports[0].Scenario.Params["seed"] = "2"
	if _, err := Compare(side(base), headParams, Options{}); !errors.Is(err, ErrParams) {
		t.Errorf("other params: %v", err)
	}
}

func TestCounts(t *testing.T) {
	count := func(f *fixture, v ...float64) *fixture {
		return f.metric("read_timeouts", report.Count, "reads", report.Lower, v...)
	}
	// 10 → 20 timeouts on every rep: past max(10%, 3×0.1, 1), significant.
	r := judge(t, count(newFixture(5), 10, 11, 10, 9, 10), count(newFixture(5), 20, 21, 19, 20, 22), Options{})
	if m := metric(r, "read_timeouts"); r.Verdict != Regression || m.Status != Worse {
		t.Errorf("counts: %s %+v", r.Verdict, m)
	}
	// A calibrated CV of 0.5 puts the threshold at 3×0.5×10 = 15.
	cal := &Calibration{Commit: "c", Date: time.Now(), Reps: 10, CV: map[string]map[string]float64{"cold-start": {"read_timeouts": 0.5}}}
	r = judge(t, count(newFixture(5), 10, 11, 10, 9, 10), count(newFixture(5), 20, 21, 19, 20, 22), Options{Noise: cal, NoiseSource: "noise.json"})
	if m := metric(r, "read_timeouts"); r.Verdict != Pass || !strings.Contains(m.Rule, "calibrated") {
		t.Errorf("calibrated: %s %+v", r.Verdict, m)
	}
	// One rep a side: past the threshold, but no verdict.
	r = judge(t, count(newFixture(1), 10), count(newFixture(1), 20), Options{})
	if m := metric(r, "read_timeouts"); r.Verdict != Inconclusive || m.Status != Unsure {
		t.Errorf("one rep: %s %+v", r.Verdict, m)
	}
	// A median moving by one event isn't a change (the floor).
	r = judge(t, count(newFixture(5), 0, 0, 0, 1, 0), count(newFixture(5), 1, 1, 0, 1, 1), Options{})
	if r.Verdict != Pass {
		t.Errorf("one event: %s: %s", r.Verdict, r.Why)
	}
}

func TestExact(t *testing.T) {
	exact := func(f *fixture, v ...float64) *fixture {
		return f.metric("leaked_goroutines", report.Exact, "goroutines", report.Lower, v...)
	}
	r := judge(t, exact(newFixture(1), 0), exact(newFixture(1), 2), Options{})
	if m := metric(r, "leaked_goroutines"); r.Verdict != Regression || m.Status != Worse {
		t.Errorf("exact: %s %+v", r.Verdict, m)
	}
	// A known issue explains it: reported, no regression.
	head := exact(newFixture(1), 2).check("quiescence", report.XFail, "#225", "fail:#225")
	r = judge(t, exact(newFixture(1), 0), head, Options{})
	if m := metric(r, "leaked_goroutines"); m.Status != Changed || r.Verdict == Regression {
		t.Errorf("explained: %s %+v", r.Verdict, m)
	}
}

func TestChecks(t *testing.T) {
	// A correctness check failing in head fails, whatever base did.
	for _, b := range []report.Status{report.Pass, report.Fail} {
		base := newFixture(1).check("integrity", b, "", string(b))
		head := newFixture(1).check("integrity", report.Fail, "", "fail")
		if r := judge(t, base, head, Options{}); r.Verdict != Fail || r.Verdict.ExitCode() != ExitFail {
			t.Errorf("integrity %s → fail: %s", b, r.Verdict)
		}
	}
	// An xfail in head is a known issue's: no fail.
	base := newFixture(1).check("integrity", report.XFail, "#231", "fail:#231")
	if r := judge(t, base, base, Options{}); r.Verdict != Pass {
		t.Errorf("xfail on both: %s: %s", r.Verdict, r.Why)
	}
	// Another check: pass or xfail → fail fails; fail → fail doesn't.
	for b, want := range map[report.Status]Verdict{report.Pass: Fail, report.XFail: Fail, report.Fail: Pass} {
		base := newFixture(1).check("etag-stable", b, "", string(b))
		head := newFixture(1).check("etag-stable", report.Fail, "", "fail")
		if r := judge(t, base, head, Options{}); r.Verdict != want {
			t.Errorf("etag-stable %s → fail: %s, want %s", b, r.Verdict, want)
		}
	}
	// xfail → xpass: a known issue appears fixed.
	r := judge(t, newFixture(1).check("etag-stable", report.XFail, "F2", "fail:F2"), newFixture(1).check("etag-stable", report.XPass, "F2", "pass"), Options{})
	if r.Verdict != Improvement || !strings.Contains(r.Why, "known issue F2 appears fixed") {
		t.Errorf("xpass: %s: %s", r.Verdict, r.Why)
	}
	// A hung rep in head fails.
	head := newFixture(1)
	head.broken = report.RepTimeout
	if r := judge(t, newFixture(1), head, Options{}); r.Verdict != Fail || !strings.Contains(r.Why, "timeout") {
		t.Errorf("hang: %s: %s", r.Verdict, r.Why)
	}
}

// A check base passed that fails on some head reps with no known
// signature is judged by its reps: one rep of five can be the lab's
// flake, every rep is a fail; a correctness check fails on one.
func TestFlakyChecks(t *testing.T) {
	five := func(fails int) []string {
		out := []string{"pass", "pass", "pass", "pass", "pass"}
		for i := range fails {
			out[i] = "fail"
		}
		return out
	}
	base := newFixture(5).check("play-ok", report.Pass, "", five(0)...)
	r := judge(t, base, newFixture(5).check("play-ok", report.Fail, "", five(1)...), Options{})
	if r.Verdict != Inconclusive || !strings.Contains(r.Why, "1 of 5 head reps and 0 of 5 base") {
		t.Errorf("one rep: %s: %s", r.Verdict, r.Why)
	}
	if got := ToConfirm(r); len(got) != 1 {
		t.Errorf("a flaky check isn't confirmed: %v", got)
	}
	// Confirmed over both runs: still one of ten, still inconclusive;
	// five more failing reps make it a fail.
	clean := judge(t, base, newFixture(5).check("play-ok", report.Pass, "", five(0)...), Options{})
	if c := Confirm(r, clean, Options{}); c.Verdict != Inconclusive || !strings.Contains(c.Why, "1 of 10 head reps") {
		t.Errorf("confirmed clean: %s: %s", c.Verdict, c.Why)
	}
	r = judge(t, base, newFixture(5).check("play-ok", report.Fail, "", five(1)...), Options{})
	bad := judge(t, base, newFixture(5).check("play-ok", report.Fail, "", five(5)...), Options{})
	if c := Confirm(r, bad, Options{}); c.Verdict != Fail {
		t.Errorf("confirmed failing: %s: %s", c.Verdict, c.Why)
	}
	// Every rep fails: a fail at once.
	if r := judge(t, base, newFixture(5).check("play-ok", report.Fail, "", five(5)...), Options{}); r.Verdict != Fail {
		t.Errorf("every rep: %s", r.Verdict)
	}
	// Base's flake isn't head's fix: no improvement.
	if r := judge(t, newFixture(5).check("play-ok", report.Fail, "", five(1)...), base, Options{}); r.Verdict != Pass {
		t.Errorf("base's flake: %s: %s", r.Verdict, r.Why)
	}
	// A correctness check fails on one rep: a fail.
	ib := newFixture(5).check("integrity", report.Pass, "", five(0)...)
	if r := judge(t, ib, newFixture(5).check("integrity", report.Fail, "", five(1)...), Options{}); r.Verdict != Fail {
		t.Errorf("integrity on one rep: %s", r.Verdict)
	}
}

// #195 over its rate in head: a fail only when it fired significantly
// more than in base, which ran under the same load.
func TestIntermittent(t *testing.T) {
	reps := func(fired, n int) []string {
		out := make([]string, n)
		for i := range out {
			out[i] = "pass"
			if i < fired {
				out[i] = "fail:#195"
			}
		}
		return out
	}
	play := func(f *fixture, fired int) *fixture {
		s, known := report.Pass, ""
		switch {
		case fired == 0:
		case overRate(tally{fired: fired, reps: f.n}, 0.25):
			s, known = report.Fail, "#195"
		default:
			s, known = report.XFail, "#195"
		}
		return f.check("play-ok", s, known, reps(fired, f.n)...)
	}
	for _, tc := range []struct {
		base, head int
		want       Verdict
	}{
		{0, 0, Pass},
		{1, 1, Pass},         // within its rate on both sides
		{2, 3, Inconclusive}, // over its rate on both: a loaded machine
		{1, 2, Inconclusive}, // over its rate in head, not significantly more
		{0, 5, Fail},         // every head rep, none in base: a regression that looks like #195
	} {
		r := judge(t, play(newFixture(5), tc.base), play(newFixture(5), tc.head), Options{})
		if r.Verdict != tc.want {
			t.Errorf("#195 on %d vs %d of 5: %s, want %s: %s", tc.base, tc.head, r.Verdict, tc.want, r.Why)
		}
	}
}

// Confirm keeps a screen's statistical verdict only when fresh reps
// show it too.
func TestConfirm(t *testing.T) {
	mk := func(baseV, headV float64) *Result {
		return judge(t, newFixture(5).timing("ttfb", steady(baseV, 5)...), newFixture(5).timing("ttfb", steady(headV, 5)...), Options{})
	}
	for _, tc := range []struct {
		screen, again float64
		want          Verdict
	}{
		{5000, 5000, Regression},  // it holds
		{5000, 1000, Regression},  // worse again, if less: still a regression
		{5000, 100, Inconclusive}, // the fresh reps contradict it: add reps
		{260, 100, Pass},          // within the threshold over both runs: the screen's was noise
		{5000, 20, Inconclusive},  // better in the fresh reps: contradicted
	} {
		screen := mk(100, tc.screen)
		if got := ToConfirm(screen); fmt.Sprint(got) != "[cold-start]" {
			t.Fatalf("ToConfirm = %v", got)
		}
		r := Confirm(screen, mk(100, tc.again), Options{})
		if r.Verdict != tc.want || r.Confirmed == nil {
			t.Errorf("again at %v: %s, want %s: %s", tc.again, r.Verdict, tc.want, r.Why)
		}
		if m := metric(r, "ttfb"); m.Screen != Worse || m.Base.N != 10 {
			t.Errorf("again at %v: metric %+v", tc.again, m)
		}
	}
	// A screen that wasn't significant isn't replicated by fresh reps that
	// are: that's a second screen, and one of two passing by chance stays
	// inconclusive.
	unsure := judge(t, newFixture(5).timing("ttfb", 100, 300, 500, 700, 900), newFixture(5).timing("ttfb", 400, 600, 800, 1000, 1200), Options{})
	if m := metric(unsure, "ttfb"); m.Status != Unsure {
		t.Fatalf("the screen: %+v", m)
	}
	if r := Confirm(unsure, mk(100, 5000), Options{}); r.Verdict != Inconclusive {
		t.Errorf("an unsure screen, a significant confirmation: %s: %s", r.Verdict, r.Why)
	}
	// A fail in the confirmation counts.
	head := newFixture(5).timing("ttfb", steady(100, 5)...).check("integrity", report.Fail, "", "fail")
	again, err := Compare(side(newFixture(5).timing("ttfb", steady(100, 5)...)), side(head), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if r := Confirm(mk(100, 5000), again, Options{}); r.Verdict != Fail {
		t.Errorf("a fail in the confirmation: %s", r.Verdict)
	}
}
