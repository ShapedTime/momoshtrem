package episode

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseInt(t *testing.T) {
	t.Parallel()
	require := require.New(t)

	tests := []struct {
		input    string
		expected int
	}{
		{"123", 123},
		{"01", 1},
		{"0", 0},
		{"", 0},
		{"1", 1},
		{"99", 99},
		{"001", 1},
		{"100", 100},
		// Invalid inputs
		{"abc", 0},
		{"12a3", 0},
		{"a123", 0},
		{"-1", 0},
		{"1.5", 0},
		{" 1", 0},
		{"1 ", 0},
	}

	for _, tc := range tests {
		result := parseInt(tc.input)
		require.Equal(tc.expected, result, "parseInt(%q) should be %d", tc.input, tc.expected)
	}
}

func TestExpandEpisodeRange(t *testing.T) {
	t.Parallel()
	require := require.New(t)

	tests := []struct {
		name     string
		start    int
		end      int
		expected []int
	}{
		{"normal range", 1, 5, []int{1, 2, 3, 4, 5}},
		{"single episode", 1, 1, []int{1}},
		{"two episodes", 1, 2, []int{1, 2}},
		{"high episode numbers", 10, 15, []int{10, 11, 12, 13, 14, 15}},
		// Edge cases - invalid inputs return just start
		{"start > end", 5, 1, []int{5}},
		{"start < 1", 0, 5, []int{0}},
		{"end > 999", 1, 1000, []int{1}},
		{"negative start", -1, 5, []int{-1}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := ExpandEpisodeRange(tc.start, tc.end)
			require.Equal(tc.expected, result)
		})
	}
}

func TestExtractMultiEpisodes(t *testing.T) {
	t.Parallel()
	require := require.New(t)

	tests := []struct {
		input    string
		expected []int
	}{
		{"E01E02E03", []int{1, 2, 3}},
		{"E1E2E3", []int{1, 2, 3}},
		{"E01", []int{1}},
		{"", []int{}},
		{"E99E100", []int{99, 100}},
		// Case insensitivity
		{"e01e02", []int{1, 2}},
		{"E01e02E03", []int{1, 2, 3}},
		// Multi-digit
		{"E001E002E003", []int{1, 2, 3}},
		{"E10E11E12", []int{10, 11, 12}},
		// No match
		{"NoEpisodes", []int{}},
		{"S01", []int{}},
	}

	for _, tc := range tests {
		result := ExtractMultiEpisodes(tc.input)
		require.Equal(tc.expected, result, "ExtractMultiEpisodes(%q)", tc.input)
	}
}

func TestPatternSxxExx(t *testing.T) {
	t.Parallel()
	require := require.New(t)

	patterns := NewCompiledPatterns()

	tests := []struct {
		input    string
		match    bool
		season   int
		episode  int
	}{
		// Standard formats
		{"Show.S01E01.720p.mkv", true, 1, 1},
		{"S01E01", true, 1, 1},
		{"s01e01", true, 1, 1},
		{"S1E1", true, 1, 1},
		{"s1e1", true, 1, 1},
		{"S01E1", true, 1, 1},
		{"S1E01", true, 1, 1},
		// Multi-digit
		{"S12E123", true, 12, 123},
		{"S99E99", true, 99, 99},
		// Embedded in filename
		{"The.Show.S02E15.1080p.BluRay.mkv", true, 2, 15},
		{"show_s03e22_720p.mp4", true, 3, 22},
		{"[Group] Show - S01E05 [1080p].mkv", true, 1, 5},
		// No match
		{"NoMatch.mkv", false, 0, 0},
		{"S01.mkv", false, 0, 0},
		{"E01.mkv", false, 0, 0},
		{"Season01Episode01.mkv", false, 0, 0},
	}

	for _, tc := range tests {
		match := patterns.SxxExx.FindStringSubmatch(tc.input)
		if tc.match {
			require.NotNil(match, "SxxExx should match %q", tc.input)
			require.Equal(tc.season, parseInt(match[1]), "season mismatch for %q", tc.input)
			require.Equal(tc.episode, parseInt(match[2]), "episode mismatch for %q", tc.input)
		} else {
			require.Nil(match, "SxxExx should NOT match %q", tc.input)
		}
	}
}

