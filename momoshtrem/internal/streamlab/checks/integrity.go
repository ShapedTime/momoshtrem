// Package checks holds streamlab's checks (docs/streamlab/PLAN.md §5
// "Checks"): integrity, HTTP and quiescence, and the probes that record
// what today's code does about suspected bugs (§3: F1, F3).
package checks

import (
	"fmt"
	"sync"

	"github.com/shapedtime/momoshtrem/internal/streamlab/media"
)

// Integrity compares every byte a client received with the media it came
// from. It's safe for concurrent use.
type Integrity struct {
	layout *media.Layout

	mu         sync.Mutex
	want       []byte
	checked    int64
	mismatched int64
	first      *BadByte
	explain    func(BadByte) string
	explained  map[string]int64
}

// Explain sets how mismatches are explained: fn names the known issue
// whose signature a mismatched byte carries, "" for none. Explained
// mismatches still count (IntegrityResult.Mismatched); IntegrityResult.
// Explained counts them by issue. Set it before the first Check.
func (c *Integrity) Explain(fn func(BadByte) string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.explain = fn
}

// BadByte is a byte that didn't match the media.
type BadByte struct {
	File   string // the file's path in the torrent
	Offset int64  // in the file
	// Torrent is the torrent-absolute offset and Piece its piece.
	Torrent int64
	Piece   int
	Got     byte
	Want    byte
}

func (b BadByte) String() string {
	return fmt.Sprintf("byte %d of %s (torrent offset %d, piece %d) = %#02x, want %#02x",
		b.Offset, b.File, b.Torrent, b.Piece, b.Got, b.Want)
}

// NewIntegrity checks bytes of l's files.
func NewIntegrity(l *media.Layout) *Integrity { return &Integrity{layout: l} }

// Check compares got with file's bytes from the file offset off. Bytes
// past the file's end are mismatches.
func (c *Integrity) Check(file int, off int64, got []byte) {
	f := c.layout.Files()[file]
	c.mu.Lock()
	defer c.mu.Unlock()
	if cap(c.want) < len(got) {
		c.want = make([]byte, len(got))
	}
	want := c.want[:len(got)]
	inFile := len(got)
	if off < 0 || off >= f.Size {
		inFile = 0
	} else if rem := f.Size - off; int64(inFile) > rem {
		inFile = int(rem)
	}
	if inFile > 0 {
		if _, err := c.layout.ReadAt(want[:inFile], f.Offset+off); err != nil {
			panic(err) // in range: the layout reads any offset of its files
		}
	}
	for i, b := range got {
		if i < inFile && b == want[i] {
			continue
		}
		c.mismatched++
		if c.first != nil && c.explain == nil {
			continue
		}
		abs := f.Offset + off + int64(i)
		bad := BadByte{File: f.Path, Offset: off + int64(i), Torrent: abs, Piece: int(abs / c.layout.PieceLength()), Got: b}
		if i < inFile {
			bad.Want = want[i]
		}
		if c.first == nil {
			c.first = &bad
		}
		if c.explain != nil {
			if id := c.explain(bad); id != "" {
				if c.explained == nil {
					c.explained = map[string]int64{}
				}
				c.explained[id]++
			}
		}
	}
	c.checked += int64(len(got))
}

// IntegrityResult is what Integrity found.
type IntegrityResult struct {
	Checked    int64 // bytes compared
	Mismatched int64
	// First is the first mismatch seen; nil without one.
	First *BadByte
	// Explained counts the mismatches a known issue explains, by issue
	// (Explain); the rest are unexplained.
	Explained map[string]int64
}

// Unexplained is how many mismatches no known issue explains.
func (r IntegrityResult) Unexplained() int64 {
	n := r.Mismatched
	for _, k := range r.Explained {
		n -= k
	}
	return n
}

// Result returns what was found so far.
func (c *Integrity) Result() IntegrityResult {
	c.mu.Lock()
	defer c.mu.Unlock()
	r := IntegrityResult{Checked: c.checked, Mismatched: c.mismatched}
	if c.first != nil {
		first := *c.first
		r.First = &first
	}
	if len(c.explained) > 0 {
		r.Explained = make(map[string]int64, len(c.explained))
		for id, n := range c.explained {
			r.Explained[id] = n
		}
	}
	return r
}

// Err fails on any mismatch, with the count and the first.
func (r IntegrityResult) Err() error {
	if r.Mismatched == 0 {
		return nil
	}
	return fmt.Errorf("integrity: %d of %d bytes don't match the media; the first: %s", r.Mismatched, r.Checked, r.First)
}
