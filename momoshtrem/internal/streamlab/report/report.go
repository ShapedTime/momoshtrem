// Package report is streamlab's report (docs/streamlab/PLAN.md §6): one
// scenario's repetitions as streamlab.report/v1 JSON, and report.md that
// sums it up. A run's index lists every scenario's verdict.
//
// Metrics are exact, count or timing (PLAN D5): exact and count metrics
// are deterministic enough to gate on; timing metrics depend on the
// machine and get a verdict only from an A/B on one machine (S8, #179).
package report

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"time"
)

// Schema names the report's format. Bump the version on any change a
// reader must know about.
const Schema = "streamlab.report/v1"

// Kind is how a metric may be judged (PLAN §6 "Metric kinds").
type Kind string

const (
	// Exact metrics are deterministic: any change is reported.
	Exact Kind = "exact"
	// Count metrics are flagged past a noise floor.
	Count Kind = "count"
	// Timing metrics are judged only by an interleaved A/B.
	Timing Kind = "timing"
)

// Better says which way a metric improves.
type Better string

const (
	Lower  Better = "lower"
	Higher Better = "higher"
	// None: neither way is better; a change is only reported.
	None Better = "none"
)

// MetricDef describes a metric: its kind, unit and which way is better.
type MetricDef struct {
	Name   string `json:"name"`
	Kind   Kind   `json:"kind"`
	Unit   string `json:"unit"`
	Better Better `json:"better"`
	Doc    string `json:"doc"`
}

// Metric is a metric over the repetitions.
type Metric struct {
	Kind   Kind      `json:"kind"`
	Unit   string    `json:"unit"`
	Better Better    `json:"better"`
	Values []float64 `json:"values"`
	Median float64   `json:"median"`
	P95    float64   `json:"p95"`
}

// NewMetric aggregates the repetitions' values of def.
func NewMetric(def MetricDef, values []float64) Metric {
	return Metric{
		Kind: def.Kind, Unit: def.Unit, Better: def.Better,
		Values: values, Median: Quantile(values, 0.5), P95: Quantile(values, 0.95),
	}
}

// Quantile is the q-quantile of values, interpolated between the closest
// ranks; NaN without values.
func Quantile(values []float64, q float64) float64 {
	if len(values) == 0 {
		return math.NaN()
	}
	s := slices.Clone(values)
	slices.Sort(s)
	pos := q * float64(len(s)-1)
	lo := int(math.Floor(pos))
	hi := int(math.Ceil(pos))
	return s[lo] + (s[hi]-s[lo])*(pos-float64(lo))
}

// Status is a check's outcome (PLAN §6, D6).
type Status string

const (
	Pass Status = "pass"
	Fail Status = "fail"
	// XFail is a failure a known issue explains: it doesn't block.
	XFail Status = "xfail"
	// XPass is a check a known issue was expected to fail, passing: the
	// issue may be fixed.
	XPass Status = "xpass"
	// NA is a check that couldn't tell: its precondition didn't hold.
	NA Status = "n/a"
)

// Check is a check over the repetitions.
type Check struct {
	ID     string `json:"id"`
	Status Status `json:"status"`
	// Known is the known issue behind an xfail or xpass, or the one that
	// fired too often to stay an xfail.
	Known    string `json:"known,omitempty"`
	Evidence string `json:"evidence"`
	// Reps is each repetition's outcome: pass, fail, fail:<known>, n/a.
	Reps []string `json:"reps"`
}

// Report is one scenario's run: its repetitions and their verdict.
type Report struct {
	Schema   string            `json:"schema"`
	Tool     Tool              `json:"tool"`
	Env      Env               `json:"env"`
	Config   Config            `json:"config"`
	Scenario Scenario          `json:"scenario"`
	Run      Run               `json:"run"`
	Verdict  Verdict           `json:"verdict"`
	Metrics  map[string]Metric `json:"metrics"`
	Checks   []Check           `json:"checks"`
	Races    []Race            `json:"race_reports,omitempty"` // streamlab check's data races, by signature (race.go)
	Reps     []Rep             `json:"reps"`
	Findings []string          `json:"findings"`
	// Artifacts are paths relative to the report's folder.
	Artifacts []string `json:"artifacts"`
}

// Verdict is a scenario's: fail when a check fails or a repetition broke
// (timed out, crashed); error when the tool couldn't run it.
type Verdict string

const (
	VerdictPass  Verdict = "pass"
	VerdictFail  Verdict = "fail"
	VerdictError Verdict = "error"
)

// Tool is the build that ran.
type Tool struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Git     Git    `json:"git"`
}

// Git is the source tree's state.
type Git struct {
	Commit string `json:"commit,omitempty"`
	Branch string `json:"branch,omitempty"`
	Dirty  bool   `json:"dirty"`
}

