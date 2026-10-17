package scenario

import (
	"encoding/binary"
	"hash/fnv"
	"math/rand/v2"
)

// A stress script's choices (offsets, sizes, delays, release moments) come
// from strands: a generator per strand of the script, derived from the
// repetition's seed and the scenario (Env.RNG's first draw) and the
// strand's ids. A strand is the script's own sequential code, the pollers,
// the chaos loop, a playback beside the workers, or one worker in one
// round. No two goroutines share one, so --set seed=N replays every
// choice; only the goroutines' schedule varies.

// Strand kinds, a strand's first id.
const (
	strandScript uint64 = iota + 1
	strandPoll
	strandChaos
	strandPlay
	// strandWorker's ids are the round (0 outside rounds) and the worker.
	strandWorker
)

// strand returns a new generator for the strand with ids, from base.
func strand(base uint64, ids ...uint64) *rand.Rand {
	h := fnv.New64a()
	var b [8]byte
	for _, id := range ids {
		binary.LittleEndian.PutUint64(b[:], id)
		h.Write(b[:])
	}
	return rand.New(rand.NewPCG(base, h.Sum64()))
}

// intn is a random int64 in [0, n) from r; 0 when n isn't positive.
func intn(r *rand.Rand, n int64) int64 {
	if n <= 0 {
		return 0
	}
	return r.Int64N(n)
}

// pickOffset is a random offset of a size-byte file, 64 KiB aligned, that
// complete rejects, from candidates drawn from one value of r: however
// many candidates complete rejects, r advances by one draw, so the
// strand's later choices don't depend on what had downloaded. The last
// candidate when complete rejects all of them.
func pickOffset(r *rand.Rand, size int64, complete func(off int64) bool) int64 {
	src := rand.New(rand.NewPCG(r.Uint64(), 0))
	var off int64
	for range 32 {
		off = intn(src, size) &^ (64<<10 - 1)
		if !complete(off) {
			return off
		}
	}
	return off
}