func TestPatternSxxExxRange(t *testing.T) {
	t.Parallel()
	require := require.New(t)

	patterns := NewCompiledPatterns()

	tests := []struct {
		input    string
		match    bool
		season   int
		startEp  int
		endEp    int
	}{
		// Standard range formats
		{"S01E01-E03", true, 1, 1, 3},
		{"S01E01-03", true, 1, 1, 3},
		{"s01e01-e03", true, 1, 1, 3},
		{"S01E01-E10", true, 1, 1, 10},
		// En-dash
		{"S01E01–E03", true, 1, 1, 3},
		// Embedded in filename
		{"Show.S02E05-E08.1080p.mkv", true, 2, 5, 8},
		// No match (not a range)
		{"S01E01.mkv", false, 0, 0, 0},
		{"S01E01E02", false, 0, 0, 0}, // This is multi, not range
	}

	for _, tc := range tests {
		match := patterns.SxxExxRange.FindStringSubmatch(tc.input)
		if tc.match {
			require.NotNil(match, "SxxExxRange should match %q", tc.input)
			require.Equal(tc.season, parseInt(match[1]), "season mismatch for %q", tc.input)
			require.Equal(tc.startEp, parseInt(match[2]), "start episode mismatch for %q", tc.input)
			require.Equal(tc.endEp, parseInt(match[3]), "end episode mismatch for %q", tc.input)
		} else {
			require.Nil(match, "SxxExxRange should NOT match %q", tc.input)
		}
	}
}

func TestPatternSxxExxMulti(t *testing.T) {
	t.Parallel()
	require := require.New(t)

	patterns := NewCompiledPatterns()

	tests := []struct {
		input       string
		match       bool
		season      int
		episodePart string
	}{
		{"S01E01E02E03", true, 1, "E01E02E03"},
		{"s01e01e02", true, 1, "e01e02"}, // Case preserved from input
		{"S01E01E02E03E04E05", true, 1, "E01E02E03E04E05"},
		// Embedded
		{"Show.S02E10E11.1080p.mkv", true, 2, "E10E11"},
		// Single episode (still matches but ExtractMultiEpisodes handles it)
		{"S01E01", true, 1, "E01"},
	}

	for _, tc := range tests {
		match := patterns.SxxExxMulti.FindStringSubmatch(tc.input)
		if tc.match {
			require.NotNil(match, "SxxExxMulti should match %q", tc.input)
			require.Equal(tc.season, parseInt(match[1]), "season mismatch for %q", tc.input)
			require.Equal(tc.episodePart, match[2], "episode part mismatch for %q", tc.input)
		} else {
			require.Nil(match, "SxxExxMulti should NOT match %q", tc.input)
		}
	}
}

func TestPatternXxYY(t *testing.T) {
	t.Parallel()
	require := require.New(t)

	patterns := NewCompiledPatterns()

	tests := []struct {
		input   string
		match   bool
		season  int
		episode int
	}{
		{"1x01", true, 1, 1},
		{"01x01", true, 1, 1},
		{"1x001", true, 1, 1},
		{"01x12", true, 1, 12},
		{"10x05", true, 10, 5},
		{"Show.1x01.mkv", true, 1, 1},
		{"Show.01x22.720p.mkv", true, 1, 22},
		// No match
		{"1x1", false, 0, 0}, // Episode needs 2+ digits
		{"01x1", false, 0, 0},
		{"S01E01", false, 0, 0},
	}

	for _, tc := range tests {
		match := patterns.XxYY.FindStringSubmatch(tc.input)
		if tc.match {
			require.NotNil(match, "XxYY should match %q", tc.input)
			require.Equal(tc.season, parseInt(match[1]), "season mismatch for %q", tc.input)
			require.Equal(tc.episode, parseInt(match[2]), "episode mismatch for %q", tc.input)
		} else {
			require.Nil(match, "XxYY should NOT match %q", tc.input)
		}
	}
}

