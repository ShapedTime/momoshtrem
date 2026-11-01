package affected

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/shapedtime/momoshtrem/internal/streamlab/scenario"
)

// fakeGit is a diff and a history.
type fakeGit struct {
	changed []string
	commits map[string][]string
}

func (g fakeGit) Changed(context.Context, string) ([]string, string, error) {
	return g.changed, "0123456789ab", nil
}

func (g fakeGit) CommitFiles(_ context.Context, c string) ([]string, error) { return g.commits[c], nil }

func (fakeGit) Prefix(context.Context) (string, error) { return "momoshtrem/", nil }

// Linked is a few of what cmd/streamlab links.
func (fakeGit) Linked(context.Context) ([]string, error) {
	return []string{"internal/vfs", "internal/library", "internal/catalog", "internal/identify/ai", "internal/streamlab/runner"}, nil
}

var history = map[string][]string{
	"9de9c67": {"momoshtrem/internal/streaming/prioritizer.go"},
	"356e74d": {"momoshtrem/internal/vfs/library_fs.go", "momoshtrem/internal/vfs/torrent_file.go"},
}

func names(hits []Hit, focus bool) []string {
	var out []string
	for _, h := range hits {
		if h.Focus == focus {
			out = append(out, h.Name)
		}
	}
	return out
}

