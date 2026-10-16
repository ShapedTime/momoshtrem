package streamtrace

import (
	"testing"
	"time"
)

// emitN records n tf.close events whose file is their expected seq, from
// first on.
func emitN(r *Recorder, first, n int) {
	for i := range n {
		r.TFClose(uint64(first+i), time.Duration(first+i), int64(first+i))
	}
}

// checkRun checks events are seqs from..to, in order, each its own tf.close.
func checkRun(t *testing.T, events []Event, from, to uint64) {
	t.Helper()
	if want := int(to - from + 1); len(events) != want {
		t.Fatalf("got %d events, want %d (seq %d..%d)", len(events), want, from, to)
	}
	for i, e := range events {
		seq := from + uint64(i)
		if e.Seq != seq || e.Kind != KindTFClose || e.p[0] != seq || e.p[1] != seq || e.p[2] != seq {
			t.Fatalf("event %d: got seq %d kind %v payload %v, want seq %d tf.close", i, e.Seq, e.Kind, e.p[:3], seq)
		}
	}
}

func checkSince(t *testing.T, r *Recorder, seq uint64, limit int, wantNext, wantLost uint64) []Event {
	t.Helper()
	events, next, lost := r.Since(seq, limit)
	if next != wantNext || lost != wantLost {
		t.Fatalf("Since(%d, %d): next %d lost %d, want next %d lost %d", seq, limit, next, lost, wantNext, wantLost)
	}
	return events
}

func TestSinceOrder(t *testing.T) {
	r := New(16)
	r.SetLevel(LevelSummary)
	emitN(r, 1, 10)

	checkRun(t, checkSince(t, r, 0, 0, 11, 0), 1, 10) // 0: from the start
	checkRun(t, checkSince(t, r, 1, 0, 11, 0), 1, 10)
	checkRun(t, checkSince(t, r, 4, 3, 7, 0), 4, 6) // limit
	checkRun(t, checkSince(t, r, 10, 0, 11, 0), 10, 10)
	if events := checkSince(t, r, 11, 0, 11, 0); len(events) != 0 {
		t.Fatalf("Since(head+1) returned %d events", len(events))
	}

	var last time.Duration
	for _, e := range checkSince(t, r, 1, 0, 11, 0) {
		if e.Time < last {
			t.Fatalf("seq %d: time %v before %v", e.Seq, e.Time, last)
		}
		last = e.Time
		if e.Level != LevelSummary {
			t.Fatalf("seq %d: level %v, want summary", e.Seq, e.Level)
		}
	}
}

func TestSinceWraparoundReportsGaps(t *testing.T) {
	r := New(8)
	r.SetLevel(LevelSummary)
	emitN(r, 1, 20) // seqs 1..12 are overwritten

	checkRun(t, checkSince(t, r, 1, 0, 21, 12), 13, 20)
	checkRun(t, checkSince(t, r, 5, 0, 21, 8), 13, 20)
	checkRun(t, checkSince(t, r, 13, 0, 21, 0), 13, 20)
	checkRun(t, checkSince(t, r, 15, 2, 17, 0), 15, 16)
	checkRun(t, checkSince(t, r, 3, 2, 15, 10), 13, 14) // lost counts before the limit

	// A cursor past the newest event (the process restarted) starts over.
	checkRun(t, checkSince(t, r, 100, 0, 21, 12), 13, 20)

	// A reader keeping up with its cursor sees every event exactly once.
	cursor := uint64(21)
	for round := range 5 {
		emitN(r, int(cursor), 5)
		events, next, lost := r.Since(cursor, 0)
		if lost != 0 || next != cursor+5 {
			t.Fatalf("round %d: next %d lost %d", round, next, lost)
		}
		checkRun(t, events, cursor, cursor+4)
		cursor = next
	}
}

func TestSinceStopsAtAnEventBeingWritten(t *testing.T) {
	r := New(8)
	r.SetLevel(LevelSummary)
	emitN(r, 1, 3)
	r.head.Add(1) // seq 4 is claimed; its writer hasn't committed yet
	emitN(r, 5, 2)

	checkRun(t, checkSince(t, r, 1, 0, 4, 0), 1, 3)
	if events := checkSince(t, r, 4, 0, 4, 0); len(events) != 0 {
		t.Fatalf("returned %d events past a pending one", len(events))
	}

	// The writer commits: the next call returns it and those after it.
	s := &r.slots[4&r.mask]
	if !s.claim(4, 8) {
		t.Fatal("claim failed")
	}
	s.hdr.Store(uint64(KindTFClose) << kindShift)
	for i := range 3 {
		s.p[i].Store(4)
	}
	s.ver.Store(4 << 1)
	checkRun(t, checkSince(t, r, 4, 0, 7, 0), 4, 6)
}

func TestDroppedEventIsLostNotAwaited(t *testing.T) {
	r := New(4)
	r.SetLevel(LevelSummary)
	emitN(r, 1, 4)

	// Seq 1's writer is "still filling" its slot when seq 5 comes round to
	// it: seq 5 drops its event and leaves a tombstone.
	s := &r.slots[1]
	s.ver.Store(1<<1 | busy)
	emitN(r, 5, 1)
	if st := r.Stats(); st.Dropped != 1 || st.Head != 5 {
		t.Fatalf("stats %+v, want 1 dropped of 5", st)
	}
	checkRun(t, checkSince(t, r, 2, 0, 6, 1), 2, 4)

	// Seq 1's writer commits late. The slot then holds an older event than
	// the lap-back one the next writer expects (5): it takes the slot
	// through claim's fallback.
	s.ver.Store(1 << 1)
	emitN(r, 6, 4) // 9 lands in slot 1
	checkRun(t, checkSince(t, r, 6, 0, 10, 0), 6, 9)
	if st := r.Stats(); st.Dropped != 1 {
		t.Fatalf("dropped %d, want 1", st.Dropped)
	}
}

func TestSinceBeforeTheRingExists(t *testing.T) {
	r := New(8)
	events, next, lost := r.Since(0, 0)
	if events != nil || next != 1 || lost != 0 {
		t.Fatalf("got %v, %d, %d", events, next, lost)
	}
}
