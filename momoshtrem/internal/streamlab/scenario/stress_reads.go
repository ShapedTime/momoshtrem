package scenario

import (
	"context"
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/shapedtime/momoshtrem/internal/streamlab/checks"
	"github.com/shapedtime/momoshtrem/internal/streamlab/client"
	"github.com/shapedtime/momoshtrem/internal/streamlab/stack"
	"github.com/shapedtime/momoshtrem/internal/vfs"
)

// The stress tier's read-path scenarios: reads blocked on a held seeder,
// and what happens to them, their files and their requests meanwhile.

var closeBlockedRead = &Scenario{
	Name: "close-blocked-read",
	Tier: Stress,
	Doc: `Reads block on pieces a held seeder doesn't send, and their files close under them, round after round: several workers at random offsets, the seeder released at a random moment of each round.

Each worker's move is one of two. Over WebDAV, a bounded GET whose read times out: the handler returns and x/net closes the file while the read's goroutine still waits in anacrolix. Directly on the VFS, an open, a ReadAt that may time out, then a Close a moment later. A release after the closes wakes the orphaned reads against readers already closed; one before lets the reads through. It guards the read path's leak and deadlock fixes: a private buffer per read and a drained pending read (0d364a2), a PriorityReader.Read that doesn't hold its mutex across the blocking read, so Close can't deadlock (46e3423), and #225, the goroutine a timed-out read leaves behind its closed file.`,
	Guards:  []string{"0d364a2", "46e3423", "#225"},
	Timeout: stressTimeout,
	Stack:   stressStack(nil, ""),
	Params: stressParams(
		Param{Name: "workers", Kind: Int, Default: "3", Doc: "Concurrent workers."},
		Param{Name: "rounds", Kind: Int, Default: "10", Doc: "Rounds: a move per worker each."},
		Param{Name: "release_within", Kind: Duration, Default: "12s", Doc: "Each round releases the seeder at a random moment this long after it starts: often after the reads timed out and their files closed, " +
			"and now and then past a Close's bound, which a Close waiting on its timed-out read (46e3423's deadlock) would then outlive."},
	),
	Checks: []string{checkOpsBounded},
	Key:    []string{"stress_ops", "read_timeouts", "leaked_goroutines"},
	Script: stressScript(func(s *stressRun) error {
		t := s.Titles[0]
		if err := s.connect(t); err != nil {
			return err
		}
		conns := make([]*client.Conn, s.P.Int("workers"))
		for w := range conns {
			conns[w] = s.Conn(s.Device(fmt.Sprintf("stress-%d", w)))
		}
		return s.rounds(t, s.P.Int("rounds"), s.P.Duration("release_within"), func(_, w int, r *rand.Rand) error {
			off := s.incomplete(t, r)
			over := intn(r, 2) == 0
			size := (1 + intn(r, 16)) << 16
			pause := time.Duration(intn(r, 300)) * time.Millisecond
			if over {
				s.get(s.Ctx, conns[w], t, off, size)
				return nil
			}
			s.readThenClose(t, off, size, pause)
			return nil
		})
	}),
}

var seekPendingRead = &Scenario{
	Name: "seek-pending-read",
	Tier: Stress,
	Doc: `Each worker keeps one VFS file open and, round after round, reads it twice at random offsets while the seeder is held: the first read times out and its goroutine stays pending, and the second, elsewhere (a seek), must drain it before it seeks the anacrolix reader.

Each round releases the seeder at a random moment, so the drain ends both ways: the pending read completes when the release brings its piece, or the drain itself times out and the ReadAt fails without seeking. A WebDAV playback of the same file runs beside them. It guards 5355720 (a ReadAt that seeked while a timed-out read held the anacrolix reader deadlocked every later read of the file) and 0d364a2 (a pending read's buffer and goroutine).`,
	Guards:  []string{"5355720", "0d364a2"},
	Timeout: stressTimeout,
	Stack:   stressStack(nil, ""),
	Params: stressParams(
		Param{Name: "workers", Kind: Int, Default: "2", Doc: "Concurrent workers, each with a file of its own."},
		Param{Name: "rounds", Kind: Int, Default: "8", Doc: "Rounds: two reads per worker each."},
		Param{Name: "release_within", Kind: Duration, Default: "6s", Doc: "Each round releases the seeder at a random moment this long after it starts: before the first read's timeout, during the second's drain, or after it."},
	),
	Checks: []string{checkOpsBounded},
	Key:    []string{"stress_ops", "read_timeouts", "seeks_actual"},
	Script: stressScript(func(s *stressRun) error {
		t := s.Titles[0]
		if err := s.connect(t); err != nil {
			return err
		}
		files := make([]vfs.File, s.P.Int("workers"))
		defer func() {
			for _, f := range files {
				if f != nil {
					s.call("Close", 2*s.Stack.ReadTimeout, func() { f.Close() })
				}
			}
		}()
		for w := range files {
			var err error
			if !s.call("Open", s.Stack.ReadTimeout, func() { files[w], err = s.Stack.FS.Open(t.Path) }) || err != nil {
				return fmt.Errorf("opening %s: %v", t.Path, err)
			}
		}
		ctx, stop := context.WithCancel(s.Ctx)
		played := make(chan struct{})
		go func() {
			defer close(played)
			conn := s.Conn(s.Device("stress-play"))
			r := strand(s.base, strandPlay)
			for ctx.Err() == nil {
				s.get(ctx, conn, t, s.incomplete(t, r), 4<<20)
			}
		}()
		err := s.rounds(t, s.P.Int("rounds"), s.P.Duration("release_within"), func(_, w int, r *rand.Rand) error {
			for range 2 {
				p := make([]byte, (1+intn(r, 8))<<15)
				s.readAt(files[w], t, p, s.incomplete(t, r))
			}
			return nil
		})
		stop()
		<-played
		return err
	}),
}

