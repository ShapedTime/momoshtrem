// Command streamlab profiles, debugs and guards momoshtrem's streaming
// path (docs/streamlab/PLAN.md): it runs Infuse-like scenarios against
// the real stack in-process, each repetition in a process of its own, and
// writes JSON and Markdown reports.
//
//	streamlab run [--tier gate] [--scenario a,b] [--reps N] [--warmup N]
//	              [--config prod.yaml] [--set k=v]... [--out dir]
//	              [--gomaxprocs N] [--timeout d] [--diagnose]
//	streamlab check [--scenario a,b] [--reps N] [--warmup N] [--set k=v]...
//	                [--out dir] [--timeout d] [--exe race-binary] [--tests=false]
//	streamlab list
//	streamlab explain <scenario|metric|check|known issue>
//	streamlab compare [--force] [--json] [--out dir] <base> <head>
//	streamlab ab --base rev [--head rev] [--reps N] [--scenario a,b | --tier t]
//	             [--race] [--confirm=false] [--set k=v]... [--keep]
//	streamlab calibrate [--rev main] [--reps N] [--scenario a,b]
//	streamlab gate [--scenario a,b] [--reps N] [--baseline path]
//	streamlab baseline update --reason "why" [--from run]
//	streamlab affected [--base main] [--json] [--strict] [files...]
//
// Exit codes: 0 pass, 1 fail, 3 tool or environment error; compare and ab
// also 2 for inconclusive (S8, compare.ExitCode).
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/shapedtime/momoshtrem/internal/streamlab/known"
	"github.com/shapedtime/momoshtrem/internal/streamlab/report"
	"github.com/shapedtime/momoshtrem/internal/streamlab/runner"
	"github.com/shapedtime/momoshtrem/internal/streamlab/scenario"
)

// Exit codes.
const (
	exitPass = 0
	exitFail = 1
	// exitInconclusive is compare's and ab's (S8, #179).
	exitInconclusive = 2
	exitTool         = 3
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		usage(stderr)
		return exitTool
	}
	switch args[0] {
	case "run":
		return runCmd(args[1:], stdout, stderr)
	case "list":
		return listCmd(stdout)
	case "explain":
		return explainCmd(args[1:], stdout, stderr)
	case "check":
		return checkCmd(args[1:], stdout, stderr)
	case runner.ChildCommand:
		return runner.RunChild(args[1:], stderr)
	case "compare":
		return compareCmd(args[1:], stdout, stderr)
	case "ab":
		return abCmd(args[1:], stdout, stderr)
	case "calibrate":
		return calibrateCmd(args[1:], stdout, stderr)
	case "gate":
		return gateCmd(args[1:], stdout, stderr)
	case "baseline":
		return baselineCmd(args[1:], stdout, stderr)
	case "affected":
		return affectedCmd(args[1:], stdout, stderr)
	case "help", "-h", "--help":
		usage(stdout)
		return exitPass
	}
	fmt.Fprintf(stderr, "streamlab: unknown command %q\n\n", args[0])
	usage(stderr)
	return exitTool
}

func usage(w io.Writer) {
	fmt.Fprint(w, `streamlab runs Infuse-like scenarios against momoshtrem's streaming stack.

Commands:
  run      run scenarios and write report.json and report.md for each
  check    build with -race, run the stress tier, report races, hangs and go test -race
  list     list the scenarios
  explain  explain a scenario, a metric, a check or a known issue
  compare  judge two runs: pass, regression, improvement, inconclusive or fail
  ab       did this change make streaming worse? an interleaved A/B against a base
  calibrate  measure this machine's noise for compare (.streamlab/noise.json)
  gate     run the gate tier and hold it to testdata/baseline.json
  baseline update --reason "why": the only way to change the baseline
  affected which scenarios cover this diff

Run from momoshtrem/: go run -tags disable_libutp ./cmd/streamlab run --tier gate
                      go run -tags disable_libutp ./cmd/streamlab check
                      go run -tags disable_libutp ./cmd/streamlab ab --base main
Exit codes: 0 pass, 1 fail, 3 tool or environment error; compare and ab:
0 pass or improvement, 1 fail or regression, 2 inconclusive, 3 tool error.
`)
}

