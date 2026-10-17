package scenario

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	lib "github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/streamlab/cdn"
	"github.com/shapedtime/momoshtrem/internal/streamlab/client"
	"github.com/shapedtime/momoshtrem/internal/streamlab/media"
	"github.com/shapedtime/momoshtrem/internal/streamlab/report"
	"github.com/shapedtime/momoshtrem/internal/streamlab/stack"
)

// The HDRezka scenarios (PLAN §5 "Tiers", #66): an HDRezka movie played
// over WebDAV through production's RemoteFile, HDRezkaSource and Streamer,
// from the fake CDN (internal/streamlab/cdn), under the CDN's faults.

func init() {
	catalog = append(catalog, hdrezkaPlay, hdrezkaSeek, hdrezkaExpiry, hdrezka503, hdrezkaStall)
	Checks = append(Checks, hdrezkaChecks...)
	Metrics = append(Metrics, hdrezkaMetrics...)
}

var hdrezkaChecks = []CheckDef{
	{ID: "remote-browse-no-open", Doc: "PROPFINDs and HEADs of an HDRezka file make no CDN request: a remote file opens without I/O (#66)."},
	{ID: "remote-one-open-per-get", Doc: "Each GET of an HDRezka file made one CDN request: its body read straight through, without a reopen (#66)."},
	{ID: "link-refresh", Doc: "Each expiry of the CDN's links cost one Invalidate and one fresh resolve, with no other resolve, and the playback carried on: the streamer refreshes a refused link once (#66)."},
	{ID: "retry-503", Doc: "A burst of one 503 is absorbed by the streamer's retry, at a seek and at a mid-stream reopen alike; a burst of two breaks exactly one body (the streamer retries once), which the player's reconnect recovers."},
	{ID: "remote-watchdog", Doc: "A stalled CDN body ends within twice the read timeout plus 2 s (the RemoteFile's idle watchdog closes it once it has had nothing for the read timeout, after the connection's buffers drain) and is reopened where it stopped, and a dropped one is reopened within 2 s: no read breaks."},
}

var hdrezkaMetrics = []report.MetricDef{
	{Name: "hdrezka_upstream_opens", Kind: report.Count, Unit: "requests", Better: report.Lower, Doc: "Requests the fake CDN got: one per body the streamer opened, retries and refreshes included. A count: under faults, a dropped request may open its body or not."},
	{Name: "link_resolves", Kind: report.Exact, Unit: "resolves", Better: report.Lower, Doc: "Link calls that found no cached link and resolved one: one per stream, and one after each Invalidate."},
	{Name: "link_invalidations", Kind: report.Exact, Unit: "calls", Better: report.Lower, Doc: "Invalidate calls: links the CDN refused."},
	{Name: "cdn_peak_bodies", Kind: report.Count, Unit: "bodies", Better: report.None, Doc: "The most CDN bodies open at once. The streamer holds at most 8."},
	{Name: "cdn_bodies_left", Kind: report.Exact, Unit: "bodies", Better: report.Lower, Doc: "CDN bodies still open after quiescence."},
	{Name: "streamer_slots_held", Kind: report.Exact, Unit: "slots", Better: report.Lower, Doc: "Of the streamer's 8 body slots, those still held after quiescence."},
}

// RemoteSlots is the known issue of a dropped HDRezka request's handler
// holding its streamer slot until its read gives up (#232).
const RemoteSlots = "#232"

// lagKnown is the known issue whose signature a handler's lag after its
// drop carries: F12 for a torrent file's that ended within the read
// timeout, RemoteSlots for an HDRezka file's that ended within the remote
// read timeout (its watchdog), each plus abortLagSlack. "" otherwise.
//
// A handler can outlive its drop by up to 3× the remote read timeout in
// general (the watchdog, then the reopen's open and its read), but the
// lab's stall ends with the watchdog, so a lag past one timeout is a
// regression, not #232.
func (c *collector) lagKnown(l abortLag, timeout time.Duration) string {
	if !l.done {
		return ""
	}
	if c.e.remoteTitle(l.path) {
		if l.lag <= c.e.Stack.HDRezkaReadTimeout+abortLagSlack {
			return RemoteSlots
		}
		return ""
	}
	if l.lag <= timeout+abortLagSlack {
		return "F12"
	}
	return ""
}

