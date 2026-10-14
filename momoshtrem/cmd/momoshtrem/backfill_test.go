package main

import (
	"testing"

	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/metadata"
)

func TestParseBackfillFlags(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantOK   bool
		wantCode int
		want     backfillFlags
	}{
		{"defaults", nil, true, 0, backfillFlags{rps: 10}},
		{
			"everything",
			[]string{"--dry-run", "--force", "--only=shows", "--limit=25", "--rps=4"},
			true, 0,
			backfillFlags{opts: metadata.BackfillOptions{DryRun: true, Force: true, Only: library.MediaShow, Limit: 25}, rps: 4},
		},
		{"only movies, single dash", []string{"-only", "movies"}, true, 0, backfillFlags{opts: metadata.BackfillOptions{Only: library.MediaMovie}, rps: 10}},
		{"fractional rps", []string{"--rps=2.5"}, true, 0, backfillFlags{rps: 2.5}},
		{"help", []string{"-h"}, false, 0, backfillFlags{}},
		{"unknown flag", []string{"--everything"}, false, 2, backfillFlags{}},
		{"bad --only", []string{"--only=films"}, false, 2, backfillFlags{}},
		{"negative limit", []string{"--limit=-1"}, false, 2, backfillFlags{}},
		{"zero rps", []string{"--rps=0"}, false, 2, backfillFlags{}},
		{"stray argument", []string{"--dry-run", "now"}, false, 2, backfillFlags{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, code, ok := parseBackfillFlags(tt.args)
			if ok != tt.wantOK || code != tt.wantCode {
				t.Fatalf("ok %v code %d, want ok %v code %d", ok, code, tt.wantOK, tt.wantCode)
			}
			if ok && got != tt.want {
				t.Errorf("flags = %+v, want %+v", got, tt.want)
			}
		})
	}
}
