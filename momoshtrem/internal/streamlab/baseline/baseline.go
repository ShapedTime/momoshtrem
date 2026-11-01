// Package baseline is the gate's expectations (docs/streamlab/PLAN.md §6
// "Baseline", D5): internal/streamlab/testdata/baseline.json holds, per
// gate scenario, each check's expected status and each exact metric's
// value. Never a timing: timings depend on the machine and are judged by
// an A/B only.
//
// streamlab gate runs the gate tier and holds it to the baseline;
// streamlab baseline update --reason is the only way to change the file.
// It records the reason, the commit and the date, and a digest of the
// expectations, so an edit by hand shows: the gate refuses a baseline
// whose digest doesn't match.
package baseline

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/shapedtime/momoshtrem/internal/streamlab/report"
)

// Schema names baseline.json's format.
const Schema = "streamlab.baseline/v1"

// File is the baseline, relative to the momoshtrem module.
const File = "internal/streamlab/testdata/baseline.json"

// doc is the file's own note to whoever opens it.
const doc = "The gate's expectations: each gate scenario's check statuses and exact metrics, never timings (docs/streamlab/PLAN.md §6). " +
	"Change it only with `streamlab baseline update --reason \"…\"`: the gate refuses a file edited by hand (digest)."

// Baseline is baseline.json.
type Baseline struct {
	Schema string `json:"schema"`
	Doc    string `json:"doc"`
	// Updates are the file's history, oldest first: each update's reason,
	// commit and date.
	Updates []Update `json:"updates"`
	// Env is where the expectations were recorded: an expectation a known
	// issue makes depend on the host (F1 and the MIME table) is only
	// noted on another.
	Env Env `json:"env"`
	// Scenarios are the expectations, by scenario.
	Scenarios map[string]Scenario `json:"scenarios"`
	// Digest is the SHA-256 of Env and Scenarios as baseline update wrote
	// them.
	Digest string `json:"digest"`
}

// Update is one change of the baseline.
type Update struct {
	Date   time.Time `json:"date"`
	Commit string    `json:"commit"`
	Dirty  bool      `json:"dirty,omitempty"`
	Reason string    `json:"reason"`
	// Reps is how many measured repetitions per scenario it was taken
	// from.
	Reps int `json:"reps"`
}

// Env is the host the expectations were recorded on.
type Env struct {
	OS              string `json:"os"`
	MIMEResolvesMKV bool   `json:"mime_resolves_mkv"`
}

// Scenario is one scenario's expectations.
type Scenario struct {
	// Checks are each check's status: pass, n/a, xfail:<issue> or
	// xpass:<issue>.
	Checks map[string]string `json:"checks"`
	// Exact are the exact metrics that held one value over every
	// repetition.
	Exact map[string]float64 `json:"exact"`
	// Unstable are exact metrics whose values differed between
	// repetitions: not expected, only listed.
	Unstable []string `json:"unstable,omitempty"`
}

// Path is the baseline in the module at moduleDir.
func Path(moduleDir string) string { return filepath.Join(moduleDir, File) }

// ErrEdited is a baseline whose digest doesn't match: edited by hand.
var ErrEdited = errors.New("edited by hand (its digest doesn't match): change it only with streamlab baseline update --reason")

// Read reads a baseline and checks its digest (ErrEdited). On ErrEdited
// the baseline is returned too, for its history.
func Read(path string) (*Baseline, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var bl Baseline
	if err := json.Unmarshal(b, &bl); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if bl.Schema != Schema {
		return nil, fmt.Errorf("%s: schema %q, want %q", path, bl.Schema, Schema)
	}
	if d := bl.digest(); d != bl.Digest {
		return &bl, fmt.Errorf("%s was %w", path, ErrEdited)
	}
	return &bl, nil
}

// Diff says how next's expectations differ from bl's, a line per change.
func (bl *Baseline) Diff(next *Baseline) []string {
	var out []string
	names := map[string]bool{}
	for n := range bl.Scenarios {
		names[n] = true
	}
	for n := range next.Scenarios {
		names[n] = true
	}
	sorted := make([]string, 0, len(names))
	for n := range names {
		sorted = append(sorted, n)
	}
	slices.Sort(sorted)
	for _, n := range sorted {
		a, inA := bl.Scenarios[n]
		b, inB := next.Scenarios[n]
		switch {
		case !inA:
			out = append(out, n+": added")
			continue
		case !inB:
			out = append(out, n+": removed")
			continue
		}
		for _, k := range union(a.Checks, b.Checks) {
			if a.Checks[k] != b.Checks[k] {
				out = append(out, fmt.Sprintf("%s: check %s %s → %s", n, k, orNone(a.Checks[k]), orNone(b.Checks[k])))
			}
		}
		for _, k := range union(a.Exact, b.Exact) {
			va, okA := a.Exact[k]
			vb, okB := b.Exact[k]
			if okA != okB || va != vb {
				out = append(out, fmt.Sprintf("%s: exact %s %s → %s", n, k, numOr(va, okA), numOr(vb, okB)))
			}
		}
	}
	return out
}

