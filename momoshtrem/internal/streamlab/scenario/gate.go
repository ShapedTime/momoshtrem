package scenario

import (
	"fmt"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"time"

	atorrent "github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/types"

	"github.com/shapedtime/momoshtrem/internal/config"
	"github.com/shapedtime/momoshtrem/internal/streamlab/checks"
	"github.com/shapedtime/momoshtrem/internal/streamlab/client"
	"github.com/shapedtime/momoshtrem/internal/streamlab/report"
	"github.com/shapedtime/momoshtrem/internal/streamlab/stack"
)

// The gate tier (PLAN §5 "Tiers"): Infuse's everyday moves, 15-45 s each,
// with the seeder above the bitrate. seeder-loss, corrupt-piece and the
// HDRezka scenarios come with S6 (#177).

// at is the file offset at frac of t, rounded down to a 64 KiB boundary
// as a player's seeks are.
func at(t *Title, frac float64) int64 {
	return int64(frac*float64(t.Size)) &^ (64<<10 - 1)
}

// window is the pieces a reader at pos prioritizes: Now for the urgent
// buffer, Readahead after it (streaming.Prioritizer.UpdateForSeek).
func (e *Env) window(t *Title, pos int64) (begin, end int) {
	sc := e.Stack.Streaming
	return t.PieceRange(pos, pos+sc.UrgentBufferBytes+sc.ReadaheadBytes)
}

// intersect is the overlap of two piece ranges.
func intersect(b1, e1, b2, e2 int) (int, int) {
	b, e := max(b1, b2), min(e1, e2)
	if b > e {
		return 0, 0
	}
	return b, e
}

