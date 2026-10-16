package checks

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"regexp"
	"runtime/pprof"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/shapedtime/momoshtrem/internal/sessions"
	"github.com/shapedtime/momoshtrem/internal/streamlab/stack"
)

// The gauges that must drop back to 0 once requests end.
const (
	gaugeOpenFiles    = "momoshtrem_streaming_open_files"
	gaugeBlockedReads = "momoshtrem_streaming_blocked_reads"
	gaugeInflight     = "momoshtrem_webdav_requests_inflight"
)

// SessionSkew is how far Quiescence moves the sessions clock: past
// sessions.Timeout, so every session whose reads are done ends.
const SessionSkew = sessions.Timeout + time.Second

// pollInterval is how often Quiescence looks while it waits.
const pollInterval = 50 * time.Millisecond

// QuiescenceResult is the stack's state once its requests ended: all
// zero is quiet. It's the leak signal that matters, because production
// never tears down.
type QuiescenceResult struct {
	// Waited is how long it took to settle; the full settle time when it
	// didn't.
	Waited time.Duration
	// Requests are the lab middleware's requests still running.
	Requests int
	// Readers are the PriorityReaders still registered.
	Readers int
	// OpenFiles, BlockedReads and InFlight are the gauges, from the
	// stack's registry (InFlight summed over methods).
	OpenFiles, BlockedReads, InFlight float64
	// Leaked are the goroutines that carry a request's label and aren't
	// Inherited: a read, a format detection or a handler that outlived its
	// request, or a goroutine one of them started on a library function
	// (go io.Copy, go t.Drop). Each entry is a stack, with its count and
	// request.
	Leaked []LabelledGoroutines
	// Inherited counts the labelled goroutines that run no momoshtrem code
	// and started in anacrolix code: a peer connection a request dialed,
	// the piece hashers that connection started. They live with the
	// torrent, so they aren't leaks.
	Inherited int
	// Sessions are the streaming sessions left after the sessions clock
	// was skewed by SessionSkew: those with a read still in flight.
	Sessions int
}

// LabelledGoroutines are goroutines with one stack and one request label.
type LabelledGoroutines struct {
	Request string // the RequestLabel value
	Count   int
	Stack   string // the goroutine profile's record (debug=1)
}

// Quiet reports whether nothing was left behind.
func (r QuiescenceResult) Quiet() bool {
	return r.Requests == 0 && r.Readers == 0 && r.OpenFiles == 0 && r.BlockedReads == 0 &&
		r.InFlight == 0 && len(r.Leaked) == 0 && r.Sessions == 0
}

// Err describes what was left behind; nil when quiet.
func (r QuiescenceResult) Err() error {
	if r.Quiet() {
		return nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "not quiet after %v: %d requests running, %d readers, gauges open_files=%v blocked_reads=%v requests_inflight=%v, %d sessions after a %v skew",
		r.Waited, r.Requests, r.Readers, r.OpenFiles, r.BlockedReads, r.InFlight, r.Sessions, SessionSkew)
	for _, g := range r.Leaked {
		fmt.Fprintf(&b, "\nleaked: %d goroutines of request %s:\n%s", g.Count, g.Request, g.Stack)
	}
	return fmt.Errorf("%s", b.String())
}

// Quiescence waits up to settle for the stack to be quiet after its last
// request, then skews the sessions clock by SessionSkew and counts the
// sessions left. Call it once every client request has ended (read,
// closed or aborted). The skew is for good: call it at the end of a run.
func Quiescence(ctx context.Context, s *stack.Stack, settle time.Duration) QuiescenceResult {
	start := time.Now()
	var r QuiescenceResult
	for {
		r = observe(s)
		if r.Quiet() || time.Since(start) >= settle || ctx.Err() != nil {
			break
		}
		select {
		case <-ctx.Done():
		case <-time.After(pollInterval):
		}
	}
	r.Waited = time.Since(start)
	s.Clock.Skew(SessionSkew)
	r.Sessions = s.Sessions.Count()
	return r
}

