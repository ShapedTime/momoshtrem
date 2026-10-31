package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/shapedtime/momoshtrem/internal/streamlab/affected"
	"github.com/shapedtime/momoshtrem/internal/streamlab/baseline"
	"github.com/shapedtime/momoshtrem/internal/streamlab/check"
	"github.com/shapedtime/momoshtrem/internal/streamlab/checks"
	"github.com/shapedtime/momoshtrem/internal/streamlab/compare"
	"github.com/shapedtime/momoshtrem/internal/streamlab/report"
	"github.com/shapedtime/momoshtrem/internal/streamlab/runner"
	"github.com/shapedtime/momoshtrem/internal/streamlab/scenario"
)

// gateCmd is streamlab gate: the gate tier held to the baseline's exact
// and correctness expectations (PLAN §6 "Baseline").
func gateCmd(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("gate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	names := fs.String("scenario", "", "comma-separated gate scenarios (default: the whole tier)")
	reps := fs.Int("reps", 1, "measured repetitions per scenario")
	warmup := fs.Int("warmup", 1, "repetitions run first and discarded")
	path := fs.String("baseline", "", "the baseline (default "+baseline.File+" in the module)")
	out := fs.String("out", "", "the run's folder (default .streamlab/runs/<id> at the repo root)")
	gomaxprocs := fs.Int("gomaxprocs", 0, "pin the repetitions' GOMAXPROCS (0: the default)")
	timeout := fs.Duration("timeout", 0, "each repetition's hard timeout, positive (0: the scenario's)")
	if err := fs.Parse(args); err != nil {
		return parseExit(err)
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(stderr, "gate: unexpected arguments %q\n", fs.Args())
		return exitTool
	}
	tier := string(scenario.Gate)
	if *names != "" {
		tier = ""
	}
	scs, err := pick(tier, *names)
	if err != nil {
		fmt.Fprintf(stderr, "gate: %v\n", err)
		return exitTool
	}
	blPath, err := baselinePath(*path)
	if err != nil {
		fmt.Fprintf(stderr, "gate: %v\n", err)
		return exitTool
	}
	bl, err := baseline.Read(blPath)
	if err != nil {
		fmt.Fprintf(stderr, "gate: %v\n", err)
		return exitTool
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	start := time.Now()
	res, err := runner.Run(ctx, runner.Options{Scenarios: scs, Reps: *reps, Warmup: *warmup, Out: *out, GOMAXPROCS: *gomaxprocs,
		Timeout: *timeout, Command: append([]string{"streamlab", "gate"}, args...), Progress: stdout})
	if err != nil {
		fmt.Fprintf(stderr, "gate: %v\n", err)
		return exitTool
	}
	g := baseline.Hold(bl, blPath, res.Reports, checks.MIME().ResolvesMKV())
	if err := g.Write(res.Dir); err != nil {
		fmt.Fprintf(stderr, "gate: %v\n", err)
		return exitTool
	}
	fmt.Fprintf(stdout, "\n%s: %s\n%s in %s: %s\n", strings.ToUpper(string(g.Verdict)), g.Why, strings.ToUpper(string(g.Verdict)),
		time.Since(start).Round(time.Second), filepath.Join(res.Dir, "gate.md"))
	switch g.Verdict {
	case report.VerdictFail:
		return exitFail
	case report.VerdictError:
		return exitTool
	}
	return exitPass
}

// baselineCmd is streamlab baseline update --reason: the only way to
// change the baseline.
func baselineCmd(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] != "update" {
		fmt.Fprintln(stderr, `usage: streamlab baseline update --reason "why" [--from run-folder] [--reps N] [--scenario a,b] [--baseline path]`)
		return exitTool
	}
	flags := flag.NewFlagSet("baseline update", flag.ContinueOnError)
	flags.SetOutput(stderr)
	reason := flags.String("reason", "", "why the expectations change, required: recorded in the file with the commit and date")
	from := flags.String("from", "", "take a gate run's reports (its folder) instead of running the gate")
	names := flags.String("scenario", "", "update only these gate scenarios (default: the whole tier)")
	reps := flags.Int("reps", 2, "measured repetitions per scenario when running the gate (2 or more spots exact metrics that vary)")
	warmup := flags.Int("warmup", 1, "repetitions run first and discarded")
	path := flags.String("baseline", "", "the baseline (default "+baseline.File+" in the module)")
	if err := flags.Parse(args[1:]); err != nil {
		return parseExit(err)
	}
	if flags.NArg() > 0 {
		fmt.Fprintf(stderr, "baseline update: unexpected arguments %q\n", flags.Args())
		return exitTool
	}
	if strings.TrimSpace(*reason) == "" {
		fmt.Fprintln(stderr, `baseline update: --reason "…" is required: it's recorded in the file, with the commit and date`)
		return exitTool
	}
	blPath, err := baselinePath(*path)
	if err != nil {
		fmt.Fprintf(stderr, "baseline update: %v\n", err)
		return exitTool
	}
	prev, err := baseline.Read(blPath)
	edited := errors.Is(err, baseline.ErrEdited)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		prev = nil
	case edited:
		fmt.Fprintf(stdout, "note: %v; this update must re-record every scenario in it\n", err)
	case err != nil:
		fmt.Fprintf(stderr, "baseline update: %v\n", err)
		return exitTool
	}
	// An edit by hand isn't put under a fresh digest by updating other
	// scenarios: every scenario the file holds is recorded again.
	refuseEdited := func(reports []*report.Report) bool {
		if !edited {
			return false
		}
		if missing := prev.Missing(reports); len(missing) > 0 {
			fmt.Fprintf(stderr, "baseline update: %s was edited by hand, and this update doesn't re-record %s: run it over every scenario\n",
				blPath, strings.Join(missing, ", "))
			return true
		}
		return false
	}

	var reports []*report.Report
	dir := ""
	if *from != "" {
		side, err := compare.Load(*from)
		if err != nil {
			fmt.Fprintf(stderr, "baseline update: %v\n", err)
			return exitTool
		}
		reports, dir = side.Reports, *from
		for _, r := range reports {
			if r.Scenario.Tier != string(scenario.Gate) {
				fmt.Fprintf(stderr, "baseline update: %s isn't a gate scenario\n", r.Scenario.Name)
				return exitTool
			}
		}
		if refuseEdited(reports) {
			return exitTool
		}
	} else {
		tier := string(scenario.Gate)
		if *names != "" {
			tier = ""
		}
		scs, err := pick(tier, *names)
		if err != nil {
			fmt.Fprintf(stderr, "baseline update: %v\n", err)
			return exitTool
		}
		planned := make([]*report.Report, len(scs))
		for i, sc := range scs {
			planned[i] = &report.Report{Scenario: report.Scenario{Name: sc.Name}}
		}
		if refuseEdited(planned) { // before the run, not after it
			return exitTool
		}
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		res, err := runner.Run(ctx, runner.Options{Scenarios: scs, Reps: *reps, Warmup: *warmup,
			Command: append([]string{"streamlab", "baseline"}, args...), Progress: stdout})
		if err != nil {
			fmt.Fprintf(stderr, "baseline update: %v\n", err)
			return exitTool
		}
		reports, dir = res.Reports, res.Dir
	}
	module, err := check.ModuleDir(".")
	if err != nil {
		fmt.Fprintf(stderr, "baseline update: %v\n", err)
		return exitTool
	}
	tool := report.ToolInfo(module)
	next, err := baseline.FromRun(reports, prev, baseline.Update{Date: time.Now().UTC(), Commit: tool.Git.Commit,
		Dirty: tool.Git.Dirty, Reason: *reason}, checks.MIME().ResolvesMKV())
	if errors.Is(err, baseline.ErrFailing) {
		fmt.Fprintf(stderr, "baseline update: %v: a baseline records a passing gate (%s)\n", err, dir)
		return exitFail
	}
	if errors.Is(err, baseline.ErrXPass) {
		fmt.Fprintf(stderr, "baseline update: %v (%s)\n", err, dir)
		return exitFail
	}
	if err != nil {
		fmt.Fprintf(stderr, "baseline update: %v\n", err)
		return exitTool
	}
	if prev != nil {
		if edited {
			fmt.Fprintln(stdout, "the changes below are against the file as it was edited by hand")
		}
		changes := prev.Diff(next)
		if len(changes) == 0 {
			changes = []string{"no expectation changed"}
		}
		fmt.Fprintf(stdout, "changes:\n  %s\n", strings.Join(changes, "\n  "))
	}
	if err := next.Write(blPath); err != nil {
		fmt.Fprintf(stderr, "baseline update: %v\n", err)
		return exitTool
	}
	fmt.Fprintf(stdout, "wrote %s (%d scenarios): %s\n", blPath, len(next.Scenarios), *reason)
	return exitPass
}

