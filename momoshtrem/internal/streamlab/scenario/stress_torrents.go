package scenario

import (
	"context"
	"fmt"
	"math"
	"math/rand/v2"
	"strings"
	"time"

	"github.com/shapedtime/momoshtrem/internal/config"
	"github.com/shapedtime/momoshtrem/internal/streamlab/known"
	"github.com/shapedtime/momoshtrem/internal/streamlab/report"
	"github.com/shapedtime/momoshtrem/internal/torrent"
	"github.com/shapedtime/momoshtrem/internal/vfs"
)

// The stress tier's torrent-lifecycle scenarios: removes, pauses, idles
// and adds racing the reads.

var removePauseIdle = &Scenario{
	Name: "remove-pause-idle",
	Tier: Stress,
	Doc: `Readers stream one movie while its torrent is removed, paused and idled under them, at random. WebDAV GETs at random offsets and a VFS file read directly run against a chaos loop. Every moment or two it removes the torrent (svc.RemoveTorrent), pauses it and resumes it a moment later (the activity manager's sticky pause), or idles it under the reads.

To idle it, the loop holds the seeder, so every read blocks. A blocked read isn't activity, so once no read has started for the idle timeout (1 s, checked every 100 ms), the activity manager idles the torrent under the blocked reads. Their next reads wake it. The loop releases the seeder once it has seen the torrent idle with a read blocked, or after the read timeout plus the idle timeout. The read timeout is 6 s here, so reads start seldom enough for an idle to come.

A removed torrent's open reads fail (its torrent closed), and the next open adds it again, its metadata from the seeder. Reads may fail here. What mustn't happen is a race, a deadlock, a leak, a wrong byte, or the service and the client disagreeing about what's loaded (torrent-consistent). It guards the serialised torrent lifecycle (bf328c5) and the idle wake (ffda214).

The removes keep the data: one that deletes it while a piece hashes deadlocks (#230), which remove-while-hashing guards. --set delete_data=true deletes it on half of them.`,
	Guards:  []string{"bf328c5", "ffda214"},
	Timeout: stressTimeout,
	Params: stressParams(
		Param{Name: "read_timeout", Kind: Duration, Default: "6s"},
		Param{Name: "file_size", Kind: Bytes, Default: "96MiB"},
		Param{Name: "duration", Kind: Duration, Default: "40s", Doc: "How long the chaos runs."},
		Param{Name: "readers", Kind: Int, Default: "3", Doc: "WebDAV readers; one more reads directly."},
		Param{Name: "chaos_every", Kind: Duration, Default: "1500ms", Doc: "A chaos move about this often (jittered ±50%)."},
		Param{Name: "idle_timeout", Kind: Duration, Default: "1s", Doc: "torrent.idle_timeout (whole seconds)."},
		Param{Name: "idle_check", Kind: Duration, Default: "100ms", Doc: "The activity manager's check interval (SetCheckInterval)."},
		Param{Name: "delete_data", Kind: Bool, Default: "false", Doc: "Delete the data on half of the removes (#230 then fails ops-bounded here)."},
	),
	Stack: stressStack(func(p Params, c *config.Config) {
		c.Torrent.IdleEnabled = true
		c.Torrent.IdleTimeout = max(int(math.Ceil(p.Duration("idle_timeout").Seconds())), 1)
	}, "idle_check"),
	Checks: []string{checkOpsBounded, checkTorrentConsistent},
	Key:    []string{"stress_ops", "stress_short_reads", "read_timeouts"},
	Script: stressScript(func(s *stressRun) error {
		t := s.Titles[0]
		if err := s.connect(t); err != nil {
			return err
		}
		ctx, stop := context.WithTimeout(s.Ctx, s.P.Duration("duration"))
		defer stop()
		read := func(w int, r *rand.Rand) error {
			if w == 0 {
				return s.directReads(ctx, t, r)
			}
			conn := s.Conn(s.Device(fmt.Sprintf("stress-%d", w)))
			for ctx.Err() == nil {
				off := intn(r, t.Size) &^ (64<<10 - 1)
				s.get(ctx, conn, t, off, (1+intn(r, 32))<<16)
			}
			return nil
		}
		chaos := func() error {
			r := strand(s.base, strandChaos)
			every := s.P.Duration("chaos_every")
			for sleepCtx(ctx, every/2+time.Duration(intn(r, int64(every)))) {
				s.chaos(ctx, t, r)
			}
			return nil
		}
		done := make(chan error, 1)
		go func() { done <- s.workers(read, s.P.Int("readers")+1) }()
		chaos()
		err := <-done
		// Leave it running, as a user would, and let the lifecycle settle.
		s.Stack.Service.Resume(t.Hash)
		s.consistent()
		if s.Ctx.Err() != nil {
			return s.Ctx.Err()
		}
		return err
	}),
}

