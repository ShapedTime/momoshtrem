package baseline

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/shapedtime/momoshtrem/internal/streamlab/report"
)

// The digest covers the history too: an update's reason edited by hand
// shows. Missing names the scenarios a run doesn't re-record.
func TestDigestHistory(t *testing.T) {
	bl, err := FromRun([]*report.Report{rep("cold-start", nil, nil), rep("steady-play", nil, nil)}, nil, update("first"), true)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "baseline.json")
	if err := bl.Write(path); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	edited := strings.Replace(string(b), `"reason": "first"`, `"reason": "approved by review"`, 1)
	if edited == string(b) {
		t.Fatal("nothing to edit")
	}
	os.WriteFile(path, []byte(edited), 0o644)
	prev, err := Read(path)
	if !errors.Is(err, ErrEdited) || prev == nil {
		t.Fatalf("an edited reason reads: %v", err)
	}
	if got := prev.Missing([]*report.Report{rep("steady-play", nil, nil)}); !slices.Equal(got, []string{"cold-start"}) {
		t.Errorf("Missing = %v", got)
	}
	if got := prev.Missing([]*report.Report{rep("steady-play", nil, nil), rep("cold-start", nil, nil)}); len(got) != 0 {
		t.Errorf("Missing, every scenario = %v", got)
	}
}

// A run where a known issue appears fixed (xpass) isn't a baseline: the
// registry changes first. An exact metric some rep didn't measure isn't
// an expectation.
func TestFromRunRefuses(t *testing.T) {
	fixed := rep("etag-stability", map[string]report.Check{"etag-stable": {ID: "etag-stable", Status: report.XPass, Known: "F2"}}, nil)
	if _, err := FromRun([]*report.Report{fixed}, nil, update("x"), true); !errors.Is(err, ErrXPass) || !strings.Contains(err.Error(), "etag-stable (F2)") {
		t.Errorf("xpass: %v", err)
	}
	r := rep("cold-start", nil, map[string][]float64{"panics": {0, 0}, "sniff_readers": {0}})
	bl, err := FromRun([]*report.Report{r}, nil, update("x"), true)
	if err != nil {
		t.Fatal(err)
	}
	if ex := bl.Scenarios["cold-start"].Exact; len(ex) != 1 || ex["panics"] != 0 {
		t.Errorf("exact %v: sniff_readers was measured on 1 of 2 reps", ex)
	}
}

// What the gate makes of each way a run can leave its expectations.
func TestHoldExpectations(t *testing.T) {
	asExpected := func() map[string]report.Check {
		return map[string]report.Check{
			"close-keeps-windows": {ID: "close-keeps-windows", Status: report.XFail, Known: "F6"},
			"etag-stable":         {ID: "etag-stable", Status: report.Pass},
		}
	}
	bl, err := FromRun([]*report.Report{rep("dual-reader-close", asExpected(), map[string][]float64{"integrity_errors": {0}})}, nil, update("first"), true)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		checks map[string]report.Check
		exact  map[string][]float64
		drop   string // a check that didn't run
		want   report.Verdict
		says   string
	}{
		{"as expected", nil, nil, "", report.VerdictPass, ""},
		{"xfail → n/a", map[string]report.Check{"close-keeps-windows": {ID: "close-keeps-windows", Status: report.NA}}, nil, "",
			report.VerdictPass, "close-keeps-windows couldn't tell this time (n/a), xfail:F6 in the baseline"},
		{"xfail → pass", map[string]report.Check{"close-keeps-windows": {ID: "close-keeps-windows", Status: report.Pass}}, nil, "",
			report.VerdictFail, "close-keeps-windows: pass, expected xfail:F6"},
		{"pass → xfail", map[string]report.Check{"etag-stable": {ID: "etag-stable", Status: report.XFail, Known: "F2"}}, nil, "",
			report.VerdictFail, "etag-stable: xfail:F2, expected pass"},
		{"a check gone", nil, nil, "integrity", report.VerdictFail, "integrity (pass in the baseline) didn't run"},
		{"an exact metric gone", nil, map[string][]float64{}, "", report.VerdictFail, "exact metric integrity_errors (0 in the baseline) wasn't measured"},
	} {
		checks := asExpected()
		for id, c := range tc.checks {
			checks[id] = c
		}
		exact := map[string][]float64{"integrity_errors": {0}}
		if tc.exact != nil {
			exact = tc.exact
		}
		r := rep("dual-reader-close", checks, exact)
		if tc.drop != "" {
			r.Checks = slices.DeleteFunc(r.Checks, func(c report.Check) bool { return c.ID == tc.drop })
		}
		g := Hold(bl, "b", []*report.Report{r}, true)
		all := strings.Join(append(g.Scenarios[0].Problems, g.Scenarios[0].Notes...), "; ")
		if g.Verdict != tc.want || !strings.Contains(all, tc.says) {
			t.Errorf("%s: %s: %s", tc.name, g.Verdict, all)
		}
	}
}

// A fail wins over a tool error whatever the order; F1's expectation
// holds on the host the baseline was recorded on.
func TestHoldVerdict(t *testing.T) {
	bl, err := FromRun([]*report.Report{rep("cold-start", nil, nil), rep("steady-play", nil, nil)}, nil, update("first"), true)
	if err != nil {
		t.Fatal(err)
	}
	broken := rep("cold-start", nil, nil)
	broken.Verdict = report.VerdictError
	failing := rep("steady-play", map[string]report.Check{"integrity": {ID: "integrity", Status: report.Fail}}, nil)
	for _, reports := range [][]*report.Report{{broken, failing}, {failing, broken}} {
		if g := Hold(bl, "b", reports, true); g.Verdict != report.VerdictFail {
			t.Errorf("%s then %s: %s", reports[0].Scenario.Name, reports[1].Scenario.Name, g.Verdict)
		}
	}
	if g := Hold(bl, "b", []*report.Report{broken, rep("steady-play", nil, nil)}, true); g.Verdict != report.VerdictError {
		t.Errorf("an error alone: %s", g.Verdict)
	}
	f1 := rep("probe-burst", map[string]report.Check{"no-sniff-reads": {ID: "no-sniff-reads", Status: report.Pass}}, nil)
	bl, _ = FromRun([]*report.Report{f1}, nil, update("first"), true)
	other := rep("probe-burst", map[string]report.Check{"no-sniff-reads": {ID: "no-sniff-reads", Status: report.XFail, Known: "F1"}}, nil)
	if g := Hold(bl, "b", []*report.Report{other}, true); g.Verdict != report.VerdictFail {
		t.Errorf("F1 on the same host: %s: %s", g.Verdict, g.Why)
	}
}
