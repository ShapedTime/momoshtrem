// Package affected is streamlab affected (docs/streamlab/PLAN.md §6
// "Baseline", §11 "Agents game the gate"): which scenarios cover a diff,
// so an agent runs ab on those first, and a warning when one diff
// changes both the streaming path and what judges it.
//
// A changed file maps to scenarios three ways:
//   - by guard: a scenario guards commits and known issues (its Guards);
//     a file one of those commits changed, or where one of those issues
//     lives (PLAN §3), maps to the scenario. These are the focus.
//   - by package: a table from the streaming path's packages to the
//     scenarios that run their code. The lab itself, go.mod and the
//     config map to every scenario.
//   - by linking: any other package the lab's binary links (go list
//     -deps ./cmd/streamlab) runs in the stack the scenarios start, so a
//     file there maps to the whole gate tier.
package affected

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/shapedtime/momoshtrem/internal/streamlab/scenario"
)

// Schema names affected's JSON.
const Schema = "streamlab.affected/v1"

// Result is what a diff affects.
type Result struct {
	Schema string `json:"schema"`
	// Base is what the diff is against, and MergeBase where it forked.
	Base      string `json:"base"`
	MergeBase string `json:"merge_base,omitempty"`
	// Files are the changed files, relative to the module (others, such
	// as web/, are dropped).
	Files []string `json:"files"`
	// Scenarios are those covering a changed file, the focus first.
	Scenarios []Hit `json:"scenarios"`
	// Warnings are for a reviewer: the diff changes the streaming path
	// and the lab's judgement together.
	Warnings []string `json:"warnings,omitempty"`
	// Commands are what to run.
	Commands []string `json:"commands,omitempty"`
}

// Hit is a scenario a diff affects, and why.
type Hit struct {
	Name string `json:"name"`
	Tier string `json:"tier"`
	// Focus is set when a guard matched: the scenario was written for
	// what changed.
	Focus bool     `json:"focus"`
	Why   []string `json:"why"`
}

// Streaming is the streaming path's production code, module-relative
// prefixes: what the lab exercises and A/Bs judge. Any other package the
// lab links counts too (Git.Linked).
var Streaming = []string{
	"internal/vfs/", "internal/webdav/", "internal/streaming/", "internal/torrent/", "internal/hdrezka/",
	"internal/service/hdrezka", "internal/sessions/", "internal/metrics/", "internal/common/", "internal/config/",
	"internal/library/", "go.mod", "go.sum",
}

// Judgement is the lab's judgement, module-relative prefixes: the whole
// lab, since all of it decides a verdict (its thresholds and floors, its
// known-issue signatures, its checks and scenarios, its baseline, but also
// the runner, the stack, the observer and the client that measure). One
// diff changing it and the streaming path together could loosen what
// would catch the change.
var Judgement = []string{"internal/streamlab/", "cmd/streamlab/"}

// lab is the lab's own code: a change there affects every scenario.
var lab = Judgement

// selector picks scenarios.
type selector func(*scenario.Scenario) bool

func all(*scenario.Scenario) bool { return true }

func hdrezka(sc *scenario.Scenario) bool { return strings.HasPrefix(sc.Name, "hdrezka-") }

func torrents(sc *scenario.Scenario) bool { return !hdrezka(sc) }

// packages maps module-relative prefixes to the scenarios that run them,
// the most specific prefix first.
var packages = []struct {
	prefix string
	pick   selector
	why    string
}{
	{"internal/vfs/remote_file.go", hdrezka, "HDRezka's remote files"},
	{"internal/service/hdrezka", hdrezka, "HDRezka's VFS source"},
	{"internal/hdrezka/", hdrezka, "the HDRezka client and streamer"},
	{"internal/vfs/", all, "the VFS every scenario reads through"},
	{"internal/webdav/", all, "the WebDAV server every scenario talks to"},
	{"internal/streaming/", torrents, "the priority readers and prioritizer"},
	{"internal/torrent/", torrents, "the torrent service and client"},
	{"internal/sessions/", all, "sessions (quiescence)"},
	{"internal/metrics/", all, "the metrics the checks read"},
	{"internal/common/", all, "shared VFS file info"},
	{"internal/config/", all, "the config every stack loads"},
	{"internal/library/", all, "the library the VFS is built from"},
	{"go.mod", all, "dependencies: anacrolix and x/net are the streaming path"},
	{"go.sum", all, "dependencies"},
}

// knownFiles are where each known issue and suspect lives (PLAN §3):
// a scenario guarding one is the focus for a diff touching its files.
var knownFiles = map[string][]string{
	"F1":   {"internal/webdav/server.go"},
	"F2":   {"internal/vfs/library_fs.go", "internal/vfs/torrent_file.go", "internal/vfs/remote_file.go", "internal/webdav/server.go"},
	"F3":   {"internal/vfs/library_fs.go"},
	"F6":   {"internal/streaming/reader.go"},
	"F11":  {"internal/torrent/client.go", "internal/torrent/piececache/"},
	"F12":  {"internal/vfs/torrent_file.go"},
	"F13":  {"internal/streaming/reader.go", "internal/streaming/prioritizer.go"},
	"#66":  {"internal/vfs/remote_file.go", "internal/hdrezka/", "internal/service/hdrezka_source.go"},
	"#195": {"go.mod", "go.sum"},
	"#225": {"internal/vfs/torrent_file.go"},
	"#230": {"internal/torrent/service.go"},
	"#231": {"internal/streaming/reader.go"},
	"#232": {"internal/vfs/remote_file.go", "internal/hdrezka/streamer.go"},
}

