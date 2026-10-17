package scenario

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"os"
	"path/filepath"
	"runtime/pprof"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/shapedtime/momoshtrem/internal/config"
	"github.com/shapedtime/momoshtrem/internal/streamlab/checks"
	"github.com/shapedtime/momoshtrem/internal/streamlab/client"
	"github.com/shapedtime/momoshtrem/internal/streamlab/known"
	"github.com/shapedtime/momoshtrem/internal/streamlab/report"
	"github.com/shapedtime/momoshtrem/internal/streamlab/stack"
)

// The stress tier (PLAN §5 "Tiers", D8): the read path's concurrency
// under seeded randomness and many iterations, for streamlab check, which
// runs it in a -race build (S7, #178). Its scenarios guard races,
// deadlocks and leaks, not timings: a stress run's timings come from a
// race build and never get a verdict.
//
// Every stress scenario runs production's concurrent readers of streaming
// state beside its script (pollers): the reader tracker, the torrent
// service's lists, stats and details, the activity manager, the sessions,
// the tree and a metrics scrape, as Activity's API, the metrics collector
// and PROPFINDs read them while streams play.
//
// S12 (#183) adds its tree-race scenarios (InvalidateTree during opens:
// F4, F5) to this tier the same way: a file of its own whose init appends
// them to the catalog, with Tier Stress.

// Stress is the tier streamlab check runs with -race.
const Stress Tier = "stress"

// stressTier is the tier's scenarios, in the order a run takes them.
var stressTier = []*Scenario{
	closeBlockedRead,
	seekPendingRead,
	abortBlockedRead,
	removePauseIdle,
	removeWhileHashing,
	concurrentAdds,
	manyReaders,
}

// The stress tier's checks and metrics; the race check is check's, which
// adds it to every scenario it runs.
const (
	checkOpsBounded        = "ops-bounded"
	checkTorrentConsistent = "torrent-consistent"
)

func init() {
	catalog = append(catalog, stressTier...)
	Checks = append(Checks,
		CheckDef{ID: known.RaceCheck, Doc: "The repetition's process reported no data race (streamlab check's -race build: the GORACE log of each repetition). " +
			"A known race explains a failure only when every race of the repetition carries its exact signature: both accesses' kinds, their top frames with file and line, " +
			"and their top momoshtrem frames. A report cut off before its first access is an unparsed race: a fail."},
		CheckDef{ID: checkOpsBounded, Doc: "Every call the script made directly returned within its bound, plus 5 s of slack. " +
			"A ReadAt has twice the read timeout: a drain of the pending read, then the read. A Close after its read has one read timeout, since nothing should hold the file. " +
			"An Open or a RemoveTorrent has the add timeout plus the read timeout, for an add in flight. A call past its bound is stuck: a deadlock the hard timeout would only show " +
			"as a hung repetition. Its stuck-N.txt holds every goroutine. #230 explains a failure whose stuck calls are all removes and opens, each dump showing its lock cycle."},
		CheckDef{ID: checkTorrentConsistent, Doc: "Once the script's adds and removes are done, the torrent service and the anacrolix client agree on every library torrent: " +
			"loaded in both or in neither. A torrent the client still has that the service forgot leaks; one the service lists that the client dropped breaks every open."},
	)
	Metrics = append(Metrics,
		report.MetricDef{Name: "race_reports", Kind: report.Exact, Unit: "races", Better: report.Lower,
			Doc: "Distinct data races the repetition's process reported, by signature (streamlab check's race build only)."},
		report.MetricDef{Name: "stress_ops", Kind: report.Count, Unit: "ops", Better: report.None,
			Doc: "Operations the stress script completed, whatever their end: reads, requests, adds, removes, pauses, listings."},
		report.MetricDef{Name: "stress_short_reads", Kind: report.Count, Unit: "reads", Better: report.None,
			Doc: "Stress reads that ended before their range's end, as a read timeout, an abort or a removed torrent ends them: expected under stress."},
		report.MetricDef{Name: "op_latency_max", Kind: report.Timing, Unit: "ms", Better: report.Lower,
			Doc: "The slowest direct VFS call of a stress script (ops-bounded)."},
	)
}

