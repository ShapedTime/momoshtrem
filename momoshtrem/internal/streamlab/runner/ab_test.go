package runner_test

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/shapedtime/momoshtrem/internal/streamlab/report"
	"github.com/shapedtime/momoshtrem/internal/streamlab/runner"
	"github.com/shapedtime/momoshtrem/internal/streamlab/scenario"
)

func runAB(t *testing.T, mode string, reps, warmup int) (*runner.ABResult, []string, [2]string) {
	t.Helper()
	t.Setenv(fakeEnv, mode)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	sc, _ := scenario.Lookup("etag-stability")
	out := t.TempDir()
	dirs := [2]string{t.TempDir(), t.TempDir()}
	var order []string
	res, err := runner.RunAB(t.Context(), runner.ABOptions{
		Options: runner.Options{Scenarios: []*scenario.Scenario{sc}, Reps: reps, Warmup: warmup},
		Base:    runner.Side{Name: "base", Exe: exe, Dir: dirs[0], Out: filepath.Join(out, "base")},
		Head:    runner.Side{Name: "head", Exe: exe, Dir: dirs[1], Out: filepath.Join(out, "head")},
		OnRep: func(side, _ string, rep int, warmup bool) {
			label := fmt.Sprint(rep)
			if warmup {
				label = "w"
			}
			order = append(order, side+label)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return res, order, dirs
}

// The sides interleave, each pair led by the other side than the last;
// each child runs in its side's tree; each side's run is written.
func TestRunAB(t *testing.T) {
	res, order, dirs := runAB(t, "ok", 3, 1)
	if want := []string{"basew", "headw", "head1", "base1", "base2", "head2", "head3", "base3"}; !slices.Equal(order, want) {
		t.Errorf("order %v, want %v", order, want)
	}
	for i, side := range []*runner.Result{res.Base, res.Head} {
		if len(side.Reports) != 1 || side.Index.Verdict != report.VerdictPass {
			t.Fatalf("side %d: %+v", i, side.Index)
		}
		for _, f := range []string{"index.json", "etag-stability/report.json"} {
			if _, err := os.Stat(filepath.Join(side.Dir, f)); err != nil {
				t.Errorf("side %d: %v", i, err)
			}
		}
		r := side.Reports[0]
		if len(r.Reps) != 4 {
			t.Fatalf("side %d: %d reps", i, len(r.Reps))
		}
		want, _ := filepath.EvalSymlinks(dirs[i])
		for _, rep := range r.Reps {
			cwd, _ := strings.CutPrefix(rep.Notes[len(rep.Notes)-1], "cwd=")
			if got, _ := filepath.EvalSymlinks(cwd); got != want {
				t.Errorf("side %d rep %d ran in %s, want %s", i, rep.Rep, cwd, want)
			}
		}
	}
}

// An intermittent issue that fires on one side's single rep gets
// confirmation reps on both, so the sides stay alike.
func TestRunABConfirms(t *testing.T) {
	res, order, _ := runAB(t, "stall1:etag-stability", 1, 0)
	if len(order) != 6 {
		t.Errorf("order %v: 1 rep and 2 confirmations a side", order)
	}
	for _, side := range []*runner.Result{res.Base, res.Head} {
		r := side.Reports[0]
		if r.Run.Confirmations != 2 || len(r.Reps) != 3 || !r.Reps[2].Confirm {
			t.Errorf("%s: confirmations %d, reps %+v", side.Dir, r.Run.Confirmations, r.Reps)
		}
	}
}
