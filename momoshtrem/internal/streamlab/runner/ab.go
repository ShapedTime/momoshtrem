package runner

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/shapedtime/momoshtrem/internal/streamlab/checks"
	"github.com/shapedtime/momoshtrem/internal/streamlab/known"
	"github.com/shapedtime/momoshtrem/internal/streamlab/report"
)

// Side is one side of an A/B (streamlab ab, S8 #179): its binary, the
// tree its repetitions run in, and its run's folder.
type Side struct {
	// Name is base or head.
	Name string
	// Exe is the side's streamlab binary.
	Exe string
	// Dir is its repetitions' working directory: the side's module root,
	// so each child records the side's commit.
	Dir string
	// Out is its run's folder: an index and a report per scenario, as a
	// run writes.
	Out string
}

// ABOptions configure an A/B: the run's options (Exe, Dir and Out are
// each side's) and the two sides.
type ABOptions struct {
	Options
	Base, Head Side
	// OnRep is called before each repetition starts, with its side,
	// scenario and number: ab records the machine's load there. Optional.
	OnRep func(side, scenario string, rep int, warmup bool)
}

// ABResult is an A/B's two runs.
type ABResult struct {
	Base, Head *Result
}

// RunAB runs the scenarios on both sides, interleaved: for each
// scenario, the warm-ups, then each measured repetition of one side and
// then the other's, each pair led by the other side than the last (AB BA
// AB …), so a load that rises or falls through the run weighs on both
// alike. Every repetition is a process of its own (PLAN D4). An
// intermittent issue that needs confirming on either side gets its
// confirmation repetitions on both, interleaved too. Each side's run is
// then judged with its own tally and written to its Out, as Run does.
func RunAB(ctx context.Context, opts ABOptions) (*ABResult, error) {
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
	case opts.Base.Exe == "" || opts.Head.Exe == "" || opts.Base.Out == "" || opts.Head.Out == "":
		return nil, errors.New("each side needs a binary and a folder")
	}
	if err := checkSets(opts.Scenarios, opts.Sets); err != nil {
		return nil, err
	}
	sides := []Side{opts.Base, opts.Head}
	type sideRun struct {
		side Side
		opts Options
		res  *Result
		done []scenarioRun
	}
	started := time.Now()
	runID := fmt.Sprintf("%s-%d", started.Format("20060102-150405"), os.Getpid())
	run := report.Run{ID: runID, Reps: opts.Reps, Warmup: opts.Warmup, GOMAXPROCS: opts.GOMAXPROCS,
		Started: started, Command: opts.Command}
	tool := report.ToolInfo(".")
	env := report.CollectEnv()
	env.GOMAXPROCS = cmp.Or(opts.GOMAXPROCS, env.GOMAXPROCS)
	resolvesMKV := checks.MIME().ResolvesMKV()

	runs := make([]*sideRun, len(sides))
	for i, s := range sides {
		out, err := filepath.Abs(s.Out)
		if err != nil {
			return nil, err
		}
		if err := os.MkdirAll(out, 0o755); err != nil {
			return nil, fmt.Errorf("creating %s: %w", out, err)
		}
		o := opts.Options
		o.Exe, o.Dir, o.Out = s.Exe, s.Dir, out
		runs[i] = &sideRun{side: s, opts: o,
			res: &Result{Dir: out, Index: &report.Index{Schema: report.IndexSchema, Tool: tool, Env: env, Verdict: report.VerdictPass}}}
	}

	for _, sc := range opts.Scenarios {
		params, err := sc.Resolve(setsFor(sc, opts.Sets))
		if err != nil {
			return nil, err
		}
		sets := setsFor(sc, opts.Sets)
		timeout := cmp.Or(opts.Timeout, sc.HardTimeout())
		srs := make([]*scenarioRun, len(runs))
		for i, r := range runs {
			dir := filepath.Join(r.opts.Out, sc.Name)
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return nil, err
			}
			srs[i] = &scenarioRun{sc: sc, p: params, dir: dir, run: run}
			srs[i].run.TimeoutS = timeout.Seconds()
		}
		scStart := time.Now()
		pair := 0
		// next runs repetition n on both sides, the pair's leader first.
		next := func(n int, warmup, confirm bool) error {
			order := []int{0, 1}
			if pair%2 == 1 {
				order = []int{1, 0}
			}
			pair++
			for _, i := range order {
				if err := ctx.Err(); err != nil {
					return err
				}
				r := runs[i]
				if opts.OnRep != nil {
					opts.OnRep(r.side.Name, sc.Name, n, warmup)
				}
				rep := runRep(ctx, r.opts.Exe, sc, n, warmup, sets, r.opts, srs[i].dir, timeout)
				if err := ctx.Err(); err != nil {
					return err
				}
				if opts.AfterRep != nil {
					opts.AfterRep(filepath.Join(srs[i].dir, repName(n, warmup)), &rep)
				}
				rep.Confirm = confirm
				srs[i].reps = append(srs[i].reps, rep)
				progress(opts.Progress, sc.Name+" "+r.side.Name, rep)
			}
			return nil
		}
		for n := 1 - opts.Warmup; n <= opts.Reps; n++ {
			if err := next(n, n <= 0, false); err != nil {
				return nil, err
			}
		}
		id, base := needsConfirmation(srs[0].reps)
		hid, head := needsConfirmation(srs[1].reps)
		if !base {
			id = hid
		}
		if base || head {
			if opts.Progress != nil {
				fmt.Fprintf(opts.Progress, "%-18s %s fired in under %d measured reps: %d confirmation reps a side\n",
					sc.Name, id, known.ConfirmBelow, known.ConfirmReps)
			}
			for c := 1; c <= known.ConfirmReps; c++ {
				if err := next(opts.Reps+c, false, true); err != nil {
					return nil, err
				}
			}
			for _, sr := range srs {
				sr.run.Confirmations = known.ConfirmReps
			}
		}
		wall := time.Since(scStart).Seconds()
		for i, r := range runs {
			srs[i].run.WallS = wall
			kenv := known.Env{MIMEResolvesMKV: resolvesMKV, Guards: sc.Guards}
			if err := build(*srs[i], tool, env, kenv, opts.ConfigPath).Write(srs[i].dir); err != nil {
				return nil, fmt.Errorf("writing %s's %s report: %w", r.side.Name, sc.Name, err)
			}
			r.done = append(r.done, *srs[i])
		}
	}

	run.WallS = time.Since(started).Seconds()
	for _, r := range runs {
		if err := r.res.finish(r.done, tool, env, resolvesMKV, opts.ConfigPath, run); err != nil {
			return nil, fmt.Errorf("%s: %w", r.side.Name, err)
		}
	}
	return &ABResult{Base: runs[0].res, Head: runs[1].res}, nil
}
