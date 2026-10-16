package torrent

import (
	"math"
	"testing"
	"time"
)

// reading is one torrent's counters at a sample.
type reading struct {
	hash          string
	read, written int64
}

func sample(s *SpeedSampler, clock *fakeClock, readings ...reading) {
	clock.Advance(SpeedInterval)
	stats := make([]FullStats, len(readings))
	for i, r := range readings {
		stats[i] = FullStats{InfoHash: r.hash, BytesReadData: r.read, BytesWrittenData: r.written}
	}
	s.Sample(stats)
}

func newTestSampler() (*SpeedSampler, *fakeClock) {
	clock := newFakeClock()
	return NewSpeedSampler(WithSpeedClock(clock.Now)), clock
}

func TestSpeedSampler_RatesAndSmoothing(t *testing.T) {
	const a, b = "aa", "bb"
	s, clock := newTestSampler()

	// The first reading only sets the baseline.
	sample(s, clock, reading{a, 1_000, 0})
	if r := s.Rate(a); r != (Rate{}) {
		t.Fatalf("rate after one reading = %+v, want 0", r)
	}

	// The second gives the rate itself: 2 MB in 2 s down, 200 kB up.
	sample(s, clock, reading{a, 2_001_000, 200_000})
	if r := s.Rate(a); r != (Rate{Down: 1_000_000, Up: 100_000}) {
		t.Fatalf("rate = %+v, want 1 MB/s down, 100 kB/s up", r)
	}

	// Then it smooths: the speed drops to 0, and the rate falls by
	// exp(-2s/5s) each reading instead of at once.
	sample(s, clock, reading{a, 2_001_000, 200_000})
	decay := math.Exp(-SpeedInterval.Seconds() / speedSmoothing.Seconds())
	if got, want := s.Rate(a).Down, int64(math.Round(1_000_000*decay)); got != want {
		t.Fatalf("smoothed rate = %d, want %d", got, want)
	}
	if got := s.Rate(a).Down; got <= 0 || got >= 1_000_000 {
		t.Fatalf("smoothed rate = %d, want between 0 and 1 MB/s", got)
	}

	// A second torrent adds to the total from its second reading.
	sample(s, clock, reading{a, 2_001_000, 200_000}, reading{b, 0, 0})
	before := s.Rate(a)
	sample(s, clock, reading{a, 2_001_000, 200_000}, reading{b, 600_000, 0})
	if r := s.Rate(b); r.Down != 300_000 {
		t.Fatalf("b's rate = %+v, want 300 kB/s", r)
	}
	if total, a2 := s.Total(), s.Rate(a); total.Down != a2.Down+300_000 || a2.Down >= before.Down {
		t.Fatalf("total = %+v, want a (%+v, falling) + b", total, a2)
	}
}

func TestSpeedSampler_DroppedAndReaddedTorrents(t *testing.T) {
	const a = "aa"
	s, clock := newTestSampler()
	sample(s, clock, reading{a, 0, 0})
	sample(s, clock, reading{a, 4_000_000, 0})
	if s.Rate(a).Down != 2_000_000 {
		t.Fatalf("rate = %+v", s.Rate(a))
	}

	// Unloaded: it drops out, and the total with it.
	sample(s, clock)
	if r, total := s.Rate(a), s.Total(); r != (Rate{}) || total != (Rate{}) {
		t.Fatalf("after unloading: rate %+v, total %+v; want 0", r, total)
	}

	// Loaded again, its counters start over: no negative speed, and a
	// fresh baseline.
	sample(s, clock, reading{a, 4_000_000, 0})
	sample(s, clock, reading{a, 100, 0})
	if r := s.Rate(a); r != (Rate{}) {
		t.Fatalf("after the counters went back: rate %+v, want 0", r)
	}
	sample(s, clock, reading{a, 2_000_100, 0})
	if r := s.Rate(a); r.Down != 1_000_000 {
		t.Fatalf("rate = %+v, want 1 MB/s", r)
	}
}

func TestSpeedSampler_History(t *testing.T) {
	const a = "aa"
	s, clock := newTestSampler()
	if h := s.History(); len(h) != 0 {
		t.Fatalf("history before any reading = %v", h)
	}

	// Each reading appends the total; the rate climbs by 2 kB/s a reading
	// (4 kB more every 2 s, one more each time), which the EWMA keeps
	// increasing.
	var counter int64
	for i := 0; i < SpeedHistoryLen+5; i++ {
		counter += int64(i) * 4_000
		sample(s, clock, reading{a, counter, 0})
	}
	h := s.History()
	if len(h) != SpeedHistoryLen {
		t.Fatalf("history has %d samples, want %d", len(h), SpeedHistoryLen)
	}
	for i := 1; i < len(h); i++ {
		if h[i].Down <= h[i-1].Down {
			t.Fatalf("history isn't oldest first: %v", h)
		}
	}
	if last := h[len(h)-1]; last != s.Total() {
		t.Errorf("newest sample %+v, want the total %+v", last, s.Total())
	}

	// It's a copy.
	h[0].Down = -1
	if s.History()[0].Down == -1 {
		t.Error("History returned its own slice")
	}
	if s.Interval() != 2*time.Second {
		t.Errorf("interval = %s", s.Interval())
	}
}
