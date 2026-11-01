// Package ab is streamlab ab (docs/streamlab/PLAN.md §6, D5, D7): did a
// change make streaming worse than its base, on this machine, now?
//
// It checks the base out in a worktree under .streamlab/worktrees/<sha>
// (in the main checkout, SharedDir), copies the working tree's lab
// (internal/streamlab, cmd/streamlab, uncommitted changes included) over
// it, and over the head's worktree with --head, so both sides run the same
// scenarios, checks the base has the production hooks
// the lab needs (H1–H3), and builds both binaries. It then runs the
// scenarios on both, interleaved repetition by repetition (runner.RunAB),
// each repetition in a process of its own whose working directory is its
// side's tree, so each records its own commit. The machine's load is
// sampled before every repetition (ab.json), and each child records its
// own (report.Env.Load), which compare uses to spot skew.
//
// compare judges the two runs. Its timing and count verdicts rest on
// statistics over a few hundred tests, so ab confirms them: the
// scenarios whose verdict rests on statistics run again on fresh
// repetitions, and a change counts only when it holds there too
// (compare.Confirm). The worktrees go when it ends.
package ab

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/shapedtime/momoshtrem/internal/streamlab/check"
	"github.com/shapedtime/momoshtrem/internal/streamlab/compare"
	"github.com/shapedtime/momoshtrem/internal/streamlab/report"
	"github.com/shapedtime/momoshtrem/internal/streamlab/runner"
	"github.com/shapedtime/momoshtrem/internal/streamlab/scenario"
)

// RecordSchema names ab.json's format.
const RecordSchema = "streamlab.ab/v1"

// Options configure an A/B.
type Options struct {
	// Base is the base's revision (main, a commit); required.
	Base string
	// Head is the head's revision; the working tree at ModuleDir, its
	// uncommitted changes included, when empty.
	Head string
	// ModuleDir is momoshtrem's module, whose lab both sides run;
	// check.ModuleDir(".") when empty.
	ModuleDir  string
	Scenarios  []*scenario.Scenario
	Reps       int
	Warmup     int
	Sets       map[string]string
	ConfigPath string
	GOMAXPROCS int
	Timeout    time.Duration
	// Race builds both sides with -race and adds the race check: for the
	// stress tier. Timings then get no verdict (PLAN D8).
	Race bool
	// Confirm runs the scenarios whose verdict rests on statistics again
	// (compare.ToConfirm), as many reps a side, and keeps only what holds.
	Confirm bool
	// Force compares even across machines or params (compare.Options).
	Force bool
	// Keep keeps the worktrees and binaries.
	Keep bool
	// Out is the A/B's folder; runner.Dir()/runs/<id>-ab when empty.
	Out      string
	Command  []string
	Progress io.Writer
}

// Result is an A/B's outcome.
type Result struct {
	Dir     string
	Record  *Record
	Compare *compare.Result
}

// Record is ab.json: what ran, where, in which order, under what load.
type Record struct {
	Schema    string     `json:"schema"`
	Started   time.Time  `json:"started"`
	WallS     float64    `json:"wall_s"`
	Command   []string   `json:"command,omitempty"`
	Base      SideRecord `json:"base"`
	Head      SideRecord `json:"head"`
	Race      bool       `json:"race"`
	Reps      int        `json:"reps"`
	Warmup    int        `json:"warmup"`
	Scenarios []string   `json:"scenarios"`
	// Confirmed are the scenarios run again to confirm a verdict.
	Confirmed []string        `json:"confirmed,omitempty"`
	Schedule  []Slot          `json:"schedule"`
	Verdict   compare.Verdict `json:"verdict,omitempty"`
	Why       string          `json:"why,omitempty"`
}

// SideRecord is one side: its revision, tree and binary.
type SideRecord struct {
	Rev    string `json:"rev"`
	Commit string `json:"commit"`
	// Tree is the module the side built and ran in: a worktree, or the
	// working tree.
	Tree  string `json:"tree"`
	Build Build  `json:"build"`
}

