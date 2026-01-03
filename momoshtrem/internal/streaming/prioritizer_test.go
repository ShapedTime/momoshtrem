package streaming

import (
	"testing"
)

func TestByteToPiece(t *testing.T) {
	tests := []struct {
		name        string
		offset      int64
		pieceLength int64
		want        int
	}{
		{"start of first piece", 0, 1024, 0},
		{"middle of first piece", 512, 1024, 0},
		{"end of first piece", 1023, 1024, 0},
		{"start of second piece", 1024, 1024, 1},
		{"large offset", 10*1024*1024 + 500, 1024*1024, 10},
		{"zero piece length", 1000, 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Prioritizer{pieceLength: tt.pieceLength}
			got := p.byteToPiece(tt.offset)
			if got != tt.want {
				t.Errorf("byteToPiece(%d) = %d, want %d", tt.offset, got, tt.want)
			}
		})
	}
}

func TestConfigDefaults(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.HeaderPriorityBytes != 10*1024*1024 {
		t.Errorf("HeaderPriorityBytes = %d, want %d", cfg.HeaderPriorityBytes, 10*1024*1024)
	}
	if cfg.FooterPriorityBytes != 5*1024*1024 {
		t.Errorf("FooterPriorityBytes = %d, want %d", cfg.FooterPriorityBytes, 5*1024*1024)
	}
	if cfg.ReadaheadBytes != 16*1024*1024 {
		t.Errorf("ReadaheadBytes = %d, want %d", cfg.ReadaheadBytes, 16*1024*1024)
	}
	if cfg.UrgentBufferBytes != 2*1024*1024 {
		t.Errorf("UrgentBufferBytes = %d, want %d", cfg.UrgentBufferBytes, 2*1024*1024)
	}
}

func TestConfigIsZero(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		want bool
	}{
		{"zero config", Config{}, true},
		{"default config", DefaultConfig(), false},
		{"partial config", Config{HeaderPriorityBytes: 1}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cfg.IsZero(); got != tt.want {
				t.Errorf("IsZero() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFormatString(t *testing.T) {
	tests := []struct {
		format Format
		want   string
	}{
		{FormatUnknown, "Unknown"},
		{FormatMP4, "MP4"},
		{FormatMKV, "MKV"},
		{FormatOther, "Other"},
		{Format(99), "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.format.String(); got != tt.want {
				t.Errorf("Format(%d).String() = %q, want %q", tt.format, got, tt.want)
			}
		})
	}
}

func TestNilPrioritizerSafety(t *testing.T) {
	var p *Prioritizer

	// All methods should be safe to call on nil
	p.InitialPrioritize()
	p.SetFormatInfo(&FormatInfo{Format: FormatMP4})
	p.UpdateForSeek(1000)

	if p.PieceLength() != 0 {
		t.Error("PieceLength() on nil should return 0")
	}

	begin, end := p.FilePieceRange()
	if begin != 0 || end != 0 {
		t.Error("FilePieceRange() on nil should return 0, 0")
	}
}
