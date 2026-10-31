package compare

import (
	"fmt"
	"math"

	"github.com/shapedtime/momoshtrem/internal/streamlab/known"
	"github.com/shapedtime/momoshtrem/internal/streamlab/report"
)

// The rules (docs/streamlab/PLAN.md §6 "Compare").
const (
	// MinTimingReps is how many measured repetitions each side needs
	// before a timing or count difference counts as a regression or an
	// improvement on statistics.
	MinTimingReps = 5
	// Alpha is the significance level of the Mann-Whitney U and Fisher
	// tests.
	Alpha = 0.05
	// TimingRel is the relative change a timing median must exceed.
	TimingRel = 0.15
	// CountRel is the relative change a count median must exceed, unless
	// 3×CV is larger.
	CountRel = 0.10
	// CVTimes is how many CVs of noise a count change must exceed.
	CVTimes = 3
)

// Load skew: the two sides ran under very different load when their
// median 1-minute load averages per CPU differ by more than LoadSkewAbs
// and by more than LoadSkewRatio times. A verdict on timings or counts is
// then inconclusive. HighLoad per CPU is noted: timings are noisier.
const (
	LoadSkewAbs   = 0.5
	LoadSkewRatio = 1.5
	HighLoad      = 1.0
	// loadFloor keeps the ratio sane near an idle machine.
	loadFloor = 0.1
)

// timingFloors are the absolute changes a timing median must exceed
// beside TimingRel, so a few milliseconds on a fast metric never count:
// loopback timings are small, and 15% of 20 ms is scheduler noise.
var timingFloors = map[string]float64{
	"ttfb":                    100,
	"time_to_playable":        200,
	"seek_latency_p50":        200,
	"seek_latency_p95":        300,
	"stall_count":             1,
	"stall_time":              500,
	"rebuffer_ratio":          0.02,
	"stall_time_after":        500,
	"next_time_to_playable":   200,
	"resume_ttfb":             200,
	"activation_wait":         100,
	"handler_lag_after_abort": 250,
	"propfind_latency":        50,
	"probe_latency":           100,
	"op_latency_max":          500,
	"wall_time":               1000,
}

// timingFloorByUnit is the floor of a timing metric without its own.
var timingFloorByUnit = map[string]float64{"ms": 100, "stalls": 1, "ratio": 0.02}

// countFloorByUnit are the absolute changes a count median must exceed
// beside CountRel and 3×CV: a median moving by one event (a stray read
// timeout) isn't a change. Units not listed take 1.
var countFloorByUnit = map[string]float64{
	"bytes":       1 << 20,
	"B/s":         64 << 10,
	"ratio":       0.05,
	"updates/MiB": 0.5,
}

// TimingFloor is a timing metric's absolute floor.
func TimingFloor(name, unit string) float64 {
	if f, ok := timingFloors[name]; ok {
		return f
	}
	if f, ok := timingFloorByUnit[unit]; ok {
		return f
	}
	return 0
}

// CountFloor is a count metric's absolute floor.
func CountFloor(unit string) float64 {
	if f, ok := countFloorByUnit[unit]; ok {
		return f
	}
	return 1
}

// survivors are metrics only the reps that got there record: a first play
// that never got playable has no time to playable. Measured on fewer of
// head's reps than base's, they compare head's best reps with all of
// base's, so a change there can't pass for the same.
var survivors = map[string]bool{
	"ttfb":                  true,
	"time_to_playable":      true,
	"next_time_to_playable": true,
	"resume_ttfb":           true,
	"seek_latency_p50":      true,
	"seek_latency_p95":      true,
}

// labOnly are metrics of the lab itself, not of the stack it runs: their
// changes are reported, never a verdict. observe_overhead is the
// observer's cost (S5 holds it to its 2% budget).
var labOnly = map[string]bool{
	"observe_overhead": true,
}

// correctness are the checks whose failure in head fails a compare
// whatever base did (PLAN §6): integrity errors, panics (http), races,
// watchdog trips and leaks (quiescence).
var correctness = map[string]bool{
	"integrity":     true,
	"http":          true,
	known.RaceCheck: true,
	"watchdog":      true,
	"quiescence":    true,
}

// exactChecks are the checks that judge each exact metric: a change a
// known issue explains there (head's check is an xfail) isn't a
// regression.
var exactChecks = map[string]string{
	"integrity_errors":            "integrity",
	"panics":                      "http",
	"leaked_goroutines":           "quiescence",
	"readers_left":                "quiescence",
	"requests_left":               "quiescence",
	"open_files_left":             "quiescence",
	"blocked_reads_left":          "quiescence",
	"sessions_left":               "quiescence",
	"cdn_bodies_left":             "quiescence",
	"streamer_slots_held":         "quiescence",
	"race_reports":                known.RaceCheck,
	"torrents_loaded_by_propfind": "propfind-no-load",
	"sniff_readers":               "no-sniff-reads",
	"etag_changes":                "etag-stable",
	"if_range_honoured":           "etag-stable",
	"link_resolves":               "link-refresh",
	"link_invalidations":          "link-refresh",
}

// Intermittent reports whether every known issue in ids (an Outcome's
// Known) fires on some reps only (#195, #225).
func Intermittent(ids string) bool {
	if ids == "" {
		return false
	}
	for _, id := range known.IDs(ids) {
		if is, ok := known.Lookup(id); !ok || !is.Intermittent() {
			return false
		}
	}
	return true
}

// ExactCheck is the check that judges an exact metric, when there's one.
func ExactCheck(metric string) (string, bool) {
	c, ok := exactChecks[metric]
	return c, ok
}

// loadSkew reports whether two sides' load medians (per CPU) differ a
// lot, and says how.
func loadSkew(base, head float64) (bool, string) {
	if math.IsNaN(base) || math.IsNaN(head) {
		return false, ""
	}
	lo, hi := math.Min(base, head), math.Max(base, head)
	if hi-lo > LoadSkewAbs && hi/math.Max(lo, loadFloor) > LoadSkewRatio {
		return true, fmt.Sprintf("the sides ran under very different load: %.2f vs %.2f per CPU (1-minute average, median over the reps)", base, head)
	}
	return false, ""
}

// worse reports whether going from base to head (delta = head - base) is
// worse for a metric with better, and whether there's a direction at all.
func worse(better report.Better, delta float64) (bad, directed bool) {
	switch better {
	case report.Lower:
		return delta > 0, true
	case report.Higher:
		return delta < 0, true
	}
	return false, false
}
