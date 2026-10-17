package scenario

import (
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/shapedtime/momoshtrem/internal/streamlab/checks"
	"github.com/shapedtime/momoshtrem/internal/streamlab/known"
	"github.com/shapedtime/momoshtrem/internal/streamlab/player"
	"github.com/shapedtime/momoshtrem/internal/streamlab/report"
	"github.com/shapedtime/momoshtrem/internal/streamlab/stack"
)

const mib = 1 << 20

// abortLagSlack is how much past the read timeout an F12 lag may run: the
// handler still writes its error and returns after the read gives up.
const abortLagSlack = time.Second

type playerResult struct {
	playerRec
	stats player.SessionStats
}

// collector turns a repetition's records into its metrics, sessions and
// checks.
type collector struct {
	e         *Env
	sc        *Scenario
	rep       *report.Rep
	players   []playerResult
	quiet     checks.QuiescenceResult
	remote    remoteQuiet // the fake CDN's and the streamer's (HDRezka)
	scriptErr error
	lags      []abortLag
}

// abortLag is a dropped request's handler time after the drop.
type abortLag struct {
	reqID string
	path  string
	lag   time.Duration
	done  bool
}

func (c *collector) set(name string, v float64) {
	if _, ok := LookupMetric(name); !ok {
		panic(fmt.Sprintf("scenario: undeclared metric %q", name))
	}
	if !math.IsNaN(v) && !math.IsInf(v, 0) {
		c.rep.Metrics[name] = v
	}
}

func (c *collector) metrics(wall time.Duration, atClose, after int64, window time.Duration) {
	e, reg := c.e, c.e.Stack.Registry
	val := func(name string) float64 { return checks.Value(reg, name) }
	upd := func(t string) float64 { return checks.LabelValue(reg, metricPriorityUpdates, "type", t) }

	var mismatched int64
	seen := map[*checks.Integrity]bool{}
	for _, t := range e.Titles {
		if !seen[t.Integrity] {
			seen[t.Integrity] = true
			mismatched += t.Integrity.Result().Mismatched
		}
	}
	c.set("integrity_errors", float64(mismatched))
	c.set("panics", float64(len(checks.Panics(e.Stack.ServerErrors()))))
	leaked := 0
	for _, g := range c.quiet.Leaked {
		leaked += g.Count
	}
	c.set("leaked_goroutines", float64(leaked))
	c.set("readers_left", float64(c.quiet.Readers))
	c.set("requests_left", float64(c.quiet.Requests))
	c.set("open_files_left", c.quiet.OpenFiles)
	c.set("blocked_reads_left", c.quiet.BlockedReads)
	c.set("sessions_left", float64(c.quiet.Sessions))
	c.set("readers_created", upd("initial"))

	readMiB := val("momoshtrem_streaming_read_bytes_total") / mib
	c.set("reads", val("momoshtrem_streaming_reads_total"))
	if readMiB > 0 {
		c.set("seek_updates_per_mib", upd("seek")/readMiB)
		c.set("debounced_updates_per_mib", upd("seek_debounced")/readMiB)
	}
	c.set("seeks_actual", val("momoshtrem_streaming_seek_actual_total"))
	c.set("seeks_redundant", val("momoshtrem_streaming_seek_redundant_total"))
	c.set("pieces_downgraded", val("momoshtrem_streaming_pieces_downgraded_total"))
	c.set("read_timeouts", val(metricReadTimeouts))
	c.set("slow_reads", val("momoshtrem_streaming_slow_reads_total"))
	if sum, n := checks.Histogram(reg, "momoshtrem_streaming_activation_wait_seconds"); n > 0 {
		c.set("activation_wait", sum/float64(n)*1000)
	}

	var served int64
	for _, r := range e.Stack.Requests.Requests() {
		if r.Method == "GET" {
			served += r.Bytes
		}
	}
	c.set("bytes_served", float64(served))
	c.set("bytes_downloaded", float64(after))
	if served > 0 {
		c.set("waste_ratio", float64(after)/float64(served))
	}
	c.set("residual_download_bytes", float64(after-atClose))
	c.rep.Torrent = report.Torrent{Downloaded: after, Served: served, Residual: after - atClose,
		ResidualWindowMS: report.MS(window)}
	if served > 0 {
		c.rep.Torrent.WasteRatio = float64(after) / float64(served)
	}

	// The players: the first one's start, every one's seeks and stalls.
	var (
		seeks    []float64
		sum      player.Summary
		first    = true
		overRead int64
	)
	for _, pr := range c.players {
		s := pr.stats.Summary()
		if len(pr.stats.Plays) == 0 {
			continue
		}
		if first {
			first = false
			c.set("ttfb", report.MS(s.TTFB))
			if pr.stats.Plays[0].Playable {
				c.set("time_to_playable", report.MS(s.TimeToPlayable))
			}
		}
		for _, l := range s.SeekLatencies {
			seeks = append(seeks, report.MS(l))
		}
		sum.Stalls += s.Stalls
		sum.StallTime += s.StallTime
		sum.PlayTime += s.PlayTime
		overRead += s.OverRead
	}
	if !first {
		c.set("stall_count", float64(sum.Stalls))
		c.set("stall_time", report.MS(sum.StallTime))
		if total := sum.StallTime + sum.PlayTime; total > 0 {
			c.set("rebuffer_ratio", float64(sum.StallTime)/float64(total))
		}
		c.set("over_read_bytes", float64(overRead))
	}
	if len(seeks) > 0 {
		c.set("seek_latency_p50", report.Quantile(seeks, 0.5))
		c.set("seek_latency_p95", report.Quantile(seeks, 0.95))
	}
	if d := slices.Max(append([]time.Duration{0}, e.rec.propfinds...)); d > 0 {
		c.set("propfind_latency", report.MS(d))
	}
	if d := slices.Max(append([]time.Duration{0}, e.rec.probes...)); d > 0 {
		c.set("probe_latency", report.MS(d))
	}
	c.set("wall_time", report.MS(wall))

	c.lags = c.abortLags()
	if len(c.lags) > 0 {
		worst := time.Duration(0)
		for _, l := range c.lags {
			worst = max(worst, l.lag)
		}
		c.set("handler_lag_after_abort", report.MS(worst))
	}

	c.faultMetrics()
	c.remoteMetrics()
	for name, v := range e.rec.metrics {
		c.set(name, v)
	}
}

