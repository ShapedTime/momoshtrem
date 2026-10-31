package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/shapedtime/momoshtrem/internal/streamlab/ab"
	"github.com/shapedtime/momoshtrem/internal/streamlab/compare"
	"github.com/shapedtime/momoshtrem/internal/streamlab/runner"
	"github.com/shapedtime/momoshtrem/internal/streamlab/scenario"
)

// compareCmd is streamlab compare base head (PLAN §6 "Compare").
func compareCmd(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("compare", flag.ContinueOnError)
	fs.SetOutput(stderr)
	force := fs.Bool("force", false, "compare runs from different machines or params: checks and exact metrics only")
	out := fs.String("out", "", "also write compare.json and compare.md to this folder")
	asJSON := fs.Bool("json", false, "print compare.json instead of the Markdown")
	pos, err := parseInterleaved(fs, args)
	if err != nil {
		return parseExit(err)
	}
	var res *compare.Result
	switch len(pos) {
	case 1:
		// An ab's folder, judged again (its confirmation included): after
		// a calibrate, say.
		res, err = ab.Rejudge(pos[0], *force)
	case 2:
		res, err = comparePaths(pos[0], pos[1], *force)
	default:
		fmt.Fprintln(stderr, "usage: streamlab compare [--force] [--json] [--out dir] <base> <head>\n"+
			"  each a run's folder, its index.json, or a scenario's report.json\n"+
			"       streamlab compare [--json] [--out dir] <ab folder>\n"+
			"  an ab's folder judged again, its confirmation included")
		return exitTool
	}
	if err != nil {
		fmt.Fprintf(stderr, "compare: %v\n", err)
		return exitTool
	}
	if *out != "" {
		if err := os.MkdirAll(*out, 0o755); err != nil {
			fmt.Fprintf(stderr, "compare: %v\n", err)
			return exitTool
		}
		if err := res.Write(*out); err != nil {
			fmt.Fprintf(stderr, "compare: %v\n", err)
			return exitTool
		}
	}
	if *asJSON {
		b, _ := json.MarshalIndent(res, "", "  ")
		fmt.Fprintln(stdout, string(b))
	} else {
		fmt.Fprint(stdout, res.Markdown())
	}
	return compareExit(res)
}

func comparePaths(basePath, headPath string, force bool) (*compare.Result, error) {
	base, err := compare.Load(basePath)
	if err != nil {
		return nil, fmt.Errorf("base: %w", err)
	}
	head, err := compare.Load(headPath)
	if err != nil {
		return nil, fmt.Errorf("head: %w", err)
	}
	opts, err := ab.NoiseFor("", head.Reports[0].Env)
	if err != nil {
		return nil, err
	}
	opts.Force = force
	res, err := compare.Compare(base, head, opts)
	if err != nil {
		return nil, err
	}
	// Two runs and no fresh reps: a screen (compare.Unconfirmed).
	return compare.Unconfirmed(res), nil
}

// compareExit is a comparison's exit code: a fail or a regression wins
// (1), then a side that couldn't run a scenario (3), then the verdict's.
func compareExit(res *compare.Result) int {
	if code := res.Verdict.ExitCode(); code == exitFail || len(res.Errors) == 0 {
		return code
	}
	return exitTool
}

// parseExit is a flag error's exit code: 0 for -h, 3 for a bad flag.
func parseExit(err error) int {
	if errors.Is(err, flag.ErrHelp) {
		return exitPass
	}
	return exitTool
}

