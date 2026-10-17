package check

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/shapedtime/momoshtrem/internal/streamlab/known"
	"github.com/shapedtime/momoshtrem/internal/streamlab/report"
	"github.com/shapedtime/momoshtrem/internal/streamlab/runner"
	"github.com/shapedtime/momoshtrem/internal/streamlab/scenario"
)

// check is tested with this test binary as its repetitions' process: with
// STREAMLAB_CHECK_FAKE set, TestMain acts a repetition out (a clean one,
// one whose race detector logged races, a deadlock) instead of running a
// scenario, so the pipeline (GORACE per repetition, the logs parsed into
// the reports, the race check, the hang's dump, the verdict) is tested
// without a race build.

const fakeEnv = "STREAMLAB_CHECK_FAKE"

func TestMain(m *testing.M) {
	if mode := os.Getenv(fakeEnv); mode != "" && len(os.Args) > 1 && os.Args[1] == runner.ChildCommand {
		os.Exit(fakeChild(mode, os.Args[2:]))
	}
	os.Exit(m.Run())
}

// The races a fake repetition logs: one in production's read path, one
// elsewhere.
const (
	closeRace = `==================
WARNING: DATA RACE
Write at 0x00c000a1c0f8 by goroutine 214:
  github.com/anacrolix/torrent.(*reader).initStorageReader()
      /m/torrent@v1.60.0/reader.go:441 +0x88
  github.com/shapedtime/momoshtrem/internal/streaming.(*PriorityReader).Read()
      /src/momoshtrem/internal/streaming/reader.go:131 +0x1c4
  github.com/shapedtime/momoshtrem/internal/vfs.(*TorrentFile).readContext.func1()
      /src/momoshtrem/internal/vfs/torrent_file.go:474 +0x6c

Previous write at 0x00c000a1c0f8 by goroutine 198:
  github.com/anacrolix/torrent.(*reader).clearStorageReader()
      /m/torrent@v1.60.0/reader.go:452 +0x7c
  github.com/shapedtime/momoshtrem/internal/streaming.(*PriorityReader).Close()
      /src/momoshtrem/internal/streaming/reader.go:210 +0x2a0
  github.com/shapedtime/momoshtrem/internal/vfs.(*TorrentFile).Close()
      /src/momoshtrem/internal/vfs/torrent_file.go:268 +0x10c

Goroutine 214 (running) created at:
  github.com/shapedtime/momoshtrem/internal/vfs.(*TorrentFile).readContext()
      /src/momoshtrem/internal/vfs/torrent_file.go:473 +0x2b8
==================
`
	sessionRace = `==================
WARNING: DATA RACE
Read at 0x00c0001a6020 by goroutine 9:
  github.com/shapedtime/momoshtrem/internal/sessions.(*Handle).End()
      /src/momoshtrem/internal/sessions/registry.go:190 +0x30

Previous write at 0x00c0001a6020 by goroutine 8:
  github.com/shapedtime/momoshtrem/internal/sessions.(*Registry).sweep()
      /src/momoshtrem/internal/sessions/registry.go:295 +0x80
==================
`
)

