package compare

import (
	"strings"
	"testing"

	"github.com/shapedtime/momoshtrem/internal/streamlab/report"
)

// firing is n reps of a check, the first k failing with id's signature.
func firing(id string, k, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = "pass"
		if i < k {
			out[i] = "fail:" + id
		}
	}
	return out
}

func check(r *Result, id string) Check {
	for _, sc := range r.Scenarios {
		for _, ch := range sc.Checks {
			if ch.ID == id {
				return ch
			}
		}
	}
	return Check{}
}

// An exact metric holds one value over the reps: a change on any rep is
// one, not only the median's.
func TestExactAnyRep(t *testing.T) {
	ex := func(v ...float64) *fixture {
		return newFixture(len(v)).metric("link_resolves", report.Exact, "resolves", report.Lower, v...)
	}
	r := judge(t, ex(1, 1, 1, 1, 1), ex(1, 1, 1, 3, 3), Options{})
	if m := metric(r, "link_resolves"); r.Verdict != Regression || m.Status != Worse || !near(m.Delta, 0.8, 1e-9) {
		t.Errorf("2 of 5 reps moved: %s %+v", r.Verdict, m)
	}
	// Moved both ways, the same on the whole: reported.
	if r := judge(t, ex(1, 1, 1, 1, 1), ex(0, 1, 1, 1, 2), Options{}); r.Verdict != Pass || metric(r, "link_resolves").Status != Changed {
		t.Errorf("both ways: %s %+v", r.Verdict, metric(r, "link_resolves"))
	}
	// The same values in another order: the same.
	if r := judge(t, ex(1, 2, 1, 1, 1), ex(1, 1, 1, 2, 1), Options{}); r.Verdict != Pass || metric(r, "link_resolves").Status != Same {
		t.Errorf("reordered: %s %+v", r.Verdict, metric(r, "link_resolves"))
	}
	// A deterministic issue's value is stable: an xfail on the judging
	// check doesn't excuse a change (an intermittent one does: TestExact).
	etag := func(v float64) *fixture {
		return newFixture(1).metric("etag_changes", report.Exact, "changes", report.Lower, v).check("etag-stable", report.XFail, "F2", "fail:F2")
	}
	if r := judge(t, etag(3), etag(4), Options{}); r.Verdict != Regression || metric(r, "etag_changes").Status != Worse {
		t.Errorf("under a deterministic xfail: %s %+v", r.Verdict, metric(r, "etag_changes"))
	}
}

// What head didn't measure it didn't reach: a directed metric measured in
// base only, or on fewer of head's reps (survivor bias), is inconclusive.
func TestMissingInHead(t *testing.T) {
	both := newFixture(5).timing("ttfb", steady(100, 5)...).timing("time_to_playable", steady(500, 5)...)
	ttfb := newFixture(5).timing("ttfb", steady(100, 5)...)
	r := judge(t, both, ttfb, Options{})
	if m := metric(r, "time_to_playable"); r.Verdict != Inconclusive || m.Status != Unsure || !strings.Contains(m.Note, "none of head's") {
		t.Errorf("base only: %s %+v", r.Verdict, m)
	}
	// Head only: nothing to compare it with, a note.
	if r := judge(t, ttfb, both, Options{}); r.Verdict != Pass || !strings.Contains(strings.Join(r.Scenarios[0].Notes, " "), "head only") {
		t.Errorf("head only: %s %v", r.Verdict, r.Scenarios[0].Notes)
	}
	// Undirected: a note.
	reads := newFixture(5).metric("reads", report.Count, "reads", report.None, 1, 2, 3, 4, 5)
	if r := judge(t, reads, newFixture(5), Options{}); r.Verdict != Pass {
		t.Errorf("undirected, base only: %s: %s", r.Verdict, r.Why)
	}
	// Measured on 2 of head's 5 reps, all of base's: the same values, but
	// head's 3 other reps never got playable.
	r = judge(t, newFixture(5).timing("time_to_playable", steady(500, 5)...), newFixture(5).timing("time_to_playable", 500, 502), Options{})
	if m := metric(r, "time_to_playable"); r.Verdict != Inconclusive || m.Status != Unsure || !strings.Contains(m.Note, "2 of 5 head reps") {
		t.Errorf("survivors: %s %+v", r.Verdict, m)
	}
	// A metric every rep measures isn't a survivor's.
	r = judge(t, newFixture(5).timing("stall_time", steady(500, 5)...), newFixture(5).timing("stall_time", 500, 502), Options{})
	if m := metric(r, "stall_time"); r.Verdict != Pass || m.Status != Same {
		t.Errorf("not a survivor: %s %+v", r.Verdict, m)
	}
}

