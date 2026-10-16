package report_test

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/shapedtime/momoshtrem/internal/streamlab/report"
)

func TestQuantile(t *testing.T) {
	tests := []struct {
		values []float64
		q      float64
		want   float64
	}{
		{[]float64{5}, 0.5, 5},
		{[]float64{5}, 0.95, 5},
		{[]float64{3, 1, 2}, 0.5, 2},
		{[]float64{1, 2, 3, 4}, 0.5, 2.5},
		{[]float64{0, 10}, 0.95, 9.5},
	}
	for _, tt := range tests {
		if got := report.Quantile(tt.values, tt.q); math.Abs(got-tt.want) > 1e-9 {
			t.Errorf("Quantile(%v, %v) = %v, want %v", tt.values, tt.q, got, tt.want)
		}
	}
	if !math.IsNaN(report.Quantile(nil, 0.5)) {
		t.Error("the quantile of nothing isn't NaN")
	}
	vals := []float64{3, 1, 2}
	report.Quantile(vals, 0.5)
	if vals[0] != 3 {
		t.Error("Quantile sorted its input")
	}
}

func sample() *report.Report {
	lag := 1234.5
	def := report.MetricDef{Name: "handler_lag_after_abort", Kind: report.Timing, Unit: "ms", Better: report.Lower}
	return &report.Report{
		Schema:   report.Schema,
		Tool:     report.Tool{Name: "streamlab", Version: "dev", Git: report.Git{Commit: "0123456789abcdef", Dirty: true}},
		Env:      report.Env{Go: "go1.26", OS: "darwin", Arch: "arm64", NumCPU: 10, GOMAXPROCS: 4, MIME: map[string]string{".mkv": "", ".mp4": "video/mp4"}},
		Config:   report.Config{Fingerprint: "abcd"},
		Scenario: report.Scenario{Name: "scrub", Tier: "gate", Doc: "Scrubs.\n\nMore.", Guards: []string{"F12"}, Params: map[string]string{"seeks": "10"}, Seed: 1},
		Run:      report.Run{ID: "20261017-000000", Reps: 2, Warmup: 1, Started: time.Unix(0, 0).UTC()},
		Verdict:  report.VerdictPass,
		Metrics:  map[string]report.Metric{def.Name: report.NewMetric(def, []float64{1000, 1234.5})},
		Checks: []report.Check{
			{ID: "abort-lag", Status: report.XFail, Known: "F12", Evidence: "F12 on 2 of 2 reps | a pipe", Reps: []string{"fail:F12", "fail:F12"}},
		},
		Reps: []report.Rep{{
			Rep: 1, Status: report.RepOK, Metrics: map[string]float64{def.Name: 1234.5},
			Checks: []report.RepCheck{{ID: "abort-lag", Status: report.Fail, Known: "F12", Evidence: "slow"}},
			Sessions: []report.Session{{Device: "infuse", Path: "/Movies/Film (2020)/Film.mkv",
				Player:   &report.PlayerStats{Stalls: 1, Plays: []report.Play{{From: 0, Playable: true}}},
				Requests: []report.Request{{ID: "s1.1", Method: "GET", Range: "bytes=0-", Status: 206, HandlerLagMS: &lag}}}},
		}},
		Findings: []string{},
	}
}

// The report survives JSON, every metric carries its kind, unit, better,
// values, median and p95, and report.md shows the checks and metrics.
func TestReportEncoding(t *testing.T) {
	dir := t.TempDir()
	r := sample()
	if err := r.Write(dir); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatal(err)
	}
	if raw["schema"] != "streamlab.report/v1" {
		t.Errorf("schema = %v", raw["schema"])
	}
	m := raw["metrics"].(map[string]any)["handler_lag_after_abort"].(map[string]any)
	for _, k := range []string{"kind", "unit", "better", "values", "median", "p95"} {
		if _, ok := m[k]; !ok {
			t.Errorf("metric lacks %q: %v", k, m)
		}
	}
	if m["kind"] != "timing" || m["median"].(float64) != 1117.25 {
		t.Errorf("metric = %v", m)
	}
	var back report.Report
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if lag := back.Reps[0].Sessions[0].Requests[0].HandlerLagMS; lag == nil || *lag != 1234.5 {
		t.Errorf("handler lag lost: %v", lag)
	}

	md, err := os.ReadFile(filepath.Join(dir, "report.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"# streamlab: scrub: PASS", "| abort-lag | xfail | F12 |", "reps \\| a pipe", "handler_lag_after_abort",
		".mkv unresolved", "### infuse: /Movies/Film (2020)/Film.mkv", "`seeks` = `10`"} {
		if !strings.Contains(string(md), want) {
			t.Errorf("report.md lacks %q:\n%s", want, md)
		}
	}
}

func TestRepRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rep.json")
	in := sample().Reps[0]
	if err := report.WriteJSON(path, in); err != nil {
		t.Fatal(err)
	}
	out, err := report.ReadRep(path)
	if err != nil {
		t.Fatal(err)
	}
	if out.Metrics["handler_lag_after_abort"] != 1234.5 || out.Checks[0].Known != "F12" || out.Sessions[0].Player.Stalls != 1 {
		t.Errorf("rep = %+v", out)
	}
}

func TestIndex(t *testing.T) {
	r := sample()
	r.Checks = append(r.Checks, report.Check{ID: "integrity", Status: report.Pass})
	e := report.NewIndexEntry(r, 42, []string{"handler_lag_after_abort", "missing"}, "scrub/report.json")
	if len(e.Checks) != 1 || e.Checks[0] != "abort-lag=xfail:F12" || len(e.KeyOrder) != 1 {
		t.Errorf("entry = %+v", e)
	}
	ix := &report.Index{Schema: report.IndexSchema, Verdict: report.VerdictPass, Scenarios: []report.IndexEntry{e}}
	md := ix.Markdown()
	if !strings.Contains(md, "[scrub](scrub/report.md)") || !strings.Contains(md, "abort-lag=xfail:F12") {
		t.Errorf("index.md:\n%s", md)
	}
}

func TestNum(t *testing.T) {
	for v, want := range map[float64]string{3: "3", 1234.5: "1234", 2.346: "2.35", 0.01234: "0.0123", math.NaN(): "-"} {
		if got := report.Num(v); got != want {
			t.Errorf("Num(%v) = %q, want %q", v, got, want)
		}
	}
}
