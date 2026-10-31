package ab

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/shapedtime/momoshtrem/internal/streamlab/check"
)

// LabDirs are the lab's code, relative to the module: HEAD's copy goes
// over the base's, so both sides run the same lab (PLAN §6 "ab").
var LabDirs = []string{"internal/streamlab", "cmd/streamlab"}

// ErrNoHooks is a base older than the production hooks the lab needs.
var ErrNoHooks = errors.New("base predates streamlab hooks")

// hooks are the production hooks the lab can't run without (PLAN §4):
// where each is declared, and how to find it.
var hooks = []struct {
	id, dir string
	decl    *regexp.Regexp
	what    string
}{
	{"H1", "internal/vfs", regexp.MustCompile(`(?m)^func NewStaticFS\(`), "vfs.NewStaticFS"},
	{"H2", "internal/torrent", regexp.MustCompile(`(?m)^func ClientConfigFor\(`), "torrent.ClientConfigFor"},
	{"H3", "internal/torrent", regexp.MustCompile(`(?m)^func \([a-z]+ \*ActivityManager\) SetCheckInterval\(`), "ActivityManager.SetCheckInterval"},
}

// CheckHooks reports ErrNoHooks, naming what's missing, when the module
// at dir lacks any of H1–H3.
func CheckHooks(dir string) error {
	var missing []string
	for _, h := range hooks {
		if !declared(filepath.Join(dir, h.dir), h.decl) {
			missing = append(missing, h.id+" "+h.what)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("%w: it has no %s (#173); compare against a later base", ErrNoHooks, strings.Join(missing, ", "))
	}
	return nil
}

// declared reports whether a non-test Go file in dir matches decl.
func declared(dir string, decl *regexp.Regexp) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, n))
		if err == nil && decl.Match(b) {
			return true
		}
	}
	return false
}

// Overlay replaces the lab's code in the module at dst with the module
// at src's: LabDirs removed, then copied over, so files the head deleted
// go too.
func Overlay(src, dst string) error {
	for _, d := range LabDirs {
		to := filepath.Join(dst, d)
		if err := os.RemoveAll(to); err != nil {
			return err
		}
		if err := copyTree(filepath.Join(src, d), to); err != nil {
			return fmt.Errorf("copying %s: %w", d, err)
		}
	}
	return nil
}

func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		to := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(to, 0o755)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		return copyFile(path, to)
	})
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	fi, err := in.Stat()
	if err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, fi.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// Build is how a side's binary was made.
type Build struct {
	Exe     string   `json:"exe"`
	Command []string `json:"command"`
	TookS   float64  `json:"took_s"`
}

// BuildLab builds cmd/streamlab of the module at dir into out, as
// streamlab is run (-tags disable_libutp, PLAN D3), or with -race for a
// race A/B (check.BuildRace).
func BuildLab(ctx context.Context, dir, out string, race bool) (Build, error) {
	if race {
		b, err := check.BuildRace(ctx, dir, out)
		return Build{Exe: b.Exe, Command: b.Command, TookS: b.TookS}, err
	}
	b := Build{Exe: out, Command: []string{"go", "build", "-tags", check.Tags, "-o", out, "./cmd/streamlab"}}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return b, err
	}
	start := time.Now()
	cmd := exec.CommandContext(ctx, b.Command[0], b.Command[1:]...)
	cmd.Dir = dir
	var msg bytes.Buffer
	cmd.Stdout, cmd.Stderr = &msg, &msg
	if err := cmd.Run(); err != nil {
		return b, fmt.Errorf("go build in %s: %w\n%s", dir, err, strings.TrimSpace(msg.String()))
	}
	b.TookS = time.Since(start).Seconds()
	return b, nil
}
