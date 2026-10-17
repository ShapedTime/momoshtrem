package scenario

import (
	"strings"
	"testing"
	"time"

	"github.com/shapedtime/momoshtrem/internal/streamlab/report"
)

// F11's forward-play variant: a quick break at a piece that was never
// complete carries F11's signature only with the log capture's proof.
func TestF11NeedsLogProof(t *testing.T) {
	title := &Title{Hash: "aa"}
	partial := failure{title: title, piece: 40, silence: 2 * time.Millisecond, broke: true, evicted: true}
	if got := partial.known(); got != "" {
		t.Errorf("a partial piece's break without proof: %q, want no known issue", got)
	}
	proved := partial
	proved.resyncFailed = true
	if got := proved.known(); got != "F11" {
		t.Errorf("with the proof: %q, want F11", got)
	}
	if d := proved.diagnosis(); !strings.Contains(d, "completion resync") {
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

// The watchdog's check: #195 only when every trip carries its signature,
// on at most two pieces between them.
func TestWatchdogOutcome(t *testing.T) {
	stuck := func(piece int, known string) report.Trip {
		return report.Trip{AtMS: 1000, ForMS: 5000, Known: known, Why: "why",
			Readers: []report.TripReader{{Title: "Film", Piece: piece, State: "now"}, {Title: "Film", Piece: 90, State: "complete"}}}
	}
	tests := []struct {
		name   string
		trips  []report.Trip
		status report.Status
		known  string
	}{
		{"no trip", nil, report.Pass, ""},
		{"#195's signature", []report.Trip{stuck(3, "#195")}, report.Fail, "#195"},
		{"twice on one piece", []report.Trip{stuck(3, "#195"), stuck(3, "#195")}, report.Fail, "#195"},
		{"on three pieces", []report.Trip{stuck(1, "#195"), stuck(2, "#195"), stuck(3, "#195")}, report.Fail, ""},
		{"one without it", []report.Trip{stuck(3, "#195"), stuck(4, "")}, report.Fail, ""},
	}
	for _, tt := range tests {
		status, known, ev := watchdogOutcome(tt.trips, 5*time.Second)
		if status != tt.status || known != tt.known || ev == "" {
			t.Errorf("%s: %s %q (%s), want %s %q", tt.name, status, known, ev, tt.status, tt.known)
		}
	}
}
