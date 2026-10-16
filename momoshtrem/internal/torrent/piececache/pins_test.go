package piececache

import (
	"bytes"
	"context"
	"crypto/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/anacrolix/torrent/storage"
)

// The pin tests run through anacrolix's resource-pieces storage, as
// production does: chunks are written to incompleted/<hex>/<offset>,
// MarkComplete copies them to completed/<hex> and deletes them, and
// completion is a Stat of completed/<hex>.

const (
	testPieceLength = 32 << 10 // two 16 KiB chunks
	testChunk       = 16 << 10
)

// pieceFixture is a single-file torrent of n pieces over random data, and
// its storage on a piececache.
type pieceFixture struct {
	info metainfo.Info
	data []byte
	refs []PieceRef
	c    *Cache
	ts   *storage.Torrent
}

func newPieceFixture(t testing.TB, n int, root string) *pieceFixture {
	t.Helper()
	data := make([]byte, n*testPieceLength)
	if _, err := rand.Read(data); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "Film.2020.mkv")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	info := metainfo.Info{PieceLength: testPieceLength}
	if err := info.BuildFromFilePath(path); err != nil {
		t.Fatal(err)
	}
	f := &pieceFixture{info: info, data: data}
	for i := range info.NumPieces() {
		ref, ok := PieceRefFor(info.Piece(i))
		if !ok {
			t.Fatal("no v1 hash")
		}
		f.refs = append(f.refs, ref)
	}
	f.open(t, root)
	return f
}

// open puts the torrent's storage on a new cache over root.
func (f *pieceFixture) open(t testing.TB, root string) {
	t.Helper()
	c, err := NewCache(root)
	if err != nil {
		t.Fatal(err)
	}
	infoBytes, err := bencode.Marshal(f.info)
	if err != nil {
		t.Fatal(err)
	}
	mi := metainfo.MetaInfo{InfoBytes: infoBytes}
	ts, err := storage.NewClient(storage.NewResourcePieces(c.AsResourceProvider())).
		OpenTorrent(context.Background(), &f.info, mi.HashInfoBytes())
	if err != nil {
		t.Fatal(err)
	}
	f.c, f.ts = c, ts
}

func (f *pieceFixture) piece(i int) storage.Piece {
	return f.ts.Piece(f.info.Piece(i))
}

// writeChunks writes piece i's chunks, as a download does.
func (f *pieceFixture) writeChunks(t testing.TB, i int) {
	t.Helper()
	p := f.info.Piece(i)
	for off := int64(0); off < p.Length(); off += testChunk {
		start := p.Offset() + off
		if _, err := f.piece(i).WriteAt(f.data[start:start+testChunk], off); err != nil {
			t.Fatal(err)
		}
	}
}

// download writes piece i and marks it complete, as a verified piece is.
func (f *pieceFixture) download(t testing.TB, i int) {
	t.Helper()
	f.writeChunks(t, i)
	if err := f.piece(i).MarkComplete(); err != nil {
		t.Fatalf("MarkComplete(%d): %v", i, err)
	}
}

// complete says whether anacrolix sees piece i complete, with its data.
func (f *pieceFixture) complete(t testing.TB, i int) bool {
	t.Helper()
	if !f.piece(i).Completion().Complete {
		return false
	}
	p := f.info.Piece(i)
	buf := make([]byte, p.Length())
	if _, err := f.piece(i).ReadAt(buf, 0); err != nil {
		t.Fatalf("ReadAt(%d): %v", i, err)
	}
	if !bytes.Equal(buf, f.data[p.Offset():p.Offset()+p.Length()]) {
		t.Fatalf("piece %d's data differs", i)
	}
	return true
}

func (f *pieceFixture) hexes(pieces ...int) []string {
	var hexes []string
	for _, i := range pieces {
		hexes = append(hexes, f.refs[i].Hex)
	}
	return hexes
}

// completeSet lists the pieces anacrolix sees complete. It only stats, so
// it leaves the access times alone; verify reads the data.
func (f *pieceFixture) completeSet(t testing.TB) map[int]bool {
	t.Helper()
	set := make(map[int]bool)
	for i := range f.info.NumPieces() {
		if f.piece(i).Completion().Complete {
			set[i] = true
		}
	}
	return set
}

// verify checks the data of every complete piece. It reads, so it makes
// them the most recently used.
func (f *pieceFixture) verify(t testing.TB) {
	t.Helper()
	for i := range f.info.NumPieces() {
		f.complete(t, i)
	}
}

