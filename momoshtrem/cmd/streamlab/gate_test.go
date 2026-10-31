package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shapedtime/momoshtrem/internal/streamlab/ab"
	"github.com/shapedtime/momoshtrem/internal/streamlab/baseline"
	"github.com/shapedtime/momoshtrem/internal/streamlab/compare"
	"github.com/shapedtime/momoshtrem/internal/streamlab/report"
	"github.com/shapedtime/momoshtrem/internal/streamlab/scenario"
)

// writeGateRun writes a run folder of gate scenarios that passed, each
// with integrity_errors 0 over one rep.
func writeGateRun(t *testing.T, names ...string) string {
	t.Helper()
	dir := t.TempDir()
	ix := report.Index{Schema: report.IndexSchema, Verdict: report.VerdictPass}
	for _, name := range names {
		r := &report.Report{Schema: report.Schema, Env: report.Env{OS: "darwin"}, Verdict: report.VerdictPass,
			Scenario: report.Scenario{Name: name, Tier: string(scenario.Gate)},
			Metrics: map[string]report.Metric{
				"integrity_errors": report.NewMetric(report.MetricDef{Name: "integrity_errors", Kind: report.Exact}, []float64{0}),
			},
			Checks: []report.Check{{ID: "integrity", Status: report.Pass, Reps: []string{"pass"}}},
			Reps:   []report.Rep{{Rep: 1, Status: report.RepOK}},
		}
		if err := os.MkdirAll(filepath.Join(dir, name), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := r.Write(filepath.Join(dir, name)); err != nil {
			t.Fatal(err)
		}
		ix.Scenarios = append(ix.Scenarios, report.IndexEntry{Name: name, Verdict: r.Verdict, Report: name + "/report.json"})
	}
	if err := ix.Write(dir); err != nil {
		t.Fatal(err)
	}
	return dir
}

// -h is help, not an error: exit 0 with the usage.
func TestHelp(t *testing.T) {
	for _, args := range [][]string{
		{"compare", "-h"}, {"ab", "-h"}, {"calibrate", "-h"}, {"gate", "-h"}, {"baseline", "update", "-h"}, {"affected", "-h"},
	} {
		if code, _, errOut := cli(args...); code != exitPass || !strings.Contains(errOut, "-") {
			t.Errorf("streamlab %q = %d: %s", args, code, errOut)
		}
	}
}

// A fail or a regression wins over a scenario a side couldn't run: exit 1,
// not 3 (retry the tool); an error with nothing worse is 3.
func TestCompareExitErrors(t *testing.T) {
	for _, tc := range []struct {
		v    compare.Verdict
		want int
	}{
		{compare.Fail, exitFail}, {compare.Regression, exitFail}, {compare.Inconclusive, exitTool}, {compare.Pass, exitTool},
	} {
		if got := compareExit(&compare.Result{Verdict: tc.v, Errors: []string{"steady-play: head couldn't run it"}}); got != tc.want {
			t.Errorf("%s with an error: %d, want %d", tc.v, got, tc.want)
		}
	}
	if got := compareExit(&compare.Result{Verdict: compare.Inconclusive}); got != exitInconclusive {
		t.Errorf("inconclusive: %d", got)
	}
}

// calibrate exits 3 when the tool couldn't run a scenario, whatever else
// passed or failed: its noise is missing.
func TestCalibrateExit(t *testing.T) {
	for _, tc := range []struct {
		res  ab.CalibrateResult
		want int
	}{
		{ab.CalibrateResult{}, exitPass},
		{ab.CalibrateResult{Skipped: []string{"a"}}, exitFail},
		{ab.CalibrateResult{Errored: []string{"a"}}, exitTool},
		{ab.CalibrateResult{Skipped: []string{"a"}, Errored: []string{"b"}}, exitTool},
	} {
		if got := calibrateExit(&tc.res); got != tc.want {
			t.Errorf("%+v: %d, want %d", tc.res, got, tc.want)
		}
	}
}

// ab refuses an --out an earlier run wrote to, before it checks anything
// out: its confirm/ would mix into this one's.
func TestABOutNotEmpty(t *testing.T) {
	out := t.TempDir()
	if err := os.WriteFile(filepath.Join(out, "ab.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, _, errOut := cli("ab", "--base", "HEAD", "--scenario", "cold-start", "--out", out)
	if code != exitTool || !strings.Contains(errOut, "isn't empty") {
		t.Errorf("%d: %s", code, errOut)
	}
}

// A baseline edited by hand isn't laundered by updating other scenarios:
// the update is refused until it re-records every scenario the file
// holds, and then the history says so.
func TestBaselineEdited(t *testing.T) {
	gate := scenario.InTier(scenario.Gate)
	a, b := gate[0].Name, gate[1].Name
	path := filepath.Join(t.TempDir(), "baseline.json")
	if code, out, errOut := cli("baseline", "update", "--reason", "first", "--from", writeGateRun(t, a, b), "--baseline", path); code != exitPass {
		t.Fatalf("first: %d %s %s", code, out, errOut)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	edited := strings.Replace(string(raw), `"integrity_errors": 0`, `"integrity_errors": 32`, 1)
	if edited == string(raw) {
		t.Fatal("nothing to edit")
	}
	if err := os.WriteFile(path, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	code, _, errOut := cli("baseline", "update", "--reason", "unrelated", "--from", writeGateRun(t, b), "--baseline", path)
	if code != exitTool || !strings.Contains(errOut, "was edited by hand") || !strings.Contains(errOut, "doesn't re-record "+a) {
		t.Errorf("a partial update of an edited baseline: %d %s", code, errOut)
	}
	if after, _ := os.ReadFile(path); string(after) != edited {
		t.Error("the refused update wrote the file")
	}
	// Every scenario re-recorded: the edit is gone, the digest fresh.
	code, out, errOut := cli("baseline", "update", "--reason", "re-record", "--from", writeGateRun(t, a, b), "--baseline", path)
	if code != exitPass || !strings.Contains(out, "edited by hand") || !strings.Contains(out, "32 → 0") {
		t.Errorf("a full update: %d %s %s", code, out, errOut)
	}
	bl, err := baseline.Read(path)
	if err != nil || bl.Scenarios[a].Exact["integrity_errors"] != 0 || len(bl.Updates) != 2 {
		t.Errorf("after: %v %+v", err, bl)
	}
}
