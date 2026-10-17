package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/shapedtime/momoshtrem/internal/streamlab/check"
	"github.com/shapedtime/momoshtrem/internal/streamlab/report"
	"github.com/shapedtime/momoshtrem/internal/streamlab/runner"
	"github.com/shapedtime/momoshtrem/internal/streamlab/scenario"
)

// checkCmd is streamlab check (PLAN D8): a race build, the stress tier,
// race reports as JSON, and go test -race over the streaming path.
func checkCmd(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	fs.SetOutput(stderr)
	names := fs.String("scenario", "", "comma-separated scenarios to run (default: the stress tier)")
	reps := fs.Int("reps", 1, "measured repetitions per scenario")
	warmup := fs.Int("warmup", 0, "repetitions run first and discarded (their races still count)")
	configPath := fs.String("config", "", "a YAML config laid over the defaults")
	out := fs.String("out", "", "the run's folder (default .streamlab/runs/<id> at the repo root)")
	gomaxprocs := fs.Int("gomaxprocs", 0, "pin the repetitions' GOMAXPROCS (0: the default)")
	timeout := fs.Duration("timeout", 0, "each repetition's hard timeout, positive (0: the scenario's)")
	exe := fs.String("exe", "", "a race build of streamlab to use instead of building one")
	tests := fs.Bool("tests", true, "also run go test -race over the streaming path's packages")
	sets := runner.Sets{}
	fs.Var(sets, "set", "override a scenario param, k=v (repeatable)")
	if err := fs.Parse(args); err != nil {
		return exitTool
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(stderr, "check: unexpected arguments %q\n", fs.Args())
		return exitTool
	}
	tier := string(scenario.Stress)
	if *names != "" {
		tier = ""
	}
	scs, err := pick(tier, *names)
	if err != nil {
		fmt.Fprintf(stderr, "check: %v\n", err)
		return exitTool
	}
	for _, p := range []*string{configPath, exe} {
		if *p == "" {
			continue
		}
		abs, err := filepath.Abs(*p)
		if err != nil {
			fmt.Fprintf(stderr, "check: %v\n", err)
			return exitTool
		}
		*p = abs
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	start := time.Now()
	res, err := check.Run(ctx, check.Options{
		Scenarios:  scs,
		Reps:       *reps,
		Warmup:     *warmup,
		Sets:       sets,
		ConfigPath: *configPath,
		Out:        *out,
		GOMAXPROCS: *gomaxprocs,
		Timeout:    *timeout,
		Exe:        *exe,
		Tests:      *tests,
		Command:    append([]string{"streamlab", "check"}, args...),
		Progress:   stdout,
	})
	if err != nil {
		fmt.Fprintf(stderr, "check: %v\n", err)
		return exitTool
	}
	s := res.Summary
	fmt.Fprintf(stdout, "\n%s in %s: %s\n%s\n", strings.ToUpper(string(s.Verdict)), time.Since(start).Round(time.Second),
		filepath.Join(res.Dir, "check.md"), s.Why)
	switch s.Verdict {
	case report.VerdictFail:
		return exitFail
	case report.VerdictError:
		return exitTool
	}
	return exitPass
}
