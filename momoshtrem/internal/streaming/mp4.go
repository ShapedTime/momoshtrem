package streaming

import (
	"encoding/binary"
	"errors"
	"io"
)

const (
	mp4AtomHeaderSize = 8
	mp4MaxScanBytes   = 100 * 1024 * 1024 // 100MB max scan
)

var (
	ErrNotMP4       = errors.New("not an MP4 file")
	ErrMoovNotFound = errors.New("moov atom not found")
)

// MP4Analyzer detects MP4 structure for streaming prioritization.
// It finds the moov atom which contains metadata needed for playback.
type MP4Analyzer struct {
	reader   io.ReaderAt
	fileSize int64
}

// NewMP4Analyzer creates an analyzer for the given reader.
func NewMP4Analyzer(reader io.ReaderAt, fileSize int64) *MP4Analyzer {
	return &MP4Analyzer{
		reader:   reader,
		fileSize: fileSize,
	}
}

// Analyze checks if file is MP4 and finds moov location.
// Returns FormatInfo with moov offset/size if found.
func (a *MP4Analyzer) Analyze() (*FormatInfo, error) {
	// Check for valid MP4 structure at start
	buf := make([]byte, mp4AtomHeaderSize)
	if _, err := a.reader.ReadAt(buf, 0); err != nil {
		return nil, err
	}

	atomType := string(buf[4:8])
	if !isValidMP4Atom(atomType) {
		return nil, ErrNotMP4
	}

	// Scan entire file for moov (up to max scan limit)
	scanLimit := min(mp4MaxScanBytes, a.fileSize)
	offset, size, err := a.findAtom("moov", 0, scanLimit)
	if err == nil {
		// Determine if moov is at "start" or "end" of file
		// moov is considered "at start" if:
		// 1. For small files (< 50MB): moov ends before 75% of file length
		// 2. For large files: moov is within first 20MB
		const headerThreshold = 20 * 1024 * 1024  // 20MB
		const smallFileThreshold = 50 * 1024 * 1024 // 50MB
		moovEnd := offset + size

		var isAtStart bool
		if a.fileSize < smallFileThreshold {
			// Small file: use percentage (moov in first 75%)
			isAtStart = moovEnd <= a.fileSize*3/4
		} else {
			// Large file: use absolute threshold
			isAtStart = moovEnd <= headerThreshold
		}

		if isAtStart {
			// Fast-start MP4: moov near beginning
			return &FormatInfo{
				Format:      FormatMP4,
				MoovOffset:  offset,
				MoovSize:    size,
				HeaderSize:  moovEnd, // Prioritize up to end of moov
				NeedsFooter: false,
			}, nil
		}

		// moov is at end of file
		return &FormatInfo{
			Format:      FormatMP4,
			MoovOffset:  offset,
			MoovSize:    size,
			HeaderSize:  10 * 1024 * 1024, // Conservative header for ftyp/mdat start
			NeedsFooter: true,
		}, nil
	}

	// MP4 without moov (unusual but possible) - use defaults
	return &FormatInfo{
		Format:      FormatMP4,
		HeaderSize:  10 * 1024 * 1024,
		NeedsFooter: true,
	}, nil
}

// findAtom searches for an atom by type within a byte range.
// Returns the atom's offset and size if found.
func (a *MP4Analyzer) findAtom(targetType string, start, end int64) (offset, size int64, err error) {
	buf := make([]byte, 16) // For extended size atoms
	pos := start

	for pos < end {
		// Read atom header (8 bytes minimum)
		n, err := a.reader.ReadAt(buf[:8], pos)
		if err != nil && err != io.EOF {
			return 0, 0, err
		}
		if n < 8 {
			break
		}

		atomSize := int64(binary.BigEndian.Uint32(buf[:4]))
		atomType := string(buf[4:8])

		// Handle extended size (size=1 means 64-bit size follows)
		if atomSize == 1 {
			n, err := a.reader.ReadAt(buf[8:16], pos+8)
			if err != nil && err != io.EOF {
				return 0, 0, err
			}
			if n < 8 {
				break
			}
			atomSize = int64(binary.BigEndian.Uint64(buf[8:16]))
		}

		// size=0 means atom extends to end of file
		if atomSize == 0 {
			atomSize = end - pos
		}

		// Check if this is our target
		if atomType == targetType {
			return pos, atomSize, nil
		}

		// Invalid atom size
		if atomSize < 8 {
			break
		}

		pos += atomSize
	}

	return 0, 0, ErrMoovNotFound
}

// isValidMP4Atom checks if the atom type is a valid MP4 top-level atom.
func isValidMP4Atom(atomType string) bool {
	switch atomType {
	case "ftyp", "moov", "mdat", "free", "skip", "wide", "pnot", "pict":
		return true
	default:
		return false
	}
}
