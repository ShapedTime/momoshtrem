package main

import "testing"

func TestParseRecommendRunFlags(t *testing.T) {
	tests := []struct {
		args     []string
		wantDry  bool
		wantOK   bool
		wantCode int
	}{
		{nil, false, true, 0},
		{[]string{"-dry-run"}, true, true, 0},
		{[]string{"--dry-run"}, true, true, 0},
		{[]string{"-h"}, false, false, 0},
		{[]string{"-model", "x"}, false, false, 2},
		{[]string{"extra"}, false, false, 2},
	}
	for _, tt := range tests {
		f, code, ok := parseRecommendRunFlags(tt.args)
		if ok != tt.wantOK || code != tt.wantCode || (ok && f.dryRun != tt.wantDry) {
			t.Errorf("parse(%q) = %+v, %d, %v", tt.args, f, code, ok)
		}
	}
}

func TestStoredConnection(t *testing.T) {
	if !storedConnection(true).Connected() || storedConnection(false).Connected() {
		t.Error("storedConnection doesn't say what it holds")
	}
}