// stressParams are the params every stress scenario redeclares (a short
// read timeout, so blocked reads time out often; a big file, so the
// script always finds pieces the leecher lacks; an unlimited seeder, so a
// release brings what reads wait on at once), with the scenario's own
// laid over them.
func stressParams(own ...Param) []Param {
	out := []Param{
		{Name: "read_timeout", Kind: Duration, Default: "2s"},
		{Name: "file_size", Kind: Bytes, Default: "256MiB"},
		{Name: "seeder_rate", Kind: Bytes, Default: "0"},
		{Name: "settle", Kind: Duration, Default: "20s"},
		{Name: "window", Kind: Bytes, Default: "4MiB", Doc: "streaming.readahead_bytes; urgent_buffer_bytes, header_priority_bytes and " +
			"footer_priority_bytes are a quarter of it. Smaller than production's (64 MiB), so a held seeder's release brings what reads " +
			"wait on at once and the file always has pieces the leecher lacks. The code paths are production's; only the sizes differ."},
	}
	for _, p := range own {
		if i := slices.IndexFunc(out, func(q Param) bool { return q.Name == p.Name }); i >= 0 {
			out[i] = p
			continue
		}
		out = append(out, p)
	}
	return out
}

// stressAddTimeout is the stress tier's torrent.add_timeout (production's
// is 60 s): it bounds how long a remove may wait for an add in flight.
const stressAddTimeout = 10 * time.Second

// stressStack is the stress tier's stack: the window param's streaming
// windows and stressAddTimeout, then tune (the scenario's own, optional),
// and the activity manager's check interval (idleCheck names its param;
// "" keeps production's).
func stressStack(tune func(Params, *config.Config), idleCheck string) func(Params) Stack {
	return func(p Params) Stack {
		st := Stack{Tune: func(c *config.Config) {
			c.Torrent.AddTimeout = int(stressAddTimeout.Seconds())
			w := p.Bytes("window")
			c.Streaming.ReadaheadBytes = w
			c.Streaming.UrgentBufferBytes = max(w/4, 1)
			c.Streaming.HeaderPriorityBytes = max(w/4, 1)
			c.Streaming.FooterPriorityBytes = max(w/4, 1)
			if tune != nil {
				tune(p, c)
			}
		}}
		if idleCheck != "" {
			st.IdleCheckInterval = p.Duration(idleCheck)
		}
		return st
	}
}

// stressTimeout is a stress repetition's hard timeout: a race build runs
// several times slower.
const stressTimeout = 6 * time.Minute

// opSlack is what a direct VFS call may take beyond its bound: a race
// build on a loaded machine.
const opSlack = 5 * time.Second

// stressRun is a stress script's view: the Env, plus the direct VFS calls
// it timed and what its reads came to.
type stressRun struct {
	*Env

	// base derives the script's strands (stress_rand.go); rng is the
	// script's own, for its sequential code only.
	base uint64
	rng  *rand.Rand

	ids atomic.Int64

	mu       sync.Mutex
	ops      int // operations ended (count, countShort)
	calls    int // direct VFS calls (call)
	slowest  time.Duration
	stuck    []string    // calls past their bound, for the evidence
	stuckOn  []stuckCall // the same calls, for their known signature
	cycle    bool        // a stuck call's dump showed #230's cycle
	outcomes map[string]int
	short    int
}

