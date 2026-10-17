package observe

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/anacrolix/torrent/types"

	"github.com/shapedtime/momoshtrem/internal/streamlab/report"
	"github.com/shapedtime/momoshtrem/internal/streamlab/stack"
)

// Facts are what the scenario knows and the observer doesn't: the
// players' stalls and the handlers' lags after their client dropped them.
type Facts struct {
	Stalls []Stall
	Lags   []Lag
}

// Stall is a player's stall: its buffer ran empty from Start to End with
// the playhead at Position, a file offset of title Title (an index into
// Config.Titles).
type Stall struct {
	Device     string
	Title      int
	Start, End time.Time
	Position   int64
}

// Lag is how long request Request's handler ran after its client dropped
// it; Done is false when it hadn't returned when measured.
type Lag struct {
	Request string
	Lag     time.Duration
	Done    bool
}

// maxPerRule is how many findings of one rule a repetition lists; one
// more line counts the rest.
const maxPerRule = 5

// The windows a stall's rates are measured over: the stall, and the
// buffer's draining before it.
const stallLookback = 2 * time.Second

// findings runs the rules over what the run recorded.
func (o *Observer) findings(f Facts, reqs []stack.Request, logs []LogEvent) []report.Finding {
	var out []report.Finding
	for _, s := range f.Stalls {
		out = append(out, o.stallFindings(s)...)
	}
	out = append(out, o.competingWindows()...)
	out = append(out, o.abortLags(f.Lags)...)
	out = append(out, o.sniffs(logs, reqs)...)
	out = append(out, o.tripFindings()...)
	out = append(out, o.storageLoss(logs)...)
	return capPerRule(out)
}

// minBelowNow is how many reader samples (250 ms apart) must find the
// reader's piece below Now during a stall for stall-below-now: one can be
// a reader between a seek and its read, which holds no piece at Now.
const minBelowNow = 2

// stallFindings explains a stall:
//   - stall-below-now: the readers' samples during the stall found the
//     reader's piece incomplete and below Now, downloads on: the
//     prioritizer's window didn't cover the playhead;
//   - starved: the swarm sent less than the bitrate over the stall and the
//     buffer's draining before it: expected;
//   - stall-unexplained: neither, the swarm above the bitrate.
func (o *Observer) stallFindings(s Stall) []report.Finding {
	t := o.cfg.Titles[s.Title]
	dur := s.End.Sub(s.Start)
	at := o.ms(s.Start)
	plot := []string{"timeline.ndjson", "timeline-" + slug(t.Name) + ".svg"}
	what := fmt.Sprintf("%s stalled %v at %d (piece %d) of %s", s.Device, dur.Round(time.Millisecond), s.Position,
		t.piece(s.Position), t.Name)
	var out []report.Finding

	samples, below := 0, 0
	var seen []string
	for _, r := range o.readers {
		if r.title != s.Title || r.t.Before(s.Start) || r.t.After(s.End) {
			continue
		}
		samples++
		if !r.state.Complete && r.state.Priority < types.PiecePriorityNow {
			below++
		}
		if len(seen) < 8 {
			seen = append(seen, fmt.Sprintf("+%s ms piece %d %s", report.Num(o.ms(r.t)-at), r.piece, stateName(r.state)))
		}
	}
	readers := fmt.Sprintf("the reader over the stall: %s", orDash(strings.Join(seen, ", ")))
	if below >= minBelowNow && o.activeDuring(t.Hash, s.Start, s.End) {
		out = append(out, report.Finding{Rule: "stall-below-now", AtMS: at, Refs: plot,
			Hint:     "the reader's piece was below Now while the player stalled, downloads on: the prioritizer's window didn't cover the playhead",
			Evidence: fmt.Sprintf("%s; %d of %d reader samples found its piece incomplete below Now; %s", what, below, samples, readers)})
	}

	sent, paused, ok := o.seederRate(t.Hash, s.Start.Add(-stallLookback), s.End)
	if !ok {
		return out
	}
	got, _ := o.downloadRate(t.Hash, s.Start.Add(-stallLookback), s.End)
	rates := fmt.Sprintf("over the stall and the %v before it, the seeder sent %s/s and the leecher got %s/s; the bitrate is %s/s",
		stallLookback, report.Bytes(int64(sent)), report.Bytes(int64(got)), report.Bytes(o.cfg.Bitrate))
	switch {
	case sent < float64(o.cfg.Bitrate):
		hint := "expected: the swarm sent less than the bitrate"
		if paused {
			hint += " (the seeder was paused)"
		}
		out = append(out, report.Finding{Rule: "starved", AtMS: at, Refs: plot, Hint: hint, Evidence: what + "; " + rates})
	case len(out) == 0:
		out = append(out, report.Finding{Rule: "stall-unexplained", AtMS: at, Refs: plot,
			Hint:     "the swarm sent more than the bitrate and the reader's piece was at Now: the delay is on our side (requests, the read path); see the timeline",
			Evidence: what + "; " + rates + "; " + readers})
	}
	return out
}

