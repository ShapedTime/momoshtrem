// Package runner runs streamlab's scenarios (docs/streamlab/PLAN.md D4):
// each repetition in a process of its own, a warm-up repetition first and
// discarded, GOMAXPROCS pinned on request, and a hard timeout that dumps
// the stuck process's goroutines. It then judges each scenario over its
// repetitions (known.Judge) and writes report.json and report.md per
// scenario, and an index of the run.
//
// slog's default logger, the MIME table, the sync.Pools and
// DefaultServeMux are process-wide: a fresh process per repetition gives
// each a clean baseline, and a deadlock can't outlive its repetition.
package runner

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/shapedtime/momoshtrem/internal/streamlab/checks"
	"github.com/shapedtime/momoshtrem/internal/streamlab/known"
	"github.com/shapedtime/momoshtrem/internal/streamlab/report"
	"github.com/shapedtime/momoshtrem/internal/streamlab/scenario"
)

// ChildCommand is the hidden subcommand a repetition's process runs.
const ChildCommand = "_child"

// Exit codes of a repetition's process.
const (
	// ChildOK: the repetition ran and wrote rep.json, whatever its checks.
	ChildOK = 0
	// ChildToolError: the tool or the environment failed it; rep.json
	// says why.
	ChildToolError = 3
)

// killGrace is how long a timed-out repetition gets to dump its
// goroutines after SIGQUIT before it's killed.
const killGrace = 10 * time.Second

// Options configure a run.
type Options struct {
	Scenarios []*scenario.Scenario
	// Reps is how many measured repetitions each scenario gets.
	Reps int
	// Warmup is how many repetitions run first and are discarded.
	Warmup int
	// Sets are --set overrides, applied to every scenario that declares
	// the param; a param no scenario declares is an error.
	Sets map[string]string
	// ConfigPath is a YAML config laid over the defaults.
	ConfigPath string
	// Out is the run's folder; Dir() under the repo when empty.
	Out string
	// GOMAXPROCS pins the repetitions' GOMAXPROCS; 0 leaves the default.
	GOMAXPROCS int
	// Timeout is each repetition's hard timeout; the scenario's when 0.
	Timeout time.Duration
	// Exe is the streamlab binary to run repetitions with;
	// os.Executable() when empty.
	Exe string
	// Command is the command line, for the report.
	Command []string
	// Progress gets a line per repetition; nil for none.
	Progress io.Writer
}

// Result is a run's outcome.
type Result struct {
	Dir     string
	Index   *report.Index
	Reports []*report.Report
}

// Failed reports whether a scenario failed.
func (r *Result) Failed() bool { return r.Index.Verdict == report.VerdictFail }

// Errored reports whether the tool or the environment failed a scenario.
func (r *Result) Errored() bool {
	for _, s := range r.Index.Scenarios {
		if s.Verdict == report.VerdictError {
			return true
		}
	}
	return false
}