var commitRe = regexp.MustCompile(`^[0-9a-f]{7,40}$`)

// Git is what affected asks git and go: the files a diff changes, those a
// commit changed, and the packages the lab links. Run uses git and go
// themselves.
type Git interface {
	// Changed are the files changed since base, repo-relative: the
	// commits since the merge base, the working tree's changes and new
	// files.
	Changed(ctx context.Context, base string) (files []string, mergeBase string, err error)
	// CommitFiles are the files a commit changed, repo-relative.
	CommitFiles(ctx context.Context, commit string) ([]string, error)
	// Prefix is the module's folder in the repo ("momoshtrem/").
	Prefix(ctx context.Context) (string, error)
	// Linked are the module's packages cmd/streamlab links, as
	// module-relative folders ("internal/catalog").
	Linked(ctx context.Context) ([]string, error)
}

// Run maps the diff against base to the scenarios; files, when given,
// stand for the diff (module-relative).
func Run(ctx context.Context, g Git, base string, files []string) (*Result, error) {
	res := &Result{Schema: Schema, Base: base}
	prefix, err := g.Prefix(ctx)
	if err != nil {
		return nil, err
	}
	if files == nil {
		changed, mb, err := g.Changed(ctx, base)
		if err != nil {
			return nil, err
		}
		res.MergeBase = mb
		files = moduleFiles(changed, prefix)
	}
	res.Files = files
	pkgs, err := g.Linked(ctx)
	if err != nil {
		return nil, err
	}
	linked := map[string]bool{}
	for _, p := range pkgs {
		linked[p] = true
	}
	hits := map[string]*Hit{}
	hit := func(sc *scenario.Scenario, focus bool, why string) {
		h := hits[sc.Name]
		if h == nil {
			h = &Hit{Name: sc.Name, Tier: string(sc.Tier)}
			hits[sc.Name] = h
		}
		h.Focus = h.Focus || focus
		if !slices.Contains(h.Why, why) {
			h.Why = append(h.Why, why)
		}
	}
	commitFiles := map[string][]string{}
	var labFiles []string
	for _, f := range files {
		if under(f, lab) {
			labFiles = append(labFiles, f)
		}
	}
	if len(labFiles) > 0 {
		why := "the lab changed: " + strings.Join(labFiles[:min(3, len(labFiles))], ", ")
		if len(labFiles) > 3 {
			why += fmt.Sprintf(" and %d more", len(labFiles)-3)
		}
		for _, sc := range scenario.All() {
			hit(sc, false, why)
		}
	}
	for _, f := range files {
		if under(f, lab) {
			continue
		}
		mapped := false
		for _, p := range packages {
			if !strings.HasPrefix(f, p.prefix) {
				continue
			}
			for _, sc := range scenario.All() {
				if p.pick(sc) {
					hit(sc, false, fmt.Sprintf("%s: %s", f, p.why))
				}
			}
			mapped = true
			break
		}
		if !mapped && linkedFile(f, linked) {
			for _, sc := range scenario.InTier(scenario.Gate) {
				hit(sc, false, fmt.Sprintf("%s: in a package the lab links (%s)", f, path.Dir(f)))
			}
		}
		for _, sc := range scenario.All() {
			for _, guard := range sc.Guards {
				if paths, ok := knownFiles[guard]; ok && matchAny(f, paths) {
					hit(sc, true, fmt.Sprintf("guards %s, which lives in %s", guard, f))
					continue
				}
				if !commitRe.MatchString(guard) {
					continue
				}
				cf, ok := commitFiles[guard]
				if !ok {
					repoFiles, err := g.CommitFiles(ctx, guard)
					if err != nil {
						return nil, err
					}
					cf = moduleFiles(repoFiles, prefix)
					commitFiles[guard] = cf
				}
				if slices.Contains(cf, f) {
					hit(sc, true, fmt.Sprintf("guards %s, which changed %s", guard, f))
				}
			}
		}
	}
	// Scenarios in catalog order, the focus first.
	for _, focus := range []bool{true, false} {
		for _, sc := range scenario.All() {
			if h := hits[sc.Name]; h != nil && h.Focus == focus {
				res.Scenarios = append(res.Scenarios, *h)
			}
		}
	}
	res.Warnings = warnings(files, linked)
	res.Commands = commands(res.Scenarios, base)
	return res, nil
}

