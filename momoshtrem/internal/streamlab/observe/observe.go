// Package observe is streamlab's observer (docs/streamlab/PLAN.md §5
// "observe/"): it lets the lab explain why a stream behaved as it did,
// with no production change. While a repetition runs it
//   - samples the stack every 250 ms to 5 s: the metrics registry, the
//     readers (ReaderTracker), each torrent's and seeder's stats, the
//     runtime and, rarely, the service's Detail (it takes the anacrolix
//     client lock), each sampler timing its own work;
//   - mirrors every piece's state from anacrolix's piece state changes,
//     for a piece-arrival timeline against the readers' positions;
//   - turns the mutex and block profiles on;
//   - watches for a stall nothing explains: requests active, a read
//     blocked, no fault declared and no progress for the watchdog's time.
//     A trip dumps the labelled goroutines, a flight recorder snapshot,
//     both clients' status and the pieces around each reader.
//
// The slog capture (Capture) runs for the whole process: the runner
// installs it before the stack starts. After the run, Finish writes the
// artifacts (samples, timeline and its plots, logs, profiles) and draws
// findings: hints from the data, each pointing at its evidence. Findings
// never change a verdict.
//
// Measure mode keeps timings faithful: the samplers cost well under 2% of
// the process's CPU (the report says how much), DEBUG logs stay off, the
// profiles sample lightly, and the flight recorder starts only once the
// watchdog's condition has held for half its time. --diagnose (Config.
// Diagnose) captures the streaming components' DEBUG records, runs the
// flight recorder all along and the profiles at full rate.
package observe

import (
	"runtime/metrics"
	"runtime/trace"
	"sync"
	"time"

	atorrent "github.com/anacrolix/torrent"

	"github.com/shapedtime/momoshtrem/internal/streamlab/report"
	"github.com/shapedtime/momoshtrem/internal/streamlab/stack"
	"github.com/shapedtime/momoshtrem/internal/streamlab/swarm"
	"github.com/shapedtime/momoshtrem/internal/torrent"
)

// tick is the samplers' base interval; each runs every few ticks.
const tick = 250 * time.Millisecond

// Config configures an Observer.
type Config struct {
	Stack  *stack.Stack
	Titles []Title
	// Dir is the repetition's folder: artifacts go there. Empty keeps
	// none.
	Dir string
	// Logs is the process's log capture; nil without one (no log proof,
	// no log counts).
	Logs *Capture
	// Diagnose: profiles at full rate and the flight recorder all along.
	Diagnose bool
	// Watchdog is how long without progress trips the watchdog; 0
	// disables it.
	Watchdog time.Duration
	// KnownStall is #195's threshold: a trip carries #195's signature only
	// when nothing progressed this long (half the read timeout, as a
	// broken body's signature in play-ok), whatever the Watchdog.
	KnownStall time.Duration
	// Bitrate is the players' bitrate, bytes per second: a stall while
	// the swarm sent less is expected.
	Bitrate int64
	// AbortLagMax is the longest a handler may run after its client
	// dropped it before it's F12's lag.
	AbortLagMax time.Duration
}

// Title is a file the scenario plays.
type Title struct {
	Name string
	// Path is its WebDAV path.
	Path string
	Hash string
	// Offset and Size place it in its torrent; PieceLength is the
	// torrent's.
	Offset, Size, PieceLength int64
	// Seeder serves its torrent.
	Seeder *swarm.Seeder
	// Remote is an HDRezka movie, a file on the fake CDN: no torrent, so
	// nothing to sample but its requests. Its RemoteFile's reads don't
	// count in blocked_reads, so the watchdog can't see them stall (the
	// remote-watchdog check covers its own).
	Remote bool
}

// FaultKey is what a fault on the title declares: its torrent's hash, or
// its path for an HDRezka movie.
func (t Title) FaultKey() string {
	if t.Remote {
		return "remote:" + t.Path
	}
	return t.Hash
}

// piece is the torrent piece holding the file offset off.
func (t Title) piece(off int64) int {
	off = min(max(off, 0), max(t.Size-1, 0))
	return int((t.Offset + off) / t.PieceLength)
}