func fakeChild(mode string, args []string) int {
	fs := flag.NewFlagSet("fake", flag.ContinueOnError)
	out := fs.String("out", "", "")
	rep := fs.Int("rep", 0, "")
	fs.String("scenario", "", "")
	fs.String("config", "", "")
	fs.Var(runner.Sets{}, "set", "")
	if err := fs.Parse(args); err != nil {
		return runner.ChildToolError
	}
	// The race detector writes <log_path>.<pid>.
	logPath := ""
	for _, kv := range strings.Fields(os.Getenv("GORACE")) {
		if v, ok := strings.CutPrefix(kv, "log_path="); ok {
			logPath = v
		}
	}
	logRaces := func(text string) {
		os.WriteFile(fmt.Sprintf("%s.%d", logPath, os.Getpid()), []byte(text), 0o644)
	}
	switch mode {
	case "deadlock":
		lockOrderDeadlock()
	case "deadlock-stack":
		// The real stack, seeder and all, then the deadlock: the dump
		// shows the stack's goroutines beside it.
		scenario.Run(context.Background(), stackDeadlock, scenario.RunOptions{Rep: *rep, Dir: *out})
	case "race":
		logRaces(closeRace + closeRace)
	case "races":
		logRaces(closeRace + sessionRace)
	}
	env := report.Env{Race: true, GOMAXPROCS: runtime.GOMAXPROCS(0)}
	r := report.Rep{Rep: *rep, Status: report.RepOK, Env: &env, Metrics: map[string]float64{},
		Notes:  []string{"GORACE=" + os.Getenv("GORACE")},
		Checks: []report.RepCheck{{ID: "script", Status: report.Pass}, {ID: "quiescence", Status: report.Pass}}}
	report.WriteJSON(filepath.Join(*out, "rep.json"), r)
	return runner.ChildOK
}

// lockOrderDeadlock deadlocks the process for good: two goroutines take
// two locks in opposite orders. A ticker keeps the runtime from calling
// it a deadlock (the stack's goroutines would): only the hard timeout
// ends it.
func lockOrderDeadlock() {
	var a, b sync.Mutex
	var held, done sync.WaitGroup
	held.Add(2)
	done.Add(2)
	go lockOrderAB(&a, &b, &held, &done)
	go lockOrderBA(&a, &b, &held, &done)
	go func() {
		for range time.Tick(100 * time.Millisecond) {
		}
	}()
	done.Wait()
}

func lockOrderAB(a, b *sync.Mutex, held, done *sync.WaitGroup) {
	defer done.Done()
	a.Lock()
	held.Done()
	held.Wait()
	b.Lock()
}

func lockOrderBA(a, b *sync.Mutex, held, done *sync.WaitGroup) {
	defer done.Done()
	b.Lock()
	held.Done()
	held.Wait()
	a.Lock()
}

// stackDeadlock is a test-only scenario: a real stack serves a probe,
// then the script deadlocks.
var stackDeadlock = &scenario.Scenario{
	Name:   "test-stack-deadlock",
	Tier:   scenario.Stress,
	Doc:    "A probe through the real stack, then a lock-order deadlock in the script.",
	Params: []scenario.Param{{Name: "file_size", Kind: scenario.Bytes, Default: "8MiB"}},
	Script: func(e *scenario.Env) error {
		t := e.Titles[0]
		if err := e.Probe(e.Conn(e.Device("probe")), t, 0, 64<<10); err != nil {
			return err
		}
		lockOrderDeadlock()
		return nil
	},
}

// testScenario is a scenario for the fake repetitions: its name and
// guards are all the runner needs.
func testScenario(guards ...string) *scenario.Scenario {
	return &scenario.Scenario{Name: "test-stress", Tier: scenario.Stress, Doc: "A fake repetition.",
		Guards: guards, Script: func(*scenario.Env) error { return nil }}
}

// runFake checks with this test binary, its repetitions acting out mode.
func runFake(t *testing.T, mode string, opts Options) *Result {
	t.Helper()
	t.Setenv(fakeEnv, mode)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if len(opts.Scenarios) == 0 {
		opts.Scenarios = []*scenario.Scenario{testScenario()}
	}
	opts.Out = t.TempDir()
	res, err := run(t.Context(), opts, Build{Exe: exe, Race: true})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// readSummary reads check.json back.
func readSummary(t *testing.T, dir string) Summary {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "check.json"))
	if err != nil {
		t.Fatal(err)
	}
	var s Summary
	if err := json.Unmarshal(b, &s); err != nil {
		t.Fatal(err)
	}
	if s.Schema != Schema {
		t.Errorf("schema = %q", s.Schema)
	}
	return s
}

func raceCheckOf(r *report.Report) report.Check {
	for _, c := range r.Checks {
		if c.ID == known.RaceCheck {
			return c
		}
	}
	return report.Check{}
}

