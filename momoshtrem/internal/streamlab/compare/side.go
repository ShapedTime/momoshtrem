package compare

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/shapedtime/momoshtrem/internal/streamlab/report"
)

// Side is one side of a comparison: a run's reports, by scenario.
type Side struct {
	// Path is where it was read from.
	Path    string
	Reports []*report.Report
}

// Load reads a side: a run's folder (its index.json), an index.json, or
// one scenario's report.json.
func Load(path string) (*Side, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if fi.IsDir() {
		path = filepath.Join(path, "index.json")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var head struct {
		Schema string `json:"schema"`
	}
	if err := json.Unmarshal(b, &head); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	s := &Side{Path: path}
	switch head.Schema {
	case report.Schema:
		r, err := decodeReport(path, b)
		if err != nil {
			return nil, err
		}
		s.Reports = append(s.Reports, r)
	case report.IndexSchema:
		var ix report.Index
		if err := json.Unmarshal(b, &ix); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		for _, e := range ix.Scenarios {
			p := filepath.Join(filepath.Dir(path), e.Report)
			rb, err := os.ReadFile(p)
			if err != nil {
				return nil, fmt.Errorf("%s's report: %w", e.Name, err)
			}
			r, err := decodeReport(p, rb)
			if err != nil {
				return nil, err
			}
			s.Reports = append(s.Reports, r)
		}
		if len(s.Reports) == 0 {
			return nil, fmt.Errorf("%s: the run has no scenario", path)
		}
	default:
		return nil, fmt.Errorf("%s: schema %q is neither %s nor %s", path, head.Schema, report.Schema, report.IndexSchema)
	}
	return s, nil
}

func decodeReport(path string, b []byte) (*report.Report, error) {
	var r report.Report
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if r.Schema != report.Schema {
		return nil, fmt.Errorf("%s: schema %q, want %q", path, r.Schema, report.Schema)
	}
	return &r, nil
}

// report is the side's report of a scenario; nil when it didn't run it.
func (s *Side) report(name string) *report.Report {
	for _, r := range s.Reports {
		if r.Scenario.Name == name {
			return r
		}
	}
	return nil
}

// env is the side's machine and build: its first report's, which is its
// first measured repetition's child.
func (s *Side) env() report.Env { return s.Reports[0].Env }

// tool is the side's build: its first report's.
func (s *Side) tool() report.Tool { return s.Reports[0].Tool }

// race reports whether any of the side's repetitions ran a race build.
func (s *Side) race() bool {
	for _, r := range s.Reports {
		if r.Env.Race {
			return true
		}
		for _, rep := range r.Reps {
			if rep.Env != nil && rep.Env.Race {
				return true
			}
		}
	}
	return false
}

// machines are the machines the side's repetitions ran on.
func (s *Side) machines() []string {
	seen := map[string]bool{}
	var out []string
	add := func(e report.Env) {
		if m := Machine(e); !seen[m] {
			seen[m] = true
			out = append(out, m)
		}
	}
	for _, r := range s.Reports {
		add(r.Env)
		for _, rep := range r.Reps {
			if rep.Env != nil {
				add(*rep.Env)
			}
		}
	}
	return out
}

// loads is the 1-minute load average per CPU of every measured
// repetition that recorded one.
func (s *Side) loads() []float64 {
	var out []float64
	for _, r := range s.Reports {
		out = append(out, repLoads(r)...)
	}
	return out
}

// measuredReps are a report's measured repetitions (confirmations
// included), as the runner judged them.
func measuredReps(r *report.Report) []report.Rep {
	var out []report.Rep
	for _, rep := range r.Reps {
		if !rep.Warmup {
			out = append(out, rep)
		}
	}
	return out
}

// okReps is how many of a report's measured repetitions ran to the end:
// those whose metrics count.
func okReps(r *report.Report) int {
	n := 0
	for _, rep := range measuredReps(r) {
		if rep.Status == report.RepOK {
			n++
		}
	}
	return n
}

// repLoads is the 1-minute load average per CPU of each measured
// repetition of r, as its process saw it when it ended (report.Env.Load):
// the average over about the minute the scenario ran.
func repLoads(r *report.Report) []float64 {
	var out []float64
	for _, rep := range measuredReps(r) {
		if rep.Env == nil || len(rep.Env.Load) == 0 || rep.Env.NumCPU <= 0 {
			continue
		}
		out = append(out, rep.Env.Load[0]/float64(rep.Env.NumCPU))
	}
	return out
}