// chaos makes one random move on t's torrent, its choices from r.
func (s *stressRun) chaos(ctx context.Context, t *Title, r *rand.Rand) {
	svc := s.Stack.Service
	switch intn(r, 4) {
	case 0, 1:
		s.remove(t, s.P.Bool("delete_data") && intn(r, 2) == 0)
	case 2:
		pause := time.Duration(200+intn(r, 1300)) * time.Millisecond
		svc.Pause(t.Hash)
		s.count("paused")
		sleepCtx(ctx, pause)
		svc.Resume(t.Hash)
	case 3:
		s.idleUnderReads(ctx, t)
	}
}

// idleUnderReads holds t's seeder so the readers' reads block, and waits
// for the activity manager to idle the torrent under them (a blocked read
// isn't activity): up to a read timeout, since each read starting marks
// the torrent active, plus the idle timeout and three checks. It counts an
// idle seen with a read blocked as "idled under a read", then releases
// the seeder: the readers' next reads wake the torrent. A seeder can only
// be held while the leecher is connected to it (a held one refuses new
// connections): after a remove, it waits up to a read timeout for the
// readers' next open to add the torrent back and connect, then gives up.
func (s *stressRun) idleUnderReads(ctx context.Context, t *Title) {
	am := s.Stack.Activity
	if am == nil {
		return
	}
	connected := func() bool {
		tor, ok := s.Torrent(t)
		return ok && tor.Info() != nil && tor.Stats().ActivePeers > 0
	}
	for wait := time.Now().Add(s.Stack.ReadTimeout); !connected(); {
		if time.Now().After(wait) || !sleepCtx(ctx, 50*time.Millisecond) {
			s.event("not idled: no connected seeder to hold")
			return
		}
	}
	t.Seeder.HoldUploads()
	defer t.Seeder.ReleaseUploads()
	// The idle lasts until the next read starts, a few milliseconds once
	// the blocked reads fail ("torrent data downloading disabled"): look
	// every millisecond.
	deadline := time.Now().Add(s.Stack.ReadTimeout + s.P.Duration("idle_timeout") + 3*s.P.Duration("idle_check"))
	for time.Now().Before(deadline) {
		if am.GetState(t.Hash) == torrent.StateIdle {
			if s.Counter("momoshtrem_streaming_blocked_reads") > 0 {
				s.count("idled under a read")
			} else {
				s.count("idled between reads")
			}
			return
		}
		if !sleepCtx(ctx, time.Millisecond) {
			return
		}
	}
	s.event("not idled: reads kept it active")
}

// remove removes t's torrent as a direct call, its data deleted with del;
// false when the call got stuck. It may wait for an add in flight: up to
// the add timeout.
func (s *stressRun) remove(t *Title, del bool) bool {
	return s.call("RemoveTorrent", s.Stack.ReadTimeout+time.Duration(s.Stack.Config.Torrent.AddTimeout)*time.Second, func() {
		if err := s.Stack.Service.RemoveTorrent(t.Hash, del); err != nil {
			s.count("remove: " + errClass(err))
			return
		}
		s.count(fmt.Sprintf("removed (delete data %v)", del))
	})
}

