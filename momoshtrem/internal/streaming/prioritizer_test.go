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
	if cfg.ReadaheadBytes != 64*1024*1024 {
		t.Errorf("ReadaheadBytes = %d, want %d", cfg.ReadaheadBytes, 64*1024*1024)
	}
	if cfg.UrgentBufferBytes != 8*1024*1024 {
		t.Errorf("UrgentBufferBytes = %d, want %d", cfg.UrgentBufferBytes, 8*1024*1024)
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

func TestUpdateForSeekTracksRanges(t *testing.T) {
	// Test that UpdateForSeek updates tracking fields for downgrade logic.
	// We can't use a real torrent.Torrent in unit tests, but we can verify
	// the tracking fields and callback invocations by constructing a
	// Prioritizer with nil torrent (the setPieceRangePriority calls will
	// panic, so we only test the debounce and tracking logic).
	cfg := DefaultConfig()

	var seekCount int
	var downgradeCount int

	p := &Prioritizer{
		cfg:         cfg,
		pieceLength: 1024 * 1024, // 1MB pieces
		fileLength:  500 * 1024 * 1024,
		onSeek: func(_ bool) {
			seekCount++
		},
		onDowngrade: func(count int) {
			downgradeCount += count
		},
	}

	// UpdateForSeek with nil torrent will panic in setPieceRangePriority,
	// so we test the debounce logic and field tracking directly.

	// Test debounce: small movement should be ignored
	p.lastSeekOffset = 100 // simulate having already seeked once
	p.UpdateForSeek(100 + p.pieceLength - 1) // Less than piece length
	if seekCount != 0 {
		t.Errorf("expected debounce to skip seek callback, got %d calls", seekCount)
	}

	// Verify tracking fields are zero-valued initially (no setPieceRangePriority called)
	if p.lastUrgentStart != 0 || p.lastUrgentEnd != 0 || p.lastReadaheadEnd != 0 {
		// These should still be 0 because the debounce skipped the update
		t.Error("tracking fields should be zero after debounced seek")
	}
}

func TestPriorityCallbacksStruct(t *testing.T) {
	// Verify PriorityCallbacks can be constructed with both callbacks
	var seekCalled bool
	var downgradeCalled bool

	callbacks := &PriorityCallbacks{
		OnSeek: func(forward bool) {
			seekCalled = true
		},
		OnDowngrade: func(count int) {
			downgradeCalled = true
		},
	}

	callbacks.OnSeek(true)
	callbacks.OnDowngrade(5)

	if !seekCalled {
		t.Error("OnSeek callback not called")
	}
	if !downgradeCalled {
		t.Error("OnDowngrade callback not called")
	}
}