func TestPinnedPiecesSurvivePressure(t *testing.T) {
	f := newPieceFixture(t, 24, t.TempDir())
	f.c.SetCapacity(4 * testPieceLength)
	f.c.Pin("keep", f.hexes(0, 1))

	for i := range 24 {
		f.download(t, i)
	}

	have := f.completeSet(t)
	if !have[0] || !have[1] {
		t.Fatalf("pinned pieces evicted: complete %v", have)
	}
	for i := 2; i < 20; i++ {
		if have[i] {
			t.Errorf("unpinned piece %d survived 20 pieces of pressure on a 4-piece cache", i)
		}
	}
	if !have[23] {
		t.Errorf("the newest piece was evicted: complete %v", have)
	}
	info := f.c.Info()
	if info.Pinned != 2*testPieceLength {
		t.Errorf("Pinned = %d, want %d", info.Pinned, 2*testPieceLength)
	}
	if info.Filled > info.Capacity {
		t.Errorf("Filled %d over Capacity %d", info.Filled, info.Capacity)
	}
	// The pinned pieces' chunks went once they were complete.
	if names := f.readdir("incompleted/" + f.refs[0].Hex); len(names) != 0 {
		t.Errorf("pinned piece's chunks left: %v", names)
	}
}

func (f *pieceFixture) readdir(dir string) []string {
	i, err := f.c.AsResourceProvider().NewInstance(dir)
	if err != nil {
		panic(err)
	}
	names, _ := i.(interface{ Readdirnames() ([]string, error) }).Readdirnames()
	return names
}

func TestPinCoversChunksWrittenLater(t *testing.T) {
	f := newPieceFixture(t, 12, t.TempDir())
	f.c.SetCapacity(3 * testPieceLength)
	// Pinned before a byte of it exists, as a Keep is.
	f.c.Pin("keep", f.hexes(0))
	f.writeChunks(t, 0) // incomplete: chunks only
	for i := 1; i < 12; i++ {
		f.download(t, i)
	}
	if names := f.readdir("incompleted/" + f.refs[0].Hex); len(names) != 2 {
		t.Fatalf("pinned chunks = %v, want both", names)
	}
	if err := f.piece(0).MarkComplete(); err != nil {
		t.Fatal(err)
	}
	if !f.complete(t, 0) {
		t.Fatal("pinned piece incomplete after MarkComplete")
	}
}

func TestReserveKeepsStreamingCacheWhenPinsFillCapacity(t *testing.T) {
	f := newPieceFixture(t, 16, t.TempDir())
	f.c.SetCapacity(4 * testPieceLength)
	f.c.SetReserve(2 * testPieceLength)
	f.c.Pin("keep", f.hexes(0, 1, 2, 3, 4))
	for i := range 16 {
		f.download(t, i)
	}
	have := f.completeSet(t)
	for i := range 5 {
		if !have[i] {
			t.Errorf("pinned piece %d evicted", i)
		}
	}
	// The pins alone are over capacity, so the unpinned bytes are kept to
	// the reserve. MarkComplete briefly holds a piece twice (its chunks and
	// its completed copy), so a 2-piece reserve keeps the newest piece.
	if !have[15] {
		t.Errorf("reserve didn't hold the newest piece: complete %v", have)
	}
	for i := 5; i < 15; i++ {
		if have[i] {
			t.Errorf("piece %d survived beyond the reserve", i)
		}
	}
	info := f.c.Info()
	if unpinned := info.Filled - info.Pinned; unpinned <= 0 || unpinned > 2*testPieceLength {
		t.Errorf("unpinned bytes = %d, want within the reserve %d", unpinned, 2*testPieceLength)
	}
	if info.Filled <= info.Capacity {
		t.Errorf("Filled %d within Capacity %d: pins should hold it over", info.Filled, info.Capacity)
	}
	f.verify(t)
}

func TestReserveWithoutPins(t *testing.T) {
	// No pins: the reserve only matters above the capacity, so a reserve
	// below it changes nothing. 4 pieces of room keep the newest 3.
	f := newPieceFixture(t, 10, t.TempDir())
	f.c.SetCapacity(4 * testPieceLength)
	f.c.SetReserve(testPieceLength)
	for i := range 10 {
		f.download(t, i)
	}
	have := f.completeSet(t)
	if len(have) != 3 || !have[7] || !have[8] || !have[9] {
		t.Errorf("complete %v, want 7, 8, 9", have)
	}
}

