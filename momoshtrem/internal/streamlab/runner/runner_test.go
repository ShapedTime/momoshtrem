package runner_test

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/shapedtime/momoshtrem/internal/streamlab/report"
	"github.com/shapedtime/momoshtrem/internal/streamlab/runner"
	"github.com/shapedtime/momoshtrem/internal/streamlab/scenario"
)

// The runner is tested with this test binary as its child: with
// STREAMLAB_FAKE_CHILD set, TestMain acts out a repetition (a result, a
// hang, a panic or a tool error) instead of running a scenario. No swarm.

const fakeEnv = "STREAMLAB_FAKE_CHILD"

func TestMain(m *testing.M) {
	if mode := os.Getenv(fakeEnv); mode != "" && len(os.Args) > 1 && os.Args[1] == runner.ChildCommand {
		os.Exit(fakeChild(mode, os.Args[2:]))
	}
	os.Exit(m.Run())
}

func fakeChild(mode string, args []string) int {
	fs := flag.NewFlagSet("fake", flag.ContinueOnError)
	out := fs.String("out", "", "")
	rep := fs.Int("rep", 0, "")
	name := fs.String("scenario", "", "")
	fs.String("config", "", "")
	fs.Var(runner.Sets{}, "set", "")
	if err := fs.Parse(args); err != nil {
		return 3
	}
	switch {
	case mode == "hang", mode == "hang-warmup" && *rep <= 0:
		select {} // the runner's SIGQUIT dumps this
	case mode == "crash":
		panic("fake child crashed")
	case mode == "tool":
		report.WriteJSON(filepath.Join(*out, "rep.json"), report.Rep{Rep: *rep, Status: report.RepError, Error: "no seeder"})
		return runner.ChildToolError
	}
	r := report.Rep{
		Rep: *rep, Status: report.RepOK, Config: report.Config{Fingerprint: "fp"},
		Env:     &report.Env{GOMAXPROCS: runtime.GOMAXPROCS(0)},
		Notes:   []string{"tmpdir=" + os.Getenv("TMPDIR"), "pipe=" + os.Getenv(runner.ParentPipeEnv)},
		Metrics: map[string]float64{"etag_changes": float64(*rep)},
		Checks: []report.RepCheck{
			{ID: "script", Status: report.Pass},
			{ID: "integrity", Status: report.Pass},
			{ID: "http", Status: report.Pass},
			{ID: "play-ok", Status: report.Pass},
			{ID: "quiescence", Status: report.Pass},
			{ID: "etag-stable", Status: report.Fail, Known: "F2", Evidence: "three ETags"},
		},
	}
	// stall:<scenario>: that scenario's playback hits the known stall on
	// every rep; stall1:<scenario>, on its first only.
	victim, every := strings.CutPrefix(mode, "stall:")
	if !every {
		victim, _ = strings.CutPrefix(mode, "stall1:")
	}
	if victim == *name && (every || *rep == 1) {
		r.Checks[3] = report.RepCheck{ID: "play-ok", Status: report.Fail, Known: "#195", Evidence: "a known stall"}
	}
	report.WriteJSON(filepath.Join(*out, "rep.json"), r)
	return runner.ChildOK
}