// abortLags matches every request a client dropped to the server's
// record of it (same User-Agent, path and Range, the first unmatched one
// that started before the drop) and measures how long its handler ran on.
func (c *collector) abortLags() []abortLag {
	type drop struct {
		ua, path, rng string
		at            time.Time
	}
	var drops []drop
	for _, pr := range c.players {
		ua := c.uaOf(pr.device)
		for _, a := range pr.stats.Aborts {
			drops = append(drops, drop{ua: ua, path: pr.title.Path, rng: a.Range, at: a.At})
		}
	}
	for _, a := range c.e.rec.aborts {
		drops = append(drops, drop{ua: a.ua, path: a.path, rng: a.rng, at: a.at})
	}
	slices.SortFunc(drops, func(a, b drop) int { return a.at.Compare(b.at) })
	reqs := c.e.Stack.Requests.Requests()
	used := map[string]bool{}
	now := time.Now()
	var out []abortLag
	for _, d := range drops {
		for _, r := range reqs {
			if used[r.ID] || r.Method != "GET" || r.UserAgent != d.ua || r.Path != d.path || r.Range != d.rng || r.Start.After(d.at) {
				continue
			}
			used[r.ID] = true
			end := r.End
			if !r.Done() {
				end = now
			}
			out = append(out, abortLag{reqID: r.ID, path: r.Path, lag: max(end.Sub(d.at), 0), done: r.Done()})
			break
		}
	}
	return out
}

func (c *collector) uaOf(device string) string {
	c.e.rec.mu.Lock()
	defer c.e.rec.mu.Unlock()
	for ua, n := range c.e.rec.devices {
		if n == device {
			return ua
		}
	}
	return device
}

