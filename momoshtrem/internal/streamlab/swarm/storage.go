package swarm

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/anacrolix/torrent/metainfo"
	"github.com/anacrolix/torrent/storage"
	"golang.org/x/time/rate"

	"github.com/shapedtime/momoshtrem/internal/streamlab/media"
)

// ChunkSize is a BitTorrent chunk, what a peer request asks for:
// anacrolix's default. A seeder reads a chunk at a time, and corruption
// works per chunk.
const ChunkSize = 16 << 10

// StorageOptions are what generated storage does besides serving its
// layout.
type StorageOptions struct {
	// Rate paces reads, in bytes per second: a read returns once its bytes'
	// tokens are in. 0 is unlimited. The burst is a tenth of a second's
	// worth, at least one chunk.
	Rate int64
	// Corrupt are pieces served wrong: the first byte of every chunk
	// flipped (BadByte). The leecher's hash check fails such a piece.
	Corrupt []int
	// CorruptServes is how many serves of each corrupt chunk are wrong,
	// after which it's served right; 0 is every serve.
	CorruptServes int
	// CorruptHold is how long the last chunk of a corrupt piece's first
	// serve waits before it's served: the piece stays incomplete, and so
	// unverified, that long after its other (bad) chunks went out. 0
	// doesn't wait.
	CorruptHold time.Duration
	// Missing are pieces the storage doesn't have: a partial seeder's.
	Missing []int
}

// BadByte reports whether a corrupt piece's serve flips the byte at the
// piece-relative offset off: the first byte of each chunk.
func BadByte(off int64) bool { return off%ChunkSize == 0 }

// Storage is piece storage generated from a layout: reads come from
// Layout.ReadAt, writes are dropped and every piece is complete. A seeder
// of any size writes nothing to disk, and anacrolix trusts the completion,
// so it hashes nothing on load either.
//
// A seeder's reads can be paced (StorageOptions.Rate): anacrolix reads a
// peer request's chunk with the client unlocked (PeerConn.servePeerRequest)
// and sends it once read, so pacing the reads paces the upload without
// anacrolix's UploadRateLimiter, whose retry timer is one of #195's lost
// wakeups (signature D).
type Storage struct {
	layout *media.Layout
	pace   *rate.Limiter // nil: unpaced
	// ctx ends the paced and held waits when the seeder stops.
	ctx  context.Context
	stop context.CancelFunc

	missing       map[int]bool
	corruptServes int
	hold          time.Duration

	mu        sync.Mutex
	corrupt   map[int]map[int64]int // corrupt piece -> chunk offset -> serves
	badServes int
}

var _ storage.ClientImpl = (*Storage)(nil)

// NewStorage returns storage that serves l as it is, unpaced.
func NewStorage(l *media.Layout) *Storage { return NewStorageWith(l, StorageOptions{}) }

// NewStorageWith returns storage that serves l with opts.
func NewStorageWith(l *media.Layout, opts StorageOptions) *Storage {
	ctx, stop := context.WithCancel(context.Background())
	s := &Storage{layout: l, pace: paceLimiter(opts.Rate), ctx: ctx, stop: stop,
		corruptServes: opts.CorruptServes, hold: opts.CorruptHold}
	if len(opts.Corrupt) > 0 {
		s.corrupt = map[int]map[int64]int{}
		for _, p := range opts.Corrupt {
			s.corrupt[p] = map[int64]int{}
		}
	}
	if len(opts.Missing) > 0 {
		s.missing = map[int]bool{}
		for _, p := range opts.Missing {
			s.missing[p] = true
		}
	}
	return s
}

// paceLimiter is a limiter of bps bytes per second; nil for 0.
func paceLimiter(bps int64) *rate.Limiter {
	if bps <= 0 {
		return nil
	}
	return rate.NewLimiter(rate.Limit(bps), int(max(ChunkSize, bps/10)))
}

// Has reports whether the storage has piece i (StorageOptions.Missing).
func (s *Storage) Has(i int) bool { return !s.missing[i] }

// BadServes is how many corrupt chunks the storage served.
func (s *Storage) BadServes() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.badServes
}

