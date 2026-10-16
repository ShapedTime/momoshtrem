package streamtrace

import (
	"sync/atomic"
	"time"
)

// The ring is a slice of slots. Each slot is eight atomic words, one 64-byte
// cache line:
//
//	ver   seq<<1 once committed, with bit 0 (busy) set while a writer fills
//	      the slot; 0 while empty
//	hdr   kind (6 bits) | level (2 bits) | ns since the recorder started (56)
//	p     the kind's payload: up to six words of IDs and numbers (events.go)
//
// Every word is atomic, so a reader racing a writer isn't a data race, and
// neither side allocates or locks.
//
// A writer claims seq from head, then takes slot seq&mask by CAS-ing ver
// from an older committed event to busy(seq). It stores hdr and the words
// its kind uses, then commits by storing seq<<1. A reader loads ver, then
// hdr and the payload, then ver again: the event is whole when both loads
// are seq<<1. Go's atomics are sequentially consistent, and a writer sets
// busy before its first payload store, so had any payload load seen a later
// writer's store, the second ver load would have seen busy or newer.
//
// Only one writer at a time holds a slot. A writer that finds its slot still
// busy (a writer a whole lap behind it stalled mid-event) or already holding
// a newer event drops its own event: writers never wait. It leaves the seq
// in the slot's tombstone (drops) so that readers count it lost instead of
// waiting for it.
type slot struct {
	ver atomic.Uint64
	hdr atomic.Uint64
	p   [payloadWords]atomic.Uint64
}

const (
	payloadWords = 6

	busy = 1

	timeBits   = 56 // ns: 2.28 years of uptime before the clock wraps
	timeMask   = 1<<timeBits - 1
	levelShift = timeBits
	kindShift  = timeBits + 2
)

// put records an event of kind k at level lvl. Each kind stores only the
// payload words it uses (kindWords).
func (r *Recorder) put(k Kind, lvl Level, p0, p1, p2, p3, p4, p5 uint64) {
	t := uint64(time.Since(r.start)) & timeMask
	seq := r.head.Add(1)
	i := seq & r.mask
	s := &r.slots[i]
	if !s.claim(seq, r.mask+1) {
		r.drop(i, seq)
		return
	}
	s.hdr.Store(uint64(k)<<kindShift | uint64(lvl)<<levelShift | t)
	switch kindWords[k] {
	case 6:
		s.p[5].Store(p5)
		fallthrough
	case 5:
		s.p[4].Store(p4)
		fallthrough
	case 4:
		s.p[3].Store(p3)
		fallthrough
	case 3:
		s.p[2].Store(p2)
		fallthrough
	case 2:
		s.p[1].Store(p1)
		fallthrough
	case 1:
		s.p[0].Store(p0)
	}
	s.ver.Store(seq << 1)
}

// claim marks the slot busy for seq, unless it's busy for an older writer
// or already holds a newer event. lap is the ring's size.
func (s *slot) claim(seq, lap uint64) bool {
	// Usually the slot holds the event a lap back, committed (or nothing, on
	// the first lap): one CAS.
	var prev uint64
	if seq > lap {
		prev = (seq - lap) << 1
	}
	if s.ver.CompareAndSwap(prev, seq<<1|busy) {
		return true
	}
	// The event a lap back was itself dropped, or is still being written.
	v := s.ver.Load()
	if v&busy != 0 || v>>1 >= seq {
		return false
	}
	return s.ver.CompareAndSwap(v, seq<<1|busy)
}

// drop counts a dropped event and raises the slot's tombstone to its seq.
func (r *Recorder) drop(i, seq uint64) {
	r.dropped.Add(1)
	d := &r.drops[i]
	for {
		old := d.Load()
		if old >= seq || d.CompareAndSwap(old, seq) {
			return
		}
	}
}

// Event is one recorded event. Its fields beyond these are the kind's
// payload, which WriteNDJSON writes out.
type Event struct {
	// Seq is its sequence number: 1, 2, … in the order writers claimed them.
	Seq uint64
	// Time is when it happened, since the recorder started (Stats.Start).
	Time  time.Duration
	Kind  Kind
	Level Level
	p     [payloadWords]uint64
}

type slotState int

const (
	slotOK      slotState = iota
	slotGone              // overwritten by a newer event
	slotPending           // not committed yet (or dropped: see drops)
)

// read returns the slot's event if it holds seq, whole.
func (s *slot) read(seq uint64) (Event, slotState) {
	v := s.ver.Load()
	if v != seq<<1 {
		if v>>1 > seq {
			return Event{}, slotGone
		}
		return Event{}, slotPending
	}
	h := s.hdr.Load()
	e := Event{
		Seq:   seq,
		Time:  time.Duration(h & timeMask),
		Kind:  Kind(h >> kindShift),
		Level: Level(h >> levelShift & 3),
	}
	for j := range kindWords[e.Kind] {
		e.p[j] = s.p[j].Load()
	}
	if s.ver.Load() != v {
		return Event{}, slotGone // a newer writer took the slot as we read it
	}
	return e, slotOK
}

// Since returns up to limit events from sequence number seq on, oldest
// first. next is the seq to ask for next time. lost counts the events in
// [seq, next) it couldn't return, because they were overwritten before
// being read or dropped by their writer, so len(events)+lost == next-seq.
//
// It stops before an event a writer is still filling: the next call returns
// it. A limit ≤ 0 means the ring's size. seq 0 means from the start. A seq
// past the newest event (a cursor from before a restart) also starts over
// from seq 1, which the caller sees as next ≤ seq.
func (r *Recorder) Since(seq uint64, limit int) (events []Event, next, lost uint64) {
	r.mu.Lock()
	slots, drops := r.slots, r.drops
	r.mu.Unlock()
	head := r.head.Load()
	if seq == 0 || seq > head+1 {
		seq = 1
	}
	if slots == nil {
		return nil, seq, 0
	}
	lap := uint64(len(slots))
	if head > lap && seq <= head-lap {
		lost = head - lap + 1 - seq
		seq = head - lap + 1
	}
	if limit <= 0 || uint64(limit) > lap {
		limit = int(lap)
	}
	events = make([]Event, 0, min(uint64(limit), head+1-seq))
	for ; seq <= head && len(events) < limit; seq++ {
		i := seq & (lap - 1)
		e, st := slots[i].read(seq)
		switch st {
		case slotOK:
			events = append(events, e)
		case slotGone:
			lost++
		case slotPending:
			if drops[i].Load() < seq {
				return events, seq, lost // still being written
			}
			lost++
		}
	}
	return events, seq, lost
}
