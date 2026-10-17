package observe

import (
	"cmp"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/shapedtime/momoshtrem/internal/streamlab/report"
	"github.com/shapedtime/momoshtrem/internal/streamlab/stack"
)

// timelineEvent is a line of timeline.ndjson: pieces changing state, the
// readers' positions, requests, stalls, faults and watchdog trips, in
// time order, times in milliseconds from the observer's start.
type timelineEvent struct {
	TMS  float64 `json:"t_ms"`
	Kind string  `json:"kind"`
	// piece
	Torrent  string `json:"torrent,omitempty"`
	Piece    *int   `json:"piece,omitempty"`
	State    string `json:"state,omitempty"`
	Complete bool   `json:"complete,omitempty"`
	// reader, request, stall
	Title   string  `json:"title,omitempty"`
	Pos     *int64  `json:"pos,omitempty"`
	Lead    *int64  `json:"lead,omitempty"`
	ID      string  `json:"id,omitempty"`
	Method  string  `json:"method,omitempty"`
	Range   string  `json:"range,omitempty"`
	Status  int     `json:"status,omitempty"`
	Bytes   int64   `json:"bytes,omitempty"`
	Aborted bool    `json:"aborted,omitempty"`
	Device  string  `json:"device,omitempty"`
	Name    string  `json:"name,omitempty"`
	EndMS   float64 `json:"end_ms,omitempty"`
	Known   string  `json:"known,omitempty"`
}

// writeTimeline writes timeline.ndjson.
func (o *Observer) writeTimeline(f Facts, hist map[string]history, reqs []stack.Request) (string, error) {
	var evs []timelineEvent
	for _, h := range o.hashes {
		short := h[:min(8, len(h))]
		for _, e := range hist[h].events {
			i := e.index
			evs = append(evs, timelineEvent{TMS: o.ms(e.t), Kind: "piece", Torrent: short, Piece: &i,
				State: mirrorState(e.state), Complete: e.state.Complete})
		}
	}
	for _, r := range o.readers {
		pos, lead, piece := r.pos, r.lead, r.piece
		evs = append(evs, timelineEvent{TMS: o.ms(r.t), Kind: "reader", Title: o.cfg.Titles[r.title].Name, Pos: &pos,
			Lead: &lead, Piece: &piece, State: stateName(r.state)})
	}
	for _, r := range reqs {
		end := o.ms(r.End)
		if !r.Done() {
			end = o.ms(o.stopped)
		}
		evs = append(evs, timelineEvent{TMS: o.ms(r.Start), Kind: "request", ID: r.ID, Method: r.Method, Title: r.Path,
			Range: r.Range, Status: r.Status, Bytes: r.Bytes, Aborted: r.Aborted, EndMS: end})
	}
	for _, s := range f.Stalls {
		pos := s.Position
		evs = append(evs, timelineEvent{TMS: o.ms(s.Start), Kind: "stall", Device: s.Device, Title: o.cfg.Titles[s.Title].Name,
			Pos: &pos, EndMS: o.ms(s.End)})
	}
	for _, ft := range o.faultList() {
		evs = append(evs, timelineEvent{TMS: o.ms(ft.start), Kind: "fault", Name: ft.name, Torrent: ft.key, EndMS: o.ms(ft.end)})
	}
	for _, t := range o.trips {
		evs = append(evs, timelineEvent{TMS: o.ms(t.at), Kind: "trip", Known: t.known, Name: t.why})
	}
	slices.SortStableFunc(evs, func(a, b timelineEvent) int { return cmp.Compare(a.TMS, b.TMS) })
	return writeNDJSON(o.cfg.Dir, "timeline.ndjson", evs)
}