var abortBlockedRead = &Scenario{
	Name: "abort-blocked-read",
	Tier: Stress,
	Doc: `A client drops a GET while the server's read is blocked on a piece a held seeder doesn't send, over and over; a random moment later (or never, in the first iteration) the seeder is released.

The read runs under context.Background() plus read_timeout, not the request's context (F12, #217): the handler lives on until the read returns, when the piece arrives or the read times out. handler_lag_after_abort is that time after the drop. abort-lag fails past abort_lag_max (1 s), with F12's signature when every lag ended within the read timeout plus a second. A longer one is a hang, not F12. The first iteration never releases, so F12 shows every time while it exists.`,
	Guards:  []string{"F12"},
	Timeout: stressTimeout,
	Stack:   stressStack(nil, ""),
	Params: stressParams(
		Param{Name: "read_timeout", Kind: Duration, Default: "3s"},
		Param{Name: "iterations", Kind: Int, Default: "8", Doc: "Dropped requests."},
		Param{Name: "abort_lag_max", Kind: Duration, Default: "1s", Doc: "The longest a handler may run after its client dropped it."},
	),
	Checks: []string{"abort-lag"},
	Key:    []string{"handler_lag_after_abort", "stress_ops", "read_timeouts"},
	Script: stressScript(func(s *stressRun) error {
		t := s.Titles[0]
		if err := s.connect(t); err != nil {
			return err
		}
		dev := s.Device("stress-abort")
		timeout := s.Stack.ReadTimeout
		for i := range s.P.Int("iterations") {
			// A declared fault (faultlog.go): the watchdog doesn't trip on
			// it.
			release := s.HoldSeeder(t, "hold-uploads")
			conn := s.Conn(dev)
			off := s.incomplete(t, s.rng)
			dropAfter := time.Duration(50+intn(s.rng, 350)) * time.Millisecond
			releaseAfter := time.Duration(intn(s.rng, int64(timeout+time.Second)))
			rng := rangeOf(off, 0)
			ctx, cancel := context.WithCancel(s.Ctx)
			got := make(chan struct{})
			go func() {
				defer close(got)
				s.get(ctx, conn, t, off, 0)
			}()
			// Drop it once the server's read is blocked.
			blocked := s.waitBlocked(dev.UserAgent, rng, 2*time.Second)
			if err := s.Sleep(dropAfter); err != nil {
				cancel()
				return err
			}
			s.Abandon(conn, t.Path, rng)
			cancel()
			<-got
			if !blocked {
				s.event("dropped before its read blocked")
			}
			if i > 0 {
				if err := s.Sleep(releaseAfter); err != nil {
					return err
				}
				release()
				s.event("seeder released")
			}
			if !s.RequestsDone(func(r stack.Request) bool { return r.UserAgent == dev.UserAgent }, timeout+5*time.Second) {
				s.Note("iteration %d: the dropped handler hadn't returned %v after the drop", i, timeout+5*time.Second)
			}
			release()
		}
		return nil
	}),
}

