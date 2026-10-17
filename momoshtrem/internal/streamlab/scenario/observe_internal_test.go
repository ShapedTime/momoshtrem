package scenario

import (
	"strings"
	"testing"
	"time"

	"github.com/shapedtime/momoshtrem/internal/streamlab/report"
)

// F11's forward-play variant: a quick break at a piece that was never
// complete carries F11's signature only with the log capture's proof and
// its written chunk gone from the cache.
func TestF11NeedsLogProof(t *testing.T) {
	title := &Title{Hash: "aa"}
	partial := failure{title: title, piece: 40, silence: 2 * time.Millisecond, broke: true, evicted: true}
	if got := partial.known(); got != "" {
		t.Errorf("a partial piece's break without proof: %q, want no known issue", got)
	}
	logOnly := partial
	logOnly.resyncFailed = true
	if got := logOnly.known(); got != "" {
		t.Errorf("the log proof with the chunk still cached: %q, want no known issue", got)
	}
	goneOnly := partial
	goneOnly.chunkGone = true
	if got := goneOnly.known(); got != "" {
		t.Errorf("the chunk gone without the log proof: %q, want no known issue", got)
	}
	proved := partial
	proved.resyncFailed, proved.chunkGone = true, true
	if got := proved.known(); got != "F11" {
		t.Errorf("with the proof: %q, want F11", got)
	}
	if d := proved.diagnosis(); !strings.Contains(d, "completion resync") || !strings.Contains(d, "evicted") {
		t.Errorf("diagnosis %q doesn't name the proof", d)
	}
	slow := proved
	slow.silence = quickBreak + time.Second
	if got := slow.known(); got != "" {
		t.Errorf("a slow break with the proof: %q, want no known issue", got)
	}
	if got := playKnown([]failure{proved, proved}, 0); got != "F11" {
		t.Errorf("play-ok over proved breaks: %q", got)
	}
	if got := playKnown([]failure{proved, partial}, 0); got != "" {
		t.Errorf("play-ok with one unproved break: %q, want unexplained", got)
	}
}

// A read that timed out while the lab held the seeder, or during a fault
// the script declared, isn't #195: failed() leaves stalled false then, and
// the diagnosis says why.
func TestHeldSeederIsNotKnown(t *testing.T) {
	f := failure{title: &Title{Hash: "aa"}, piece: 3, silence: 8 * time.Second, timedOut: true, broke: true, seederHeld: true, faulted: true}
	if got := f.known(); got != "" {
		t.Errorf("known = %q, want none", got)
	}
	if d := f.diagnosis(); !strings.Contains(d, "seeder held") || !strings.Contains(d, "declared") {
		t.Errorf("diagnosis %q doesn't name the hold and the fault", d)
	}
}

// The watchdog's check: #195 only when every trip carries its signature
// and lasted #195's threshold, on at most two pieces between them; a
// fault left open fails it.
func TestWatchdogOutcome(t *testing.T) {
	stuck := func(piece int, known string) report.Trip {
		return report.Trip{AtMS: 1000, ForMS: 5000, Known: known, Why: "why",
			Readers: []report.TripReader{{Title: "Film", Piece: piece, State: "now"}, {Title: "Film", Piece: 90, State: "complete"}}}
	}
	short := stuck(3, "#195")
	short.ForMS = 2500
	tests := []struct {
		name   string
		trips  []report.Trip
		open   []string
		status report.Status
		known  string
	}{
		{"no trip", nil, nil, report.Pass, ""},
		{"#195's signature", []report.Trip{stuck(3, "#195")}, nil, report.Fail, "#195"},
		{"twice on one piece", []report.Trip{stuck(3, "#195"), stuck(3, "#195")}, nil, report.Fail, "#195"},
		{"on three pieces", []report.Trip{stuck(1, "#195"), stuck(2, "#195"), stuck(3, "#195")}, nil, report.Fail, ""},
		{"one without it", []report.Trip{stuck(3, "#195"), stuck(4, "")}, nil, report.Fail, ""},
		{"shorter than #195's threshold", []report.Trip{short}, nil, report.Fail, ""},
		{"a fault left open", nil, []string{"seeder-pause"}, report.Fail, ""},
		{"a fault left open hides no trip", []report.Trip{stuck(3, "#195")}, []string{"x"}, report.Fail, ""},
	}
	for _, tt := range tests {
		status, known, ev := watchdogOutcome(tt.trips, tt.open, 2*time.Second, 5*time.Second)
		if status != tt.status || known != tt.known || ev == "" {
			t.Errorf("%s: %s %q (%s), want %s %q", tt.name, status, known, ev, tt.status, tt.known)
		}
		if len(tt.open) > 0 && !strings.Contains(ev, "still open when the script ended") {
			t.Errorf("%s: evidence %q", tt.name, ev)
		}
	}
}