// titleTimelines sums up each title's pieces against its readers, and
// plots the ones that played.
func (o *Observer) titleTimelines(f Facts, hist map[string]history, reqs []stack.Request) []report.TitleTimeline {
	var out []report.TitleTimeline
	for ti, t := range o.cfg.Titles {
		h := hist[t.Hash]
		first, last := t.piece(0), t.piece(t.Size-1)
		tl := report.TitleTimeline{Title: t.Name, Pieces: last - first + 1}
		// A reader seen at an incomplete piece waited for it, from that
		// sample until the piece completed.
		waitFrom := map[int]time.Time{}
		var leads []float64
		for _, r := range o.readers {
			if r.title != ti {
				continue
			}
			tl.ReaderSamples++
			leads = append(leads, float64(r.lead))
			if r.state.Complete {
				continue
			}
			tl.AtIncomplete++
			if _, ok := waitFrom[r.piece]; !ok {
				waitFrom[r.piece] = r.t
			}
		}
		var waits []float64
		for _, c := range h.completions(first, last) {
			tl.Completed++
			if from, ok := waitFrom[c.index]; ok && !c.t.Before(from) {
				waits = append(waits, report.MS(c.t.Sub(from)))
				delete(waitFrom, c.index)
			} else {
				tl.Ahead++
			}
		}
		tl.Waited = len(waits)
		if len(waits) > 0 {
			tl.WaitP50MS, tl.WaitP95MS = report.Quantile(waits, 0.5), report.Quantile(waits, 0.95)
		}
		if len(leads) > 0 {
			tl.LeadP50, tl.LeadMin = int64(report.Quantile(leads, 0.5)), int64(slices.Min(leads))
		}
		if tl.ReaderSamples == 0 && tl.Completed == 0 {
			continue
		}
		if o.cfg.Dir != "" {
			name := "timeline-" + slug(t.Name) + ".svg"
			svg := o.plot(ti, h, f, reqs)
			if os.WriteFile(filepath.Join(o.cfg.Dir, name), []byte(svg), 0o644) == nil {
				tl.Plot = name
			}
		}
		out = append(out, tl)
	}
	return out
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

func slug(s string) string {
	return strings.Trim(nonSlug.ReplaceAllString(strings.ToLower(s), "-"), "-")
}

// The plot's geometry.
const (
	plotW, plotH                   = 960, 440
	padL, padR, padT, padB float64 = 64, 16, 40, 40
)

// plot draws a title's timeline as SVG: time across, the file offset up.
// Readers' positions are blue dots, pieces completing green marks, GETs
// orange lines at their range's start, stalls red bands, faults grey
// bands, watchdog trips dashed lines.
func (o *Observer) plot(ti int, h history, f Facts, reqs []stack.Request) string {
	t := o.cfg.Titles[ti]
	span := math.Max(o.stopped.Sub(o.start).Seconds(), 1)
	size := math.Max(float64(t.Size), 1)
	w, hgt := float64(plotW)-padL-padR, float64(plotH)-padT-padB
	x := func(at time.Time) float64 { return padL + math.Min(math.Max(at.Sub(o.start).Seconds()/span, 0), 1)*w }
	y := func(off int64) float64 { return padT + (1-math.Min(math.Max(float64(off)/size, 0), 1))*hgt }

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d" font-family="sans-serif" font-size="11">`+"\n", plotW, plotH, plotW, plotH)
	fmt.Fprintf(&b, `<rect width="100%%" height="100%%" fill="#fff"/>`+"\n")
	fmt.Fprintf(&b, `<text x="%.0f" y="16" font-size="13" font-weight="bold">%s: pieces against the readers</text>`+"\n", padL, xmlEscape(t.Name))
	legend := []struct{ color, label string }{{"#2563eb", "reader position"}, {"#16a34a", "piece complete"}, {"#f59e0b", "GET (range start)"},
		{"#dc2626", "stall"}, {"#6b7280", "fault"}, {"#111827", "watchdog trip"}}
	lx := padL
	for _, l := range legend {
		fmt.Fprintf(&b, `<rect x="%.0f" y="24" width="10" height="10" fill="%s"/><text x="%.0f" y="33">%s</text>`+"\n", lx, l.color, lx+14, l.label)
		lx += 30 + float64(len(l.label))*6
	}

	for _, s := range f.Stalls {
		if s.Title == ti {
			fmt.Fprintf(&b, `<rect x="%.1f" y="%.0f" width="%.1f" height="%.0f" fill="#dc2626" fill-opacity="0.18"/>`+"\n",
				x(s.Start), padT, math.Max(x(s.End)-x(s.Start), 1), hgt)
		}
	}
	for _, ft := range o.faultList() {
		if ft.key != "" && ft.key != t.FaultKey() {
			continue
		}
		fmt.Fprintf(&b, `<rect x="%.1f" y="%.0f" width="%.1f" height="%.0f" fill="#6b7280" fill-opacity="0.15"/>`+"\n",
			x(ft.start), padT, math.Max(x(ft.end)-x(ft.start), 1), hgt)
	}

	// Axes and ticks.
	fmt.Fprintf(&b, `<g stroke="#9ca3af" stroke-width="1"><line x1="%.0f" y1="%.0f" x2="%.0f" y2="%.0f"/><line x1="%.0f" y1="%.0f" x2="%.0f" y2="%.0f"/></g>`+"\n",
		padL, padT+hgt, padL+w, padT+hgt, padL, padT, padL, padT+hgt)
	step := niceStep(span, 10, []float64{0.5, 1, 2, 5, 10, 15, 30, 60, 120, 300})
	for s := 0.0; s <= span+1e-9; s += step {
		px := padL + s/span*w
		fmt.Fprintf(&b, `<line x1="%.1f" y1="%.0f" x2="%.1f" y2="%.0f" stroke="#e5e7eb"/><text x="%.1f" y="%.0f" text-anchor="middle">%s s</text>`+"\n",
			px, padT, px, padT+hgt, px, padT+hgt+14, strconv.FormatFloat(s, 'f', -1, 64))
	}
	mib := size / (1 << 20)
	ystep := niceStep(mib, 8, []float64{1, 2, 4, 8, 16, 32, 64, 128, 256, 512, 1024, 2048, 4096})
	for m := 0.0; m <= mib+1e-9; m += ystep {
		py := y(int64(m * (1 << 20)))
		fmt.Fprintf(&b, `<line x1="%.0f" y1="%.1f" x2="%.0f" y2="%.1f" stroke="#e5e7eb"/><text x="%.0f" y="%.1f" text-anchor="end">%s MiB</text>`+"\n",
			padL, py, padL+w, py, padL-4, py+4, strconv.FormatFloat(m, 'f', -1, 64))
	}
	fmt.Fprintf(&b, `<text x="%.0f" y="%d" text-anchor="middle" fill="#6b7280">time since the stack started</text>`+"\n", padL+w/2, plotH-6)

	// GETs at their range's start, from start to end.
	for _, r := range reqs {
		if r.Path != t.Path || r.Method != "GET" {
			continue
		}
		end := r.End
		if !r.Done() {
			end = o.stopped
		}
		ry := y(rangeStart(r.Range))
		fmt.Fprintf(&b, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="#f59e0b" stroke-width="2" stroke-opacity="0.7"><title>%s %s</title></line>`+"\n",
			x(r.Start), ry, math.Max(x(end), x(r.Start)+1), ry, r.ID, xmlEscape(r.Range))
	}
	// Pieces completing.
	b.WriteString(`<g fill="#16a34a">` + "\n")
	for _, e := range h.completions(t.piece(0), t.piece(t.Size-1)) {
		mid := int64(e.index)*h.pieceLen + h.pieceLen/2 - t.Offset
		fmt.Fprintf(&b, `<rect x="%.1f" y="%.1f" width="3" height="2"/>`+"\n", x(e.t)-1.5, y(mid)-1)
	}
	b.WriteString("</g>\n")
	// Readers.
	b.WriteString(`<g fill="#2563eb">` + "\n")
	for _, r := range o.readers {
		if r.title == ti {
			fmt.Fprintf(&b, `<circle cx="%.1f" cy="%.1f" r="1.8"/>`+"\n", x(r.t), y(r.pos))
		}
	}
	b.WriteString("</g>\n")
	for _, tr := range o.trips {
		fmt.Fprintf(&b, `<line x1="%.1f" y1="%.0f" x2="%.1f" y2="%.0f" stroke="#111827" stroke-dasharray="4 3"><title>watchdog: %s</title></line>`+"\n",
			x(tr.at), padT, x(tr.at), padT+hgt, xmlEscape(tr.why))
	}
	b.WriteString("</svg>\n")
	return b.String()
}

// niceStep is the smallest of steps that cuts span into at most n parts.
func niceStep(span float64, n int, steps []float64) float64 {
	for _, s := range steps {
		if span/s <= float64(n) {
			return s
		}
	}
	return steps[len(steps)-1] * math.Ceil(span/steps[len(steps)-1]/float64(n))
}

// rangeStart is a Range header's first byte; 0 without one.
func rangeStart(rng string) int64 {
	v, _, _ := strings.Cut(strings.TrimPrefix(rng, "bytes="), "-")
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0
	}
	return n
}

func xmlEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;").Replace(s)
}

// writeNDJSON writes one JSON value per line to dir/name; "" when there's
// no folder.
func writeNDJSON[T any](dir, name string, vs []T) (string, error) {
	if dir == "" {
		return "", nil
	}
	f, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		return "", err
	}
	enc := json.NewEncoder(f)
	for _, v := range vs {
		if err := enc.Encode(v); err != nil {
			f.Close()
			return "", err
		}
	}
	return name, f.Close()
}