// Each repetition logs its races to its own folder, and a clean run
// passes with an empty race report.
func TestCheckClean(t *testing.T) {
	res := runFake(t, "clean", Options{Reps: 2})
	s := readSummary(t, res.Dir)
	if s.Verdict != report.VerdictPass || len(s.Races) != 0 || len(s.Hangs) != 0 || !s.Env.Race {
		t.Errorf("summary = %+v", s)
	}
	r := res.Run.Reports[0]
	if c := raceCheckOf(r); c.Status != report.Pass {
		t.Errorf("race check = %+v", c)
	}
	for _, rep := range r.Reps {
		want := "GORACE=log_path=" + filepath.Join(res.Dir, "test-stress", fmt.Sprintf("rep-%d", rep.Rep), "race") + " halt_on_error=0 exitcode=0"
		if len(rep.Notes) == 0 || rep.Notes[0] != want {
			t.Errorf("rep %d's GORACE = %q, want %q", rep.Rep, rep.Notes, want)
		}
		if rep.Metrics["race_reports"] != 0 {
			t.Errorf("rep %d: race_reports = %v", rep.Rep, rep.Metrics["race_reports"])
		}
	}
	if _, err := os.Stat(filepath.Join(res.Dir, "check.md")); err != nil {
		t.Error(err)
	}
}

// A race fails the check: its report, deduplicated, is in the
// repetition, the scenario's report and check.json.
func TestCheckRace(t *testing.T) {
	res := runFake(t, "race", Options{})
	s := readSummary(t, res.Dir)
	if s.Verdict != report.VerdictFail || len(s.Races) != 1 {
		t.Fatalf("verdict %s, races %+v", s.Verdict, s.Races)
	}
	rc := s.Races[0]
	if rc.Count != 2 || rc.Known != "" || !slices.Equal(rc.Scenarios, []string{"test-stress"}) ||
		len(rc.Where) != 1 || !strings.HasPrefix(rc.Where[0], filepath.Join("test-stress", "rep-1", "race.")) {
		t.Errorf("race = %+v", rc)
	}
	if !strings.Contains(rc.Signature, "internal/vfs.(*TorrentFile).readContext.func1") || len(rc.Accesses) != 2 {
		t.Errorf("signature %q, accesses %+v", rc.Signature, rc.Accesses)
	}
	r := res.Run.Reports[0]
	if c := raceCheckOf(r); c.Status != report.Fail || c.Known != "" {
		t.Errorf("race check = %+v", c)
	}
	if len(r.Races) != 1 || r.Metrics["race_reports"].Values[0] != 1 || r.Metrics["race_reports"].Kind != report.Exact {
		t.Errorf("report races %+v, metric %+v", r.Races, r.Metrics["race_reports"])
	}
	if !strings.Contains(s.Why, "1 new data races") {
		t.Errorf("why = %q", s.Why)
	}
	md, _ := os.ReadFile(filepath.Join(res.Dir, "check.md"))
	if !strings.Contains(string(md), "WARNING: DATA RACE") || !strings.Contains(string(md), "**Scenarios:** test-stress") {
		t.Errorf("check.md:\n%s", md)
	}
}

// registerRace registers a known race for the test's length.
func registerRace(t *testing.T, id, signature string) {
	t.Helper()
	races, issues := known.Races, known.Issues
	t.Cleanup(func() { known.Races, known.Issues = races, issues })
	is := known.Issue{ID: id, Title: "a test race", Ref: id, Checks: []string{known.RaceCheck}, MaxRate: known.Always, Doc: "test"}
	known.Races = append(slices.Clone(races), known.RaceIssue{Issue: is, Signature: signature})
	known.Issues = append(slices.Clone(issues), is)
}

