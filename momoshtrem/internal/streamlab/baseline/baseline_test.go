package baseline

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/shapedtime/momoshtrem/internal/streamlab/report"
	"github.com/shapedtime/momoshtrem/internal/streamlab/scenario"
)

func rep(name string, checks map[string]report.Check, exact map[string][]float64) *report.Report {
	r := &report.Report{Schema: report.Schema, Verdict: report.VerdictPass, Env: report.Env{OS: "darwin"},
		Scenario: report.Scenario{Name: name}, Metrics: map[string]report.Metric{}}
	for _, id := range []string{"script", "integrity", "http", "play-ok", "quiescence"} {
		c, ok := checks[id]
		if !ok {
			c = report.Check{ID: id, Status: report.Pass}
		}
		r.Checks = append(r.Checks, c)
	}
	for id, c := range checks {
		if !slices.ContainsFunc(r.Checks, func(x report.Check) bool { return x.ID == id }) {
			r.Checks = append(r.Checks, c)
		}
		if c.Status == report.Fail {
			r.Verdict = report.VerdictFail
		}
	}
	n := 0
	for name, v := range exact {
		r.Metrics[name] = report.NewMetric(report.MetricDef{Name: name, Kind: report.Exact}, v)
		n = max(n, len(v))
	}
	r.Metrics["ttfb"] = report.NewMetric(report.MetricDef{Name: "ttfb", Kind: report.Timing}, []float64{120})
	for i := range max(n, 1) {
		r.Reps = append(r.Reps, report.Rep{Rep: i + 1, Status: report.RepOK})
	}
	return r
}

func update(reason string) Update {
	return Update{Date: time.Date(2026, 10, 17, 0, 0, 0, 0, time.UTC), Commit: "abc", Reason: reason}
}

func TestFromRun(t *testing.T) {
	r := rep("etag-stability", map[string]report.Check{
		"etag-stable": {ID: "etag-stable", Status: report.XFail, Known: "F2"},
		"play-ok":     {ID: "play-ok", Status: report.XFail, Known: "#195"},
	}, map[string][]float64{"etag_changes": {3, 3}, "readers_created": {2, 3}})
	bl, err := FromRun([]*report.Report{r}, nil, update("first"), true)
	if err != nil {
		t.Fatal(err)
	}
	sc := bl.Scenarios["etag-stability"]
	if sc.Checks["etag-stable"] != "xfail:F2" {
		t.Errorf("a deterministic known issue is an expectation: %v", sc.Checks)
	}
	if sc.Checks["play-ok"] != "pass" {
		t.Errorf("an intermittent issue that happened to fire is no expectation: %v", sc.Checks)
	}
	if sc.Exact["etag_changes"] != 3 || !slices.Equal(sc.Unstable, []string{"readers_created"}) {
		t.Errorf("exact %v, unstable %v", sc.Exact, sc.Unstable)
	}
	if _, ok := sc.Exact["ttfb"]; ok {
		t.Error("a timing in the baseline")
	}
	if len(bl.Updates) != 1 || bl.Updates[0].Reps != 2 || bl.Updates[0].Reason != "first" {
		t.Errorf("updates %+v", bl.Updates)
	}

	// No reason, or a failing run: refused.
	if _, err := FromRun([]*report.Report{r}, nil, update(" "), true); err == nil {
		t.Error("an update without a reason")
	}
	bad := rep("etag-stability", map[string]report.Check{"integrity": {ID: "integrity", Status: report.Fail}}, nil)
	if _, err := FromRun([]*report.Report{bad}, nil, update("x"), true); !errors.Is(err, ErrFailing) {
		t.Errorf("a failing run: %v", err)
	}

	// A partial update keeps the other scenarios and the history.
	next, err := FromRun([]*report.Report{rep("cold-start", nil, nil)}, bl, update("second"), true)
	if err != nil {
		t.Fatal(err)
	}
	if len(next.Scenarios) != 2 || len(next.Updates) != 2 {
		t.Errorf("scenarios %d, updates %d", len(next.Scenarios), len(next.Updates))
	}
	if d := bl.Diff(next); !slices.Equal(d, []string{"cold-start: added"}) {
		t.Errorf("diff %v", d)
	}
}

// An edit by hand shows: the digest no longer matches.
func TestDigest(t *testing.T) {
	bl, err := FromRun([]*report.Report{rep("cold-start", nil, map[string][]float64{"panics": {0}})}, nil, update("first"), true)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "baseline.json")
	if err := bl.Write(path); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(path); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	os.WriteFile(path, []byte(strings.Replace(string(b), `"panics": 0`, `"panics": 1`, 1)), 0o644)
	if _, err := Read(path); !errors.Is(err, ErrEdited) {
		t.Errorf("an edited baseline reads: %v", err)
	}
}

