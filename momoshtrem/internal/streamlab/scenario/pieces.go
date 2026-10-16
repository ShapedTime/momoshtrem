package scenario

import (
	"fmt"
	"strings"
	"time"

	atorrent "github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/types"
)

// Snapshot is the effective priority and completion of a title's pieces
// at one moment. Effective is what anacrolix requests by: the highest of
// the file's, the readers' and the piece's own priority, None once
// complete or while downloads are disallowed (PLAN §5 "Priority
// invariants").
type Snapshot struct {
	At     time.Time
	Begin  int // the first piece's index
	States []atorrent.PieceState
}

// state returns piece i's state; ok is false outside the snapshot.
func (s Snapshot) state(i int) (atorrent.PieceState, bool) {
	if i < s.Begin || i >= s.Begin+len(s.States) {
		return atorrent.PieceState{}, false
	}
	return s.States[i-s.Begin], true
}

// Snapshot reads t's pieces, under one client lock (PieceStateRuns).
func (e *Env) Snapshot(t *Title) Snapshot {
	tor, ok := e.Torrent(t)
	if !ok || tor.Info() == nil {
		return Snapshot{At: time.Now()}
	}
	runs := tor.PieceStateRuns()
	at := time.Now()
	all := make([]atorrent.PieceState, 0, tor.NumPieces())
	for _, r := range runs {
		for range r.Length {
			all = append(all, r.PieceState)
		}
	}
	begin, end := t.Pieces()
	end = min(end, len(all))
	if begin >= end {
		return Snapshot{At: at}
	}
	return Snapshot{At: at, Begin: begin, States: all[begin:end]}
}

// PieceRange is the pieces covering the file offsets [from, to), clamped
// to the file: [begin, end).
func (t *Title) PieceRange(from, to int64) (begin, end int) {
	from, to = max(from, 0), min(to, t.Size)
	if from >= to {
		return 0, 0
	}
	return t.Piece(from), t.Piece(to-1) + 1
}

// Change is what happened to a window of pieces between two snapshots.
type Change struct {
	// Watched are the pieces incomplete in both snapshots: only their
	// priority means anything.
	Watched int
	// Lowered are the watched pieces whose effective priority dropped.
	Lowered []int
	details []string
}

// String lists the lowered pieces: "12 now→readahead, 13 readahead→high".
func (c Change) String() string {
	if len(c.Lowered) == 0 {
		return fmt.Sprintf("none of %d incomplete pieces lowered", c.Watched)
	}
	d := c.details
	if len(d) > 12 {
		d = append(d[:12:12], fmt.Sprintf("… %d more", len(c.details)-12))
	}
	return fmt.Sprintf("%d of %d incomplete pieces lowered: %s", len(c.Lowered), c.Watched, strings.Join(d, ", "))
}

// Compare compares before and after over the pieces [begin, end).
func Compare(before, after Snapshot, begin, end int) Change {
	var c Change
	for i := begin; i < end; i++ {
		b, ok1 := before.state(i)
		a, ok2 := after.state(i)
		if !ok1 || !ok2 || b.Complete || a.Complete {
			continue
		}
		c.Watched++
		if a.Priority < b.Priority {
			c.Lowered = append(c.Lowered, i)
			c.details = append(c.details, fmt.Sprintf("%d %s→%s", i, PriorityName(b.Priority), PriorityName(a.Priority)))
		}
	}
	return c
}

// Above counts the pieces of [begin, end) incomplete in s with an
// effective priority above p.
func (s Snapshot) Above(begin, end int, p types.PiecePriority) int {
	n := 0
	for i := begin; i < end; i++ {
		if st, ok := s.state(i); ok && !st.Complete && st.Priority > p {
			n++
		}
	}
	return n
}

// Complete counts the complete pieces of [begin, end).
func (s Snapshot) Complete(begin, end int) int {
	n := 0
	for i := begin; i < end; i++ {
		if st, ok := s.state(i); ok && st.Complete {
			n++
		}
	}
	return n
}

// PriorityName names a piece priority.
func PriorityName(p types.PiecePriority) string {
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