// stressScript wraps a stress script: the pollers run beside it, every
// seeder is released after it, and its ops-bounded check and metrics are
// reported.
func stressScript(script func(*stressRun) error) func(*Env) error {
	return func(e *Env) error {
		// Env.RNG is seeded with the seed and the scenario: its first draw
		// derives every strand.
		base := e.RNG.Uint64()
		s := &stressRun{Env: e, base: base, rng: strand(base, strandScript), outcomes: map[string]int{}}
		e.quietKnown = func(q checks.QuiescenceResult) string {
			s.mu.Lock()
			cycle := s.cycle
			s.mu.Unlock()
			return quietKnown(q, e.Counter(metricReadTimeouts), cycle)
		}
		ctx, stop := context.WithCancel(e.Ctx)
		polled := s.poll(ctx)
		err := script(s)
		stop()
		polled()
		for _, t := range e.Titles {
			t.Seeder.ReleaseUploads()
		}
		s.report()
		return err
	}
}

// report records the stress metrics and the ops-bounded check.
func (s *stressRun) report() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Metric("stress_ops", float64(s.ops))
	s.Metric("stress_short_reads", float64(s.short))
	if s.slowest > 0 {
		s.Metric("op_latency_max", report.MS(s.slowest))
	}
	if len(s.outcomes) > 0 {
		keys := make([]string, 0, len(s.outcomes))
		for k := range s.outcomes {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var parts []string
		for _, k := range keys {
			parts = append(parts, fmt.Sprintf("%d %s", s.outcomes[k], k))
		}
		s.Note("outcomes: %s", strings.Join(parts, ", "))
	}
	// The scenarios that make direct calls declare ops-bounded; the
	// collector drops a check the scenario doesn't declare.
	switch {
	case s.calls == 0:
		s.Check(checkOpsBounded, report.NA, "", "the script made no direct VFS call")
	case len(s.stuck) > 0:
		s.Check(checkOpsBounded, report.Fail, stuckKnown(s.stuckOn),
			fmt.Sprintf("%d of %d calls past their bound: %s", len(s.stuck), s.calls, quote(first3(s.stuck))))
	default:
		s.Check(checkOpsBounded, report.Pass, "", fmt.Sprintf("%d direct calls, the slowest %v", s.calls, s.slowest.Round(time.Millisecond)))
	}
}

// label is a request ID for a direct VFS call: the stack's prefix, so the
// quiescence check counts what the call leaves running as the stack's.
func (s *stressRun) label() string {
	return fmt.Sprintf("%s.vfs%d", s.Stack.ID, s.ids.Add(1))
}

// call runs fn as a direct VFS call: in a goroutine of its own under a
// request label (goroutines it starts inherit it), waiting up to bound
// plus opSlack. A call past that is stuck: it's recorded (ops-bounded),
// left running, and call returns false.
func (s *stressRun) call(name string, bound time.Duration, fn func()) bool {
	done := make(chan struct{})
	start := time.Now()
	id := s.label()
	go pprof.Do(context.Background(), pprof.Labels(stack.RequestLabel, id), func(context.Context) {
		defer close(done)
		fn()
	})
	t := time.NewTimer(bound + opSlack)
	defer t.Stop()
	select {
	case <-done:
		took := time.Since(start)
		s.mu.Lock()
		s.calls++
		s.slowest = max(s.slowest, took)
		s.mu.Unlock()
		return true
	case <-t.C:
		s.mu.Lock()
		s.calls++
		n := len(s.stuck) + 1
		s.stuck = append(s.stuck, fmt.Sprintf("%s (%s) still running after %v", name, id, bound+opSlack))
		s.mu.Unlock()
		cycle := deleteDeadlock(s.dumpStuck(n, name, id))
		s.mu.Lock()
		s.stuckOn = append(s.stuckOn, stuckCall{name: name, cycle: cycle})
		s.cycle = s.cycle || cycle
		s.mu.Unlock()
		return false
	}
}