// activeDuring reports whether the torrent's downloads were on (its
// activity state active) at every sample in [a, b].
func (o *Observer) activeDuring(hash string, a, b time.Time) bool {
	for _, s := range o.torrents {
		if s.hash == hash && !s.t.Before(a) && !s.t.After(b) && s.state != "active" {
			return false
		}
	}
	return true
}

// seederRate is what the torrent's seeder sent per second over [a, b]
// (stretched to two samples at least), and whether it was paused then.
func (o *Observer) seederRate(hash string, a, b time.Time) (rate float64, paused, ok bool) {
	var pts []point
	for _, s := range o.seeds {
		if s.hash == hash {
			pts = append(pts, point{s.t, float64(s.sent)})
			if !s.t.Before(a) && !s.t.After(b) && s.paused {
				paused = true
			}
		}
	}
	rate, ok = rateOver(pts, a, b)
	return rate, paused, ok
}

// downloadRate is the useful data the leecher got per second over [a, b].
func (o *Observer) downloadRate(hash string, a, b time.Time) (float64, bool) {
	var pts []point
	for _, s := range o.torrents {
		if s.hash == hash {
			pts = append(pts, point{s.t, float64(s.useful)})
		}
	}
	return rateOver(pts, a, b)
}

type point struct {
	t time.Time
	v float64
}

// rateOver is a cumulative series' rate over [a, b]: from the last point
// at or before a (the first, when none is) to the first at or after b
// (the last, when none is).
func rateOver(pts []point, a, b time.Time) (float64, bool) {
	if len(pts) < 2 {
		return 0, false
	}
	i := 0
	for i+1 < len(pts) && !pts[i+1].t.After(a) {
		i++
	}
	j := len(pts) - 1
	for j-1 > i && !pts[j-1].t.Before(b) {
		j--
	}
	if j <= i {
		return 0, false
	}
	return (pts[j].v - pts[i].v) / pts[j].t.Sub(pts[i].t).Seconds(), true
}