// Run runs the scenarios. The error is a tool error that stopped the run
// (bad options, a folder it can't write, an interrupt); a scenario the
// tool couldn't run is an error verdict in the result.
func Run(ctx context.Context, opts Options) (*Result, error) {
	switch {
	case len(opts.Scenarios) == 0:
		return nil, errors.New("no scenario to run")
	case opts.Reps < 1:
		return nil, errors.New("reps must be at least 1")
	case opts.Warmup < 0:
		return nil, errors.New("warmup can't be negative")
	case opts.Timeout < 0:
		return nil, errors.New("the hard timeout must be positive (0: the scenario's)")
	case opts.GOMAXPROCS < 0:
		return nil, errors.New("gomaxprocs can't be negative")
	}
	if err := checkSets(opts.Scenarios, opts.Sets); err != nil {
		return nil, err
	}
	params := make([]scenario.Params, len(opts.Scenarios))
	for si, sc := range opts.Scenarios {
		p, err := sc.Resolve(setsFor(sc, opts.Sets))
		if err != nil {
			return nil, err
		}
		params[si] = p
	}
	exe := opts.Exe
	if exe == "" {
		var err error
		if exe, err = os.Executable(); err != nil {
			return nil, fmt.Errorf("finding the streamlab binary: %w", err)
		}
	}
	started := time.Now()
	runID := fmt.Sprintf("%s-%d", started.Format("20060102-150405"), os.Getpid())
	dir := opts.Out
	if dir == "" {
		dir = filepath.Join(Dir(), "runs", runID)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating %s: %w", dir, err)
	}
	if opts.Out == "" {
		latest := filepath.Join(Dir(), "runs", "latest")
		os.Remove(latest)
		os.Symlink(runID, latest) // a convenience: ignore a failure
	}

	// The parent's view of the build and the machine; each report takes
	// its children's, which ran the scenario (S8's ab runs two binaries).
	tool := report.ToolInfo(".")
	env := report.CollectEnv()
	env.GOMAXPROCS = cmp.Or(opts.GOMAXPROCS, env.GOMAXPROCS)
	resolvesMKV := checks.MIME().ResolvesMKV()
	run := report.Run{ID: runID, Reps: opts.Reps, Warmup: opts.Warmup, GOMAXPROCS: opts.GOMAXPROCS,
		Started: started, Command: opts.Command}
	res := &Result{Dir: dir, Index: &report.Index{Schema: report.IndexSchema, Tool: tool, Env: env, Verdict: report.VerdictPass}}

	// Run every scenario, writing each one's report as it ends. An
	// intermittent issue's rate counts over the whole run, so once all
	// have run, the reports are judged again with the run's tally.
	var done []scenarioRun
	for si, sc := range opts.Scenarios {
		scDir := filepath.Join(dir, sc.Name)
		if err := os.MkdirAll(scDir, 0o755); err != nil {
			return nil, err
		}
		sets := setsFor(sc, opts.Sets)
		timeout := cmp.Or(opts.Timeout, sc.HardTimeout())
		sr := scenarioRun{sc: sc, p: params[si], dir: scDir, run: run}
		sr.run.TimeoutS = timeout.Seconds()
		scStart := time.Now()
		next := func(n int, warmup, confirm bool) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			rep := runRep(ctx, exe, sc, n, warmup, sets, opts, scDir, timeout)
			if err := ctx.Err(); err != nil {
				return err // interrupted: the repetition was killed
			}
			rep.Confirm = confirm
			sr.reps = append(sr.reps, rep)
			progress(opts.Progress, sc.Name, rep)
			return nil
		}
		for n := 1 - opts.Warmup; n <= opts.Reps; n++ {
			if err := next(n, n <= 0, false); err != nil {
				return nil, err
			}
		}
		// An intermittent issue that fired in too few repetitions to tell
		// from its known rate: confirm it with more (known.ConfirmReps).
		if id, ok := needsConfirmation(sr.reps); ok {
			if opts.Progress != nil {
				fmt.Fprintf(opts.Progress, "%-18s %s fired in under %d measured reps: %d confirmation reps\n",
					sc.Name, id, known.ConfirmBelow, known.ConfirmReps)
			}
			for c := 1; c <= known.ConfirmReps; c++ {
				if err := next(opts.Reps+c, false, true); err != nil {
					return nil, err
				}
			}
			sr.run.Confirmations = known.ConfirmReps
		}
		sr.run.WallS = time.Since(scStart).Seconds()
		kenv := known.Env{MIMEResolvesMKV: resolvesMKV, Guards: sc.Guards}
		if err := build(sr, tool, env, kenv, opts.ConfigPath).Write(scDir); err != nil {
			return nil, fmt.Errorf("writing %s's report: %w", sc.Name, err)
		}
		done = append(done, sr)
	}

	tally := known.TallyRun(outcomes(done))
	for _, sr := range done {
		kenv := known.Env{MIMEResolvesMKV: resolvesMKV, Guards: sr.sc.Guards, Run: tally}
		r := build(sr, tool, env, kenv, opts.ConfigPath)
		if err := r.Write(sr.dir); err != nil {
			return nil, fmt.Errorf("writing %s's report: %w", sr.sc.Name, err)
		}
		res.Reports = append(res.Reports, r)
		entry := report.NewIndexEntry(r, sr.run.WallS, sr.sc.Key, filepath.Join(sr.sc.Name, "report.json"))
		res.Index.Scenarios = append(res.Index.Scenarios, entry)
		switch r.Verdict {
		case report.VerdictFail:
			res.Index.Verdict = report.VerdictFail
		case report.VerdictError:
			if res.Index.Verdict == report.VerdictPass {
				res.Index.Verdict = report.VerdictError
			}
		}
	}
	if len(res.Reports) > 0 {
		res.Index.Tool, res.Index.Env = res.Reports[0].Tool, res.Reports[0].Env
	}
	run.WallS = time.Since(started).Seconds()
	res.Index.Run = run
	if err := res.Index.Write(dir); err != nil {
		return nil, fmt.Errorf("writing the run's index: %w", err)
	}
	return res, nil
}

