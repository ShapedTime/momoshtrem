package streamtrace

import "testing"

func TestParseRange(t *testing.T) {
	tests := []struct {
		header     string
		start, end int64
	}{
		{"", -1, -1},
		{"bytes=0-499", 0, 499},
		{"bytes=500-", 500, -1},
		{"bytes=-500", -1, 500},
		{"bytes= 100 - 199 ", 100, 199},
		{"bytes=0-0,-1", 0, 0}, // the first of several
		{"bytes=9223372036854775807-", 1<<63 - 1, -1},
		{"bytes=9223372036854775808-", -1, -1}, // overflows
		{"bytes=-", -1, -1},
		{"bytes=abc-", -1, -1},
		{"bytes=1-x", -1, -1},
		{"bytes=12", -1, -1},
		{"items=0-1", -1, -1},
		{"Bytes=0-1", -1, -1}, // as net/http: the unit is matched exactly
	}
	for _, tt := range tests {
		start, end := ParseRange(tt.header)
		if start != tt.start || end != tt.end {
			t.Errorf("ParseRange(%q) = %d, %d; want %d, %d", tt.header, start, end, tt.start, tt.end)
		}
	}
	if allocs := testing.AllocsPerRun(100, func() { ParseRange("bytes=1048576-") }); allocs != 0 {
		t.Errorf("ParseRange allocates: %v", allocs)
	}
}