// competingWindows finds two or more readers of one file farther apart
// than the streaming window (urgent and readahead), for two samples or
// more: their windows don't overlap, so they compete for the swarm.
func (o *Observer) competingWindows() []report.Finding {
	window := o.cfg.Stack.Streaming.UrgentBufferBytes + o.cfg.Stack.Streaming.ReadaheadBytes
	type episode struct {
		from, to time.Time
		n        int
		readers  int
		spread   int64
		at       []int64
	}
	var out []report.Finding
	open := map[int]*episode{}
	closeEp := func(ti int) {
		ep := open[ti]
		delete(open, ti)
		if ep == nil || ep.n < 2 {
			return
		}
		t := o.cfg.Titles[ti]
		out = append(out, report.Finding{Rule: "competing-windows", AtMS: o.ms(ep.from),
			Refs: []string{"timeline.ndjson", "timeline-" + slug(t.Name) + ".svg"},
			Hint: "readers of one file farther apart than the streaming window: their windows compete for the swarm",
			Evidence: fmt.Sprintf("%s: up to %d readers as far as %s apart (window %s) for %v from %s ms; first seen at %s",
				t.Name, ep.readers, report.Bytes(ep.spread), report.Bytes(window), ep.to.Sub(ep.from).Round(time.Millisecond),
				report.Num(o.ms(ep.from)), positions(ep.at))})
	}
	for i := 0; i < len(o.readers); {
		now := o.readers[i].t
		byTitle := map[int][]int64{}
		for ; i < len(o.readers) && o.readers[i].t.Equal(now); i++ {
			byTitle[o.readers[i].title] = append(byTitle[o.readers[i].title], o.readers[i].pos)
		}
		for ti := range o.cfg.Titles {
			pos := byTitle[ti]
			if len(pos) < 2 || slices.Max(pos)-slices.Min(pos) <= window {
				closeEp(ti)
				continue
			}
			ep := open[ti]
			if ep == nil {
				ep = &episode{from: now, at: slices.Sorted(slices.Values(pos))}
				open[ti] = ep
			}
			ep.to, ep.n = now, ep.n+1
			ep.readers = max(ep.readers, len(pos))
			ep.spread = max(ep.spread, slices.Max(pos)-slices.Min(pos))
		}
	}
	for ti := range o.cfg.Titles {
		closeEp(ti)
	}
	return out
}

func positions(ps []int64) string {
	s := make([]string, len(ps))
	for i, p := range ps {
		s[i] = report.Bytes(p)
	}
	return strings.Join(s, ", ")
}

// abortLags names the handlers that outlived their client's drop by more
// than AbortLagMax: F12.
func (o *Observer) abortLags(lags []Lag) []report.Finding {
	var out []report.Finding
	for _, l := range lags {
		if l.Lag <= o.cfg.AbortLagMax {
			continue
		}
		state := "then returned"
		if !l.Done {
			state = "and was still running"
		}
		out = append(out, report.Finding{Rule: "handler-lag", Known: "F12", Refs: []string{l.Request},
			Hint:     "a handler outlived its client's drop: its read blocks under context.Background, not the request's (F12, #217)",
			Evidence: fmt.Sprintf("request %s's handler ran %v after its client dropped it, %s", l.Request, l.Lag.Round(time.Millisecond), state)})
	}
	return out
}

// sniffs finds, in --diagnose's prioritizer records, a reader prioritized
// at offset 0 while the requests open on its file asked for ranges above
// 0 (or none, a HEAD or PROPFIND): content sniffed at offset 0 (F1).
func (o *Observer) sniffs(logs []LogEvent, reqs []stack.Request) []report.Finding {
	type hit struct {
		first time.Time
		n     int
		ids   []string
	}
	hits := map[int]*hit{}
	var order []int
	for _, e := range logs {
		if e.Component != "prioritizer" || e.Msg != "updated seek priorities" || e.Get("offset") != "0" {
			continue
		}
		ti, ok := o.titleByFile(e.Get("file"))
		if !ok {
			continue
		}
		path := o.cfg.Titles[ti].Path
		var open []stack.Request
		atZero := false
		for _, r := range reqs {
			if r.Path != path || r.Start.After(e.T) || (r.Done() && r.End.Before(e.T)) {
				continue
			}
			open = append(open, r)
			atZero = atZero || (r.Method == "GET" && rangeStart(r.Range) == 0)
		}
		if len(open) == 0 || atZero {
			continue
		}
		h := hits[ti]
		if h == nil {
			h = &hit{first: e.T}
			hits[ti] = h
			order = append(order, ti)
		}
		h.n++
		for _, r := range open {
			desc := r.ID + " " + r.Method
			if r.Range != "" {
				desc += " " + r.Range
			}
			if !slices.Contains(h.ids, desc) && len(h.ids) < 6 {
				h.ids = append(h.ids, desc)
			}
		}
	}
	var out []report.Finding
	for _, ti := range order {
		h := hits[ti]
		out = append(out, report.Finding{Rule: "sniff-at-zero", Known: "F1", AtMS: o.ms(h.first), Refs: append([]string{"logs.ndjson"}, h.ids...),
			Hint: "a reader was prioritized at offset 0 though no request open on the file asked for it: content sniffed at offset 0 (F1)",
			Evidence: fmt.Sprintf("%d UpdateForSeek(0) of %s while only these were open: %s", h.n, o.cfg.Titles[ti].Name,
				strings.Join(h.ids, "; "))})
	}
	return out
}