// baselinePath is the baseline: path, or the module's.
func baselinePath(path string) (string, error) {
	if path != "" {
		return filepath.Abs(path)
	}
	module, err := check.ModuleDir(".")
	if err != nil {
		return "", err
	}
	return baseline.Path(module), nil
}

// affectedCmd is streamlab affected: the scenarios covering a diff.
func affectedCmd(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("affected", flag.ContinueOnError)
	fs.SetOutput(stderr)
	base := fs.String("base", "main", "the diff's base: git diff <base>... and the working tree")
	asJSON := fs.Bool("json", false, "print JSON")
	strict := fs.Bool("strict", false, "exit 1 on a warning")
	files, err := parseInterleaved(fs, args)
	if err != nil {
		return parseExit(err)
	}
	module, err := check.ModuleDir(".")
	if err != nil {
		fmt.Fprintf(stderr, "affected: %v\n", err)
		return exitTool
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	res, err := affected.Run(ctx, affected.GitCLI{Dir: module}, *base, files)
	if err != nil {
		fmt.Fprintf(stderr, "affected: %v\n", err)
		return exitTool
	}
	if *asJSON {
		b, _ := json.MarshalIndent(res, "", "  ")
		fmt.Fprintln(stdout, string(b))
	} else {
		printAffected(stdout, res)
	}
	for _, w := range res.Warnings {
		fmt.Fprintf(stderr, "WARNING: %s\n", w)
	}
	if *strict && len(res.Warnings) > 0 {
		return exitFail
	}
	return exitPass
}

func printAffected(w io.Writer, res *affected.Result) {
	fmt.Fprintf(w, "%d files changed against %s", len(res.Files), res.Base)
	if res.MergeBase != "" {
		fmt.Fprintf(w, " (merge base %s)", short(res.MergeBase))
	}
	fmt.Fprintln(w)
	if len(res.Scenarios) == 0 {
		fmt.Fprintln(w, "no scenario covers them: nothing on the streaming path changed")
		return
	}
	fmt.Fprintf(w, "\n%d scenarios cover them (* written for what changed):\n", len(res.Scenarios))
	for _, h := range res.Scenarios {
		mark := " "
		if h.Focus {
			mark = "*"
		}
		why := h.Why
		if len(why) > 3 {
			why = append(why[:3:3], fmt.Sprintf("and %d more", len(h.Why)-3))
		}
		fmt.Fprintf(w, " %s %-20s %-6s %s\n", mark, h.Name, h.Tier, strings.Join(why, "; "))
	}
	if len(res.Commands) > 0 {
		fmt.Fprintf(w, "\nRun:\n  %s\n", strings.Join(res.Commands, "\n  "))
	}
}