// A correctness check that couldn't tell in head, or didn't run there,
// can't vouch for head: inconclusive. Other checks' n/a is fine; every
// check base ran is listed.
func TestCorrectnessNA(t *testing.T) {
	na := []string{"n/a", "n/a", "n/a", "n/a", "n/a"}
	r := judge(t, newFixture(5), newFixture(5).check("integrity", report.NA, "", na...), Options{})
	if r.Verdict != Inconclusive || !strings.Contains(r.Why, "check integrity: pass → n/a") {
		t.Errorf("integrity pass → n/a: %s: %s", r.Verdict, r.Why)
	}
	if r := judge(t, newFixture(5), newFixture(5).check("play-ok", report.NA, "", na...), Options{}); r.Verdict != Pass {
		t.Errorf("play-ok pass → n/a: %s: %s", r.Verdict, r.Why)
	}
	// n/a on both sides: nothing changed.
	both := newFixture(5).check("integrity", report.NA, "", na...)
	if r := judge(t, both, both, Options{}); r.Verdict != Pass {
		t.Errorf("n/a on both: %s: %s", r.Verdict, r.Why)
	}
	scriptOnly := newFixture(5)
	scriptOnly.checks = scriptOnly.checks[:1]
	r = judge(t, newFixture(5), scriptOnly, Options{})
	if r.Verdict != Inconclusive || len(r.Scenarios[0].Checks) != 5 {
		t.Fatalf("checks gone from head: %s, %d checks", r.Verdict, len(r.Scenarios[0].Checks))
	}
	for id, want := range map[string]Verdict{"integrity": Inconclusive, "http": Inconclusive, "quiescence": Inconclusive, "play-ok": Pass} {
		if ch := check(r, id); ch.Verdict != want || ch.Base != report.Pass || !strings.Contains(ch.Note, "base only") {
			t.Errorf("%s gone from head: %+v, want %s", id, ch, want)
		}
	}
}

// A failing rep counts unless its own signature explains the check (a
// signature outside the scenario's guards doesn't): one rep of five is a
// flake or a regression (inconclusive), as a plain fail is. #195's rep
// beside a plain fail is judgeIssue's to count, not judgeFlake's too
// (#279).
func TestUnexplainedReps(t *testing.T) {
	base := newFixture(5).check("play-ok", report.Pass, "", firing("", 0, 5)...)
	for name, tc := range map[string]struct {
		reps []string
		why  string
	}{
		"no signature":           {[]string{"fail", "pass", "pass", "pass", "pass"}, "1 of 5 head reps"},
		"F11 outside its guards": {[]string{"fail:F11", "pass", "pass", "pass", "pass"}, "1 of 5 head reps"},
		"#195 beside a fail":     {[]string{"fail:#195", "fail", "pass", "pass", "pass"}, "1 of 5 head reps"},
	} {
		r := judge(t, base, newFixture(5).check("play-ok", report.Fail, "", tc.reps...), Options{})
		if r.Verdict != Inconclusive || !strings.Contains(r.Why, tc.why) {
			t.Errorf("%s: %s: %s", name, r.Verdict, r.Why)
		}
	}
	// Guarded, F11 explains it: the runner's xfail, and a pass → xfail of
	// a deterministic issue is a regression (it's back).
	guarded := newFixture(5).check("play-ok", report.XFail, "F11", "fail:F11", "fail:F11", "fail:F11", "fail:F11", "fail:F11")
	if r := judge(t, base, guarded, Options{}); r.Verdict != Regression || !strings.Contains(r.Why, "known issue F11 is back") {
		t.Errorf("F11 back: %s: %s", r.Verdict, r.Why)
	}
	// Base failed as #195 fired over its rate: head not firing it isn't a
	// fix (judgeIssue judges the firings). Base's plain flake isn't either
	// (TestFlakyChecks); every base rep failing and head passing is.
	over := newFixture(5).check("play-ok", report.Fail, "#195", firing("#195", 3, 5)...)
	if r := judge(t, over, base, Options{}); r.Verdict != Pass || !strings.Contains(check(r, "play-ok").Note, "judged as an intermittent issue") {
		t.Errorf("base's #195: %s %+v", r.Verdict, check(r, "play-ok"))
	}
	broken := newFixture(5).check("play-ok", report.Fail, "", "fail", "fail", "fail", "fail", "fail")
	if r := judge(t, broken, base, Options{}); r.Verdict != Improvement {
		t.Errorf("base failed on every rep: %s: %s", r.Verdict, r.Why)
	}
}

