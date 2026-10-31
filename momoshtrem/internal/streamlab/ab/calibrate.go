package ab

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/shapedtime/momoshtrem/internal/streamlab/check"
	"github.com/shapedtime/momoshtrem/internal/streamlab/compare"
	"github.com/shapedtime/momoshtrem/internal/streamlab/report"
	"github.com/shapedtime/momoshtrem/internal/streamlab/runner"
	"github.com/shapedtime/momoshtrem/internal/streamlab/scenario"
)

// CalibrateOptions configure a calibration.
type CalibrateOptions struct {
	// Rev is the revision whose noise is measured: main by default. It
	// runs in a worktree with the working tree's lab, as ab's base does.
	Rev        string
	ModuleDir  string
	Scenarios  []*scenario.Scenario
	Reps       int
	Warmup     int
	Sets       map[string]string
	ConfigPath string
	GOMAXPROCS int
	Timeout    time.Duration
	Keep       bool
	// Out is the run's folder; runner.Dir()/runs/<id>-calibrate when
	// empty.
	Out      string
	Command  []string
	Progress io.Writer
}

// CalibrateResult is a calibration's outcome.
type CalibrateResult struct {
	Dir         string
	Calibration *compare.Calibration
	// Path is the noise.json it was written to.
	Path string
	// Skipped are the scenarios that failed: their numbers measure a
	// failure, not noise.
	Skipped []string
	// Errored are the scenarios the tool couldn't run.
	Errored []string
}

// Calibrate runs a revision's scenarios Reps times and stores each
// metric's coefficient of variation in noise.json, under this machine
// (compare.Machine): compare's count thresholds (3×CV) come from it.
// noise.json and the worktree are the main checkout's (SharedDir), so
// every checkout of the repository reads the calibration. When no
// scenario passed, the result has no Calibration and nothing is written.
func Calibrate(ctx context.Context, opts CalibrateOptions) (*CalibrateResult, error) {
	if opts.Reps < 2 {
		return nil, errors.New("calibrate: needs at least 2 reps to measure noise")
	}
	if opts.ModuleDir == "" {
		var err error
		if opts.ModuleDir, err = check.ModuleDir("."); err != nil {
			return nil, err
		}
	}
	rev := cmp.Or(opts.Rev, "main")
	sha, err := Commit(ctx, opts.ModuleDir, rev)
	if err != nil {
		return nil, err
	}
	lab := runner.Dir()
	shared, err := SharedDir(ctx, opts.ModuleDir)
	if err != nil {
		return nil, err
	}
	started := time.Now()
	if err := runner.EnsureDir(lab); err != nil {
		return nil, err
	}
	out := cmp.Or(opts.Out, filepath.Join(lab, "runs", fmt.Sprintf("%s-%d-calibrate", started.Format("20060102-150405"), os.Getpid())))
	if out, err = filepath.Abs(out); err != nil {
		return nil, err
	}
	worktrees := filepath.Join(shared, "worktrees")
	progress := func(format string, args ...any) {
		if opts.Progress != nil {
			fmt.Fprintf(opts.Progress, format+"\n", args...)
		}
	}
	t, err := AddTree(ctx, opts.ModuleDir, worktrees, rev, sha, "")
	if err != nil {
		return nil, err
	}
	progress("checked out %s (%s) in %s", rev, short(sha), t.Path)
	defer t.Done(ctx, opts.Keep) // with --keep too: it holds the claim until the end
	if err := CheckHooks(t.Module); err != nil {
		return nil, fmt.Errorf("%s (%s): %w", rev, short(sha), err)
	}
	if err := Overlay(opts.ModuleDir, t.Module); err != nil {
		return nil, err
	}
	bin := filepath.Join(lab, "bin", fmt.Sprintf("calibrate-%d", os.Getpid()))
	if !opts.Keep {
		defer os.RemoveAll(bin)
	}
	progress("building %s …", rev)
	b, err := BuildLab(ctx, t.Module, filepath.Join(bin, "streamlab"), false)
	if err != nil {
		return nil, err
	}
	res, err := runner.Run(ctx, runner.Options{Scenarios: opts.Scenarios, Reps: opts.Reps, Warmup: opts.Warmup, Sets: opts.Sets,
		ConfigPath: opts.ConfigPath, Out: out, GOMAXPROCS: opts.GOMAXPROCS, Timeout: opts.Timeout, Exe: b.Exe, Dir: t.Module,
		Command: opts.Command, Progress: opts.Progress})
	if err != nil {
		return nil, err
	}
	cr := &CalibrateResult{Dir: res.Dir, Path: compare.NoisePath(shared)}
	for _, r := range res.Reports {
		switch r.Verdict {
		case report.VerdictError:
			cr.Errored = append(cr.Errored, r.Scenario.Name)
		case report.VerdictFail:
			cr.Skipped = append(cr.Skipped, r.Scenario.Name)
		}
	}
	if len(cr.Skipped)+len(cr.Errored) == len(res.Reports) {
		return cr, nil // nothing passed: no noise to measure
	}
	cal, err := compare.Calibrate(res.Reports, sha, time.Now())
	if err != nil {
		return nil, err
	}
	cr.Calibration = cal
	noise, err := compare.ReadNoise(cr.Path)
	if err != nil {
		return nil, err
	}
	// Scenarios calibrated before and not now keep their numbers.
	if old := noise.Machines[cal.Machine]; old != nil {
		for name, cvs := range old.CV {
			if _, ok := cal.CV[name]; !ok {
				cal.CV[name] = cvs
			}
		}
	}
	noise.Machines[cal.Machine] = cal
	if err := noise.Write(cr.Path); err != nil {
		return nil, err
	}
	return cr, nil
}