// sessions groups the server's requests by device and path, with the
// player's numbers where one played the path.
func (c *collector) sessions() {
	e := c.e
	lagOf := map[string]float64{}
	for _, l := range c.lags {
		lagOf[l.reqID] = report.MS(l.lag)
	}
	type key struct{ ua, path string }
	var order []key
	byKey := map[key]*report.Session{}
	var first time.Time
	reqs := e.Stack.Requests.Requests()
	if len(reqs) > 0 {
		first = reqs[0].Start
	}
	for _, r := range reqs {
		k := key{r.UserAgent, r.Path}
		s, ok := byKey[k]
		if !ok {
			s = &report.Session{Device: e.deviceName(r.UserAgent), Path: r.Path}
			byKey[k] = s
			order = append(order, k)
		}
		s.Requests = append(s.Requests, request(r, first, lagOf))
	}
	failures := e.rec.failures
	for _, pr := range c.players {
		k := key{c.uaOf(pr.device), pr.title.Path}
		s, ok := byKey[k]
		if !ok {
			s = &report.Session{Device: pr.device, Path: pr.title.Path}
			byKey[k] = s
			order = append(order, k)
		}
		if s.Player != nil {
			continue // one player per device and path in the report
		}
		s.Player = playerStats(pr.stats, pr.device, failures, first)
	}
	for _, k := range order {
		c.rep.Sessions = append(c.rep.Sessions, *byKey[k])
	}
}

func request(r stack.Request, first time.Time, lagOf map[string]float64) report.Request {
	q := report.Request{ID: r.ID, Method: r.Method, Path: r.Path, Range: r.Range, Status: r.Status,
		StartMS: report.MS(r.Start.Sub(first)), TTFBMS: report.MS(r.FirstByte), Bytes: r.Bytes,
		DurationMS: report.MS(r.Duration()), Aborted: r.Aborted, Done: r.Done()}
	if lag, ok := lagOf[r.ID]; ok {
		q.HandlerLagMS = &lag
	}
	return q
}

// playerStats is a player session's numbers for the report; times are
// from first, the repetition's first request.
func playerStats(st player.SessionStats, device string, failures []failure, first time.Time) *report.PlayerStats {
	sum := st.Summary()
	ps := &report.PlayerStats{
		TTFBMS: report.MS(sum.TTFB), TimeToPlayableMS: report.MS(sum.TimeToPlayable),
		Stalls: sum.Stalls, StallTimeMS: report.MS(sum.StallTime), RebufferRatio: sum.RebufferRatio,
		OverRead: sum.OverRead, Received: sum.Received, HTTPErrors: st.HTTPErrors,
	}
	for _, l := range sum.SeekLatencies {
		ps.SeekLatenciesMS = append(ps.SeekLatenciesMS, report.MS(l))
	}
	for _, p := range st.Plays {
		rp := report.Play{From: p.From, Seek: p.Seek, SeekLatencyMS: report.MS(p.SeekLatency), TTFBMS: report.MS(p.TTFB),
			TimeToPlayableMS: report.MS(p.TimeToPlayable), Playable: p.Playable, StallTimeMS: report.MS(p.StallTime),
			PlayTimeMS: report.MS(p.PlayTime), PauseTimeMS: report.MS(p.PauseTime), Received: p.Received,
			Consumed: p.Consumed, OverRead: p.OverRead, EOF: p.EOF, Requests: p.Requests,
			DurationMS: report.MS(p.Duration), State: p.State, Err: p.Err}
		for _, s := range p.Stalls {
			rp.Stalls = append(rp.Stalls, report.Stall{AtMS: report.MS(s.At), DurationMS: report.MS(s.Duration), Position: s.Position, Open: s.Open})
		}
		ps.Plays = append(ps.Plays, rp)
	}
	for _, re := range st.ReadErrors {
		r := report.ReadError{Offset: re.Offset, Err: re.Err}
		if !first.IsZero() {
			r.AtMS = report.MS(re.At.Sub(first))
		}
		for _, f := range failures {
			if f.device == device && f.title.Path == st.Path && f.offset == re.Offset {
				r.Diagnosis = f.diagnosis()
				break
			}
		}
		ps.ReadErrors = append(ps.ReadErrors, r)
	}
	return ps
}