// Slot is one repetition as scheduled, with the machine's load (1, 5 and
// 15 minute averages) just before it started.
type Slot struct {
	Stage    string    `json:"stage"`
	Side     string    `json:"side"`
	Scenario string    `json:"scenario"`
	Rep      int       `json:"rep"`
	Warmup   bool      `json:"warmup,omitempty"`
	At       time.Time `json:"at"`
	Load     []float64 `json:"load,omitempty"`
}

// Stages of an A/B.
const (
	StageScreen  = "screen"
	StageConfirm = "confirm"
)

// Run runs an A/B. The error is a tool or environment error: a revision
// that isn't one, a base without the hooks (ErrNoHooks), a build that
// fails, an interrupt, a comparison compare refuses.
func Run(ctx context.Context, opts Options) (*Result, error) {
	if opts.Base == "" {
		return nil, errors.New("ab: --base is required (main, say)")
	}
	if opts.ModuleDir == "" {
		var err error
		if opts.ModuleDir, err = check.ModuleDir("."); err != nil {
			return nil, err
		}
	}
	started := time.Now()
	lab := runner.Dir()
	shared, err := SharedDir(ctx, opts.ModuleDir)
	if err != nil {
		return nil, err
	}
	id := fmt.Sprintf("%s-%d-ab", started.Format("20060102-150405"), os.Getpid())
	if err := runner.EnsureDir(lab); err != nil {
		return nil, err
	}
	out := cmp.Or(opts.Out, filepath.Join(lab, "runs", id))
	if out, err = filepath.Abs(out); err != nil {
		return nil, err
	}
	// A folder an earlier A/B wrote would mix its confirm/ into this one's.
	if entries, err := os.ReadDir(out); err == nil && len(entries) > 0 {
		return nil, fmt.Errorf("%s isn't empty: give ab a new folder", out)
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		return nil, err
	}
	rec := &Record{Schema: RecordSchema, Started: started, Command: opts.Command, Race: opts.Race,
		Reps: opts.Reps, Warmup: opts.Warmup, Schedule: []Slot{}}
	for _, sc := range opts.Scenarios {
		rec.Scenarios = append(rec.Scenarios, sc.Name)
	}

	// The trees.
	baseSHA, err := Commit(ctx, opts.ModuleDir, opts.Base)
	if err != nil {
		return nil, err
	}
	rec.Base = SideRecord{Rev: opts.Base, Commit: baseSHA}
	worktrees := filepath.Join(shared, "worktrees")
	var trees []*Tree
	// The trees, and their claims, are held to the end (Tree.Done).
	defer func() {
		for _, t := range trees {
			if err := t.Done(ctx, opts.Keep); err != nil {
				opts.progress("removing the worktree %s: %v", t.Path, err)
			}
		}
	}()
	addTree := func(rev, sha, name string) (*Tree, error) {
		t, err := AddTree(ctx, opts.ModuleDir, worktrees, rev, sha, name)
		if err != nil {
			return nil, err
		}
		opts.progress("checked out %s (%s) in %s", rev, short(sha), t.Path)
		trees = append(trees, t)
		if err := CheckHooks(t.Module); err != nil {
			return nil, fmt.Errorf("%s (%s): %w", rev, short(sha), err)
		}
		if err := Overlay(opts.ModuleDir, t.Module); err != nil {
			return nil, fmt.Errorf("copying the lab over %s: %w", rev, err)
		}
		return t, nil
	}
	base, err := addTree(opts.Base, baseSHA, "")
	if err != nil {
		return nil, err
	}
	rec.Base.Tree = base.Module
	headModule := opts.ModuleDir
	rec.Head = SideRecord{Rev: "working tree", Tree: opts.ModuleDir}
	if opts.Head != "" {
		headSHA, err := Commit(ctx, opts.ModuleDir, opts.Head)
		if err != nil {
			return nil, err
		}
		name := ""
		if headSHA == baseSHA {
			name = headSHA + "-head"
		}
		head, err := addTree(opts.Head, headSHA, name)
		if err != nil {
			return nil, err
		}
		headModule = head.Module
		rec.Head = SideRecord{Rev: opts.Head, Commit: headSHA, Tree: head.Module}
	} else {
		if err := CheckHooks(headModule); err != nil {
			return nil, fmt.Errorf("the working tree: %w", err)
		}
		if sha, err := Commit(ctx, headModule, "HEAD"); err == nil {
			rec.Head.Commit = sha
		}
	}

	// The binaries, built side by side.
	bin := filepath.Join(lab, "bin", fmt.Sprintf("ab-%d", os.Getpid()))
	if !opts.Keep {
		defer os.RemoveAll(bin)
	}
	opts.progress("building both sides%s …", map[bool]string{true: " with -race", false: ""}[opts.Race])
	var wg sync.WaitGroup
	var baseErr, headErr error
	wg.Add(2)
	go func() {
		defer wg.Done()
		rec.Base.Build, baseErr = BuildLab(ctx, base.Module, filepath.Join(bin, "streamlab-base"), opts.Race)
	}()
	go func() {
		defer wg.Done()
		rec.Head.Build, headErr = BuildLab(ctx, headModule, filepath.Join(bin, "streamlab-head"), opts.Race)
	}()
	wg.Wait()
	if baseErr != nil {
		return nil, fmt.Errorf("building the base with HEAD's lab: %w", baseErr)
	}
	if headErr != nil {
		return nil, fmt.Errorf("building the head: %w", headErr)
	}
	opts.progress("built in %.0f s and %.0f s", rec.Base.Build.TookS, rec.Head.Build.TookS)

	run := func(stage string, scs []*scenario.Scenario, reps, warmup int, dir string) (*runner.ABResult, error) {
		ro := runner.Options{Scenarios: scs, Reps: reps, Warmup: warmup, Sets: opts.Sets, ConfigPath: opts.ConfigPath,
			GOMAXPROCS: opts.GOMAXPROCS, Timeout: opts.Timeout, Command: opts.Command, Progress: opts.Progress}
		if opts.Race {
			ro.Scenarios, ro.RepEnv, ro.AfterRep = check.RaceHooks(scs)
		}
		return runner.RunAB(ctx, runner.ABOptions{
			Options: ro,
			Base:    runner.Side{Name: "base", Exe: rec.Base.Build.Exe, Dir: base.Module, Out: filepath.Join(dir, "base")},
			Head:    runner.Side{Name: "head", Exe: rec.Head.Build.Exe, Dir: headModule, Out: filepath.Join(dir, "head")},
			OnRep: func(side, sc string, rep int, warmup bool) {
				rec.Schedule = append(rec.Schedule, Slot{Stage: stage, Side: side, Scenario: sc, Rep: rep, Warmup: warmup,
					At: time.Now(), Load: report.LoadAvg()})
			},
		})
	}
	opts.progress("\n%d scenarios, %d reps a side (+%d warm-up), interleaved:", len(opts.Scenarios), opts.Reps, opts.Warmup)
	screen, err := run(StageScreen, opts.Scenarios, opts.Reps, opts.Warmup, out)
	if err != nil {
		return nil, err
	}
	// The runs are on disk: ab.json first, so a comparison refused (two
	// machines, say) can be judged again with compare --force <folder>.
	rec.WallS = time.Since(started).Seconds()
	if err := report.WriteJSON(filepath.Join(out, "ab.json"), rec); err != nil {
		return nil, err
	}
	copts, err := NoiseFor(shared, screen.Head.Reports[0].Env)
	if err != nil {
		return nil, err
	}
	copts.Force = opts.Force
	res, err := compare.Compare(sideOf(screen.Base), sideOf(screen.Head), copts)
	if err != nil {
		return nil, err
	}
	names := compare.ToConfirm(res)
	confirm := opts.Confirm && len(names) > 0 && opts.Reps >= compare.MinTimingReps
	if opts.Confirm && len(names) > 0 && !confirm {
		opts.progress("\nscreen: %s\nnot confirming: %d reps a side is too few to judge timings (%d)", res.Line(), opts.Reps, compare.MinTimingReps)
	}
	if !confirm {
		res = compare.Unconfirmed(res)
	}
	if confirm {
		var scs []*scenario.Scenario
		for _, n := range names {
			if sc, ok := scenario.Lookup(n); ok {
				scs = append(scs, sc)
			}
		}
		opts.progress("\nscreen: %s\nconfirming %d scenarios on fresh reps, %d a side (+%d warm-up):", res.Line(), len(scs), opts.Reps, opts.Warmup)
		// Warm-ups again: the first fresh rep is otherwise the cold one,
		// and base always takes it.
		again, err := run(StageConfirm, scs, opts.Reps, opts.Warmup, filepath.Join(out, "confirm"))
		if err != nil {
			return nil, err
		}
		ares, err := compare.Compare(sideOf(again.Base), sideOf(again.Head), copts)
		if err != nil {
			return nil, err
		}
		res = compare.Confirm(res, ares, copts)
		rec.Confirmed = names
	}
	rec.WallS = time.Since(started).Seconds()
	rec.Verdict, rec.Why = res.Verdict, res.Why
	if err := res.Write(out); err != nil {
		return nil, err
	}
	if err := report.WriteJSON(filepath.Join(out, "ab.json"), rec); err != nil {
		return nil, err
	}
	return &Result{Dir: out, Record: rec, Compare: res}, nil
}

