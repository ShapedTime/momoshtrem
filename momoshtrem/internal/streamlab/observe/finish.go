package observe

import (
	"cmp"
	"log/slog"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/shapedtime/momoshtrem/internal/streamlab/report"
)

// Result is what Finish makes of a repetition.
type Result struct {
	Observation *report.Observation
	Findings    []report.Finding
	// Artifacts are the files written, by name, relative to Config.Dir.
	Artifacts map[string]string
	// Metrics are the scenario metrics observation measures (declared in
	// the scenario package).
	Metrics map[string]float64
}

// The metrics Finish reports.
const (
	MetricBenignStorageErrors = "benign_storage_errors"
	MetricStorageReadFailures = "storage_read_failures"
	MetricLogErrors           = "log_errors"
	MetricLogWarnings         = "log_warnings"
	MetricWatchdogTrips       = "watchdog_trips"
	MetricOverhead            = "observe_overhead"
)

// topLogs is how many distinct messages the report lists.
const topLogs = 10

// Finish stops the observer if it runs, writes the artifacts and draws
// the findings.
func (o *Observer) Finish(f Facts) Result {
	if o == nil {
		return Result{}
	}
	o.Stop()
	o.holdKnownStall()
	hist := map[string]history{}
	var piecesBusy time.Duration
	var pieceEvents int
	for h, w := range o.watches {
		hist[h] = w.history()
		w.mu.Lock()
		piecesBusy += w.busy
		w.mu.Unlock()
		pieceEvents += len(hist[h].events)
	}
	reqs := o.cfg.Stack.Requests.Requests()
	var logs []LogEvent
	if o.cfg.Logs != nil {
		logs = o.cfg.Logs.Events(o.start)
	}
	res := Result{Artifacts: map[string]string{}, Metrics: map[string]float64{}}
	obs := &report.Observation{Diagnose: o.cfg.Diagnose, WallMS: report.MS(o.stopped.Sub(o.start)), ProcessCPUMS: report.MS(o.cpu),
		Mutex: o.cont.mutex, Block: o.cont.block, ClientLockMS: o.cont.clientLockMS, SetPriorityMS: o.cont.setPriorityMS}
	obs.Samplers, obs.OverheadPct = o.samplerStats(piecesBusy, pieceEvents)
	obs.Logs = summarizeLogs(logs, o.cfg.Logs)
	obs.Timeline = o.titleTimelines(f, hist, reqs)
	for _, t := range o.trips {
		obs.Trips = append(obs.Trips, report.Trip{AtMS: o.ms(t.at), ForMS: report.MS(t.stuck), Readers: t.readers,
			Known: t.known, Why: t.why, Artifacts: t.artifacts})
	}
	for _, ft := range o.faultList() {
		obs.Faults = append(obs.Faults, report.Fault{Name: ft.name, Torrent: ft.key, StartMS: o.ms(ft.start), EndMS: o.ms(ft.end)})
	}
	res.Observation = obs
	res.Findings = o.findings(f, reqs, logs)

	writes := []struct {
		key   string
		write func() (string, error)
	}{
		{"samples", o.writeSamples},
		{"timeline", func() (string, error) { return o.writeTimeline(f, hist, reqs) }},
		{"logs", func() (string, error) { return writeNDJSON(o.cfg.Dir, "logs.ndjson", logLines(logs, o)) }},
	}
	for _, w := range writes {
		if name, err := w.write(); err == nil && name != "" {
			res.Artifacts[w.key] = name
		}
	}
	for k, v := range o.cont.artifacts {
		res.Artifacts[k] = v
	}
	for _, t := range obs.Timeline {
		if t.Plot != "" {
			res.Artifacts["timeline-plot-"+slug(t.Title)] = t.Plot
		}
	}
	for _, t := range o.trips {
		for _, a := range t.artifacts {
			res.Artifacts[strings.TrimSuffix(a, filepath.Ext(a))] = a
		}
	}

	res.Metrics[MetricOverhead] = obs.OverheadPct
	if o.cfg.Watchdog > 0 {
		res.Metrics[MetricWatchdogTrips] = float64(len(o.trips))
	}
	if o.cfg.Logs != nil {
		res.Metrics[MetricBenignStorageErrors] = float64(obs.Logs.BenignStorageErrors)
		res.Metrics[MetricStorageReadFailures] = float64(obs.Logs.ReadFailures)
		res.Metrics[MetricLogErrors] = float64(obs.Logs.Errors)
		res.Metrics[MetricLogWarnings] = float64(obs.Logs.Warnings)
	}
	return res
}