// dumpStuck returns every goroutine's stack (debug=2) when a direct call
// is stuck, and writes it to the repetition's folder (stuck-<n>.txt): what
// holds what it waits for. The runner's SIGQUIT dump comes only with a
// hard timeout.
func (s *stressRun) dumpStuck(n int, name, id string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s (request label %s) was still running %v past its bound. Every goroutine:\n\n", name, id, opSlack)
	pprof.Lookup("goroutine").WriteTo(&b, 2)
	if s.dir == "" {
		return b.String()
	}
	file := fmt.Sprintf("stuck-%d.txt", n)
	if os.WriteFile(filepath.Join(s.dir, file), []byte(b.String()), 0o644) == nil {
		s.rec.mu.Lock()
		s.rec.artifacts[strings.TrimSuffix(file, ".txt")] = file
		s.rec.mu.Unlock()
	}
	return b.String()
}

// readAt reads len(p) bytes of t's file f at off as a direct call, checks
// what came and counts how it ended.
func (s *stressRun) readAt(f interface {
	ReadAt([]byte, int64) (int, error)
}, t *Title, p []byte, off int64) {
	var n int
	var err error
	ok := s.call(fmt.Sprintf("ReadAt(%d bytes at %d)", len(p), off), 2*s.Stack.ReadTimeout, func() {
		n, err = f.ReadAt(p, off)
	})
	if !ok {
		return
	}
	if n > 0 {
		t.Integrity.Check(t.File, off, p[:n])
	}
	switch {
	case err == nil:
		s.count("whole read")
	case errors.Is(err, context.DeadlineExceeded):
		s.countShort("read timed out")
	default:
		s.countShort("read failed: " + errClass(err))
	}
}

// count tallies an operation that ended as what (stress_ops).
func (s *stressRun) count(what string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ops++
	s.outcomes[what]++
}

// countShort tallies a read that ended early as what: stress_short_reads
// too.
func (s *stressRun) countShort(what string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ops++
	s.outcomes[what]++
	s.short++
}

// event tallies something that happened, not an operation: a round, a
// release.
func (s *stressRun) event(what string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.outcomes[what]++
}

// errClass names an error by its kind, for the outcome tally: the
// message without offsets and addresses.
func errClass(err error) string {
	msg := err.Error()
	for _, cut := range []string{": ", " at ", " ("} {
		if i := strings.LastIndex(msg, cut); i > 0 && len(msg) > 60 {
			msg = msg[i+len(cut):]
		}
	}
	if len(msg) > 60 {
		msg = msg[:60]
	}
	return msg
}

// get sends a Range GET of t on conn from off, n bytes (to the end for 0),
// and reads what comes, checking every byte. A short body is expected
// under stress (a read timeout, a removed torrent); a 206 with the wrong
// headers is an http failure. Cancelling ctx drops the request: an abort.
func (s *stressRun) get(ctx context.Context, conn *client.Conn, t *Title, off, n int64) {
	end := t.Size - 1
	if n > 0 {
		end = min(off+n-1, t.Size-1)
		n = end - off + 1
	}
	st, err := conn.Get(ctx, t.Path, http.Header{"Range": {rangeOf(off, n)}})
	if err != nil {
		if ctx.Err() != nil {
			s.countShort("aborted before the headers")
		} else {
			s.countShort("request failed: " + errClass(err))
		}
		return
	}
	defer st.Close()
	if st.Response.StatusCode != http.StatusPartialContent {
		s.countShort(fmt.Sprintf("status %d", st.Response.StatusCode))
		return
	}
	if herr := checks.RangeResponse(st.Response, off, end, t.Size); herr != nil {
		s.httpError(herr)
		return
	}
	buf := make([]byte, 32<<10)
	var got int64
	for {
		m, rerr := st.Read(buf)
		if m > 0 {
			t.Integrity.Check(t.File, off+got, buf[:m])
			got += int64(m)
		}
		if rerr == nil {
			continue
		}
		switch {
		case errors.Is(rerr, io.EOF) && got == end-off+1:
			s.count("whole body")
		case ctx.Err() != nil:
			s.countShort("aborted mid-body")
		default:
			s.countShort("short body")
		}
		return
	}
}

