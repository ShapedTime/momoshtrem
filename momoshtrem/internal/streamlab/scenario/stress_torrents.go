package scenario

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/shapedtime/momoshtrem/internal/config"
	"github.com/shapedtime/momoshtrem/internal/streamlab/known"
	"github.com/shapedtime/momoshtrem/internal/streamlab/report"
	"github.com/shapedtime/momoshtrem/internal/vfs"
)

// The stress tier's torrent-lifecycle scenarios: removes, pauses, idles
// and adds racing the reads.

var removePauseIdle = &Scenario{
	Name: "remove-pause-idle",
	Tier: Stress,
	Doc: `Readers stream one movie while its torrent is removed, paused and idled under them, at random. WebDAV GETs at random offsets and a VFS file read directly run against a chaos loop. Every moment or two it removes the torrent (svc.RemoveTorrent), pauses it and resumes it a moment later (the activity manager's sticky pause), or leaves it to the activity manager. The idle timeout (1 s, checked every 100 ms) idles it under any read blocked that long.

A removed torrent's open reads fail (its torrent closed), and the next open adds it again, its metadata from the seeder. Reads may fail here. What mustn't happen is a race, a deadlock, a leak, a wrong byte, or the service and the client disagreeing about what's loaded (torrent-consistent). It guards the serialised torrent lifecycle (bf328c5) and the idle wake (ffda214).

The removes keep the data: one that deletes it while a piece hashes deadlocks (#230), which remove-while-hashing guards. --set delete_data=true deletes it on half of them.`,
	Guards:  []string{"bf328c5", "ffda214"},
	Timeout: stressTimeout,
	Params: stressParams(
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
		read := func(w int) error {
			if w == 0 {
				return s.directReads(ctx, t)
			}
			conn := s.Conn(s.Device(fmt.Sprintf("stress-%d", w)))
			for ctx.Err() == nil {
				s.get(ctx, conn, t, s.Rand(t.Size)&^(64<<10-1), (1+s.Rand(32))<<16)
			}
			return nil
		}
		chaos := func() error {
			every := s.P.Duration("chaos_every")
			for sleepCtx(ctx, every/2+time.Duration(s.Rand(int64(every)))) {
				s.chaos(ctx, t)
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

// chaos makes one random move on t's torrent.
func (s *stressRun) chaos(ctx context.Context, t *Title) {
	svc := s.Stack.Service
	switch s.Rand(4) {
	case 0, 1:
		s.remove(t, s.P.Bool("delete_data") && s.Rand(2) == 0)
	case 2:
		svc.Pause(t.Hash)
		s.count("paused")
		sleepCtx(ctx, time.Duration(200+s.Rand(1300))*time.Millisecond)
		svc.Resume(t.Hash)
	case 3:
		// Leave it to the activity manager: a read blocked over the idle
		// timeout idles the torrent under it.
		s.event("left to idle")
	}
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
// a file kept for a few reads, then closed.
func (s *stressRun) directReads(ctx context.Context, t *Title) error {
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
		for range 1 + s.Rand(4) {
			s.readAt(f, t, make([]byte, (1+s.Rand(8))<<15), s.Rand(t.Size)&^(64<<10-1))
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
			ctx, cancel := context.WithCancel(s.Ctx)
			got := make(chan struct{})
			go func() {
				defer close(got)
				s.get(ctx, conn, t, s.Rand(t.Size)&^(64<<10-1), 4<<20)
			}()
			s.Sleep(time.Duration(s.Rand(int64(s.P.Duration("remove_within")) + 1)))
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
					t := s.Titles[s.Rand(int64(len(s.Titles)))]
					conn := s.Conn(s.Device(fmt.Sprintf("adder-%d", c)))
					fns = append(fns, func() error {
						s.get(s.Ctx, conn, t, s.Rand(t.Size)&^(64<<10-1), 256<<10)
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
			if s.Rand(3) == 0 {
				fns = append(fns, func() error {
					sleepCtx(s.Ctx, time.Duration(s.Rand(300))*time.Millisecond)
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