var manyReaders = &Scenario{
	Name: "many-readers",
	Tier: Stress,
	Doc: `Many connections read one movie at once, each its own device: bounded ranges of random sizes at random offsets, round after round, while the pollers read the readers, sessions and torrent state.

Every connection gets a PriorityReader of its own (efd4c62), so the tracker, the prioritizers, the sessions and the metrics all see many concurrent readers of one file. Every range must arrive whole and right: play-ok, with #195's known stall as its only excuse.`,
	Guards:  []string{"efd4c62"},
	Timeout: stressTimeout,
	Stack:   stressStack(nil, ""),
	Params: stressParams(
		Param{Name: "read_timeout", Kind: Duration, Default: "10s"},
		Param{Name: "file_size", Kind: Bytes, Default: "96MiB"},
		Param{Name: "readers", Kind: Int, Default: "16", Doc: "Concurrent connections, each its own device."},
		Param{Name: "rounds", Kind: Int, Default: "6", Doc: "Ranges per connection."},
		Param{Name: "range_max", Kind: Bytes, Default: "2MiB", Doc: "The largest range; each is 64 KiB to this."},
	),
	Key: []string{"stress_ops", "probe_latency", "readers_created"},
	Script: stressScript(func(s *stressRun) error {
		t := s.Titles[0]
		maxRange := max(s.P.Bytes("range_max")>>16, 1)
		return s.workers(func(w int, r *rand.Rand) error {
			conn := s.Conn(s.Device(fmt.Sprintf("reader-%d", w)))
			for range s.P.Int("rounds") {
				n := (1 + intn(r, maxRange)) << 16
				off := min(intn(r, t.Size)&^(64<<10-1), t.Size-n)
				if err := s.Probe(conn, t, max(off, 0), min(n, t.Size)); err != nil {
					return err
				}
				s.count("range read")
			}
			return nil
		}, s.P.Int("readers"))
	}),
}

// connect reads t's first bytes over WebDAV, so its torrent loads and the
// leecher connects to the seeder before the script holds it (a held
// complete seeder refuses new connections: swarm.Seeder.HoldUploads).
func (s *stressRun) connect(t *Title) error {
	dev := s.Device("stress-connect")
	if err := s.Probe(s.Conn(dev), t, 0, 64<<10); err != nil {
		return err
	}
	if _, ok := s.Torrent(t); !ok {
		return fmt.Errorf("%s didn't load", t.Name)
	}
	return nil
}

// workers runs fn(w, r) for w in 0..n-1 at once, n the workers param
// unless given, each with its own strand r, and waits for them.
func (s *stressRun) workers(fn func(w int, r *rand.Rand) error, n ...int64) error {
	return s.inRound(0, fn, n...)
}

// inRound runs the workers of round (1 and up; 0 outside rounds): worker
// w's strand is (round, w), so its choices are the same whatever the
// others' timing.
func (s *stressRun) inRound(round int, fn func(w int, r *rand.Rand) error, n ...int64) error {
	var count int64
	if len(n) > 0 {
		count = n[0]
	} else {
		count = s.P.Int("workers")
	}
	fns := make([]func() error, count)
	for w := range fns {
		r := strand(s.base, strandWorker, uint64(round), uint64(w))
		fns[w] = func() error { return fn(w, r) }
	}
	return s.Go(fns...)
}

// readThenClose opens t's file directly, reads size bytes at off (a read
// that may time out, its goroutine left pending), waits pause, then closes
// the file: the pending read, if any, outlives its file and wakes later.
func (s *stressRun) readThenClose(t *Title, off, size int64, pause time.Duration) {
	var f vfs.File
	var err error
	if !s.call("Open", s.Stack.ReadTimeout, func() { f, err = s.Stack.FS.Open(t.Path) }) {
		return
	}
	if err != nil {
		s.countShort("open failed: " + errClass(err))
		return
	}
	s.readAt(f, t, make([]byte, size), off)
	s.Sleep(pause)
	// The read returned: nothing holds the file, so a Close has no reason
	// to wait for the seeder. One that does (46e3423's deadlock) outlives
	// this bound when the release comes late.
	s.call("Close", s.Stack.ReadTimeout, func() { f.Close() })
}

// waitBlocked waits up to limit until the stack has a read blocked and
// the request of ua for rng is running; false when it never was.
func (s *stressRun) waitBlocked(ua, rng string, limit time.Duration) bool {
	deadline := time.Now().Add(limit)
	for time.Now().Before(deadline) {
		active := false
		for _, r := range s.Stack.Requests.Active() {
			active = active || (r.UserAgent == ua && r.Range == rng)
		}
		if active && checks.Value(s.Stack.Registry, "momoshtrem_streaming_blocked_reads") > 0 {
			return true
		}
		if s.Sleep(10*time.Millisecond) != nil {
			return false
		}
	}
	return false
}