// needsConfirmation reports whether an intermittent issue fired in a
// scenario with fewer than known.ConfirmBelow measured repetitions, and
// which.
func needsConfirmation(reps []report.Rep) (string, bool) {
	measured := 0
	fired := ""
	for _, m := range outcomes([]scenarioRun{{reps: reps}}) {
		measured++
		if id, ok := known.Fired(m); ok && fired == "" {
			fired = id
		}
	}
	return fired, fired != "" && measured < known.ConfirmBelow
}

// scenarioRun is one scenario's repetitions in a run.
type scenarioRun struct {
	sc   *scenario.Scenario
	p    scenario.Params
	dir  string
	run  report.Run
	reps []report.Rep
}

// outcomes are the checks of every measured repetition that ran, by
// check ID: what TallyRun counts.
func outcomes(runs []scenarioRun) []map[string]known.Outcome {
	var out []map[string]known.Outcome
	for _, sr := range runs {
		for _, rep := range sr.reps {
			if rep.Warmup || rep.Status != report.RepOK {
				continue
			}
			m := map[string]known.Outcome{}
			for _, c := range rep.Checks {
				m[c.ID] = known.Outcome{Status: c.Status, Known: c.Known, Evidence: c.Evidence}
			}
			out = append(out, m)
		}
	}
	return out
}

// checkSets refuses a --set no scenario of the run declares.
func checkSets(scs []*scenario.Scenario, sets map[string]string) error {
	for k := range sets {
		found := false
		for _, sc := range scs {
			for _, p := range sc.AllParams() {
				found = found || p.Name == k
			}
		}
		if !found {
			return fmt.Errorf("--set %s: no scenario of the run has that param (see streamlab explain <scenario>)", k)
		}
	}
	return nil
}

// setsFor are the --set overrides sc declares.
func setsFor(sc *scenario.Scenario, sets map[string]string) map[string]string {
	out := map[string]string{}
	for _, p := range sc.AllParams() {
		if v, ok := sets[p.Name]; ok {
			out[p.Name] = v
		}
	}
	return out
}

