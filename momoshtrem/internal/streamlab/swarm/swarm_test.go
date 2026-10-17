package swarm

import (
	"bytes"
	"context"
	"crypto/sha1"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	atorrent "github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/anacrolix/torrent/storage"
	"golang.org/x/time/rate"

	"github.com/shapedtime/momoshtrem/internal/streamlab/media"
)

const testPiece = 64 << 10

func buildPack(t *testing.T) (*media.Layout, *metainfo.MetaInfo, metainfo.Info) {
	t.Helper()
	l := media.MustNew(media.EpisodePack("Show", 1, 3, 11, testPiece, 200_003, media.MKV))
	mi, err := media.BuildMetainfo(l)
	if err != nil {
		t.Fatal(err)
	}
	info, err := mi.UnmarshalInfo()
	if err != nil {
		t.Fatal(err)
	}
	return l, mi, info
}

// Every piece the storage serves hashes to the metainfo's hash, through
// the same wrapper anacrolix reads it with, and writes change nothing.
func TestStorage_ServesTheLayout(t *testing.T) {
	l, mi, info := buildPack(t)
	ts, err := storage.NewClient(NewStorage(l)).OpenTorrent(context.Background(), &info, mi.HashInfoBytes())
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < info.NumPieces(); i++ {
		mp := info.Piece(i)
		p := ts.Piece(mp)
		if c := p.Completion(); !c.Ok || !c.Complete {
			t.Fatalf("piece %d completion = %+v", i, c)
		}
		if _, err := p.WriteAt(make([]byte, 100), 0); err != nil {
			t.Fatalf("piece %d: WriteAt: %v", i, err)
		}
		var buf bytes.Buffer
		if _, err := p.WriteTo(&buf); err != nil {
			t.Fatalf("piece %d: %v", i, err)
		}
		if int64(buf.Len()) != mp.Length() {
			t.Fatalf("piece %d: %d bytes, want %d", i, buf.Len(), mp.Length())
		}
		if sum := sha1.Sum(buf.Bytes()); !bytes.Equal(sum[:], info.Pieces[i*20:(i+1)*20]) {
			t.Fatalf("piece %d doesn't match its hash", i)
		}
		want := make([]byte, mp.Length())
		if _, err := l.ReadAt(want, mp.Offset()); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(buf.Bytes(), want) {
			t.Fatalf("piece %d differs from the layout", i)
		}
	}
}

func TestStoragePiece_ReadAtBounds(t *testing.T) {
	l, _, info := buildPack(t)
	last := info.Piece(info.NumPieces() - 1)
	p := generatedPiece{s: NewStorage(l), index: last.Index(), off: last.Offset(), length: last.Length()}
	buf := make([]byte, last.Length()+10)
	n, err := p.ReadAt(buf, 0)
	if int64(n) != last.Length() || err != io.EOF {
		t.Errorf("reading past the last piece's end = %d, %v; want %d, EOF", n, err, last.Length())
	}
	if n, err := p.ReadAt(buf, last.Length()); n != 0 || err != io.EOF {
		t.Errorf("reading at the piece's end = %d, %v", n, err)
	}
	if _, err := p.ReadAt(buf, -1); err == nil {
		t.Error("reading at -1 succeeded")
	}
}

func TestStorage_RejectsAnotherTorrent(t *testing.T) {
	l, _, info := buildPack(t)
	other := info
	other.PieceLength *= 2
	if _, err := NewStorage(l).OpenTorrent(context.Background(), &other, metainfo.Hash{}); err == nil {
		t.Error("opened a torrent with another piece length")
	}
	short := media.MustNew(media.SingleFile("x", 1, testPiece, 1000, media.Raw))
	if _, err := NewStorage(short).OpenTorrent(context.Background(), &info, metainfo.Hash{}); err == nil {
		t.Error("opened a torrent with another length")
	}
}

func TestPaceLimiter(t *testing.T) {
	if l := paceLimiter(0); l != nil {
		t.Errorf("0 B/s gives limit %v, want none", l.Limit())
	}
	for _, bps := range []int64{1000, 100 << 10, 50 << 20} {
		l := paceLimiter(bps)
		if l.Limit() != rate.Limit(bps) || l.Burst() < ChunkSize {
			t.Errorf("%d B/s: limit %v burst %d", bps, l.Limit(), l.Burst())
		}
	}
}

// A corrupt piece is served with one byte flipped, CorruptServes times,
// and then right; the other pieces are untouched. Its hash fails while
// it's bad.
func TestStorage_CorruptPiece(t *testing.T) {
	l, mi, info := buildPack(t)
	const bad = 3
	st := NewStorageWith(l, StorageOptions{Corrupt: []int{bad}, CorruptServes: 2})
	ts, err := storage.NewClient(st).OpenTorrent(context.Background(), &info, mi.HashInfoBytes())
	if err != nil {
		t.Fatal(err)
	}
	hashOK := func(i int) bool {
		var buf bytes.Buffer
		if _, err := ts.Piece(info.Piece(i)).WriteTo(&buf); err != nil {
			t.Fatalf("piece %d: %v", i, err)
		}
		sum := sha1.Sum(buf.Bytes())
		return bytes.Equal(sum[:], info.Pieces[i*20:(i+1)*20])
	}
	for serve := 1; serve <= 2; serve++ {
		if hashOK(bad) {
			t.Fatalf("serve %d of the corrupt piece passed its hash", serve)
		}
	}
	if !hashOK(bad) {
		t.Error("the third serve of the corrupt piece is still bad")
	}
	if !hashOK(bad-1) || !hashOK(bad+1) {
		t.Error("a piece next to the corrupt one is bad")
	}
	chunks := int(info.PieceLength / ChunkSize)
	if n := st.BadServes(); n != 2*chunks {
		t.Errorf("BadServes = %d, want 2 serves of %d chunks", n, chunks)
	}
	// A read that misses every chunk's first byte serves no chunk.
	if _, err := ts.Piece(info.Piece(bad)).ReadAt(make([]byte, 10), 1); err != nil {
		t.Fatal(err)
	}
	if n := st.BadServes(); n != 2*chunks {
		t.Errorf("a read away from the bad bytes counted: BadServes = %d", n)
	}
}

