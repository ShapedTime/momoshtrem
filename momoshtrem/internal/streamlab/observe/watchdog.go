package observe

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime/pprof"
	"slices"
	"strings"
	"time"

	atorrent "github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/types"

	"github.com/shapedtime/momoshtrem/internal/streamlab/report"
	"github.com/shapedtime/momoshtrem/internal/torrent"
)

// maxDumps is how many trips of a repetition dump their artifacts.
const maxDumps = 3

// pieceContext is how many pieces before and after a reader's the pieces
// dump shows.
const (
	piecesBefore = 4
	piecesAfter  = 28
)

// watchdogState is the current stretch without progress.
type watchdogState struct {
	since    time.Time // when it began; zero: none
	progress float64   // reads, bytes read and downloaded, summed
	tripped  bool      // this stretch tripped already
}

// tripRecord is a watchdog trip.
type tripRecord struct {
	at        time.Time
	stuck     time.Duration
	readers   []report.TripReader
	known     string
	why       string
	artifacts []string
}

// watch trips the watchdog when requests are active, a read is blocked,
// no fault runs, and nothing was read or downloaded for cfg.Watchdog.
// Halfway there, in measure mode, it starts the flight recorder so a trip
// has the stall's last seconds; when the stretch ends it stops it.
func (o *Observer) watch(now time.Time) {
	if len(o.metrics) == 0 {
		return
	}
	m := o.metrics[len(o.metrics)-1]
	progress := m.reads + m.readBytes
	for _, d := range o.downloaded {
		progress += float64(d)
	}
	w := &o.wd
	active := len(o.cfg.Stack.Requests.Active()) > 0
	if !active || m.blocked < 1 || o.faulting() || progress != w.progress || w.since.IsZero() {
		w.since, w.progress, w.tripped = now, progress, false
		o.stopRecorder(false)
		return
	}
	stuck := now.Sub(w.since)
	if !o.cfg.Diagnose && stuck >= o.cfg.Watchdog/2 {
		o.startRecorder(true)
	}
	if stuck >= o.cfg.Watchdog && !w.tripped {
		w.tripped = true
		o.trip(now, stuck)
	}
}

// trip records a trip: the readers, whether it carries #195's signature,
// and (for the first maxDumps) the dumps.
func (o *Observer) trip(now time.Time, stuck time.Duration) {
	n := len(o.trips) + 1
	r := tripRecord{at: now, stuck: stuck}
	r.readers, r.known, r.why = o.diagnoseTrip()
	if n <= maxDumps && o.cfg.Dir != "" {
		start := time.Now()
		r.artifacts = o.dump(n, r)
		o.dumpTime += time.Since(start)
	}
	o.trips = append(o.trips, r)
}

// diagnoseTrip reads the server's readers at a trip, the stuck ones (at an
// incomplete piece) first, and says whether the trip carries #195's
// signature: the same as a broken body's in play-ok, measured live. Every
// stuck piece is wanted (its effective priority above None) and
// incomplete, its torrent active (downloads on) with a seeder connected,
// which the lab didn't pause, on at most two pieces. Anything else is a
// stall no known issue explains.
func (o *Observer) diagnoseTrip() (readers []report.TripReader, known, why string) {
	var stuck []report.TripReader
	var reasons []string
	pieces := map[string]bool{}
	for _, h := range o.hashes {
		tor, ok := o.cfg.Stack.Torrent(h)
		if !ok {
			continue
		}
		for _, snap := range o.cfg.Stack.Readers.GetReaders(h) {
			ti, ok := o.titleIndex(h, snap.FilePath)
			if !ok {
				continue
			}
			t := o.cfg.Titles[ti]
			p := t.piece(snap.Position)
			st := tor.PieceState(p)
			tr := report.TripReader{Title: t.Name, Position: snap.Position, Piece: p, State: stateName(st)}
			if st.Complete {
				readers = append(readers, tr)
				continue
			}
			stuck = append(stuck, tr)
			pieces[fmt.Sprintf("%s/%d", h, p)] = true
			switch {
			case st.Priority == types.PiecePriorityNone:
				reasons = append(reasons, fmt.Sprintf("piece %d isn't wanted (priority none)", p))
			case tor.Stats().ConnectedSeeders < 1:
				reasons = append(reasons, "no seeder is connected")
			case t.Seeder != nil && t.Seeder.Paused():
				reasons = append(reasons, "the seeder is paused (the lab's fault, undeclared)")
			case o.activity(h) != string(torrent.StateActive):
				reasons = append(reasons, "downloads are off (the torrent is "+o.activity(h)+")")
			}
		}
	}
	readers = append(stuck, readers...)
	switch {
	case len(stuck) == 0:
		return readers, "", "no reader is at an incomplete piece: the blocked reads wait on something else"
	case len(reasons) > 0:
		slices.Sort(reasons)
		return readers, "", "not #195's signature: " + strings.Join(slices.Compact(reasons), "; ")
	case len(pieces) > 2:
		return readers, "", fmt.Sprintf("not #195's signature: %d pieces stuck, more than the two one wedged connection holds", len(pieces))
	}
	return readers, "#195", "#195's signature: wanted, incomplete pieces with the seeder connected and downloads on, and nothing arriving"
}