func run(t *testing.T, mode string, opts runner.Options, names ...string) *runner.Result {
	t.Helper()
	res, err := runCtx(t.Context(), t, mode, opts, names...)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func runCtx(ctx context.Context, t *testing.T, mode string, opts runner.Options, names ...string) (*runner.Result, error) {
	t.Helper()
	t.Setenv(fakeEnv, mode)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if len(names) == 0 {
		names = []string{"etag-stability"}
	}
	for _, n := range names {
		sc, ok := scenario.Lookup(n)
		if !ok {
			t.Fatalf("no scenario %s", n)
		}
		opts.Scenarios = append(opts.Scenarios, sc)
	}
	opts.Exe = exe
	opts.Out = t.TempDir()
	if opts.Reps == 0 {
		opts.Reps = 1
	}
	return runner.Run(ctx, opts)
}

// A known issue on every rep of a deterministic suspect is an xfail; the
// warm-up's numbers are left out; the reports and the index are written.
func TestRunReports(t *testing.T) {
	res := run(t, "ok", runner.Options{Reps: 2, Warmup: 1, GOMAXPROCS: 2})
	if res.Failed() || res.Errored() {
		t.Fatalf("verdict %s", res.Index.Verdict)
	}
	r := res.Reports[0]
	if len(r.Reps) != 3 || !r.Reps[0].Warmup || r.Reps[1].Rep != 1 || r.Reps[2].Rep != 2 {
		t.Fatalf("reps = %+v", r.Reps)
	}
	if m := r.Metrics["etag_changes"]; len(m.Values) != 2 || m.Values[0] != 1 || m.Values[1] != 2 || m.Kind != report.Exact {
		t.Errorf("etag_changes = %+v, want the two measured reps", m)
	}
	var etag report.Check
	for _, c := range r.Checks {
		if c.ID == "etag-stable" {
			etag = c
		}
	}
	if etag.Status != report.XFail || etag.Known != "F2" || strings.Join(etag.Reps, " ") != "fail:F2 fail:F2" {
		t.Errorf("etag-stable = %+v", etag)
	}
	if r.Run.GOMAXPROCS != 2 || r.Config.Fingerprint != "fp" || r.Scenario.Params["seed"] != "1" {
		t.Errorf("run %+v, config %+v, params %v", r.Run, r.Config, r.Scenario.Params)
	}
	// The child's view: GOMAXPROCS as it ran, its own temp folder (gone
	// now), and the pipe to its parent.
	if r.Env.GOMAXPROCS != 2 {
		t.Errorf("the child ran with GOMAXPROCS %d, want 2", r.Env.GOMAXPROCS)
	}
	notes := strings.Join(r.Reps[1].Notes, " ")
	tmp, _ := strings.CutPrefix(r.Reps[1].Notes[0], "tmpdir=")
	if !strings.Contains(notes, "pipe=1") || !strings.Contains(filepath.Base(tmp), "streamlab-rep-") {
		t.Errorf("child env: %s", notes)
	}
	if _, err := os.Stat(tmp); !os.IsNotExist(err) {
		t.Errorf("the child's temp folder %s is still there: %v", tmp, err)
	}
	for _, f := range []string{"etag-stability/report.json", "etag-stability/report.md", "etag-stability/rep-1/rep.json",
		"etag-stability/warmup-1/child.log", "index.json", "index.md"} {
		if _, err := os.Stat(filepath.Join(res.Dir, f)); err != nil {
			t.Errorf("%s: %v", f, err)
		}
	}
	b, _ := os.ReadFile(filepath.Join(res.Dir, "index.json"))
	var ix report.Index
	if err := json.Unmarshal(b, &ix); err != nil || len(ix.Scenarios) != 1 || ix.Scenarios[0].Checks[0] != "etag-stable=xfail:F2" {
		t.Errorf("index = %+v, %v", ix, err)
	}
}

// A repetition past the hard timeout gets SIGQUIT: its goroutine dump is
// kept, and the scenario fails.
func TestRunTimeout(t *testing.T) {
	res := run(t, "hang", runner.Options{Timeout: 2 * time.Second})
	if !res.Failed() {
		t.Fatalf("verdict %s, want fail", res.Index.Verdict)
	}
	rep := res.Reports[0].Reps[0]
	if rep.Status != report.RepTimeout || rep.Artifacts["goroutines"] == "" {
		t.Fatalf("rep = %+v", rep)
	}
	dump, err := os.ReadFile(filepath.Join(res.Dir, "etag-stability", rep.Artifacts["goroutines"]))
	if err != nil || !strings.Contains(string(dump), "goroutine ") || !strings.Contains(string(dump), "fakeChild") {
		t.Errorf("dump (%v):\n%.500s", err, dump)
	}
}

func TestRunCrash(t *testing.T) {
	res := run(t, "crash", runner.Options{})
	if rep := res.Reports[0].Reps[0]; !res.Failed() || rep.Status != report.RepCrash || !strings.Contains(rep.Error, "exited 2") {
		t.Errorf("verdict %s, rep %+v", res.Index.Verdict, rep)
	}
}

func TestRunToolError(t *testing.T) {
	res := run(t, "tool", runner.Options{})
	if rep := res.Reports[0].Reps[0]; !res.Errored() || res.Failed() || rep.Status != report.RepError || rep.Error != "no seeder" {
		t.Errorf("verdict %s, rep %+v", res.Index.Verdict, rep)
	}
}

// #195 on a scenario's only rep gets two confirmation reps. A stray stall
// doesn't come back: an xfail. A regression does: a fail.
func TestRunConfirms(t *testing.T) {
	five := []string{"etag-stability", "scrub", "cold-start", "steady-play", "seek-fwd"}
	res := run(t, "stall1:scrub", runner.Options{}, five...)
	if res.Failed() {
		t.Errorf("a stall that didn't come back failed the run")
	}
	scrub := report.Report{}
	for _, r := range res.Reports {
		if r.Scenario.Name == "scrub" {
			scrub = *r
		}
	}
	if len(scrub.Reps) != 3 || scrub.Run.Confirmations != 2 || !scrub.Reps[1].Confirm || !scrub.Reps[2].Confirm {
		t.Fatalf("scrub's reps = %+v, run %+v", scrub.Reps, scrub.Run)
	}
	if c := playOK(res, "scrub"); c.Status != report.XFail || c.Known != "#195" || !strings.Contains(c.Evidence, "1 of the run's 7") {
		t.Errorf("scrub's play-ok = %+v", c)
	}
	for _, r := range res.Reports {
		if r.Scenario.Name != "scrub" && len(r.Reps) != 1 {
			t.Errorf("%s got %d reps, want 1: only a firing scenario is confirmed", r.Scenario.Name, len(r.Reps))
		}
	}

	res = run(t, "stall:scrub", runner.Options{}, five...)
	if c := playOK(res, "scrub"); !res.Failed() || c.Status != report.Fail || c.Known != "#195" {
		t.Errorf("a stall on every confirmation: verdict %s, play-ok %+v", res.Index.Verdict, c)
	}

	// Two measured reps are enough to judge: no confirmation.
	res = run(t, "stall1:scrub", runner.Options{Reps: 2}, "etag-stability", "scrub")
	if c := playOK(res, "scrub"); res.Failed() || c.Status != report.XFail || len(res.Reports[1].Reps) != 2 {
		t.Errorf("one stall in two reps: verdict %s, play-ok %+v", res.Index.Verdict, c)
	}
}

// A warm-up that hangs fails the scenario: warm-ups drop timings, not
// correctness.
func TestRunWarmupHang(t *testing.T) {
	res := run(t, "hang-warmup", runner.Options{Warmup: 1, Timeout: 2 * time.Second})
	r := res.Reports[0]
	if !res.Failed() || r.Reps[0].Status != report.RepTimeout || r.Reps[1].Status != report.RepOK {
		t.Errorf("verdict %s, reps %+v", res.Index.Verdict, r.Reps)
	}
	if res.Index.Scenarios[0].Error == "" {
		t.Error("the index doesn't show the warm-up's hang")
	}
}

// Ctrl-C stops the run: the child is killed, Run returns the context's
// error.
func TestRunInterrupt(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	time.AfterFunc(time.Second, cancel)
	start := time.Now()
	_, err := runCtx(ctx, t, "hang", runner.Options{Timeout: time.Minute}, "etag-stability", "scrub")
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Run = %v, want context.Canceled", err)
	}
	if took := time.Since(start); took > 30*time.Second {
		t.Errorf("the interrupt took %v", took)
	}
}

