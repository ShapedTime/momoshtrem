package scenario

import (
	"slices"
	"strings"

	"github.com/shapedtime/momoshtrem/internal/streamlab/checks"
	"github.com/shapedtime/momoshtrem/internal/streamlab/known"
)

// The delete-data deadlock's signatures (known.DeleteDataDeadlock, #230):
// a RemoveTorrent with its data blocked for good in deletePieces, against
// an anacrolix piece hasher holding the same piece's read lock twice.
// Each goroutine is judged by where it blocks: its first frames below the
// runtime's and sync's, not anything further down its stack (every add
// has singleflight and the hash lock on its stack, blocked or not).

const (
	markNotComplete = "github.com/anacrolix/torrent/storage.piecePerResourcePiece.MarkNotComplete"
	deletePieces    = "github.com/shapedtime/momoshtrem/internal/torrent.(*service).deletePieces"
	newReader       = "github.com/anacrolix/torrent/storage.piecePerResourcePiece.NewReader"
	writeTo         = "github.com/anacrolix/torrent/storage.piecePerResourcePiece.WriteTo"
	hashLock        = "github.com/shapedtime/momoshtrem/internal/torrent.(*keyedMutex).Lock"
	sharedAdd       = "golang.org/x/sync/singleflight.(*Group).Do"
)

// call is a frame of a goroutine dump: the function and, in a debug=2
// dump, its arguments ("..." when the frame was inlined).
type call struct {
	fn, args string
}

// waiting reports whether fn is the runtime's or sync's: the frames a
// blocked goroutine waits in, above the code that blocked.
func waiting(fn string) bool {
	for _, p := range []string{"runtime.", "sync.", "internal/sync.", "internal/runtime/"} {
		if strings.HasPrefix(fn, p) {
			return true
		}
	}
	return false
}

// blockedAt splits a goroutine's frames (innermost first) where it
// blocked: the lock it waits on (the last runtime or sync frame, "" for
// none) and the frames from the code that took it.
func blockedAt(calls []call) (lock call, below []call) {
	for i, c := range calls {
		if !waiting(c.fn) {
			if i > 0 {
				lock = calls[i-1]
			}
			return lock, calls[i:]
		}
	}
	return call{}, nil
}

// dumpCalls parses one goroutine of a debug=2 dump ("goroutine N [...]:",
// then a "func(args)" line and a "\tfile:line" line per frame) into its
// frames, innermost first.
func dumpCalls(g string) []call {
	var out []call
	for _, line := range strings.Split(g, "\n") {
		if line == "" || strings.HasPrefix(line, "\t") || strings.HasPrefix(line, "goroutine ") || strings.HasPrefix(line, "created by ") {
			continue
		}
		out = append(out, splitCall(line))
	}
	return out
}

// splitCall splits "pkg.(*T).f(0x1, {0x2})" into its function and its
// arguments: the parenthesis that closes the line and the one it matches.
func splitCall(line string) call {
	if !strings.HasSuffix(line, ")") {
		return call{fn: line}
	}
	depth := 0
	for i := len(line) - 1; i >= 0; i-- {
		switch line[i] {
		case ')':
			depth++
		case '(':
			depth--
			if depth == 0 {
				return call{fn: line[:i], args: line[i+1 : len(line)-1]}
			}
		}
	}
	return call{fn: line}
}

// profileCalls parses one record of a debug=1 goroutine profile
// ("#\t0x…\tfunc+0x…\tfile:line" per frame) into its frames, innermost
// first, without arguments.
func profileCalls(rec string) []call {
	var out []call
	for _, line := range strings.Split(rec, "\n") {
		f := strings.Split(line, "\t")
		if len(f) < 3 || f[0] != "#" {
			continue
		}
		fn, _, _ := strings.Cut(f[2], "+0x")
		out = append(out, call{fn: fn})
	}
	return out
}

// deleteDeadlock reports whether a debug=2 goroutine dump shows the
// delete-data deadlock's cycle on one mutex: a goroutine blocked in
// RWMutex.Lock from MarkNotComplete, called by deletePieces, and one
// blocked in RWMutex.RLock from NewReader under the hasher's WriteTo.
// Where a build inlined the lock and printed its argument as "...", that
// side matches any address; where both print one, they must be the same.
func deleteDeadlock(dump string) bool {
	var writers, readers []string
	for _, g := range strings.Split(dump, "\n\n") {
		lock, below := blockedAt(dumpCalls(g))
		switch {
		case lock.fn == "sync.(*RWMutex).Lock" && len(below) >= 2 && below[0].fn == markNotComplete && below[1].fn == deletePieces:
			writers = append(writers, lock.args)
		case lock.fn == "sync.(*RWMutex).RLock" && len(below) >= 1 && below[0].fn == newReader &&
			slices.ContainsFunc(below, func(c call) bool { return c.fn == writeTo }):
			readers = append(readers, lock.args)
		}
	}
	for _, w := range writers {
		for _, r := range readers {
			if w == r || w == "..." || r == "..." {
				return true
			}
		}
	}
	return false
}

// stuckKnown is the known issue every stuck call of the run carries: the
// delete-data deadlock when each was a RemoveTorrent or an Open (which
// waits for the hash lock the remove holds) and each one's dump showed the
// cycle. "" otherwise.
func stuckKnown(stuck []stuckCall) string {
	if len(stuck) == 0 {
		return ""
	}
	for _, c := range stuck {
		if !c.cycle || (c.name != "RemoveTorrent" && c.name != "Open") {
			return ""
		}
	}
	return known.DeleteDataDeadlock
}

// stuckCall is a direct call past its bound.
type stuckCall struct {
	name  string
	cycle bool // its dump showed the delete-data deadlock
}

// deadlocked reports whether a leaked goroutine (a debug=1 record) is the
// delete-data deadlock's: blocked in deletePieces' MarkNotComplete, or
// queued behind the hash lock the remove holds (the lock itself, or an add
// waiting for the one in flight).
func deadlocked(rec string) bool {
	_, below := blockedAt(profileCalls(rec))
	switch {
	case len(below) >= 2 && below[0].fn == markNotComplete && below[1].fn == deletePieces:
		return true
	case len(below) >= 1 && (below[0].fn == hashLock || below[0].fn == sharedAdd):
		return true
	}
	return false
}

// quietKnown is the known issues a quiescence failure carries, joined by
// known.Sep: the delete-data deadlock when the repetition saw its cycle
// and every leaked goroutine is the remove or a caller queued behind it,
// with #225's orphaned reads beside them (orphanedReads' signature). ""
// when anything else is left.
func quietKnown(q checks.QuiescenceResult, timeouts float64, cycle bool) string {
	if !cycle || q.Readers != 0 || q.OpenFiles != 0 || q.BlockedReads != 0 || q.Sessions != 0 {
		return ""
	}
	var orphans []checks.LabelledGoroutines
	stuck := 0
	for _, g := range q.Leaked {
		if deadlocked(g.Stack) {
			stuck += g.Count
		} else {
			orphans = append(orphans, g)
		}
	}
	if stuck == 0 {
		return ""
	}
	ids := []string{known.DeleteDataDeadlock}
	if len(orphans) > 0 {
		// What's left must be #225's: requests stuck behind the hash lock
		// are the deadlock's, so judge the orphans alone.
		rest := checks.QuiescenceResult{Leaked: orphans}
		if orphanedReads(rest, timeouts) == "" {
			return ""
		}
		ids = append(ids, "#225")
	}
	slices.Sort(ids)
	return strings.Join(ids, known.Sep)
}
