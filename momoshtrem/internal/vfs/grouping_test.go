package vfs

import (
	"reflect"
	"sort"
	"testing"

	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/subtitle"
)

func TestSeasonEntries(t *testing.T) {
	const pack = "Show.S01.1080p/Show.S01E01E02.mkv"
	tests := []struct {
		name   string
		season int
		eps    []SeasonEpisode
		want   []SeasonEntry
	}{
		{
			name:   "a job's multi-episode file is one entry, named after the first",
			season: 1,
			eps: []SeasonEpisode{
				{Number: 2, Name: "Second", InfoHash: "h", FilePath: pack, Grouped: true},
				{Number: 1, Name: "Pilot", InfoHash: "h", FilePath: pack, Grouped: true},
				{Number: 3, Name: "Third", InfoHash: "h", FilePath: "Show.S01.1080p/Show.S01E03.mkv", Grouped: true},
			},
			want: []SeasonEntry{
				{FileName: "Show - S01E01-E02 - Pilot.mkv", Episodes: []int{1, 0}},
				{FileName: "Show - S01E03 - Third.mkv", Episodes: []int{2}},
			},
		},
		{
			name:   "three episodes name the first and the last",
			season: 2,
			eps: []SeasonEpisode{
				{Number: 9, Name: "", InfoHash: "h", FilePath: "p.mp4", Grouped: true},
				{Number: 10, Name: "Ten", InfoHash: "h", FilePath: "p.mp4", Grouped: true},
				{Number: 11, Name: "Eleven", InfoHash: "h", FilePath: "p.mp4", Grouped: true},
			},
			want: []SeasonEntry{{FileName: "Show - S02E09-E11 - Episode 9.mp4", Episodes: []int{0, 1, 2}}},
		},
		{
			name:   "legacy rows (no match_source) keep one entry per episode",
			season: 1,
			eps: []SeasonEpisode{
				{Number: 1, Name: "Pilot", InfoHash: "h", FilePath: pack},
				{Number: 2, Name: "Second", InfoHash: "h", FilePath: pack},
			},
			want: []SeasonEntry{
				{FileName: "Show - S01E01 - Pilot.mkv", Episodes: []int{0}},
				{FileName: "Show - S01E02 - Second.mkv", Episodes: []int{1}},
			},
		},
		{
			name:   "a legacy row never joins a job's group",
			season: 1,
			eps: []SeasonEpisode{
				{Number: 1, Name: "Pilot", InfoHash: "h", FilePath: pack},
				{Number: 2, Name: "Second", InfoHash: "h", FilePath: pack, Grouped: true},
			},
			want: []SeasonEntry{
				{FileName: "Show - S01E01 - Pilot.mkv", Episodes: []int{0}},
				{FileName: "Show - S01E02 - Second.mkv", Episodes: []int{1}},
			},
		},
		{
			name:   "the same path in another torrent is another file",
			season: 1,
			eps: []SeasonEpisode{
				{Number: 1, Name: "Pilot", InfoHash: "a", FilePath: pack, Grouped: true},
				{Number: 2, Name: "Second", InfoHash: "b", FilePath: pack, Grouped: true},
			},
			want: []SeasonEntry{
				{FileName: "Show - S01E01 - Pilot.mkv", Episodes: []int{0}},
				{FileName: "Show - S01E02 - Second.mkv", Episodes: []int{1}},
			},
		},
		{
			name:   "specials group in Season 00 too",
			season: 0,
			eps: []SeasonEpisode{
				{Number: 3, Name: "Recap", InfoHash: "h", FilePath: "sp.mkv", Grouped: true},
				{Number: 4, Name: "Recap 2", InfoHash: "h", FilePath: "sp.mkv", Grouped: true},
			},
			want: []SeasonEntry{{FileName: "Show - S00E03-E04 - Recap.mkv", Episodes: []int{0, 1}}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SeasonEntries("Show", tt.season, tt.eps)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got  %+v\nwant %+v", got, tt.want)
			}
		})
	}
}