// runRep runs repetition i of sc in a child process and reads its result.
func runRep(ctx context.Context, exe string, sc *scenario.Scenario, i int, warmup bool, sets map[string]string,
	opts Options, scDir string, timeout time.Duration) report.Rep {
	name := "rep-" + strconv.Itoa(i)
	if warmup {
		name = "warmup-" + strconv.Itoa(1-i)
	}
	repDir := filepath.Join(scDir, name)
	failed := func(status report.RepStatus, format string, args ...any) report.Rep {
		return report.Rep{Rep: i, Warmup: warmup, Status: status, Error: fmt.Sprintf(format, args...),
			Artifacts: map[string]string{"log": filepath.Join(name, "child.log")}}
	}
	if err := os.MkdirAll(repDir, 0o755); err != nil {
		return failed(report.RepError, "creating %s: %v", repDir, err)
	}
	args := []string{ChildCommand, "--scenario", sc.Name, "--rep", strconv.Itoa(i), "--out", repDir}
	if opts.ConfigPath != "" {
		args = append(args, "--config", opts.ConfigPath)
	}
	keys := make([]string, 0, len(sets))
	for k := range sets {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		args = append(args, "--set", k+"="+sets[k])
	}
	logPath := filepath.Join(repDir, "child.log")
	logFile, err := os.Create(logPath)
	if err != nil {
		return failed(report.RepError, "creating %s: %v", logPath, err)
	}
	defer logFile.Close()

	// The child's temp files (the stack's folder) go in a folder of its
	// own, removed whatever becomes of the child.
	tmp, err := os.MkdirTemp("", "streamlab-rep-*")
	if err != nil {
		return failed(report.RepError, "creating a temp folder: %v", err)
	}
	defer removeAll(tmp)

	cmd := exec.Command(exe, args...)
	cmd.Stdout, cmd.Stderr = logFile, logFile
	cmd.Env = append(os.Environ(), "GOTRACEBACK=all", "TMPDIR="+tmp, ParentPipeEnv+"=1")
	if opts.GOMAXPROCS > 0 {
		cmd.Env = append(cmd.Env, "GOMAXPROCS="+strconv.Itoa(opts.GOMAXPROCS))
	}
	// The child's stdin stays open as long as this process lives: when it
	// dies, the child sees EOF and quits with a goroutine dump.
	if _, err := cmd.StdinPipe(); err != nil {
		return failed(report.RepError, "the child's stdin: %v", err)
	}
	start := time.Now()
	if err := cmd.Start(); err != nil {
		return failed(report.RepError, "starting %s: %v", exe, err)
	}
	waited := make(chan error, 1)
	go func() { waited <- cmd.Wait() }()

	var waitErr error
	timedOut := false
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case waitErr = <-waited:
	case <-ctx.Done():
		cmd.Process.Kill()
		<-waited
		return failed(report.RepError, "interrupted")
	case <-timer.C:
		timedOut = true
		quit(cmd.Process)
		select {
		case waitErr = <-waited:
		case <-time.After(killGrace):
			cmd.Process.Kill()
			waitErr = <-waited
		}
	}
	wall := time.Since(start)
	logFile.Sync()

	if timedOut {
		r := failed(report.RepTimeout, "no result after the %v hard timeout: SIGQUIT sent, goroutines dumped", timeout)
		if dump := extractDump(logPath); dump != "" {
			dumpPath := filepath.Join(repDir, "goroutines.txt")
			if os.WriteFile(dumpPath, []byte(dump), 0o644) == nil {
				r.Artifacts["goroutines"] = filepath.Join(name, "goroutines.txt")
			}
		}
		r.WallMS = report.MS(wall)
		return r
	}
	rep, readErr := report.ReadRep(filepath.Join(repDir, "rep.json"))
	code := exitCode(waitErr)
	switch {
	case readErr == nil && code == ChildOK:
		rep.Warmup = warmup
		arts := map[string]string{"log": filepath.Join(name, "child.log"), "rep": filepath.Join(name, "rep.json")}
		for k, v := range rep.Artifacts {
			arts[k] = filepath.Join(name, v)
		}
		rep.Artifacts = arts
		return *rep
	case readErr == nil && code == ChildToolError:
		rep.Warmup, rep.Status = warmup, report.RepError
		rep.Artifacts = map[string]string{"log": filepath.Join(name, "child.log")}
		return *rep
	case code == ChildToolError:
		return failed(report.RepError, "tool error; see %s", filepath.Join(name, "child.log"))
	}
	r := failed(report.RepCrash, "the process exited %d without a result (a panic?); see %s: %s",
		code, filepath.Join(name, "child.log"), lastLines(logPath, 3))
	r.WallMS = report.MS(wall)
	return r
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode()
	}
	return -1
}

// extractDump is the goroutine dump at the end of a log: from the
// runtime's "SIGQUIT: quit" on.
func extractDump(logPath string) string {
	b, err := os.ReadFile(logPath)
	if err != nil {
		return ""
	}
	s := string(b)
	if i := strings.LastIndex(s, "SIGQUIT: quit"); i >= 0 {
		return s[i:]
	}
	return ""
}

func lastLines(path string, n int) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, " / ")
}

func progress(w io.Writer, name string, rep report.Rep) {
	if w == nil {
		return
	}
	label := fmt.Sprintf("rep %d", rep.Rep)
	switch {
	case rep.Warmup:
		label = "warm-up"
	case rep.Confirm:
		label = fmt.Sprintf("confirm %d", rep.Rep)
	}
	var bad []string
	for _, c := range rep.Checks {
		if c.Status == report.Fail {
			s := c.ID
			if c.Known != "" {
				s += "(" + c.Known + ")"
			}
			bad = append(bad, s)
		}
	}
	line := fmt.Sprintf("%-18s %-9s %-7s %5.1f s", name, label, rep.Status, rep.WallMS/1000)
	if len(bad) > 0 {
		line += "  failing: " + strings.Join(bad, ", ")
	}
	if rep.Error != "" {
		line += "  " + rep.Error
	}
	fmt.Fprintln(w, line)
}