// Env is the machine and the build: what timings depend on.
type Env struct {
	Host       string `json:"host"`
	Go         string `json:"go"`
	OS         string `json:"os"`
	Arch       string `json:"arch"`
	CPU        string `json:"cpu,omitempty"`
	NumCPU     int    `json:"num_cpu"`
	GOMAXPROCS int    `json:"gomaxprocs"`
	CGO        bool   `json:"cgo"`
	Race       bool   `json:"race"`
	Tags       string `json:"tags,omitempty"`
	// UTP is the uTP build (pure Go, or libutp through cgo); the lab runs
	// TCP only over loopback either way (PLAN D3).
	UTP string `json:"utp"`
	// MIME is what the MIME table says of .mkv and .mp4: "" when it
	// doesn't resolve them, as in production's image (F1).
	MIME map[string]string `json:"mime"`
	// Load is the 1, 5 and 15 minute load averages, when known.
	Load []float64 `json:"load,omitempty"`
}

// Config is the effective config's fingerprint and where it came from.
type Config struct {
	Fingerprint string `json:"fingerprint"`
	Path        string `json:"path,omitempty"`
}

// Scenario is what ran: the scenario, its effective params and seed.
type Scenario struct {
	Name   string            `json:"name"`
	Tier   string            `json:"tier"`
	Doc    string            `json:"doc"`
	Guards []string          `json:"guards"`
	Params map[string]string `json:"params"`
	Seed   uint64            `json:"seed"`
}

// Run is how it ran.
type Run struct {
	ID         string    `json:"id"`
	Reps       int       `json:"reps"`
	Warmup     int       `json:"warmup"`
	GOMAXPROCS int       `json:"gomaxprocs,omitempty"`
	TimeoutS   float64   `json:"timeout_s"`
	Started    time.Time `json:"started"`
	WallS      float64   `json:"wall_s"`
	Command    []string  `json:"command,omitempty"`
	// Confirmations are the repetitions added because an intermittent
	// known issue fired in too few to judge (known.ConfirmReps).
	Confirmations int `json:"confirmations,omitempty"`
}

// RepStatus is how a repetition's process ended.
type RepStatus string

const (
	// RepOK: the repetition ran and wrote its result.
	RepOK RepStatus = "ok"
	// RepTimeout: it outlived the hard timeout; its goroutines were dumped.
	RepTimeout RepStatus = "timeout"
	// RepCrash: it died (a panic) without a result.
	RepCrash RepStatus = "crash"
	// RepError: the tool or the environment failed it.
	RepError RepStatus = "error"
)

// Rep is one repetition, in its own process (PLAN D4).
type Rep struct {
	Rep    int  `json:"rep"`
	Warmup bool `json:"warmup,omitempty"`
	// Confirm marks a confirmation repetition (Run.Confirmations).
	Confirm bool      `json:"confirm,omitempty"`
	Status  RepStatus `json:"status"`
	Error   string    `json:"error,omitempty"`
	// Started and WallMS are the scenario's own: stack up to teardown.
	Started time.Time `json:"started"`
	WallMS  float64   `json:"wall_ms"`
	// Env and Tool are the child process's: the build and the machine
	// that ran the scenario, GOMAXPROCS as it ran.
	Env       *Env               `json:"env,omitempty"`
	Tool      *Tool              `json:"tool,omitempty"`
	Config    Config             `json:"config"`
	Params    map[string]string  `json:"params,omitempty"`
	Seed      uint64             `json:"seed"`
	Metrics   map[string]float64 `json:"metrics"`
	Checks    []RepCheck         `json:"checks"`
	Races     []Race             `json:"race_reports,omitempty"` // the process's data races (race.go)
	Notes     []string           `json:"notes,omitempty"`
	Sessions  []Session          `json:"sessions"`
	Torrent   Torrent            `json:"torrent"`
	Resources Resources          `json:"resources"`
	Artifacts map[string]string  `json:"artifacts,omitempty"`
	// Observe is the observer's view (observe.go); nil when it was off.
	// Findings are its hints, which never change a verdict.
	Observe  *Observation `json:"observe,omitempty"`
	Findings []Finding    `json:"findings,omitempty"`
}

// RepCheck is a check's outcome in one repetition: pass, fail or n/a.
// Known names the known issue whose signature a failure carries.
type RepCheck struct {
	ID       string `json:"id"`
	Status   Status `json:"status"`
	Known    string `json:"known,omitempty"`
	Evidence string `json:"evidence"`
}

// Session is one device's requests for one path, with the player's
// numbers when it played the file.
type Session struct {
	Device   string       `json:"device"`
	Path     string       `json:"path"`
	Player   *PlayerStats `json:"player,omitempty"`
	Requests []Request    `json:"requests"`
}

