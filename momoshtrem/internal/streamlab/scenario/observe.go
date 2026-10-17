package scenario

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	atorrent "github.com/anacrolix/torrent"

	"github.com/shapedtime/momoshtrem/internal/streamlab/observe"
	"github.com/shapedtime/momoshtrem/internal/streamlab/report"
)

// Observation (internal/streamlab/observe): every repetition is observed
// unless --set observe=false. These are its params, check and metrics;
// they join the common lists here, next to the code that reports them.
func init() {
	CommonParams = append(CommonParams,
		Param{Name: "observe", Kind: Bool, Default: "true", Doc: "Observe the repetition: samplers, the piece timeline, profiles, the watchdog and findings (observe). false runs it unobserved, for comparing timings."},
		Param{Name: "watchdog", Kind: Duration, Default: "0s", Doc: "How long without progress (nothing read or downloaded while a read is blocked and no fault runs) trips the watchdog; 0 is half the read timeout, #195's threshold."},
	)
	CommonChecks = append(CommonChecks, "watchdog")
	Checks = append(Checks, CheckDef{ID: "watchdog", Doc: "The watchdog never tripped, and the script ended every fault it declared: while a title's requests were active and a read blocked, with no fault declared for its torrent, something was read or downloaded at least every watchdog param (half the read timeout by default). A trip dumps the labelled goroutines, a flight recorder snapshot, both clients' status and the pieces around each reader. #195 explains a trip that carries its signature, measured at the trip: every stuck piece wanted and incomplete, its torrent active with a seeder connected that the lab didn't pause, nothing progressing for half the read timeout or more, on at most two pieces. A fault still open when the script ends fails the check. n/a with observe=false."})
	Metrics = append(Metrics,
		report.MetricDef{Name: observe.MetricWatchdogTrips, Kind: report.Count, Unit: "trips", Better: report.Lower, Doc: "Watchdog trips: stretches without progress while requests were active and a read blocked, no fault declared."},
		report.MetricDef{Name: observe.MetricBenignStorageErrors, Kind: report.Count, Unit: "reads", Better: report.Lower, Doc: "anacrolix reads that logged at ERROR (\"initial read failed\") and then succeeded on a retry: #219's benign filecache noise."},
		report.MetricDef{Name: observe.MetricStorageReadFailures, Kind: report.Count, Unit: "reads", Better: report.Lower, Doc: "anacrolix reads that gave up after resyncing the piece's completion (\"read failed after completion resync\"): data the piece cache lost (F11)."},
		report.MetricDef{Name: observe.MetricLogErrors, Kind: report.Count, Unit: "records", Better: report.Lower, Doc: "ERROR log records, momoshtrem's and anacrolix's."},
		report.MetricDef{Name: observe.MetricLogWarnings, Kind: report.Count, Unit: "records", Better: report.Lower, Doc: "WARN log records, momoshtrem's and anacrolix's."},
		report.MetricDef{Name: observe.MetricOverhead, Kind: report.Timing, Unit: "%", Better: report.Lower, Doc: "The observer's samplers, piece mirror and log capture: their busy time over the process's CPU time. The budget is 2%. The mutex and block profiles' cost inside the runtime isn't in it: an A/B against observe=false measures that."},
	)
}

// Fault declares a fault starting now that starves t's torrent (a paused
// seeder, a dropped link), or every torrent when t is nil: the watchdog
// doesn't trip for that torrent while it runs. Call the returned func
// when it ends; one still running when the script ends fails the
// watchdog check (the script forgot it, and a forgotten fault would hide
// a hang).
func (e *Env) Fault(name string, t *Title) (end func()) {
	hash := ""
	if t != nil {
		hash = t.Hash
	}
	return e.obs.Fault(name, hash)
}

// PauseSeeder pauses t's seeder (swarm.Seeder.Pause) as a declared fault,
// until the returned func resumes it; calling it again does nothing. A
// pause the script doesn't resume is resumed when the script ends.
func (e *Env) PauseSeeder(t *Title) (resume func()) {
	end := e.Fault("seeder-pause", t)
	t.Seeder.Pause()
	var once sync.Once
	resume = func() {
		once.Do(func() {
			t.Seeder.Resume()
			end()
		})
	}
	e.faults.mu.Lock()
	e.faults.resumes = append(e.faults.resumes, resume)
	e.faults.mu.Unlock()
	return resume
}

// envFaults are the faults a script injected through the Env.
type envFaults struct {
	mu      sync.Mutex
	resumes []func() // PauseSeeder's
	// open are the faults still running when the script ended.
	open []string
}

// endFaults, once the script has ended, notes the faults it left running,
// ends them and resumes the seeders it paused, so the teardown and the
// quiescence check run on a healthy swarm.
func (e *Env) endFaults() {
	open := e.obs.EndFaults()
	e.faults.mu.Lock()
	resumes := e.faults.resumes
	e.faults.open = open
	e.faults.mu.Unlock()
	for _, r := range resumes {
		r()
	}
}

