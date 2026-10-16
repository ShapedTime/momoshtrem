package streamtrace

import (
	"testing"
	"time"
)

// The benchmark loops call the hooks directly, so that the hooks inline into
// them as they will into the hook sites.

func tfReads(r *Recorder, n int) {
	for i := range n {
		r.TFRead(uint64(i), int64(i)<<15, 32<<10, time.Duration(i), 0, 0)
	}
}

func tfReadsParallel(r *Recorder, pb *testing.PB) {
	for i := 0; pb.Next(); i++ {
		r.TFRead(uint64(i), int64(i)<<15, 32<<10, time.Duration(i), 0, 0)
	}
}

func prioUpdates(r *Recorder, n int) {
	for i := range n {
		r.PrioUpdate(uint64(i), PrioSeek, true, int64(i), int64(i)+8<<20, int64(i)+8<<20, int64(i)+40<<20, 1, time.Duration(i))
	}
}

func prioUpdatesParallel(r *Recorder, pb *testing.PB) {
	for i := 0; pb.Next(); i++ {
		r.PrioUpdate(uint64(i), PrioSeek, true, int64(i), int64(i)+8<<20, int64(i)+8<<20, int64(i)+40<<20, 1, time.Duration(i))
	}
}

// BenchmarkEmit measures a hook at each level, on a ring of the default
// size:
//
//   - off: a tf.read with tracing off, what every production hook costs when
//     STREAM_TRACE is off (one atomic load);
//   - summary: a prio.update (six payload words, the largest summary event),
//     recorded;
//   - summary-skip: a plain tf.read at summary, what a detailed-only hook
//     costs at summary (the check and a call);
//   - detailed: a plain tf.read, recorded.
//
// Each runs on one goroutine, then on GOMAXPROCS goroutines (parallel),
// where every recorded emit contends on the ring's head.
func BenchmarkEmit(b *testing.B) {
	cases := []struct {
		name     string
		level    Level
		loop     func(*Recorder, int)
		parallel func(*Recorder, *testing.PB)
	}{
		{"off", LevelOff, tfReads, tfReadsParallel},
		{"summary", LevelSummary, prioUpdates, prioUpdatesParallel},
		{"summary-skip", LevelSummary, tfReads, tfReadsParallel},
		{"detailed", LevelDetailed, tfReads, tfReadsParallel},
	}
	for _, c := range cases {
		b.Run(c.name, func(b *testing.B) {
			r := New(DefaultSize)
			r.SetLevel(c.level)
			b.ReportAllocs()
			b.ResetTimer()
			c.loop(r, b.N)
		})
		b.Run(c.name+"/parallel", func(b *testing.B) {
			r := New(DefaultSize)
			r.SetLevel(c.level)
			b.ReportAllocs()
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) { c.parallel(r, pb) })
		})
	}
}

// BenchmarkEmitDefaultOff is a hook as production writes it, through
// Default(), with tracing off: Default inlines to the recorder's address and
// the check to a single load.
func BenchmarkEmitDefaultOff(b *testing.B) {
	if Default().Level() != LevelOff {
		b.Skip("Default isn't off")
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		Default().TFRead(uint64(i), int64(i)<<15, 32<<10, time.Duration(i), 0, 0)
	}
}

// BenchmarkSince measures reading a full default ring.
func BenchmarkSince(b *testing.B) {
	r := New(DefaultSize)
	r.SetLevel(LevelDetailed)
	tfReads(r, DefaultSize)
	b.ReportAllocs()
	for b.Loop() {
		if events, _, _ := r.Since(0, 0); len(events) != DefaultSize {
			b.Fatalf("%d events", len(events))
		}
	}
}
