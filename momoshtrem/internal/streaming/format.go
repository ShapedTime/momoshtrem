package streaming

import (
	"io"
	"path/filepath"
	"strings"
)

const (
	// Default header size for unknown formats
	defaultHeaderSize = 10 * 1024 * 1024 // 10MB
)

// DetectFormat analyzes a file to determine its container format.
// It uses file extension hints when available, then falls back to probing.
// Always returns a valid FormatInfo (never nil), using defaults for unknown formats.
func DetectFormat(reader io.ReaderAt, fileSize int64, filename string) *FormatInfo {
	ext := strings.ToLower(filepath.Ext(filename))

	// Try format based on extension hint
	switch ext {
	case ".mp4", ".m4v", ".mov", ".m4a":
		analyzer := NewMP4Analyzer(reader, fileSize)
		if info, err := analyzer.Analyze(); err == nil {
			return info
		}

	case ".mkv", ".webm", ".mka":
		analyzer := NewMKVAnalyzer(reader, fileSize)
		if info, err := analyzer.Analyze(); err == nil {
			return info
		}
	}

	// No extension match or analysis failed - probe both formats
	mp4Analyzer := NewMP4Analyzer(reader, fileSize)
	if info, err := mp4Analyzer.Analyze(); err == nil {
		return info
	}

	mkvAnalyzer := NewMKVAnalyzer(reader, fileSize)
	if info, err := mkvAnalyzer.Analyze(); err == nil {
		return info
	}

	// Unknown format - return conservative defaults
	headerSize := int64(defaultHeaderSize)
	if fileSize < headerSize {
		headerSize = fileSize
	}

	return &FormatInfo{
		Format:      FormatOther,
		HeaderSize:  headerSize,
		NeedsFooter: true, // Assume footer may be important
	}
}

// IsVideoExtension returns true if the file extension indicates a video file.
func IsVideoExtension(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".mp4", ".m4v", ".mov", ".mkv", ".webm", ".avi", ".wmv", ".flv",
		".ts", ".m2ts", ".vob", ".divx", ".3gp", ".ogv":
		return true
	default:
		return false
	}
}
