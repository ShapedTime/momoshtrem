package streaming

import (
	"errors"
	"io"
)

// EBML signature bytes (Matroska/WebM identifier)
var ebmlSignature = []byte{0x1A, 0x45, 0xDF, 0xA3}

var ErrNotMKV = errors.New("not an MKV file")

const (
	// Conservative header size for MKV files.
	// MKV SeekHead and Cues elements are typically within first 10-20MB.
	mkvDefaultHeaderSize = 20 * 1024 * 1024 // 20MB
)

// MKVAnalyzer detects MKV/WebM structure for streaming prioritization.
// It verifies the EBML signature and returns conservative header size estimates.
type MKVAnalyzer struct {
	reader   io.ReaderAt
	fileSize int64
}

// NewMKVAnalyzer creates an analyzer for the given reader.
func NewMKVAnalyzer(reader io.ReaderAt, fileSize int64) *MKVAnalyzer {
	return &MKVAnalyzer{
		reader:   reader,
		fileSize: fileSize,
	}
}

// Analyze checks if file is MKV/WebM and returns format info.
// MKV files start with the EBML signature (0x1A 0x45 0xDF 0xA3).
func (a *MKVAnalyzer) Analyze() (*FormatInfo, error) {
	// Read first 4 bytes to check EBML signature
	buf := make([]byte, 4)
	n, err := a.reader.ReadAt(buf, 0)
	if err != nil && err != io.EOF {
		return nil, err
	}
	if n < 4 {
		return nil, ErrNotMKV
	}

	// Verify EBML signature
	for i := 0; i < 4; i++ {
		if buf[i] != ebmlSignature[i] {
			return nil, ErrNotMKV
		}
	}

	// MKV detected - use conservative header estimate
	// Full EBML parsing is complex; 20MB covers most SeekHead/Cues
	headerSize := int64(mkvDefaultHeaderSize)
	if a.fileSize < headerSize {
		headerSize = a.fileSize
	}

	return &FormatInfo{
		Format:      FormatMKV,
		HeaderSize:  headerSize,
		NeedsFooter: true, // Cues element may be at end in some MKVs
	}, nil
}
