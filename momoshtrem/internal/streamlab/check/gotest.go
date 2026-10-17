package check

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/shapedtime/momoshtrem/internal/streamlab/race"
	"github.com/shapedtime/momoshtrem/internal/streamlab/report"
)

// TestPackages are the packages check runs go test -race over: the lab
// itself and the streaming path's (PLAN §10).
var TestPackages = []string{
	"./internal/streamlab/...",
	"./cmd/streamlab/...",
	"./internal/vfs",
	"./internal/streaming",
	"./internal/torrent",
	"./internal/sessions",
}

// testTimeout bounds go test's run, each package's.
const testTimeout = "15m"

// TestOutput is the file the tests' output goes to, in the run's folder.
const TestOutput = "go-test-race.txt"

// Tests is go test -race's outcome over TestPackages.
type Tests struct {
	Command []string `json:"command"`
	// Verdict is fail when a package failed (a test, a race, a build),
	// error when go test couldn't run.
	Verdict  report.Verdict `json:"verdict"`
	Error    string         `json:"error,omitempty"`
	Packages []Package      `json:"packages"`
	// Races are the data races the tests reported, by signature.
	Races  []report.Race `json:"race_reports"`
	Output string        `json:"output"`
	TookS  float64       `json:"took_s"`
}

// Package is one package's result.
type Package struct {
	Name     string  `json:"name"`
	Status   string  `json:"status"` // pass, fail or skip (no test files)
	ElapsedS float64 `json:"elapsed_s"`
	// Failed are its tests that failed.
	Failed []string `json:"failed,omitempty"`
	// Races is how many race reports its output had.
	Races int `json:"races,omitempty"`
}

// testEvent is one line of go test -json (test2json).
type testEvent struct {
	Action  string
	Package string
	Test    string
	Elapsed float64
	Output  string
}

// GoTestRace runs go test -race -tags Tags -json over pkgs in moduleDir,
// writing its output as text to dir/TestOutput. STREAMLAB isn't set: the
// lab's heavy tests skip.
func GoTestRace(ctx context.Context, moduleDir, dir string, pkgs []string) (t Tests) {
	t = Tests{Command: append([]string{"go", "test", "-race", "-tags", Tags, "-count=1", "-timeout", testTimeout, "-json"}, pkgs...),
		Verdict: report.VerdictPass, Output: TestOutput}
	start := time.Now()
	defer func() { t.TookS = time.Since(start).Seconds() }()
	out, err := os.Create(filepath.Join(dir, TestOutput))
	if err != nil {
		t.Verdict, t.Error = report.VerdictError, err.Error()
		return t
	}
	defer out.Close()

	cmd := exec.CommandContext(ctx, t.Command[0], t.Command[1:]...)
	cmd.Dir = moduleDir
	cmd.Env = append(withoutEnv(os.Environ(), "STREAMLAB"), "CGO_ENABLED=1")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Verdict, t.Error = report.VerdictError, err.Error()
		return t
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Verdict, t.Error = report.VerdictError, err.Error()
		return t
	}
	pkgs2, races, perr := parseTestJSON(stdout, out)
	waitErr := cmd.Wait()
	t.Packages, t.Races = pkgs2, races
	for _, p := range t.Packages {
		if p.Status == "fail" {
			t.Verdict = report.VerdictFail
		}
	}
	switch {
	case ctx.Err() != nil:
		t.Verdict, t.Error = report.VerdictError, "interrupted"
	case perr != nil:
		t.Verdict, t.Error = report.VerdictError, perr.Error()
	case waitErr != nil && t.Verdict == report.VerdictPass:
		// go test failed without a failing package: a build or setup error.
		var ee *exec.ExitError
		if !errors.As(waitErr, &ee) {
			t.Verdict, t.Error = report.VerdictError, waitErr.Error()
		} else {
			t.Verdict, t.Error = report.VerdictFail, fmt.Sprintf("go test exited %d: %s", ee.ExitCode(), lastLine(stderr.String()))
		}
	}
	if s := strings.TrimSpace(stderr.String()); s != "" {
		fmt.Fprintf(out, "\n== stderr ==\n%s\n", s)
	}
	return t
}

// parseTestJSON reads go test -json events from r, writing their output
// as text to w, and returns each package's result and the races its
// output reported.
func parseTestJSON(r io.Reader, w io.Writer) ([]Package, []report.Race, error) {
	byName := map[string]*Package{}
	var order []string
	outputs := map[string]*strings.Builder{}
	pkg := func(name string) *Package {
		p, ok := byName[name]
		if !ok {
			p = &Package{Name: name}
			byName[name] = p
			order = append(order, name)
			outputs[name] = &strings.Builder{}
		}
		return p
	}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), 16<<20)
	for sc.Scan() {
		line := sc.Bytes()
		var ev testEvent
		if err := json.Unmarshal(line, &ev); err != nil || ev.Package == "" {
			// Not an event (a build error goes to stderr; some lines go
			// to stdout as text): keep it.
			fmt.Fprintf(w, "%s\n", line)
			continue
		}
		p := pkg(ev.Package)
		switch ev.Action {
		case "output", "build-output":
			io.WriteString(w, ev.Output)
			outputs[ev.Package].WriteString(ev.Output)
		case "fail":
			if ev.Test == "" {
				p.Status, p.ElapsedS = "fail", ev.Elapsed
			} else if !slices.Contains(p.Failed, ev.Test) {
				p.Failed = append(p.Failed, ev.Test)
			}
		case "pass":
			if ev.Test == "" {
				p.Status, p.ElapsedS = "pass", ev.Elapsed
			}
		case "skip":
			if ev.Test == "" {
				p.Status, p.ElapsedS = "skip", ev.Elapsed
			}
		}
	}
	var all []report.Race
	out := make([]Package, 0, len(order))
	for _, name := range order {
		p := byName[name]
		races, _ := race.Parse(strings.NewReader(outputs[name].String()), name)
		p.Races = 0
		for _, rc := range races {
			p.Races += rc.Count
		}
		all = report.MergeRaces(all, races)
		if p.Status == "" {
			p.Status = "fail" // no final event: it didn't finish
		}
		out = append(out, *p)
	}
	return out, all, sc.Err()
}

// withoutEnv drops key from an environment.
func withoutEnv(env []string, key string) []string {
	return slices.DeleteFunc(slices.Clone(env), func(kv string) bool { return strings.HasPrefix(kv, key+"=") })
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return lines[len(lines)-1]
}