// Observer watches one repetition. Start it once the stack is up, Stop it
// when the repetition's measurements are done, then Finish it. Its
// methods are safe on a nil Observer (observation off).
type Observer struct {
	cfg    Config
	start  time.Time
	cpu0   time.Duration
	hashes []string

	stopc    chan struct{}
	done     chan struct{}
	stopOnce sync.Once
	watchWG  sync.WaitGroup

	samplers []*sampler
	watches  map[string]*pieceWatch
	prof     *profiles

	// rec is the flight recorder: on all along with Diagnose, else
	// started by the watchdog (recLazy) and stopped when the stall ends.
	recMu   sync.Mutex
	rec     *trace.FlightRecorder
	recLazy bool

	faultMu sync.Mutex
	faults  []fault // faults.go

	// The series, written by the sampling goroutine, read after Stop.
	metrics    []metricsSample
	readers    []readerSample
	torrents   []torrentSample
	seeds      []seederSample
	rts        []runtimeSample
	details    []detailSample
	counts     []countSample // ReaderTracker.Count
	downloaded map[string]int64
	titleOf    map[string]int // hash + "/" + anacrolix path → title
	wd         watchdogState
	trips      []tripRecord
	dumpTime   time.Duration // spent dumping at trips

	// Set by Stop.
	stopped time.Time
	cpu     time.Duration
	cpuOK   bool
	cont    contention
}

// sampler is one source sampled every few ticks, timing its own work.
type sampler struct {
	name  string
	every int  // ticks
	lock  bool // it takes the anacrolix client lock
	fn    func(now time.Time)
	n     int
	busy  time.Duration
	max   time.Duration
}

func (s *sampler) run(now time.Time) {
	start := time.Now()
	s.fn(now)
	d := time.Since(start)
	s.n++
	s.busy += d
	s.max = max(s.max, d)
}

// Start starts observing.
func Start(cfg Config) *Observer {
	o := &Observer{
		cfg: cfg, start: time.Now(), stopc: make(chan struct{}), done: make(chan struct{}),
		watches: map[string]*pieceWatch{}, downloaded: map[string]int64{}, titleOf: map[string]int{},
	}
	o.cpu0, _ = processCPU()
	o.prof = startProfiles(cfg.Diagnose)
	if cfg.Diagnose {
		o.startRecorder(false)
	}
	for _, t := range cfg.Titles {
		if _, ok := o.watches[t.Hash]; ok || t.Remote {
			continue
		}
		o.hashes = append(o.hashes, t.Hash)
		w := &pieceWatch{hash: t.Hash}
		o.watches[t.Hash] = w
		o.watchWG.Add(1)
		go func() {
			defer o.watchWG.Done()
			w.follow(cfg.Stack, o.stopc)
		}()
	}
	o.samplers = []*sampler{
		{name: "readers", every: 1, lock: true, fn: o.sampleReaders},
		{name: "metrics", every: 2, fn: o.sampleMetrics},
		{name: "torrent", every: 2, lock: true, fn: o.sampleTorrents},
		{name: "runtime", every: 4, fn: o.sampleRuntime},
		{name: "detail", every: 20, lock: true, fn: o.sampleDetail},
	}
	if cfg.Watchdog > 0 {
		// After metrics and torrent, on their fresh samples.
		o.samplers = append(o.samplers, &sampler{name: "watchdog", every: 2, fn: o.watch})
	}
	go o.loop()
	return o
}

func (o *Observer) loop() {
	defer close(o.done)
	t := time.NewTicker(tick)
	defer t.Stop()
	for k := 0; ; k++ {
		now := time.Now()
		for _, s := range o.samplers {
			if k%s.every == 0 {
				s.run(now)
			}
		}
		select {
		case <-o.stopc:
			return
		case <-t.C:
		}
	}
}

// Stop stops sampling and the profiles; the measurements are done. It's
// safe to call more than once.
func (o *Observer) Stop() {
	if o == nil {
		return
	}
	o.stopOnce.Do(func() {
		close(o.stopc)
		<-o.done
		o.watchWG.Wait()
		o.stopped = time.Now()
		o.endStretch(o.stopped)
		if cpu, ok := processCPU(); ok {
			o.cpu, o.cpuOK = cpu-o.cpu0, true
		}
		o.cont = collectProfiles(o.cfg.Dir)
		o.prof.stop()
		o.stopRecorder(true)
	})
}

// ms is t's time since the observer started, in milliseconds.
func (o *Observer) ms(t time.Time) float64 { return report.MS(t.Sub(o.start)) }

