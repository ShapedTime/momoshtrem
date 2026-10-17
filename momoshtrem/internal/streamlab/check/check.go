// Package check is streamlab check (docs/streamlab/PLAN.md D8, S7 #178):
// races, deadlocks and leaks, kept apart from timing because -race
// distorts it.
//
// It builds the lab with -race (BuildRace), runs the stress tier through
// the runner with that binary, each repetition's races logged to its
// folder (GORACE log_path), and turns the "WARNING: DATA RACE" blocks into
// JSON: race_reports on every repetition and report, deduplicated by
// signature (internal/streamlab/race), and a race check that fails on any
// race but a known one, matched by its exact signature. A repetition that
// hangs past its hard timeout is SIGQUIT and its goroutine dump kept (the
// runner): a deadlock is a fail. Then go test -race runs over the
// streaming path's packages (TestPackages). check.json
// (streamlab.check/v1) and check.md sum it all up beside the run's index.
package check

import (
	"cmp"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/shapedtime/momoshtrem/internal/streamlab/known"
	"github.com/shapedtime/momoshtrem/internal/streamlab/race"
	"github.com/shapedtime/momoshtrem/internal/streamlab/report"
	"github.com/shapedtime/momoshtrem/internal/streamlab/runner"
	"github.com/shapedtime/momoshtrem/internal/streamlab/scenario"
)

// Schema names check.json's format.
const Schema = "streamlab.check/v1"

// BinName is the race binary's name in runner.Dir()/bin.
const BinName = "streamlab-race"

// Options configure a check.
type Options struct {
	// Scenarios are what to run; the stress tier when empty. Each gets
	// the race check (known.RaceCheck).
	Scenarios []*scenario.Scenario
	Reps      int
	Warmup    int
	Sets      map[string]string
	// ConfigPath is a YAML config laid over the defaults.
	ConfigPath string
	// Out is the run's folder; runner.Dir()/runs/<id> when empty.
	Out        string
	GOMAXPROCS int
	Timeout    time.Duration
	// Exe is a race build of streamlab to run: check builds one into
	// runner.Dir()/bin when empty.
	Exe string
	// Tests runs go test -race over TestPackages after the stress tier.
	Tests bool
	// ModuleDir is momoshtrem's module root; ModuleDir(".") when empty.
	ModuleDir string
	// Command is the command line, for the reports.
	Command []string
	// Progress gets a line per step and per repetition; nil for none.
	Progress io.Writer
}

// Result is a check's outcome.
type Result struct {
	Dir     string
	Summary *Summary
	Run     *runner.Result
}

// Summary is check.json: the build, the stress tier's verdicts, every
// race and every hung repetition, and go test -race.
type Summary struct {
	Schema string      `json:"schema"`
	Tool   report.Tool `json:"tool"`
	// Env is the race binary's: race is true.
	Env   report.Env `json:"env"`
	Build Build      `json:"build"`
	// Verdict is fail when a scenario failed (a race included), a
	// repetition hung or crashed, or go test -race failed; error when the
	// tool couldn't run something.
	Verdict report.Verdict `json:"verdict"`
	// Why says what decided the verdict, on one line.
	Why       string              `json:"why"`
	Run       report.Run          `json:"run"`
	Scenarios []report.IndexEntry `json:"scenarios"`
	// Races are the stress tier's data races, by signature, over every
	// scenario and repetition (warm-ups included).
	Races []Race `json:"race_reports"`
	// Hangs are the repetitions that outlived their hard timeout: the
	// runner sent SIGQUIT and kept the goroutine dump.
	Hangs []Hang `json:"hangs"`
	// Tests is go test -race's outcome; nil when skipped.
	Tests *Tests `json:"go_test,omitempty"`

	// notRace is set when a repetition's process said it wasn't a race
	// build.
	notRace bool
}

// Race is a race and the scenarios it was reported in.
type Race struct {
	report.Race
	Scenarios []string `json:"scenarios"`
}

// Hang is a repetition that hit the hard timeout.
type Hang struct {
	Scenario string `json:"scenario"`
	Rep      int    `json:"rep"`
	Warmup   bool   `json:"warmup,omitempty"`
	// Dump is the goroutine dump, relative to the run's folder.
	Dump  string `json:"dump,omitempty"`
	Error string `json:"error"`
}

// Run runs a check. The error is a tool error that stopped it: a module
// it can't find, a race build that fails, a folder it can't write, an
// interrupt.
func Run(ctx context.Context, opts Options) (*Result, error) {
	if opts.ModuleDir == "" {
		var err error
		if opts.ModuleDir, err = ModuleDir("."); err != nil {
			return nil, err
		}
	}
	var b Build
	var err error
	if opts.Exe != "" {
		if b, err = Inspect(Build{Exe: opts.Exe}); err != nil {
			return nil, err
		}
	} else {
		out := filepath.Join(runner.Dir(), "bin", BinName)
		opts.progress("building %s with -race -tags %s …", out, Tags)
		if b, err = BuildRace(ctx, opts.ModuleDir, out); err != nil {
			return nil, err
		}
		opts.progress("built in %.0f s", b.TookS)
	}
	return run(ctx, opts, b)
}