// Each rep is judged by its own signature, whatever its sibling reps did
// (#279). One unsigned rep of a deterministic xfail check fails the
// runner's verdict on the whole check (Known ""), but the reps F13 signed
// stay F13's: an unchanged tree's ab once read 4 F13 reps and one unsigned
// as 5 of 5 head reps unexplained, a fail.
func TestOwnSignature(t *testing.T) {
	open := func(s report.Status, known string, reps ...string) *fixture {
		f := newFixture(len(reps))
		f.name, f.guards = "dual-reader-open", []string{"F13"}
		return f.check("open-keeps-windows", s, known, reps...)
	}
	f13 := func(reps ...string) *fixture { return open(report.XFail, "F13", reps...) }
	all := f13("fail:F13", "fail:F13", "fail:F13", "fail:F13", "fail:F13")
	someNA := f13("fail:F13", "fail:F13", "fail:F13", "n/a", "fail:F13")
	oneUnsigned := open(report.Fail, "", "fail:F13", "fail:F13", "fail:F13", "fail:F13", "fail")
	plain := open(report.Fail, "", "fail", "fail", "fail", "fail", "fail")
	counts := func(r *Result, headFailed, headReps, baseFailed, baseReps int) bool {
		ch := check(r, "open-keeps-windows")
		return ch.HeadFailed == headFailed && ch.HeadReps == headReps && ch.BaseFailed == baseFailed && ch.BaseReps == baseReps
	}

	// One unsigned head rep against none of base's: a flake or a
	// regression, whatever base's n/a.
	for name, tc := range map[string]struct {
		base     *fixture
		baseReps int
		why      string
	}{
		"base F13 on every rep": {all, 5, "1 of 5 head reps and 0 of 5 base (Fisher p 0.500)"},
		"base F13 and an n/a":   {someNA, 4, "1 of 5 head reps and 0 of 4 base (Fisher p 0.556)"},
	} {
		r := judge(t, tc.base, oneUnsigned, Options{})
		if r.Verdict != Inconclusive || !counts(r, 1, 5, 0, tc.baseReps) || !strings.Contains(r.Why, tc.why) {
			t.Errorf("%s: %s %+v: %s", name, r.Verdict, check(r, "open-keeps-windows"), r.Why)
		}
	}
	// The ab that showed it: the screen xfail on both sides, the
	// confirmation's head with one unsigned rep.
	screen := judge(t, all, f13("fail:F13", "fail:F13", "fail:F13", "fail:F13", "n/a"), Options{})
	if screen.Verdict != Pass {
		t.Fatalf("the screen: %s: %s", screen.Verdict, screen.Why)
	}
	r := Confirm(screen, judge(t, someNA, oneUnsigned, Options{}), Options{})
	if r.Verdict != Inconclusive || !strings.Contains(r.Why, "confirmation: check open-keeps-windows: xfail (F13) → fail: "+
		"fails with no known signature on 1 of 5 head reps and 0 of 4 base (Fisher p 0.556)") {
		t.Errorf("the ab: %s: %s", r.Verdict, r.Why)
	}

	// The mirror: base's one unsigned rep is 1 of 5, not 5 of 5, so a head
	// that fails every rep unsigned isn't "both sides about as often".
	if r := judge(t, oneUnsigned, plain, Options{}); r.Verdict != Fail || !counts(r, 5, 5, 1, 5) ||
		!strings.Contains(r.Why, "5 of 5 head reps and 1 of 5 base (Fisher p 0.024)") {
		t.Errorf("the mirror: %s %+v: %s", r.Verdict, check(r, "open-keeps-windows"), r.Why)
	}
	// Unsigned on every head rep against an xfail base: a fail, the
	// every-rep rule.
	if r := judge(t, all, plain, Options{}); r.Verdict != Fail || !counts(r, 5, 5, 0, 5) {
		t.Errorf("unsigned on every head rep: %s %+v: %s", r.Verdict, check(r, "open-keeps-windows"), r.Why)
	}

	// A signature the registry doesn't tie to the check explains nothing,
	// in a scenario that guards it too: F11 explains play-ok, not
	// open-keeps-windows.
	for _, sig := range []string{"F11", "F13+F11", "F99"} {
		head := open(report.Fail, "", "fail:F13", "fail:F13", "fail:F13", "fail:F13", "fail:"+sig)
		head.guards = append(head.guards, "F11")
		if r := judge(t, all, head, Options{}); r.Verdict != Inconclusive || !counts(r, 1, 5, 0, 5) {
			t.Errorf("one rep fail:%s: %s %+v", sig, r.Verdict, check(r, "open-keeps-windows"))
		}
	}
	f11 := open(report.Fail, "", firing("F11", 5, 5)...)
	f11.guards = append(f11.guards, "F11")
	if r := judge(t, all, f11, Options{}); r.Verdict != Fail || !counts(r, 5, 5, 0, 5) {
		t.Errorf("fail:F11 on every rep: %s %+v", r.Verdict, check(r, "open-keeps-windows"))
	}

	// Nor does one that can't happen on the host: F1 where the MIME table
	// resolves .mkv.
	for mkv, want := range map[string]int{"": 1, "video/x-matroska": 5} {
		sniff := func(s report.Status, reps ...string) *fixture {
			f := newFixture(5)
			f.guards, f.mkv = []string{"F1"}, mkv
			return f.check("no-sniff-reads", s, "", reps...)
		}
		r := judge(t, sniff(report.Pass, firing("", 0, 5)...), sniff(report.Fail, "fail:F1", "fail:F1", "fail:F1", "fail:F1", "fail"), Options{})
		if ch := check(r, "no-sniff-reads"); ch.HeadFailed != want {
			t.Errorf(".mkv %q: %d unexplained head reps, want %d: %+v", mkv, ch.HeadFailed, want, ch)
		}
	}
}