func runCmd(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(stderr)
	tier := fs.String("tier", "", "run the tier's scenarios (gate); the default when no --scenario is given")
	names := fs.String("scenario", "", "comma-separated scenarios to run")
	reps := fs.Int("reps", 1, "measured repetitions per scenario")
	warmup := fs.Int("warmup", 1, "repetitions run first and discarded")
	configPath := fs.String("config", "", "a YAML config laid over the defaults (prod.yaml, say)")
	out := fs.String("out", "", "the run's folder (default .streamlab/runs/<id> at the repo root)")
	gomaxprocs := fs.Int("gomaxprocs", 0, "pin the repetitions' GOMAXPROCS (0: the default)")
	timeout := fs.Duration("timeout", 0, "each repetition's hard timeout, positive (0: the scenario's)")
	diagnose := fs.Bool("diagnose", false, "observe in depth: DEBUG logs of the prioritizer, priority readers and activity manager, "+
		"full-rate mutex and block profiles, the flight recorder all along (timings aren't faithful then)")
	sets := runner.Sets{}
	fs.Var(sets, "set", "override a scenario param, k=v (repeatable)")
	if err := fs.Parse(args); err != nil {
		return exitTool
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(stderr, "run: unexpected arguments %q\n", fs.Args())
		return exitTool
	}
	scs, err := pick(*tier, *names)
	if err != nil {
		fmt.Fprintf(stderr, "run: %v\n", err)
		return exitTool
	}
	if *configPath != "" {
		abs, err := filepath.Abs(*configPath)
		if err != nil {
			fmt.Fprintf(stderr, "run: %v\n", err)
			return exitTool
		}
		*configPath = abs
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	start := time.Now()
	res, err := runner.Run(ctx, runner.Options{
		Scenarios:  scs,
		Reps:       *reps,
		Warmup:     *warmup,
		Sets:       sets,
		ConfigPath: *configPath,
		Out:        *out,
		GOMAXPROCS: *gomaxprocs,
		Timeout:    *timeout,
		Command:    append([]string{"streamlab", "run"}, args...),
		Progress:   stdout,
		Diagnose:   *diagnose,
	})
	if err != nil {
		fmt.Fprintf(stderr, "run: %v\n", err)
		return exitTool
	}
	fmt.Fprintf(stdout, "\n%s in %s: %s\n", strings.ToUpper(string(res.Index.Verdict)), time.Since(start).Round(time.Second),
		filepath.Join(res.Dir, "index.md"))
	// The exit code follows the verdict printed above.
	switch res.Index.Verdict {
	case report.VerdictFail:
		return exitFail
	case report.VerdictError:
		return exitTool
	}
	return exitPass
}

// pick is the scenarios a run asks for: by name, else the tier's (gate by
// default).
func pick(tier, names string) ([]*scenario.Scenario, error) {
	if names != "" {
		if tier != "" {
			return nil, errors.New("give --tier or --scenario, not both")
		}
		var out []*scenario.Scenario
		for _, n := range strings.Split(names, ",") {
			sc, ok := scenario.Lookup(strings.TrimSpace(n))
			if !ok {
				return nil, fmt.Errorf("unknown scenario %q (streamlab list)", n)
			}
			out = append(out, sc)
		}
		return out, nil
	}
	if tier == "" {
		tier = string(scenario.Gate)
	}
	scs := scenario.InTier(scenario.Tier(tier))
	if len(scs) == 0 {
		return nil, fmt.Errorf("no scenario in tier %q", tier)
	}
	return scs, nil
}

func listCmd(stdout io.Writer) int {
	tw := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "SCENARIO\tTIER\tGUARDS\tWHAT IT DOES")
	for _, sc := range scenario.All() {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", sc.Name, sc.Tier, strings.Join(sc.Guards, ","), firstSentence(sc.Doc))
	}
	tw.Flush()
	return exitPass
}

