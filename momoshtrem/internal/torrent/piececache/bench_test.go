package piececache

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha1"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/anacrolix/missinggo/v2/filecache"
	"github.com/anacrolix/missinggo/v2/resource"
	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/anacrolix/torrent/storage"
)

// The benchmarks compare upstream's filecache and piececache, next to
// unrelated completed pieces: 1k, and 50k (a ~150 GiB cache of 4 MiB pieces
// holds ~38k).
//
//	go test -run '^$' -bench . ./internal/torrent/piececache/

// opener opens a cache over root: its resource provider, and create, which
// makes an empty item.
type opener func(b *testing.B, root string) (rp resource.Provider, create func(path string) error)

var benchCaches = []struct {
	name string
	open opener
}{
	{"upstream", func(b *testing.B, root string) (resource.Provider, func(string) error) {
		c, err := filecache.NewCache(root)
		if err != nil {
			b.Fatal(err)
		}
		return c.AsResourceProvider(), func(p string) error {
			f, err := c.OpenFile(p, os.O_CREATE|os.O_WRONLY)
			if err == nil {
				err = f.Close()
			}
			return err
		}
	}},
	{"piececache", func(b *testing.B, root string) (resource.Provider, func(string) error) {
		c, err := NewCache(root)
		if err != nil {
			b.Fatal(err)
		}
		return c.AsResourceProvider(), func(p string) error {
			f, err := c.OpenFile(p, os.O_CREATE|os.O_WRONLY)
			if err == nil {
				err = f.Close()
			}
			return err
		}
	}},
}

var benchItems = []int{1000, 50000}

// openFilled opens a cache with n unrelated completed pieces.
func openFilled(b *testing.B, open opener, n int) (resource.Provider, func(string) error) {
	b.Helper()
	rp, create := open(b, b.TempDir())
	for i := range n {
		if err := create(fmt.Sprintf("completed/%040x", i)); err != nil {
			b.Fatal(err)
		}
	}
	return rp, create
}

// BenchmarkHashIncompletePiece hashes a 4 MiB piece still in its 256
// chunks, as anacrolix does once the last chunk lands: WriteTo reads the
// piece 32 KiB at a time, and each read lists the piece's chunks with
// Readdirnames. Upstream's Readdirnames walks every item in the cache;
// piececache's reads the piece's directory.
func BenchmarkHashIncompletePiece(b *testing.B) {
	const pieceLength = 4 << 20
	data := make([]byte, pieceLength)
	if _, err := rand.Read(data); err != nil {
		b.Fatal(err)
	}
	path := filepath.Join(b.TempDir(), "piece")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		b.Fatal(err)
	}
	info := metainfo.Info{PieceLength: pieceLength}
	if err := info.BuildFromFilePath(path); err != nil {
		b.Fatal(err)
	}
	infoBytes, err := bencode.Marshal(info)
	if err != nil {
		b.Fatal(err)
	}
	mi := metainfo.MetaInfo{InfoBytes: infoBytes}
	ih := mi.HashInfoBytes()
	want := info.Piece(0).V1Hash().Unwrap()

	for _, items := range benchItems {
		for _, c := range benchCaches {
			b.Run(fmt.Sprintf("items=%d/%s", items, c.name), func(b *testing.B) {
				rp, _ := openFilled(b, c.open, items)
				ts, err := storage.NewClient(storage.NewResourcePieces(rp)).OpenTorrent(context.Background(), &info, ih)
				if err != nil {
					b.Fatal(err)
				}
				piece := ts.Piece(info.Piece(0))
				for off := 0; off < pieceLength; off += testChunk {
					if _, err := piece.WriteAt(data[off:off+testChunk], int64(off)); err != nil {
						b.Fatal(err)
					}
				}
				h := sha1.New()
				b.SetBytes(pieceLength)
				for b.Loop() {
					h.Reset()
					if _, err := piece.WriteTo(h); err != nil {
						b.Fatal(err)
					}
				}
				if !bytes.Equal(h.Sum(nil), want.Bytes()) {
					b.Fatal("hash mismatch")
				}
			})
		}
	}
}

// BenchmarkReaddirnames lists a piece's 256 chunks. Each call holds the
// cache's lock throughout, so this is also how long every other read and
// write of the cache waits behind it; hashing a 4 MiB piece makes 128.
func BenchmarkReaddirnames(b *testing.B) {
	const dir = "incompleted/00000000000000000000000000000000000000ff"
	for _, items := range benchItems {
		for _, c := range benchCaches {
			b.Run(fmt.Sprintf("items=%d/%s", items, c.name), func(b *testing.B) {
				rp, create := openFilled(b, c.open, items)
				for off := 0; off < 256*testChunk; off += testChunk {
					if err := create(fmt.Sprintf("%s/%d", dir, off)); err != nil {
						b.Fatal(err)
					}
				}
				i, err := rp.NewInstance(dir)
				if err != nil {
					b.Fatal(err)
				}
				for b.Loop() {
					names, err := i.(resource.DirInstance).Readdirnames()
					if err != nil || len(names) != 256 {
						b.Fatalf("Readdirnames = %d names, %v", len(names), err)
					}
				}
			})
		}
	}
}
