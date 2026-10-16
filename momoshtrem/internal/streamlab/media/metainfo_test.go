package media

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/metainfo"
)

// writeFiles writes the layout's files under dir as a torrent client
// would lay them out on disk, and returns the torrent's root path.
func writeFiles(t *testing.T, l *Layout, dir string) string {
	t.Helper()
	for i, f := range l.Files() {
		p := filepath.Join(dir, filepath.FromSlash(f.Path))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(l.FileReader(i))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return filepath.Join(dir, l.Name())
}

// The info dict is the one anacrolix builds from the same files on disk,
// so the piece hashes and the file list are right.
func TestBuildMetainfo_MatchesAnacrolixFromFiles(t *testing.T) {
	specs := map[string]Spec{
		"single file": SingleFile("Film.2020.1080p.mkv", 5, testPiece, 1_000_003, MKV),
		"pack":        EpisodePack("Show", 2, 3, 6, testPiece, 300_007, MP4),
	}
	for name, spec := range specs {
		t.Run(name, func(t *testing.T) {
			l := MustNew(spec)
			mi, err := BuildMetainfo(l)
			if err != nil {
				t.Fatal(err)
			}
			want := metainfo.Info{PieceLength: testPiece}
			if err := want.BuildFromFilePath(writeFiles(t, l, t.TempDir())); err != nil {
				t.Fatal(err)
			}
			wantBytes, err := bencode.Marshal(want)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(mi.InfoBytes, wantBytes) {
				got, _ := mi.UnmarshalInfo()
				t.Fatalf("info dict differs from anacrolix's:\n got %+v\nwant %+v", infoSummary(got), infoSummary(want))
			}
			again, err := BuildMetainfo(l)
			if err != nil {
				t.Fatal(err)
			}
			if again.HashInfoBytes() != mi.HashInfoBytes() {
				t.Error("two builds have different info hashes")
			}
		})
	}
}

func infoSummary(i metainfo.Info) string {
	var b strings.Builder
	b.WriteString(i.Name)
	for _, f := range i.UpvertedFiles() {
		b.WriteString(" " + strings.Join(f.Path, "/"))
	}
	return b.String()
}

func TestCache_MissThenHit(t *testing.T) {
	c := NewCache(t.TempDir())
	l := MustNew(SingleFile("Film.mkv", 1, testPiece, 2_000_000, MKV))
	built, hit, err := c.Metainfo(l)
	if err != nil || hit {
		t.Fatalf("first Metainfo: hit=%v err=%v; want a miss", hit, err)
	}
	cached, hit, err := c.Metainfo(l)
	if err != nil || !hit {
		t.Fatalf("second Metainfo: hit=%v err=%v; want a hit", hit, err)
	}
	if !bytes.Equal(cached.InfoBytes, built.InfoBytes) {
		t.Error("the cached info dict differs from the built one")
	}

	// Another seed, piece length or size is another key.
	for _, other := range []Spec{
		SingleFile("Film.mkv", 2, testPiece, 2_000_000, MKV),
		SingleFile("Film.mkv", 1, 2*testPiece, 2_000_000, MKV),
		SingleFile("Film.mkv", 1, testPiece, 2_000_001, MKV),
		SingleFile("Film.mkv", 1, testPiece, 2_000_000, Raw),
	} {
		ol := MustNew(other)
		if ol.Key() == l.Key() {
			t.Errorf("%+v has the same key", other)
		}
		if _, hit, _ := c.Metainfo(ol); hit {
			t.Errorf("%+v hit another layout's entry", other)
		}
	}
}

// A corrupt entry, or one whose content doesn't match (another generator),
// is rebuilt and replaced.
func TestCache_RejectsBadEntries(t *testing.T) {
	l := MustNew(SingleFile("Film.mkv", 1, testPiece, 2_000_000, MKV))
	wrongContent, err := BuildMetainfo(MustNew(SingleFile("Film.mkv", 99, testPiece, 2_000_000, MKV)))
	if err != nil {
		t.Fatal(err)
	}
	var wrong bytes.Buffer
	if err := wrongContent.Write(&wrong); err != nil {
		t.Fatal(err)
	}
	entries := map[string][]byte{
		"not bencode":   []byte("garbage"),
		"other content": wrong.Bytes(),
	}
	for name, data := range entries {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			c := NewCache(dir)
			path := filepath.Join(dir, l.Key()+".torrent")
			if err := os.WriteFile(path, data, 0o644); err != nil {
				t.Fatal(err)
			}
			mi, hit, err := c.Metainfo(l)
			if err != nil || hit {
				t.Fatalf("hit=%v err=%v; want a rebuild", hit, err)
			}
			want, _ := BuildMetainfo(l)
			if mi.HashInfoBytes() != want.HashInfoBytes() {
				t.Error("returned the bad entry")
			}
			if _, hit, _ := c.Metainfo(l); !hit {
				t.Error("the rebuilt entry wasn't stored")
			}
		})
	}
}

func TestCache_UnwritableFallsBackToBuilding(t *testing.T) {
	// A regular file where the cache folder should be.
	blocker := filepath.Join(t.TempDir(), "cache")
	if err := os.WriteFile(blocker, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	l := MustNew(SingleFile("Film.mkv", 1, testPiece, 500_000, Raw))
	want, _ := BuildMetainfo(l)
	for _, c := range []*Cache{NewCache(blocker), NewCache(""), nil} {
		mi, hit, err := c.Metainfo(l)
		if err != nil || hit || mi.HashInfoBytes() != want.HashInfoBytes() {
			t.Errorf("cache %+v: hit=%v err=%v", c, hit, err)
		}
	}
}

func TestDefaultCache(t *testing.T) {
	if _, err := os.UserCacheDir(); err != nil {
		t.Skip("no user cache directory")
	}
	if dir := DefaultCache().dir; filepath.Base(dir) != "momoshtrem-streamlab" {
		t.Errorf("DefaultCache dir = %q", dir)
	}
}