// samplerStats is each sampler's cost, the piece mirror's and the log
// capture's, and their total over the process's CPU time, in percent (over
// the wall time on one core where the process's CPU time isn't known). A
// trip's dumps aren't counted in the watchdog's busy time (its max keeps
// them): the repetition failed then.
func (o *Observer) samplerStats(piecesBusy time.Duration, pieceEvents int) ([]report.SamplerStat, float64) {
	base := o.cpu
	if !o.cpuOK || base <= 0 {
		base = o.stopped.Sub(o.start)
	}
	pct := func(d time.Duration) float64 {
		if base <= 0 {
			return 0
		}
		return 100 * float64(d) / float64(base)
	}
	var out []report.SamplerStat
	var total time.Duration
	for _, s := range o.samplers {
		busy := s.busy
		if s.name == "watchdog" {
			busy = max(busy-o.dumpTime, 0)
		}
		out = append(out, report.SamplerStat{Name: s.name, EveryMS: report.MS(time.Duration(s.every) * tick), Samples: s.n,
			BusyMS: report.MS(busy), MaxMS: report.MS(s.max), CPUPct: pct(busy), ClientLock: s.lock})
		total += busy
	}
	out = append(out, report.SamplerStat{Name: "pieces", Samples: pieceEvents, BusyMS: report.MS(piecesBusy), CPUPct: pct(piecesBusy)})
	total += piecesBusy
	if o.cfg.Logs != nil {
		busy, kept, _ := o.cfg.Logs.cost()
		out = append(out, report.SamplerStat{Name: "logs", Samples: kept, BusyMS: report.MS(busy), CPUPct: pct(busy)})
		total += busy
	}
	return out, pct(total)
}

// summarizeLogs counts the captured records: by level, #219's benign
// storage errors (reads that logged "initial read failed" and then
// succeeded), F11's read failures, and the most frequent messages.
func summarizeLogs(logs []LogEvent, c *Capture) report.LogSummary {
	var s report.LogSummary
	if c == nil {
		return s
	}
	_, _, s.Dropped = c.cost()
	type key struct{ level, component, msg string }
	counts := map[key]int{}
	initial := 0
	for _, e := range logs {
		switch {
		case e.Level >= slog.LevelError:
			s.Errors++
		case e.Level >= slog.LevelWarn:
			s.Warnings++
		default:
			s.Debug++
		}
		switch e.Msg {
		case MsgInitialFailed:
			initial++
		case MsgResyncFailed:
			s.ReadFailures++
		}
		counts[key{e.Level.String(), e.Component, e.Msg}]++
	}
	s.BenignStorageErrors = max(initial-s.ReadFailures, 0)
	for k, n := range counts {
		s.Top = append(s.Top, report.LogCount{Level: k.level, Component: k.component, Msg: k.msg, Count: n})
	}
	slices.SortFunc(s.Top, func(a, b report.LogCount) int {
		return cmp.Or(cmp.Compare(b.Count, a.Count), cmp.Compare(a.Msg, b.Msg), cmp.Compare(a.Component, b.Component))
	})
	if len(s.Top) > topLogs {
		s.Top = s.Top[:topLogs]
	}
	return s
}

// logLine is a line of logs.ndjson.
type logLine struct {
	TMS       float64           `json:"t_ms"`
	Level     string            `json:"level"`
	Component string            `json:"component,omitempty"`
	Msg       string            `json:"msg"`
	Attrs     map[string]string `json:"attrs,omitempty"`
}

func logLines(logs []LogEvent, o *Observer) []logLine {
	out := make([]logLine, 0, len(logs))
	for _, e := range logs {
		l := logLine{TMS: o.ms(e.T), Level: e.Level.String(), Component: e.Component, Msg: e.Msg}
		if len(e.Attrs) > 0 {
			l.Attrs = make(map[string]string, len(e.Attrs))
			for _, a := range e.Attrs {
				l.Attrs[a.Key] = a.Value
			}
		}
		out = append(out, l)
	}
	return out
}

// writeSamples writes samples.ndjson: every sampler's readings, in time
// order.
func (o *Observer) writeSamples() (string, error) {
	var lines []map[string]any
	add := func(t time.Time, kind string, kv ...any) {
		m := map[string]any{"t_ms": o.ms(t), "kind": kind}
		for i := 0; i+1 < len(kv); i += 2 {
			m[kv[i].(string)] = kv[i+1]
		}
		lines = append(lines, m)
	}
	for _, s := range o.metrics {
		add(s.t, "metrics", "reads", s.reads, "read_bytes", s.readBytes, "read_timeouts", s.timeouts, "slow_reads", s.slow,
			"blocked_reads", s.blocked, "open_files", s.openFiles, "requests_inflight", s.inflight,
			"updates_initial", s.initial, "updates_seek", s.seek, "updates_debounced", s.debounced, "pieces_downgraded", s.downgraded)
	}
	for _, s := range o.counts {
		add(s.t, "readers", "count", s.n)
	}
	for _, s := range o.readers {
		add(s.t, "reader", "title", o.cfg.Titles[s.title].Name, "pos", s.pos, "lead", s.lead, "piece", s.piece, "state", stateName(s.state))
	}
	for _, s := range o.torrents {
		add(s.t, "torrent", "hash", s.hash, "downloaded", s.downloaded, "useful", s.useful, "seeders", s.seeders, "peers", s.peers, "state", s.state)
	}
	for _, s := range o.seeds {
		add(s.t, "seeder", "hash", s.hash, "sent", s.sent, "paused", s.paused)
	}
	for _, s := range o.rts {
		add(s.t, "runtime", "goroutines", s.goroutines, "heap_bytes", s.heap, "gc_cycles", s.gcCycles)
	}
	for _, s := range o.details {
		add(s.t, "detail", "hash", s.hash, "have", s.have, "partial", s.partial, "wanted", s.wanted, "missing", s.missing,
			"peers", s.peers, "seeders", s.seeders)
	}
	slices.SortStableFunc(lines, func(a, b map[string]any) int { return cmp.Compare(a["t_ms"].(float64), b["t_ms"].(float64)) })
	return writeNDJSON(o.cfg.Dir, "samples.ndjson", lines)
}
