package check

import (
	"bufio"
	"context"
	"debug/buildinfo"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Module is momoshtrem's module path: check builds and tests it.
const Module = "github.com/shapedtime/momoshtrem"

// Tags are the build tags of the race build: production runs anacrolix's
// pure-Go uTP (CGO_ENABLED=0), and disable_libutp keeps a cgo build on it
// (PLAN D3). Race needs cgo.
const Tags = "disable_libutp"

// Build is how the race binary was made.
type Build struct {
	// Exe is the binary's path.
	Exe string `json:"exe"`
	// Command is the go build that made it; empty for a binary given
	// with --exe.
	Command []string `json:"command,omitempty"`
	TookS   float64  `json:"took_s"`
	// Race is whether the binary's build settings say -race: check
	// refuses one that doesn't.
	Race bool   `json:"race"`
	Tags string `json:"tags,omitempty"`
}

// ModuleDir finds momoshtrem's module root from dir: dir or a parent
// holding its go.mod, or a momoshtrem folder in one of them (the
// repository's root).
func ModuleDir(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	for d := abs; ; d = filepath.Dir(d) {
		for _, c := range []string{d, filepath.Join(d, "momoshtrem")} {
			if isModule(filepath.Join(c, "go.mod")) {
				return c, nil
			}
		}
		if filepath.Dir(d) == d {
			return "", fmt.Errorf("no %s module at or above %s: run from momoshtrem/", Module, abs)
		}
	}
}

// isModule reports whether the go.mod at path declares Module.
func isModule(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if line := strings.TrimSpace(sc.Text()); strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module ")) == Module
		}
	}
	return false
}

// BuildRace builds cmd/streamlab with -race and Tags into out, from the
// module at moduleDir. Race needs cgo: without it (CGO_ENABLED=0, or no
// C compiler) the build fails and says so.
func BuildRace(ctx context.Context, moduleDir, out string) (Build, error) {
	b := Build{Exe: out}
	cgo, err := goEnv(ctx, moduleDir, "CGO_ENABLED")
	if err != nil {
		return b, err
	}
	if cgo != "1" {
		return b, fmt.Errorf("the race detector needs cgo, and go env CGO_ENABLED is %q: set CGO_ENABLED=1 with a C compiler installed", cgo)
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return b, err
	}
	b.Command = []string{"go", "build", "-race", "-tags", Tags, "-o", out, "./cmd/streamlab"}
	start := time.Now()
	cmd := exec.CommandContext(ctx, b.Command[0], b.Command[1:]...)
	cmd.Dir = moduleDir
	cmd.Env = append(os.Environ(), "CGO_ENABLED=1")
	if msg, err := cmd.CombinedOutput(); err != nil {
		return b, fmt.Errorf("go build -race: %w\n%s", err, strings.TrimSpace(string(msg)))
	}
	b.TookS = time.Since(start).Seconds()
	return Inspect(b)
}

// Inspect reads a binary's build settings into b: whether it's a race
// build, and its tags.
func Inspect(b Build) (Build, error) {
	info, err := buildinfo.ReadFile(b.Exe)
	if err != nil {
		return b, fmt.Errorf("reading %s's build info: %w", b.Exe, err)
	}
	for _, s := range info.Settings {
		switch s.Key {
		case "-race":
			b.Race = s.Value == "true"
		case "-tags":
			b.Tags = s.Value
		}
	}
	if !b.Race {
		return b, fmt.Errorf("%s isn't a race build (built without -race): check runs the stress tier with the race detector", b.Exe)
	}
	return b, nil
}

func goEnv(ctx context.Context, dir, key string) (string, error) {
	cmd := exec.CommandContext(ctx, "go", "env", key)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return "", fmt.Errorf("go env %s: %w: %s", key, err, ee.Stderr)
		}
		return "", fmt.Errorf("go env %s: %w", key, err)
	}
	return strings.TrimSpace(string(out)), nil
}