func TestPatternSeasonEpisode(t *testing.T) {
	t.Parallel()
	require := require.New(t)

	patterns := NewCompiledPatterns()

	tests := []struct {
		input   string
		match   bool
		season  int
		episode int
	}{
		{"Season 1 Episode 1", true, 1, 1},
		{"Season.1.Episode.1", true, 1, 1},
		{"Season 01 Episode 05", true, 1, 5},
		{"season 2 episode 10", true, 2, 10},
		{"Season1Episode1", true, 1, 1},
		// Embedded
		{"Show Season 3 Episode 15 720p.mkv", true, 3, 15},
		// No match
		{"S01E01", false, 0, 0},
		{"Season 1", false, 0, 0},
		{"Episode 1", false, 0, 0},
	}

	for _, tc := range tests {
		match := patterns.SeasonEpisode.FindStringSubmatch(tc.input)
		if tc.match {
			require.NotNil(match, "SeasonEpisode should match %q", tc.input)
			require.Equal(tc.season, parseInt(match[1]), "season mismatch for %q", tc.input)
			require.Equal(tc.episode, parseInt(match[2]), "episode mismatch for %q", tc.input)
		} else {
			require.Nil(match, "SeasonEpisode should NOT match %q", tc.input)
		}
	}
}

func TestPatternResolution(t *testing.T) {
	t.Parallel()
	require := require.New(t)

	patterns := NewCompiledPatterns()

	tests := []struct {
		input string
		match bool
	}{
		{"2160p", true},
		{"1080p", true},
		{"720p", true},
		{"480p", true},
		{"4K", true},
		{"4k", true},
		{"UHD", true},
		{"Show.1080p.BluRay.mkv", true},
		{"Show.4K.HDR.mkv", true},
		// No match
		{"360p", false},
		{"HD", false},
	}

	for _, tc := range tests {
		match := patterns.Resolution.FindString(tc.input)
		if tc.match {
			require.NotEmpty(match, "Resolution should match %q", tc.input)
		} else {
			require.Empty(match, "Resolution should NOT match %q", tc.input)
		}
	}
}

func TestPatternSource(t *testing.T) {
	t.Parallel()
	require := require.New(t)

	patterns := NewCompiledPatterns()

	tests := []struct {
		input string
		match bool
	}{
		{"BluRay", true},
		{"Blu-Ray", true},
		{"BDRip", true},
		{"WEB-DL", true},
		{"WEBDL", true},
		{"WEB.DL", true},
		{"WEBRip", true},
		{"HDTV", true},
		{"DVDRip", true},
		{"Show.1080p.BluRay.x264.mkv", true},
		{"Show.WEB-DL.1080p.mkv", true},
		// No match
		{"CAM", false},
		{"TS", false},
	}

	for _, tc := range tests {
		match := patterns.Source.FindString(tc.input)
		if tc.match {
			require.NotEmpty(match, "Source should match %q", tc.input)
		} else {
			require.Empty(match, "Source should NOT match %q", tc.input)
		}
	}
}

func TestPatternCodec(t *testing.T) {
	t.Parallel()
	require := require.New(t)

	patterns := NewCompiledPatterns()

	tests := []struct {
		input string
		match bool
	}{
		{"x264", true},
		{"x265", true},
		{"H.264", true},
		{"H264", true},
		{"H.265", true},
		{"H265", true},
		{"HEVC", true},
		{"AV1", true},
		{"AVC", true},
		{"Show.1080p.BluRay.x265.mkv", true},
		{"Show.HEVC.1080p.mkv", true},
		// No match
		{"DivX", false},
		{"XviD", false},
	}

	for _, tc := range tests {
		match := patterns.Codec.FindString(tc.input)
		if tc.match {
			require.NotEmpty(match, "Codec should match %q", tc.input)
		} else {
			require.Empty(match, "Codec should NOT match %q", tc.input)
		}
	}
}

func TestPatternHDR(t *testing.T) {
	t.Parallel()
	require := require.New(t)

	patterns := NewCompiledPatterns()

	tests := []struct {
		input string
		match bool
	}{
		{"HDR", true},
		{"HDR10", true},
		{"HDR10+", true},
		{"Dolby Vision", true},
		{"DolbyVision", true},
		{"Dolby.Vision", true},
		{"DV", true},
		{"DoVi", true},
		{"Show.2160p.BluRay.HDR.mkv", true},
		{"Show.4K.Dolby.Vision.mkv", true},
		// No match
		{"SDR", false},
		{"Show.1080p.mkv", false},
	}

	for _, tc := range tests {
		match := patterns.HDR.MatchString(tc.input)
		if tc.match {
			require.True(match, "HDR should match %q", tc.input)
		} else {
			require.False(match, "HDR should NOT match %q", tc.input)
		}
	}
}