// observe reads the stack's state once, sessions aside.
func observe(s *stack.Stack) QuiescenceResult {
	r := QuiescenceResult{
		Requests: len(s.Requests.Active()),
		Readers:  s.Readers.Count(),
	}
	r.OpenFiles = Value(s.Registry, gaugeOpenFiles)
	r.BlockedReads = Value(s.Registry, gaugeBlockedReads)
	r.InFlight = Value(s.Registry, gaugeInflight)
	r.Leaked, r.Inherited = labelledGoroutines(stack.RequestLabel, s.Requests.Owns)
	return r
}

// Value sums the gauge or counter called name over its labels: 0 when it
// has none yet, NaN when the registry can't be read.
func Value(g prometheus.Gatherer, name string) float64 {
	mfs, err := g.Gather()
	if err != nil {
		return math.NaN()
	}
	var sum float64
	for _, mf := range mfs {
		if mf.GetName() != name {
			continue
		}
		for _, m := range mf.GetMetric() {
			sum += m.GetGauge().GetValue() + m.GetCounter().GetValue()
		}
	}
	return sum
}

// ownCode marks a stack frame of momoshtrem (the lab included).
const ownCode = "github.com/shapedtime/momoshtrem/"

// anacrolixCode marks a function of anacrolix's.
const anacrolixCode = "github.com/anacrolix/"

// labelledGoroutines reads the goroutine profile and returns the
// goroutines labelled with key, split in two. Inherited ones (others, a
// count) run no momoshtrem code and started in anacrolix code: anacrolix's
// own goroutines, which a request's goroutine triggered and which live
// with the torrent. A goroutine's stack starts at the function it was
// started with, so those show no momoshtrem frame. Every other one is own:
// it runs momoshtrem code, or momoshtrem started it on a library function
// (go io.Copy). Only labels mine accepts count: the requests of one stack.
func labelledGoroutines(key string, mine func(string) bool) (own []LabelledGoroutines, others int) {
	var buf bytes.Buffer
	if err := pprof.Lookup("goroutine").WriteTo(&buf, 1); err != nil {
		return []LabelledGoroutines{{Stack: "reading the goroutine profile: " + err.Error()}}, 0
	}
	return parseLabelled(buf.String(), key, mine)
}

// countLine is a debug=1 record's first line: "<count> @ <pcs>".
var countLine = regexp.MustCompile(`^(\d+) @`)

// parseLabelled parses a debug=1 goroutine profile: a "goroutine profile:
// total N" line, then records separated by blank lines, each "<count> @
// …", then "# labels: {…}" when the goroutines carry labels, then the
// frames.
func parseLabelled(profile, key string, mine func(string) bool) (own []LabelledGoroutines, others int) {
	label := regexp.MustCompile(`"` + regexp.QuoteMeta(key) + `":"([^"]*)"`)
	for _, rec := range strings.Split(profile, "\n\n") {
		rec = strings.Trim(rec, "\n")
		if strings.HasPrefix(rec, "goroutine profile:") {
			_, rec, _ = strings.Cut(rec, "\n")
		}
		m := countLine.FindStringSubmatch(rec)
		if m == nil {
			continue
		}
		var req string
		for _, line := range strings.Split(rec, "\n") {
			if strings.HasPrefix(line, "# labels: ") {
				if lm := label.FindStringSubmatch(line); lm != nil {
					req = lm[1]
				}
				break
			}
		}
		if req == "" || !mine(req) {
			continue
		}
		n, _ := strconv.Atoi(m[1])
		if strings.Contains(rec, ownCode) || !strings.HasPrefix(rootFunc(rec), anacrolixCode) {
			own = append(own, LabelledGoroutines{Request: req, Count: n, Stack: rec})
		} else {
			others += n
		}
	}
	return own, others
}

// rootFunc returns the function a debug=1 record's goroutines started
// with: the one in its last frame line, "#\t0x…\t<func>+0x…\t<file>:<line>".
// "" when the record has no frame.
func rootFunc(rec string) string {
	lines := strings.Split(rec, "\n")
	last := lines[len(lines)-1]
	if !strings.HasPrefix(last, "#\t") {
		return ""
	}
	fields := strings.Split(last, "\t")
	if len(fields) < 3 {
		return ""
	}
	return fields[2]
}
