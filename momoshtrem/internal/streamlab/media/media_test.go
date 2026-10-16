package media

import (
	"bytes"
	"errors"
	"io"
	"math/rand/v2"
	"strings"
	"testing"
)

const testPiece = 64 << 10

// mixedPack is a multi-file layout with every container, odd sizes and
// headers that straddle reads.
func mixedPack(t *testing.T, seed uint64) *Layout {
	t.Helper()
	l, err := New(Spec{
		Name:        "Mixed",
		Seed:        seed,
		PieceLength: testPiece,
		Files: []FileSpec{
			{Path: "a.mkv", Size: 300_001, Container: MKV},
			{Path: "b/c.mp4", Size: 200_003, Container: MP4, MoovSize: 4096},
			{Path: "d.bin", Size: 77},
			{Path: "e.mp4", Size: 150_000, Container: MP4, MoovSize: 1000},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func readAll(t *testing.T, l *Layout) []byte {
	t.Helper()
	buf := make([]byte, l.Size())
	if n, err := l.ReadAt(buf, 0); n != len(buf) || err != nil {
		t.Fatalf("ReadAt(all) = %d, %v", n, err)
	}
	return buf
}

func TestReadAt_Deterministic(t *testing.T) {
	a := readAll(t, mixedPack(t, 7))
	b := readAll(t, mixedPack(t, 7))
	if !bytes.Equal(a, b) {
		t.Fatal("two layouts of one spec differ")
	}
	c := readAll(t, mixedPack(t, 8))
	same := 0
	for i := range a {
		if a[i] == c[i] {
			same++
		}
	}
	// Headers are shared; random bytes agree about 1 time in 256.
	if same > len(a)/100 {
		t.Errorf("seeds 7 and 8 share %d of %d bytes", same, len(a))
	}
}

// Any read at any offset, aligned or not, returns what one big read does,
// container headers included.
func TestReadAt_RandomAccessMatchesSequential(t *testing.T) {
	l := mixedPack(t, 42)
	all := readAll(t, l)
	rng := rand.New(rand.NewPCG(1, 2))
	for i := 0; i < 5000; i++ {
		off := rng.Int64N(l.Size())
		n := 1 + rng.IntN(70_000)
		if i%10 == 0 {
			n = 1 + rng.IntN(9) // word-straddling reads
		}
		buf := make([]byte, n)
		got, err := l.ReadAt(buf, off)
		want := min(int64(n), l.Size()-off)
		if int64(got) != want {
			t.Fatalf("ReadAt(%d bytes at %d) = %d, want %d", n, off, got, want)
		}
		if (int64(n) > want) != errors.Is(err, io.EOF) {
			t.Fatalf("ReadAt(%d bytes at %d) err = %v", n, off, err)
		}
		if !bytes.Equal(buf[:got], all[off:off+int64(got)]) {
			t.Fatalf("ReadAt(%d bytes at %d) differs from the sequential read", n, off)
		}
	}
}

func TestReadAt_Bounds(t *testing.T) {
	l := mixedPack(t, 1)
	if n, err := l.ReadAt(make([]byte, 10), l.Size()); n != 0 || err != io.EOF {
		t.Errorf("ReadAt at the end = %d, %v; want 0, EOF", n, err)
	}
	if _, err := l.ReadAt(make([]byte, 10), -1); err == nil {
		t.Error("ReadAt at -1 succeeded")
	}
	if n, err := l.ReadAt(nil, 5); n != 0 || err != nil {
		t.Errorf("empty ReadAt = %d, %v", n, err)
	}
}

func TestReadAt_IsSplitmix64(t *testing.T) {
	// splitmix64 seeded with 0: its first output, little-endian.
	l := MustNew(SingleFile("x", 0, testPiece, 8, Raw))
	got := make([]byte, 8)
	if _, err := l.ReadAt(got, 0); err != nil {
		t.Fatal(err)
	}
	want := []byte{0xaf, 0xcd, 0x1d, 0x7b, 0x39, 0xa8, 0x20, 0xe2} // 0xe220a8397b1dcdaf
	if !bytes.Equal(got, want) {
		t.Errorf("first word = % x, want % x", got, want)
	}
}

func TestLayout_Files(t *testing.T) {
	l := MustNew(EpisodePack("Show", 1, 3, 9, testPiece, 100_000, MKV))
	files := l.Files()
	wantPaths := []string{"Show.S01/Show.S01E01.mkv", "Show.S01/Show.S01E02.mkv", "Show.S01/Show.S01E03.mkv"}
	var off int64
	for i, f := range files {
		if f.Path != wantPaths[i] || f.Offset != off || f.Size != 100_000 || f.Container != MKV {
			t.Errorf("file %d = %+v", i, f)
		}
		off += f.Size
		// The file reader is the layout at the file's offset.
		got := make([]byte, 1000)
		if _, err := l.FileReader(i).ReadAt(got, 99_500); err != io.EOF {
			t.Errorf("file %d: reading past its end: err = %v, want EOF", i, err)
		}
		want := make([]byte, 500)
		if _, err := l.ReadAt(want, f.Offset+99_500); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got[:500], want) {
			t.Errorf("file %d: FileReader differs from ReadAt", i)
		}
	}
	if l.Size() != off || l.NumPieces() != int((off+testPiece-1)/testPiece) {
		t.Errorf("size %d, %d pieces", l.Size(), l.NumPieces())
	}

	single := MustNew(SingleFile("Film.2020.mkv", 1, testPiece, 1000, MKV))
	if f := single.Files(); len(f) != 1 || f[0].Path != "Film.2020.mkv" {
		t.Errorf("single-file layout's files = %+v", f)
	}
}

func TestNew_Validates(t *testing.T) {
	ok := FileSpec{Path: "a.mkv", Size: 1000, Container: MKV}
	tests := []struct {
		name string
		spec Spec
		want string
	}{
		{"no name", Spec{PieceLength: testPiece, Files: []FileSpec{ok}}, "name"},
		{"slash in name", Spec{Name: "a/b", PieceLength: testPiece, Files: []FileSpec{ok}}, "name"},
		{"piece too small", Spec{Name: "x", PieceLength: 8 << 10, Files: []FileSpec{ok}}, "piece length"},
		{"piece not a power of two", Spec{Name: "x", PieceLength: 48 << 10, Files: []FileSpec{ok}}, "piece length"},
		{"no files", Spec{Name: "x", PieceLength: testPiece}, "no files"},
		{"empty path in a pack", Spec{Name: "x", PieceLength: testPiece, Files: []FileSpec{ok, {Size: 1}}}, "path"},
		{"unclean path", Spec{Name: "x", PieceLength: testPiece, Files: []FileSpec{{Path: "a//b", Size: 1}}}, "path"},
		{"escaping path", Spec{Name: "x", PieceLength: testPiece, Files: []FileSpec{{Path: "../b", Size: 1}}}, "path"},
		{"duplicate path", Spec{Name: "x", PieceLength: testPiece, Files: []FileSpec{ok, ok}}, "duplicate"},
		{"empty file", Spec{Name: "x", PieceLength: testPiece, Files: []FileSpec{{Path: "a", Size: 0}}}, "size"},
		{"tiny mkv", Spec{Name: "x", PieceLength: testPiece, Files: []FileSpec{{Path: "a", Size: 10, Container: MKV}}}, "MKV"},
		{"tiny mp4", Spec{Name: "x", PieceLength: testPiece, Files: []FileSpec{{Path: "a", Size: 1000, Container: MP4}}}, "MP4"},
		{"moov on mkv", Spec{Name: "x", PieceLength: testPiece, Files: []FileSpec{{Path: "a", Size: 1000, Container: MKV, MoovSize: 100}}}, "moov"},
		{"unknown container", Spec{Name: "x", PieceLength: testPiece, Files: []FileSpec{{Path: "a", Size: 1000, Container: "avi"}}}, "container"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New(tt.spec)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %v, want one about %q", err, tt.want)
			}
		})
	}
}

func TestLayout_SpecIsACopy(t *testing.T) {
	spec := EpisodePack("Show", 1, 2, 1, testPiece, 1000, Raw)
	l := MustNew(spec)
	spec.Files[0].Size = 5
	got := l.Spec()
	got.Files[1].Size = 6
	if l.Files()[0].Size != 1000 || l.Spec().Files[1].Size != 1000 {
		t.Error("the layout shares its spec's files with callers")
	}
}

func BenchmarkReadAt(b *testing.B) {
	l := MustNew(SingleFile("x", 1, 1<<20, 1<<30, MKV))
	buf := make([]byte, 32<<10)
	b.SetBytes(int64(len(buf)))
	for i := 0; b.Loop(); i++ {
		_, _ = l.ReadAt(buf, int64(i%32768)*int64(len(buf)))
	}
}