// warnings flags a diff that changes the streaming path (or any package
// the lab links) and the lab's judgement together.
func warnings(files []string, linked map[string]bool) []string {
	var prod, judge []string
	for _, f := range files {
		switch {
		case under(f, Judgement):
			judge = append(judge, f)
		case under(f, Streaming), linkedFile(f, linked):
			prod = append(prod, f)
		}
	}
	if len(prod) == 0 || len(judge) == 0 {
		return nil
	}
	return []string{fmt.Sprintf("this diff changes code the lab runs (%s) and the lab itself (%s) together: what it measures, its thresholds, "+
		"signatures, scenarios or baseline. ab runs one lab on both sides, so a looser lab hides a regression from it: land the lab change on its own first, "+
		"and have a reviewer check it doesn't loosen what would catch the streaming change (PLAN §11)", strings.Join(prod, ", "), strings.Join(judge, ", "))}
}

// commands are what to run: ab over the focus, then over every affected
// gate scenario, and check over affected stress scenarios.
func commands(hits []Hit, base string) []string {
	var focus, gate, stress []string
	for _, h := range hits {
		switch h.Tier {
		case string(scenario.Gate):
			gate = append(gate, h.Name)
			if h.Focus {
				focus = append(focus, h.Name)
			}
		case string(scenario.Stress):
			stress = append(stress, h.Name)
		}
	}
	var out []string
	if len(focus) > 0 && len(focus) < len(gate) {
		out = append(out, fmt.Sprintf("streamlab ab --base %s --scenario %s", base, strings.Join(focus, ",")))
	}
	if len(gate) > 0 {
		if len(gate) == len(scenario.InTier(scenario.Gate)) {
			out = append(out, fmt.Sprintf("streamlab ab --base %s --tier gate", base))
		} else {
			out = append(out, fmt.Sprintf("streamlab ab --base %s --scenario %s", base, strings.Join(gate, ",")))
		}
	}
	if len(stress) > 0 {
		out = append(out, fmt.Sprintf("streamlab check --scenario %s", strings.Join(stress, ",")))
	}
	return out
}

// linkedFile reports whether f is built into a package the lab links: a
// file in its folder (a .go file, or one it embeds), not a test.
func linkedFile(f string, linked map[string]bool) bool {
	return linked[path.Dir(f)] && !strings.HasSuffix(f, "_test.go")
}

func under(f string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(f, p) {
			return true
		}
	}
	return false
}

func matchAny(f string, paths []string) bool {
	for _, p := range paths {
		if f == p || (strings.HasSuffix(p, "/") && strings.HasPrefix(f, p)) {
			return true
		}
	}
	return false
}

// moduleFiles are the repo-relative files under the module's prefix,
// made module-relative.
func moduleFiles(files []string, prefix string) []string {
	var out []string
	for _, f := range files {
		if rel, ok := strings.CutPrefix(f, prefix); ok {
			out = append(out, rel)
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// GitCLI is Git through the git command, in dir.
type GitCLI struct{ Dir string }

func (g GitCLI) run(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = g.Dir
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(errb.String()))
	}
	return out.String(), nil
}

// Changed implements Git.
func (g GitCLI) Changed(ctx context.Context, base string) ([]string, string, error) {
	mb, err := g.run(ctx, "merge-base", base, "HEAD")
	if err != nil {
		return nil, "", err
	}
	mb = strings.TrimSpace(mb)
	// The merge base against the working tree: committed and uncommitted
	// changes alike, as git diff base... with the working tree. Paths
	// from the repo's root even under diff.relative.
	diff, err := g.run(ctx, "diff", "--no-relative", "--name-only", mb)
	if err != nil {
		return nil, "", err
	}
	untracked, err := g.run(ctx, "ls-files", "--others", "--exclude-standard", "--full-name", ":/")
	if err != nil {
		return nil, "", err
	}
	return append(lines(diff), lines(untracked)...), mb, nil
}

// CommitFiles implements Git.
func (g GitCLI) CommitFiles(ctx context.Context, commit string) ([]string, error) {
	out, err := g.run(ctx, "show", "--no-relative", "--name-only", "--format=", commit)
	if err != nil {
		return nil, nil // a guard that isn't a commit here (a short hash of another history): no files
	}
	return lines(out), nil
}

// Prefix implements Git.
func (g GitCLI) Prefix(ctx context.Context) (string, error) {
	out, err := g.run(ctx, "rev-parse", "--show-prefix")
	if err != nil {
		return "", err
	}
	// "momoshtrem/", or "" for a module at the repo's root.
	return strings.TrimSpace(out), nil
}

// Linked implements Git: go list -deps over cmd/streamlab, the module's
// packages only.
func (g GitCLI) Linked(ctx context.Context) ([]string, error) {
	cmd := exec.CommandContext(ctx, "go", "list", "-deps", "-f", "{{if not .Standard}}{{.Dir}}{{end}}", "./cmd/streamlab")
	cmd.Dir = g.Dir
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("go list -deps ./cmd/streamlab: %w: %s", err, strings.TrimSpace(errb.String()))
	}
	root, err := filepath.Abs(g.Dir)
	if err != nil {
		return nil, err
	}
	var pkgs []string
	for _, dir := range lines(out.String()) {
		rel, err := filepath.Rel(root, dir)
		if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
			continue // the module's root, or another module (a dependency)
		}
		pkgs = append(pkgs, filepath.ToSlash(rel))
	}
	return pkgs, nil
}

func lines(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}
