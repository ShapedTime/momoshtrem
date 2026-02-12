package streaming

// Format represents detected video container format
type Format int

const (
	FormatUnknown Format = iota
	FormatMP4
	FormatMKV
	FormatOther
)

// String returns a human-readable format name
func (f Format) String() string {
	switch f {
	case FormatMP4:
		return "MP4"
	case FormatMKV:
		return "MKV"
	case FormatOther:
		return "Other"
	default:
		return "Unknown"
	}
}

// FormatInfo contains format-specific priority hints
type FormatInfo struct {
	Format      Format
	MoovOffset  int64 // MP4: offset of moov atom (0 if at start, >0 if at end)
	MoovSize    int64 // MP4: size of moov atom
	HeaderSize  int64 // Recommended header bytes to prioritize
	NeedsFooter bool  // Whether footer contains important metadata
}

// Config holds streaming optimization settings
type Config struct {
	HeaderPriorityBytes int64
	FooterPriorityBytes int64
	ReadaheadBytes      int64
	UrgentBufferBytes   int64
}

// DefaultConfig returns sensible defaults for streaming optimization
func DefaultConfig() Config {
	return Config{
		HeaderPriorityBytes: 10 * 1024 * 1024, // 10MB
		FooterPriorityBytes: 5 * 1024 * 1024,  // 5MB
		ReadaheadBytes:      32 * 1024 * 1024,  // 32MB
		UrgentBufferBytes:   8 * 1024 * 1024,   // 8MB
	}
}

// IsZero returns true if config has no values set
func (c Config) IsZero() bool {
	return c.HeaderPriorityBytes == 0 &&
		c.FooterPriorityBytes == 0 &&
		c.ReadaheadBytes == 0 &&
		c.UrgentBufferBytes == 0
}