// chunkCached reports whether the piece cache still holds the written
// (dirty) chunk of an incomplete piece at the torrent offset off:
// anacrolix's resource storage keeps it at
// cache/incompleted/<piece hash>/<chunk offset in the piece>.
func (e *Env) chunkCached(tor *atorrent.Torrent, off int64) bool {
	info := tor.Info()
	if info == nil || off < 0 || info.PieceLength <= 0 {
		return false
	}
	i := int(off / info.PieceLength)
	if i >= info.NumPieces() {
		return false
	}
	h := info.Piece(i).V1Hash()
	if !h.Ok {
		return false
	}
	chunk := (off - int64(i)*info.PieceLength) &^ (chunkSize - 1)
	_, err := os.Stat(filepath.Join(e.Stack.Config.Torrent.MetadataFolder, "cache", "incompleted", h.Value.HexString(),
		strconv.FormatInt(chunk, 10)))
	return err == nil
}

// chunkSize is anacrolix's chunk size (its default, which the leecher's
// config keeps): the unit a piece's dirty data is written in.
const chunkSize = 16 << 10

// startObserver observes the repetition, unless the observe param is
// off.
func (e *Env) startObserver(opts RunOptions) {
	if !e.P.Bool("observe") {
		return
	}
	titles := make([]observe.Title, len(e.Titles))
	for i, t := range e.Titles {
		titles[i] = observe.Title{Name: t.Name, Path: t.Path, Hash: t.Hash, Offset: t.Offset, Size: t.Size,
			PieceLength: t.Layout.PieceLength(), Seeder: t.Seeder}
	}
	lagMax := time.Second
	if v, ok := e.P.Values()["abort_lag_max"]; ok {
		if d, err := time.ParseDuration(v); err == nil {
			lagMax = d
		}
	}
	e.obs = observe.Start(observe.Config{Stack: e.Stack, Titles: titles, Dir: e.dir, Logs: opts.Logs,
		Diagnose: opts.Diagnose, Watchdog: e.watchdogAfter(), KnownStall: e.knownStall(),
		Bitrate: e.P.Bytes("bitrate"), AbortLagMax: lagMax})
}

// knownStall is #195's threshold: a body silent this long, a trip this
// long without progress (half the read timeout).
func (e *Env) knownStall() time.Duration { return e.P.Duration("read_timeout") / 2 }

// watchdogAfter is the watchdog param, half the read timeout when 0.
func (e *Env) watchdogAfter() time.Duration {
	if d := e.P.Duration("watchdog"); d > 0 {
		return d
	}
	return e.P.Duration("read_timeout") / 2
}

// observation finishes the observer: the repetition's observation,
// findings and artifacts, its metrics, and the watchdog check.
func (c *collector) observation() {
	e := c.e
	if e.obs == nil {
		c.rep.Checks = append(c.rep.Checks, report.RepCheck{ID: "watchdog", Status: report.NA, Evidence: "observe=false: not watched"})
		return
	}
	res := e.obs.Finish(c.facts())
	c.rep.Observe, c.rep.Findings = res.Observation, res.Findings
	for k, v := range res.Artifacts {
		e.rec.artifacts[k] = v
	}
	for k, v := range res.Metrics {
		c.set(k, v)
	}
	status, known, ev := watchdogOutcome(res.Observation.Trips, e.faults.open, e.watchdogAfter(), e.knownStall())
	c.rep.Checks = append(c.rep.Checks, report.RepCheck{ID: "watchdog", Status: status, Known: known, Evidence: ev})
}

// facts are the players' stalls and the handlers' lags, for the findings.
func (c *collector) facts() observe.Facts {
	var f observe.Facts
	for _, pr := range c.players {
		ti := slices.Index(c.e.Titles, pr.title)
		if ti < 0 {
			continue
		}
		for _, p := range pr.stats.Plays {
			for _, s := range p.Stalls {
				start := p.Start.Add(s.At)
				f.Stalls = append(f.Stalls, observe.Stall{Device: pr.device, Title: ti, Start: start,
					End: start.Add(s.Duration), Position: s.Position})
			}
		}
	}
	for _, l := range c.lags {
		f.Lags = append(f.Lags, observe.Lag{Request: l.reqID, Lag: l.lag, Done: l.done})
	}
	return f
}

// watchdogOutcome judges the watchdog: a fault the script left running is
// a fail (it would have hidden any hang after it); no trip passes; trips
// that all carry #195's signature, each a stall of #195's threshold
// (knownStall, half the read timeout) or more, on at most two pieces
// between them (what one wedged connection holds, as play-ok's
// signature), fail with #195; any other trip is a plain fail.
func watchdogOutcome(trips []report.Trip, open []string, after, knownStall time.Duration) (report.Status, string, string) {
	if len(open) > 0 {
		return report.Fail, "", fmt.Sprintf("fault %q still open when the script ended: the watchdog was off for its torrent from then on",
			strings.Join(open, `", "`))
	}
	if len(trips) == 0 {
		return report.Pass, "", fmt.Sprintf("never %v without progress while a read was blocked", after)
	}
	known := "#195"
	pieces := map[string]bool{}
	var lines []string
	for _, t := range trips {
		if t.Known != "#195" || t.ForMS < report.MS(knownStall) {
			known = ""
		}
		for _, r := range t.Readers {
			if r.State != "complete" {
				pieces[fmt.Sprintf("%s/%d", r.Title, r.Piece)] = true
			}
		}
		lines = append(lines, fmt.Sprintf("at %s ms, %s ms without progress: %s (%s)", report.Num(t.AtMS), report.Num(t.ForMS),
			t.Why, strings.Join(t.Artifacts, ", ")))
	}
	if len(pieces) > 2 {
		known = ""
	}
	return report.Fail, known, fmt.Sprintf("%d watchdog trips: %s", len(trips), quote(first3(lines)))
}