// abCmd is streamlab ab (PLAN §6 "ab").
func abCmd(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("ab", flag.ContinueOnError)
	fs.SetOutput(stderr)
	base := fs.String("base", "", "the base revision, required: main, a branch, a commit")
	head := fs.String("head", "", "the head revision (default: the working tree, uncommitted changes included)")
	tier := fs.String("tier", "", "run the tier's scenarios (gate; stress with --race); the default when no --scenario is given")
	names := fs.String("scenario", "", "comma-separated scenarios to run")
	reps := fs.Int("reps", compare.MinTimingReps, "measured repetitions a side per scenario (timings need 5)")
	warmup := fs.Int("warmup", 1, "repetitions a side run first and discarded")
	configPath := fs.String("config", "", "a YAML config laid over the defaults, on both sides")
	race := fs.Bool("race", false, "build both sides with -race and add the race check (the stress tier by default); timings get no verdict")
	confirm := fs.Bool("confirm", true, "run the scenarios whose verdict rests on statistics again, and keep only what holds")
	force := fs.Bool("force", false, "compare even across machines (never needed for one ab)")
	keep := fs.Bool("keep", false, "keep the worktrees and binaries")
	out := fs.String("out", "", "the A/B's folder (default .streamlab/runs/<id>-ab at the repo root)")
	gomaxprocs := fs.Int("gomaxprocs", 0, "pin the repetitions' GOMAXPROCS (0: the default)")
	timeout := fs.Duration("timeout", 0, "each repetition's hard timeout, positive (0: the scenario's)")
	sets := runner.Sets{}
	fs.Var(sets, "set", "override a scenario param on both sides, k=v (repeatable)")
	if err := fs.Parse(args); err != nil {
		return parseExit(err)
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(stderr, "ab: unexpected arguments %q\n", fs.Args())
		return exitTool
	}
	if *base == "" {
		fmt.Fprintln(stderr, "ab: --base is required (streamlab ab --base main)")
		return exitTool
	}
	if *race && *tier == "" && *names == "" {
		*tier = string(scenario.Stress)
	}
	scs, err := pick(*tier, *names)
	if err != nil {
		fmt.Fprintf(stderr, "ab: %v\n", err)
		return exitTool
	}
	if *reps < 1 || *warmup < 0 {
		fmt.Fprintln(stderr, "ab: --reps must be at least 1 and --warmup at least 0")
		return exitTool
	}
	if err := absPaths(configPath, out); err != nil {
		fmt.Fprintf(stderr, "ab: %v\n", err)
		return exitTool
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	start := time.Now()
	res, err := ab.Run(ctx, ab.Options{Base: *base, Head: *head, Scenarios: scs, Reps: *reps, Warmup: *warmup, Sets: sets,
		ConfigPath: *configPath, GOMAXPROCS: *gomaxprocs, Timeout: *timeout, Race: *race, Confirm: *confirm, Force: *force,
		Keep: *keep, Out: *out, Command: append([]string{"streamlab", "ab"}, args...), Progress: stdout})
	if err != nil {
		fmt.Fprintf(stderr, "ab: %v\n", err)
		return exitTool
	}
	fmt.Fprintf(stdout, "\n%s\n%s in %s: %s\n", res.Compare.Line(), strings.ToUpper(string(res.Compare.Verdict)),
		time.Since(start).Round(time.Second), filepath.Join(res.Dir, "compare.md"))
	return compareExit(res.Compare)
}

// calibrateCmd is streamlab calibrate: this machine's noise, for
// compare's count thresholds.
func calibrateCmd(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("calibrate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	rev := fs.String("rev", "main", "the revision whose noise to measure, with the working tree's lab")
	tier := fs.String("tier", "", "calibrate the tier's scenarios (gate); the default when no --scenario is given")
	names := fs.String("scenario", "", "comma-separated scenarios to calibrate")
	reps := fs.Int("reps", 10, "measured repetitions per scenario")
	warmup := fs.Int("warmup", 1, "repetitions run first and discarded")
	configPath := fs.String("config", "", "a YAML config laid over the defaults")
	keep := fs.Bool("keep", false, "keep the worktree and binary")
	out := fs.String("out", "", "the run's folder (default .streamlab/runs/<id>-calibrate at the repo root)")
	gomaxprocs := fs.Int("gomaxprocs", 0, "pin the repetitions' GOMAXPROCS (0: the default)")
	timeout := fs.Duration("timeout", 0, "each repetition's hard timeout, positive (0: the scenario's)")
	sets := runner.Sets{}
	fs.Var(sets, "set", "override a scenario param, k=v (repeatable)")
	if err := fs.Parse(args); err != nil {
		return parseExit(err)
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(stderr, "calibrate: unexpected arguments %q\n", fs.Args())
		return exitTool
	}
	scs, err := pick(*tier, *names)
	if err != nil {
		fmt.Fprintf(stderr, "calibrate: %v\n", err)
		return exitTool
	}
	if *reps < 2 {
		fmt.Fprintln(stderr, "calibrate: --reps must be at least 2")
		return exitTool
	}
	if err := absPaths(configPath, out); err != nil {
		fmt.Fprintf(stderr, "calibrate: %v\n", err)
		return exitTool
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	res, err := ab.Calibrate(ctx, ab.CalibrateOptions{Rev: *rev, Scenarios: scs, Reps: *reps, Warmup: *warmup, Sets: sets,
		ConfigPath: *configPath, GOMAXPROCS: *gomaxprocs, Timeout: *timeout, Keep: *keep, Out: *out,
		Command: append([]string{"streamlab", "calibrate"}, args...), Progress: stdout})
	if err != nil {
		fmt.Fprintf(stderr, "calibrate: %v\n", err)
		return exitTool
	}
	if c := res.Calibration; c != nil {
		fmt.Fprintf(stdout, "\ncalibrated %s on %s over %d reps: %s\n", short(c.Commit), c.Machine, c.Reps, res.Path)
	} else {
		fmt.Fprintln(stdout, "\nno scenario passed: nothing calibrated")
	}
	if len(res.Skipped) > 0 {
		fmt.Fprintf(stdout, "skipped, they didn't pass: %s (%s)\n", strings.Join(res.Skipped, ", "), filepath.Join(res.Dir, "index.md"))
	}
	if len(res.Errored) > 0 {
		fmt.Fprintf(stdout, "skipped, the tool couldn't run them: %s (%s)\n", strings.Join(res.Errored, ", "), filepath.Join(res.Dir, "index.md"))
	}
	return calibrateExit(res)
}

// calibrateExit is calibrate's exit code: 3 when the tool couldn't run a
// scenario (its noise is missing, whatever else passed), 1 when one
// failed, else 0.
func calibrateExit(res *ab.CalibrateResult) int {
	switch {
	case len(res.Errored) > 0:
		return exitTool
	case len(res.Skipped) > 0:
		return exitFail
	}
	return exitPass
}

// parseInterleaved parses flags wherever they are among the positional
// arguments, and returns the positional ones.
func parseInterleaved(fs *flag.FlagSet, args []string) ([]string, error) {
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		if fs.NArg() == 0 {
			return pos, nil
		}
		pos = append(pos, fs.Arg(0))
		args = fs.Args()[1:]
	}
}

// absPaths makes each non-empty path absolute: the repetitions run in
// other folders.
func absPaths(paths ...*string) error {
	for _, p := range paths {
		if *p == "" {
			continue
		}
		abs, err := filepath.Abs(*p)
		if err != nil {
			return err
		}
		*p = abs
	}
	return nil
}

func short(sha string) string {
	if len(sha) > 12 {
		return sha[:12]
	}
	return sha
}
