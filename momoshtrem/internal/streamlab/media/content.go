package media

import "encoding/binary"

// The content is splitmix64 seeded with the layout's seed: the torrent's
// i-th 8-byte word is the stream's i-th output, little-endian. Each output
// is a pure function of (seed, i), so any offset is generated on its own.

const splitmixGamma = 0x9E3779B97F4A7C15

// word returns splitmix64's i-th output (from 0) for seed.
func word(seed, i uint64) uint64 {
	z := seed + (i+1)*splitmixGamma
	z = (z ^ (z >> 30)) * 0xBF58476D1CE4E5B9
	z = (z ^ (z >> 27)) * 0x94D049BB133111EB
	return z ^ (z >> 31)
}

// generate fills p with the content at the torrent-absolute offset off.
func generate(seed uint64, p []byte, off int64) {
	o := uint64(off)
	var w [8]byte
	if r := o & 7; r != 0 {
		binary.LittleEndian.PutUint64(w[:], word(seed, o>>3))
		n := copy(p, w[r:])
		p = p[n:]
		o += uint64(n)
	}
	i := o >> 3
	for len(p) >= 8 {
		binary.LittleEndian.PutUint64(p, word(seed, i))
		p = p[8:]
		i++
	}
	if len(p) > 0 {
		binary.LittleEndian.PutUint64(w[:], word(seed, i))
		copy(p, w[:])
	}
}
