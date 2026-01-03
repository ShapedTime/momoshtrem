package streaming

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// bytesReaderAt wraps a byte slice to implement io.ReaderAt
type bytesReaderAt struct {
	data []byte
}

func (r *bytesReaderAt) ReadAt(p []byte, off int64) (n int, err error) {
	if off >= int64(len(r.data)) {
		return 0, nil
	}
	n = copy(p, r.data[off:])
	return n, nil
}

// makeAtom creates an MP4 atom with the given type and size
func makeAtom(atomType string, size uint32) []byte {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint32(buf[:4], size)
	copy(buf[4:8], atomType)
	return buf
}

// makeAtomWithData creates an MP4 atom with data
func makeAtomWithData(atomType string, data []byte) []byte {
	size := uint32(8 + len(data))
	buf := make([]byte, size)
	binary.BigEndian.PutUint32(buf[:4], size)
	copy(buf[4:8], atomType)
	copy(buf[8:], data)
	return buf
}

func TestMP4AnalyzerMoovAtStart(t *testing.T) {
	// Create MP4 with moov at start: ftyp (20 bytes) + moov (100 bytes) + mdat
	var mp4Data bytes.Buffer
	mp4Data.Write(makeAtomWithData("ftyp", make([]byte, 12))) // 20 bytes total
	mp4Data.Write(makeAtomWithData("moov", make([]byte, 92))) // 100 bytes total
	mp4Data.Write(makeAtomWithData("mdat", make([]byte, 1000)))

	reader := &bytesReaderAt{data: mp4Data.Bytes()}
	analyzer := NewMP4Analyzer(reader, int64(mp4Data.Len()))

	info, err := analyzer.Analyze()
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}

	if info.Format != FormatMP4 {
		t.Errorf("Format = %v, want FormatMP4", info.Format)
	}
	if info.MoovOffset != 20 {
		t.Errorf("MoovOffset = %d, want 20", info.MoovOffset)
	}
	if info.MoovSize != 100 {
		t.Errorf("MoovSize = %d, want 100", info.MoovSize)
	}
	if info.NeedsFooter {
		t.Error("NeedsFooter should be false for moov-at-start")
	}
}

func TestMP4AnalyzerMoovAtEnd(t *testing.T) {
	// Create MP4 with moov at end: ftyp + mdat + moov
	var mp4Data bytes.Buffer
	mp4Data.Write(makeAtomWithData("ftyp", make([]byte, 12)))   // 20 bytes
	mp4Data.Write(makeAtomWithData("mdat", make([]byte, 10000))) // Large mdat
	moovOffset := mp4Data.Len()
	mp4Data.Write(makeAtomWithData("moov", make([]byte, 92))) // 100 bytes

	reader := &bytesReaderAt{data: mp4Data.Bytes()}
	analyzer := NewMP4Analyzer(reader, int64(mp4Data.Len()))

	info, err := analyzer.Analyze()
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}

	if info.Format != FormatMP4 {
		t.Errorf("Format = %v, want FormatMP4", info.Format)
	}
	if info.MoovOffset != int64(moovOffset) {
		t.Errorf("MoovOffset = %d, want %d", info.MoovOffset, moovOffset)
	}
	if info.MoovSize != 100 {
		t.Errorf("MoovSize = %d, want 100", info.MoovSize)
	}
	if !info.NeedsFooter {
		t.Error("NeedsFooter should be true for moov-at-end")
	}
}

func TestMP4AnalyzerNotMP4(t *testing.T) {
	// Create non-MP4 data
	data := []byte("This is not an MP4 file, just some random text data")
	reader := &bytesReaderAt{data: data}
	analyzer := NewMP4Analyzer(reader, int64(len(data)))

	_, err := analyzer.Analyze()
	if err != ErrNotMP4 {
		t.Errorf("Analyze() error = %v, want ErrNotMP4", err)
	}
}

func TestMP4AnalyzerWithFreeAtom(t *testing.T) {
	// Create MP4 with free atom: ftyp + free + moov + mdat
	var mp4Data bytes.Buffer
	mp4Data.Write(makeAtomWithData("ftyp", make([]byte, 12)))
	mp4Data.Write(makeAtomWithData("free", make([]byte, 100))) // padding atom
	moovOffset := mp4Data.Len()
	mp4Data.Write(makeAtomWithData("moov", make([]byte, 92)))
	mp4Data.Write(makeAtomWithData("mdat", make([]byte, 1000)))

	reader := &bytesReaderAt{data: mp4Data.Bytes()}
	analyzer := NewMP4Analyzer(reader, int64(mp4Data.Len()))

	info, err := analyzer.Analyze()
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}

	if info.MoovOffset != int64(moovOffset) {
		t.Errorf("MoovOffset = %d, want %d", info.MoovOffset, moovOffset)
	}
}

func TestIsValidMP4Atom(t *testing.T) {
	validAtoms := []string{"ftyp", "moov", "mdat", "free", "skip", "wide", "pnot", "pict"}
	for _, atom := range validAtoms {
		if !isValidMP4Atom(atom) {
			t.Errorf("isValidMP4Atom(%q) = false, want true", atom)
		}
	}

	invalidAtoms := []string{"", "xxxx", "test", "html"}
	for _, atom := range invalidAtoms {
		if isValidMP4Atom(atom) {
			t.Errorf("isValidMP4Atom(%q) = true, want false", atom)
		}
	}
}

func TestMP4AnalyzerEmptyFile(t *testing.T) {
	reader := &bytesReaderAt{data: []byte{}}
	analyzer := NewMP4Analyzer(reader, 0)

	_, err := analyzer.Analyze()
	if err == nil {
		t.Error("Analyze() should fail on empty file")
	}
}
