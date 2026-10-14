package catalog

import "testing"

func TestMovieStage(t *testing.T) {
	tests := []struct {
		name                             string
		inLibrary, assigned, downloading bool
		want                             Stage
	}{
		{"not in library", false, false, false, StageNone},
		{"not in library ignores the rest", false, true, true, StageNone},
		{"no torrent", true, false, false, StageNeedsTorrent},
		{"downloading", true, true, true, StageDownloading},
		{"ready", true, true, false, StageReady},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MovieStage(tt.inLibrary, tt.assigned, tt.downloading); got != tt.want {
				t.Errorf("MovieStage = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestShowStage(t *testing.T) {
	tests := []struct {
		name                                          string
		inLibrary, anyAssigned, frontier, downloading bool
		want                                          Stage
	}{
		{"not in library", false, true, false, false, StageNone},
		{"nothing assigned", true, false, false, false, StageNeedsTorrent},
		{"frontier", true, true, true, false, StageNeedsTorrent},
		{"frontier beats downloading", true, true, true, true, StageNeedsTorrent},
		{"downloading", true, true, false, true, StageDownloading},
		{"ready", true, true, false, false, StageReady},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ShowStage(tt.inLibrary, tt.anyAssigned, tt.frontier, tt.downloading); got != tt.want {
				t.Errorf("ShowStage = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestEpisodeStateOf(t *testing.T) {
	const today = "2026-10-14"
	tests := []struct {
		name                 string
		hasFile, downloading bool
		airDate              string
		want                 EpisodeState
	}{
		{"file", true, false, "2020-01-01", EpisodeReady},
		{"file before it airs", true, false, "2099-01-01", EpisodeReady},
		{"downloading", true, true, "2020-01-01", EpisodeDownloading},
		{"aired without file", false, false, "2020-01-01", EpisodeMissing},
		{"airs today", false, false, today, EpisodeMissing},
		{"airs tomorrow", false, false, "2026-10-15", EpisodeUpcoming},
		{"no air date", false, false, "", EpisodeUpcoming},
		{"downloading without a file is not a thing", false, true, "2020-01-01", EpisodeMissing},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := EpisodeStateOf(tt.hasFile, tt.downloading, tt.airDate, today); got != tt.want {
				t.Errorf("EpisodeStateOf = %s, want %s", got, tt.want)
			}
		})
	}
}