// incomplete is a random offset of t, 64 KiB aligned, whose piece the
// leecher lacks, from r (one draw: pickOffset): any offset before the
// torrent loads, and the last one tried when every try found a complete
// piece.
func (s *stressRun) incomplete(t *Title, r *rand.Rand) int64 {
	return pickOffset(r, t.Size, func(off int64) bool {
		tor, ok := s.Torrent(t)
		return ok && tor.Info() != nil && tor.PieceState(t.Piece(off)).Complete
	})
}

// rounds runs fn rounds times over t's held seeder: each round holds it,
// releases it at a random moment in [0, release) while fn runs, then
// releases it (if fn ended first) and gives the reads woken a moment. A
// release after a read timed out and its file closed wakes that read's
// orphaned goroutine against a closed reader; one before lets the read
// through. Workers of one round run fn(round, w, r) at once, each with its
// own strand r; the script's strand picks the release moments.
func (s *stressRun) rounds(t *Title, rounds int64, release time.Duration, fn func(round, w int, r *rand.Rand) error) error {
	for round := range int(rounds) {
		if err := s.Ctx.Err(); err != nil {
			return err
		}
		t.Seeder.HoldUploads()
		at := time.Duration(intn(s.rng, int64(release)+1))
		pause := time.Duration(100+intn(s.rng, 400)) * time.Millisecond
		timer := time.AfterFunc(at, func() { t.Seeder.ReleaseUploads() })
		err := s.inRound(round+1, func(w int, r *rand.Rand) error { return fn(round, w, r) })
		timer.Stop()
		t.Seeder.ReleaseUploads()
		s.event("round")
		if err != nil {
			return err
		}
		if err := s.Sleep(pause); err != nil {
			return err
		}
	}
	return nil
}

// sleepCtx waits d; false when ctx ended first.
func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

// poll runs production's concurrent readers of streaming state until ctx
// ends, one at a time at random every 5-40 ms: what Activity's API, the
// metrics collector, PROPFINDs and the debug endpoints read while streams
// play. wait returns once it stopped.
func (s *stressRun) poll(ctx context.Context) (wait func()) {
	done := make(chan struct{})
	r := strand(s.base, strandPoll)
	go func() {
		defer close(done)
		for sleepCtx(ctx, time.Duration(5+intn(r, 35))*time.Millisecond) {
			t := s.Titles[intn(r, int64(len(s.Titles)))]
			svc := s.Stack.Service
			switch intn(r, 12) {
			case 0:
				s.Stack.Readers.GetReaders(t.Hash)
				s.Stack.Readers.Count()
			case 1:
				svc.ListTorrents()
			case 2:
				svc.CollectStats()
			case 3:
				svc.Detail(t.Hash)
			case 4:
				svc.FilesProgress(t.Hash)
				svc.GetStatus(t.Hash)
			case 5:
				svc.IsActive(t.Hash)
				svc.Pauses()
				svc.PausedAt(t.Hash)
			case 6:
				if am := s.Stack.Activity; am != nil {
					am.GetStats()
					am.GetState(t.Hash)
					am.LastAccess(t.Hash)
				}
			case 7:
				s.Stack.Sessions.Count()
				s.Stack.Sessions.Sessions(ctx)
			case 8:
				s.Stack.FS.Stat(t.Path)
				s.Stack.FS.ReadDir(t.Dir)
			case 9:
				s.Stack.Registry.Gather()
			case 10:
				s.Snapshot(t)
			case 11:
				s.ServerPositions(t)
			}
		}
	}()
	return func() { <-done }
}

// consistent checks that the torrent service and the client agree on
// each library torrent (torrent-consistent).
func (s *stressRun) consistent() {
	if p := s.agree(); p != "" {
		s.Check(checkTorrentConsistent, report.Fail, "", p)
		return
	}
	s.Check(checkTorrentConsistent, report.Pass, "", fmt.Sprintf("the service and the client agree on %d torrents", len(s.Stack.Torrents())))
}