// A race registered with its exact signature is an xfail in the scenario
// that guards it; a new race beside it still fails.
func TestCheckKnownRace(t *testing.T) {
	sig := signatureOf(t, closeRace)
	registerRace(t, "#9001", sig)
	guarded := []*scenario.Scenario{testScenario("#9001")}

	res := runFake(t, "race", Options{Scenarios: guarded})
	s := readSummary(t, res.Dir)
	if c := raceCheckOf(res.Run.Reports[0]); s.Verdict != report.VerdictPass || c.Status != report.XFail || c.Known != "#9001" {
		t.Errorf("a known race: verdict %s (%s), race check %+v", s.Verdict, s.Why, c)
	}
	if len(s.Races) != 1 || s.Races[0].Known != "#9001" || !strings.Contains(s.Why, "1 known races") {
		t.Errorf("races %+v, why %q", s.Races, s.Why)
	}

	res = runFake(t, "races", Options{Scenarios: guarded})
	s = readSummary(t, res.Dir)
	if c := raceCheckOf(res.Run.Reports[0]); s.Verdict != report.VerdictFail || c.Status != report.Fail {
		t.Errorf("a new race beside the known one: verdict %s, race check %+v", s.Verdict, c)
	}

	// Where it isn't guarded, a known race can't explain the failure.
	res = runFake(t, "race", Options{})
	if s := readSummary(t, res.Dir); s.Verdict != report.VerdictFail {
		t.Errorf("a known race in a scenario that doesn't guard it: verdict %s", s.Verdict)
	}
}

// A deliberate deadlock (two goroutines taking two locks in opposite
// orders) is caught: the repetition outlives its hard timeout, the runner
// sends SIGQUIT and keeps the goroutine dump, and the check fails with
// the hang listed.
func TestCheckDeadlock(t *testing.T) {
	res := runFake(t, "deadlock", Options{Timeout: 3 * time.Second})
	s := readSummary(t, res.Dir)
	if s.Verdict != report.VerdictFail || len(s.Hangs) != 1 {
		t.Fatalf("verdict %s, hangs %+v", s.Verdict, s.Hangs)
	}
	h := s.Hangs[0]
	if h.Scenario != "test-stress" || h.Rep != 1 || h.Dump == "" || !strings.Contains(s.Why, "hung past the hard timeout") {
		t.Errorf("hang = %+v, why %q", h, s.Why)
	}
	dump, err := os.ReadFile(filepath.Join(res.Dir, h.Dump))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"SIGQUIT", "lockOrderAB", "lockOrderBA", "sync.(*Mutex).Lock"} {
		if !strings.Contains(string(dump), want) {
			t.Errorf("the dump lacks %q:\n%.2000s", want, dump)
		}
	}
}

// The same deadlock with the real stack running (STREAMLAB=1: it starts a
// seeder and a stack): caught, dumped with the stack's goroutines.
func TestCheckDeadlockInStack(t *testing.T) {
	if os.Getenv("STREAMLAB") == "" {
		t.Skip("set STREAMLAB=1 to deadlock a real stack")
	}
	res := runFake(t, "deadlock-stack", Options{Scenarios: []*scenario.Scenario{stackDeadlock}, Timeout: time.Minute})
	s := readSummary(t, res.Dir)
	if s.Verdict != report.VerdictFail || len(s.Hangs) != 1 || s.Hangs[0].Dump == "" {
		t.Fatalf("verdict %s, hangs %+v", s.Verdict, s.Hangs)
	}
	dump, err := os.ReadFile(filepath.Join(res.Dir, s.Hangs[0].Dump))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"lockOrderAB", "lockOrderBA", "github.com/anacrolix/torrent.", "internal/torrent.(*ActivityManager)"} {
		if !strings.Contains(string(dump), want) {
			t.Errorf("the dump lacks %q", want)
		}
	}
}

// A binary that isn't a race build is refused; the repetitions' env
// saying race=false is an error.
func TestNotARaceBuild(t *testing.T) {
	exe, _ := os.Executable()
	b, err := Inspect(Build{Exe: exe})
	if report.CollectEnv().Race {
		if err != nil || !b.Race {
			t.Errorf("this race test binary: %+v, %v", b, err)
		}
	} else if err == nil || !strings.Contains(err.Error(), "isn't a race build") {
		t.Errorf("Inspect(a build without -race) = %+v, %v", b, err)
	}
	s := &Summary{Verdict: report.VerdictPass}
	s.judge()
	if s.Verdict != report.VerdictError {
		t.Errorf("env.race false: verdict %s", s.Verdict)
	}
}