// The samples.

type metricsSample struct {
	t                                    time.Time
	reads, readBytes, timeouts, slow     float64
	blocked, openFiles, inflight         float64
	initial, seek, debounced, downgraded float64
}

// readerSample is a server reader at one moment: its position, its piece
// and that piece's state as anacrolix has it (the effective priority
// included), and the complete data ahead of it.
type readerSample struct {
	t         time.Time
	title     int
	pos, lead int64
	piece     int
	state     atorrent.PieceState
}

// countSample is how many PriorityReaders were open (the run's peak is
// report.Resources.PeakReaders).
type countSample struct {
	t time.Time
	n int
}

type torrentSample struct {
	t                  time.Time
	hash               string
	downloaded, useful int64
	seeders, peers     int
	state              string
}

type seederSample struct {
	t    time.Time
	hash string
	sent int64
	held bool // swarm.Seeder.Held
}

type runtimeSample struct {
	t          time.Time
	goroutines uint64
	heap       uint64
	gcCycles   uint64
}

type detailSample struct {
	t                              time.Time
	hash                           string
	have, partial, wanted, missing int
	peers, seeders                 int
}

// sampleReaders records every server reader's position, its piece's state
// (PieceState: the client's read lock, once per reader) and the complete
// data ahead of it (from the piece mirror, no lock).
func (o *Observer) sampleReaders(now time.Time) {
	o.counts = append(o.counts, countSample{t: now, n: o.cfg.Stack.Readers.Count()})
	for _, h := range o.hashes {
		snaps := o.cfg.Stack.Readers.GetReaders(h)
		if len(snaps) == 0 {
			continue
		}
		tor, ok := o.cfg.Stack.Torrent(h)
		if !ok || tor.Info() == nil {
			continue
		}
		for _, r := range snaps {
			ti, ok := o.titleIndex(h, r.FilePath)
			if !ok {
				continue
			}
			t := o.cfg.Titles[ti]
			s := readerSample{t: now, title: ti, pos: r.Position, piece: t.piece(r.Position)}
			s.state = tor.PieceState(s.piece)
			s.lead, _ = o.watches[h].lead(t.Offset, t.Size, r.Position)
			o.readers = append(o.readers, s)
		}
	}
}

// titleIndex is the title of the torrent file path names (anacrolix's
// File.Path), once the torrent has loaded.
func (o *Observer) titleIndex(hash, path string) (int, bool) {
	key := hash + "/" + path
	if i, ok := o.titleOf[key]; ok {
		return i, true
	}
	for i, t := range o.cfg.Titles {
		if t.Hash != hash {
			continue
		}
		if p, ok := o.watches[hash].fileAt(t.Offset); ok && p == path {
			o.titleOf[key] = i
			return i, true
		}
	}
	return 0, false
}

// The stack's metrics the samples keep.
const (
	mReads      = "momoshtrem_streaming_reads_total"
	mReadBytes  = "momoshtrem_streaming_read_bytes_total"
	mTimeouts   = "momoshtrem_streaming_read_timeouts_total"
	mSlow       = "momoshtrem_streaming_slow_reads_total"
	mBlocked    = "momoshtrem_streaming_blocked_reads"
	mOpenFiles  = "momoshtrem_streaming_open_files"
	mInflight   = "momoshtrem_webdav_requests_inflight"
	mUpdates    = "momoshtrem_streaming_priority_updates_total"
	mDowngraded = "momoshtrem_streaming_pieces_downgraded_total"
)

// sampleMetrics gathers the stack's registry once.
func (o *Observer) sampleMetrics(now time.Time) {
	mfs, err := o.cfg.Stack.Registry.Gather()
	if err != nil {
		return
	}
	s := metricsSample{t: now}
	for _, mf := range mfs {
		var sum float64
		for _, m := range mf.GetMetric() {
			v := m.GetGauge().GetValue() + m.GetCounter().GetValue()
			sum += v
			if mf.GetName() == mUpdates {
				for _, lp := range m.GetLabel() {
					if lp.GetName() != "type" {
						continue
					}
					switch lp.GetValue() {
					case "initial":
						s.initial += v
					case "seek":
						s.seek += v
					case "seek_debounced":
						s.debounced += v
					}
				}
			}
		}
		switch mf.GetName() {
		case mReads:
			s.reads = sum
		case mReadBytes:
			s.readBytes = sum
		case mTimeouts:
			s.timeouts = sum
		case mSlow:
			s.slow = sum
		case mBlocked:
			s.blocked = sum
		case mOpenFiles:
			s.openFiles = sum
		case mInflight:
			s.inflight = sum
		case mDowngraded:
			s.downgraded = sum
		}
	}
	o.metrics = append(o.metrics, s)
}