// The load guard excuses an issue that stalls a play (#195) under skewed
// load, never one a correctness check failed on (#225's leak), nor one
// that fired on every head rep (D6). Within its rate, an issue that fired
// significantly more often is a regression.
func TestIssuesUnderLoad(t *testing.T) {
	mk := func(ch, id string, k, n int, load float64) *fixture {
		f := newFixture(n)
		f.load = load
		s, known := report.Pass, ""
		switch {
		case k == 0:
		case overRate(tally{fired: k, reps: n}, 0.25):
			s, known = report.Fail, id
		default:
			s, known = report.XFail, id
		}
		return f.check(ch, s, known, firing(id, k, n)...)
	}
	for _, tc := range []struct {
		check, id string
		head, n   int
		skew      bool
		want      Verdict
	}{
		{"quiescence", "#225", 5, 5, false, Fail},
		{"quiescence", "#225", 5, 5, true, Fail},        // every rep: no load explains it
		{"play-ok", "#195", 5, 5, true, Fail},           // every rep
		{"quiescence", "#225", 10, 20, true, Fail},      // a leak: the guard doesn't apply
		{"play-ok", "#195", 10, 20, false, Fail},        // over its rate, significantly more
		{"play-ok", "#195", 10, 20, true, Inconclusive}, // the same under skewed load
		{"play-ok", "#195", 5, 20, false, Regression},   // within its rate (5 of 20), significantly more
		{"play-ok", "#195", 5, 20, true, Inconclusive},
		{"play-ok", "#195", 2, 20, false, Pass}, // within its rate, not significantly more
	} {
		load := 0.4
		if tc.skew {
			load = 3.0
		}
		r := judge(t, mk(tc.check, tc.id, 0, tc.n, 0.4), mk(tc.check, tc.id, tc.head, tc.n, load), Options{})
		if r.Verdict != tc.want {
			t.Errorf("%s on %d of %d head reps, skewed %v: %s, want %s: %s", tc.id, tc.head, tc.n, tc.skew, r.Verdict, tc.want, r.Why)
		}
	}
}