func TestRunRefusesBadOptions(t *testing.T) {
	sc, _ := scenario.Lookup("etag-stability")
	for _, opts := range []runner.Options{
		{Reps: 1, Warmup: -1},
		{Reps: 1, Timeout: -time.Second},
		{Reps: 0},
		{Reps: 1, GOMAXPROCS: -2},
	} {
		opts.Scenarios = []*scenario.Scenario{sc}
		opts.Out = t.TempDir()
		if _, err := runner.Run(t.Context(), opts); err == nil {
			t.Errorf("Run(%+v) accepted", opts)
		}
	}
}

func playOK(res *runner.Result, scenario string) report.Check {
	for _, r := range res.Reports {
		if r.Scenario.Name != scenario {
			continue
		}
		for _, c := range r.Checks {
			if c.ID == "play-ok" {
				return c
			}
		}
	}
	return report.Check{}
}

func TestRunRefusesUnknownSet(t *testing.T) {
	sc, _ := scenario.Lookup("etag-stability")
	_, err := runner.Run(t.Context(), runner.Options{Scenarios: []*scenario.Scenario{sc}, Reps: 1, Sets: runner.Sets{"seeks": "3"},
		Out: t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "seeks") {
		t.Errorf("Run = %v, want the unknown param refused", err)
	}
}

func TestSets(t *testing.T) {
	s := runner.Sets{}
	for _, v := range []string{"a=1", "b=x=y"} {
		if err := s.Set(v); err != nil {
			t.Fatal(err)
		}
	}
	if s["a"] != "1" || s["b"] != "x=y" {
		t.Errorf("sets = %v", s)
	}
	if err := s.Set("novalue"); err == nil {
		t.Error("a set without = was accepted")
	}
}
