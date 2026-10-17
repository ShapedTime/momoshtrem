package scenario_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/shapedtime/momoshtrem/internal/streamlab/checks"
	"github.com/shapedtime/momoshtrem/internal/streamlab/known"
	"github.com/shapedtime/momoshtrem/internal/streamlab/report"
	"github.com/shapedtime/momoshtrem/internal/streamlab/scenario"
)

func TestCatalog(t *testing.T) {
	if err := scenario.Validate(); err != nil {
		t.Fatal(err)
	}
	gate := scenario.InTier(scenario.Gate)
	want := []string{"cold-start", "steady-play", "seek-fwd", "seek-back", "scrub", "probe-burst", "parallel-ranges",
		"dual-reader-close", "dual-reader-open", "pause-resume", "reconnect", "next-episode", "small-cache", "etag-stability"}
	var got []string
	for _, sc := range gate {
		got = append(got, sc.Name)
	}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("gate = %v, want %v", got, want)
	}
	// Every known issue explains checks that exist.
	for _, is := range known.Issues {
		for _, c := range is.Checks {
			if _, ok := scenario.LookupCheck(c); !ok {
				t.Errorf("known issue %s explains undeclared check %q", is.ID, c)
			}
		}
	}
}

func TestParams(t *testing.T) {
	sc, ok := scenario.Lookup("scrub")
	if !ok {
		t.Fatal("no scrub")
	}
	p, err := sc.Resolve(map[string]string{"seeks": "4", "bitrate": "2MiB", "read_timeout": "7s"})
	if err != nil {
		t.Fatal(err)
	}
	if p.Int("seeks") != 4 || p.Bytes("bitrate") != 2<<20 || p.Duration("read_timeout") != 7*time.Second {
		t.Errorf("params = %v", p.Values())
	}
	if p.Bytes("seeder_rate") != 3<<20 {
		t.Errorf("seeder_rate = %d, want the common default", p.Bytes("seeder_rate"))
	}
	if _, err := sc.Resolve(map[string]string{"nope": "1"}); err == nil || !strings.Contains(err.Error(), "unknown param") {
		t.Errorf("unknown param: %v", err)
	}
	if _, err := sc.Resolve(map[string]string{"seeks": "many"}); err == nil {
		t.Error("a bad int was accepted")
	}
	open, _ := scenario.Lookup("dual-reader-open")
	if p, _ := open.Resolve(nil); p.Bytes("seeder_rate") != 1536<<10 {
		t.Errorf("dual-reader-open's seeder_rate = %d, want its own default", p.Bytes("seeder_rate"))
	}
	for in, want := range map[string]int64{"4096": 4096, "512KiB": 512 << 10, "1.5MiB": 3 << 19, "1GiB": 1 << 30, "300B": 300} {
		if got, err := scenario.ParseBytes(in); err != nil || got != want {
			t.Errorf("ParseBytes(%q) = %d, %v; want %d", in, got, err, want)
		}
	}
	if _, err := scenario.ParseBytes("3MB"); err == nil {
		t.Error("ParseBytes accepted 3MB")
	}
}

// TestGate runs the gate scenarios once each, in this process, with
// STREAMLAB=1 (STREAMLAB_SCENARIO=name for one). The runner runs each
// repetition in a process of its own; this is the scenarios' own test,
// judged by the runner's rule: when an intermittent known issue fires, the
// scenario runs known.ConfirmReps more times, and each check is judged
// over all of them (known.Judge). A check that fails fails the test.
func TestGate(t *testing.T) {
	if os.Getenv("STREAMLAB") == "" {
		t.Skip("set STREAMLAB=1 to run the gate scenarios")
	}
	only := os.Getenv("STREAMLAB_SCENARIO")
	for _, sc := range scenario.InTier(scenario.Gate) {
		if only != "" && sc.Name != only {
			continue
		}
		env := known.Env{MIMEResolvesMKV: checks.MIME().ResolvesMKV(), Guards: sc.Guards}
		t.Run(sc.Name, func(t *testing.T) {
			var reps []*report.Rep
			run := func() {
				ctx, cancel := context.WithTimeout(context.Background(), sc.HardTimeout())
				defer cancel()
				rep, err := scenario.Run(ctx, sc, scenario.RunOptions{Rep: len(reps) + 1})
				if err != nil {
					t.Fatalf("run: %v", err)
				}
				for _, n := range rep.Notes {
					t.Logf("rep %d note: %s", rep.Rep, n)
				}
				t.Logf("rep %d: wall %.1f s", rep.Rep, rep.WallMS/1000)
				reps = append(reps, rep)
			}
			run()
			if id, fired := known.Fired(outcomes(reps[0])); fired {
				t.Logf("%s fired: %d confirmation reps", id, known.ConfirmReps)
				for range known.ConfirmReps {
					run()
				}
			}
			for _, id := range sc.AllChecks() {
				var per []known.Outcome
				for _, rep := range reps {
					o, ok := outcomes(rep)[id]
					if !ok {
						o = known.Outcome{Status: report.NA, Evidence: "not reported"}
					}
					per = append(per, o)
				}
				v := known.Judge(id, per, env)
				t.Logf("%-20s %-5s %-9s %s", id, v.Status, v.Known, per[0].Evidence)
				if v.Status == report.Fail {
					t.Errorf("%s failed: %s", id, v.Note)
				}
			}
			for _, k := range sc.Key {
				if v, ok := reps[0].Metrics[k]; ok {
					t.Logf("%s = %s", k, report.Num(v))
				}
			}
		})
	}
}

// outcomes are a repetition's checks by ID.
func outcomes(rep *report.Rep) map[string]known.Outcome {
	m := map[string]known.Outcome{}
	for _, c := range rep.Checks {
		m[c.ID] = known.Outcome{Status: c.Status, Known: c.Known, Evidence: c.Evidence}
	}
	return m
}