// Each scenario within its rate, the run's firings significantly more:
// a regression of the run, which a single compare can't confirm.
func TestRunWideIssue(t *testing.T) {
	var bs, hs []*fixture
	for i := range 22 {
		b, h := newFixture(5), newFixture(5)
		b.name = string(rune('a'+i)) + "-sc"
		h.name = b.name
		if i < 6 {
			h.check("play-ok", report.XFail, "#195", firing("#195", 1, 5)...)
		}
		bs, hs = append(bs, b), append(hs, h)
	}
	r, err := Compare(side(bs...), side(hs...), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if r.Verdict != Regression || !strings.Contains(r.Why, "over the run: #195 fired on 6 of the run's 110 head reps") || len(ToConfirm(r)) != 0 {
		t.Fatalf("the run: %s: %s; ToConfirm %v", r.Verdict, r.Why, ToConfirm(r))
	}
	for _, sc := range r.Scenarios {
		if sc.Verdict != Pass {
			t.Errorf("%s: %s", sc.Name, sc.Verdict)
		}
	}
	if u := Unconfirmed(r); u.Verdict != Inconclusive || !strings.Contains(u.Why, "the scenarios' to confirm") {
		t.Errorf("unconfirmed: %s: %s", u.Verdict, u.Why)
	}
}

// Hangs and crashes in head are fails, the warm-up's included.
func TestBrokenWarmup(t *testing.T) {
	head := side(newFixture(5))
	head.Reports[0].Reps = append(head.Reports[0].Reps, report.Rep{Rep: 0, Warmup: true, Status: report.RepCrash, Error: "panic: nil map"})
	r, err := Compare(side(newFixture(5)), head, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if r.Verdict != Fail || !strings.Contains(r.Why, "head's the warm-up crash: panic: nil map") {
		t.Errorf("%s: %s", r.Verdict, r.Why)
	}
}

// The worst verdict wins: fail, regression, inconclusive, improvement,
// pass.
func TestVerdictPrecedence(t *testing.T) {
	order := []Verdict{Pass, Improvement, Inconclusive, Regression, Fail}
	status := map[Verdict]MetricStatus{Improvement: Better, Inconclusive: Unsure, Regression: Worse}
	for i, lo := range order {
		for _, hi := range order[i+1:] {
			for _, pair := range [][2]Verdict{{lo, hi}, {hi, lo}} {
				sc := Scenario{Name: "s"}
				for _, v := range pair {
					switch v {
					case Fail:
						sc.Broken = append(sc.Broken, "a hang")
					case Pass:
						sc.Metrics = append(sc.Metrics, Metric{Name: "same", Status: Same})
					default:
						sc.Metrics = append(sc.Metrics, Metric{Name: string(v), Status: status[v]})
					}
				}
				sc.settle()
				if sc.Verdict != hi {
					t.Errorf("scenario %v: %s, want %s", pair, sc.Verdict, hi)
				}
				r := &Result{Scenarios: []Scenario{{Name: "a", Verdict: pair[0]}, {Name: "b", Verdict: pair[1]}}}
				r.settle()
				if r.Verdict != hi {
					t.Errorf("result %v: %s, want %s", pair, r.Verdict, hi)
				}
			}
		}
	}
	// A run-wide issue counts as a scenario's does.
	r := &Result{Scenarios: []Scenario{{Name: "a", Verdict: Inconclusive}}, Intermittent: []Issue{{ID: "#195", Verdict: Regression, Note: "x"}}}
	if r.settle(); r.Verdict != Regression || r.Why != "over the run: x" {
		t.Errorf("run-wide: %s: %s", r.Verdict, r.Why)
	}
}

// A check failing on both sides is judged by its reps too: a base flake
// (one rep, or #195 over its rate) can't hide a head that fails every
// rep. Both failing about as often is both sides' (pass).
func TestBothSidesFail(t *testing.T) {
	plain := func(fails int) *fixture {
		reps := []string{"pass", "pass", "pass", "pass", "pass"}
		for i := range fails {
			reps[i] = "fail"
		}
		s := report.Pass
		if fails > 0 {
			s = report.Fail
		}
		return newFixture(5).check("play-ok", s, "", reps...)
	}
	over195 := newFixture(5).check("play-ok", report.Fail, "#195", "fail:#195", "fail:#195", "pass", "pass", "pass")
	for _, tc := range []struct {
		name       string
		base, head *fixture
		want       Verdict
		why        string
	}{
		{"a base flake, head every rep", plain(1), plain(5), Fail, "5 of 5 head reps and 1 of 5 base (Fisher p 0.024)"},
		{"#195 over its rate in base, head every rep", over195, plain(5), Fail, "5 of 5 head reps and 0 of 5 base"},
		{"every rep on both", plain(5), plain(5), Pass, ""},
		{"head less often", plain(2), plain(1), Pass, ""},
		{"head more often, not significantly", plain(1), plain(3), Inconclusive, "a flake or a regression? add reps"},
	} {
		r := judge(t, tc.base, tc.head, Options{})
		if u := Unconfirmed(judge(t, tc.base, tc.head, Options{})); r.Verdict != tc.want || u.Verdict != tc.want || !strings.Contains(r.Why, tc.why) {
			t.Errorf("%s: %s (standalone %s): %s", tc.name, r.Verdict, u.Verdict, r.Why)
		}
	}
	// ab's confirmation judges both runs' reps: 1 of 5 against 3 of 5,
	// then 1 of 5 against 5 of 5, is 2 of 10 against 8 of 10.
	screen := judge(t, plain(1), plain(3), Options{})
	if r := Confirm(screen, judge(t, plain(1), plain(5), Options{}), Options{}); r.Verdict != Fail || !strings.Contains(r.Why, "8 of 10 head reps and 2 of 10 base") {
		t.Errorf("confirmed: %s: %s", r.Verdict, r.Why)
	}
}

// When base failed a check too, head failing on every rep is judged by
// Fisher's test alone: base 1 of 5 against 5 of 5 fails (p 0.024), 2 of 5
// (p 0.083) and 4 of 5 (p 0.5) are inconclusive and go to confirmation. A
// correctness check failing on both sides fails whatever the counts (D6).
func TestBothSidesFisher(t *testing.T) {
	five := func(id string, fails int) *fixture {
		reps := []string{"pass", "pass", "pass", "pass", "pass"}
		for i := range fails {
			reps[i] = "fail"
		}
		return newFixture(5).check(id, report.Fail, "", reps...)
	}
	for _, tc := range []struct {
		id         string
		base, head int
		want       Verdict
		why        string
	}{
		{"play-ok", 1, 5, Fail, "Fisher p 0.024"},
		{"play-ok", 2, 5, Inconclusive, "Fisher p 0.083"},
		{"play-ok", 3, 5, Inconclusive, "Fisher p 0.222"},
		{"play-ok", 4, 5, Inconclusive, "Fisher p 0.500"},
		{"play-ok", 5, 5, Pass, ""},
		{"integrity", 5, 1, Fail, "it fails in base too"},
		{"quiescence", 1, 1, Fail, "it fails in base too"},
	} {
		r := judge(t, five(tc.id, tc.base), five(tc.id, tc.head), Options{})
		if r.Verdict != tc.want || !strings.Contains(r.Why, tc.why) {
			t.Errorf("%s %d of 5 base, %d of 5 head: %s: %s", tc.id, tc.base, tc.head, r.Verdict, r.Why)
		}
		if tc.want == Inconclusive && len(ToConfirm(r)) != 1 {
			t.Errorf("%s %d v %d: not confirmed: %v", tc.id, tc.base, tc.head, ToConfirm(r))
		}
	}
}

// An intermittent issue on every rep of both sides is a loaded machine
// (inconclusive), as a plain fail on both sides passes; on a correctness
// check it stays a fail.
func TestEveryRepBothSides(t *testing.T) {
	all := func(ch, id string) *fixture {
		return newFixture(5).check(ch, report.Fail, id, firing(id, 5, 5)...)
	}
	if r := judge(t, all("play-ok", "#195"), all("play-ok", "#195"), Options{}); r.Verdict != Inconclusive || !strings.Contains(r.Why, "not significantly more than base") {
		t.Errorf("#195 on every rep of both: %s: %s", r.Verdict, r.Why)
	}
	if r := judge(t, all("quiescence", "#225"), all("quiescence", "#225"), Options{}); r.Verdict != Fail {
		t.Errorf("#225 on every rep of both: %s: %s", r.Verdict, r.Why)
	}
}
