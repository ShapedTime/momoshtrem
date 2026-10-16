package downloads

import (
	"crypto/sha1"
	"fmt"
	"testing"

	"github.com/anacrolix/torrent/metainfo"

	"github.com/shapedtime/momoshtrem/internal/torrent/piececache"
)

const testPiece = 16 * 1024

// v1Info is a v1 multi-file torrent with made-up piece hashes.
func v1Info(name string, sizes ...int64) *metainfo.Info {
	info := &metainfo.Info{Name: name, PieceLength: testPiece}
	var total int64
	for i, size := range sizes {
		info.Files = append(info.Files, metainfo.FileInfo{Path: []string{fileName(i)}, Length: size})
		total += size
	}
	n := int((total + testPiece - 1) / testPiece)
	for i := range n {
		h := sha1.Sum([]byte(fmt.Sprintf("%s/%d", name, i)))
		info.Pieces = append(info.Pieces, h[:]...)
	}
	return info
}

func fileName(i int) string { return string(rune('a'+i)) + ".mkv" }

// The spans hold every piece with a file's bytes, the boundary pieces
// shared with its neighbours included, and the cache's names for them are
// piececache's.
func TestPieceMap_V1(t *testing.T) {
	// a: pieces 0–2 (ends 1000 into piece 2); b: pieces 2–3; c: 3–5 (the
	// last piece short).
	info := v1Info("Show", 2*testPiece+1000, testPiece, 2*testPiece)
	pm := NewPieceMap(info)
	if !pm.V1() || pm.NumPieces() != 6 {
		t.Fatalf("V1 %v, %d pieces", pm.V1(), pm.NumPieces())
	}
	for path, want := range map[string]FileSpan{
		"Show/a.mkv": {Offset: 0, Length: 2*testPiece + 1000, Begin: 0, End: 3},
		"Show/b.mkv": {Offset: 2*testPiece + 1000, Length: testPiece, Begin: 2, End: 4},
		"Show/c.mkv": {Offset: 3*testPiece + 1000, Length: 2 * testPiece, Begin: 3, End: 6},
	} {
		if got, ok := pm.File(path); !ok || got != want {
			t.Errorf("File(%s) = %+v %v, want %+v", path, got, ok, want)
		}
	}
	if _, ok := pm.File("Show/missing.mkv"); ok {
		t.Error("a path the torrent hasn't got has a span")
	}
	for i := range pm.NumPieces() {
		want, ok := piececache.PieceRefFor(info.Piece(i))
		if got := pm.Ref(i); !ok || got != want {
			t.Errorf("Ref(%d) = %+v, piececache names it %+v", i, got, want)
		}
	}
	if pm.Length(5) != 1000 {
		t.Errorf("last piece = %d bytes, want 1000", pm.Length(5))
	}
	// Piece 2 holds a's last 1000 bytes and b's first 15384.
	a, _ := pm.File("Show/a.mkv")
	b, _ := pm.File("Show/b.mkv")
	if pm.Overlap(a, 2) != 1000 || pm.Overlap(b, 2) != testPiece-1000 || pm.Overlap(a, 4) != 0 {
		t.Errorf("overlaps: a %d, b %d, a in 4 %d", pm.Overlap(a, 2), pm.Overlap(b, 2), pm.Overlap(a, 4))
	}

	// A single-file torrent's file is named after the torrent.
	single := &metainfo.Info{Name: "Film.mkv", PieceLength: testPiece, Length: 3 * testPiece, Pieces: make([]byte, 60)}
	if span, ok := NewPieceMap(single).File("Film.mkv"); !ok || span.Begin != 0 || span.End != 3 {
		t.Errorf("single file = %+v %v", span, ok)
	}
}

// A hybrid torrent's files are piece aligned: a file's last piece ends with
// it, as metainfo (and so the storage) has it. A pure v2 torrent has no v1
// hashes: the cache can't name its pieces.
func TestPieceMap_V2(t *testing.T) {
	tree := metainfo.FileTree{Dir: map[string]metainfo.FileTree{
		"a.mkv": {File: metainfo.FileTreeFile{Length: testPiece + 100}},
		"b.mkv": {File: metainfo.FileTreeFile{Length: 2*testPiece + 5}},
	}}
	pure := &metainfo.Info{Name: "Show", PieceLength: testPiece, MetaVersion: 2, FileTree: tree}
	if NewPieceMap(pure).V1() {
		t.Error("a pure v2 torrent reads as v1")
	}

	hybrid := *pure
	hybrid.Pieces = make([]byte, 5*metainfo.HashSize) // a: 2 pieces, b: 3
	pm := NewPieceMap(&hybrid)
	if !pm.V1() || pm.NumPieces() != 5 {
		t.Fatalf("hybrid: V1 %v, %d pieces", pm.V1(), pm.NumPieces())
	}
	if b, ok := pm.File("Show/b.mkv"); !ok || b.Begin != 2 || b.End != 5 || b.Offset != 2*testPiece {
		t.Errorf("b = %+v %v, want pieces 2–4 from its aligned offset", b, ok)
	}
	for i := range pm.NumPieces() {
		if got, want := pm.Length(i), hybrid.Piece(i).Length(); got != want {
			t.Errorf("Length(%d) = %d, metainfo says %d", i, got, want)
		}
	}
}
