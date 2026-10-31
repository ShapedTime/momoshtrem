package ab

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/shapedtime/momoshtrem/internal/streamlab/compare"
	"github.com/shapedtime/momoshtrem/internal/streamlab/scenario"
)

func write(t *testing.T, path, s string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCheckHooks(t *testing.T) {
	// This module has them.
	if err := CheckHooks(filepath.Join("..", "..", "..")); err != nil {
		t.Fatal(err)
	}
	// A base from before #173: each missing hook named.
	dir := t.TempDir()
	write(t, filepath.Join(dir, "internal/vfs/fs.go"), "package vfs\n")
	write(t, filepath.Join(dir, "internal/torrent/activity.go"), "package torrent\n\nfunc (am *ActivityManager) Start() {}\n")
	write(t, filepath.Join(dir, "internal/torrent/client.go"), "package torrent\n\nfunc ClientConfigFor(cfg, cc any) any { return nil }\n")
	// A test file doesn't count.
	write(t, filepath.Join(dir, "internal/vfs/export_test.go"), "package vfs\n\nfunc NewStaticFS() {}\n")
	err := CheckHooks(dir)
	if !errors.Is(err, ErrNoHooks) || !strings.Contains(err.Error(), "base predates streamlab hooks") ||
		!strings.Contains(err.Error(), "H1 vfs.NewStaticFS") || !strings.Contains(err.Error(), "H3 ActivityManager.SetCheckInterval") ||
		strings.Contains(err.Error(), "H2") {
		t.Errorf("err = %v", err)
	}
}

// A worktree is claimed by a flock on its .lock file: free, then held
// (the next name), both held (an error); released, it's free again, and a
// lock its holder dropped (a run that died) is free at once.
func TestClaim(t *testing.T) {
	root := t.TempDir()
	p, l1, err := claim(root, "abc")
	if err != nil || p != filepath.Join(root, "abc") {
		t.Fatalf("free: %s %v", p, err)
	}
	p2, l2, err := claim(root, "abc")
	if err != nil || p2 != filepath.Join(root, fmt.Sprintf("abc-%d", os.Getpid())) {
		t.Fatalf("held by another run: %s %v", p2, err)
	}
	if _, _, err := claim(root, "abc"); err == nil {
		t.Error("both names held: claimed anyway")
	}
	unlock(l1)
	unlock(l2)
	if p, l, err := claim(root, "abc"); err != nil || p != filepath.Join(root, "abc") {
		t.Errorf("released: %s %v", p, err)
	} else {
		unlock(l)
	}
}

// A worktree an interrupted add left locked (git's "initializing" lock)
// doesn't stop the next AddTree, and Remove touches no other worktree:
// one whose folder is missing for now keeps its record (no prune).
func TestAddTreeRecovers(t *testing.T) {
	repo := t.TempDir()
	run := func(dir string, args ...string) string {
		t.Helper()
		out, err := git(t.Context(), dir, args...)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	run(repo, "init", "-q")
	write(t, filepath.Join(repo, "momoshtrem", "go.mod"), "module x\n")
	run(repo, "add", "-A")
	run(repo, "-c", "user.email=lab@example.com", "-c", "user.name=lab", "commit", "-qm", "one")
	sha := run(repo, "rev-parse", "HEAD")
	module := filepath.Join(repo, "momoshtrem")
	root := filepath.Join(t.TempDir(), "worktrees")

	tr, err := AddTree(t.Context(), module, root, "HEAD", sha, "")
	if err != nil {
		t.Fatal(err)
	}
	// As an add killed midway can leave it: locked, the folder gone.
	run(repo, "worktree", "lock", tr.Path)
	os.RemoveAll(tr.Path)
	tr.release()
	tr, err = AddTree(t.Context(), module, root, "HEAD", sha, "")
	if err != nil {
		t.Fatalf("after an interrupted add: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tr.Module, "go.mod")); err != nil {
		t.Fatal(err)
	}

	// Someone's own worktree, its folder away for now.
	mine := filepath.Join(t.TempDir(), "mine")
	run(repo, "worktree", "add", "-q", "--detach", mine, sha)
	os.Rename(mine, mine+".away")
	if err := tr.Remove(t.Context()); err != nil {
		t.Fatal(err)
	}
	list := run(repo, "worktree", "list", "--porcelain")
	if !strings.Contains(list, mine) || strings.Contains(list, tr.Path) {
		t.Errorf("worktrees after Remove:\n%s", list)
	}
}

// Takers racing for one base never share a name: each name has one
// holder at a time (the reviewer's repro shared one in 28 of 40 trials
// with the old .pid takeover).
func TestClaimRace(t *testing.T) {
	for trial := range 20 {
		root := t.TempDir()
		var wg sync.WaitGroup
		got := make([]string, 8)
		locks := make([]*os.File, 8)
		for i := range got {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if p, l, err := claim(root, "base"); err == nil {
					got[i], locks[i] = p, l
				}
			}()
		}
		wg.Wait()
		seen := map[string]bool{}
		for i, p := range got {
			if p == "" {
				continue
			}
			if seen[p] {
				t.Fatalf("trial %d: %s claimed twice: %v", trial, p, got)
			}
			seen[p] = true
			unlock(locks[i])
		}
		if !seen[filepath.Join(root, "base")] {
			t.Fatalf("trial %d: nobody got the base's own name: %v", trial, got)
		}
	}
}

// Overlay replaces the lab: the head's files land, the base's own go.
func TestOverlay(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	write(t, filepath.Join(src, "internal/streamlab/known/known.go"), "head")
	write(t, filepath.Join(src, "cmd/streamlab/main.go"), "head main")
	write(t, filepath.Join(dst, "internal/streamlab/known/known.go"), "base")
	write(t, filepath.Join(dst, "internal/streamlab/old/old.go"), "base only")
	write(t, filepath.Join(dst, "internal/vfs/fs.go"), "production")
	if err := Overlay(src, dst); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]string{
		"internal/streamlab/known/known.go": "head",
		"cmd/streamlab/main.go":             "head main",
		"internal/vfs/fs.go":                "production",
	} {
		if b, err := os.ReadFile(filepath.Join(dst, path)); err != nil || string(b) != want {
			t.Errorf("%s = %q, %v; want %q", path, b, err, want)
		}
	}
	if _, err := os.Stat(filepath.Join(dst, "internal/streamlab/old")); !os.IsNotExist(err) {
		t.Errorf("the base's own lab file is left: %v", err)
	}
}

// End to end, on the real tree (STREAMLAB=1): a worktree of HEAD, both
// sides built and run, the worktree gone after. HEAD against the working
// tree is an unchanged tree: never a regression.
func TestAB(t *testing.T) {
	if os.Getenv("STREAMLAB") != "1" {
		t.Skip("STREAMLAB=1 runs the A/B end to end (two builds and a scenario a side)")
	}
	sc, _ := scenario.Lookup("etag-stability")
	lab := t.TempDir()
	t.Chdir(filepath.Join("..", "..", ".."))
	res, err := Run(t.Context(), Options{Base: "HEAD", Scenarios: []*scenario.Scenario{sc}, Reps: 1, Out: lab, Progress: os.Stderr})
	if err != nil {
		t.Fatal(err)
	}
	if v := res.Compare.Verdict; v == compare.Regression || v == compare.Fail {
		t.Errorf("an unchanged tree: %s: %s", v, res.Compare.Why)
	}
	if res.Record.Base.Commit == "" || len(res.Record.Schedule) != 2 || res.Record.Schedule[0].Load == nil {
		t.Errorf("record %+v", res.Record)
	}
	for _, f := range []string{"compare.json", "compare.md", "ab.json", "base/index.json", "head/index.json"} {
		if _, err := os.Stat(filepath.Join(lab, f)); err != nil {
			t.Error(err)
		}
	}
	if _, err := os.Stat(res.Record.Base.Tree); !os.IsNotExist(err) {
		t.Errorf("the base's worktree is left: %v", err)
	}
	// The base child recorded the base's commit, from its own tree.
	if c := res.Compare.Base.Commit; c != res.Record.Base.Commit {
		t.Errorf("base's reports say commit %s, want %s", c, res.Record.Base.Commit)
	}
}
