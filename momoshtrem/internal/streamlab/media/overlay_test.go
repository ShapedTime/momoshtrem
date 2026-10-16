package media

import (
	"encoding/binary"
	"io"
	"testing"

	"github.com/shapedtime/momoshtrem/internal/streaming"
)

type box struct {
	typ       string
	off, size int64
}

// mp4Boxes walks a file's top-level boxes; they must tile it exactly.
func mp4Boxes(t *testing.T, r io.ReaderAt, size int64) []box {
	t.Helper()
	var boxes []box
	for off := int64(0); off < size; {
		var h [16]byte
		if _, err := r.ReadAt(h[:8], off); err != nil {
			t.Fatalf("box header at %d: %v", off, err)
		}
		n := int64(binary.BigEndian.Uint32(h[:4]))
		if n == 1 {
			if _, err := r.ReadAt(h[8:], off+8); err != nil {
				t.Fatal(err)
			}
			n = int64(binary.BigEndian.Uint64(h[8:]))
		}
		if n < 8 || off+n > size {
			t.Fatalf("box %q at %d has size %d in a %d-byte file", h[4:8], off, n, size)
		}
		boxes = append(boxes, box{typ: string(h[4:8]), off: off, size: n})
		off += n
	}
	return boxes
}

func TestMP4_BoxesTileTheFile(t *testing.T) {
	tests := []struct {
		name     string
		size     int64
		moov     int64
		wantMdat int64 // header bytes
	}{
		{"small", 1 << 20, 4096, 8},
		{"default moov", 16 << 20, 0, 8},
		// mdat over 4 GiB takes a 64-bit size.
		{"large mdat", 5 << 30, 1 << 20, 16},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := MustNew(SingleFile("f.mp4", 3, 1<<20, tt.size, MP4))
			if tt.moov != 0 {
				l = MustNew(Spec{Name: "f.mp4", Seed: 3, PieceLength: 1 << 20,
					Files: []FileSpec{{Size: tt.size, Container: MP4, MoovSize: tt.moov}}})
			}
			moov := moovSize(tt.moov)
			got := mp4Boxes(t, l.FileReader(0), tt.size)
			want := []box{
				{"ftyp", 0, 32},
				{"mdat", 32, tt.size - moov - 32},
				{"moov", tt.size - moov, moov},
			}
			if len(got) != len(want) {
				t.Fatalf("boxes = %+v, want %+v", got, want)
			}
			for i := range want {
				if got[i] != want[i] {
					t.Errorf("box %d = %+v, want %+v", i, got[i], want[i])
				}
			}
			var h [4]byte
			if _, err := l.ReadAt(h[:], 32); err != nil {
				t.Fatal(err)
			}
			if large := binary.BigEndian.Uint32(h[:]) == 1; large != (tt.wantMdat == 16) {
				t.Errorf("mdat uses a 64-bit size: %v, want %v", large, tt.wantMdat == 16)
			}
		})
	}
}

// streaming.DetectFormat is what the prioritizer runs on a file. A moov past
// its scan limit (100 MB) is not found, so it falls back to header and
// footer defaults.
func TestDetectFormat(t *testing.T) {
	tests := []struct {
		name string
		spec Spec
		file int
		want streaming.FormatInfo
	}{
		{
			name: "mkv",
			spec: SingleFile("Film.mkv", 1, 1<<20, 64<<20, MKV),
			want: streaming.FormatInfo{Format: streaming.FormatMKV, HeaderSize: 20 << 20, NeedsFooter: true},
		},
		{
			name: "small mkv",
			spec: SingleFile("Film.mkv", 1, 1<<20, 3<<20, MKV),
			want: streaming.FormatInfo{Format: streaming.FormatMKV, HeaderSize: 3 << 20, NeedsFooter: true},
		},
		{
			name: "mp4, moov at the end, within the scan",
			spec: SingleFile("Film.mp4", 1, 1<<20, 64<<20, MP4),
			want: streaming.FormatInfo{
				Format: streaming.FormatMP4, MoovOffset: 64<<20 - DefaultMoovSize, MoovSize: DefaultMoovSize,
				HeaderSize: 10 << 20, NeedsFooter: true,
			},
		},
		{
			name: "mp4, moov past the scan limit",
			spec: SingleFile("Film.mp4", 1, 1<<20, 128<<20, MP4),
			want: streaming.FormatInfo{Format: streaming.FormatMP4, HeaderSize: 10 << 20, NeedsFooter: true},
		},
		{
			name: "mp4, 64-bit mdat",
			spec: SingleFile("Film.mp4", 1, 4<<20, 5<<30, MP4),
			want: streaming.FormatInfo{Format: streaming.FormatMP4, HeaderSize: 10 << 20, NeedsFooter: true},
		},
		{
			name: "second file of a pack",
			spec: EpisodePack("Show", 1, 3, 1, 1<<20, 8<<20, MP4),
			file: 1,
			want: streaming.FormatInfo{
				Format: streaming.FormatMP4, MoovOffset: 8<<20 - DefaultMoovSize, MoovSize: DefaultMoovSize,
				HeaderSize: 10 << 20, NeedsFooter: true,
			},
		},
		{
			name: "raw",
			spec: SingleFile("Film.bin", 1, 1<<20, 64<<20, Raw),
			want: streaming.FormatInfo{Format: streaming.FormatOther, HeaderSize: 10 << 20, NeedsFooter: true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := MustNew(tt.spec)
			f := l.Files()[tt.file]
			got := streaming.DetectFormat(l.FileReader(tt.file), f.Size, f.Path)
			if *got != tt.want {
				t.Errorf("DetectFormat = %+v, want %+v", *got, tt.want)
			}
		})
	}
}

// The MKV header is real EBML: DocType matroska, then a Segment that ends
// with the file.
func TestMKV_EBMLHeader(t *testing.T) {
	const size = 5_000_000
	l := MustNew(SingleFile("Film.mkv", 2, 1<<20, size, MKV))
	h := make([]byte, mkvHeaderLen)
	if _, err := l.ReadAt(h, 0); err != nil {
		t.Fatal(err)
	}
	if got := binary.BigEndian.Uint32(h[:4]); got != 0x1A45DFA3 {
		t.Fatalf("EBML ID = %#x", got)
	}
	if h[4] != 0x80|35 {
		t.Fatalf("EBML size vint = %#x, want 35 bytes", h[4])
	}
	// Children: 2-byte ID, 1-byte size vint, data.
	var docType string
	for p := 5; p < 40; {
		id := binary.BigEndian.Uint16(h[p:])
		n := int(h[p+2] &^ 0x80)
		if id == 0x4282 {
			docType = string(h[p+3 : p+3+n])
		}
		p += 3 + n
	}
	if docType != "matroska" {
		t.Errorf("DocType = %q", docType)
	}
	if got := binary.BigEndian.Uint32(h[40:44]); got != 0x18538067 {
		t.Fatalf("Segment ID = %#x", got)
	}
	if h[44] != 0x01 {
		t.Fatalf("Segment size isn't an 8-byte vint: %#x", h[44])
	}
	segSize := binary.BigEndian.Uint64(h[44:52]) &^ (0x01 << 56)
	if int64(segSize) != size-mkvHeaderLen {
		t.Errorf("Segment size = %d, want %d (to the end of the file)", segSize, size-mkvHeaderLen)
	}
}
