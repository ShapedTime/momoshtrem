package streaming

import "testing"

func TestBufferedAhead(t *testing.T) {
	const pl = 100 // piece length
	// Pieces 0–1 complete, 2 missing, 3–6 complete, 7 missing.
	runs := []pieceRun{{true, 2}, {false, 1}, {true, 4}, {false, 1}}
	tests := []struct {
		name      string
		runs      []pieceRun
		from, end int64
		want      int64
	}{
		{"start of a complete run", runs, 0, 800, 200},
		{"inside the first piece", runs, 50, 800, 150},
		{"a missing piece", runs, 250, 800, 0},
		{"runs that follow add up", runs, 300, 800, 400},
		{"mid-piece, two runs complete", []pieceRun{{true, 3}, {true, 2}, {false, 1}}, 120, 600, 380},
		{"the file ends inside the run", runs, 300, 520, 220},
		{"everything complete to the file's end", []pieceRun{{true, 8}}, 10, 790, 780},
		{"past the last piece", runs, 900, 1000, 0},
		{"at the end", runs, 800, 800, 0},
		{"no runs (no info yet)", nil, 0, 800, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := bufferedAhead(tt.runs, pl, tt.from, tt.end); got != tt.want {
				t.Errorf("bufferedAhead(%d, %d) = %d, want %d", tt.from, tt.end, got, tt.want)
			}
		})
	}
}