func TestPinsOverCapacityWithoutReserveKeepTheWrite(t *testing.T) {
	f := newPieceFixture(t, 8, t.TempDir())
	f.c.SetCapacity(2 * testPieceLength)
	f.c.Pin("keep", f.hexes(0, 1, 2))
	for i := range 3 {
		f.download(t, i)
	}
	// Nothing unpinned is left to evict: the trim stops instead of
	// panicking, and an explicit trim does too.
	f.c.TrimToCapacity()
	// With reserve 0 every unpinned byte may go, but not the key being
	// written: a chunk survives its own write, and the next write takes it.
	// Only a reserve keeps a streaming cache next to pins this large.
	f.writeChunks(t, 5)
	if names := f.readdir("incompleted/" + f.refs[5].Hex); len(names) != 1 || names[0] != "16384" {
		t.Fatalf("piece 5's chunks = %v, want the last one written", names)
	}
	for i := range 3 {
		if !f.complete(t, i) {
			t.Errorf("pinned piece %d evicted", i)
		}
	}
}

func TestHashFailRemovesPinnedPiece(t *testing.T) {
	f := newPieceFixture(t, 4, t.TempDir())
	f.c.SetCapacity(4 * testPieceLength)
	f.c.Pin("keep", f.hexes(0))
	f.download(t, 0)
	if got := f.c.Info().Pinned; got != testPieceLength {
		t.Fatalf("Pinned = %d, want %d", got, testPieceLength)
	}

	// A piece that fails its hash is marked not complete: its file goes,
	// pinned or not.
	if err := f.piece(0).MarkNotComplete(); err != nil {
		t.Fatal(err)
	}
	if f.piece(0).Completion().Complete {
		t.Fatal("piece still complete after MarkNotComplete")
	}
	if _, err := os.Stat(filepath.Join(f.c.root, "completed", f.refs[0].Hex)); !os.IsNotExist(err) {
		t.Fatalf("completed file still on disk: %v", err)
	}
	if info := f.c.Info(); info.Pinned != 0 || info.Filled != 0 || info.NumItems != 0 {
		t.Fatalf("Info after removal = %+v", info)
	}

	// The pin stays: the new download is pinned as it's written.
	f.writeChunks(t, 0)
	if got := f.c.Info().Pinned; got != testPieceLength {
		t.Errorf("re-downloaded chunks pinned %d bytes, want %d", got, testPieceLength)
	}
	if err := f.piece(0).MarkComplete(); err != nil {
		t.Fatal(err)
	}
	if !f.complete(t, 0) {
		t.Error("re-downloaded piece incomplete")
	}
}

// sameSet says whether have holds exactly the pieces in want.
func sameSet(have map[int]bool, want ...int) bool {
	if len(have) != len(want) {
		return false
	}
	for _, i := range want {
		if !have[i] {
			return false
		}
	}
	return true
}

func TestUnpinReturnsItemsToLRU(t *testing.T) {
	// 5 pieces of capacity, 2 pinned: 3 of room, which keeps the newest 2
	// unpinned pieces (MarkComplete briefly holds a piece twice).
	f := newPieceFixture(t, 8, t.TempDir())
	f.c.SetCapacity(5 * testPieceLength)
	f.c.Pin("keep", f.hexes(0, 1))
	for i := range 6 {
		f.download(t, i)
	}
	if have := f.completeSet(t); !sameSet(have, 0, 1, 4, 5) {
		t.Fatalf("before unpin, complete %v, want 0, 1, 4, 5", have)
	}
	// Reading piece 1 makes it the most recently used, pinned or not.
	f.complete(t, 1)

	f.c.Unpin("keep")
	if got := f.c.Info().Pinned; got != 0 {
		t.Fatalf("Pinned after Unpin = %d", got)
	}
	// Back in the LRU with their own access times: 0 the oldest of all, 1
	// the newest. Two more pieces evict 0, then 4.
	f.download(t, 6)
	f.download(t, 7)
	if have := f.completeSet(t); !sameSet(have, 1, 5, 6, 7) {
		t.Fatalf("after unpin, complete %v, want 1, 5, 6, 7", have)
	}
	f.verify(t)
}

