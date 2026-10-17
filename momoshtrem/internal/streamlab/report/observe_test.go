package report

import (
	"strings"
	"testing"
)

// The report lists each finding once with the repetitions that found it,
// and the last good repetition's observation.
func TestObservationReport(t *testing.T) {
	f := Finding{Rule: "starved", Hint: "expected: the swarm sent less than the bitrate", Evidence: "infuse stalled 2s", AtMS: 1200,
		Refs: []string{"timeline.ndjson"}}
	rep := func(n int, fs ...Finding) Rep {
		return Rep{Rep: n, Status: RepOK, Findings: fs, Observe: &Observation{
			OverheadPct: 0.4, ProcessCPUMS: 3000,
			Samplers:    []SamplerStat{{Name: "readers", EveryMS: 250, Samples: 100, BusyMS: 1.5, CPUPct: 0.05, ClientLock: true}},
			Timeline:    []TitleTimeline{{Title: "Film (2020)", Pieces: 192, Completed: 150, Plot: "timeline-film-2020.svg"}},
			Mutex:       []ContentionSite{{Site: "x.(*Cache).Remove", Count: 10, DelayMS: 9}},
			Logs:        LogSummary{Errors: 4, BenignStorageErrors: 2},
			Trips:       []Trip{{AtMS: 5000, ForMS: 3000, Why: "not #195's signature: the seeder is paused", Artifacts: []string{"watchdog-1-goroutines.txt"}}},
		}}
	}
	reps := []Rep{rep(1, f), rep(2, f, Finding{Rule: "handler-lag", Known: "F12", Hint: "a handler outlived its client's drop"})}
	lines := FindingLines(reps)
	if len(lines) != 2 || lines[0] != "starved: expected: the swarm sent less than the bitrate (rep 1, 2)" {
		t.Errorf("lines = %q", lines)
	}
	r := &Report{Scenario: Scenario{Name: "steady-play"}, Verdict: VerdictPass, Reps: reps, Findings: lines}
	md := r.Markdown()
	for _, want := range []string{"## Findings", "(rep 1, 2)", "## Observation (rep 2, measure mode)", "0.40% of the process's CPU",
		"| readers | 250 |", "timeline-film-2020.svg", "x.(*Cache).Remove", "2 benign storage errors", "### Watchdog",
		"watchdog-1-goroutines.txt", "**handler-lag** (F12)"} {
		if !strings.Contains(md, want) {
			t.Errorf("report.md lacks %q", want)
		}
	}
}
