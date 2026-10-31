package ab

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/shapedtime/momoshtrem/internal/streamlab/runner"
)

// git runs git in dir and returns its trimmed output.
func git(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(errb.String()))
	}
	return strings.TrimSpace(out.String()), nil
}

// Commit resolves rev to a commit's full hash in the repository at dir.
func Commit(ctx context.Context, dir, rev string) (string, error) {
	sha, err := git(ctx, dir, "rev-parse", "--verify", "--quiet", rev+"^{commit}")
	if err != nil || sha == "" {
		return "", fmt.Errorf("%q is no commit here (git rev-parse)", rev)
	}
	return sha, nil
}

// SharedDir is streamlab's folder every checkout of the repository at dir
// shares: .streamlab in the main checkout, so a calibration or a worktree
// made from one of the repository's worktrees (an agent's under
// .claude/worktrees) serves the others. The main checkout is the parent
// of git's common dir when that dir is the checkout's own .git folder.
// Otherwise (a bare repository's worktree, a submodule, whose common dir
// is under the superproject's .git/modules) it's dir's own checkout's
// .streamlab, as runner.Dir finds it.
func SharedDir(ctx context.Context, dir string) (string, error) {
	common, err := git(ctx, dir, "rev-parse", "--git-common-dir")
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(common) {
		common = filepath.Join(dir, common)
	}
	common = filepath.Clean(common)
	main := filepath.Dir(common)
	if dotGit, err := os.Stat(filepath.Join(main, ".git")); err == nil && dotGit.IsDir() {
		if fi, err := os.Stat(common); err == nil && os.SameFile(dotGit, fi) {
			return filepath.Join(main, ".streamlab"), nil
		}
	}
	top, err := git(ctx, dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	return filepath.Join(top, ".streamlab"), nil
}

// Tree is a commit checked out in a worktree of its own, for one side of
// an A/B: under .streamlab/worktrees/<sha> (gitignored).
type Tree struct {
	Rev    string `json:"rev"`
	Commit string `json:"commit"`
	// Path is the worktree's root; Module the momoshtrem module in it.
	Path   string `json:"path"`
	Module string `json:"module"`
	repo   string
	lock   *os.File
}

// AddTree checks out commit sha of the repository holding moduleDir in a
// worktree under root (.streamlab/worktrees): root/<name>, <name> the
// sha unless given. Its owner holds an exclusive lock on
// root/<name>.lock for as long as it runs (flock: the kernel drops it
// when the owner dies), and a name another run holds makes this one
// root/<name>-<pid>. So ab runs side by side in one checkout, and a
// worktree a run that died left behind is the next run's to remove.
func AddTree(ctx context.Context, moduleDir, root, rev, sha, name string) (*Tree, error) {
	top, err := git(ctx, moduleDir, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, err
	}
	real, err := filepath.EvalSymlinks(moduleDir)
	if err != nil {
		return nil, err
	}
	top, err = filepath.EvalSymlinks(top)
	if err != nil {
		return nil, err
	}
	rel, err := filepath.Rel(top, real)
	if err != nil || strings.HasPrefix(rel, "..") {
		return nil, fmt.Errorf("%s isn't inside its repository %s", moduleDir, top)
	}
	if name == "" {
		name = sha
	}
	if err := runner.EnsureDir(filepath.Dir(root)); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	path, lock, err := claim(root, name)
	if err != nil {
		return nil, err
	}
	t := &Tree{Rev: rev, Commit: sha, Path: path, Module: filepath.Join(path, rel), repo: moduleDir, lock: lock}
	if _, err := os.Stat(path); err == nil {
		// Ours now, so a run that died left it: removed.
		t.removeTree()
	}
	// Twice forced: the path is ours even when git still records it, or
	// locked it, from an add that was interrupted. Not cancelled midway
	// (git would leave its "initializing" lock behind); checked after.
	if _, err := git(context.WithoutCancel(ctx), moduleDir, "worktree", "add", "--detach", "-f", "-f", t.Path, sha); err != nil {
		t.release()
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		t.Remove(ctx)
		return nil, err
	}
	return t, nil
}

// claim locks root/name for this process, or root/name-<pid> when another
// run holds it. The lock files stay: removing one another run may have
// opened would let two runs lock "the same" file.
func claim(root, name string) (string, *os.File, error) {
	for _, n := range []string{name, fmt.Sprintf("%s-%d", name, os.Getpid())} {
		path := filepath.Join(root, n)
		f, ok, err := tryLock(path + ".lock")
		if err != nil {
			return "", nil, err
		}
		if ok {
			return path, f, nil
		}
	}
	return "", nil, fmt.Errorf("can't claim a worktree under %s for %s: both names are held", root, name)
}

// Done ends the run's hold on the tree: removed (Remove), or kept for a
// look (--keep) with its name given up. A run defers it as soon as it
// has the tree, so the tree, and the lock file its claim rests on, stay
// reachable until the run ends: an *os.File nothing references is
// closed by its finalizer, and closing it drops the flock, which would
// let another run claim the name and remove the tree under this one.
func (t *Tree) Done(ctx context.Context, keep bool) error {
	if keep {
		t.release()
		return nil
	}
	return t.Remove(ctx)
}

// Remove removes the worktree and its record in the repository, and
// gives its name up. It touches no other worktree (no repository-wide
// prune).
func (t *Tree) Remove(ctx context.Context) error {
	err := t.removeTree()
	t.release()
	return err
}

func (t *Tree) removeTree() error {
	// Twice forced: a locked or dirty worktree (the lab copied over it)
	// is still ours. A folder git doesn't know is removed below.
	git(context.Background(), t.repo, "worktree", "remove", "--force", "--force", t.Path)
	return os.RemoveAll(t.Path)
}

func (t *Tree) release() {
	if t.lock != nil {
		unlock(t.lock)
		t.lock = nil
	}
}