func TestPatternSpecial(t *testing.T) {
	t.Parallel()
	require := require.New(t)

	patterns := NewCompiledPatterns()

	tests := []struct {
		input   string
		match   bool
		episode int // Only for S00Exx format
	}{
		{"S00E01", true, 1},
		{"S00E05", true, 5},
		{"s00e10", true, 10},
		{"Show.Special.mkv", true, 0},
		{"Show.OVA.mkv", true, 0},
		{"Show.OAD.mkv", true, 0},
		{"Show.ONA.mkv", true, 0},
		{"Show - Special - Title.mkv", true, 0},
		{" Special ", true, 0}, // Bounded by spaces
		// No match
		{"S01E01", false, 0},
		{"Show.S01E01.mkv", false, 0},
		{"Specialists.mkv", false, 0}, // "Special" not bounded - partial word
	}

	for _, tc := range tests {
		match := patterns.Special.FindStringSubmatch(tc.input)
		if tc.match {
			require.NotNil(match, "Special should match %q", tc.input)
			if tc.episode > 0 && match[1] != "" {
				require.Equal(tc.episode, parseInt(match[1]), "episode mismatch for %q", tc.input)
			}
		} else {
			require.Nil(match, "Special should NOT match %q", tc.input)
		}
	}
}

func TestPatternSeasonFolder(t *testing.T) {
	t.Parallel()
	require := require.New(t)

	patterns := NewCompiledPatterns()

	tests := []struct {
		input  string
		match  bool
		season int
	}{
		{"Season 01/", true, 1},
		{"Season.01/", true, 1},
		{"Season 1/", true, 1},
		{"S01/", true, 1},
		{"S1/", true, 1},
		{"season 02/", true, 2},
		{"Show/Season 03/episode.mkv", true, 3},
		// Windows paths
		{"Season 01\\", true, 1},
		{"S01\\", true, 1},
		// No match
		{"Episode 01/", false, 0},
		{"Part 01/", false, 0},
	}

	for _, tc := range tests {
		match := patterns.SeasonFolder.FindStringSubmatch(tc.input)
		if tc.match {
			require.NotNil(match, "SeasonFolder should match %q", tc.input)
			require.Equal(tc.season, parseInt(match[1]), "season mismatch for %q", tc.input)
		} else {
			require.Nil(match, "SeasonFolder should NOT match %q", tc.input)
		}
	}
}

func TestPatternEpNumber(t *testing.T) {
	t.Parallel()
	require := require.New(t)

	patterns := NewCompiledPatterns()

	tests := []struct {
		input   string
		match   bool
		episode int
	}{
		{"Ep 1", true, 1},
		{"Ep01", true, 1},
		{"Ep.01", true, 1},
		{"Episode 1", true, 1},
		{"Episode.01", true, 1},
		{"Episode 12", true, 12},
		{"E01 ", true, 1},
		{"-E01-", true, 1},
		{"Show.Episode.05.720p.mkv", true, 5},
		// No match
		{"S01E01", false, 0}, // Handled by SxxExx
		{"Scene01", false, 0},
	}

	for _, tc := range tests {
		match := patterns.EpNumber.FindStringSubmatch(tc.input)
		if tc.match {
			require.NotNil(match, "EpNumber should match %q", tc.input)
			require.Equal(tc.episode, parseInt(match[1]), "episode mismatch for %q", tc.input)
		} else {
			require.Nil(match, "EpNumber should NOT match %q", tc.input)
		}
	}
}

