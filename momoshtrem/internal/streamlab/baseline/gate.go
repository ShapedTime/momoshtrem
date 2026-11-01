package baseline

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/shapedtime/momoshtrem/internal/streamlab/compare"
	"github.com/shapedtime/momoshtrem/internal/streamlab/known"
	"github.com/shapedtime/momoshtrem/internal/streamlab/report"
)

// GateSchema names gate.json's format.
const GateSchema = "streamlab.gate/v1"

// Gate is gate.json: a gate run held to the baseline.
type Gate struct {
	Schema string `json:"schema"`
	// Verdict is fail when a scenario failed or broke an expectation,
	// error when the tool couldn't run one.
	Verdict report.Verdict `json:"verdict"`
	Why     string         `json:"why"`
	// Baseline is the file, and Updated its last update.
	Baseline  string         `json:"baseline"`
	Updated   Update         `json:"updated"`
	Scenarios []GateScenario `json:"scenarios"`
	Notes     []string       `json:"notes,omitempty"`
}

// GateScenario is one scenario held to its expectations.
type GateScenario struct {
	Name    string         `json:"name"`
	Verdict report.Verdict `json:"verdict"`
	// Problems are what fail it: the run's failures and broken
	// expectations.
	Problems []string `json:"problems,omitempty"`
	// Notes are differences that don't: an intermittent issue that fired,
	// a check that couldn't tell this time, an expectation that depends on
	// the host.
	Notes []string `json:"notes,omitempty"`
}

// Hold holds a gate run's reports to the baseline. mimeResolvesMKV is
// this host's MIME table (checks.MIME).
func Hold(bl *Baseline, path string, reports []*report.Report, mimeResolvesMKV bool) *Gate {
	g := &Gate{Schema: GateSchema, Verdict: report.VerdictPass, Baseline: path}
	if n := len(bl.Updates); n > 0 {
		g.Updated = bl.Updates[n-1]
	}
	otherHost := bl.Env.MIMEResolvesMKV != mimeResolvesMKV
	if otherHost {
		g.Notes = append(g.Notes, fmt.Sprintf("The baseline was recorded where the MIME table %s .mkv, and this host's %s: expectations that depend on it (F1) are only noted.",
			resolves(bl.Env.MIMEResolvesMKV), resolves(mimeResolvesMKV)))
	}
	for _, r := range reports {
		gs := holdScenario(bl, r, otherHost)
		switch {
		case gs.Verdict == report.VerdictError && g.Verdict == report.VerdictPass:
			g.Verdict = report.VerdictError
		case gs.Verdict == report.VerdictFail:
			g.Verdict = report.VerdictFail
		}
		g.Scenarios = append(g.Scenarios, gs)
	}
	var why []string
	for _, gs := range g.Scenarios {
		if gs.Verdict != report.VerdictPass {
			why = append(why, gs.Name+": "+strings.Join(gs.Problems, "; "))
		}
	}
	if len(why) == 0 {
		why = append(why, "every scenario passed and held its baseline")
	}
	g.Why = strings.Join(why, " | ")
	return g
}

func resolves(b bool) string {
	if b {
		return "resolves"
	}
	return "doesn't resolve"
}

