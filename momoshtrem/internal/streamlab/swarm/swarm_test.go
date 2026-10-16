package swarm

import (
	"bytes"
	"context"
	"crypto/sha1"
	"io"
	"net"
	"strings"
	"testing"

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
	p := generatedPiece{layout: l, off: last.Offset(), length: last.Length()}
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

func TestUploadLimiter(t *testing.T) {
	if l := uploadLimiter(0); l.Limit() != rate.Inf {
		t.Errorf("0 B/s gives limit %v, want unlimited", l.Limit())
	}
	for _, bps := range []int64{1000, 100 << 10, 50 << 20} {
		l := uploadLimiter(bps)
		if l.Limit() != rate.Limit(bps) || l.Burst() < minUploadBurst {
			t.Errorf("%d B/s: limit %v burst %d", bps, l.Limit(), l.Burst())
		}
	}
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
