package scenario_test

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/shapedtime/momoshtrem/internal/streamlab/observe"
	"github.com/shapedtime/momoshtrem/internal/streamlab/report"
	"github.com/shapedtime/momoshtrem/internal/streamlab/scenario"
)

// How the stall scenario pauses its seeder (holds its uploads).
type pauseMode int

const (
	// undeclared: Seeder.HoldUploads, a stall nothing declared, which the
	// watchdog must catch.
	undeclared pauseMode = iota
	// declared: Env.HoldSeeder, released after the pause.
	declared
	// forgotten: Env.HoldSeeder, never released: the script forgets its
	// fault.
	forgotten
)

// stallScenario plays a movie and pauses its seeder mid-play for pause:
// the player's small buffer runs dry and it stalls.
func stallScenario(mode pauseMode, pause time.Duration) *scenario.Scenario {
	return &scenario.Scenario{
		Name: "stall-injected",
		Tier: "test",
		Doc:  "A player plays; its seeder pauses mid-play, and the player stalls.",
		Params: []scenario.Param{
			{Name: "bitrate", Kind: scenario.Bytes, Default: "2MiB"},
			{Name: "seeder_rate", Kind: scenario.Bytes, Default: "2560KiB"},
			{Name: "max_buffer", Kind: scenario.Bytes, Default: "2MiB"},
			{Name: "playable_bytes", Kind: scenario.Bytes, Default: "1MiB"},
			{Name: "read_timeout", Kind: scenario.Duration, Default: "30s"},
			{Name: "watchdog", Kind: scenario.Duration, Default: "3s"},
			{Name: "settle", Kind: scenario.Duration, Default: "10s"},
			{Name: "retries", Kind: scenario.Int, Default: "0"},
		},
		Script: func(e *scenario.Env) error {
			t := e.Titles[0]
			dev := e.Device("infuse")
			p, err := e.Player(dev, t).Open(e.Ctx, e.Conn(dev), 0)
			if err := e.Live(err); err != nil {
				return err
			}
			if err := e.Live(p.Watch(3 * time.Second)); err != nil {
				return err
			}
			switch mode {
			case undeclared:
				t.Seeder.HoldUploads()
				timer := time.AfterFunc(pause, t.Seeder.ReleaseUploads)
				defer timer.Stop()
				defer t.Seeder.ReleaseUploads()
			case declared:
				release := e.HoldSeeder(t, "seeder-pause")
				timer := time.AfterFunc(pause, release)
				defer timer.Stop()
				defer release() // a no-op once the timer released it
			case forgotten:
				e.HoldSeeder(t, "seeder-pause")
			}
			err = e.Live(p.Watch(pause))
			p.Stop()
			return err
		},
	}
}

