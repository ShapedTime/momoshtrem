package streamtrace

import (
	"fmt"
	"runtime"
	"sync"
	"testing"
	"time"
)

// The concurrency test's writers emit events whose every field derives from
// (writer, i), so an event mixing words from two writes (a torn read) fails
// its check. Three kinds of different sizes share the slots, so that a
// header from one kind over another's payload fails too.

func tornKey(w, i int) uint64 { return uint64(w)<<32 | uint64(i) }

func emitChecked(r *Recorder, w, i int) {
	k := tornKey(w, i)
	switch i % 3 {
	case 0:
		r.TFRead(k, int64(k*3), int(k%(1<<20)), time.Duration(k*5), time.Duration(k*7), ReadFlags(k%64))
	case 1:
		r.PrioUpdate(k, PrioType(k%5), k%2 == 0, int64(k*11), int64(k*13), int64(k*17), int64(k*19), int(k%(1<<16)), time.Duration(k%(1<<40)))
	case 2:
		// One payload word: the hash ID carries (w, i), the states derive from it.
		id := uint32(w)<<24 | uint32(i)&(1<<24-1)
		r.TorrentState(ID(id), State(id%4), State((id+1)%4))
	}
}

// checkEvent reports a torn event, and returns its writer and i.
func checkEvent(e Event) (w, i int, err error) {
	switch e.Kind {
	case KindTFRead:
		k := e.p[0]
		want := [5]uint64{k, k * 3, uint64(clamp32(int64(k%(1<<20)))) | (k%64)<<32, k * 5, k * 7}
		if [5]uint64(e.p[:5]) != want {
			return 0, 0, fmt.Errorf("torn tf.read %d: %v, want %v", e.Seq, e.p[:5], want)
		}
		return int(k >> 32), int(uint32(k)), nil
	case KindPrioUpdate:
		k := e.p[0]
		meta := (k % 5) | bit(k%2 == 0, 1<<4) | (k%(1<<16))<<8 | (k%(1<<40))<<24
		want := [6]uint64{k, meta, k * 11, k * 13, k * 17, k * 19}
		if e.p != want {
			return 0, 0, fmt.Errorf("torn prio.update %d: %v, want %v", e.Seq, e.p, want)
		}
		return int(k >> 32), int(uint32(k)), nil
	case KindTorrentState:
		id := uint32(e.p[0])
		want := uint64(id) | uint64(id%4)<<32 | uint64((id+1)%4)<<40
		if e.p[0] != want {
			return 0, 0, fmt.Errorf("torn torrent.state %d: %#x, want %#x", e.Seq, e.p[0], want)
		}
		return int(id >> 24), int(id & (1<<24 - 1)), nil
	}
	return 0, 0, fmt.Errorf("event %d: unexpected kind %v", e.Seq, e.Kind)
}

func TestConcurrentWritersAndReader(t *testing.T) {
	writers, perWriter := 8, 30000
	if testing.Short() {
		perWriter = 3000
	}
	// Rings of 2 and 4 slots make writers lap each other mid-event, which
	// is what claim's CAS is for.
	for _, size := range []int{2, 4, 16, 256, 4096} {
		t.Run(fmt.Sprintf("ring %d", size), func(t *testing.T) {
			r := New(size)
			r.SetLevel(LevelDetailed)

			start := make(chan struct{})
			var wg sync.WaitGroup
			for w := range writers {
				wg.Add(1)
				go func() {
					defer wg.Done()
					<-start
					for i := range perWriter {
						emitChecked(r, w, i)
						if i%512 == 0 {
							runtime.Gosched() // let writers lap each other
						}
					}
				}()
			}

			stop := make(chan struct{})
			type result struct{ got, lost uint64 }
			res := make(chan result)
			go func() {
				var got, lost uint64
				cursor := uint64(1)
				lastI := make([]int, writers)
				lastT := make([]time.Duration, writers)
				for w := range lastI {
					lastI[w] = -1
				}
				read := func(limit int) bool {
					events, next, l := r.Since(cursor, limit)
					if next < cursor || uint64(len(events))+l != next-cursor {
						t.Errorf("Since(%d): %d events, lost %d, next %d", cursor, len(events), l, next)
						return false
					}
					prev := cursor - 1
					for _, e := range events {
						if e.Seq <= prev || e.Seq >= next {
							t.Errorf("seq %d out of order (prev %d, next %d)", e.Seq, prev, next)
						}
						prev = e.Seq
						w, i, err := checkEvent(e)
						if err != nil {
							t.Error(err)
							continue
						}
						// One writer's events come in its order, at increasing times.
						if i <= lastI[w] || e.Time < lastT[w] {
							t.Errorf("writer %d: event %d (seq %d) after %d", w, i, e.Seq, lastI[w])
						}
						lastI[w], lastT[w] = i, e.Time
					}
					got += uint64(len(events))
					lost += l
					progressed := next != cursor
					cursor = next
					return progressed
				}
				for {
					select {
					case <-stop:
						// Every writer is done: nothing is pending any more.
						for read(0) {
						}
						res <- result{got, lost}
						return
					default:
						read(97)
					}
				}
			}()

			close(start)
			wg.Wait()
			close(stop)
			out := <-res

			st := r.Stats()
			total := uint64(writers * perWriter)
			if st.Head != total || out.got+out.lost != total {
				t.Fatalf("emitted %d (head %d): read %d + lost %d", total, st.Head, out.got, out.lost)
			}
			if st.Dropped > out.lost {
				t.Fatalf("%d dropped but only %d lost", st.Dropped, out.lost)
			}
			t.Logf("ring %d: %d events, read %d, lost %d (dropped by writers %d)", size, total, out.got, out.lost, st.Dropped)
		})
	}
}

// TestConcurrentLevelChanges flips the level while writers emit: the ring
// is allocated once, and nothing races.
func TestConcurrentLevelChanges(t *testing.T) {
	r := New(64)
	var wg sync.WaitGroup
	stop := make(chan struct{})
	for w := range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; ; i++ {
				select {
				case <-stop:
					return
				default:
				}
				emitChecked(r, w, i)
				r.Intern("x")
				r.NewID()
			}
		}()
	}
	for i := range 200 {
		r.SetLevel(Level(i % 3))
		r.Since(0, 0)
		r.Stats()
	}
	close(stop)
	wg.Wait()
	events, _, _ := r.Since(0, 0)
	for _, e := range events {
		if _, _, err := checkEvent(e); err != nil {
			t.Error(err)
		}
	}
}
