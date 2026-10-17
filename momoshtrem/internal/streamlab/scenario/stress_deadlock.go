package scenario

import (
	"regexp"
	"slices"
	"strings"

	"github.com/shapedtime/momoshtrem/internal/streamlab/checks"
	"github.com/shapedtime/momoshtrem/internal/streamlab/known"
)

// The delete-data deadlock's signatures (known.DeleteDataDeadlock): a
// RemoveTorrent with its data blocked for good in deletePieces, against
// an anacrolix piece hasher holding the same piece's read lock twice.

var (
	// rwLockLine is a goroutine blocked taking an RWMutex, in a debug=2
	// dump: "sync.(*RWMutex).Lock(0xc000235528)", or RLock.
	rwLockLine = regexp.MustCompile(`sync\.\(\*RWMutex\)\.(R?Lock)\((0x[0-9a-f]+)\)`)
)

// deleteDeadlock reports whether a debug=2 goroutine dump shows the
// delete-data deadlock's cycle: a goroutine in deletePieces' MarkNotComplete
// blocked in RWMutex.Lock, and a goroutine hashing a piece (WriteTo) blocked
// in RWMutex.RLock under NewReader, on the same mutex.
func deleteDeadlock(dump string) bool {
	writers, readers := map[string]bool{}, map[string]bool{}
	for _, g := range strings.Split(dump, "\n\n") {
		m := rwLockLine.FindStringSubmatch(g)
		if m == nil {
			continue
		}
		switch {
		case m[1] == "Lock" && strings.Contains(g, "storage.piecePerResourcePiece.MarkNotComplete") &&
			strings.Contains(g, "internal/torrent.(*service).deletePieces"):
			writers[m[2]] = true
		case m[1] == "RLock" && strings.Contains(g, "storage.piecePerResourcePiece.NewReader") &&
			strings.Contains(g, "storage.piecePerResourcePiece.WriteTo"):
			readers[m[2]] = true
		}
	}
	for addr := range writers {
		if readers[addr] {
			return true
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

// deadlockLeaks are the stacks the delete-data deadlock leaves running: the
// remove itself, and every add, open or remove of the hash queued behind
// its hash lock.
var deadlockLeaks = []string{
	"internal/torrent.(*service).deletePieces",
	"internal/torrent.(*keyedMutex).Lock",
	"singleflight.(*Group).Do",
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
	deadlocked := 0
	for _, g := range q.Leaked {
		switch {
		case slices.ContainsFunc(deadlockLeaks, func(s string) bool { return strings.Contains(g.Stack, s) }):
			deadlocked += g.Count
		default:
			orphans = append(orphans, g)
		}
	}
	if deadlocked == 0 {
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
