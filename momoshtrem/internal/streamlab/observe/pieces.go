package observe

import (
	"slices"
	"sync"
	"time"

	atorrent "github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/types"

	"github.com/shapedtime/momoshtrem/internal/streamlab/stack"
)

// pieceWatch mirrors one torrent's pieces from anacrolix's piece state
// changes (SubscribePieceStateChanges), without taking the client lock:
// every change, for the timeline, and the current state, for the readers'
// lead. Only the initial snapshot (PieceStateRuns) takes the lock.
//
// anacrolix v1.60.0 publishes a change when a piece's completion,
// progress (partial), hashing or pending state changes, and not when its
// priority moves between wanted levels (Readahead to Now, say:
// updatePiecePriority publishes only when the pending set changes). So the
// mirror is exact for complete, partial, hashing and wanted-or-not, and
// its priority levels are stale. What needs the level reads it from
// anacrolix (the readers sampler, the watchdog).
type pieceWatch struct {
	hash string

	mu        sync.Mutex
	loaded    bool
	pieceLen  int64
	initial   []atorrent.PieceState // at initialAt
	initialAt time.Time
	cur       []atorrent.PieceState
	events    []pieceEvent
	busy      time.Duration // spent applying events
	files     []fileSpan    // the torrent's files, for the readers' paths
}

// pieceEvent is a piece's new state.
type pieceEvent struct {
	t     time.Time
	index int
	state atorrent.PieceState
}

// fileSpan is a torrent file: its anacrolix path and where it lies.
type fileSpan struct {
	path         string
	offset, size int64
}

// follow waits for the torrent to load, then mirrors it until stop.
func (w *pieceWatch) follow(s *stack.Stack, stop <-chan struct{}) {
	var tor *atorrent.Torrent
	for {
		if t, ok := s.Torrent(w.hash); ok && t.Info() != nil {
			tor = t
			break
		}
		select {
		case <-stop:
			return
		case <-time.After(5 * time.Millisecond):
		}
	}
	sub := tor.SubscribePieceStateChanges()
	defer sub.Close()
	runs := tor.PieceStateRuns()
	at := time.Now()
	states := make([]atorrent.PieceState, 0, tor.NumPieces())
	for _, r := range runs {
		for range r.Length {
			states = append(states, r.PieceState)
		}
	}
	var files []fileSpan
	for _, f := range tor.Files() {
		files = append(files, fileSpan{path: f.Path(), offset: f.Offset(), size: f.Length()})
	}
	w.mu.Lock()
	w.loaded, w.pieceLen, w.initialAt, w.files = true, tor.Info().PieceLength, at, files
	w.initial, w.cur = states, slices.Clone(states)
	w.mu.Unlock()
	for {
		select {
		case <-stop:
			return
		case v, ok := <-sub.Values:
			if !ok {
				return
			}
			start := time.Now()
			w.mu.Lock()
			if v.Index >= 0 && v.Index < len(w.cur) {
				w.cur[v.Index] = v.PieceState
				w.events = append(w.events, pieceEvent{t: start, index: v.Index, state: v.PieceState})
			}
			w.busy += time.Since(start)
			w.mu.Unlock()
		}
	}
}

// lead is the complete data ahead of the file offset pos of the file
// [offset, offset+size): from pos to the first incomplete piece, 0 when
// pos's own piece is incomplete.
func (w *pieceWatch) lead(offset, size, pos int64) (int64, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.loaded || w.pieceLen <= 0 || pos < 0 || pos >= size {
		return 0, false
	}
	first := int((offset + pos) / w.pieceLen)
	last := int((offset + size - 1) / w.pieceLen)
	if first >= len(w.cur) {
		return 0, false
	}
	i := first
	for i <= last && i < len(w.cur) && w.cur[i].Complete {
		i++
	}
	end := min(int64(i)*w.pieceLen-offset, size)
	return max(end-pos, 0), true
}

// fileAt is the anacrolix path (File.Path) of the file at the torrent
// offset.
func (w *pieceWatch) fileAt(offset int64) (string, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, f := range w.files {
		if f.offset == offset {
			return f.path, true
		}
	}
	return "", false
}

// history is a copy of the mirror for after the run.
type history struct {
	loaded    bool
	pieceLen  int64
	initial   []atorrent.PieceState
	initialAt time.Time
	events    []pieceEvent
}

func (w *pieceWatch) history() history {
	w.mu.Lock()
	defer w.mu.Unlock()
	return history{loaded: w.loaded, pieceLen: w.pieceLen, initial: w.initial, initialAt: w.initialAt,
		events: slices.Clone(w.events)}
}

// completions are the times pieces [first, last] completed during the run,
// in order: a piece evicted and fetched again completes twice.
func (h history) completions(first, last int) []pieceEvent {
	if !h.loaded {
		return nil
	}
	done := map[int]bool{}
	for i := first; i <= last && i < len(h.initial); i++ {
		done[i] = h.initial[i].Complete
	}
	var out []pieceEvent
	for _, e := range h.events {
		if e.index < first || e.index > last {
			continue
		}
		if e.state.Complete && !done[e.index] {
			out = append(out, e)
		}
		done[e.index] = e.state.Complete
	}
	return out
}

// mirrorState names a piece's mirrored state by what the mirror knows
// exactly: complete, hashing, partial, wanted (its priority above None,
// whatever the level), none.
func mirrorState(st atorrent.PieceState) string {
	switch {
	case st.Complete:
		return "complete"
	case st.Checking:
		return "hashing"
	case st.Partial:
		return "partial"
	case st.Priority > types.PiecePriorityNone:
		return "wanted"
	}
	return "none"
}