func explainCmd(args []string, stdout, stderr io.Writer) int {
	if len(args) != 1 {
		fmt.Fprintln(stderr, "usage: streamlab explain <scenario|metric|check|known issue>")
		return exitTool
	}
	name := args[0]
	found := false
	if sc, ok := scenario.Lookup(name); ok {
		found = true
		explainScenario(stdout, sc)
	}
	if m, ok := scenario.LookupMetric(name); ok {
		found = true
		fmt.Fprintf(stdout, "metric %s: %s, in %s, %s is better\n\n%s\n", m.Name, m.Kind, m.Unit, m.Better, wrap(m.Doc))
		fmt.Fprintln(stdout, "\n"+kindNote(string(m.Kind)))
	}
	if c, ok := scenario.LookupCheck(name); ok {
		found = true
		fmt.Fprintf(stdout, "check %s\n\n%s\n", c.ID, wrap(c.Doc))
		for _, is := range known.ForCheck(c.ID) {
			fmt.Fprintf(stdout, "\nKnown issue %s (%s) can explain its failures: %s\n", is.ID, is.Ref, is.Title)
		}
	}
	if is, ok := known.Lookup(name); ok {
		found = true
		rate := fmt.Sprintf("intermittent: an xfail on at most %.0f%% of reps and never all; a firing in a scenario with under %d reps "+
			"gets %d confirmation reps, and a second firing over the rate fails", is.Rate()*100, known.ConfirmBelow, known.ConfirmReps)
		if !is.Intermittent() {
			rate = "deterministic: fires on every rep while it exists"
		}
		fmt.Fprintf(stdout, "known issue %s (%s): %s\n\n%s\n\nChecks: %s. %s.\n", is.ID, is.Ref, is.Title, wrap(is.Doc),
			strings.Join(is.Checks, ", "), rate)
	}
	if !found {
		fmt.Fprintf(stderr, "explain: %q is no scenario, metric, check or known issue (streamlab list)\n", name)
		return exitTool
	}
	return exitPass
}

func explainScenario(w io.Writer, sc *scenario.Scenario) {
	fmt.Fprintf(w, "scenario %s (tier %s)\n\n", sc.Name, sc.Tier)
	for _, para := range strings.Split(sc.Doc, "\n\n") {
		fmt.Fprintf(w, "%s\n\n", wrap(para))
	}
	fmt.Fprintf(w, "Guards: %s\n", strings.Join(sc.Guards, ", "))
	fmt.Fprintf(w, "Checks: %s\n", strings.Join(sc.AllChecks(), ", "))
	if len(sc.Key) > 0 {
		fmt.Fprintf(w, "Key metrics: %s\n", strings.Join(sc.Key, ", "))
	}
	fmt.Fprintf(w, "Hard timeout: %v\n\nParams (--set k=v):\n", sc.HardTimeout())
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	own := map[string]bool{}
	for _, p := range sc.Params {
		own[p.Name] = true
	}
	for _, p := range sc.AllParams() {
		mark := ""
		if own[p.Name] {
			mark = "*"
		}
		fmt.Fprintf(tw, "  %s%s\t%s\t%s\t%s\n", p.Name, mark, p.Kind, p.Default, p.Doc)
	}
	tw.Flush()
	fmt.Fprintln(w, "\n* the scenario's own, or a common param with its own default")
}

func kindNote(kind string) string {
	switch kind {
	case "exact":
		return "Exact metrics are deterministic: any change is reported."
	case "count":
		return "Count metrics are flagged past a noise floor (compare, S8)."
	}
	return "Timing metrics depend on the machine: they get a verdict only from an interleaved A/B on one machine (PLAN D5)."
}

// firstSentence is a doc's first sentence, cut at a word boundary to
// about 100 characters, and before a parenthesis it would cut through.
func firstSentence(doc string) string {
	p, _, _ := strings.Cut(strings.TrimSpace(doc), "\n\n")
	p = strings.Join(strings.Fields(p), " ")
	if i := strings.Index(p, ". "); i >= 0 {
		p = p[:i+1]
	}
	const limit = 100
	if len(p) <= limit {
		return p
	}
	cut := strings.LastIndex(p[:limit], " ")
	if cut <= 0 {
		cut = limit
	}
	if strings.Count(p[:cut], "(") > strings.Count(p[:cut], ")") {
		cut = strings.LastIndex(p[:cut], " (")
	}
	return strings.TrimRight(p[:cut], ",;: ") + " …"
}

// wrap folds text at 100 columns.
func wrap(s string) string {
	var b strings.Builder
	col := 0
	for _, w := range strings.Fields(s) {
		if col > 0 && col+1+len(w) > 100 {
			b.WriteByte('\n')
			col = 0
		} else if col > 0 {
			b.WriteByte(' ')
			col++
		}
		b.WriteString(w)
		col += len(w)
	}
	return b.String()
}
