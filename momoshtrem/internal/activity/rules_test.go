package activity

import (
	"testing"

	"github.com/shapedtime/momoshtrem/internal/library"
)

func TestStateOf(t *testing.T) {
	tests := []struct {
		name  string
		facts Facts
		want  State
	}{
		{"not loaded", Facts{}, StateIdle},
		{"not loaded, paused", Facts{Paused: true}, StatePaused},
		{"loaded, network off", Facts{Loaded: true}, StateIdle},
		{"loaded, network on, partial", Facts{Loaded: true, Active: true}, StateDownloading},
		{"loaded, network on, complete", Facts{Loaded: true, Active: true, Complete: true}, StateIdle},
		{"a session", Facts{Loaded: true, Active: true, Sessions: 1}, StateStreaming},
		{"sessions on a complete torrent", Facts{Loaded: true, Complete: true, Sessions: 2}, StateStreaming},
		{"paused wins over a session", Facts{Paused: true, Loaded: true, Active: true, Sessions: 1}, StatePaused},
		{"sessions don't count while not loaded", Facts{Sessions: 1}, StateIdle},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := StateOf(tt.facts); got != tt.want {
				t.Errorf("StateOf(%+v) = %s, want %s", tt.facts, got, tt.want)
			}
		})
	}
}

func TestScope(t *testing.T) {
	run := func(season, from, to int) []library.EpisodeNumber {
		var out []library.EpisodeNumber
		for e := from; e <= to; e++ {
			out = append(out, library.EpisodeNumber{Season: season, Episode: e})
		}
		return out
	}
	join := func(runs ...[]library.EpisodeNumber) []library.EpisodeNumber {
		var out []library.EpisodeNumber
		for _, r := range runs {
			out = append(out, r...)
		}
		return out
	}
	tests := []struct {
		name     string
		episodes []library.EpisodeNumber
		sizes    map[int]int
		want     string // "" for nil
	}{
		{"none", nil, nil, ""},
		{"a run in a long season", run(1, 29, 38), map[int]int{1: 52}, "S01E29–E38"},
		{"one episode", run(1, 5, 5), map[int]int{1: 12}, "S01E05"},
		{"a whole season", run(1, 1, 12), map[int]int{1: 12}, "Season 1"},
		{"whole seasons in a row", join(run(1, 1, 10), run(2, 1, 8), run(3, 1, 12)), map[int]int{1: 10, 2: 8, 3: 12}, "Seasons 1–3"},
		{"a run and two more", join(run(1, 1, 5), run(1, 8, 8), run(1, 10, 10)), map[int]int{1: 24}, "S01E01–E05 +2"},
		{"whole seasons with a gap", join(run(1, 1, 12), run(3, 1, 12)), map[int]int{1: 12, 2: 12, 3: 12}, "Season 1 +12"},
		{"whole seasons then a partial one", join(run(1, 1, 12), run(2, 1, 12), run(3, 1, 4)), map[int]int{1: 12, 2: 12, 3: 10}, "Seasons 1–2 +4"},
		{"specials alone", run(0, 1, 3), map[int]int{0: 3}, "Specials"},
		{"some specials", run(0, 1, 2), map[int]int{0: 5}, "S00E01–E02"},
		{"specials go last", join(run(0, 1, 1), run(1, 1, 12)), map[int]int{0: 5, 1: 12}, "Season 1 +1"},
		{"out of order and repeated", join(run(1, 3, 3), run(1, 1, 2), run(1, 2, 2)), map[int]int{1: 20}, "S01E01–E03"},
		{"long seasons keep their digits", run(1, 1124, 1130), map[int]int{1: 1155}, "S01E1124–E1130"},
		{"a season the sizes don't know", run(2, 1, 3), nil, "S02E01–E03"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Scope(tt.episodes, tt.sizes)
			switch {
			case tt.want == "" && got != nil:
				t.Errorf("Scope = %q, want nil", *got)
			case tt.want != "" && (got == nil || *got != tt.want):
				t.Errorf("Scope = %v, want %q", deref(got), tt.want)
			}
		})
	}
}

func deref(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}