// dump writes the trip's artifacts to the repetition's folder: the
// labelled goroutine profile, the flight recorder's snapshot, both
// clients' status and the pieces around each reader.
func (o *Observer) dump(n int, r tripRecord) []string {
	prefix := fmt.Sprintf("watchdog-%d-", n)
	var out []string
	write := func(name string, fill func(*os.File) error) {
		f, err := os.Create(filepath.Join(o.cfg.Dir, prefix+name))
		if err != nil {
			return
		}
		err = fill(f)
		if cerr := f.Close(); err == nil && cerr == nil {
			out = append(out, prefix+name)
		}
	}
	write("goroutines.txt", func(f *os.File) error { return pprof.Lookup("goroutine").WriteTo(f, 1) })
	o.recMu.Lock()
	if rec := o.rec; rec != nil {
		write("trace.out", func(f *os.File) error {
			_, err := rec.WriteTo(f)
			return err
		})
	}
	o.recMu.Unlock()
	write("status.txt", func(f *os.File) error {
		fmt.Fprintf(f, "watchdog trip %d: no progress for %v; %s\n\n== leecher ==\n", n, r.stuck.Round(time.Millisecond), r.why)
		o.cfg.Stack.Client.WriteStatus(f)
		seen := map[any]bool{}
		for _, t := range o.cfg.Titles {
			if t.Seeder == nil || seen[t.Seeder] {
				continue
			}
			seen[t.Seeder] = true
			fmt.Fprintf(f, "\n== seeder of %s (paused %v) ==\n", t.Hash, t.Seeder.Paused())
			t.Seeder.WriteStatus(f)
		}
		return nil
	})
	write("pieces.txt", func(f *os.File) error {
		o.writePieces(f, r.readers)
		return nil
	})
	return out
}

// writePieces writes the pieces around each reader, from anacrolix's
// piece state runs (under its lock, once per torrent).
func (o *Observer) writePieces(f *os.File, readers []report.TripReader) {
	runs := map[string][]atorrent.PieceState{}
	for _, r := range readers {
		var t Title
		for _, tt := range o.cfg.Titles {
			if tt.Name == r.Title {
				t = tt
			}
		}
		states, ok := runs[t.Hash]
		if !ok {
			if tor, loaded := o.cfg.Stack.Torrent(t.Hash); loaded {
				for _, run := range tor.PieceStateRuns() {
					for range run.Length {
						states = append(states, run.PieceState)
					}
				}
			}
			runs[t.Hash] = states
		}
		fmt.Fprintf(f, "%s: reader at %d, piece %d (%s)\n", r.Title, r.Position, r.Piece, r.State)
		from, to := max(r.Piece-piecesBefore, 0), min(r.Piece+piecesAfter, len(states))
		for i := from; i < to; {
			j := i
			for j < to && stateName(states[j]) == stateName(states[i]) {
				j++
			}
			mark := ""
			if r.Piece >= i && r.Piece < j {
				mark = "  <- the reader"
			}
			fmt.Fprintf(f, "  %d-%d: %s%s\n", i, j-1, stateName(states[i]), mark)
			i = j
		}
		fmt.Fprintln(f)
	}
}

// stateName names a piece state: complete, or its effective priority,
// with partial when some of it arrived and hashing while it's verified.
func stateName(st atorrent.PieceState) string {
	if st.Complete {
		return "complete"
	}
	s := priorityName(st.Priority)
	if st.Partial {
		s += " partial"
	}
	if st.Checking {
		s += " hashing"
	}
	return s
}

func priorityName(p types.PiecePriority) string {
	switch p {
	case types.PiecePriorityNone:
		return "none"
	case types.PiecePriorityNormal:
		return "normal"
	case types.PiecePriorityHigh:
		return "high"
	case types.PiecePriorityReadahead:
		return "readahead"
	case types.PiecePriorityNext:
		return "next"
	case types.PiecePriorityNow:
		return "now"
	}
	return fmt.Sprint(int(p))
}
