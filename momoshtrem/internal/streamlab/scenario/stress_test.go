package scenario_test

import (
	"context"
	"os"
	"slices"
	"testing"

	"github.com/shapedtime/momoshtrem/internal/streamlab/checks"
	"github.com/shapedtime/momoshtrem/internal/streamlab/known"
	"github.com/shapedtime/momoshtrem/internal/streamlab/report"
	"github.com/shapedtime/momoshtrem/internal/streamlab/scenario"
)

func TestStressCatalog(t *testing.T) {
	var got []string
	for _, sc := range scenario.InTier(scenario.Stress) {
		got = append(got, sc.Name)
		if len(sc.Guards) == 0 {
			t.Errorf("%s guards nothing", sc.Name)
		}
		p, err := sc.Resolve(nil)
		if err != nil {
			t.Fatal(err)
		}
		if p.Duration("read_timeout") > 10e9 || p.Bytes("window") <= 0 {
			t.Errorf("%s: read_timeout %v, window %d", sc.Name, p.Duration("read_timeout"), p.Bytes("window"))
		}
	}
	want := []string{"close-blocked-read", "seek-pending-read", "abort-blocked-read", "remove-pause-idle", "remove-while-hashing",
		"concurrent-adds", "many-readers"}
	if !slices.Equal(got, want) {
		t.Errorf("stress = %v, want %v", got, want)
	}
	if _, ok := scenario.LookupCheck(known.RaceCheck); !ok {
		t.Error("the race check isn't declared")
	}
	if m, ok := scenario.LookupMetric("race_reports"); !ok || m.Kind != report.Exact {
		t.Errorf("race_reports = %+v, %v; want an exact metric", m, ok)
	}
}

// TestStress runs the stress scenarios once each, in this process, with
// STREAMLAB=1 (STREAMLAB_SCENARIO=name for one). Under go test -race the
// race detector fails the test on a race; streamlab check runs them in a
// race build, a process per repetition, and reports the races as JSON.
func TestStress(t *testing.T) {
	if os.Getenv("STREAMLAB") == "" {
		t.Skip("set STREAMLAB=1 to run the stress scenarios")
	}
	only := os.Getenv("STREAMLAB_SCENARIO")
	for _, sc := range scenario.InTier(scenario.Stress) {
		if only != "" && sc.Name != only {
			continue
		}
		env := known.Env{MIMEResolvesMKV: checks.MIME().ResolvesMKV(), Guards: sc.Guards}
		t.Run(sc.Name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), sc.HardTimeout())
			defer cancel()
			rep, err := scenario.Run(ctx, sc, scenario.RunOptions{Rep: 1})
			if err != nil {
				t.Fatalf("run: %v", err)
			}
			for _, n := range rep.Notes {
				t.Logf("note: %s", n)
			}
			for _, id := range sc.AllChecks() {
				o, ok := outcomes(rep)[id]
				if !ok {
					o = known.Outcome{Status: report.NA, Evidence: "not reported"}
				}
				v := known.Judge(id, []known.Outcome{o}, env)
				t.Logf("%-20s %-5s %-9s %.300s", id, v.Status, v.Known, o.Evidence)
				if v.Status == report.Fail {
					t.Errorf("%s failed: %s", id, v.Note)
				}
			}
		})
	}
}
