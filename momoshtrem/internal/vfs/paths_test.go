package vfs

import (
	"reflect"
	"sort"
	"testing"

	"github.com/shapedtime/momoshtrem/internal/library"
)

// Golden paths: WebDAV paths must never change for existing titles (Infuse
// keys its library and watch history on them), so these are literal.
func TestPaths_Golden(t *testing.T) {
	tests := []struct {
		name string
		got  string
		want string
	}{
		{"movie dir", MovieDir("Dune: Part Two", 2024), "/Movies/Dune- Part Two (2024)"},
		{"movie file", MovieFilePath("Dune: Part Two", 2024, "Dune.Part.Two.2024.2160p/Dune.Part.Two.mkv"),
			"/Movies/Dune- Part Two (2024)/Dune- Part Two (2024).mkv"},
		{"movie file mp4", MovieFilePath("Past Lives", 2023, "Past.Lives.2023.1080p.mp4"),
			"/Movies/Past Lives (2023)/Past Lives (2023).mp4"},
		{"movie file without extension", MovieFilePath("Heat", 1995, "Heat/VIDEO"), "/Movies/Heat (1995)/Heat (1995).mkv"},
		{"movie sanitized", MovieDir(`What? <Who> "Me" | Them\Us*`, 2001), "/Movies/What Who 'Me' - Them-Us (2001)"},
		{"show dir", ShowDir("Frieren: Beyond Journey's End", 2023), "/TV Shows/Frieren- Beyond Journey's End (2023)"},
		{"season dir", SeasonDir("Doctor Who", 2005, 3), "/TV Shows/Doctor Who (2005)/Season 03"},
		{"specials dir", SeasonDir("Doctor Who", 2005, 0), "/TV Shows/Doctor Who (2005)/Season 00"},
		{"three-digit season", SeasonDir("One Piece", 1999, 100), "/TV Shows/One Piece (1999)/Season 100"},
		{"episode file", EpisodeFilePath("Doctor Who", 2005, 1, 5, "World War Three (2)", "DW/S01E05.mkv"),
			"/TV Shows/Doctor Who (2005)/Season 01/Doctor Who - S01E05 - World War Three (2).mkv"},
		{"episode unnamed", EpisodeFilePath("Bluey", 2018, 2, 12, "", "Bluey.S02E12.mp4"),
			"/TV Shows/Bluey (2018)/Season 02/Bluey - S02E12 - Episode 12.mp4"},
		{"episode sanitized", EpisodeFilePath("Frieren: Beyond", 2023, 1, 2, "It Didn't Have to Be Magic…/Or?", "x.mkv"),
			"/TV Shows/Frieren- Beyond (2023)/Season 01/Frieren- Beyond - S01E02 - It Didn't Have to Be Magic…-Or.mkv"},
		{"episode three digits", EpisodeFilePath("One Piece", 1999, 22, 1100, "Luffy", "op.mkv"),
			"/TV Shows/One Piece (1999)/Season 22/One Piece - S22E1100 - Luffy.mkv"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("got  %q\nwant %q", tt.got, tt.want)
			}
		})
	}
}

// The exported builders compose the same pieces the tree code used to join
// by hand before they existed.
func TestPaths_MatchTreeComposition(t *testing.T) {
	const title, year, source = "Show: Name?", 2019, "pack/Show.S01E02.1080p.mkv"

	folder := makeMediaFolderName(title, year)
	if got, want := MovieDir(title, year), MoviesPath+"/"+folder; got != want {
		t.Errorf("MovieDir = %q, want %q", got, want)
	}
	if got, want := MovieFilePath(title, year, source), MoviesPath+"/"+folder+"/"+folder+getVideoExt(source); got != want {
		t.Errorf("MovieFilePath = %q, want %q", got, want)
	}
	showPath := TVShowsPath + "/" + folder
	if got := ShowDir(title, year); got != showPath {
		t.Errorf("ShowDir = %q, want %q", got, showPath)
	}
	seasonPath := showPath + "/" + makeSeasonFolderName(1)
	if got := SeasonDir(title, year, 1); got != seasonPath {
		t.Errorf("SeasonDir = %q, want %q", got, seasonPath)
	}
	want := seasonPath + "/" + makeEpisodeFileName(title, 1, 2, "Pilot", getVideoExt(source))
	if got := EpisodeFilePath(title, year, 1, 2, "Pilot", source); got != want {
		t.Errorf("EpisodeFilePath = %q, want %q", got, want)
	}
}

// HDRezka streams are MP4s with made-up paths: a movie keeps its name with
// .mp4, and episodes are never grouped, even two that play one stream.
func TestPaths_HDRezkaStreams(t *testing.T) {
	movie := library.StreamRef{Provider: "hdrezka", PostID: 2259, TranslatorID: 56, Quality: "1080p"}
	if got, want := MovieFilePath("Interstellar", 2014, movie.FilePath()), "/Movies/Interstellar (2014)/Interstellar (2014).mp4"; got != want {
		t.Errorf("movie file = %q, want %q", got, want)
	}

	ep := library.StreamRef{Provider: "hdrezka", PostID: 646, TranslatorID: 1, Season: 1, Episode: 1, Quality: "720p"}
	row := func(id int64) *library.TorrentAssignment {
		// The CHECK forbids a match source on a stream; the tree holds anyway.
		return &library.TorrentAssignment{ID: id, Kind: library.AssignmentKindHDRezka, FilePath: ep.FilePath(), FileSize: 9,
			Stream: &ep, MatchSource: library.SourceManual}
	}
	season := library.Season{SeasonNumber: 1, Episodes: []library.Episode{
		{ID: 11, EpisodeNumber: 1, Name: "Pilot", Assignment: row(1)},
		{ID: 12, EpisodeNumber: 2, Name: "Second", Assignment: row(2)},
	}}
	tree, _, _ := newEmptyTree()
	dir := NewVirtualDir("Season 01")
	addSeasonEntries(tree, dir, "/TV Shows/Show (2020)/Season 01", "Show", season)
	var names []string
	for name := range dir.children {
		names = append(names, name)
	}
	sort.Strings(names)
	if want := []string{"Show - S01E01 - Pilot.mp4", "Show - S01E02 - Second.mp4"}; !reflect.DeepEqual(names, want) {
		t.Errorf("files = %q, want %q", names, want)
	}
}

func TestPublicURL(t *testing.T) {
	tests := []struct {
		base, path, want string
	}{
		{"https://momo.example:36911/", "/TV Shows/Frieren (2023)/",
			"https://momo.example:36911/TV%20Shows/Frieren%20%282023%29/"},
		{"https://momo.example:36911", "/Movies/Dune- Part Two (2024)/Dune- Part Two (2024).mkv",
			"https://momo.example:36911/Movies/Dune-%20Part%20Two%20%282024%29/Dune-%20Part%20Two%20%282024%29.mkv"},
		{"http://localhost:36911/", "/Movies/Amélie #1? 100% (2001)",
			"http://localhost:36911/Movies/Am%C3%A9lie%20%231%3F%20100%25%20%282001%29"},
		{"http://localhost:36911/", "Movies/X (1)", "http://localhost:36911/Movies/X%20%281%29"},
	}
	for _, tt := range tests {
		if got := PublicURL(tt.base, tt.path); got != tt.want {
			t.Errorf("PublicURL(%q, %q)\n got  %q\n want %q", tt.base, tt.path, got, tt.want)
		}
	}
}