// PlayerStats is what the viewer felt (player.SessionStats).
type PlayerStats struct {
	TTFBMS           float64     `json:"ttfb_ms"`
	TimeToPlayableMS float64     `json:"time_to_playable_ms"`
	SeekLatenciesMS  []float64   `json:"seek_latencies_ms,omitempty"`
	Stalls           int         `json:"stalls"`
	StallTimeMS      float64     `json:"stall_time_ms"`
	RebufferRatio    float64     `json:"rebuffer_ratio"`
	OverRead         int64       `json:"over_read"`
	Received         int64       `json:"received"`
	Plays            []Play      `json:"plays"`
	ReadErrors       []ReadError `json:"read_errors,omitempty"`
	HTTPErrors       []string    `json:"http_errors,omitempty"`
}

// Play is one playback.
type Play struct {
	From             int64   `json:"from"`
	Seek             bool    `json:"seek,omitempty"`
	SeekLatencyMS    float64 `json:"seek_latency_ms,omitempty"`
	TTFBMS           float64 `json:"ttfb_ms"`
	TimeToPlayableMS float64 `json:"time_to_playable_ms"`
	Playable         bool    `json:"playable"`
	Stalls           []Stall `json:"stalls,omitempty"`
	StallTimeMS      float64 `json:"stall_time_ms"`
	PlayTimeMS       float64 `json:"play_time_ms"`
	PauseTimeMS      float64 `json:"pause_time_ms,omitempty"`
	Received         int64   `json:"received"`
	Consumed         int64   `json:"consumed"`
	OverRead         int64   `json:"over_read"`
	EOF              bool    `json:"eof,omitempty"`
	Requests         int     `json:"requests"`
	DurationMS       float64 `json:"duration_ms"`
	State            string  `json:"state"`
	Err              string  `json:"error,omitempty"`
}

// Stall is one stall of a playback.
type Stall struct {
	AtMS       float64 `json:"at_ms"`
	DurationMS float64 `json:"duration_ms"`
	Position   int64   `json:"position"`
	Open       bool    `json:"open,omitempty"`
}

// ReadError is a body that broke, and what the lab saw then.
type ReadError struct {
	AtMS   float64 `json:"at_ms"`
	Offset int64   `json:"offset"`
	Err    string  `json:"error"`
	// Diagnosis is the lab's look at the failure: a known stall, say.
	Diagnosis string `json:"diagnosis,omitempty"`
}

// Request is one HTTP request as the server saw it (stack.Request).
type Request struct {
	ID         string  `json:"id"`
	Method     string  `json:"method"`
	Path       string  `json:"path,omitempty"`
	Range      string  `json:"range,omitempty"`
	Status     int     `json:"status"`
	StartMS    float64 `json:"start_ms"`
	TTFBMS     float64 `json:"ttfb_ms"`
	Bytes      int64   `json:"bytes"`
	DurationMS float64 `json:"duration_ms"`
	Aborted    bool    `json:"aborted,omitempty"`
	Done       bool    `json:"done"`
	// HandlerLagMS is how long the handler ran after the client dropped
	// the request (F12); nil when the client didn't.
	HandlerLagMS *float64 `json:"handler_lag_ms,omitempty"`
}

// Torrent is what the leecher downloaded against what it served.
type Torrent struct {
	Downloaded int64 `json:"downloaded"`
	Served     int64 `json:"served"`
	// WasteRatio is Downloaded over Served.
	WasteRatio float64 `json:"waste_ratio"`
	// Residual is what downloaded after the last request closed, through
	// the quiescence wait and ResidualWindowMS more.
	Residual         int64   `json:"residual"`
	ResidualWindowMS float64 `json:"residual_window_ms"`
}

// Resources are the process's peaks while the scenario ran, and what the
// quiescence check found left behind.
type Resources struct {
	PeakGoroutines int    `json:"peak_goroutines"`
	PeakReaders    int    `json:"peak_readers"`
	PeakHeapBytes  uint64 `json:"peak_heap_bytes"`
	Leaked         []Leak `json:"leaked,omitempty"`
	// Inherited counts the labelled anacrolix goroutines a request
	// started that live with the torrent (not leaks).
	Inherited int `json:"inherited"`
}

// Leak is goroutines a request left running, by its label.
type Leak struct {
	Request string `json:"request"`
	Count   int    `json:"count"`
	Stack   string `json:"stack"`
}

// MS is d in milliseconds.
func MS(d time.Duration) float64 { return float64(d) / float64(time.Millisecond) }

// WriteJSON writes v to path, indented.
func WriteJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("report: encoding %s: %w", filepath.Base(path), err)
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

// ReadRep reads a repetition's rep.json.
func ReadRep(path string) (*Rep, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var r Rep
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, fmt.Errorf("report: %s: %w", path, err)
	}
	return &r, nil
}

// Write writes report.json and report.md to dir.
func (r *Report) Write(dir string) error {
	if err := WriteJSON(filepath.Join(dir, "report.json"), r); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "report.md"), []byte(r.Markdown()), 0o644)
}