// The verdict: a new race, a hang or a failed test fails; go test that
// can't run is an error unless something failed; a run that wasn't a
// race build is an error whatever it found.
func TestJudge(t *testing.T) {
	race := report.Env{Race: true}
	newRace := []Race{{Race: report.Race{Signature: "x"}}}
	knownRace := []Race{{Race: report.Race{Signature: "y", Known: "#9"}}}
	for _, tt := range []struct {
		name string
		s    Summary
		want report.Verdict
	}{
		{"clean", Summary{Verdict: report.VerdictPass, Env: race}, report.VerdictPass},
		{"a known race", Summary{Verdict: report.VerdictPass, Env: race, Races: knownRace}, report.VerdictPass},
		{"a new race", Summary{Verdict: report.VerdictPass, Env: race, Races: newRace}, report.VerdictFail},
		{"a hang", Summary{Verdict: report.VerdictPass, Env: race, Hangs: []Hang{{Scenario: "x"}}}, report.VerdictFail},
		{"a scenario errored", Summary{Verdict: report.VerdictError, Env: race}, report.VerdictError},
		{"errored and a new race", Summary{Verdict: report.VerdictError, Env: race, Races: newRace}, report.VerdictFail},
		{"tests failed", Summary{Verdict: report.VerdictPass, Env: race, Tests: &Tests{Verdict: report.VerdictFail}}, report.VerdictFail},
		{"tests couldn't run", Summary{Verdict: report.VerdictPass, Env: race, Tests: &Tests{Verdict: report.VerdictError, Error: "x"}}, report.VerdictError},
		{"not a race build", Summary{Verdict: report.VerdictFail, Env: race, Races: newRace, notRace: true}, report.VerdictError},
	} {
		s := tt.s
		s.judge()
		if s.Verdict != tt.want || s.Why == "" {
			t.Errorf("%s: verdict %s (%s), want %s", tt.name, s.Verdict, s.Why, tt.want)
		}
	}
}

func TestRaceCheck(t *testing.T) {
	newRace := report.Race{Signature: "write a | write b"}
	knownA := report.Race{Signature: "x", Known: "#2"}
	knownB := report.Race{Signature: "y", Known: "#1"}
	for _, tt := range []struct {
		races  []report.Race
		status report.Status
		known  string
	}{
		{nil, report.Pass, ""},
		{[]report.Race{newRace}, report.Fail, ""},
		{[]report.Race{knownA, knownB, knownA}, report.Fail, "#1+#2"},
		{[]report.Race{knownA, newRace}, report.Fail, ""},
	} {
		c := raceCheck(tt.races)
		if c.ID != known.RaceCheck || c.Status != tt.status || c.Known != tt.known || c.Evidence == "" {
			t.Errorf("raceCheck(%v) = %+v, want %s %q", tt.races, c, tt.status, tt.known)
		}
	}
}