// remoteTitle reports whether path is an HDRezka movie's.
func (e *Env) remoteTitle(path string) bool {
	for _, t := range e.Titles {
		if t.Path == path && t.Remote != nil {
			return true
		}
	}
	return false
}

// hdrezkaParams are every HDRezka scenario's own params.
var hdrezkaParams = []Param{
	{Name: "hdrezka_read_timeout", Kind: Duration, Default: "3s", Doc: "Bounds a remote file's opens and each silent stretch of its reads. Production's hdrezkaReadTimeout is 30 s."},
	{Name: "cdn_rate", Kind: Bytes, Default: "4MiB", Doc: "How fast each CDN body sends, bytes per second; 0 is unlimited. Above the bitrate unless a scenario says otherwise."},
}

// withHDRezka is params with the HDRezka scenarios' own.
func withHDRezka(ps ...Param) []Param { return append(append([]Param(nil), hdrezkaParams...), ps...) }

// hdrezkaMedia is one HDRezka movie: an MP4 of file_size the CDN serves.
func hdrezkaMedia(p Params) Media {
	spec := media.SingleFile("Interstellar.2014.1080p.mp4", uint64(p.Int("seed")), p.Bytes("piece_length"), p.Bytes("file_size"), media.MP4)
	return Media{Remote: []RemoteRef{{Title: "Interstellar", Year: 2014, Spec: spec, Stream: lib.StreamRef{
		Provider: "hdrezka", Path: "/films/fiction/2259-interstellar-2014.html", PostID: 2259, Title: "Interstellar",
		TranslatorID: 238, TranslatorName: "Оригинал (+субтитры)", Quality: "1080p"}}}}
}

func hdrezkaStack(p Params) Stack {
	return Stack{HDRezkaReadTimeout: p.Duration("hdrezka_read_timeout")}
}

// remote is the script's HDRezka movie and the CDN, paced at cdn_rate.
func (e *Env) remote() (*Title, *cdn.CDN) {
	c := e.swarm.cdn
	c.SetRate(e.P.Bytes("cdn_rate"))
	return e.Titles[0], c
}

// remoteQuiet is the fake CDN's and the streamer's state once the
// requests ended: no body open and every slot free is quiet.
type remoteQuiet struct {
	on        bool // the scenario has HDRezka movies
	bodies    int  // CDN bodies open
	slotsHeld int
	waited    time.Duration
}

// Err describes what was left; nil when quiet.
func (r remoteQuiet) Err() error {
	if !r.on || (r.bodies == 0 && r.slotsHeld == 0) {
		return nil
	}
	return fmt.Errorf("HDRezka not quiet after %v: %d CDN bodies open, %d of the streamer's %d slots held",
		r.waited.Round(time.Millisecond), r.bodies, r.slotsHeld, cdn.StreamerSlots)
}

func (r remoteQuiet) String() string {
	if !r.on {
		return ""
	}
	return fmt.Sprintf("; HDRezka: no CDN body open, all %d streamer slots free", cdn.StreamerSlots)
}