// titleByFile is the title of an anacrolix file path (the prioritizer's
// "file" attribute).
func (o *Observer) titleByFile(path string) (int, bool) {
	for i, t := range o.cfg.Titles {
		if p, ok := o.watches[t.Hash].fileAt(t.Offset); ok && p == path {
			return i, true
		}
	}
	return 0, false
}

// tripFindings points at each watchdog trip's dumps.
func (o *Observer) tripFindings() []report.Finding {
	var out []report.Finding
	for _, t := range o.trips {
		var rs []string
		for _, r := range t.readers {
			rs = append(rs, fmt.Sprintf("%s at %d (piece %d, %s)", r.Title, r.Position, r.Piece, r.State))
		}
		out = append(out, report.Finding{Rule: "watchdog", Known: t.known, AtMS: o.ms(t.at), Refs: t.artifacts,
			Hint:     fmt.Sprintf("no progress for %v with requests active and a read blocked: %s", t.stuck.Round(time.Millisecond), t.why),
			Evidence: "readers: " + strings.Join(rs, "; ")})
	}
	return out
}

// storageLoss finds anacrolix reads that gave up after resyncing the
// piece's completion: the storage lost data anacrolix believed it had
// (F11). Each failure's piece is its chain's last "error reading".
func (o *Observer) storageLoss(logs []LogEvent) []report.Finding {
	type loss struct {
		first  time.Time
		n      int
		pieces []string
		err    string
	}
	byTorrent := map[string]*loss{}
	var order []string
	for i, r := range logs {
		if r.Msg != MsgResyncFailed {
			continue
		}
		ih := r.Get("torrent.ih")
		piece := "?"
		for j := i - 1; j >= 0 && !logs[j].T.Before(r.T.Add(-proofWindow)); j-- {
			if logs[j].Msg == MsgReadError && sameTorrent(logs[j], ih) {
				piece = logs[j].Get("piece")
				break
			}
		}
		l := byTorrent[ih]
		if l == nil {
			l = &loss{first: r.T, err: r.Get("err")}
			byTorrent[ih] = l
			order = append(order, ih)
		}
		l.n++
		if !slices.Contains(l.pieces, piece) {
			l.pieces = append(l.pieces, piece)
		}
	}
	var out []report.Finding
	for _, ih := range order {
		l := byTorrent[ih]
		out = append(out, report.Finding{Rule: "storage-loss", Known: "F11", AtMS: o.ms(l.first), Refs: []string{"logs.ndjson"},
			Hint: "anacrolix gave up reading data it believed it had, after resyncing the piece's completion: the piece cache lost it (F11, #215)",
			Evidence: fmt.Sprintf("%d reads of torrent %s failed after the completion resync, on pieces %s (err %s)", l.n, orDash(ih),
				strings.Join(l.pieces, ", "), orDash(l.err))})
	}
	return out
}

// capPerRule keeps maxPerRule findings of each rule and counts the rest.
func capPerRule(fs []report.Finding) []report.Finding {
	n := map[string]int{}
	var rules []string
	var out []report.Finding
	for _, f := range fs {
		if n[f.Rule] == 0 {
			rules = append(rules, f.Rule)
		}
		n[f.Rule]++
		if n[f.Rule] <= maxPerRule {
			out = append(out, f)
		}
	}
	for _, rule := range rules {
		if k := n[rule]; k > maxPerRule {
			out = append(out, report.Finding{Rule: rule, Hint: fmt.Sprintf("%d more like the above", k-maxPerRule),
				Evidence: "the rule's other findings, left out"})
		}
	}
	return out
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