var coldStart = &Scenario{
	Name: "cold-start",
	Tier: Gate,
	Doc: `A player opens a movie on a cold stack: it lists the movie's folder (a Depth 1 PROPFIND, which loads the torrent today: F3), stats the file, sends a HEAD, probes the header and the footer at once on two connections, then plays from the start.

It guards the cold-start races: piece priorities set before the activation wait (356e74d), and the activation wait itself under start_paused (ffda214). A regression there shows as broken probes or plays (play-ok) or a slower start (time_to_playable, by A/B).`,
	Guards: []string{"356e74d", "ffda214", "F3"},
	Params: []Param{{Name: "watch", Kind: Duration, Default: "10s", Doc: "How much media to play."}},
	Checks: []string{"propfind-no-load"},
	Key:    []string{"time_to_playable", "probe_latency", "activation_wait", "torrents_loaded_by_propfind"},
	Script: func(e *Env) error {
		t := e.Titles[0]
		dev := e.Device("infuse")
		a, b := e.Conn(dev), e.Conn(dev)
		if _, err := e.F3(a, t.Dir); err != nil {
			return err
		}
		if _, err := e.Propfind(a, t.Path, 0); err != nil {
			return fmt.Errorf("PROPFIND %s: %w", t.Path, err)
		}
		if _, err := e.Head(a, t); err != nil {
			return err
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
		return nil
	},
}

var steadyPlay = &Scenario{
	Name: "steady-play",
	Tier: Gate,
	Doc: `A player plays a movie from the start, without seeking.

With the seeder above the bitrate, playback shouldn't stall once started. It guards the stalls of 9de9c67 (incomplete pieces behind the cursor downgraded) and the read-path deadlock of 5355720; its counts (priority updates per MiB, waste) are the baseline the other scenarios read against.`,
	Guards: []string{"9de9c67", "5355720"},
	Params: []Param{{Name: "watch", Kind: Duration, Default: "20s", Doc: "How much media to play."}},
	Key:    []string{"time_to_playable", "stall_count", "seek_updates_per_mib", "waste_ratio"},
	Script: func(e *Env) error {
		t := e.Titles[0]
		dev := e.Device("infuse")
		p, err := e.Player(dev, t).Open(e.Ctx, e.Conn(dev), 0)
		if err := e.Live(err); err != nil {
			return err
		}
		if err := e.Live(p.Watch(e.P.Duration("watch"))); err != nil {
			return err
		}
		p.Stop()
		return nil
	},
}

var seekFwd = &Scenario{
	Name: "seek-fwd",
	Tier: Gate,
	Doc: `A player plays from the start, then seeks forward twice, to 40% and to 75%: each seek drops the playing request and opens a new one, as Infuse does.

Each seek lands where nothing is downloaded yet, so its latency is a cold piece fetch. It guards 9de9c67 and 5355720.`,
	Guards: []string{"9de9c67", "5355720"},
	Params: []Param{{Name: "watch", Kind: Duration, Default: "5s", Doc: "How much media to play at each position."}},
	Key:    []string{"seek_latency_p50", "stall_count", "waste_ratio", "handler_lag_after_abort"},
	Script: func(e *Env) error {
		t := e.Titles[0]
		dev := e.Device("infuse")
		conn := e.Conn(dev)
		s := e.Player(dev, t)
		watch := e.P.Duration("watch")
		p, err := s.Open(e.Ctx, conn, 0)
		if err := e.Live(err); err != nil {
			return err
		}
		for _, frac := range []float64{0, 0.40, 0.75} {
			if frac > 0 {
				p, err = s.Seek(e.Ctx, p, conn, at(t, frac))
				if err := e.Live(err); err != nil {
					return err
				}
			}
			if err := e.Live(p.Watch(watch)); err != nil {
				return err
			}
		}
		p.Stop()
		return nil
	},
}

var seekBack = &Scenario{
	Name: "seek-back",
	Tier: Gate,
	Doc: `A player plays from 30%, rewinds a few seconds (into data it already downloaded), then seeks back to 5% (data nobody downloaded).

A rewind into downloaded pieces should be quick; the far seek back is a cold fetch behind the old window. It guards 9de9c67 and 5355720.`,
	Guards: []string{"9de9c67", "5355720"},
	Params: []Param{
		{Name: "watch", Kind: Duration, Default: "6s", Doc: "How much media to play at each position."},
		{Name: "rewind", Kind: Duration, Default: "5s", Doc: "How far the first seek goes back, in media time."},
	},
	Key: []string{"seek_latency_p50", "seek_latency_p95", "stall_count", "waste_ratio"},
	Script: func(e *Env) error {
		t := e.Titles[0]
		dev := e.Device("infuse")
		conn := e.Conn(dev)
		s := e.Player(dev, t)
		watch := e.P.Duration("watch")
		p, err := s.Open(e.Ctx, conn, at(t, 0.30))
		if err := e.Live(err); err != nil {
			return err
		}
		if err := e.Live(p.Watch(watch)); err != nil {
			return err
		}
		back := int64(e.P.Duration("rewind").Seconds() * float64(e.P.Bytes("bitrate")))
		to := max(p.Playhead()-back, 0) &^ (64<<10 - 1)
		p, err = s.Seek(e.Ctx, p, conn, to)
		if err := e.Live(err); err != nil {
			return err
		}
		if err := e.Live(p.Watch(watch / 2)); err != nil {
			return err
		}
		p, err = s.Seek(e.Ctx, p, conn, at(t, 0.05))
		if err := e.Live(err); err != nil {
			return err
		}
		if err := e.Live(p.Watch(watch)); err != nil {
			return err
		}
		p.Stop()
		return nil
	},
}

var scrub = &Scenario{
	Name: "scrub",
	Tier: Gate,
	Doc: `A player plays a few seconds, then scrubs: 10 seeks in 3 s to random positions, each reading for a moment before the next drops it, then plays from where it settled.

A dropped request whose handler is blocked in a read lives on until the read returns (F12): its reader and Now window compete with the new position. abort-lag measures each handler's time after its client dropped it. It fails past abort_lag_max (1 s); F12's signature is a lag in (abort_lag_max, read_timeout + 1 s], a handler that did end once its read returned or timed out. A longer lag is a hang, not F12.`,
	Guards: []string{"F12"},
	Params: []Param{
		{Name: "seeks", Kind: Int, Default: "10", Doc: "How many seeks the scrub makes."},
		{Name: "scrub_time", Kind: Duration, Default: "3s", Doc: "How long the scrub takes."},
		{Name: "watch", Kind: Duration, Default: "5s", Doc: "How much media to play before and after the scrub."},
		{Name: "abort_lag_max", Kind: Duration, Default: "1s", Doc: "The longest a handler may run after its client dropped it."},
	},
	Checks: []string{"abort-lag"},
	Key:    []string{"handler_lag_after_abort", "seek_latency_p50", "readers_created", "read_timeouts"},
	Script: func(e *Env) error {
		t := e.Titles[0]
		dev := e.Device("infuse")
		conn := e.Conn(dev)
		s := e.Player(dev, t)
		watch := e.P.Duration("watch")
		p, err := s.Open(e.Ctx, conn, 0)
		if err := e.Live(err); err != nil {
			return err
		}
		if err := e.Live(p.Watch(watch / 2)); err != nil {
			return err
		}
		n := max(e.P.Int("seeks"), 1)
		step := e.P.Duration("scrub_time") / time.Duration(n)
		for range n {
			to := at(t, 0.1+0.8*float64(e.Rand(1000))/1000)
			p, err = s.Seek(e.Ctx, p, conn, to)
			if err := e.Live(err); err != nil {
				return err
			}
			if err := e.Live(p.Scrub(step)); err != nil {
				return err
			}
		}
		to := at(t, 0.1+0.8*float64(e.Rand(1000))/1000)
		p, err = s.Seek(e.Ctx, p, conn, to)
		if err := e.Live(err); err != nil {
			return err
		}
		if err := e.Live(p.Watch(watch)); err != nil {
			return err
		}
		p.Stop()
		return nil
	},
}

var probeBurst = &Scenario{
	Name: "probe-burst",
	Tier: Gate,
	Doc: `A player browses a season: a Depth 1 PROPFIND of the season folder, a HEAD of each episode, then a header and a footer probe of each episode at once on two connections, as Infuse does when it reads a season's metadata.

The PROPFIND loads the season's torrent (F3). Where the MIME table doesn't resolve .mkv, every PROPFIND and HEAD sniffs 512 bytes at offset 0 (F1): no-sniff-reads counts the readers they create. Every probe opens a reader whose InitialPrioritize asks for the episode's header and footer, and the next episode's: residual_download_bytes is what that keeps downloading.`,
	Guards: []string{"F1", "F3"},
	Params: []Param{
		{Name: "episodes", Kind: Int, Default: "6", Doc: "Episodes in the season pack."},
		{Name: "episode_size", Kind: Bytes, Default: "24MiB", Doc: "Each episode's size."},
	},
	Media:  func(p Params) Media { return SeasonMedia(p, int(p.Int("episodes")), p.Bytes("episode_size")) },
	Checks: []string{"propfind-no-load", "no-sniff-reads"},
	Key:    []string{"torrents_loaded_by_propfind", "sniff_readers", "readers_created", "residual_download_bytes"},
	Script: func(e *Env) error {
		dev := e.Device("infuse")
		a, b := e.Conn(dev), e.Conn(dev)
		before := e.ReadersCreated()
		if _, err := e.F3(a, e.Titles[0].Dir); err != nil {
			return err
		}
		for _, t := range e.Titles {
			if _, err := e.Head(a, t); err != nil {
				return err
			}
		}
		sniffs := e.ReadersCreated() - before
		e.Metric("sniff_readers", sniffs)
		ev := fmt.Sprintf("a PROPFIND and %d HEADs created %v readers (MIME: %s)", len(e.Titles), sniffs, mimeNote())
		if sniffs == 0 {
			e.Check("no-sniff-reads", report.Pass, "", ev)
		} else {
			e.Check("no-sniff-reads", report.Fail, "F1", ev)
		}
		n := e.P.Bytes("probe_bytes")
		for _, t := range e.Titles {
			if err := e.Go(
				func() error { return e.Probe(a, t, 0, n) },
				func() error { return e.Probe(b, t, t.Size-n, n) },
			); err != nil {
				return err
			}
		}
		return nil
	},
}

var parallelRanges = &Scenario{
	Name: "parallel-ranges",
	Tier: Gate,
	Doc: `Two playbacks of one movie at once on two connections, at the start and at 60%, while a third connection fetches bounded ranges at 20%, 40%, 80% and 95%.

Each connection gets its own reader since efd4c62 removed the shared reader cache, which made parallel ranges thrash one reader's position. Every range must come back whole and right, and neither playback should starve.`,
	Guards: []string{"efd4c62"},
	Params: []Param{
		{Name: "watch", Kind: Duration, Default: "10s", Doc: "How much media each playback plays."},
		{Name: "range_bytes", Kind: Bytes, Default: "1MiB", Doc: "Each bounded range's size."},
	},
	Key: []string{"time_to_playable", "stall_count", "probe_latency", "readers_created"},
	Script: func(e *Env) error {
		t := e.Titles[0]
		main, second, ranges := e.Device("infuse"), e.Device("infuse-2"), e.Device("infuse-ranges")
		watch := e.P.Duration("watch")
		play := func(dev client.Device, from int64) func() error {
			return func() error {
				p, err := e.Player(dev, t).Open(e.Ctx, e.Conn(dev), from)
				if err := e.Live(err); err != nil {
					return err
				}
				err = e.Live(p.Watch(watch))
				p.Stop()
				return err
			}
		}
		fetch := func() error {
			if err := e.Sleep(2 * time.Second); err != nil {
				return err
			}
			n := e.P.Bytes("range_bytes")
			var fns []func() error
			for _, frac := range []float64{0.20, 0.40, 0.80, 0.95} {
				conn := e.Conn(ranges)
				fns = append(fns, func() error { return e.Probe(conn, t, min(at(t, frac), t.Size-n), n) })
			}
			return e.Go(fns...)
		}
		return e.Go(play(main, 0), play(second, at(t, 0.60)), fetch)
	},
}

var dualReaderClose = &Scenario{
	Name: "dual-reader-close",
	Tier: Gate,
	Doc: `Reader A plays a movie from the start; reader B opens at 60%, reads a moment and pauses; once B has been open 31 s, B closes while A is paused (so A's window holds still).

PriorityReader.Close resets the whole file's priorities when its reader was open 30 s or more (F6): A's Readahead window drops to None. close-keeps-windows compares the effective priority of A's incomplete pieces just before and just after B's close; stall_time_after is A's stall time once it resumes.`,
	Guards:  []string{"F6"},
	Timeout: 4 * time.Minute,
	Params: []Param{
		{Name: "b_open", Kind: Duration, Default: "31s", Doc: "How long B stays open before it closes: over the 30 s minResetDuration."},
		{Name: "b_from", Kind: Float, Default: "0.6", Doc: "Where B opens, as a fraction of the file."},
		{Name: "file_size", Kind: Bytes, Default: "192MiB", Doc: "The movie's size: enough that neither window completes in time."},
		{Name: "watch_after", Kind: Duration, Default: "5s", Doc: "How much A plays after B closed."},
	},
	Checks: []string{"close-keeps-windows"},
	Key:    []string{"pieces_lowered_by_close", "stall_time_after", "stall_count"},
	Script: func(e *Env) error {
		t := e.Titles[0]
		devA, devB := e.Device("infuse-a"), e.Device("infuse-b")
		a, err := e.Player(devA, t).Open(e.Ctx, e.Conn(devA), 0)
		if err := e.Live(err); err != nil {
			return err
		}
		if err := e.Live(a.Watch(3 * time.Second)); err != nil {
			return err
		}
		bOpened := time.Now()
		b, err := e.Player(devB, t).Open(e.Ctx, e.Conn(devB), at(t, e.P.Float("b_from")))
		if err := e.Live(err); err != nil {
			return err
		}
		if err := e.Live(b.Watch(time.Second)); err != nil {
			return err
		}
		b.Pause()
		closeAt := bOpened.Add(e.P.Duration("b_open"))
		if err := e.Live(a.Watch(time.Until(closeAt.Add(-2 * time.Second)))); err != nil {
			return err
		}
		a.Pause()
		if !e.WaitServerSettled(t, 300*time.Millisecond, 5*time.Second) {
			e.Note("A's reader didn't settle before B closed")
		}
		if err := e.Sleep(time.Until(closeAt)); err != nil {
			return err
		}
		posA, ok := lowest(e.ServerPositions(t))
		before := e.Snapshot(t)
		b.Stop()
		closed := e.RequestsDone(func(r stack.Request) bool { return r.UserAgent == devB.UserAgent }, 10*time.Second)
		after := e.Snapshot(t)
		switch {
		case !ok:
			e.Check("close-keeps-windows", report.NA, "", "reader A had no open reader server side")
		case !closed:
			e.Check("close-keeps-windows", report.NA, "", "reader B's handler hadn't returned 10 s after its client closed")
		default:
			begin, end := e.window(t, posA)
			ch := Compare(before, after, begin, end)
			wanted := before.Above(begin, end, types.PiecePriorityNone)
			e.Metric("pieces_lowered_by_close", float64(len(ch.Lowered)))
			ev := fmt.Sprintf("A at %d, window pieces %d-%d (%d incomplete and wanted before B closed): %s", posA, begin, end-1, wanted, ch)
			// F6's signature: ResetPriorities sets the file's every piece
			// to None, so every lowered piece went to None.
			toNone := !slices.ContainsFunc(ch.Lowered, func(i int) bool {
				st, _ := after.state(i)
				return st.Priority != types.PiecePriorityNone
			})
			switch {
			case wanted == 0:
				e.Check("close-keeps-windows", report.NA, "", ev)
			case len(ch.Lowered) > 0 && toNone:
				e.Check("close-keeps-windows", report.Fail, "F6", ev)
			case len(ch.Lowered) > 0:
				e.Check("close-keeps-windows", report.Fail, "", ev+" (not all to none: not F6's signature)")
			default:
				e.Check("close-keeps-windows", report.Pass, "", ev)
			}
		}
		stalled := a.Model().StallTime(time.Now())
		err = e.Live(a.Watch(e.P.Duration("watch_after")))
		e.Metric("stall_time_after", report.MS(a.Model().StallTime(time.Now())-stalled))
		a.Stop()
		return err
	},
}

var dualReaderOpen = &Scenario{
	Name: "dual-reader-open",
	Tier: Gate,
	Doc: `Reader A plays a movie from 20 MiB before its end and pauses (its window holds still); reader B then opens a header probe.

Every new reader runs InitialPrioritize, which sets the file's header and footer to explicit High (F13, #209). A's Readahead window runs to the end of the file, so it holds the footer: B's open lowers A's incomplete footer pieces from Readahead to High, beyond what A's own anacrolix reader asks for. open-keeps-windows compares the effective priority of A's incomplete pieces just before B opened and just after B's InitialPrioritize; stall_time_after is A's stall time once it resumes. The seeder runs slower here (still above the bitrate) so A's window is still incomplete when B opens.

At the start of the file the overlap is the header's last pieces, which the server's read-ahead into the socket usually reaches first: too few to measure every time.`,
	Guards: []string{"F13"},
	Params: []Param{
		{Name: "seeder_rate", Kind: Bytes, Default: "1536KiB"},
		{Name: "a_watch", Kind: Duration, Default: "1s", Doc: "How much A plays before it pauses."},
		{Name: "a_tail", Kind: Bytes, Default: "20MiB", Doc: "Where A starts: this far before the end of the file."},
		{Name: "watch_after", Kind: Duration, Default: "6s", Doc: "How much A plays after B opened."},
	},
	Checks: []string{"open-keeps-windows"},
	Key:    []string{"pieces_lowered_by_open", "stall_time_after", "stall_count"},
	Script: func(e *Env) error {
		t := e.Titles[0]
		devA, devB := e.Device("infuse-a"), e.Device("infuse-b")
		a, err := e.Player(devA, t).Open(e.Ctx, e.Conn(devA), max(t.Size-e.P.Bytes("a_tail"), 0)&^(64<<10-1))
		if err := e.Live(err); err != nil {
			return err
		}
		if err := e.Live(a.Watch(e.P.Duration("a_watch"))); err != nil {
			return err
		}
		a.Pause()
		if !e.WaitServerSettled(t, 300*time.Millisecond, 5*time.Second) {
			e.Note("A's reader didn't settle before B opened")
		}
		posA, ok := lowest(e.ServerPositions(t))
		before := e.Snapshot(t)
		created := e.ReadersCreated()
		n := e.P.Bytes("probe_bytes")
		probed := make(chan error, 1)
		go func() { probed <- e.Probe(e.Conn(devB), t, 0, n) }()
		opened := e.WaitReadersCreated(created+1, 10*time.Second)
		after := e.Snapshot(t)
		if err := <-probed; err != nil {
			return err
		}
		sc := e.Stack.Streaming
		hb, he := t.PieceRange(0, sc.HeaderPriorityBytes)
		fb, fe := t.PieceRange(t.Size-sc.FooterPriorityBytes, t.Size)
		switch {
		case !ok:
			e.Check("open-keeps-windows", report.NA, "", "reader A had no open reader server side")
		case !opened:
			e.Check("open-keeps-windows", report.NA, "", "reader B's InitialPrioritize wasn't seen within 10 s")
		default:
			begin, end := e.window(t, posA)
			ch := Compare(before, after, begin, end)
			b1, e1 := intersect(begin, end, hb, he)
			b2, e2 := intersect(begin, end, fb, fe)
			exposed := before.Above(b1, e1, types.PiecePriorityHigh) + before.Above(b2, e2, types.PiecePriorityHigh)
			e.Metric("pieces_lowered_by_open", float64(len(ch.Lowered)))
			ev := fmt.Sprintf("A at %d, window pieces %d-%d; %d of them incomplete above High where B prioritizes (header %d-%d, footer %d-%d) before B opened: %s",
				posA, begin, end-1, exposed, hb, he-1, fb, fe-1, ch)
			// F13's signature: InitialPrioritize touches the header and the
			// footer only, so every lowered piece is in one of them.
			inRanges := !slices.ContainsFunc(ch.Lowered, func(i int) bool {
				return !(i >= hb && i < he) && !(i >= fb && i < fe)
			})
			switch {
			case exposed == 0:
				e.Check("open-keeps-windows", report.NA, "", ev)
			case len(ch.Lowered) > 0 && inRanges:
				e.Check("open-keeps-windows", report.Fail, "F13", ev)
			case len(ch.Lowered) > 0:
				e.Check("open-keeps-windows", report.Fail, "", ev+" (outside the header and footer: not F13's signature)")
			default:
				e.Check("open-keeps-windows", report.Pass, "", ev)
			}
		}
		stalled := a.Model().StallTime(time.Now())
		err = e.Live(a.Watch(e.P.Duration("watch_after")))
		e.Metric("stall_time_after", report.MS(a.Model().StallTime(time.Now())-stalled))
		a.Stop()
		return err
	},
}

var pauseResume = &Scenario{
	Name: "pause-resume",
	Tier: Gate,
	Doc: `A player plays, pauses until the activity manager idles the torrent (downloads off), then resumes on the same request.

The read after the pause must wake the torrent (MarkActive) and playback carry on: idle-resume checks the torrent went idle and came back. The player's buffer is small here, so a slow wake shows as a stall. It guards the activation path of ffda214.

The idle timeout stays above the read timeout, as production's (300 s against 120 s): the idle check doesn't count a read blocked on a piece as activity, so a shorter one idles the torrent under the read, which then fails ("torrent data downloading disabled").`,
	Guards: []string{"ffda214"},
	Params: []Param{
		{Name: "idle_timeout", Kind: Duration, Default: "12s", Doc: "torrent.idle_timeout (whole seconds). Keep it above read_timeout, as production does."},
		{Name: "idle_check", Kind: Duration, Default: "250ms", Doc: "The activity manager's check interval (SetCheckInterval)."},
		{Name: "pause", Kind: Duration, Default: "25s", Doc: "The longest the player stays paused: it resumes a second after the torrent idles."},
		{Name: "max_buffer", Kind: Bytes, Default: "4MiB"},
		{Name: "watch", Kind: Duration, Default: "6s", Doc: "How much media to play before and after the pause."},
	},
	Stack: func(p Params) Stack {
		return Stack{
			Tune: func(c *config.Config) {
				c.Torrent.IdleEnabled = true
				c.Torrent.IdleTimeout = max(int(math.Ceil(p.Duration("idle_timeout").Seconds())), 1)
			},
			IdleCheckInterval: p.Duration("idle_check"),
		}
	},
	Checks: []string{"idle-resume"},
	Key:    []string{"stall_count", "stall_time", "time_to_playable"},
	Script: func(e *Env) error {
		t := e.Titles[0]
		dev := e.Device("infuse")
		watch := e.P.Duration("watch")
		p, err := e.Player(dev, t).Open(e.Ctx, e.Conn(dev), 0)
		if err := e.Live(err); err != nil {
			return err
		}
		if err := e.Live(p.Watch(watch)); err != nil {
			return err
		}
		p.Pause()
		// Hold the pause until the torrent idles (after the server has
		// filled the connection's buffers and the idle timeout has passed),
		// a moment more, then resume; give up at the pause param.
		paused := time.Now()
		idled := false
		deadline := paused.Add(e.P.Duration("pause"))
		for time.Now().Before(deadline) {
			if e.Stack.Activity != nil && string(e.Stack.Activity.GetState(t.Hash)) == "idle" {
				idled = true
				if soon := time.Now().Add(time.Second); soon.Before(deadline) {
					deadline = soon
				}
			}
			if err := e.Sleep(50 * time.Millisecond); err != nil {
				return err
			}
		}
		e.Note("paused %v", time.Since(paused).Round(100*time.Millisecond))
		received := p.Model().Received()
		if err := e.Live(p.Watch(watch)); err != nil {
			return err
		}
		more := p.Model().Received() - received
		state := "unknown"
		if e.Stack.Activity != nil {
			state = string(e.Stack.Activity.GetState(t.Hash))
		}
		ev := fmt.Sprintf("idle during the pause: %v; %s received after it; state after: %s", idled, report.Bytes(more), state)
		if idled && more > 0 && state == "active" {
			e.Check("idle-resume", report.Pass, "", ev)
		} else {
			e.Check("idle-resume", report.Fail, "", ev)
		}
		p.Stop()
		return nil
	},
}

var reconnect = &Scenario{
	Name: "reconnect",
	Tier: Gate,
	Doc: `A player's connection drops three times while it plays; each time it reconnects at once with a new Range GET from where its buffer ends, keeping the buffer.

Each drop leaves a handler, a TorrentFile and a reader to tear down: quiescence must find none left. It guards the read-path deadlock and leak fixes (5355720) and the stalls of 9de9c67.`,
	Guards: []string{"9de9c67", "5355720"},
	Params: []Param{
		{Name: "drops", Kind: Int, Default: "3", Doc: "How often the connection drops."},
		{Name: "watch", Kind: Duration, Default: "4s", Doc: "How much media to play between drops."},
	},
	Key: []string{"stall_count", "readers_created", "handler_lag_after_abort", "leaked_goroutines"},
	Script: func(e *Env) error {
		t := e.Titles[0]
		dev := e.Device("infuse")
		watch := e.P.Duration("watch")
		p, err := e.Player(dev, t).Open(e.Ctx, e.Conn(dev), 0)
		if err := e.Live(err); err != nil {
			return err
		}
		if err := e.Live(p.Watch(watch)); err != nil {
			return err
		}
		for range e.P.Int("drops") {
			if err := e.Live(p.Reconnect()); err != nil {
				return err
			}
			if err := e.Live(p.Watch(watch)); err != nil {
				return err
			}
		}
		p.Stop()
		return nil
	},
}

var nextEpisode = &Scenario{
	Name: "next-episode",
	Tier: Gate,
	Doc: `A player watches the end of episode 1 of a season pack, then opens episode 2 from the start.

Opening episode 1 prefetches episode 2's header and footer at Normal priority (3907536), so episode 2 should start quickly: next_header_prefetched_bytes is how much of its header was complete when it opened, next_time_to_playable its start.`,
	Guards: []string{"3907536"},
	Params: []Param{
		{Name: "episodes", Kind: Int, Default: "3", Doc: "Episodes in the season pack."},
		{Name: "episode_size", Kind: Bytes, Default: "48MiB", Doc: "Each episode's size."},
		{Name: "tail", Kind: Bytes, Default: "10MiB", Doc: "How much of episode 1's end the player watches."},
		{Name: "watch", Kind: Duration, Default: "5s", Doc: "How much of episode 2 to play."},
	},
	Media:  func(p Params) Media { return SeasonMedia(p, int(p.Int("episodes")), p.Bytes("episode_size")) },
	Checks: []string{"next-prefetch"},
	Key:    []string{"next_header_prefetched_bytes", "next_time_to_playable", "time_to_playable", "stall_count"},
	Script: func(e *Env) error {
		ep1, ep2 := e.Titles[0], e.Titles[1]
		dev := e.Device("infuse")
		conn := e.Conn(dev)
		p, err := e.Player(dev, ep1).Open(e.Ctx, conn, max(ep1.Size-e.P.Bytes("tail"), 0)&^(64<<10-1))
		if err := e.Live(err); err != nil {
			return err
		}
		if err := e.Live(p.UntilEOF()); err != nil {
			return err
		}
		p.Stop()
		snap := e.Snapshot(ep2)
		hb, he := ep2.PieceRange(0, e.Stack.Streaming.HeaderPriorityBytes)
		prefetched := int64(snap.Complete(hb, he)) * ep2.Layout.PieceLength()
		e.Metric("next_header_prefetched_bytes", float64(prefetched))
		ev := fmt.Sprintf("%s of episode 2's first %s complete when it opened", report.Bytes(prefetched), report.Bytes(e.Stack.Streaming.HeaderPriorityBytes))
		if prefetched > 0 {
			e.Check("next-prefetch", report.Pass, "", ev)
		} else {
			e.Check("next-prefetch", report.Fail, "", ev)
		}
		q, err := e.Player(dev, ep2).Open(e.Ctx, conn, 0)
		if err := e.Live(err); err != nil {
			return err
		}
		if err := e.Live(q.Watch(e.P.Duration("watch"))); err != nil {
			return err
		}
		st := q.Stop()
		if st.Playable {
			e.Metric("next_time_to_playable", report.MS(st.TimeToPlayable))
		}
		return nil
	},
}

var smallCache = &Scenario{
	Name: "small-cache",
	Tier: Gate,
	Doc: `A player reads the first 48 MiB of a movie through a 16 MiB piece cache, then seeks back to the start.

The filecache LRU evicts pieces anacrolix still believes complete (F11): cache-holds-complete counts them, from anacrolix's completion and the cache's completed/ files. The seek back replays evicted pieces: its latency and any broken read show what that costs. The seeder runs faster here so the cache fills quickly.`,
	Guards: []string{"F11"},
	Params: []Param{
		{Name: "cache_size", Kind: Bytes, Default: "16MiB", Doc: "torrent.global_cache_size (whole MiB)."},
		{Name: "fill", Kind: Bytes, Default: "48MiB", Doc: "How much the player reads before seeking back."},
		{Name: "seeder_rate", Kind: Bytes, Default: "8MiB"},
		{Name: "max_buffer", Kind: Bytes, Default: "64MiB"},
		{Name: "watch", Kind: Duration, Default: "3s", Doc: "How much media to play after seeking back."},
	},
	Stack: func(p Params) Stack {
		return Stack{Tune: func(c *config.Config) { c.Torrent.GlobalCacheSize = max(p.Bytes("cache_size")>>20, 1) }}
	},
	Checks: []string{"cache-holds-complete"},
	Key:    []string{"evicted_complete_pieces", "seek_latency_p50", "read_timeouts"},
	Script: func(e *Env) error {
		t := e.Titles[0]
		dev := e.Device("infuse")
		conn := e.Conn(dev)
		s := e.Player(dev, t)
		p, err := s.Open(e.Ctx, conn, 0)
		if err := e.Live(err); err != nil {
			return err
		}
		if err := e.Live(p.Fill(e.P.Bytes("fill"))); err != nil {
			return err
		}
		p.Pause()
		complete, evicted, err := e.evicted(t)
		if err != nil {
			return err
		}
		e.Metric("evicted_complete_pieces", float64(len(evicted)))
		ev := fmt.Sprintf("%d pieces complete to anacrolix, %d of them evicted from the %s cache: %v",
			complete, len(evicted), report.Bytes(e.P.Bytes("cache_size")), firstInts(evicted, 12))
		switch {
		case int64(complete)*t.Layout.PieceLength() <= e.P.Bytes("cache_size"):
			// Everything complete fits: nothing had to be evicted.
			e.Check("cache-holds-complete", report.NA, "", ev+"; it all fits in the cache, so nothing was put to the test")
		case len(evicted) == 0:
			e.Check("cache-holds-complete", report.Pass, "", ev)
		default:
			e.Check("cache-holds-complete", report.Fail, "F11", ev)
		}
		p, err = s.Seek(e.Ctx, p, conn, 0)
		if err := e.Live(err); err != nil {
			return err
		}
		if err := e.Live(p.Watch(e.P.Duration("watch"))); err != nil {
			return err
		}
		p.Stop()
		return nil
	},
}

// evicted returns how many of t's torrent pieces anacrolix believes
// complete, and those whose data the piece cache no longer holds.
func (e *Env) evicted(t *Title) (complete int, evicted []int, err error) {
	tor, ok := e.Torrent(t)
	if !ok || tor.Info() == nil {
		return 0, nil, fmt.Errorf("the torrent of %s isn't loaded", t.Name)
	}
	for i := range tor.NumPieces() {
		if !tor.PieceState(i).Complete {
			continue
		}
		complete++
		if !e.cached(tor, i) {
			evicted = append(evicted, i)
		}
	}
	return complete, evicted, nil
}

// cached reports whether the piece cache holds piece i's completed data:
// the resource storage keeps it at cache/completed/<piece hash>.
func (e *Env) cached(tor *atorrent.Torrent, i int) bool {
	info := tor.Info()
	if info == nil || i < 0 || i >= info.NumPieces() {
		return false
	}
	h := info.Piece(i).V1Hash()
	if !h.Ok {
		return false
	}
	_, err := os.Stat(filepath.Join(e.Stack.Config.Torrent.MetadataFolder, "cache", "completed", h.Value.HexString()))
	return err == nil
}

var etagStability = &Scenario{
	Name: "etag-stability",
	Tier: Gate,
	Doc: `A player asks for a file's ETag twice by HEAD and twice by PROPFIND, then resumes a download with a Range GET whose If-Range is that ETag.

The ETag comes from ModTime, which is time.Now() in every Stat (F2, #73): each request gets a new one, so If-Range never matches and the server answers 200 with the whole file. etag-stable needs one ETag and a 206.`,
	Guards: []string{"F2"},
	Checks: []string{"etag-stable"},
	Key:    []string{"etag_changes", "if_range_honoured"},
	Script: func(e *Env) error {
		t := e.Titles[0]
		dev := e.Device("infuse")
		conn := e.Conn(dev)
		var etags []string
		for range 2 {
			resp, err := e.Head(conn, t)
			if err != nil {
				return err
			}
			etags = append(etags, resp.Header.Get("ETag"))
		}
		for range 2 {
			entries, err := e.Propfind(conn, t.Path, 0)
			if err != nil {
				return fmt.Errorf("PROPFIND %s: %w", t.Path, err)
			}
			if len(entries) == 1 {
				etags = append(etags, entries[0].ETag)
			}
		}
		n := e.P.Bytes("probe_bytes")
		rng := rangeOf(0, n)
		start := time.Now()
		st, err := conn.Get(e.Ctx, t.Path, http.Header{"Range": {rng}, "If-Range": {etags[0]}})
		if err != nil {
			return fmt.Errorf("GET %s with If-Range: %w", t.Path, err)
		}
		status := st.Response.StatusCode
		switch status {
		case http.StatusPartialContent:
			if herr := checks.RangeResponse(st.Response, 0, n-1, t.Size); herr != nil {
				e.httpError(herr)
			} else if err := e.body("infuse", t, st, 0, n, start); err != nil {
				return err
			}
			st.Close()
		case http.StatusOK:
			// The whole file: drop it once the headers are in.
			e.Abandon(conn, t.Path, rng)
			st.Abort()
		default:
			e.httpError(fmt.Errorf("GET %s with Range %s and If-Range answered %d, want 206 (or 200 when the ETag changed)", t.Path, rng, status))
			e.Abandon(conn, t.Path, rng)
			st.Abort()
		}
		distinct := slices.Compact(slices.Sorted(slices.Values(etags)))
		e.Metric("etag_changes", float64(len(distinct)-1))
		honoured := 0.0
		if status == http.StatusPartialContent {
			honoured = 1
		}
		e.Metric("if_range_honoured", honoured)
		ev := fmt.Sprintf("ETags %q; GET with If-Range %s answered %d", etags, etags[0], status)
		switch {
		case len(distinct) == 1 && distinct[0] != "" && status == http.StatusPartialContent:
			e.Check("etag-stable", report.Pass, "", ev)
		case len(distinct) > 1 && status == http.StatusOK && !slices.Contains(etags, ""):
			// F2's signature: every request a new ETag, so If-Range
			// misses and the server sends the whole file.
			e.Check("etag-stable", report.Fail, "F2", ev)
		default:
			e.Check("etag-stable", report.Fail, "", ev)
		}
		return nil
	},
}

// lowest is the smallest of positions; ok is false without any.
func lowest(positions []int64) (int64, bool) {
	if len(positions) == 0 {
		return 0, false
	}
	return slices.Min(positions), true
}

func firstInts(v []int, n int) string {
	if len(v) > n {
		return fmt.Sprintf("%v … %d more", v[:n], len(v)-n)
	}
	return fmt.Sprint(v)
}

func mimeNote() string {
	if m := checks.MIME(); m.ResolvesMKV() {
		return ".mkv is " + m.MKV + " here, so F1 can't show"
	}
	return ".mkv unresolved, as in production"
}