// directReads reads t through VFS files opened directly until ctx ends:
// a file kept for a few reads, then closed. Its choices come from r.
func (s *stressRun) directReads(ctx context.Context, t *Title, r *rand.Rand) error {
	for ctx.Err() == nil {
		var f vfs.File
		var err error
		if !s.call("Open", s.Stack.ReadTimeout+time.Duration(s.Stack.Config.Torrent.AddTimeout)*time.Second,
			func() { f, err = s.Stack.FS.Open(t.Path) }) {
			return nil
		}
		if err != nil {
			s.countShort("open failed: " + errClass(err))
			sleepCtx(ctx, 100*time.Millisecond)
			continue
		}
		for range 1 + intn(r, 4) {
			p := make([]byte, (1+intn(r, 8))<<15)
			s.readAt(f, t, p, intn(r, t.Size)&^(64<<10-1))
		}
		s.call("Close", 2*s.Stack.ReadTimeout, func() { f.Close() })
	}
	return nil
}

var removeWhileHashing = &Scenario{
	Name: "remove-while-hashing",
	Tier: Stress,
	Doc: `A player downloads a movie, and the torrent is removed with its data a random moment into it, again and again: each GET adds the torrent back, its pieces download and hash, and RemoveTorrent(hash, true) deletes them.

deletePieces marks each piece not complete under the piece's lock, and anacrolix's piece hasher takes that lock for reading twice: a remove mid-hash deadlocks for good, holding the torrent's hash lock, and every open of the torrent waits behind it (#230). The script stops at the first stuck remove. ops-bounded and quiescence carry #230's signature then: the cycle on one mutex in the goroutine dump, and nothing left but the remove and what waits behind its hash lock.`,
	Guards:  []string{known.DeleteDataDeadlock, "bf328c5"},
	Timeout: stressTimeout,
	Stack:   stressStack(nil, ""),
	Params: stressParams(
		Param{Name: "file_size", Kind: Bytes, Default: "96MiB"},
		Param{Name: "attempts", Kind: Int, Default: "40", Doc: "Removes with data, each a random moment into a download."},
		Param{Name: "remove_within", Kind: Duration, Default: "400ms", Doc: "Each remove comes this long after its GET at most."},
	),
	Checks: []string{checkOpsBounded, checkTorrentConsistent},
	Key:    []string{"stress_ops", "stress_short_reads"},
	Script: stressScript(func(s *stressRun) error {
		t := s.Titles[0]
		conn := s.Conn(s.Device("stress-hash"))
		for range s.P.Int("attempts") {
			if err := s.Ctx.Err(); err != nil {
				return err
			}
			off := intn(s.rng, t.Size) &^ (64<<10 - 1)
			after := time.Duration(intn(s.rng, int64(s.P.Duration("remove_within"))+1))
			ctx, cancel := context.WithCancel(s.Ctx)
			got := make(chan struct{})
			go func() {
				defer close(got)
				s.get(ctx, conn, t, off, 4<<20)
			}()
			s.Sleep(after)
			ok := s.remove(t, true)
			cancel()
			<-got
			if !ok {
				s.event("a remove stuck")
				break
			}
		}
		s.consistent()
		return nil
	}),
}