func union[V any](a, b map[string]V) []string {
	keys := map[string]bool{}
	for k := range a {
		keys[k] = true
	}
	for k := range b {
		keys[k] = true
	}
	out := make([]string, 0, len(keys))
	for k := range keys {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

func orNone(s string) string {
	if s == "" {
		return "none"
	}
	return s
}

func numOr(v float64, ok bool) string {
	if !ok {
		return "none"
	}
	return report.Num(v)
}

// Write writes the baseline, its digest set.
func (bl *Baseline) Write(path string) error {
	bl.Digest = bl.digest()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return report.WriteJSON(path, bl)
}

func (bl *Baseline) digest() string {
	b, _ := json.Marshal(struct {
		Updates   []Update            `json:"updates"`
		Env       Env                 `json:"env"`
		Scenarios map[string]Scenario `json:"scenarios"`
	}{bl.Updates, bl.Env, bl.Scenarios})
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// ErrFailing is a run that can't become a baseline: a scenario didn't
// pass.
var ErrFailing = errors.New("the run didn't pass")

// ErrXPass is a run where a known issue appears fixed (an xpass): no
// expectation until the registry says so.
var ErrXPass = errors.New("a known issue appears fixed")

// Missing are the scenarios bl holds that none of reports re-records:
// after an edit by hand (ErrEdited), an update must re-record them all,
// or it would put the edit under a fresh digest.
func (bl *Baseline) Missing(reports []*report.Report) []string {
	ran := map[string]bool{}
	for _, r := range reports {
		ran[r.Scenario.Name] = true
	}
	var out []string
	for name := range bl.Scenarios {
		if !ran[name] {
			out = append(out, name)
		}
	}
	slices.Sort(out)
	return out
}

// FromRun makes a baseline of a gate run's reports, with prev's history
// and this update. Every scenario must have passed (xfail included): a
// baseline records what a passing gate looks like.
func FromRun(reports []*report.Report, prev *Baseline, up Update, mimeResolvesMKV bool) (*Baseline, error) {
	if strings.TrimSpace(up.Reason) == "" {
		return nil, errors.New("a baseline update needs a reason")
	}
	var failing []string
	for _, r := range reports {
		if r.Verdict != report.VerdictPass {
			failing = append(failing, fmt.Sprintf("%s %s", r.Scenario.Name, r.Verdict))
		}
	}
	if len(failing) > 0 {
		return nil, fmt.Errorf("%w: %s", ErrFailing, strings.Join(failing, ", "))
	}
	// An xpass is a known issue that appears fixed: the registry changes
	// first (its entry removed), then the baseline.
	var fixed []string
	for _, r := range reports {
		for _, c := range r.Checks {
			if c.Status == report.XPass {
				fixed = append(fixed, fmt.Sprintf("%s %s (%s)", r.Scenario.Name, c.ID, c.Known))
			}
		}
	}
	if len(fixed) > 0 {
		return nil, fmt.Errorf("%w: %s: remove those issues from internal/streamlab/known first", ErrXPass, strings.Join(fixed, ", "))
	}
	bl := &Baseline{Schema: Schema, Doc: doc, Scenarios: map[string]Scenario{}}
	if prev != nil {
		bl.Updates = slices.Clone(prev.Updates)
		// A partial run keeps the other scenarios' expectations.
		for name, sc := range prev.Scenarios {
			bl.Scenarios[name] = sc
		}
	}
	for _, r := range reports {
		bl.Env = Env{OS: r.Env.OS, MIMEResolvesMKV: mimeResolvesMKV}
		up.Reps = max(up.Reps, measured(r))
		bl.Scenarios[r.Scenario.Name] = expect(r)
	}
	bl.Updates = append(bl.Updates, up)
	return bl, nil
}

func measured(r *report.Report) int {
	n := 0
	for _, rep := range r.Reps {
		if !rep.Warmup {
			n++
		}
	}
	return n
}

// expect is a report's expectations.
func expect(r *report.Report) Scenario {
	sc := Scenario{Checks: map[string]string{}, Exact: map[string]float64{}}
	for _, c := range r.Checks {
		s := Status(c.Status, c.Known)
		// An intermittent issue (#195) that happened to fire is no
		// expectation: the gate tolerates it anywhere it may fire.
		if c.Status == report.XFail && intermittent(c.Known) {
			s = string(report.Pass)
		}
		sc.Checks[c.ID] = s
	}
	ok := 0
	for _, rep := range r.Reps {
		if !rep.Warmup && rep.Status == report.RepOK {
			ok++
		}
	}
	for name, m := range r.Metrics {
		// Only what every rep measured: the gate holds an absent one to
		// account.
		if m.Kind != report.Exact || len(m.Values) == 0 || len(m.Values) < ok {
			continue
		}
		if slices.Min(m.Values) != slices.Max(m.Values) {
			sc.Unstable = append(sc.Unstable, name)
			continue
		}
		sc.Exact[name] = m.Values[0]
	}
	slices.Sort(sc.Unstable)
	return sc
}

// Status is a check's status as the baseline writes it: the known issue
// after an xfail or an xpass.
func Status(s report.Status, known string) string {
	if known != "" && (s == report.XFail || s == report.XPass) {
		return string(s) + ":" + known
	}
	return string(s)
}