// Stop ends the paced and held reads waiting: they fail. A seeder stops
// its storage when it closes.
func (s *Storage) Stop() { s.stop() }

// OpenTorrent opens the layout's torrent. Its info must describe the
// layout: same total length and piece length.
func (s *Storage) OpenTorrent(_ context.Context, info *metainfo.Info, _ metainfo.Hash) (storage.TorrentImpl, error) {
	if info.TotalLength() != s.layout.Size() || info.PieceLength != s.layout.PieceLength() {
		return storage.TorrentImpl{}, fmt.Errorf(
			"swarm: torrent %q (%d bytes, %d-byte pieces) isn't the layout's (%d bytes, %d-byte pieces)",
			info.Name, info.TotalLength(), info.PieceLength, s.layout.Size(), s.layout.PieceLength())
	}
	return storage.TorrentImpl{
		Piece: func(p metainfo.Piece) storage.PieceImpl {
			return generatedPiece{s: s, index: p.Index(), off: p.Offset(), length: p.Length()}
		},
	}, nil
}

// wait paces a read of n bytes; an error once the storage stopped.
func (s *Storage) wait(n int) error {
	if s.pace == nil {
		return nil
	}
	for n > 0 {
		step := min(n, s.pace.Burst())
		if err := s.pace.WaitN(s.ctx, step); err != nil {
			return err
		}
		n -= step
	}
	return nil
}

// sleep waits d, or until the storage stops.
func (s *Storage) sleep(d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-s.ctx.Done():
		return s.ctx.Err()
	}
}

// serve records a read of [off, off+n) of piece index (piece-relative,
// length bytes long) and returns the chunk offsets whose bytes it serves
// wrong, and how long it must hold first: the read that completes the
// piece's first serve waits the hold.
func (s *Storage) serve(index int, length, off int64, n int) (bad []int64, hold time.Duration) {
	if s.corrupt == nil {
		return nil, 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	served, ok := s.corrupt[index]
	if !ok {
		return nil, 0
	}
	chunks := int((length + ChunkSize - 1) / ChunkSize)
	firstRound := len(served) < chunks
	for c := (off + ChunkSize - 1) / ChunkSize * ChunkSize; c < off+int64(n) && c < length; c += ChunkSize {
		served[c]++
		if s.corruptServes == 0 || served[c] <= s.corruptServes {
			bad = append(bad, c)
			s.badServes++
		}
	}
	if firstRound && len(served) == chunks {
		hold = s.hold
	}
	return bad, hold
}

// generatedPiece is one piece of a layout.
type generatedPiece struct {
	s           *Storage
	index       int
	off, length int64
}

var _ storage.PieceImpl = generatedPiece{}

// ReadAt reads the piece at the piece-relative offset off.
func (p generatedPiece) ReadAt(b []byte, off int64) (int, error) {
	if off < 0 {
		return 0, os.ErrInvalid
	}
	if off >= p.length {
		return 0, io.EOF
	}
	short := false
	if rem := p.length - off; int64(len(b)) > rem {
		b = b[:rem]
		short = true
	}
	bad, hold := p.s.serve(p.index, p.length, off, len(b))
	if hold > 0 {
		if err := p.s.sleep(hold); err != nil {
			return 0, fmt.Errorf("swarm: the seeder stopped: %w", err)
		}
	}
	if err := p.s.wait(len(b)); err != nil {
		return 0, fmt.Errorf("swarm: the seeder stopped: %w", err)
	}
	n, err := p.s.layout.ReadAt(b, p.off+off)
	for _, c := range bad {
		if i := c - off; i < int64(n) {
			b[i] ^= 0xff
		}
	}
	if err == nil && short {
		err = io.EOF
	}
	return n, err
}

// WriteAt drops the data: the piece is already complete.
func (generatedPiece) WriteAt(b []byte, _ int64) (int, error) { return len(b), nil }

func (generatedPiece) MarkComplete() error    { return nil }
func (generatedPiece) MarkNotComplete() error { return nil }

// Completion is complete, but for a missing piece.
func (p generatedPiece) Completion() storage.Completion {
	return storage.Completion{Ok: true, Complete: p.s.Has(p.index)}
}
