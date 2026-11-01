package compare

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/shapedtime/momoshtrem/internal/streamlab/report"
)

// Line is the verdict on one line: what compare and ab print, and
// compare.md's first line.
func (r *Result) Line() string {
	return fmt.Sprintf("%s: %s", strings.ToUpper(string(r.Verdict)), r.Why)
}

// Write writes compare.json and compare.md to dir.
func (r *Result) Write(dir string) error {
	if err := report.WriteJSON(filepath.Join(dir, "compare.json"), r); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "compare.md"), []byte(r.Markdown()), 0o644)
}

// Markdown is the comparison for a reader, the verdict line first: the
// sides, the conditions, a row per scenario, then what changed in each.
func (r *Result) Markdown() string {
	var b strings.Builder
	fmt.Fprintf(&b, "**%s**: %s\n\n", strings.ToUpper(string(r.Verdict)), strings.Join(strings.Fields(r.Why), " "))
	b.WriteString("# streamlab compare\n\n| | Base | Head |\n|---|---|---|\n")
	fmt.Fprintf(&b, "| Commit | %s | %s |\n", sideCommit(r.Base), sideCommit(r.Head))
	fmt.Fprintf(&b, "| Reps (most per scenario) | %d | %d |\n", r.Base.Reps, r.Head.Reps)
	fmt.Fprintf(&b, "| Load per CPU, min / median / max | %s | %s |\n", loadText(r.Base.Load), loadText(r.Head.Load))
	fmt.Fprintf(&b, "| Machine | %s | %s |\n", cell(r.Base.Machine), cell(r.Head.Machine))
	fmt.Fprintf(&b, "| Race build | %v | %v |\n", r.Base.Race, r.Head.Race)
	fmt.Fprintf(&b, "| Path | %s | %s |\n\n", cell(r.Base.Path), cell(r.Head.Path))
	fmt.Fprintf(&b, "- **Timings:** %s\n", judgedText(r.Timing))
	fmt.Fprintf(&b, "- **Counts:** %s\n", judgedText(r.Counts))
	noise := fmt.Sprintf("defaults (CV %.2f)", r.Noise.DefaultCV)
	if r.Noise.Calibrated != "" {
		noise = fmt.Sprintf("%s (calibrated %s)", r.Noise.Source, r.Noise.Calibrated)
	}
	fmt.Fprintf(&b, "- **Count noise:** %s\n", noise)
	if c := r.Confirmed; c != nil {
		fmt.Fprintf(&b, "- **Confirmed:** %d scenarios judged again on fresh reps (%d a side): %s\n",
			len(c.Scenarios), c.Head.Reps, orDash(strings.Join(c.Scenarios, ", ")))
	}
	for _, n := range r.Notes {
		fmt.Fprintf(&b, "- %s\n", n)
	}
	for _, e := range r.Errors {
		fmt.Fprintf(&b, "- **Error:** %s\n", e)
	}

	b.WriteString("\n## Scenarios\n\n| Scenario | Verdict | Why |\n|---|---|---|\n")
	for _, sc := range r.Scenarios {
		fmt.Fprintf(&b, "| %s | %s | %s |\n", sc.Name, sc.Verdict, cell(strings.Join(sc.Why, "; ")))
	}
	if is := fired(r.Intermittent); len(is) > 0 {
		b.WriteString("\n## Intermittent known issues over the run\n\n")
		issuesTable(&b, is)
	}
	for _, sc := range r.Scenarios {
		sc.markdown(&b)
	}
	return b.String()
}

