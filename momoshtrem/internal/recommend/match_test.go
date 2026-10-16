package recommend

import "testing"

func TestSameYear(t *testing.T) {
	for _, tt := range []struct {
		given, actual int
		want          bool
	}{{2024, 2024, true}, {2023, 2024, true}, {2025, 2024, true}, {2022, 2024, false}, {0, 2024, true}, {2024, 0, true}} {
		if got := SameYear(tt.given, tt.actual); got != tt.want {
			t.Errorf("SameYear(%d, %d) = %v", tt.given, tt.actual, got)
		}
	}
}

func TestSameTitle(t *testing.T) {
	for _, tt := range []struct {
		given string
		names []string
		want  bool
	}{
		{"Amélie", []string{"Amelie"}, true},
		{"The Office", []string{"The Office (US)"}, true},
		{"Frieren", []string{"Frieren: Beyond Journey's End", "葬送のフリーレン"}, true},
		{"葬送のフリーレン", []string{"Frieren: Beyond Journey's End", "葬送のフリーレン"}, true},
		{"Law & Order", []string{"Law and Order"}, true},
		{"Dark", []string{"Darkness"}, false},
		{"It", []string{"It Follows"}, false},
		{"Severance", []string{"Silo"}, false},
		// #160's live run: ids remembered for the wrong show
		{"Upload", []string{"The Mandalorian"}, false},
		{"Russian Doll", []string{"How It Really Happened"}, false},
		{"", []string{"Silo"}, false},
	} {
		if got := SameTitle(tt.given, tt.names...); got != tt.want {
			t.Errorf("SameTitle(%q, %q) = %v", tt.given, tt.names, got)
		}
	}
}