func TestPatternAnimeEpisode(t *testing.T) {
	t.Parallel()
	require := require.New(t)

	patterns := NewCompiledPatterns()

	// The pattern expects: (?:\]|^)\s*-?\s*(\d{2,3})(?:v\d)?(?:\s*[\[\(]|$)
	// This matches ] followed by optional space/dash, then 2-3 digit number
	tests := []struct {
		input   string
		match   bool
		episode int
	}{
		{"] - 01 [", true, 1},
		{"]01[", true, 1},
		{"] 05 [", true, 5},
		{"]- 12 [", true, 12},
		{"]100[", true, 100},
		// With version
		{"] 01v2 [", true, 1},
		// At start of string
		{"01 [1080p]", true, 1},
		{"05[720p]", true, 5},
		// No match - pattern needs ] immediately before or at start
		{"Show - 05", false, 0}, // No ] before and no [ after
		{"S01E01", false, 0},
		{"Episode 01", false, 0},
	}

	for _, tc := range tests {
		match := patterns.AnimeEpisode.FindStringSubmatch(tc.input)
		if tc.match {
			require.NotNil(match, "AnimeEpisode should match %q", tc.input)
			require.Equal(tc.episode, parseInt(match[1]), "episode mismatch for %q", tc.input)
		} else {
			require.Nil(match, "AnimeEpisode should NOT match %q", tc.input)
		}
	}
}

func TestPatternDateYMD(t *testing.T) {
	t.Parallel()
	require := require.New(t)

	patterns := NewCompiledPatterns()

	tests := []struct {
		input string
		match bool
		year  int
		month int
		day   int
	}{
		{"2024.01.15", true, 2024, 1, 15},
		{"2024-01-15", true, 2024, 1, 15},
		{"Show.2024.03.22.720p.mkv", true, 2024, 3, 22},
		{"Daily.Show.2023-12-01.mkv", true, 2023, 12, 1},
		// No match
		{"2024", false, 0, 0, 0},
		{"01.15.2024", false, 0, 0, 0}, // Wrong order
	}

	for _, tc := range tests {
		match := patterns.DateYMD.FindStringSubmatch(tc.input)
		if tc.match {
			require.NotNil(match, "DateYMD should match %q", tc.input)
			require.Equal(tc.year, parseInt(match[1]), "year mismatch for %q", tc.input)
			require.Equal(tc.month, parseInt(match[2]), "month mismatch for %q", tc.input)
			require.Equal(tc.day, parseInt(match[3]), "day mismatch for %q", tc.input)
		} else {
			require.Nil(match, "DateYMD should NOT match %q", tc.input)
		}
	}
}

func TestPatternConcatenated4(t *testing.T) {
	t.Parallel()
	require := require.New(t)

	patterns := NewCompiledPatterns()

	tests := []struct {
		input   string
		match   bool
		season  int
		episode int
	}{
		{".0101.", true, 1, 1},
		{"-0112-", true, 1, 12},
		{"_1201_", true, 12, 1},
		{"Show.0205.720p.mkv", true, 2, 5},
		// No match (not bounded)
		{"12345", false, 0, 0},
		{"Show0101End", false, 0, 0},
	}

	for _, tc := range tests {
		match := patterns.Concatenated4.FindStringSubmatch(tc.input)
		if tc.match {
			require.NotNil(match, "Concatenated4 should match %q", tc.input)
			require.Equal(tc.season, parseInt(match[1]), "season mismatch for %q", tc.input)
			require.Equal(tc.episode, parseInt(match[2]), "episode mismatch for %q", tc.input)
		} else {
			require.Nil(match, "Concatenated4 should NOT match %q", tc.input)
		}
	}
}

func TestPatternConcatenated3(t *testing.T) {
	t.Parallel()
	require := require.New(t)

	patterns := NewCompiledPatterns()

	tests := []struct {
		input   string
		match   bool
		season  int
		episode int
	}{
		{".101.", true, 1, 1},
		{"-112-", true, 1, 12},
		{"_923_", true, 9, 23},
		{"Show.205.720p.mkv", true, 2, 5},
		// No match
		{"1234", false, 0, 0},
		{"Show101End", false, 0, 0},
	}

	for _, tc := range tests {
		match := patterns.Concatenated3.FindStringSubmatch(tc.input)
		if tc.match {
			require.NotNil(match, "Concatenated3 should match %q", tc.input)
			require.Equal(tc.season, parseInt(match[1]), "season mismatch for %q", tc.input)
			require.Equal(tc.episode, parseInt(match[2]), "episode mismatch for %q", tc.input)
		} else {
			require.Nil(match, "Concatenated3 should NOT match %q", tc.input)
		}
	}
}