// remoteQuiescence waits up to limit (a second at least) for the CDN's
// bodies to end, then probes the streamer's free slots: what the stack's
// quiescence check can't see. Call it after checks.Quiescence.
func (e *Env) remoteQuiescence(limit time.Duration) remoteQuiet {
	if e.swarm == nil || e.swarm.cdn == nil || e.Stack.HDRezka == nil {
		return remoteQuiet{}
	}
	start := time.Now()
	deadline := start.Add(max(limit, time.Second))
	r := remoteQuiet{on: true}
	for {
		r.bodies = e.swarm.cdn.Open()
		if r.bodies == 0 {
			r.slotsHeld = cdn.StreamerSlots - e.swarm.links.FreeSlots(e.Stack.HDRezka, cdn.StreamerSlots, 500*time.Millisecond)
		}
		if (r.bodies == 0 && r.slotsHeld == 0) || time.Now().After(deadline) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	r.waited = time.Since(start)
	return r
}

// remoteMetrics sets the HDRezka metrics when the scenario has a CDN.
func (c *collector) remoteMetrics() {
	ss := c.e.swarm
	if ss == nil || ss.cdn == nil {
		return
	}
	st, ls := ss.cdn.Stats(), ss.links.Stats()
	c.set("hdrezka_upstream_opens", float64(st.Requests))
	c.set("link_resolves", float64(ls.Resolves))
	c.set("link_invalidations", float64(ls.Invalidations))
	c.set("cdn_peak_bodies", float64(st.PeakOpen))
	if c.remote.on {
		c.set("cdn_bodies_left", float64(c.remote.bodies))
		c.set("streamer_slots_held", float64(c.remote.slotsHeld))
	}
}

// writeRemoteStatus writes the CDN's stats and request log, for a failure
// dump.
func (e *Env) writeRemoteStatus(w io.Writer) {
	st, ls := e.swarm.cdn.Stats(), e.swarm.links.Stats()
	fmt.Fprintf(w, "\n== fake CDN ==\n%+v\nlinks: %+v\n", st, ls)
	for i, r := range e.swarm.cdn.Log() {
		fmt.Fprintf(w, "%3d %s from %d: %d, %d bytes, stalled=%v dropped=%v, ended %v\n", i+1,
			r.At.Format("15:04:05.000"), r.From, r.Status, r.Sent, r.Stalled, r.Dropped, !r.End.IsZero())
	}
}

// onePerGet checks that each GET of t made one CDN request, once its
// requests have ended.
func (e *Env) onePerGet(t *Title) {
	e.RequestsDone(func(r stack.Request) bool { return r.Path == t.Path }, 10*time.Second)
	gets := 0
	for _, r := range e.Stack.Requests.Requests() {
		if r.Method == http.MethodGet && r.Path == t.Path {
			gets++
		}
	}
	opens := e.swarm.cdn.Stats().Requests
	ev := fmt.Sprintf("%d GETs, %d CDN requests", gets, opens)
	if opens == gets {
		e.Check("remote-one-open-per-get", report.Pass, "", ev)
	} else {
		e.Check("remote-one-open-per-get", report.Fail, "", ev+": "+cdnLog(e.swarm.cdn))
	}
}

// cdnLog is the CDN's requests in a line each, for evidence.
func cdnLog(c *cdn.CDN) string {
	var parts []string
	for _, r := range c.Log() {
		parts = append(parts, fmt.Sprintf("from %d: %d, %d B", r.From, r.Status, r.Sent))
	}
	return strings.Join(parts, "; ")
}

var hdrezkaPlay = &Scenario{
	Name: "hdrezka-play",
	Tier: Gate,
	Doc: `A player opens an HDRezka movie as Infuse does: a Depth 1 PROPFIND of its folder, a PROPFIND and a HEAD of the file, the header and the footer probed at once on two connections, then 10 s of play.

The file is production's RemoteFile over HDRezkaSource and the Streamer, its bytes from the fake CDN. It opens without I/O, so browsing makes no CDN request (remote-browse-no-open), and each GET reads one CDN body straight through (remote-one-open-per-get). It guards #66's streaming path.`,
	Guards: []string{"#66"},
	Params: withHDRezka(Param{Name: "watch", Kind: Duration, Default: "10s", Doc: "How much media to play."}),
	Media:  hdrezkaMedia,
	Stack:  hdrezkaStack,
	Checks: []string{"remote-browse-no-open", "remote-one-open-per-get"},
	Key:    []string{"time_to_playable", "probe_latency", "hdrezka_upstream_opens", "cdn_bodies_left"},
	Script: func(e *Env) error {
		t, c := e.remote()
		dev := e.Device("infuse")
		a, b := e.Conn(dev), e.Conn(dev)
		if _, err := e.Propfind(a, t.Dir, 1); err != nil {
			return fmt.Errorf("PROPFIND %s: %w", t.Dir, err)
		}
		if _, err := e.Propfind(a, t.Path, 0); err != nil {
			return fmt.Errorf("PROPFIND %s: %w", t.Path, err)
		}
		if _, err := e.Head(a, t); err != nil {
			return err
		}
		ev := fmt.Sprintf("2 PROPFINDs and a HEAD made %d CDN requests", c.Stats().Requests)
		if c.Stats().Requests == 0 {
			e.Check("remote-browse-no-open", report.Pass, "", ev)
		} else {
			e.Check("remote-browse-no-open", report.Fail, "", ev)
		}
		n := e.P.Bytes("probe_bytes")
		if err := e.Go(
			func() error { return e.Probe(a, t, 0, n) },
			func() error { return e.Probe(b, t, t.Size-n, n) },
		); err != nil {
			return err
		}
		p, err := e.Player(dev, t).Open(e.Ctx, a, 0)
		if err := e.Live(err); err != nil {
			return err
		}
		if err := e.Live(p.Watch(e.P.Duration("watch"))); err != nil {
			return err
		}
		p.Stop()
		e.onePerGet(t)
		return nil
	},
}

var hdrezkaSeek = &Scenario{
	Name: "hdrezka-seek",
	Tier: Gate,
	Doc: `A player plays an HDRezka movie from the start, seeks forward to 40% and 75%, rewinds a few seconds, then seeks back to 10%. Each seek drops the playing request and opens a new one, as Infuse does.

Each GET gets a RemoteFile of its own, so each seek closes one CDN body (its handler ends at its next write, freeing its streamer slot) and opens another at the new position: one CDN request per GET (remote-one-open-per-get), every handler gone within abort_lag_max of its drop (abort-lag), and no body or slot left (quiescence). It guards #66.`,
	Guards: []string{"#66"},
	Params: withHDRezka(
		Param{Name: "watch", Kind: Duration, Default: "4s", Doc: "How much media to play at each position."},
		Param{Name: "rewind", Kind: Duration, Default: "5s", Doc: "How far the rewind goes back, in media time."},
		Param{Name: "abort_lag_max", Kind: Duration, Default: "1s", Doc: "The longest a handler may run after its client dropped it."},
	),
	Media:  hdrezkaMedia,
	Stack:  hdrezkaStack,
	Checks: []string{"remote-one-open-per-get", "abort-lag"},
	Key:    []string{"seek_latency_p50", "hdrezka_upstream_opens", "handler_lag_after_abort", "cdn_peak_bodies"},
	Script: func(e *Env) error {
		t, _ := e.remote()
		dev := e.Device("infuse")
		conn := e.Conn(dev)
		s := e.Player(dev, t)
		watch := e.P.Duration("watch")
		p, err := s.Open(e.Ctx, conn, 0)
		if err := e.Live(err); err != nil {
			return err
		}
		if err := e.Live(p.Watch(watch)); err != nil {
			return err
		}
		back := int64(e.P.Duration("rewind").Seconds() * float64(e.P.Bytes("bitrate")))
		for _, step := range []func() int64{
			func() int64 { return at(t, 0.40) },
			func() int64 { return at(t, 0.75) },
			func() int64 { return max(p.Playhead()-back, 0) &^ (64<<10 - 1) },
			func() int64 { return at(t, 0.10) },
		} {
			if p, err = s.Seek(e.Ctx, p, conn, step()); e.Live(err) != nil {
				return e.Ctx.Err()
			}
			if err := e.Live(p.Watch(watch)); err != nil {
				return err
			}
		}
		p.Stop()
		e.onePerGet(t)
		return nil
	},
}

var hdrezkaExpiry = &Scenario{
	Name: "hdrezka-expiry",
	Tier: Gate,
	Doc: `The CDN's links expire while a player watches an HDRezka movie. Each expiry refuses every link issued before it, with 403, then 404, then 410 (the statuses HDRezka's CDN uses), and the next request finds its link refused: the streamer invalidates it, resolves a fresh one and tries once more.

The first two expiries meet a seek's GET. The third meets the RemoteFile's own reopen: the CDN drops the playing body, and the reopen at its position (#66: "a 404 mid-stream refreshes and resumes byte-exact") finds the refused link. link-refresh needs one Invalidate and one fresh resolve per expiry and no break.`,
	Guards: []string{"#66"},
	Params: withHDRezka(Param{Name: "watch", Kind: Duration, Default: "3s", Doc: "How much media to play between expiries."}),
	Media:  hdrezkaMedia,
	Stack:  hdrezkaStack,
	Checks: []string{"link-refresh"},
	Key:    []string{"link_resolves", "link_invalidations", "seek_latency_p50", "hdrezka_upstream_opens"},
	Script: func(e *Env) error {
		t, c := e.remote()
		dev := e.Device("infuse")
		conn := e.Conn(dev)
		s := e.Player(dev, t)
		watch := e.P.Duration("watch")
		p, err := s.Open(e.Ctx, conn, 0)
		if err := e.Live(err); err != nil {
			return err
		}
		if err := e.Live(p.Watch(watch)); err != nil {
			return err
		}
		statuses := []int{http.StatusForbidden, http.StatusNotFound, http.StatusGone}
		for i, status := range statuses {
			c.Expire(status)
			if i < len(statuses)-1 {
				if p, err = s.Seek(e.Ctx, p, conn, at(t, 0.3*float64(i+1))); e.Live(err) != nil {
					return e.Ctx.Err()
				}
			} else if n := c.Drop(); n != 1 {
				e.Note("the drop cut %d bodies, want the playing one", n)
			}
			if err := e.Live(p.Watch(watch)); err != nil {
				return err
			}
		}
		p.Stop()
		e.RequestsDone(func(r stack.Request) bool { return r.Path == t.Path }, 10*time.Second)
		st, ls := c.Stats(), e.swarm.links.Stats()
		n := len(statuses)
		ev := fmt.Sprintf("%d expiries: %d refused, %d invalidations, %d resolves, %d CDN requests", n, st.Refused, ls.Invalidations, ls.Resolves, st.Requests)
		if st.Refused == n && ls.Invalidations == n && ls.Resolves == n+1 && p.Err() == nil {
			e.Check("link-refresh", report.Pass, "", ev)
		} else {
			e.Check("link-refresh", report.Fail, "", fmt.Sprintf("%s, want %d, %d, %d; playback error: %v; %s", ev, n, n, n+1, p.Err(), cdnLog(c)))
		}
		return nil
	},
}

var hdrezka503 = &Scenario{
	Name: "hdrezka-503",
	Tier: Gate,
	Doc: `The CDN answers bursts of 503 while a player watches an HDRezka movie.

A burst of one meets a seek: the streamer retries once a second later, so the seek is a second slower and nothing breaks. A burst of two meets the next seek: it exhausts that one retry, so the GET's body, its 206 headers already sent, breaks before its first byte, and the player reconnects (expected, once). Last, a burst of one meets the RemoteFile's reopen of a body the CDN dropped mid-stream: retried there too, no break. retry-503 checks each phase.`,
	Guards: []string{"#66"},
	Params: withHDRezka(Param{Name: "watch", Kind: Duration, Default: "3s", Doc: "How much media to play after each burst."}),
	Media:  hdrezkaMedia,
	Stack:  hdrezkaStack,
	Checks: []string{"retry-503"},
	Key:    []string{"seek_latency_p50", "seek_latency_p95", "hdrezka_upstream_opens", "stall_count"},
	Script: func(e *Env) error {
		t, c := e.remote()
		dev := e.Device("infuse")
		conn := e.Conn(dev)
		s := e.Player(dev, t)
		watch := e.P.Duration("watch")
		p, err := s.Open(e.Ctx, conn, 0)
		if err := e.Live(err); err != nil {
			return err
		}
		if err := e.Live(p.Watch(watch)); err != nil {
			return err
		}
		failures := func() int {
			e.rec.mu.Lock()
			defer e.rec.mu.Unlock()
			return len(e.rec.failures)
		}
		var notes []string
		ok := true
		// phase runs a burst of 503s with move, then watches. breakAt is
		// where the one body the burst breaks asks from (before its first
		// byte), or -1 when nothing may break.
		phase := func(name string, burst int, breakAt int64, move func() error) error {
			before := failures()
			wantBreaks := 0
			var x *Expectation
			if breakAt >= 0 {
				wantBreaks = 1
				x = e.Expect(t, breakAt, 1, fmt.Sprintf("a burst of %d 503s exhausts the streamer's one retry", burst))
			}
			c.Unavailable(burst)
			if err := move(); err != nil {
				return err
			}
			if err := e.Live(p.Watch(watch)); err != nil {
				return err
			}
			breaks := failures() - before
			if x != nil {
				e.End(x)
			}
			notes = append(notes, fmt.Sprintf("%s: %d breaks", name, breaks))
			ok = ok && breaks == wantBreaks && p.Err() == nil
			return nil
		}
		seek := func(frac float64) func() error {
			return func() error {
				var err error
				p, err = s.Seek(e.Ctx, p, conn, at(t, frac))
				return e.Live(err)
			}
		}
		if err := phase("a burst of 1 at a seek", 1, -1, seek(0.30)); err != nil {
			return err
		}
		if err := phase("a burst of 2 at a seek", 2, at(t, 0.60), seek(0.60)); err != nil {
			return err
		}
		if err := phase("a burst of 1 at a mid-stream reopen", 1, -1, func() error { c.Drop(); return nil }); err != nil {
			return err
		}
		p.Stop()
		st := c.Stats()
		ev := fmt.Sprintf("%s; %d 503s answered", strings.Join(notes, ", "), st.Unavailable)
		if ok && st.Unavailable == 4 {
			e.Check("retry-503", report.Pass, "", ev)
		} else {
			e.Check("retry-503", report.Fail, "", fmt.Sprintf("%s, want 0, 1 and 0 breaks and 4 503s; playback error %v; %s", ev, p.Err(), cdnLog(c)))
		}
		return nil
	},
}

var hdrezkaStall = &Scenario{
	Name: "hdrezka-stall",
	Tier: Gate,
	Doc: `The CDN body a player reads stalls, and the RemoteFile's idle watchdog closes it after the read timeout (hdrezka_read_timeout; production's is 30 s) and reopens at its position: the player sees a pause, not a break. Then the CDN drops the body mid-stream, and the RemoteFile reopens at once. Then the CDN slows to half the bitrate for slow_time: the player drains its buffer and may stall (a timing metric, expected), but no read breaks. remote-watchdog checks the stalled body ended within twice the read timeout plus 2 s (its watchdog counts from the last bytes the RemoteFile got, which the connection's buffers still deliver after the stall) and both reopens resumed where the body stopped.

Last, a scrub while every CDN body stalls: stall_scrub GETs, each dropped scrub_time/stall_scrub after it was sent, as Infuse drops seeks that haven't started. Each one's handler blocks in a remote read, holding one of the streamer's 8 slots, until the watchdog gives up: the later GETs wait for a slot (cdn_peak_bodies shows the cap), and abort-lag fails with #232's signature (F12's HDRezka counterpart): each slow handler ended within hdrezka_read_timeout plus a second, its watchdog. The stall lasts no longer than the watchdog (scrub_time is the stall too), so the reopen doesn't stall again, and a lag past one timeout is a regression, not #232. quiescence checks no CDN body or slot is left once they end.`,
	Guards: []string{"#66", RemoteSlots},
	Params: withHDRezka(
		Param{Name: "watch", Kind: Duration, Default: "3s", Doc: "How much media to play between faults."},
		Param{Name: "slow_time", Kind: Duration, Default: "6s", Doc: "How long the CDN sends at half the bitrate."},
		Param{Name: "max_buffer", Kind: Bytes, Default: "6MiB"},
		Param{Name: "stall_scrub", Kind: Int, Default: "12", Doc: "GETs of the scrub while every CDN body stalls; 0 skips it."},
		Param{Name: "scrub_time", Kind: Duration, Default: "3s", Doc: "How long that scrub takes, and the stall. Keep it within hdrezka_read_timeout: a longer stall stalls the reopen too, and the lags run past #232's signature."},
		Param{Name: "abort_lag_max", Kind: Duration, Default: "1s", Doc: "The longest a handler may run after its client dropped it."},
	),
	Media:  hdrezkaMedia,
	Stack:  hdrezkaStack,
	Checks: []string{"remote-watchdog", "abort-lag"},
	Key:    []string{"cdn_peak_bodies", "handler_lag_after_abort", "stall_time", "streamer_slots_held"},
	Script: func(e *Env) error {
		t, c := e.remote()
		dev := e.Device("infuse")
		s := e.Player(dev, t)
		watch := e.P.Duration("watch")
		timeout := e.P.Duration("hdrezka_read_timeout")
		p, err := s.Open(e.Ctx, e.Conn(dev), 0)
		if err := e.Live(err); err != nil {
			return err
		}
		if err := e.Live(p.Watch(watch)); err != nil {
			return err
		}
		var problems, notes []string
		// The stall: the watchdog must close the body and reopen where it
		// stopped.
		logged := len(c.Log())
		stalled := time.Now()
		if n := c.Stall(); n != 1 {
			problems = append(problems, fmt.Sprintf("the stall caught %d bodies, want the playing one", n))
		}
		if err := e.Live(p.Watch(timeout + watch)); err != nil {
			return err
		}
		// The watchdog counts from the RemoteFile's last progress, and the
		// bytes already in the connection's buffers still arrive after the
		// stall (measured: 1.3-2.1 s at a 3 s timeout), so the bound is twice
		// the timeout plus slack. Without the watchdog the body would never
		// end.
		if msg, ok := reopened(c.Log(), logged, stalled, 2*timeout+2*time.Second); ok {
			notes = append(notes, "stall: "+msg)
		} else {
			problems = append(problems, "stall: "+msg)
		}
		// The drop: the RemoteFile reopens at once.
		logged = len(c.Log())
		dropped := time.Now()
		c.Drop()
		if err := e.Live(p.Watch(watch)); err != nil {
			return err
		}
		if msg, ok := reopened(c.Log(), logged, dropped, 2*time.Second); ok {
			notes = append(notes, "drop: "+msg)
		} else {
			problems = append(problems, "drop: "+msg)
		}
		// The slow body: the player may stall, nothing breaks.
		c.SetRate(e.P.Bytes("bitrate") / 2)
		if err := e.Live(p.Watch(e.P.Duration("slow_time"))); err != nil {
			return err
		}
		c.SetRate(e.P.Bytes("cdn_rate"))
		if err := e.Live(p.Watch(watch)); err != nil {
			return err
		}
		p.Stop()
		if n := e.P.Int("stall_scrub"); n > 0 {
			note, err := e.scrubDuringStall(t, c, dev, int(n))
			if err != nil {
				return err
			}
			notes = append(notes, note)
		}
		ev := strings.Join(append(notes, problems...), "; ")
		if len(problems) == 0 && p.Err() == nil {
			e.Check("remote-watchdog", report.Pass, "", ev)
		} else {
			e.Check("remote-watchdog", report.Fail, "", fmt.Sprintf("%s; playback error %v; %s", ev, p.Err(), cdnLog(c)))
		}
		return nil
	},
}

// scrubDuringStall scrubs while every CDN body stalls: n GETs at random
// positions, each dropped scrub_time/n after it was sent, headers or not,
// as Infuse drops a seek that hasn't started. Each one's handler blocks in
// a remote read, holding a streamer slot, until the read gives up. Then a
// player opens at 50% once the stall is over, and plays.
func (e *Env) scrubDuringStall(t *Title, c *cdn.CDN, dev client.Device, n int) (string, error) {
	scrub := e.P.Duration("scrub_time")
	step := scrub / time.Duration(n)
	peak := c.Stats().PeakOpen
	c.StallFor(scrub)
	for range n {
		conn := e.Conn(dev)
		rng := rangeOf(at(t, 0.1+0.8*float64(e.Rand(1000))/1000), 0)
		ctx, cancel := context.WithCancel(e.Ctx)
		go func() {
			if st, err := conn.Get(ctx, t.Path, http.Header{"Range": {rng}}); err == nil {
				<-ctx.Done()
				st.Abort()
			}
		}()
		err := e.Sleep(step)
		e.Abandon(conn, t.Path, rng)
		cancel()
		if err != nil {
			return "", err
		}
	}
	if err := e.Sleep(time.Second); err != nil {
		return "", err
	}
	scrubPeak := c.Stats().PeakOpen
	q, err := e.Player(dev, t).Open(e.Ctx, e.Conn(dev), at(t, 0.5))
	if err := e.Live(err); err != nil {
		return "", err
	}
	if err := e.Live(q.Watch(e.P.Duration("watch"))); err != nil {
		return "", err
	}
	st := q.Stop()
	return fmt.Sprintf("scrub during a stall: %d GETs dropped, %d CDN bodies open at most (%d before); the play after it playable in %v",
		n, scrubPeak, peak, st.TimeToPlayable.Round(time.Millisecond)), nil
}

// reopened judges a body the CDN stalled or dropped at at: the body that
// was open then (the log's last before index from) ended within limit,
// and the next request asked for the byte after its last one, and
// succeeded.
func reopened(log []cdn.Request, from int, at time.Time, limit time.Duration) (string, bool) {
	if from == 0 || from > len(log) {
		return "no body was open", false
	}
	old := log[from-1]
	switch {
	case old.End.IsZero():
		return fmt.Sprintf("the body from %d never ended", old.From), false
	case old.End.Sub(at) > limit:
		return fmt.Sprintf("the body from %d ended %v after, over %v", old.From, old.End.Sub(at).Round(time.Millisecond), limit), false
	case len(log) <= from:
		return fmt.Sprintf("the body from %d ended %v after, and nothing reopened", old.From, old.End.Sub(at).Round(time.Millisecond)), false
	}
	next := log[from]
	resume := old.From + old.Sent
	msg := fmt.Sprintf("the body from %d ended %v after, %d bytes sent; reopened from %d (%d)", old.From,
		old.End.Sub(at).Round(time.Millisecond), old.Sent, next.From, next.Status)
	// The reopen asks for the byte after the last one the RemoteFile read,
	// at or before the last one the CDN sent (some may sit unread in the
	// connection's buffers).
	if next.From < old.From || next.From > resume || next.Status != http.StatusPartialContent {
		return msg + fmt.Sprintf(", want a 206 from at most %d", resume), false
	}
	return msg, true
}