// build judges a scenario's repetitions into its report.
func build(sr scenarioRun, tool report.Tool, env report.Env, kenv known.Env, configPath string) *report.Report {
	sc, p, reps, run := sr.sc, sr.p, sr.reps, sr.run
	r := &report.Report{
		Schema: report.Schema, Tool: tool, Env: env, Run: run,
		Scenario: report.Scenario{Name: sc.Name, Tier: string(sc.Tier), Doc: sc.Doc, Guards: sc.Guards,
			Params: p.Values(), Seed: uint64(p.Int("seed"))},
		Config:   report.Config{Path: configPath},
		Verdict:  report.VerdictPass,
		Metrics:  map[string]report.Metric{},
		Reps:     reps,
		Findings: []string{},
	}
	var measured []report.Rep
	childEnv := false
	for _, rep := range reps {
		for _, a := range rep.Artifacts {
			r.Artifacts = append(r.Artifacts, a)
		}
		if rep.Warmup {
			// A warm-up's numbers are dropped, its correctness isn't: one
			// that hung or crashed fails the scenario.
			if rep.Status == report.RepTimeout || rep.Status == report.RepCrash {
				r.Verdict = report.VerdictFail
			}
			continue
		}
		measured = append(measured, rep)
		if rep.Config.Fingerprint != "" {
			r.Config.Fingerprint = rep.Config.Fingerprint
		}
		// The children ran the scenario: their build and machine.
		if rep.Env != nil && !childEnv {
			r.Env, childEnv = *rep.Env, true
		}
		if rep.Tool != nil && r.Tool == tool {
			r.Tool = *rep.Tool
		}
		switch rep.Status {
		case report.RepTimeout, report.RepCrash:
			r.Verdict = report.VerdictFail
		case report.RepError:
			if r.Verdict == report.VerdictPass {
				r.Verdict = report.VerdictError
			}
		}
	}
	slices.Sort(r.Artifacts)

	// Metrics over the measured repetitions that ran.
	values := map[string][]float64{}
	for _, rep := range measured {
		for name, v := range rep.Metrics {
			values[name] = append(values[name], v)
		}
	}
	for name, vs := range values {
		if def, ok := scenario.LookupMetric(name); ok {
			r.Metrics[name] = report.NewMetric(def, vs)
		}
	}

	// Checks: each one judged over the repetitions that ran it.
	for _, id := range sc.AllChecks() {
		var outcomes []known.Outcome
		var per []string
		evidence := ""
		for _, rep := range measured {
			o := known.Outcome{Status: report.NA, Evidence: "the repetition " + string(rep.Status)}
			for _, c := range rep.Checks {
				if c.ID == id {
					o = known.Outcome{Status: c.Status, Known: c.Known, Evidence: c.Evidence}
				}
			}
			if rep.Status != report.RepOK {
				o = known.Outcome{Status: report.NA, Evidence: rep.Error}
			}
			outcomes = append(outcomes, o)
			s := string(o.Status)
			if o.Status == report.Fail && o.Known != "" {
				s += ":" + o.Known
			}
			per = append(per, s)
			if evidence == "" || o.Status == report.Fail {
				evidence = o.Evidence
			}
		}
		v := known.Judge(id, outcomes, kenv)
		if v.Note != "" {
			evidence = v.Note + ". " + evidence
		}
		r.Checks = append(r.Checks, report.Check{ID: id, Status: v.Status, Known: v.Known, Evidence: evidence, Reps: per})
		if v.Status == report.Fail {
			r.Verdict = report.VerdictFail
		}
	}
	return r
}

// Dir is streamlab's folder: .streamlab at the root of the repository
// holding the working directory (it's gitignored), or in the working
// directory outside one.
func Dir() string {
	wd, err := os.Getwd()
	if err != nil {
		return ".streamlab"
	}
	for d := wd; ; d = filepath.Dir(d) {
		if _, err := os.Stat(filepath.Join(d, ".git")); err == nil {
			return filepath.Join(d, ".streamlab")
		}
		if filepath.Dir(d) == d {
			return filepath.Join(wd, ".streamlab")
		}
	}
}
