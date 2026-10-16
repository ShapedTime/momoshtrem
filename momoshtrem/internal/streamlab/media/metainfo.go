package media

import (
	"bytes"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/metainfo"
)

// generatorVersion names the content generator and the overlays. Change it
// whenever either changes: it's part of the cache key, so stale .torrent
// files are never served.
const generatorVersion = "streamlab-media/1"

// BuildMetainfo returns the layout's .torrent: a v1 info dict whose piece
// hashes come from the generator. Nothing else is set (no announce list, no
// creation date), so the info hash depends only on the spec. Hashing reads
// every byte, about 0.5 s per GiB per core; the pieces are hashed in
// parallel.
func BuildMetainfo(l *Layout) (*metainfo.MetaInfo, error) {
	infoBytes, err := bencode.Marshal(l.info(l.hashPieces()))
	if err != nil {
		return nil, fmt.Errorf("media: encoding the info dict: %w", err)
	}
	return &metainfo.MetaInfo{InfoBytes: infoBytes}, nil
}

// info is the layout's info dict with the given piece hashes.
func (l *Layout) info(pieces []byte) metainfo.Info {
	info := metainfo.Info{
		Name:        l.spec.Name,
		PieceLength: l.spec.PieceLength,
		Pieces:      pieces,
	}
	if len(l.spec.Files) == 1 && l.spec.Files[0].Path == "" {
		info.Length = l.size
		return info
	}
	for _, f := range l.spec.Files {
		info.Files = append(info.Files, metainfo.FileInfo{
			Length: f.Size,
			Path:   strings.Split(f.Path, "/"),
		})
	}
	return info
}

// hashPieces returns the SHA-1 of every piece, concatenated.
func (l *Layout) hashPieces() []byte {
	n := l.NumPieces()
	hashes := make([]byte, n*sha1.Size)
	workers := min(runtime.GOMAXPROCS(0), n)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			buf := make([]byte, l.spec.PieceLength)
			for i := w; i < n; i += workers {
				sum := l.pieceHash(i, buf)
				copy(hashes[i*sha1.Size:], sum[:])
			}
		}(w)
	}
	wg.Wait()
	return hashes
}

// pieceHash hashes piece i, using buf (one piece long) as scratch.
func (l *Layout) pieceHash(i int, buf []byte) [sha1.Size]byte {
	off := int64(i) * l.spec.PieceLength
	length := min(l.spec.PieceLength, l.size-off)
	b := buf[:length]
	_, _ = l.ReadAt(b, off) // in range: never short
	return sha1.Sum(b)
}

// Key identifies the layout's content and torrent: a hash of the spec and
// the generator's version.
func (l *Layout) Key() string {
	spec, _ := json.Marshal(l.spec) // plain data: can't fail
	sum := sha256.Sum256(append([]byte(generatorVersion+"\n"), spec...))
	return hex.EncodeToString(sum[:16])
}

// Cache keeps built metainfo on disk, keyed by the spec (seed, files and
// piece length) and the generator's version, so big layouts are hashed once
// per machine. A cache that can't be read or written only costs a rebuild.
type Cache struct {
	dir string
}

// NewCache returns a cache in dir; "" is a cache that never stores.
func NewCache(dir string) *Cache { return &Cache{dir: dir} }

// DefaultCache is os.UserCacheDir()/momoshtrem-streamlab, or a cache that
// never stores when the machine has no user cache directory.
func DefaultCache() *Cache {
	dir, err := os.UserCacheDir()
	if err != nil {
		return &Cache{}
	}
	return &Cache{dir: filepath.Join(dir, "momoshtrem-streamlab")}
}

// Metainfo returns the layout's metainfo: from the cache when it holds a
// valid copy (hit), else built and stored. Only building can fail.
func (c *Cache) Metainfo(l *Layout) (mi *metainfo.MetaInfo, hit bool, err error) {
	if c == nil || c.dir == "" {
		mi, err = BuildMetainfo(l)
		return mi, false, err
	}
	path := filepath.Join(c.dir, l.Key()+".torrent")
	if mi, err := c.load(path, l); err == nil {
		return mi, true, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		slog.Debug("streamlab: rebuilding cached metainfo", "path", path, "error", err)
	}
	mi, err = BuildMetainfo(l)
	if err != nil {
		return nil, false, err
	}
	if err := c.store(path, mi); err != nil {
		slog.Debug("streamlab: metainfo not cached", "path", path, "error", err)
	}
	return mi, false, nil
}

// load reads path and checks it is l's metainfo: the info dict must be the
// one l builds with the stored hashes, and the first, middle and last
// piece hashes must match the generator.
func (c *Cache) load(path string, l *Layout) (*metainfo.MetaInfo, error) {
	mi, err := metainfo.LoadFromFile(path)
	if err != nil {
		return nil, err
	}
	info, err := mi.UnmarshalInfo()
	if err != nil {
		return nil, err
	}
	if len(info.Pieces) != l.NumPieces()*sha1.Size {
		return nil, fmt.Errorf("has %d piece hash bytes, want %d", len(info.Pieces), l.NumPieces()*sha1.Size)
	}
	want, err := bencode.Marshal(l.info(info.Pieces))
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(mi.InfoBytes, want) {
		return nil, errors.New("info dict isn't the layout's")
	}
	buf := make([]byte, l.spec.PieceLength)
	n := l.NumPieces()
	for _, i := range []int{0, n / 2, n - 1} {
		sum := l.pieceHash(i, buf)
		if !bytes.Equal(info.Pieces[i*sha1.Size:(i+1)*sha1.Size], sum[:]) {
			return nil, fmt.Errorf("piece %d's hash doesn't match the content", i)
		}
	}
	return mi, nil
}

// store writes mi to path atomically: a temp file in the folder, renamed.
func (c *Cache) store(path string, mi *metainfo.MetaInfo) error {
	if err := os.MkdirAll(c.dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(c.dir, ".metainfo-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // a no-op once renamed
	if err := mi.Write(tmp); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