func holdScenario(bl *Baseline, r *report.Report, otherHost bool) GateScenario {
	gs := GateScenario{Name: r.Scenario.Name, Verdict: report.VerdictPass}
	problem := func(format string, args ...any) {
		gs.Problems = append(gs.Problems, fmt.Sprintf(format, args...))
		if gs.Verdict == report.VerdictPass {
			gs.Verdict = report.VerdictFail
		}
	}
	note := func(format string, args ...any) { gs.Notes = append(gs.Notes, fmt.Sprintf(format, args...)) }

	switch r.Verdict {
	case report.VerdictError:
		gs.Verdict = report.VerdictError
		gs.Problems = append(gs.Problems, "the tool couldn't run it")
		return gs
	case report.VerdictFail:
		var failing []string
		for _, c := range r.Checks {
			if c.Status == report.Fail {
				s := c.ID + " fail"
				if c.Known != "" {
					s += " (" + c.Known + " over its rate)"
				}
				failing = append(failing, s)
			}
		}
		for _, rep := range r.Reps {
			if rep.Status == report.RepTimeout || rep.Status == report.RepCrash {
				failing = append(failing, fmt.Sprintf("rep %d %s", rep.Rep, rep.Status))
			}
		}
		problem("the run failed: %s", strings.Join(failing, ", "))
	}
	exp, ok := bl.Scenarios[r.Scenario.Name]
	if !ok {
		note("not in the baseline: add it with streamlab baseline update --reason")
		return gs
	}
	hostDependent := func(statuses ...string) bool {
		if !otherHost {
			return false
		}
		for _, s := range statuses {
			for _, id := range known.IDs(knownOf(s)) {
				if is, ok := known.Lookup(id); ok && is.AppliesTo != nil {
					return true
				}
			}
		}
		return false
	}

	checks := map[string]report.Check{}
	for _, c := range r.Checks {
		checks[c.ID] = c
		e, ok := exp.Checks[c.ID]
		a := Status(c.Status, c.Known)
		switch {
		case !ok:
			note("%s isn't in the baseline (%s now)", c.ID, a)
		case c.Status == report.Fail:
			// The run's failure says it already.
		case e == a:
		case c.Status == report.XFail && intermittent(c.Known):
			note("%s: %s fired within its rate", c.ID, c.Known)
		case passOrNA(e) && passOrNA(a):
			if a == string(report.NA) {
				note("%s couldn't tell this time (n/a)", c.ID)
			}
		case c.Status == report.NA:
			// F3, F6, F11, F12, F13 and #232 have timing-dependent n/a
			// paths: a rep that couldn't tell breaks no expectation.
			note("%s couldn't tell this time (n/a), %s in the baseline", c.ID, e)
		case hostDependent(e, a):
			note("%s: %s here, %s in the baseline (another host)", c.ID, a, e)
		case strings.HasPrefix(e, string(report.XFail)+":") && c.Status == report.XPass:
			problem("%s: known issue %s appears fixed (xfail → xpass): remove it from the registry (internal/streamlab/known) "+
				"and update the baseline (streamlab baseline update --reason)", c.ID, knownOf(e))
		default:
			problem("%s: %s, expected %s", c.ID, a, e)
		}
	}
	// A scenario's checks are fixed (Scenario.AllChecks): one that didn't
	// run means the lab changed under the baseline.
	ids := make([]string, 0, len(exp.Checks))
	for id := range exp.Checks {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	for _, id := range ids {
		if _, ok := checks[id]; !ok {
			problem("%s (%s in the baseline) didn't run", id, exp.Checks[id])
		}
	}

	names := make([]string, 0, len(exp.Exact))
	for n := range exp.Exact {
		names = append(names, n)
	}
	slices.Sort(names)
	for _, n := range names {
		want := exp.Exact[n]
		m, ok := r.Metrics[n]
		if !ok {
			// The baseline holds only exact metrics every rep measured.
			problem("exact metric %s (%s in the baseline) wasn't measured", n, report.Num(want))
			continue
		}
		if len(m.Values) > 0 && slices.Min(m.Values) == want && slices.Max(m.Values) == want {
			continue
		}
		got := make([]string, len(m.Values))
		for i, v := range m.Values {
			got[i] = report.Num(v)
		}
		if id, ok := compare.ExactCheck(n); ok {
			if c, ok := checks[id]; ok && c.Status == report.XFail && intermittent(c.Known) {
				note("%s: %s, %s in the baseline; %s is an xfail (%s)", n, strings.Join(got, " "), report.Num(want), id, c.Known)
				continue
			}
			if c, ok := checks[id]; ok && hostDependent(Status(c.Status, c.Known), exp.Checks[id]) {
				note("%s: %s, %s in the baseline (another host)", n, strings.Join(got, " "), report.Num(want))
				continue
			}
		}
		problem("exact metric %s: %s, expected %s", n, strings.Join(got, " "), report.Num(want))
	}
	return gs
}

// knownOf is the known issue of an "xfail:<issue>" status.
func knownOf(s string) string {
	_, id, _ := strings.Cut(s, ":")
	return id
}

func passOrNA(s string) bool { return s == string(report.Pass) || s == string(report.NA) }

// intermittent reports whether every known issue in ids is intermittent.
func intermittent(ids string) bool { return compare.Intermittent(ids) }

// Write writes gate.json and gate.md to dir.
func (g *Gate) Write(dir string) error {
	if err := report.WriteJSON(filepath.Join(dir, "gate.json"), g); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "gate.md"), []byte(g.Markdown()), 0o644)
}

// Markdown is the gate for a reader, the verdict first.
func (g *Gate) Markdown() string {
	var b strings.Builder
	fmt.Fprintf(&b, "**%s**: %s\n\n# streamlab gate\n\n", strings.ToUpper(string(g.Verdict)), strings.Join(strings.Fields(g.Why), " "))
	fmt.Fprintf(&b, "- **Baseline:** %s, updated %s at %s: %s\n", g.Baseline, g.Updated.Date.Format("2006-01-02"),
		shortSHA(g.Updated.Commit), g.Updated.Reason)
	for _, n := range g.Notes {
		fmt.Fprintf(&b, "- %s\n", n)
	}
	b.WriteString("\n| Scenario | Verdict | Problems | Notes |\n|---|---|---|---|\n")
	for _, s := range g.Scenarios {
		fmt.Fprintf(&b, "| %s | %s | %s | %s |\n", s.Name, s.Verdict, cell(strings.Join(s.Problems, "; ")), cell(strings.Join(s.Notes, "; ")))
	}
	return b.String()
}

func shortSHA(c string) string {
	if len(c) > 12 {
		return c[:12]
	}
	return c
}

func cell(s string) string {
	s = strings.ReplaceAll(strings.Join(strings.Fields(s), " "), "|", "\\|")
	if s == "" {
		return "-"
	}
	return s
}
