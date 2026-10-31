package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shapedtime/momoshtrem/internal/streamlab/compare"
	"github.com/shapedtime/momoshtrem/internal/streamlab/report"
)

// writeRun writes a one-scenario run folder: cold-start over 5 reps on
// one machine, with ttfb's values and the integrity check's status.
func writeRun(t *testing.T, ttfb []float64, integrity report.Status, host string) string {
	t.Helper()
	return writeRunAt(t, t.TempDir(), ttfb, integrity, host)
}

func writeRunAt(t *testing.T, dir string, ttfb []float64, integrity report.Status, host string) string {
	t.Helper()
	env := report.Env{Host: host, OS: "darwin", Arch: "arm64", NumCPU: 8, Load: []float64{2, 2, 2}}
	r := &report.Report{Schema: report.Schema, Env: env, Verdict: report.VerdictPass,
		Scenario: report.Scenario{Name: "cold-start", Params: map[string]string{"seed": "1"}},
		Metrics: map[string]report.Metric{
			"ttfb": report.NewMetric(report.MetricDef{Name: "ttfb", Kind: report.Timing, Unit: "ms", Better: report.Lower}, ttfb),
		},
		Checks: []report.Check{{ID: "integrity", Status: integrity, Reps: []string{string(integrity)}}},
	}
	if integrity == report.Fail {
		r.Verdict = report.VerdictFail
	}
	for i := range ttfb {
		e := env
		r.Reps = append(r.Reps, report.Rep{Rep: i + 1, Status: report.RepOK, Env: &e})
	}
	if err := os.MkdirAll(filepath.Join(dir, "cold-start"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := r.Write(filepath.Join(dir, "cold-start")); err != nil {
		t.Fatal(err)
	}
	ix := report.Index{Schema: report.IndexSchema, Verdict: r.Verdict,
		Scenarios: []report.IndexEntry{{Name: "cold-start", Verdict: r.Verdict, Report: "cold-start/report.json"}}}
	if err := ix.Write(dir); err != nil {
		t.Fatal(err)
	}
	return dir
}

// compare's exit codes: 0 pass, 1 regression or fail, 2 inconclusive,
// 3 a tool error (here, two machines without --force).
func TestCompareExitCodes(t *testing.T) {
	fast := []float64{100, 110, 105, 98, 102}
	slow := []float64{900, 950, 920, 905, 930}
	base := writeRun(t, fast, report.Pass, "mac")
	for name, tc := range map[string]struct {
		head []string
		code int
		line string
	}{
		"pass": {[]string{writeRun(t, fast, report.Pass, "mac")}, exitPass, "**PASS**"},
		// A single compare is a screen: no fresh reps confirmed it.
		"screen":       {[]string{writeRun(t, slow, report.Pass, "mac")}, exitInconclusive, "**INCONCLUSIVE**: cold-start: ttfb 102 → 920 ms"},
		"fail":         {[]string{writeRun(t, fast, report.Fail, "mac")}, exitFail, "**FAIL**"},
		"inconclusive": {[]string{writeRun(t, slow[:3], report.Pass, "mac")}, exitInconclusive, "**INCONCLUSIVE**"},
		"machines":     {[]string{writeRun(t, fast, report.Pass, "linux")}, exitTool, ""},
		"forced":       {[]string{"--force", writeRun(t, slow, report.Pass, "linux")}, exitPass, "**PASS**"},
	} {
		args := append([]string{"compare", base}, tc.head...)
		code, out, errOut := cli(args...)
		if code != tc.code || !strings.HasPrefix(out, tc.line) {
			t.Errorf("%s: exit %d, want %d; out %q %s", name, code, tc.code, firstLine(out), errOut)
		}
	}
	// --json and --out.
	outDir := t.TempDir()
	code, out, _ := cli("compare", "--json", "--out", outDir, base, writeRun(t, slow, report.Pass, "mac"))
	var res compare.Result
	if err := json.Unmarshal([]byte(out), &res); err != nil || code != exitInconclusive || res.Schema != compare.Schema ||
		res.Verdict != compare.Inconclusive || !strings.Contains(res.Why, "a single compare is a screen") {
		t.Errorf("--json: %d %v %+v", code, err, res)
	}
	for _, f := range []string{"compare.json", "compare.md"} {
		if _, err := os.Stat(filepath.Join(outDir, f)); err != nil {
			t.Error(err)
		}
	}
}

// compare <ab folder> judges an ab again, its confirmation included:
// without one it's a screen (inconclusive); a regression the fresh reps
// show too holds, one they don't is inconclusive.
func TestCompareABFolder(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ab.json"), []byte(`{"schema":"streamlab.ab/v1"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	fast := []float64{100, 110, 105, 98, 102}
	slow := []float64{900, 950, 920, 905, 930}
	writeRunAt(t, filepath.Join(dir, "base"), fast, report.Pass, "mac")
	writeRunAt(t, filepath.Join(dir, "head"), slow, report.Pass, "mac")
	if code, out, errOut := cli("compare", dir); code != exitInconclusive || !strings.Contains(firstLine(out), "a single compare is a screen") {
		t.Fatalf("no confirmation: %d %q %s", code, firstLine(out), errOut)
	}
	writeRunAt(t, filepath.Join(dir, "confirm", "base"), fast, report.Pass, "mac")
	writeRunAt(t, filepath.Join(dir, "confirm", "head"), slow, report.Pass, "mac")
	if code, out, errOut := cli("compare", dir); code != exitFail || !strings.HasPrefix(out, "**REGRESSION**") {
		t.Errorf("confirmed: %d %q %s", code, firstLine(out), errOut)
	}
	os.RemoveAll(filepath.Join(dir, "confirm"))
	writeRunAt(t, filepath.Join(dir, "confirm", "base"), fast, report.Pass, "mac")
	writeRunAt(t, filepath.Join(dir, "confirm", "head"), fast, report.Pass, "mac")
	if code, out, errOut := cli("compare", dir); code != exitInconclusive || !strings.Contains(firstLine(out), "didn't hold up") {
		t.Errorf("contradicted: %d %q %s", code, firstLine(out), errOut)
	}
	if code, _, _ := cli("compare", t.TempDir()); code != exitTool {
		t.Errorf("a folder without ab.json: %d", code)
	}
}

func firstLine(s string) string {
	l, _, _ := strings.Cut(s, "\n")
	return l
}

func TestS8ToolErrors(t *testing.T) {
	for _, args := range [][]string{
		{"compare", "only-one"},
		{"compare", "/nonexistent/a", "/nonexistent/b"},
		{"ab", "--reps", "5"},
		{"ab", "--base", "main", "--scenario", "nope"},
		{"ab", "--base", "main", "--reps", "0"},
		{"calibrate", "--reps", "1"},
		{"calibrate", "--scenario", "nope"},
		{"gate", "--scenario", "nope"},
		{"gate", "extra"},
		{"baseline"},
		{"baseline", "update"},
		{"baseline", "update", "--reason", " "},
	} {
		if code, _, _ := cli(args...); code != exitTool {
			t.Errorf("streamlab %q = %d, want %d", args, code, exitTool)
		}
	}
}