func TestHold(t *testing.T) {
	base := rep("etag-stability", map[string]report.Check{
		"etag-stable": {ID: "etag-stable", Status: report.XFail, Known: "F2"},
	}, map[string][]float64{"etag_changes": {3}, "leaked_goroutines": {0}})
	bl, err := FromRun([]*report.Report{base}, nil, update("first"), true)
	if err != nil {
		t.Fatal(err)
	}
	hold := func(r *report.Report) *Gate { return Hold(bl, "baseline.json", []*report.Report{r}, true) }

	if g := hold(base); g.Verdict != report.VerdictPass {
		t.Errorf("the same run: %s: %s", g.Verdict, g.Why)
	}
	// An exact metric moved: fail, naming it.
	moved := rep("etag-stability", map[string]report.Check{"etag-stable": {ID: "etag-stable", Status: report.XFail, Known: "F2"}},
		map[string][]float64{"etag_changes": {4}, "leaked_goroutines": {0}})
	if g := hold(moved); g.Verdict != report.VerdictFail || !strings.Contains(g.Why, "etag_changes: 4, expected 3") {
		t.Errorf("moved: %s: %s", g.Verdict, g.Why)
	}
	// xfail → xpass: the known issue appears fixed, and the baseline must say so.
	fixed := rep("etag-stability", map[string]report.Check{"etag-stable": {ID: "etag-stable", Status: report.XPass, Known: "F2"}},
		map[string][]float64{"etag_changes": {3}, "leaked_goroutines": {0}})
	if g := hold(fixed); g.Verdict != report.VerdictFail || !strings.Contains(g.Why, "F2 appears fixed") {
		t.Errorf("xpass: %s: %s", g.Verdict, g.Why)
	}
	// #195 firing within its rate, a leak #225 explains: noted, not failed.
	noisy := rep("etag-stability", map[string]report.Check{
		"etag-stable": {ID: "etag-stable", Status: report.XFail, Known: "F2"},
		"play-ok":     {ID: "play-ok", Status: report.XFail, Known: "#195"},
		"quiescence":  {ID: "quiescence", Status: report.XFail, Known: "#225"},
	}, map[string][]float64{"etag_changes": {3}, "leaked_goroutines": {2}})
	if g := hold(noisy); g.Verdict != report.VerdictPass || len(g.Scenarios[0].Notes) < 2 {
		t.Errorf("noisy: %s: %s %v", g.Verdict, g.Why, g.Scenarios[0].Notes)
	}
	// A failing run fails the gate.
	failing := rep("etag-stability", map[string]report.Check{"integrity": {ID: "integrity", Status: report.Fail}}, nil)
	if g := hold(failing); g.Verdict != report.VerdictFail || !strings.Contains(g.Why, "integrity fail") {
		t.Errorf("failing: %s: %s", g.Verdict, g.Why)
	}
	// F1 depends on the host's MIME table: only noted on another host.
	f1 := rep("probe-burst", map[string]report.Check{"no-sniff-reads": {ID: "no-sniff-reads", Status: report.Pass}}, nil)
	bl, _ = FromRun([]*report.Report{f1}, nil, update("first"), true)
	other := rep("probe-burst", map[string]report.Check{"no-sniff-reads": {ID: "no-sniff-reads", Status: report.XFail, Known: "F1"}}, nil)
	if g := Hold(bl, "b", []*report.Report{other}, false); g.Verdict != report.VerdictPass {
		t.Errorf("another host: %s: %s", g.Verdict, g.Why)
	}
}

// The committed baseline reads, covers every gate scenario, and holds
// no timing.
func TestCommittedBaseline(t *testing.T) {
	bl, err := Read(filepath.Join("..", "testdata", "baseline.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, sc := range scenario.InTier(scenario.Gate) {
		if _, ok := bl.Scenarios[sc.Name]; !ok {
			t.Errorf("%s isn't in the baseline: streamlab baseline update --reason", sc.Name)
		}
	}
	for name, sc := range bl.Scenarios {
		for m := range sc.Exact {
			if def, ok := scenario.LookupMetric(m); !ok || def.Kind != report.Exact {
				t.Errorf("%s: %s in the baseline isn't an exact metric", name, m)
			}
		}
	}
	for _, u := range bl.Updates {
		if strings.TrimSpace(u.Reason) == "" || u.Commit == "" {
			t.Errorf("an update without its reason or commit: %+v", u)
		}
	}
}