func (o Options) progress(format string, args ...any) {
	if o.Progress != nil {
		fmt.Fprintf(o.Progress, format+"\n", args...)
	}
}

// run runs the check with the binary b.
func run(ctx context.Context, opts Options, b Build) (*Result, error) {
	scs := opts.Scenarios
	if len(scs) == 0 {
		scs = scenario.InTier(scenario.Stress)
	}
	res, err := runner.Run(ctx, runner.Options{
		Scenarios:  withRaceCheck(scs),
		Reps:       cmp.Or(opts.Reps, 1),
		Warmup:     opts.Warmup,
		Sets:       opts.Sets,
		ConfigPath: opts.ConfigPath,
		Out:        opts.Out,
		GOMAXPROCS: opts.GOMAXPROCS,
		Timeout:    opts.Timeout,
		Exe:        b.Exe,
		RepEnv:     func(repDir string) []string { return []string{race.Env(repDir)} },
		AfterRep:   collectRaces,
		Command:    opts.Command,
		Progress:   opts.Progress,
	})
	if err != nil {
		return nil, err
	}
	s := summarize(res, b)
	if opts.Tests {
		opts.progress("go test -race over %s …", strings.Join(TestPackages, " "))
		t := GoTestRace(ctx, opts.ModuleDir, res.Dir, TestPackages)
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		s.Tests = &t
		opts.progress("go test -race: %s in %.0f s", t.Verdict, t.TookS)
	}
	s.judge()
	if err := s.Write(res.Dir); err != nil {
		return nil, err
	}
	return &Result{Dir: res.Dir, Summary: s, Run: res}, nil
}

// withRaceCheck is each scenario with the race check added: the runner
// judges it from what collectRaces records.
func withRaceCheck(scs []*scenario.Scenario) []*scenario.Scenario {
	out := make([]*scenario.Scenario, len(scs))
	for i, sc := range scs {
		c := *sc
		if !slices.Contains(c.Checks, known.RaceCheck) {
			c.Checks = append(slices.Clone(sc.Checks), known.RaceCheck)
		}
		out[i] = &c
	}
	return out
}

// collectRaces reads a repetition's race logs into it once its process
// ended (runner.Options.AfterRep): race_reports, its metric, and the race
// check.
func collectRaces(repDir string, rep *report.Rep) {
	races, err := race.ParseDir(repDir, filepath.Base(repDir))
	if err != nil {
		rep.Notes = append(rep.Notes, "reading the race logs: "+err.Error())
	}
	for i := range races {
		if is, ok := known.MatchRace(races[i].Signature); ok {
			races[i].Known = is.ID
		}
	}
	rep.Races = races
	if rep.Metrics == nil {
		rep.Metrics = map[string]float64{}
	}
	rep.Metrics["race_reports"] = float64(len(races))
	rep.Checks = slices.DeleteFunc(rep.Checks, func(c report.RepCheck) bool { return c.ID == known.RaceCheck })
	rep.Checks = append(rep.Checks, raceCheck(races))
}

// raceCheck is a repetition's race check: pass without a race; a fail
// naming the known issues when every race carries a known signature
// exactly; a plain fail otherwise.
func raceCheck(races []report.Race) report.RepCheck {
	c := report.RepCheck{ID: known.RaceCheck, Status: report.Pass, Evidence: "no data race reported"}
	if len(races) == 0 {
		return c
	}
	c.Status = report.Fail
	var ids, sigs []string
	allKnown := true
	for _, r := range races {
		if r.Known == "" {
			allKnown = false
			sigs = append(sigs, r.Signature)
			continue
		}
		if !slices.Contains(ids, r.Known) {
			ids = append(ids, r.Known)
		}
	}
	slices.Sort(ids)
	if allKnown {
		c.Known = strings.Join(ids, known.Sep)
		c.Evidence = fmt.Sprintf("%d known races (%s)", len(races), c.Known)
		return c
	}
	c.Evidence = fmt.Sprintf("%d data races, %d new: %s", len(races), len(sigs), strings.Join(sigs, "; "))
	return c
}