// checks evaluates the common checks and abort-lag, and reports the
// scenario's own checks, n/a where the script didn't reach them.
func (c *collector) checks() {
	e := c.e
	add := func(id string, status report.Status, known, evidence string) {
		c.rep.Checks = append(c.rep.Checks, report.RepCheck{ID: id, Status: status, Known: known, Evidence: evidence})
	}
	if c.scriptErr != nil {
		add("script", report.Fail, "", c.scriptErr.Error())
	} else {
		add("script", report.Pass, "", "ran to the end")
	}

	var checked, mismatched, unexplained int64
	var first string
	explained := map[string]int64{}
	seen := map[*checks.Integrity]bool{}
	for _, t := range e.Titles {
		if seen[t.Integrity] {
			continue
		}
		seen[t.Integrity] = true
		r := t.Integrity.Result()
		checked += r.Checked
		mismatched += r.Mismatched
		unexplained += r.Unexplained()
		for id, n := range r.Explained {
			explained[id] += n
		}
		if r.First != nil && first == "" {
			first = r.First.String()
		}
	}
	if mismatched > 0 {
		add("integrity", report.Fail, integrityKnown(unexplained, explained),
			fmt.Sprintf("%d of %d bytes don't match the media%s; the first: %s", mismatched, checked, explainedNote(explained), first))
	} else {
		add("integrity", report.Pass, "", fmt.Sprintf("%s checked, all matching", report.Bytes(checked)))
	}

	httpErrs := append([]string(nil), e.rec.httpErrors...)
	for _, pr := range c.players {
		httpErrs = append(httpErrs, pr.stats.HTTPErrors...)
	}
	panics := checks.Panics(e.Stack.ServerErrors())
	switch {
	case len(panics) > 0:
		add("http", report.Fail, "", fmt.Sprintf("%d handler panics: %s", len(panics), panics[0]))
	case len(httpErrs) > 0:
		add("http", report.Fail, "", fmt.Sprintf("%d wrong answers: %s", len(httpErrs), quote(first3(httpErrs))))
	default:
		add("http", report.Pass, "", "every answer as expected")
	}

	add(c.playOK())

	switch err := c.quiet.Err(); {
	case err != nil:
		known := ""
		if c.remote.Err() == nil {
			known = orphanedReads(c.quiet, c.rep.Metrics["read_timeouts"])
		}
		add("quiescence", report.Fail, known, joinErrs(err, c.remote.Err()))
	case c.remote.Err() != nil:
		add("quiescence", report.Fail, "", c.remote.Err().Error())
	default:
		add("quiescence", report.Pass, "", fmt.Sprintf("quiet after %v (%d labelled anacrolix goroutines inherited)%s",
			c.quiet.Waited.Round(time.Millisecond), c.quiet.Inherited, c.remote.String()))
	}

	if slices.Contains(c.sc.Checks, "abort-lag") {
		add(c.abortLag())
	}
	for _, id := range c.sc.Checks {
		if id == "abort-lag" {
			continue
		}
		if rc, ok := e.rec.checks[id]; ok {
			c.rep.Checks = append(c.rep.Checks, rc)
		} else {
			add(id, report.NA, "", "the script didn't reach it")
		}
	}
}

// orphanedReads is #225's signature on a quiescence failure: nothing left
// but goroutines blocked in anacrolix's waitAvailable under
// TorrentFile.readContext, at most one per server read that timed out in
// the repetition (readContext drains a timed-out read before the next, so
// each leaves one at most): the read goroutine a timed-out read leaves
// outlives its closed file. "" otherwise.
func orphanedReads(q checks.QuiescenceResult, timeouts float64) string {
	if timeouts == 0 || len(q.Leaked) == 0 || q.Requests != 0 || q.Readers != 0 || q.OpenFiles != 0 ||
		q.BlockedReads != 0 || q.InFlight != 0 || q.Sessions != 0 {
		return ""
	}
	orphans := 0
	for _, g := range q.Leaked {
		if !strings.Contains(g.Stack, "github.com/anacrolix/torrent.(*reader).waitAvailable") ||
			!strings.Contains(g.Stack, "internal/vfs.(*TorrentFile).readContext") {
			return ""
		}
		orphans += g.Count
	}
	if float64(orphans) > timeouts {
		return ""
	}
	return "#225"
}

