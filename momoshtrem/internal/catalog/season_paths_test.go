package catalog

import (
	"testing"

	"github.com/shapedtime/momoshtrem/internal/library"
)

// webdav_path follows the tree: a file a job mapped to two episodes is one
// grouped entry; legacy rows keep their per-episode paths.
func TestEpisodePaths(t *testing.T) {
	file := func(source string) *library.TorrentAssignment {
		return &library.TorrentAssignment{InfoHash: "h", FilePath: "Pack/Show.S01E01E02.mkv", MatchSource: source}
	}
	const dir = "/TV Shows/Show (2020)/Season 01/"
	tests := []struct {
		name   string
		source string
		want   map[int64]string
	}{
		{"job rows", "rules", map[int64]string{
			1: dir + "Show - S01E01-E02 - Pilot.mkv",
			2: dir + "Show - S01E01-E02 - Pilot.mkv",
		}},
		{"legacy rows", "", map[int64]string{
			1: dir + "Show - S01E01 - Pilot.mkv",
			2: dir + "Show - S01E02 - Second.mkv",
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EpisodePaths("Show", 2020, 1, []library.Episode{
				{ID: 1, EpisodeNumber: 1, Name: "Pilot", Assignment: file(tt.source)},
				{ID: 2, EpisodeNumber: 2, Name: "Second", Assignment: file(tt.source)},
				{ID: 3, EpisodeNumber: 3, Name: "Third"},
			})
			if len(got) != len(tt.want) {
				t.Fatalf("paths = %v", got)
			}
			for id, want := range tt.want {
				if got[id] != want {
					t.Errorf("episode %d: %q, want %q", id, got[id], want)
				}
			}
		})
	}

	// HDRezka streams are never grouped, like the tree's entries.
	ref := &library.StreamRef{PostID: 646, TranslatorID: 56, Season: 1, Episode: 1, Quality: "1080p"}
	stream := &library.TorrentAssignment{Kind: library.AssignmentKindHDRezka, FilePath: ref.FilePath(), Stream: ref,
		MatchSource: library.SourceManual}
	got := EpisodePaths("Show", 2020, 1, []library.Episode{
		{ID: 1, EpisodeNumber: 1, Name: "Pilot", Assignment: stream},
		{ID: 2, EpisodeNumber: 2, Name: "Second", Assignment: stream},
	})
	if got[1] != dir+"Show - S01E01 - Pilot.mp4" || got[2] != dir+"Show - S01E02 - Second.mp4" {
		t.Errorf("stream paths = %v", got)
	}
}