func TestPinRefcountsAndReplace(t *testing.T) {
	f := newPieceFixture(t, 3, t.TempDir())
	for i := range 3 {
		f.download(t, i)
	}
	pinned := func() int64 { return f.c.Info().Pinned }

	f.c.Pin("a", f.hexes(0, 1))
	f.c.Pin("b", f.hexes(1, 2))
	if got := pinned(); got != 3*testPieceLength {
		t.Fatalf("Pinned = %d, want 3 pieces", got)
	}
	// Replacing a's set drops 0, which only a held; 1 stays for b.
	f.c.Pin("a", f.hexes(1))
	if got := pinned(); got != 2*testPieceLength {
		t.Fatalf("after replace, Pinned = %d, want 2 pieces", got)
	}
	f.c.Unpin("b")
	if got := pinned(); got != testPieceLength {
		t.Fatalf("after Unpin(b), Pinned = %d, want 1 piece", got)
	}
	// Upper case and junk: the same pin, and nothing.
	f.c.Pin("c", []string{strings.ToUpper(f.refs[1].Hex), "not hex", "", "../x"})
	f.c.Unpin("a")
	if got := pinned(); got != testPieceLength {
		t.Fatalf("after Unpin(a), Pinned = %d, want c's piece", got)
	}
	f.c.Pin("c", nil)
	if got := pinned(); got != 0 {
		t.Fatalf("Pinned = %d after every unpin", got)
	}
	f.c.mu.Lock()
	defer f.c.mu.Unlock()
	if len(f.c.owners) != 0 || len(f.c.pinCount) != 0 {
		t.Errorf("pin tables left: %v %v", f.c.owners, f.c.pinCount)
	}
	if n := f.c.policy.NumItems(); n != len(f.c.items) {
		t.Errorf("policy holds %d items, cache %d", n, len(f.c.items))
	}
}

func TestDemoteEvictsFirst(t *testing.T) {
	// 5 pieces of capacity hold 0 to 3 with room to download one more.
	f := newPieceFixture(t, 6, t.TempDir())
	f.c.SetCapacity(5 * testPieceLength)
	for i := range 4 {
		f.download(t, i)
	}
	// 0 is the oldest; demote 3 and 2, so they go first, 2 before 3 as
	// they were before.
	f.c.Demote(f.hexes(3, 2))
	f.download(t, 4)
	if have := f.completeSet(t); !sameSet(have, 0, 1, 3, 4) {
		t.Fatalf("after one piece, complete %v, want 0, 1, 3, 4", have)
	}
	f.download(t, 5)
	if have := f.completeSet(t); !sameSet(have, 0, 1, 4, 5) {
		t.Fatalf("after two pieces, complete %v, want 0, 1, 4, 5", have)
	}
}

func TestDemotePinnedKeepsTimeForUnpin(t *testing.T) {
	f := newPieceFixture(t, 6, t.TempDir())
	f.c.SetCapacity(5 * testPieceLength)
	for i := range 4 {
		f.download(t, i)
	}
	f.c.Pin("keep", f.hexes(3))
	f.c.Demote(f.hexes(3))
	f.download(t, 4) // evicts 0, the oldest unpinned
	if have := f.completeSet(t); !sameSet(have, 1, 2, 3, 4) {
		t.Fatalf("pinned and demoted: complete %v, want 1, 2, 3, 4", have)
	}
	f.c.Unpin("keep")
	f.download(t, 5) // 3 is the oldest now
	if have := f.completeSet(t); !sameSet(have, 1, 2, 4, 5) {
		t.Fatalf("unpinned: complete %v, want 1, 2, 4, 5", have)
	}
}

func TestHavePieces(t *testing.T) {
	f := newPieceFixture(t, 4, t.TempDir())
	f.download(t, 0)
	f.writeChunks(t, 1) // chunks only
	refs := []PieceRef{
		f.refs[0],
		f.refs[1],
		f.refs[2],
		{Hex: f.refs[0].Hex, Length: f.refs[0].Length - 1},
		{Hex: "zz", Length: 1},
	}
	got := f.c.HavePieces(refs)
	want := []bool{true, false, false, false, false}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("HavePieces[%d] = %v, want %v", i, got[i], want[i])
		}
	}
	for i, ref := range f.refs {
		if has, anacrolix := f.c.HavePieces([]PieceRef{ref})[0], f.piece(i).Completion().Complete; has != anacrolix {
			t.Errorf("piece %d: HavePieces %v, anacrolix's completion %v", i, has, anacrolix)
		}
	}
}