// sampleTorrents reads each loaded torrent's stats (the client's read
// lock), its activity state, and each seeder's.
func (o *Observer) sampleTorrents(now time.Time) {
	for _, h := range o.hashes {
		tor, ok := o.cfg.Stack.Torrent(h)
		if !ok {
			continue
		}
		st := tor.Stats()
		s := torrentSample{t: now, hash: h, downloaded: st.BytesReadData.Int64(), useful: st.BytesReadUsefulData.Int64(),
			seeders: st.ConnectedSeeders, peers: st.ActivePeers, state: o.activity(h)}
		o.downloaded[h] = s.downloaded
		o.torrents = append(o.torrents, s)
	}
	seen := map[*swarm.Seeder]bool{}
	for _, t := range o.cfg.Titles {
		if t.Seeder == nil || seen[t.Seeder] {
			continue
		}
		seen[t.Seeder] = true
		st := t.Seeder.Stats()
		o.seeds = append(o.seeds, seederSample{t: now, hash: t.Hash, sent: st.BytesWrittenData.Int64(), held: t.Seeder.Held()})
	}
}

// activity is the torrent's activity state: active when idle mode is off.
func (o *Observer) activity(hash string) string {
	if o.cfg.Stack.Activity == nil {
		return string(torrent.StateActive)
	}
	return string(o.cfg.Stack.Activity.GetState(hash))
}

var runtimeNames = []string{"/sched/goroutines:goroutines", "/memory/classes/heap/objects:bytes", "/gc/cycles/total:gc-cycles"}

func (o *Observer) sampleRuntime(now time.Time) {
	ms := make([]metrics.Sample, len(runtimeNames))
	for i, n := range runtimeNames {
		ms[i].Name = n
	}
	metrics.Read(ms)
	u := func(i int) uint64 {
		if ms[i].Value.Kind() == metrics.KindUint64 {
			return ms[i].Value.Uint64()
		}
		return 0
	}
	o.rts = append(o.rts, runtimeSample{t: now, goroutines: u(0), heap: u(1), gcCycles: u(2)})
}

// sampleDetail reads the service's Detail of each loaded torrent: its
// piece runs under the client lock, so it runs rarely.
func (o *Observer) sampleDetail(now time.Time) {
	for _, h := range o.hashes {
		d, err := o.cfg.Stack.Service.Detail(h)
		if err != nil || !d.HasInfo {
			continue
		}
		s := detailSample{t: now, hash: h, peers: d.Peers, seeders: d.Seeders}
		for _, r := range d.Pieces {
			switch r.State {
			case torrent.PieceHave:
				s.have += r.Count
			case torrent.PiecePartial:
				s.partial += r.Count
			case torrent.PiecePriority:
				s.wanted += r.Count
			default:
				s.missing += r.Count
			}
		}
		o.details = append(o.details, s)
	}
}

// startRecorder starts the flight recorder: all along (lazy false), or for
// a suspected stall. One recorder at a time; an error leaves none.
func (o *Observer) startRecorder(lazy bool) {
	o.recMu.Lock()
	defer o.recMu.Unlock()
	if o.rec != nil {
		return
	}
	age := 2 * o.cfg.Watchdog
	if !lazy {
		age = max(age, 10*time.Second)
	}
	fr := trace.NewFlightRecorder(trace.FlightRecorderConfig{MinAge: age, MaxBytes: 64 << 20})
	if fr.Start() != nil {
		return
	}
	o.rec, o.recLazy = fr, lazy
}

// stopRecorder stops a recorder the watchdog started, or any when all.
func (o *Observer) stopRecorder(all bool) {
	o.recMu.Lock()
	defer o.recMu.Unlock()
	if o.rec == nil || (!all && !o.recLazy) {
		return
	}
	o.rec.Stop()
	o.rec = nil
}
