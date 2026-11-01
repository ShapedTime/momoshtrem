package compare

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/shapedtime/momoshtrem/internal/streamlab/report"
)

// NoiseSchema names noise.json's format.
const NoiseSchema = "streamlab.noise/v1"

// NoiseFile is noise.json's name in streamlab's folder (runner.Dir()):
// per machine, never committed (.streamlab is gitignored).
const NoiseFile = "noise.json"

// DefaultCV is the noise a count metric is assumed to have on a machine
// calibrate never measured: conservative, so an uncalibrated compare
// flags a count only past 3×10% = 30%.
const DefaultCV = 0.10

// Noise is noise.json: each machine's calibration.
type Noise struct {
	Schema string `json:"schema"`
	// Machines are by Machine(env).
	Machines map[string]*Calibration `json:"machines"`
}

// Calibration is one machine's noise: each scenario's metrics' CVs over
// calibrate's repetitions of one commit.
type Calibration struct {
	Machine string    `json:"machine"`
	Commit  string    `json:"commit"`
	Date    time.Time `json:"date"`
	Reps    int       `json:"reps"`
	// Load is the 1-minute load average per CPU over the repetitions:
	// min, median and max. Noise measured on an idle machine understates
	// a loaded one's.
	Load []float64 `json:"load_per_cpu,omitempty"`
	// CV is each scenario's metrics' coefficient of variation.
	CV map[string]map[string]float64 `json:"cv"`
}

// Machine is the machine a report ran on, as compare and noise.json key
// it: host, OS and architecture, CPU model and count. Timings from two
// machines can't be compared.
func Machine(e report.Env) string {
	return fmt.Sprintf("%s %s/%s %s x%d", e.Host, e.OS, e.Arch, e.CPU, e.NumCPU)
}

// NoisePath is noise.json in dir (runner.Dir()).
func NoisePath(dir string) string { return filepath.Join(dir, NoiseFile) }

// ReadNoise reads noise.json; an empty one when it doesn't exist.
func ReadNoise(path string) (*Noise, error) {
	n := &Noise{Schema: NoiseSchema, Machines: map[string]*Calibration{}}
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return n, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, n); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if n.Schema != NoiseSchema {
		return nil, fmt.Errorf("%s: schema %q, want %q", path, n.Schema, NoiseSchema)
	}
	if n.Machines == nil {
		n.Machines = map[string]*Calibration{}
	}
	return n, nil
}

// Write writes noise.json.
func (n *Noise) Write(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return report.WriteJSON(path, n)
}

// Calibrate measures the noise of reports, every scenario's measured
// repetitions on one machine, into a calibration: each metric's CV. Only
// scenarios that passed count; a failing one's numbers measure the
// failure, not the noise.
func Calibrate(reports []*report.Report, commit string, now time.Time) (*Calibration, error) {
	c := &Calibration{Commit: commit, Date: now.UTC(), CV: map[string]map[string]float64{}}
	var loads []float64
	for _, r := range reports {
		if r.Verdict != report.VerdictPass {
			continue
		}
		m := Machine(r.Env)
		if c.Machine == "" {
			c.Machine = m
		} else if c.Machine != m {
			return nil, fmt.Errorf("calibrate: reports from two machines (%s, %s)", c.Machine, m)
		}
		cvs := map[string]float64{}
		for name, mt := range r.Metrics {
			if len(mt.Values) >= 2 {
				cvs[name] = CV(mt.Values)
			}
		}
		c.CV[r.Scenario.Name] = cvs
		c.Reps = max(c.Reps, len(measuredReps(r)))
		loads = append(loads, repLoads(r)...)
	}
	if c.Machine == "" {
		return nil, errors.New("calibrate: no scenario passed, so there is no noise to measure")
	}
	if len(loads) > 0 {
		c.Load = []float64{slices.Min(loads), report.Quantile(loads, 0.5), slices.Max(loads)}
	}
	return c, nil
}

// CVFor is a scenario's metric's calibrated CV, and whether calibrate
// measured it.
func (c *Calibration) CVFor(scenario, metric string) (float64, bool) {
	if c == nil {
		return 0, false
	}
	v, ok := c.CV[scenario][metric]
	return v, ok
}
