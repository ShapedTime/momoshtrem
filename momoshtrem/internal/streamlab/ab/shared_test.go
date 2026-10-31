package ab

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// gitIn runs git in dir for a test.
func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.email=lab@example.com", "-c", "user.name=lab", "-c", "protocol.file.allow=always"}, args...)...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
}

// newRepo is a repository at dir with one commit holding momoshtrem/go.mod.
func newRepo(t *testing.T, dir string) {
	t.Helper()
	write(t, filepath.Join(dir, "momoshtrem", "go.mod"), "module x\n")
	gitIn(t, dir, "init", "-q", "-b", "main")
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "-qm", "one")
}

func real(t *testing.T, p string) string {
	t.Helper()
	r, err := filepath.EvalSymlinks(p)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// The shared folder is the main checkout's .streamlab, from it and from
// its worktrees; a bare repository's worktree and a submodule keep their
// own (their common dir isn't a checkout's .git).
func TestSharedDir(t *testing.T) {
	base := real(t, t.TempDir())
	shared := func(dir string) string {
		t.Helper()
		d, err := SharedDir(t.Context(), dir)
		if err != nil {
			t.Fatal(err)
		}
		return d
	}

	main := filepath.Join(base, "main")
	newRepo(t, main)
	gitIn(t, main, "worktree", "add", "-q", filepath.Join(base, "wt"))
	for _, dir := range []string{main, filepath.Join(main, "momoshtrem"), filepath.Join(base, "wt", "momoshtrem")} {
		if got := shared(dir); got != filepath.Join(main, ".streamlab") {
			t.Errorf("%s: %s", dir, got)
		}
	}

	bare := filepath.Join(base, "bare.git")
	gitIn(t, base, "clone", "-q", "--bare", main, bare)
	gitIn(t, bare, "worktree", "add", "-q", filepath.Join(base, "bare-wt"), "main")
	if got := shared(filepath.Join(base, "bare-wt", "momoshtrem")); got != filepath.Join(base, "bare-wt", ".streamlab") {
		t.Errorf("a bare repository's worktree: %s", got)
	}

	super := filepath.Join(base, "super")
	newRepo(t, super)
	gitIn(t, super, "submodule", "add", "-q", main, "sub")
	if got := shared(filepath.Join(super, "sub", "momoshtrem")); got != filepath.Join(super, "sub", ".streamlab") {
		t.Errorf("a submodule: %s", got)
	}
}

// AddTree creates the shared folder with a .gitignore of its own, so a
// checkout on a branch from before streamlab doesn't list it.
func TestSharedDirIgnored(t *testing.T) {
	repo := real(t, t.TempDir())
	newRepo(t, repo)
	shared := filepath.Join(repo, ".streamlab")
	tr, err := AddTree(t.Context(), filepath.Join(repo, "momoshtrem"), filepath.Join(shared, "worktrees"), "HEAD", "HEAD", "head")
	if err != nil {
		t.Fatal(err)
	}
	defer tr.Done(t.Context(), false)
	if b, err := os.ReadFile(filepath.Join(shared, ".gitignore")); err != nil || string(b) != "*\n" {
		t.Errorf(".gitignore: %q %v", b, err)
	}
	out, err := exec.Command("git", "-C", repo, "status", "--porcelain", "--untracked-files=all").Output()
	if err != nil || len(out) != 0 {
		t.Errorf("git status: %q %v", out, err)
	}
}

// A kept tree (--keep) holds its claim until Done, through garbage
// collection: an *os.File nothing references is closed by its finalizer,
// which drops the flock (calibrate --keep did, mid-run).
func TestKeptTreeHoldsItsClaim(t *testing.T) {
	repo := real(t, t.TempDir())
	newRepo(t, repo)
	root := filepath.Join(t.TempDir(), "worktrees")
	func() {
		tr, err := AddTree(t.Context(), filepath.Join(repo, "momoshtrem"), root, "HEAD", "HEAD", "base")
		if err != nil {
			t.Fatal(err)
		}
		defer tr.Done(t.Context(), true)
		for range 5 {
			runtime.GC()
			time.Sleep(10 * time.Millisecond)
		}
		p, l, err := claim(root, "base")
		if err != nil {
			t.Fatal(err)
		}
		unlock(l)
		if p == filepath.Join(root, "base") {
			t.Error("another run claimed the kept tree while its run held it")
		}
	}()
	p, l, err := claim(root, "base")
	if err != nil || p != filepath.Join(root, "base") {
		t.Errorf("after Done: %s %v", p, err)
	}
	unlock(l)
	if _, err := os.Stat(filepath.Join(root, "base", "momoshtrem", "go.mod")); err != nil {
		t.Errorf("the kept tree: %v", err)
	}
}
