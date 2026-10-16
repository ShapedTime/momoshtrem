package report

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// IndexSchema names a run's index format.
const IndexSchema = "streamlab.index/v1"

// Index lists a run's scenarios and their verdicts: index.json and
// index.md in the run's folder.
type Index struct {
	Schema    string       `json:"schema"`
	Tool      Tool         `json:"tool"`
	Env       Env          `json:"env"`
	Run       Run          `json:"run"`
	Verdict   Verdict      `json:"verdict"`
	Scenarios []IndexEntry `json:"scenarios"`
}

// IndexEntry is one scenario of a run.
type IndexEntry struct {
	Name    string  `json:"name"`
	Verdict Verdict `json:"verdict"`
	// WallS is the scenario's wall time over its repetitions, warm-up
	// included, process start to exit.
	WallS float64 `json:"wall_s"`
	// Checks are the checks that didn't pass, as id=status[:known].
	Checks []string `json:"checks"`
	// Key are the scenario's key metrics' medians.
	Key map[string]float64 `json:"key"`
	// KeyOrder is the order to show Key in.
	KeyOrder []string `json:"key_order"`
	Error    string   `json:"error,omitempty"`
	// Report is the scenario's report.json, relative to the index.
	Report string `json:"report"`
}

// NewIndexEntry sums r up; key are the metrics to show, in order.
func NewIndexEntry(r *Report, wallS float64, key []string, reportPath string) IndexEntry {
	e := IndexEntry{Name: r.Scenario.Name, Verdict: r.Verdict, WallS: wallS, Key: map[string]float64{}, Report: reportPath}
	for _, c := range r.Checks {
		if c.Status == Pass {
			continue
		}
		s := c.ID + "=" + string(c.Status)
		if c.Known != "" {
			s += ":" + c.Known
		}
		e.Checks = append(e.Checks, s)
	}
	for _, k := range key {
		if m, ok := r.Metrics[k]; ok {
			e.Key[k] = m.Median
			e.KeyOrder = append(e.KeyOrder, k)
		}
	}
	for _, rep := range r.Reps {
		if rep.Error != "" && rep.Status != RepOK {
			e.Error = rep.Error
		}
	}
	return e
}

// Write writes index.json and index.md to dir.
func (ix *Index) Write(dir string) error {
	if err := WriteJSON(filepath.Join(dir, "index.json"), ix); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "index.md"), []byte(ix.Markdown()), 0o644)
}

// Markdown is the run as a table: a row per scenario.
func (ix *Index) Markdown() string {
	var b strings.Builder
	fmt.Fprintf(&b, "# streamlab run %s: %s\n\n", ix.Run.ID, strings.ToUpper(string(ix.Verdict)))
	fmt.Fprintf(&b, "- **Tool:** %s %s, commit %s%s\n", ix.Tool.Name, ix.Tool.Version, orDash(short(ix.Tool.Git.Commit)), dirty(ix.Tool.Git.Dirty))
	fmt.Fprintf(&b, "- **Env:** %s\n", ix.Env.Line())
	fmt.Fprintf(&b, "- **Reps:** %d per scenario (+%d warm-up discarded), %.0f s in all\n", ix.Run.Reps, ix.Run.Warmup, ix.Run.WallS)
	if len(ix.Run.Command) > 0 {
		fmt.Fprintf(&b, "- **Command:** `%s`\n", strings.Join(ix.Run.Command, " "))
	}
	b.WriteString("\n| Scenario | Verdict | Not passing | Key metrics (median) | Wall |\n|---|---|---|---|---|\n")
	for _, e := range ix.Scenarios {
		var key []string
		for _, k := range e.KeyOrder {
			key = append(key, fmt.Sprintf("%s %s", k, Num(e.Key[k])))
		}
		checks := strings.Join(e.Checks, ", ")
		if e.Error != "" {
			checks = strings.TrimSpace(checks + " " + e.Error)
		}
		fmt.Fprintf(&b, "| [%s](%s) | %s | %s | %s | %.0f s |\n",
			e.Name, strings.TrimSuffix(e.Report, ".json")+".md", e.Verdict, cell(checks), cell(strings.Join(key, ", ")), e.WallS)
	}
	return b.String()
}