var concurrentAdds = &Scenario{
	Name: "concurrent-adds",
	Tier: Stress,
	Doc: `Over and over, a season pack's torrent is removed, then added by many callers at once: torrent service adds, WebDAV GETs of its episodes (each open runs GetOrAddTorrent), a Depth 1 PROPFIND of the season (which opens every episode it lists: F3), and now and then a remove racing them all.

Adds of one hash share one attempt and its result, and adds, waits, drops and removes of one hash are serialised (bf328c5). Every GET must get right bytes or fail cleanly, and once each round settles the service and the client must agree on what's loaded (torrent-consistent). The stored metainfo goes with each remove, so every add fetches the info dict from the seeder. The removes keep the data (deleting it mid-hash is #230's, which remove-while-hashing guards).`,
	Guards:  []string{"bf328c5"},
	Timeout: stressTimeout,
	Stack:   stressStack(nil, ""),
	Params: stressParams(
		Param{Name: "file_size", Kind: Bytes, Default: "16MiB", Doc: "Each episode's size."},
		Param{Name: "iterations", Kind: Int, Default: "12", Doc: "Rounds of remove, then concurrent adds."},
		Param{Name: "callers", Kind: Int, Default: "6", Doc: "Concurrent adders per round."},
	),
	Media:  func(p Params) Media { return SeasonMedia(p, 3, p.Bytes("file_size")) },
	Checks: []string{checkTorrentConsistent},
	Key:    []string{"stress_ops", "stress_short_reads"},
	Script: stressScript(func(s *stressRun) error {
		svc := s.Stack.Service
		hash, magnet := s.Titles[0].Hash, s.Titles[0].Seeder.Magnet()
		var problems []string
		for i := range s.P.Int("iterations") {
			if err := s.Ctx.Err(); err != nil {
				return err
			}
			if err := svc.RemoveTorrent(hash, false); err == nil {
				s.count("removed")
			}
			callers := s.P.Int("callers")
			fns := make([]func() error, 0, callers+1)
			for c := range callers {
				switch c % 3 {
				case 0:
					fns = append(fns, func() error {
						if _, err := svc.AddTorrent(magnet); err != nil {
							s.countShort("add: " + errClass(err))
						} else {
							s.count("added")
						}
						return nil
					})
				case 1:
					t := s.Titles[intn(s.rng, int64(len(s.Titles)))]
					off := intn(s.rng, t.Size) &^ (64<<10 - 1)
					conn := s.Conn(s.Device(fmt.Sprintf("adder-%d", c)))
					fns = append(fns, func() error {
						s.get(s.Ctx, conn, t, off, 256<<10)
						return nil
					})
				case 2:
					conn := s.Conn(s.Device(fmt.Sprintf("lister-%d", c)))
					fns = append(fns, func() error {
						if _, err := s.Propfind(conn, s.Titles[0].Dir, 1); err != nil {
							s.countShort("propfind: " + errClass(err))
						} else {
							s.count("listed")
						}
						return nil
					})
				}
			}
			racing := intn(s.rng, 3) == 0
			after := time.Duration(intn(s.rng, 300)) * time.Millisecond
			if racing {
				fns = append(fns, func() error {
					sleepCtx(s.Ctx, after)
					if svc.RemoveTorrent(hash, false) == nil {
						s.count("removed while adding")
					}
					return nil
				})
			}
			if err := s.Go(fns...); err != nil {
				return err
			}
			if p := s.agree(); p != "" {
				problems = append(problems, fmt.Sprintf("round %d: %s", i+1, p))
			}
		}
		// A last read, once nothing races it: the torrent must still serve.
		t := s.Titles[0]
		if err := s.Probe(s.Conn(s.Device("final")), t, 0, 256<<10); err != nil {
			return err
		}
		if len(problems) > 0 {
			s.Check(checkTorrentConsistent, report.Fail, "", strings.Join(first3(problems), "; "))
			return nil
		}
		s.consistent()
		return nil
	}),
}

// agree is "" when the service and the client agree on every library
// torrent, else what they disagree on.
func (s *stressRun) agree() string {
	var bad []string
	for _, h := range s.Stack.Torrents() {
		_, err := s.Stack.Service.GetTorrent(h)
		inService := err == nil
		_, inClient := s.Stack.Torrent(h)
		if inService != inClient {
			bad = append(bad, fmt.Sprintf("%s loaded in the service %v, in the client %v", h[:8], inService, inClient))
		}
	}
	return strings.Join(bad, ", ")
}