func TestParseTestJSON(t *testing.T) {
	events := []string{
		`{"Action":"start","Package":"m/a"}`,
		`{"Action":"run","Package":"m/a","Test":"TestX"}`,
		`{"Action":"output","Package":"m/a","Test":"TestX","Output":"=== RUN   TestX\n"}`,
	}
	for _, line := range strings.SplitAfter(closeRace, "\n") {
		b, _ := json.Marshal(map[string]string{"Action": "output", "Package": "m/a", "Test": "TestX", "Output": line})
		events = append(events, string(b))
	}
	events = append(events,
		`{"Action":"output","Package":"m/a","Test":"TestX","Output":"    testing.go:1617: race detected during execution of test\n"}`,
		`{"Action":"fail","Package":"m/a","Test":"TestX","Elapsed":0.5}`,
		`{"Action":"fail","Package":"m/a","Elapsed":1.5}`,
		`{"Action":"pass","Package":"m/b","Elapsed":0.2}`,
		`{"Action":"output","Package":"m/c","Output":"?   \tm/c\t[no test files]\n"}`,
		`{"Action":"skip","Package":"m/c","Elapsed":0}`,
		`not json`,
	)
	var out strings.Builder
	pkgs, races, err := parseTestJSON(strings.NewReader(strings.Join(events, "\n")), &out)
	if err != nil {
		t.Fatal(err)
	}
	want := []Package{
		{Name: "m/a", Status: "fail", ElapsedS: 1.5, Failed: []string{"TestX"}, Races: 1},
		{Name: "m/b", Status: "pass", ElapsedS: 0.2},
		{Name: "m/c", Status: "skip"},
	}
	if fmt.Sprint(pkgs) != fmt.Sprint(want) {
		t.Errorf("packages = %+v\nwant %+v", pkgs, want)
	}
	if len(races) != 1 || races[0].Where[0] != "m/a" {
		t.Errorf("races = %+v", races)
	}
	if !strings.Contains(out.String(), "WARNING: DATA RACE") || !strings.Contains(out.String(), "not json") {
		t.Errorf("text output:\n%s", out.String())
	}
	tests := Tests{Verdict: report.VerdictFail, Packages: pkgs}
	if got := tests.failed(); !slices.Equal(got, []string{"m/a (TestX)"}) {
		t.Errorf("failed = %v", got)
	}
}

func TestModuleDir(t *testing.T) {
	wd, _ := os.Getwd() // internal/streamlab/check
	root := filepath.Clean(filepath.Join(wd, "..", "..", ".."))
	for _, from := range []string{wd, root, filepath.Dir(root)} {
		got, err := ModuleDir(from)
		if err != nil || got != root {
			t.Errorf("ModuleDir(%s) = %s, %v; want %s", from, got, err, root)
		}
	}
	if _, err := ModuleDir(t.TempDir()); err == nil {
		t.Error("found a module in an empty folder")
	}
}

// The race build works here: cgo, -race and the lab's tags (STREAMLAB=1:
// it takes a minute).
func TestBuildRace(t *testing.T) {
	if os.Getenv("STREAMLAB") == "" {
		t.Skip("set STREAMLAB=1 to build the race binary")
	}
	root, err := ModuleDir(".")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Minute)
	defer cancel()
	b, err := BuildRace(ctx, root, filepath.Join(t.TempDir(), BinName))
	if err != nil {
		t.Fatal(err)
	}
	if !b.Race || !strings.Contains(","+b.Tags+",", ","+Tags+",") || b.TookS <= 0 {
		t.Errorf("build = %+v", b)
	}
}

// go test -race over one small package: its result, its time and its
// output (STREAMLAB=1: it compiles with -race).
func TestGoTestRace(t *testing.T) {
	if os.Getenv("STREAMLAB") == "" {
		t.Skip("set STREAMLAB=1 to run go test -race")
	}
	root, err := ModuleDir(".")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	got := GoTestRace(t.Context(), root, dir, []string{"./internal/streamlab/race"})
	if got.Verdict != report.VerdictPass || len(got.Packages) != 1 || got.Packages[0].Status != "pass" || got.TookS <= 0 {
		t.Errorf("GoTestRace = %+v", got)
	}
	if out, err := os.ReadFile(filepath.Join(dir, TestOutput)); err != nil || !strings.Contains(string(out), "TestParse") {
		t.Errorf("%s: %v\n%.500s", TestOutput, err, out)
	}
}

// signatureOf parses one report's signature.
func signatureOf(t *testing.T, text string) string {
	t.Helper()
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "race.1"), []byte(text), 0o644)
	rep := &report.Rep{}
	collectRaces(dir, rep)
	if len(rep.Races) != 1 {
		t.Fatalf("%d races in the fixture", len(rep.Races))
	}
	return rep.Races[0].Signature
}