// Rejudge judges an ab's folder again, as Run did: base/ against head/,
// and the confirmation's confirm/base against confirm/head when it ran;
// without it, the screen is all there is (compare.Unconfirmed). After a
// calibrate, say, or once compare's rules change.
func Rejudge(dir string, force bool) (*compare.Result, error) {
	if _, err := os.Stat(filepath.Join(dir, "ab.json")); err != nil {
		return nil, fmt.Errorf("%s isn't an ab's folder (no ab.json)", dir)
	}
	load := func(sub string) (*compare.Side, *compare.Side, error) {
		b, err := compare.Load(filepath.Join(dir, sub, "base"))
		if err != nil {
			return nil, nil, err
		}
		h, err := compare.Load(filepath.Join(dir, sub, "head"))
		return b, h, err
	}
	base, head, err := load("")
	if err != nil {
		return nil, err
	}
	copts, err := NoiseFor("", head.Reports[0].Env)
	if err != nil {
		return nil, err
	}
	copts.Force = force
	res, err := compare.Compare(base, head, copts)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(filepath.Join(dir, "confirm", "base", "index.json")); err != nil {
		return compare.Unconfirmed(res), nil
	}
	ab, ah, err := load("confirm")
	if err != nil {
		return nil, err
	}
	again, err := compare.Compare(ab, ah, copts)
	if err != nil {
		return nil, err
	}
	return compare.Confirm(res, again, copts), nil
}

// NoiseFor is compare's options with the calibration of the machine env
// describes, from noise.json in shared (SharedDir; the working
// directory's when empty); none when there's none.
func NoiseFor(shared string, env report.Env) (compare.Options, error) {
	if shared == "" {
		shared = sharedHere()
	}
	path := compare.NoisePath(shared)
	n, err := compare.ReadNoise(path)
	if err != nil {
		return compare.Options{}, err
	}
	if cal := n.Machines[compare.Machine(env)]; cal != nil {
		return compare.Options{Noise: cal, NoiseSource: path}, nil
	}
	return compare.Options{}, nil
}

// sharedHere is SharedDir of the working directory's repository; this
// checkout's .streamlab outside one.
func sharedHere() string {
	if d, err := SharedDir(context.Background(), "."); err == nil {
		return d
	}
	return runner.Dir()
}

func sideOf(r *runner.Result) *compare.Side {
	return &compare.Side{Path: filepath.Join(r.Dir, "index.json"), Reports: r.Reports}
}

func (o Options) progress(format string, args ...any) {
	if o.Progress != nil {
		fmt.Fprintf(o.Progress, format+"\n", args...)
	}
}

func short(sha string) string {
	if len(sha) > 12 {
		return sha[:12]
	}
	return sha
}