// summarize sums a stress run up.
func summarize(res *runner.Result, b Build) *Summary {
	s := &Summary{Schema: Schema, Tool: res.Index.Tool, Env: res.Index.Env, Build: b, Verdict: res.Index.Verdict,
		Run: res.Index.Run, Scenarios: res.Index.Scenarios, Races: []Race{}, Hangs: []Hang{}}
	// The env is the repetitions' own: the first that reported one (a hung
	// or crashed one reports none), else the binary's build info.
	childEnv := false
	for _, r := range res.Reports {
		for _, rep := range r.Reps {
			if rep.Env == nil {
				continue
			}
			if !childEnv {
				s.Env, childEnv = *rep.Env, true
			}
			s.notRace = s.notRace || !rep.Env.Race
		}
	}
	if !childEnv {
		s.Env.Race = b.Race
	}
	for _, r := range res.Reports {
		for _, rc := range r.Races {
			i := slices.IndexFunc(s.Races, func(x Race) bool { return x.Signature == rc.Signature })
			if i < 0 {
				rc.Where = prefixed(r.Scenario.Name, rc.Where)
				s.Races = append(s.Races, Race{Race: rc, Scenarios: []string{r.Scenario.Name}})
				continue
			}
			s.Races[i].Count += rc.Count
			s.Races[i].Where = append(s.Races[i].Where, prefixed(r.Scenario.Name, rc.Where)...)
			if !slices.Contains(s.Races[i].Scenarios, r.Scenario.Name) {
				s.Races[i].Scenarios = append(s.Races[i].Scenarios, r.Scenario.Name)
			}
		}
		for _, rep := range r.Reps {
			if rep.Status != report.RepTimeout {
				continue
			}
			h := Hang{Scenario: r.Scenario.Name, Rep: rep.Rep, Warmup: rep.Warmup, Error: rep.Error}
			if d := rep.Artifacts["goroutines"]; d != "" {
				h.Dump = filepath.Join(r.Scenario.Name, d)
			}
			s.Hangs = append(s.Hangs, h)
		}
	}
	return s
}

func prefixed(dir string, paths []string) []string {
	out := make([]string, len(paths))
	for i, p := range paths {
		out[i] = filepath.Join(dir, p)
	}
	return out
}

// judge decides the verdict and why: the stress run's, failed by any new
// race (a warm-up's too) and by go test -race; an error when a build
// isn't a race build after all.
func (s *Summary) judge() {
	var why []string
	notRace := s.notRace || !s.Env.Race
	if notRace {
		why = append(why, "the repetitions didn't run a race build (env.race false)")
	}
	// fail beats error, as in the run's index; a run that wasn't a race
	// build is an error whatever it found.
	failed, errored := s.Verdict == report.VerdictFail, s.Verdict == report.VerdictError
	defer func() {
		switch {
		case notRace, errored && !failed:
			s.Verdict = report.VerdictError
		case failed:
			s.Verdict = report.VerdictFail
		default:
			s.Verdict = report.VerdictPass
		}
	}()
	var bad []string
	for _, e := range s.Scenarios {
		if e.Verdict != report.VerdictPass {
			bad = append(bad, fmt.Sprintf("%s %s (%s)", e.Name, e.Verdict, strings.Join(append(slices.Clone(e.Checks), e.Error), " ")))
		}
	}
	if len(bad) > 0 {
		why = append(why, "scenarios: "+strings.Join(bad, "; "))
	}
	newRaces := 0
	for _, r := range s.Races {
		if r.Known == "" {
			newRaces++
		}
	}
	if newRaces > 0 {
		failed = true
		why = append(why, fmt.Sprintf("%d new data races", newRaces))
	}
	if n := len(s.Races) - newRaces; n > 0 {
		why = append(why, fmt.Sprintf("%d known races", n))
	}
	if len(s.Hangs) > 0 {
		failed = true
		why = append(why, fmt.Sprintf("%d repetitions hung past the hard timeout (goroutine dumps kept)", len(s.Hangs)))
	}
	if t := s.Tests; t != nil {
		switch t.Verdict {
		case report.VerdictFail:
			failed = true
			why = append(why, "go test -race failed: "+strings.Join(t.failed(), ", "))
		case report.VerdictError:
			errored = true
			why = append(why, "go test -race couldn't run: "+t.Error)
		}
		if len(t.Races) > 0 {
			why = append(why, fmt.Sprintf("go test -race reported %d races", len(t.Races)))
		}
	}
	if len(why) == 0 {
		why = append(why, "no race, no hang, every check passed or is a known issue")
	}
	s.Why = strings.Join(why, "; ")
}

// failed names the packages that failed, with their failed tests.
func (t *Tests) failed() []string {
	var out []string
	for _, p := range t.Packages {
		if p.Status != "fail" {
			continue
		}
		s := p.Name
		if len(p.Failed) > 0 {
			s += " (" + strings.Join(p.Failed, ", ") + ")"
		}
		out = append(out, s)
	}
	if len(out) == 0 && t.Error != "" {
		out = append(out, t.Error)
	}
	return out
}

// Write writes check.json and check.md to dir.
func (s *Summary) Write(dir string) error {
	if err := report.WriteJSON(filepath.Join(dir, "check.json"), s); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "check.md"), []byte(s.Markdown()), 0o644)
}
