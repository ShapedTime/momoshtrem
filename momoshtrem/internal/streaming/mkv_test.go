package streaming

import (
	"testing"
)

func TestMKVAnalyzerValidMKV(t *testing.T) {
	// Create valid MKV data with EBML signature
	data := make([]byte, 1000)
	copy(data[:4], ebmlSignature)

	reader := &bytesReaderAt{data: data}
	analyzer := NewMKVAnalyzer(reader, int64(len(data)))

	info, err := analyzer.Analyze()
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}

	if info.Format != FormatMKV {
		t.Errorf("Format = %v, want FormatMKV", info.Format)
	}
	// File is smaller than default header size, so HeaderSize should be file size
	if info.HeaderSize != int64(len(data)) {
		t.Errorf("HeaderSize = %d, want %d", info.HeaderSize, len(data))
	}
	if !info.NeedsFooter {
		t.Error("NeedsFooter should be true for MKV")
	}
}

func TestMKVAnalyzerLargeFile(t *testing.T) {
	// Simulate large MKV file
	data := make([]byte, 100)
	copy(data[:4], ebmlSignature)

	reader := &bytesReaderAt{data: data}
	// Pretend file is 1GB
	analyzer := NewMKVAnalyzer(reader, 1024*1024*1024)

	info, err := analyzer.Analyze()
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}

	// Should use default header size (20MB)
	if info.HeaderSize != mkvDefaultHeaderSize {
		t.Errorf("HeaderSize = %d, want %d", info.HeaderSize, mkvDefaultHeaderSize)
	}
}

func TestMKVAnalyzerNotMKV(t *testing.T) {
	// Create non-MKV data
	data := []byte("This is not an MKV file")
	reader := &bytesReaderAt{data: data}
	analyzer := NewMKVAnalyzer(reader, int64(len(data)))

	_, err := analyzer.Analyze()
	if err != ErrNotMKV {
		t.Errorf("Analyze() error = %v, want ErrNotMKV", err)
	}
}

func TestMKVAnalyzerMP4File(t *testing.T) {
	// Create MP4-like data (starts with ftyp)
	data := make([]byte, 100)
	copy(data[4:8], "ftyp")

	reader := &bytesReaderAt{data: data}
	analyzer := NewMKVAnalyzer(reader, int64(len(data)))

	_, err := analyzer.Analyze()
	if err != ErrNotMKV {
		t.Errorf("Analyze() error = %v, want ErrNotMKV", err)
	}
}

func TestMKVAnalyzerShortFile(t *testing.T) {
	// File too short to have EBML signature
	data := []byte{0x1A, 0x45}
	reader := &bytesReaderAt{data: data}
	analyzer := NewMKVAnalyzer(reader, int64(len(data)))

	_, err := analyzer.Analyze()
	if err != ErrNotMKV {
		t.Errorf("Analyze() error = %v, want ErrNotMKV", err)
	}
}

func TestMKVAnalyzerEmptyFile(t *testing.T) {
	reader := &bytesReaderAt{data: []byte{}}
	analyzer := NewMKVAnalyzer(reader, 0)

	_, err := analyzer.Analyze()
	if err != ErrNotMKV {
		t.Errorf("Analyze() error = %v, want ErrNotMKV", err)
	}
}

func TestMKVAnalyzerPartialSignature(t *testing.T) {
	// Has first 3 bytes of signature but not 4th
	data := []byte{0x1A, 0x45, 0xDF, 0x00}
	reader := &bytesReaderAt{data: data}
	analyzer := NewMKVAnalyzer(reader, int64(len(data)))

	_, err := analyzer.Analyze()
	if err != ErrNotMKV {
		t.Errorf("Analyze() error = %v, want ErrNotMKV", err)
	}
}