// playOK judges the broken bodies: none passes. When each of them carries
// a known issue's signature (#195's stall, F11's quick break at an
// evicted piece), the failure names them all.
func (c *collector) playOK() (string, report.Status, string, string) {
	fs, expected := unexpected(c.e.rec.failures)
	plays := 0
	for _, pr := range c.players {
		plays += len(pr.stats.Plays)
	}
	if len(fs) == 0 {
		return "play-ok", report.Pass, "", fmt.Sprintf("%d playbacks and %d probes got their bytes%s", plays, c.e.rec.probesDone, expected)
	}
	pieces := map[string]bool{}
	var lines []string
	for _, f := range fs {
		pieces[pieceKey(f)] = true
		lines = append(lines, fmt.Sprintf("%s %s at %d: %s (%s)", f.device, f.title.Name, f.offset, f.err, f.diagnosis()))
	}
	return "play-ok", report.Fail, playKnown(fs, c.rep.Metrics["read_timeouts"]),
		fmt.Sprintf("%d broken bodies on %d pieces: %s", len(fs), len(pieces), quote(first3(lines)))
}

func pieceKey(f failure) string { return fmt.Sprintf("%s/%d", f.title.Hash, f.piece) }

// playKnown is the known issues the broken bodies' signatures name, joined
// by known.Sep: every failure must carry one. #195's are on at most two
// pieces (a stall wedges one connection) and at most as many as the
// server's read timeouts (each is a timed-out read). "" when any failure
// is unexplained.
func playKnown(fs []failure, readTimeouts float64) string {
	ids := map[string]bool{}
	stalled := map[string]bool{}
	stalls := 0
	for _, f := range fs {
		id := f.known()
		if id == "" {
			return ""
		}
		ids[id] = true
		if id == "#195" {
			stalled[pieceKey(f)] = true
			stalls++
		}
	}
	if len(stalled) > 2 || float64(stalls) > readTimeouts {
		return ""
	}
	out := make([]string, 0, len(ids))
	for id := range ids {
		out = append(out, id)
	}
	slices.Sort(out)
	return strings.Join(out, known.Sep)
}

// abortLag judges the handlers' lag after their client dropped them (F12).
func (c *collector) abortLag() (string, report.Status, string, string) {
	limit := c.e.P.Duration("abort_lag_max")
	timeout := c.e.P.Duration("read_timeout")
	if len(c.lags) == 0 {
		return "abort-lag", report.NA, "", "no request was dropped"
	}
	var over []string
	worst := time.Duration(0)
	ids := map[string]bool{}
	explained := true
	for _, l := range c.lags {
		worst = max(worst, l.lag)
		if l.lag <= limit {
			continue
		}
		over = append(over, fmt.Sprintf("%s %v", l.reqID, l.lag.Round(time.Millisecond)))
		id := c.lagKnown(l, timeout)
		ids[id] = true
		explained = explained && id != ""
	}
	if len(over) == 0 {
		return "abort-lag", report.Pass, "", fmt.Sprintf("%d dropped requests, the slowest handler ended %v after", len(c.lags), worst.Round(time.Millisecond))
	}
	known := ""
	if explained {
		var list []string
		for id := range ids {
			list = append(list, id)
		}
		slices.Sort(list)
		known = strings.Join(list, knownSep)
	}
	timeouts := fmt.Sprintf("read timeout %v", timeout)
	if c.e.Stack.HDRezka != nil {
		timeouts += fmt.Sprintf(", HDRezka's %v", c.e.Stack.HDRezkaReadTimeout)
	}
	return "abort-lag", report.Fail, known, fmt.Sprintf("%d of %d handlers ran over %v after their client dropped them (%s); the slowest %v (%s)",
		len(over), len(c.lags), limit, strings.Join(over, ", "), worst.Round(time.Millisecond), timeouts)
}

func first3(s []string) []string {
	if len(s) > 3 {
		return append(s[:3:3], fmt.Sprintf("… %d more", len(s)-3))
	}
	return s
}
