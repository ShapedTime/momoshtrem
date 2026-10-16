package downloads

import (
	"encoding/hex"
	"sort"
	"strings"

	"github.com/anacrolix/torrent/metainfo"

	"github.com/shapedtime/momoshtrem/internal/torrent/piececache"
)

// PieceMap is a torrent's pieces as the piece cache names them (the v1
// hash in hex, and the piece's length), with each file's span of pieces.
// It's built once from the info dict, which never changes under its hash.
//
// A file's span is [Begin, End): every piece holding any of its bytes,
// the boundary pieces it shares with its neighbours included. A pure v2
// torrent has no v1 hashes, so the cache can't name its pieces: its files
// are unavailable to downloads (V1 false).
type PieceMap struct {
	pieceLength int64
	numPieces   int
	// hashes are the v1 piece hashes, 20 bytes each; nil for pure v2.
	hashes []byte
	// aligned is a v2 (hybrid) layout: files start on a piece boundary,
	// and a file's last piece ends with the file.
	aligned bool
	files   map[string]FileSpan
	// spans are the files in torrent order, for aligned piece lengths.
	spans []FileSpan
	total int64
}

// FileSpan is a file's place in its torrent: its bytes, and the pieces
// that hold them.
type FileSpan struct {
	Offset, Length int64
	// Begin and End are the pieces [Begin, End) holding its bytes.
	Begin, End int
}

// NewPieceMap reads info's layout. File paths are the client's: the
// torrent's name, then the file's path, joined by "/".
func NewPieceMap(info *metainfo.Info) *PieceMap {
	m := &PieceMap{
		pieceLength: info.PieceLength,
		numPieces:   info.NumPieces(),
		aligned:     info.FilesArePieceAligned(),
		files:       map[string]FileSpan{},
	}
	if info.HasV1() {
		m.hashes = info.Pieces
	}
	name := info.BestName()
	for fi := range info.UpvertedFilesIter() {
		span := FileSpan{Offset: fi.TorrentOffset, Length: fi.Length,
			Begin: fi.BeginPieceIndex(info.PieceLength), End: fi.EndPieceIndex(info.PieceLength)}
		span.End = min(span.End, m.numPieces)
		path := strings.Join(append([]string{name}, fi.BestPath()...), "/")
		m.files[path] = span
		m.spans = append(m.spans, span)
		m.total = max(m.total, fi.TorrentOffset+fi.Length)
	}
	return m
}

// V1 reports whether the cache can name the torrent's pieces.
func (m *PieceMap) V1() bool {
	return m != nil && len(m.hashes) >= m.numPieces*metainfo.HashSize
}

// File returns the file's span; ok is false when the torrent has no such
// file.
func (m *PieceMap) File(path string) (FileSpan, bool) {
	span, ok := m.files[path]
	return span, ok
}

// NumPieces is how many pieces the torrent has.
func (m *PieceMap) NumPieces() int { return m.numPieces }

// Length is piece i's length, as the storage writes it: the piece length,
// but for the last piece (v1), or a file's last piece (aligned layouts).
func (m *PieceMap) Length(i int) int64 {
	if !m.aligned {
		if i < m.numPieces-1 {
			return m.pieceLength
		}
		return m.total - int64(i)*m.pieceLength
	}
	// The last file starting at or before piece i holds its end (metainfo's
	// Piece.Length for v2 layouts).
	n := sort.Search(len(m.spans), func(k int) bool { return m.spans[k].Begin > i })
	if n == 0 {
		return m.pieceLength
	}
	end := m.spans[n-1].Offset + m.spans[n-1].Length
	return min(end-int64(i)*m.pieceLength, m.pieceLength)
}

// Ref is piece i as the cache names it; V1 must hold.
func (m *PieceMap) Ref(i int) piececache.PieceRef {
	h := m.hashes[i*metainfo.HashSize : (i+1)*metainfo.HashSize]
	return piececache.PieceRef{Hex: hex.EncodeToString(h), Length: m.Length(i)}
}

// Overlap is how many of the file's bytes piece i holds.
func (m *PieceMap) Overlap(span FileSpan, i int) int64 {
	start := int64(i) * m.pieceLength
	end := start + m.Length(i)
	return max(0, min(end, span.Offset+span.Length)-max(start, span.Offset))
}