func runStall(t *testing.T, sc *scenario.Scenario, sets map[string]string) (*report.Rep, string) {
	t.Helper()
	if os.Getenv("STREAMLAB") == "" {
		t.Skip("set STREAMLAB=1 to run the injected stall")
	}
	dir := t.TempDir()
	if out := os.Getenv("STREAMLAB_OUT"); out != "" {
		// Keep the artifacts to look at.
		dir = filepath.Join(out, t.Name())
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	// The log capture, as the runner's child installs it. (Not over slog's
	// initial default handler: it writes through package log, which slog
	// then routes back to the new default.)
	logs := observe.NewCapture(slog.NewTextHandler(io.Discard, nil), false)
	prev := slog.Default()
	slog.SetDefault(slog.New(logs))
	defer slog.SetDefault(prev)
	rep, err := scenario.Run(ctx, sc, scenario.RunOptions{Rep: 1, Dir: dir, Logs: logs, Sets: sets})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	for _, c := range rep.Checks {
		if c.Status != report.Pass {
			t.Logf("check %s %s (%s): %.300s", c.ID, c.Status, c.Known, c.Evidence)
		}
	}
	for _, f := range rep.Findings {
		t.Logf("finding %s (%s): %s; %s", f.Rule, f.Known, f.Hint, f.Evidence)
	}
	if o := rep.Observe; o != nil {
		t.Logf("observation cost %.3f%% of %.0f ms CPU", o.OverheadPct, o.ProcessCPUMS)
	}
	return rep, dir
}

// stalled fails the test unless the player stalled: the scenario must
// produce what it tests.
func stalled(t *testing.T, rep *report.Rep) {
	t.Helper()
	stalls := 0
	for _, s := range rep.Sessions {
		if s.Player != nil {
			stalls += s.Player.Stalls
		}
	}
	if stalls == 0 {
		t.Fatalf("the pause didn't stall the player: the scenario must be retuned (sessions %+v)", rep.Sessions)
	}
}

func check(rep *report.Rep, id string) report.RepCheck {
	for _, c := range rep.Checks {
		if c.ID == id {
			return c
		}
	}
	return report.RepCheck{}
}

func rules(rep *report.Rep) []string {
	var out []string
	for _, f := range rep.Findings {
		out = append(out, f.Rule)
	}
	return out
}

// A seeder paused with nothing declared: the player stalls, the watchdog
// trips (no known issue explains a paused seeder) and dumps its
// artifacts, the timeline shows the stall, and the finding says the swarm
// sent less than the bitrate, not that the prioritizer failed.
func TestInjectedStall(t *testing.T) {
	rep, dir := runStall(t, stallScenario(undeclared, 8*time.Second), nil)
	stalled(t, rep)
	wd := check(rep, "watchdog")
	if wd.Status != report.Fail || wd.Known != "" {
		t.Errorf("watchdog = %+v, want a fail no known issue explains", wd)
	}
	if o := rep.Observe; o == nil || len(o.Trips) == 0 || !strings.Contains(o.Trips[0].Why, "uploads are held") {
		t.Errorf("trips = %+v, want one naming the held seeder", o)
	}
	for _, key := range []string{"timeline", "samples", "watchdog-1-goroutines", "watchdog-1-status", "watchdog-1-pieces", "watchdog-1-trace"} {
		name, ok := rep.Artifacts[key]
		if !ok {
			t.Errorf("no %s artifact: %v", key, rep.Artifacts)
			continue
		}
		if st, err := os.Stat(filepath.Join(dir, name)); err != nil || st.Size() == 0 {
			t.Errorf("%s: %v", name, err)
		}
	}
	if g, err := os.ReadFile(filepath.Join(dir, rep.Artifacts["watchdog-1-goroutines"])); err == nil && !strings.Contains(string(g), `"slreq"`) {
		t.Error("the goroutine dump carries no request labels")
	}
	got := rules(rep)
	if !slices.Contains(got, "starved") {
		t.Errorf("findings %v, want starved: the swarm sent less than the bitrate", got)
	}
	if slices.Contains(got, "stall-below-now") {
		t.Errorf("findings %v: the playhead's piece stayed at Now, the prioritizer isn't to blame", got)
	}
	tl, err := os.ReadFile(filepath.Join(dir, rep.Artifacts["timeline"]))
	if err != nil || !strings.Contains(string(tl), `"kind":"stall"`) || !strings.Contains(string(tl), `"kind":"trip"`) {
		t.Errorf("timeline (%v) lacks the stall or the trip", err)
	}
	if o := rep.Observe; o == nil || len(o.Timeline) == 0 || o.Timeline[0].Plot == "" || len(o.Samplers) == 0 {
		t.Errorf("observation = %+v, want a plotted timeline and the samplers' cost", o)
	}
}

// The same pause, declared as a fault on the title's torrent: the
// watchdog stays quiet, the finding is the same.
func TestInjectedStallDeclared(t *testing.T) {
	rep, _ := runStall(t, stallScenario(declared, 8*time.Second), nil)
	stalled(t, rep)
	if wd := check(rep, "watchdog"); wd.Status != report.Pass {
		t.Errorf("watchdog = %+v, want a pass during a declared fault", wd)
	}
	if o := rep.Observe; o == nil || len(o.Faults) != 1 || o.Faults[0].Name != "seeder-pause" || o.Faults[0].Torrent == "" {
		t.Errorf("faults = %+v, want the seeder's pause on its torrent", rep.Observe)
	}
	if got := rules(rep); !slices.Contains(got, "starved") || slices.Contains(got, "stall-below-now") {
		t.Errorf("findings %v, want starved and not stall-below-now", got)
	}
}

// A declared pause longer than the read timeout: the read times out and
// the body breaks with the seeder connected and the piece wanted, but the
// lab held the seeder, so it isn't #195: play-ok is a plain fail.
func TestHeldSeederStallIsNotKnown(t *testing.T) {
	rep, _ := runStall(t, stallScenario(declared, 14*time.Second), map[string]string{"read_timeout": "6s"})
	ok := check(rep, "play-ok")
	if ok.Status != report.Fail || ok.Known != "" || !strings.Contains(ok.Evidence, "seeder held") {
		t.Errorf("play-ok = %+v, want a plain fail naming the held seeder", ok)
	}
	if wd := check(rep, "watchdog"); wd.Status != report.Pass {
		t.Errorf("watchdog = %+v, want a pass during a declared fault", wd)
	}
}

// A pause the script never ends: the seeder is resumed for the teardown,
// and the watchdog check fails, since the fault hid the stall from it.
func TestForgottenFault(t *testing.T) {
	rep, _ := runStall(t, stallScenario(forgotten, 6*time.Second), nil)
	wd := check(rep, "watchdog")
	if wd.Status != report.Fail || wd.Known != "" || !strings.Contains(wd.Evidence, `fault "seeder-pause" still open`) {
		t.Errorf("watchdog = %+v, want a fail naming the open fault", wd)
	}
	if q := check(rep, "quiescence"); q.Status != report.Pass {
		t.Errorf("quiescence = %+v: the seeder wasn't resumed for the teardown?", q)
	}
}