// markdown writes what changed in a scenario: checks that moved, metrics
// past their threshold or changed, issues that fired, notes.
func (sc *Scenario) markdown(b *strings.Builder) {
	var checks []Check
	for _, c := range sc.Checks {
		if c.Base != c.Head || c.Verdict != Pass || c.Head == report.Fail {
			checks = append(checks, c)
		}
	}
	var metrics []Metric
	for _, m := range sc.Metrics {
		if m.Status != Same || m.Screen != "" {
			metrics = append(metrics, m)
		}
	}
	issues := fired(sc.Intermittent)
	if len(checks)+len(metrics)+len(issues)+len(sc.Notes)+len(sc.Broken)+len(sc.Errors) == 0 && sc.Again == nil {
		return
	}
	fmt.Fprintf(b, "\n## %s: %s\n\n", sc.Name, sc.Verdict)
	for _, x := range sc.Broken {
		fmt.Fprintf(b, "- **Broken:** %s\n", cell(x))
	}
	for _, x := range sc.Errors {
		fmt.Fprintf(b, "- **Error:** %s\n", cell(x))
	}
	if sc.Again != nil {
		fmt.Fprintf(b, "- **Confirmation run:** %s: %s\n", sc.Again.Verdict, cell(strings.Join(sc.Again.Why, "; ")))
	}
	for _, n := range sc.Notes {
		fmt.Fprintf(b, "- %s\n", n)
	}
	if len(checks) > 0 {
		b.WriteString("\n| Check | Base | Head | Verdict | Note |\n|---|---|---|---|---|\n")
		for _, c := range checks {
			fmt.Fprintf(b, "| %s | %s | %s | %s | %s |\n", c.ID, statusText(c.Base, c.BaseKnown), statusText(c.Head, c.HeadKnown), c.Verdict, cell(c.Note))
		}
	}
	if len(metrics) > 0 {
		b.WriteString("\n| Metric | Kind | Base median | Head median | Δ | p | Threshold | Status | Note |\n|---|---|---|---|---|---|---|---|---|\n")
		for _, m := range metrics {
			delta := signed(m.Delta)
			if m.DeltaPct != nil {
				delta += fmt.Sprintf(" (%+.0f%%)", *m.DeltaPct)
			}
			p := "-"
			if m.P != nil {
				p = PText(*m.P)
			}
			status := string(m.Status)
			if m.Screen != "" {
				status = fmt.Sprintf("%s (screen %s)", m.Status, m.Screen)
			}
			fmt.Fprintf(b, "| %s | %s | %s %s (n %d) | %s (n %d) | %s | %s | %s | %s | %s |\n",
				m.Name, m.Kind, report.Num(m.Base.Median), m.Unit, m.Base.N, report.Num(m.Head.Median), m.Head.N, delta, p,
				report.Num(m.Threshold), status, cell(m.Note))
		}
	}
	if len(issues) > 0 {
		b.WriteString("\n")
		issuesTable(b, issues)
	}
}

// PText formats a p-value: three decimals, "<0.001" below.
func PText(p float64) string {
	if p < 0.001 {
		return "<0.001"
	}
	return fmt.Sprintf("%.3f", p)
}

// fired are the issues that fired on a side.
func fired(issues []Issue) []Issue {
	var out []Issue
	for _, is := range issues {
		if is.HeadFired > 0 || is.BaseFired > 0 {
			out = append(out, is)
		}
	}
	return out
}

func issuesTable(b *strings.Builder, issues []Issue) {
	b.WriteString("| Issue | Base fired | Head fired | Fisher p (head more) | Verdict |\n|---|---|---|---|---|\n")
	for _, is := range issues {
		fmt.Fprintf(b, "| %s | %d of %d | %d of %d | %s | %s |\n", is.ID, is.BaseFired, is.BaseReps, is.HeadFired, is.HeadReps, PText(is.P), is.Verdict)
	}
}

func sideCommit(s SideInfo) string {
	c := shortCommit(s.Commit)
	if c == "" {
		c = "unknown"
	}
	if s.Dirty {
		c += " (dirty)"
	}
	if s.Branch != "" && s.Branch != "HEAD" {
		c += " on " + s.Branch
	}
	return cell(c)
}

func loadText(l LoadStats) string {
	if l.Reps == 0 {
		return "-"
	}
	return fmt.Sprintf("%.2f / %.2f / %.2f (%d reps)", l.Min, l.Median, l.Max, l.Reps)
}

func judgedText(j Judged) string {
	if j.Judged {
		return "judged"
	}
	return "not judged: " + j.Why
}

// cell makes s safe in a Markdown table cell: one line, no pipes.
func cell(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	s = strings.ReplaceAll(s, "|", "\\|")
	if r := []rune(s); len(r) > 600 {
		s = string(r[:600]) + "…"
	}
	return orDash(s)
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