// The chunk that completes a corrupt piece's first serve waits the hold;
// the others, and the later serves, don't.
func TestStorage_CorruptHold(t *testing.T) {
	l, mi, info := buildPack(t)
	const hold = 300 * time.Millisecond
	st := NewStorageWith(l, StorageOptions{Corrupt: []int{1}, CorruptServes: 1, CorruptHold: hold})
	ts, err := storage.NewClient(st).OpenTorrent(context.Background(), &info, mi.HashInfoBytes())
	if err != nil {
		t.Fatal(err)
	}
	p := ts.Piece(info.Piece(1))
	buf := make([]byte, ChunkSize)
	read := func(off int64) (time.Duration, byte) {
		start := time.Now()
		if _, err := p.ReadAt(buf, off); err != nil {
			t.Fatal(err)
		}
		return time.Since(start), buf[0]
	}
	want := make([]byte, 1)
	last := info.PieceLength - ChunkSize
	for off := int64(0); off < last; off += ChunkSize {
		if took, _ := read(off); took >= hold {
			t.Errorf("chunk at %d waited %v", off, took)
		}
	}
	took, got := read(last)
	if took < hold {
		t.Errorf("the last chunk of the first serve took %v, want the %v hold", took, hold)
	}
	if _, err := l.ReadAt(want, info.Piece(1).Offset()+last); err != nil {
		t.Fatal(err)
	}
	if got != want[0]^0xff {
		t.Errorf("the held chunk's first byte = %#02x, want it flipped from %#02x", got, want[0])
	}
	if took, got := read(last); took >= hold || got != want[0] {
		t.Errorf("the second serve took %v and gave %#02x: want no hold and the right byte %#02x", took, got, want[0])
	}
}

// Paced reads take at least their bytes' worth of time past the burst,
// and Stop fails the ones waiting.
func TestStorage_Paced(t *testing.T) {
	l, mi, info := buildPack(t)
	const bps = 256 << 10
	st := NewStorageWith(l, StorageOptions{Rate: bps})
	ts, err := storage.NewClient(st).OpenTorrent(context.Background(), &info, mi.HashInfoBytes())
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	var read int64
	buf := make([]byte, 16<<10)
	for i := range 4 {
		p := ts.Piece(info.Piece(i))
		for off := int64(0); off < info.Piece(i).Length(); off += int64(len(buf)) {
			n, err := p.ReadAt(buf, off)
			if err != nil && err != io.EOF {
				t.Fatal(err)
			}
			read += int64(n)
		}
	}
	// A floor, not a timing check: a loaded machine only takes longer.
	floor := time.Duration(float64(read-bps/10) / bps * 0.8 * float64(time.Second))
	if elapsed := time.Since(start); elapsed < floor {
		t.Errorf("read %d bytes in %v at %d B/s: the pace isn't applied", read, elapsed, bps)
	}
	st.Stop()
	p := ts.Piece(info.Piece(5))
	for range 100 { // past the burst, a read must wait: it fails at once
		if _, err := p.ReadAt(buf, 0); err != nil {
			return
		}
	}
	t.Error("reads still succeed after Stop")
}

// The magnet is how a leecher finds the seeder: service.add parses it with
// TorrentSpecFromMagnetUri, which turns x.pe into a peer address.
func TestSeeder_Magnet(t *testing.T) {
	l, mi, _ := buildPack(t)
	s, err := StartSeeder(SeederConfig{Layout: l, Metainfo: mi})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	host, port, err := net.SplitHostPort(s.Addr())
	if err != nil || host != "127.0.0.1" || port == "0" {
		t.Fatalf("Addr = %q", s.Addr())
	}
	if s.InfoHash() != mi.HashInfoBytes().HexString() {
		t.Errorf("InfoHash = %s", s.InfoHash())
	}
	spec, err := atorrent.TorrentSpecFromMagnetUri(s.Magnet())
	if err != nil {
		t.Fatal(err)
	}
	if spec.InfoHash != mi.HashInfoBytes() || len(spec.PeerAddrs) != 1 || spec.PeerAddrs[0] != s.Addr() {
		t.Errorf("magnet %q gives hash %v, peers %v", s.Magnet(), spec.InfoHash, spec.PeerAddrs)
	}
	if spec.DisplayName != l.Name() {
		t.Errorf("magnet display name = %q, want %q", spec.DisplayName, l.Name())
	}
	// A plain TCP dial reaches it.
	conn, err := net.Dial("tcp", s.Addr())
	if err != nil {
		t.Fatalf("dialing the seeder: %v", err)
	}
	conn.Close()
}

func TestStartSeeder_RejectsAMismatchedLayout(t *testing.T) {
	_, mi, _ := buildPack(t)
	other := media.MustNew(media.SingleFile("x", 1, testPiece, 1000, media.Raw))
	_, err := StartSeeder(SeederConfig{Layout: other, Metainfo: mi})
	if err == nil || !strings.Contains(err.Error(), "layout") {
		t.Errorf("err = %v, want the storage's mismatch", err)
	}
	if _, err := StartSeeder(SeederConfig{Layout: other}); err == nil {
		t.Error("started without metainfo")
	}
}
