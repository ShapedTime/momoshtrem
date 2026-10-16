package swarm

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/anacrolix/torrent/metainfo"
	"github.com/anacrolix/torrent/storage"

	"github.com/shapedtime/momoshtrem/internal/streamlab/media"
)

// Storage is piece storage generated from a layout: reads come from
// Layout.ReadAt, writes are dropped and every piece is complete. A seeder
// of any size writes nothing to disk, and anacrolix trusts the completion,
// so it hashes nothing on load either.
type Storage struct {
	layout *media.Layout
}

var _ storage.ClientImpl = (*Storage)(nil)

// NewStorage returns storage that serves l.
func NewStorage(l *media.Layout) *Storage { return &Storage{layout: l} }

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
			return generatedPiece{layout: s.layout, off: p.Offset(), length: p.Length()}
		},
	}, nil
}

// generatedPiece is one piece of a layout.
type generatedPiece struct {
	layout      *media.Layout
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
	n, err := p.layout.ReadAt(b, p.off+off)
	if err == nil && short {
		err = io.EOF
	}
	return n, err
}

// WriteAt drops the data: the piece is already complete.
func (generatedPiece) WriteAt(b []byte, _ int64) (int, error) { return len(b), nil }

func (generatedPiece) MarkComplete() error    { return nil }
func (generatedPiece) MarkNotComplete() error { return nil }

// Completion is always complete.
func (generatedPiece) Completion() storage.Completion {
	return storage.Completion{Ok: true, Complete: true}
}
