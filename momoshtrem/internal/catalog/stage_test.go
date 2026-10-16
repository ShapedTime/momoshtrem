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
		name                                       string
		watched, hasFile, downloading, underReview bool
		airDate                                    string
		want                                       EpisodeState
	}{
		{"file", false, true, false, false, "2020-01-01", EpisodeReady},
		{"file before it airs", false, true, false, false, "2099-01-01", EpisodeReady},
		{"downloading", false, true, true, false, "2020-01-01", EpisodeDownloading},
		{"aired without file", false, false, false, false, "2020-01-01", EpisodeMissing},
		{"airs today", false, false, false, false, today, EpisodeMissing},
		{"airs tomorrow", false, false, false, false, "2026-10-15", EpisodeUpcoming},
		{"no air date", false, false, false, false, "", EpisodeUpcoming},
		{"downloading without a file is not a thing", false, false, true, false, "2020-01-01", EpisodeMissing},
		{"under review, aired", false, false, false, true, "2020-01-01", EpisodeNeedsReview},
		{"under review, not aired yet", false, false, false, true, "2099-01-01", EpisodeNeedsReview},
		{"a file wins over review", false, true, false, true, "2020-01-01", EpisodeReady},
		{"downloading wins over review", false, true, true, true, "2020-01-01", EpisodeDownloading},
		// A play wins over everything (B5).
		{"watched without a file", true, false, false, false, "2020-01-01", EpisodeWatched},
		{"watched with a file", true, true, false, false, "2020-01-01", EpisodeWatched},
		{"watched while downloading", true, true, true, false, "2020-01-01", EpisodeWatched},
		{"watched under review", true, false, false, true, "2020-01-01", EpisodeWatched},
		{"watched before TMDB says it aired", true, false, false, false, "2099-01-01", EpisodeWatched},
		{"watched without an air date", true, false, false, false, "", EpisodeWatched},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := EpisodeStateOf(tt.watched, tt.hasFile, tt.downloading, tt.underReview, tt.airDate, today); got != tt.want {
				t.Errorf("EpisodeStateOf = %s, want %s", got, tt.want)
			}
		})
	}
}