func TestByGuard(t *testing.T) {
	g := fakeGit{changed: []string{"momoshtrem/internal/streaming/prioritizer.go", "web/src/x.ts"}, commits: history}
	res, err := Run(t.Context(), g, "main", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(res.Files, []string{"internal/streaming/prioritizer.go"}) {
		t.Errorf("files %v: the web change is no module file", res.Files)
	}
	focus := names(res.Scenarios, true)
	for _, want := range []string{"steady-play", "seek-fwd", "seek-back", "reconnect", "dual-reader-open"} {
		if !slices.Contains(focus, want) {
			t.Errorf("%s guards the prioritizer (9de9c67 or F13): focus %v", want, focus)
		}
	}
	// The package table: every torrent scenario runs the prioritizer, no
	// HDRezka one does.
	all := append(focus, names(res.Scenarios, false)...)
	if !slices.Contains(all, "cold-start") || slices.ContainsFunc(all, func(n string) bool { return strings.HasPrefix(n, "hdrezka-") }) {
		t.Errorf("scenarios %v", all)
	}
	if len(res.Warnings) != 0 {
		t.Errorf("warnings %v", res.Warnings)
	}
	if len(res.Commands) == 0 || !strings.Contains(res.Commands[0], "--scenario steady-play,") {
		t.Errorf("commands %v", res.Commands)
	}
}

func TestLabAndGaming(t *testing.T) {
	// The lab alone: every scenario, no warning.
	res, err := Run(t.Context(), fakeGit{commits: history}, "main", []string{"internal/streamlab/report/report.go"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Scenarios) != len(scenario.All()) || len(res.Warnings) != 0 {
		t.Errorf("the lab: %d scenarios, warnings %v", len(res.Scenarios), res.Warnings)
	}
	// The streaming path and a known issue's signature together: warned.
	res, err = Run(t.Context(), fakeGit{commits: history}, "main", []string{"internal/vfs/torrent_file.go", "internal/streamlab/known/known.go"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], "internal/streamlab/known/known.go") {
		t.Errorf("warnings %v", res.Warnings)
	}
	// HDRezka's remote file: HDRezka scenarios (#66), and etag-stability
	// for F2, which lives there too.
	res, err = Run(t.Context(), fakeGit{commits: history}, "main", []string{"internal/vfs/remote_file.go"})
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range res.Scenarios {
		if !strings.HasPrefix(h.Name, "hdrezka-") && h.Name != "etag-stability" {
			t.Errorf("%s for a remote file change: %v", h.Name, h.Why)
		}
	}
	// Nothing the lab links: nothing to run.
	for _, f := range []string{"internal/api/router.go", "internal/catalog/title_test.go", "web/src/x.ts"} {
		res, err = Run(t.Context(), fakeGit{commits: history}, "main", []string{f})
		if err != nil || len(res.Scenarios) != 0 || len(res.Commands) != 0 {
			t.Errorf("%s: %v %+v", f, err, res)
		}
	}
}

// A package the lab links but the table doesn't name runs in every
// stack: the whole gate tier. The library is in the table: every
// scenario.
func TestLinked(t *testing.T) {
	gate := scenario.InTier(scenario.Gate)
	for _, f := range []string{"internal/catalog/title.go", "internal/identify/ai/prompt.txt"} {
		res, err := Run(t.Context(), fakeGit{commits: history}, "main", []string{f})
		if err != nil {
			t.Fatal(err)
		}
		if len(res.Scenarios) != len(gate) || !strings.Contains(res.Scenarios[0].Why[0], "in a package the lab links") ||
			!slices.Equal(res.Commands, []string{"streamlab ab --base main --tier gate"}) {
			t.Errorf("%s: %d scenarios %+v, commands %v", f, len(res.Scenarios), res.Scenarios[0], res.Commands)
		}
	}
	res, err := Run(t.Context(), fakeGit{commits: history}, "main", []string{"internal/library/models.go"})
	if err != nil || len(res.Scenarios) != len(scenario.All()) {
		t.Errorf("library: %v, %d scenarios", err, len(res.Scenarios))
	}
}

// The judgement is the whole lab: the runner that measures as much as
// the thresholds. With any code the lab runs in the same diff: warned.
func TestWarnings(t *testing.T) {
	for _, tc := range []struct {
		files []string
		warn  bool
	}{
		{[]string{"internal/vfs/torrent_file.go", "internal/streamlab/runner/runner.go"}, true},
		{[]string{"internal/catalog/title.go", "cmd/streamlab/compare.go"}, true},
		{[]string{"internal/library/models.go", "internal/streamlab/compare/rules.go"}, true},
		{[]string{"internal/api/router.go", "internal/streamlab/compare/rules.go"}, false},
		{[]string{"internal/vfs/torrent_file.go", "internal/vfs/library_fs.go"}, false},
		{[]string{"internal/streamlab/compare/rules.go"}, false},
	} {
		res, err := Run(t.Context(), fakeGit{commits: history}, "main", tc.files)
		if err != nil {
			t.Fatal(err)
		}
		if got := len(res.Warnings) > 0; got != tc.warn {
			t.Errorf("%v: warnings %v", tc.files, res.Warnings)
		}
	}
}

// GitCLI lists paths from the repo's root whatever diff.relative says,
// and Linked names the module's packages cmd/streamlab links.
func TestGitCLI(t *testing.T) {
	repo := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	write := func(name, s string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(filepath.Join(repo, name)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(repo, name), []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	git("init", "-q", "-b", "main")
	git("config", "diff.relative", "true")
	write("momoshtrem/internal/vfs/fs.go", "package vfs\n")
	write("web/x.ts", "1")
	git("add", "-A")
	git("-c", "user.email=lab@example.com", "-c", "user.name=lab", "commit", "-qm", "one")
	write("momoshtrem/internal/vfs/fs.go", "package vfs // changed\n")
	write("web/x.ts", "2")
	g := GitCLI{Dir: filepath.Join(repo, "momoshtrem")}
	files, _, err := g.Changed(t.Context(), "main")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(files, []string{"momoshtrem/internal/vfs/fs.go", "web/x.ts"}) {
		t.Errorf("changed %v", files)
	}

	pkgs, err := GitCLI{Dir: filepath.Join("..", "..", "..")}.Linked(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"internal/vfs", "internal/library", "internal/streamlab/compare", "cmd/streamlab"} {
		if !slices.Contains(pkgs, want) {
			t.Errorf("Linked lacks %s: %v", want, pkgs)
		}
	}
	if slices.Contains(pkgs, "internal/api") || slices.ContainsFunc(pkgs, func(p string) bool { return strings.HasPrefix(p, "..") || filepath.IsAbs(p) }) {
		t.Errorf("Linked %v", pkgs)
	}
}