// Rows written before assignment jobs (match_source NULL) must keep their
// exact paths: Infuse keys its library and watch history on them.
func TestSeasonEpisodePaths_LegacyByteIdentical(t *testing.T) {
	const title, year = "Frieren: Beyond Journey's End", 2023
	pack := "[SubsPlease] Sousou no Frieren (01-02)/[SubsPlease] Sousou no Frieren - 01-02 (1080p).mkv"
	eps := []SeasonEpisode{
		{Number: 1, Name: "The Journey's End", InfoHash: "h", FilePath: pack},
		{Number: 2, Name: "It Didn't Have to Be Magic...", InfoHash: "h", FilePath: pack},
		{Number: 3, Name: "", InfoHash: "h", FilePath: "x/ep03.mp4"},
	}
	got := SeasonEpisodePaths(title, year, 1, eps)
	want := []string{
		"/TV Shows/Frieren- Beyond Journey's End (2023)/Season 01/Frieren- Beyond Journey's End - S01E01 - The Journey's End.mkv",
		"/TV Shows/Frieren- Beyond Journey's End (2023)/Season 01/Frieren- Beyond Journey's End - S01E02 - It Didn't Have to Be Magic....mkv",
		"/TV Shows/Frieren- Beyond Journey's End (2023)/Season 01/Frieren- Beyond Journey's End - S01E03 - Episode 3.mp4",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
	for i, ep := range eps {
		if old := EpisodeFilePath(title, year, 1, ep.Number, ep.Name, ep.FilePath); got[i] != old {
			t.Errorf("episode %d: %q, before jobs %q", ep.Number, got[i], old)
		}
	}

	grouped := append([]SeasonEpisode(nil), eps...)
	grouped[0].Grouped, grouped[1].Grouped = true, true
	paths := SeasonEpisodePaths(title, year, 1, grouped)
	multi := "/TV Shows/Frieren- Beyond Journey's End (2023)/Season 01/Frieren- Beyond Journey's End - S01E01-E02 - The Journey's End.mkv"
	if paths[0] != multi || paths[1] != multi || paths[2] != want[2] {
		t.Errorf("grouped paths = %q", paths)
	}
}

// A grouped entry is one video with every episode's subtitles named after
// it; legacy rows of the same file stay two videos.
func TestAddSeasonEntries(t *testing.T) {
	const hash, file = "hhhh", "Pack/Show.S01E01E02.mkv"
	row := func(id int64, source string) *library.TorrentAssignment {
		return &library.TorrentAssignment{ID: id, InfoHash: hash, MagnetURI: "magnet:?xt=urn:btih:" + hash,
			FilePath: file, FileSize: 2000, MatchSource: source}
	}
	season := library.Season{SeasonNumber: 1, Episodes: []library.Episode{
		{ID: 11, EpisodeNumber: 1, Name: "Pilot", Assignment: row(1, "rules")},
		{ID: 12, EpisodeNumber: 2, Name: "Second", Assignment: row(2, "rules")},
		{ID: 13, EpisodeNumber: 3, Name: "Third"}, // no file
	}}
	ep11 := subtitle.Item{Type: subtitle.ItemTypeEpisode, ID: 11}
	ep12 := subtitle.Item{Type: subtitle.ItemTypeEpisode, ID: 12}
	fake := &fakeSubtitles{subs: map[subtitle.Item][]*subtitle.Subtitle{
		ep11: {{ID: 1, LanguageCode: "en", Format: "srt", Source: subtitle.SourceTorrent, InfoHash: hash, FilePath: "Pack/en.srt"}},
		ep12: {{ID: 2, LanguageCode: "en", Format: "srt", Source: subtitle.SourceOpenSubtitles, FilePath: "/subs/12/en.srt"}},
	}}
	fs := &LibraryFS{subtitleRepo: fake}

	tree, _, _ := newEmptyTree()
	dir := NewVirtualDir("Season 01")
	const dirPath = "/TV Shows/Show (2020)/Season 01"
	dirs := addSeasonEntries(tree, dir, dirPath, "Show", season)
	subs := fs.loadSubtitles(dirs)
	for _, d := range dirs {
		addSubtitleFiles(tree, d, d.subtitles(subs))
	}

	var names []string
	for name := range dir.children {
		names = append(names, name)
	}
	sort.Strings(names)
	want := []string{
		"Show - S01E01-E02 - Pilot.2.en.srt", // a torrent subtitle after the downloaded one
		"Show - S01E01-E02 - Pilot.en.srt",
		"Show - S01E01-E02 - Pilot.mkv",
	}
	if !reflect.DeepEqual(names, want) {
		t.Errorf("files = %q, want %q", names, want)
	}
	if fake.calls != 1 {
		t.Errorf("subtitle queries = %d, want 1", fake.calls)
	}
	video, ok := tree.pathMap[dirPath+"/Show - S01E01-E02 - Pilot.mkv"].(*PlaceholderFile)
	if !ok || video.assignment.FilePath != file {
		t.Errorf("video = %#v", tree.pathMap[dirPath+"/Show - S01E01-E02 - Pilot.mkv"])
	}

	// The same rows without a match_source: two entries, as before jobs.
	season.Episodes[0].Assignment, season.Episodes[1].Assignment = row(1, ""), row(2, "")
	legacyDir := NewVirtualDir("Season 01")
	addSeasonEntries(tree, legacyDir, dirPath, "Show", season)
	if _, ok := legacyDir.children["Show - S01E01 - Pilot.mkv"]; !ok || len(legacyDir.children) != 2 {
		t.Errorf("legacy files = %v", legacyDir.children)
	}
}
