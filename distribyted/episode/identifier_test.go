package episode

import (
	"testing"

	"github.com/distribyted/distribyted/torrent/loader"
	"github.com/stretchr/testify/require"
)

// ============================================================
// File Type Helper Tests
// ============================================================

func TestIsVideoFile(t *testing.T) {
	t.Parallel()
	require := require.New(t)

	tests := []struct {
		path     string
		expected bool
	}{
		// Video files
		{"video.mkv", true},
		{"video.mp4", true},
		{"video.avi", true},
		{"video.wmv", true},
		{"video.mov", true},
		{"video.m4v", true},
		{"video.webm", true},
		{"video.ts", true},
		{"video.m2ts", true},
		{"video.vob", true},
		{"video.flv", true},
		{"video.divx", true},
		// Case insensitivity
		{"video.MKV", true},
		{"video.MP4", true},
		// With path
		{"Show/Season 01/video.mkv", true},
		// Not video files
		{"subtitle.srt", false},
		{"readme.txt", false},
		{"info.nfo", false},
		{"image.jpg", false},
		{"video", false}, // No extension
	}

	for _, tc := range tests {
		result := isVideoFile(tc.path)
		require.Equal(tc.expected, result, "isVideoFile(%q)", tc.path)
	}
}

func TestIsSubtitleFile(t *testing.T) {
	t.Parallel()
	require := require.New(t)

	tests := []struct {
		path     string
		expected bool
	}{
		// Subtitle files
		{"subtitle.srt", true},
		{"subtitle.sub", true},
		{"subtitle.ass", true},
		{"subtitle.ssa", true},
		{"subtitle.vtt", true},
		{"subtitle.idx", true},
		{"subtitle.smi", true},
		// Case insensitivity
		{"subtitle.SRT", true},
		{"subtitle.ASS", true},
		// With path
		{"Show/Season 01/subtitle.srt", true},
		// Not subtitle files
		{"video.mkv", false},
		{"readme.txt", false},
		{"video.mp4", false},
	}

	for _, tc := range tests {
		result := isSubtitleFile(tc.path)
		require.Equal(tc.expected, result, "isSubtitleFile(%q)", tc.path)
	}
}

func TestShouldSkip(t *testing.T) {
	t.Parallel()
	require := require.New(t)

	tests := []struct {
		path     string
		expected bool
	}{
		// Should skip
		{"sample.mkv", true},
		{"Sample.mkv", true},
		{"SAMPLE.mkv", true},
		{"video-sample.mkv", true},
		{"Sample/video.mkv", true},
		{"trailer.mp4", true},
		{"Trailer.mp4", true},
		{"preview.mkv", true},
		{"extras/feature.mkv", true},
		{"Extras/feature.mkv", true},
		{"extras\\feature.mkv", true},
		{"featurette.mkv", true},
		{"deleted.scene.mkv", true},
		{"deleted_scene.mkv", true},
		{"deleted-scene.mkv", true},
		{"behind.the.scene.mkv", true},
		{"behind_the_scene.mkv", true},
		{"behind-the-scene.mkv", true},
		{"bonus/video.mkv", true},
		{"bonus\\video.mkv", true},
		{"/extra/video.mkv", true},
		{"\\extra\\video.mkv", true},
		// Should NOT skip
		{"Show.S01E01.mkv", false},
		{"episode.mkv", false},
		{"Show.S01E01.1080p.mkv", false},
		{"extraterrestrial.mkv", false},
		{"samplecase.mkv", true}, // Contains "sample"
	}

	for _, tc := range tests {
		result := shouldSkip(tc.path)
		require.Equal(tc.expected, result, "shouldSkip(%q)", tc.path)
	}
}

// ============================================================
// Normalization Function Tests
// ============================================================

func TestNormalizeResolution(t *testing.T) {
	t.Parallel()
	require := require.New(t)

	tests := []struct {
		input    string
		expected string
	}{
		// 2160p variants
		{"2160p", "2160p"},
		{"2160P", "2160p"},
		{"4K", "2160p"},
		{"4k", "2160p"},
		{"UHD", "2160p"},
		{"uhd", "2160p"},
		// 1080p variants
		{"1080p", "1080p"},
		{"1080P", "1080p"},
		// 720p variants
		{"720p", "720p"},
		{"720P", "720p"},
		// 480p variants
		{"480p", "480p"},
		{"480P", "480p"},
		// Unknown (passthrough)
		{"360p", "360p"},
		{"HD", "HD"},
	}

	for _, tc := range tests {
		result := normalizeResolution(tc.input)
		require.Equal(tc.expected, result, "normalizeResolution(%q)", tc.input)
	}
}

func TestNormalizeSource(t *testing.T) {
	t.Parallel()
	require := require.New(t)

	tests := []struct {
		input    string
		expected string
	}{
		// BluRay variants
		{"BluRay", "BluRay"},
		{"BLURAY", "BluRay"},
		{"Blu-Ray", "BluRay"},
		{"BLU-RAY", "BluRay"},
		{"BDRip", "BluRay"},
		{"BDRIP", "BluRay"},
		// WEB-DL variants
		{"WEB-DL", "WEB-DL"},
		{"WEBDL", "WEB-DL"},
		{"WEB.DL", "WEB-DL"},
		{"web-dl", "WEB-DL"},
		// WEBRip
		{"WEBRip", "WEBRip"},
		{"WEBRIP", "WEBRip"},
		// HDTV
		{"HDTV", "HDTV"},
		{"hdtv", "HDTV"},
		// DVDRip
		{"DVDRip", "DVDRip"},
		{"DVDRIP", "DVDRip"},
		// Unknown (passthrough)
		{"CAM", "CAM"},
		{"TS", "TS"},
	}

	for _, tc := range tests {
		result := normalizeSource(tc.input)
		require.Equal(tc.expected, result, "normalizeSource(%q)", tc.input)
	}
}

func TestNormalizeCodec(t *testing.T) {
	t.Parallel()
	require := require.New(t)

	tests := []struct {
		input    string
		expected string
	}{
		// HEVC variants
		{"x265", "HEVC"},
		{"X265", "HEVC"},
		{"H.265", "HEVC"},
		{"H265", "HEVC"},
		{"h265", "HEVC"},
		{"HEVC", "HEVC"},
		{"hevc", "HEVC"},
		// H.264 variants
		{"x264", "H.264"},
		{"X264", "H.264"},
		{"H.264", "H.264"},
		{"H264", "H.264"},
		{"h264", "H.264"},
		{"AVC", "H.264"},
		{"avc", "H.264"},
		// AV1
		{"AV1", "AV1"},
		{"av1", "AV1"},
		// Unknown (passthrough)
		{"DivX", "DivX"},
		{"XviD", "XviD"},
	}

	for _, tc := range tests {
		result := normalizeCodec(tc.input)
		require.Equal(tc.expected, result, "normalizeCodec(%q)", tc.input)
	}
}

// ============================================================
// Context Extraction Tests
// ============================================================

func TestExtractContext(t *testing.T) {
	t.Parallel()
	require := require.New(t)

	identifier := NewIdentifier(nil)

	// Note: SeasonFolder pattern requires / or \ or end of string after season number
	// So "S01.Something" won't match, but "S01/" or "S01" at end will
	tests := []struct {
		name         string
		torrentName  string
		seasonHint   *int
		isComplete   bool
		qualityHint  string
	}{
		{
			name:        "season hint from S01/",
			torrentName: "Show/S01/1080p.BluRay",
			seasonHint:  intPtr(1),
			isComplete:  false,
			qualityHint: "1080p",
		},
		{
			name:        "season hint from Season 02/",
			torrentName: "Show/Season 02/720p",
			seasonHint:  intPtr(2),
			isComplete:  false,
			qualityHint: "720p",
		},
		{
			name:        "no season hint without path separator",
			torrentName: "Show.S01.1080p.BluRay",
			seasonHint:  nil, // S01 is not followed by / or end
			isComplete:  false,
			qualityHint: "1080p",
		},
		{
			name:        "complete series",
			torrentName: "Show.Complete.Series.1080p",
			seasonHint:  nil,
			isComplete:  true,
			qualityHint: "1080p",
		},
		{
			name:        "full series",
			torrentName: "Show Full Series 2160p",
			seasonHint:  nil,
			isComplete:  true,
			qualityHint: "2160p",
		},
		{
			name:        "all seasons",
			torrentName: "Show All Seasons 1080p",
			seasonHint:  nil,
			isComplete:  true,
			qualityHint: "1080p",
		},
		{
			name:        "4K quality",
			torrentName: "Show.4K.HDR",
			seasonHint:  nil,
			isComplete:  false,
			qualityHint: "2160p",
		},
		{
			name:        "no hints",
			torrentName: "RandomName",
			seasonHint:  nil,
			isComplete:  false,
			qualityHint: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := identifier.extractContext(tc.torrentName)
			require.Equal(tc.torrentName, ctx.TorrentName)
			require.Equal(tc.isComplete, ctx.IsComplete)
			require.Equal(tc.qualityHint, ctx.QualityHint)

			if tc.seasonHint == nil {
				require.Nil(ctx.SeasonHint)
			} else {
				require.NotNil(ctx.SeasonHint)
				require.Equal(*tc.seasonHint, *ctx.SeasonHint)
			}
		})
	}
}

func TestExtractSeasonFromPath(t *testing.T) {
	t.Parallel()
	require := require.New(t)

	identifier := NewIdentifier(nil)

	tests := []struct {
		path     string
		season   int
		hasMatch bool
	}{
		{"Show/Season 01/episode.mkv", 1, true},
		{"Show/Season 1/episode.mkv", 1, true},
		{"Show/Season.01/episode.mkv", 1, true},
		{"Show/S01/episode.mkv", 1, true},
		{"Show/S1/episode.mkv", 1, true},
		{"Show/Season 02/episode.mkv", 2, true},
		{"Show/Season 12/episode.mkv", 12, true},
		{"Show/S10/episode.mkv", 10, true},
		// Nested
		{"Show/Season 01/Disc 1/episode.mkv", 1, true},
		// Case insensitivity
		{"Show/season 01/episode.mkv", 1, true},
		{"Show/SEASON 01/episode.mkv", 1, true},
		// No match
		{"Show/episode.mkv", 0, false},
		{"Show/Part 01/episode.mkv", 0, false},
		{"Show/Disc 01/episode.mkv", 0, false},
	}

	for _, tc := range tests {
		season, hasMatch := identifier.extractSeasonFromPath(tc.path)
		require.Equal(tc.hasMatch, hasMatch, "hasMatch for path %q", tc.path)
		if tc.hasMatch {
			require.Equal(tc.season, season, "season for path %q", tc.path)
		}
	}
}

// ============================================================
// Quality Extraction Tests
// ============================================================

func TestExtractQuality(t *testing.T) {
	t.Parallel()
	require := require.New(t)

	identifier := NewIdentifier(nil)

	tests := []struct {
		name       string
		filename   string
		context    *Context
		resolution string
		source     string
		codec      string
		hdr        bool
	}{
		{
			name:       "full quality info",
			filename:   "Show.S01E01.2160p.BluRay.x265.HDR.mkv",
			context:    &Context{},
			resolution: "2160p",
			source:     "BluRay",
			codec:      "HEVC",
			hdr:        true,
		},
		{
			name:       "1080p WEB-DL x264",
			filename:   "Show.S01E01.1080p.WEB-DL.x264.mkv",
			context:    &Context{},
			resolution: "1080p",
			source:     "WEB-DL",
			codec:      "H.264",
			hdr:        false,
		},
		{
			name:       "720p HDTV",
			filename:   "Show.S01E01.720p.HDTV.mkv",
			context:    &Context{},
			resolution: "720p",
			source:     "HDTV",
			codec:      "",
			hdr:        false,
		},
		{
			name:       "4K Dolby Vision",
			filename:   "Show.S01E01.4K.Dolby.Vision.mkv",
			context:    &Context{},
			resolution: "2160p",
			source:     "",
			codec:      "",
			hdr:        true,
		},
		{
			name:       "HDR10+",
			filename:   "Show.S01E01.2160p.HDR10+.mkv",
			context:    &Context{},
			resolution: "2160p",
			source:     "",
			codec:      "",
			hdr:        true,
		},
		{
			name:       "quality from context when not in filename",
			filename:   "Show.S01E01.mkv",
			context:    &Context{QualityHint: "1080p"},
			resolution: "1080p",
			source:     "",
			codec:      "",
			hdr:        false,
		},
		{
			name:       "filename overrides context",
			filename:   "Show.S01E01.720p.mkv",
			context:    &Context{QualityHint: "1080p"},
			resolution: "720p",
			source:     "",
			codec:      "",
			hdr:        false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			quality := identifier.extractQuality(tc.filename, tc.context)
			require.Equal(tc.resolution, quality.Resolution)
			require.Equal(tc.source, quality.Source)
			require.Equal(tc.codec, quality.Codec)
			require.Equal(tc.hdr, quality.HDR)
		})
	}
}

// ============================================================
// tryPatterns Tests
// ============================================================

func TestTryPatterns(t *testing.T) {
	t.Parallel()
	require := require.New(t)

	identifier := NewIdentifier(nil)

	tests := []struct {
		name            string
		filename        string
		folderSeason    int
		hasFolderSeason bool
		expectMatch     bool
		expectedSeason  int
		expectedEps     []int
		confidence      Confidence
		pattern         string
		isSpecial       bool
	}{
		// HIGH CONFIDENCE - SxxExx
		{
			name:           "standard SxxExx",
			filename:       "Show.S01E01.mkv",
			expectMatch:    true,
			expectedSeason: 1,
			expectedEps:    []int{1},
			confidence:     ConfidenceHigh,
			pattern:        "SxxExx",
		},
		{
			name:           "SxxExx with multi-digit",
			filename:       "Show.S12E99.mkv",
			expectMatch:    true,
			expectedSeason: 12,
			expectedEps:    []int{99},
			confidence:     ConfidenceHigh,
			pattern:        "SxxExx",
		},
		// HIGH CONFIDENCE - SxxExx Range
		{
			name:           "SxxExx range",
			filename:       "Show.S01E01-E03.mkv",
			expectMatch:    true,
			expectedSeason: 1,
			expectedEps:    []int{1, 2, 3},
			confidence:     ConfidenceHigh,
			pattern:        "SxxExx-Exx",
		},
		{
			name:           "SxxExx range without E",
			filename:       "Show.S02E05-08.mkv",
			expectMatch:    true,
			expectedSeason: 2,
			expectedEps:    []int{5, 6, 7, 8},
			confidence:     ConfidenceHigh,
			pattern:        "SxxExx-Exx",
		},
		// HIGH CONFIDENCE - SxxExx Multi
		{
			name:           "SxxExx multi-episode",
			filename:       "Show.S01E01E02E03.mkv",
			expectMatch:    true,
			expectedSeason: 1,
			expectedEps:    []int{1, 2, 3},
			confidence:     ConfidenceHigh,
			pattern:        "SxxExxExx",
		},
		// HIGH CONFIDENCE - XxYY
		{
			name:           "XxYY format",
			filename:       "Show.1x01.mkv",
			expectMatch:    true,
			expectedSeason: 1,
			expectedEps:    []int{1},
			confidence:     ConfidenceHigh,
			pattern:        "XxYY",
		},
		{
			name:           "XxYY with leading zeros",
			filename:       "Show.01x12.mkv",
			expectMatch:    true,
			expectedSeason: 1,
			expectedEps:    []int{12},
			confidence:     ConfidenceHigh,
			pattern:        "XxYY",
		},
		// MEDIUM CONFIDENCE - Season X Episode Y
		{
			name:           "Season Episode format",
			filename:       "Season 1 Episode 5.mkv",
			expectMatch:    true,
			expectedSeason: 1,
			expectedEps:    []int{5},
			confidence:     ConfidenceMedium,
			pattern:        "Season X Episode Y",
		},
		// MEDIUM CONFIDENCE - Episode number with folder context
		{
			name:            "Episode number with folder",
			filename:        "Episode.01.mkv",
			folderSeason:    1,
			hasFolderSeason: true,
			expectMatch:     true,
			expectedSeason:  1,
			expectedEps:     []int{1},
			confidence:      ConfidenceMedium,
			pattern:         "Ep/Episode + folder",
		},
		// SPECIAL EPISODES
		{
			name:           "S00Exx special",
			filename:       "Show.S00E01.mkv",
			expectMatch:    true,
			expectedSeason: 0,
			expectedEps:    []int{1},
			confidence:     ConfidenceHigh,
			pattern:        "S00Exx",
			isSpecial:      true,
		},
		{
			name:           "Special keyword",
			filename:       "Show.Special.mkv",
			expectMatch:    true,
			expectedSeason: 0,
			expectedEps:    []int{1},
			confidence:     ConfidenceMedium,
			pattern:        "Special keyword",
			isSpecial:      true,
		},
		{
			name:           "OVA keyword",
			filename:       "Show.OVA.mkv",
			expectMatch:    true,
			expectedSeason: 0,
			expectedEps:    []int{1},
			confidence:     ConfidenceMedium,
			pattern:        "Special keyword",
			isSpecial:      true,
		},
		// LOW CONFIDENCE - Concatenated with folder context
		{
			name:            "Concatenated 4-digit with folder",
			filename:        "Show.0105.mkv",
			folderSeason:    1,
			hasFolderSeason: true,
			expectMatch:     true,
			expectedSeason:  1,
			expectedEps:     []int{5},
			confidence:      ConfidenceLow,
			pattern:         "SSEE + folder",
		},
		{
			name:            "Concatenated 3-digit with folder",
			filename:        "Show.105.mkv",
			folderSeason:    1,
			hasFolderSeason: true,
			expectMatch:     true,
			expectedSeason:  1,
			expectedEps:     []int{5},
			confidence:      ConfidenceLow,
			pattern:         "SEE + folder",
		},
		// NO MATCH
		{
			name:        "no pattern match",
			filename:    "Movie.2024.1080p.mkv",
			expectMatch: false,
		},
		{
			name:        "random filename",
			filename:    "random_file.mkv",
			expectMatch: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			season, episodes, confidence, pattern, isSpecial, ok := identifier.tryPatterns(
				tc.filename, tc.folderSeason, tc.hasFolderSeason,
			)

			require.Equal(tc.expectMatch, ok, "match expectation")
			if tc.expectMatch {
				require.Equal(tc.expectedSeason, season, "season")
				require.Equal(tc.expectedEps, episodes, "episodes")
				require.Equal(tc.confidence, confidence, "confidence")
				require.Equal(tc.pattern, pattern, "pattern")
				require.Equal(tc.isSpecial, isSpecial, "isSpecial")
			}
		})
	}
}

// ============================================================
// Full Identify() Integration Tests
// ============================================================

func TestIdentifyStandardTVSeason(t *testing.T) {
	t.Parallel()
	require := require.New(t)

	identifier := NewIdentifier(nil)

	files := []TorrentFile{
		{Path: "Show/Season 01/Show.S01E01.1080p.mkv", Size: 1000000000},
		{Path: "Show/Season 01/Show.S01E02.1080p.mkv", Size: 1000000000},
		{Path: "Show/Season 01/Show.S01E03.1080p.mkv", Size: 1000000000},
		{Path: "Show/Season 01/sample.mkv", Size: 50000000},           // Should be skipped
		{Path: "Show/Season 01/Show.S01E01.srt", Size: 50000},         // Subtitle
		{Path: "Show/Season 01/Show.S01E02.srt", Size: 50000},         // Subtitle
		{Path: "Show/Season 01/Show.S01E01.nfo", Size: 1000},          // Non-media, ignored
	}

	result := identifier.Identify(nil, files, "Show.S01.1080p.BluRay")

	require.Equal("Show.S01.1080p.BluRay", result.TorrentName)
	require.Equal(5, result.TotalFiles)      // 3 videos + 2 subtitles (nfo and sample excluded)
	require.Equal(5, result.IdentifiedCount)
	require.Len(result.IdentifiedFiles, 5)
	require.Len(result.UnidentifiedFiles, 0)

	// Check first video file
	found := false
	for _, f := range result.IdentifiedFiles {
		if f.FilePath == "Show/Season 01/Show.S01E01.1080p.mkv" {
			found = true
			require.Equal(1, f.Season)
			require.Equal([]int{1}, f.Episodes)
			require.Equal(FileTypeVideo, f.FileType)
			require.Equal(ConfidenceHigh, f.Confidence)
			require.Equal("SxxExx", f.PatternUsed)
			require.Equal("1080p", f.Quality.Resolution)
		}
	}
	require.True(found, "should find S01E01 video")

	// Check subtitle file
	found = false
	for _, f := range result.IdentifiedFiles {
		if f.FilePath == "Show/Season 01/Show.S01E01.srt" {
			found = true
			require.Equal(FileTypeSubtitle, f.FileType)
		}
	}
	require.True(found, "should find S01E01 subtitle")
}

func TestIdentifyMixedPatterns(t *testing.T) {
	t.Parallel()
	require := require.New(t)

	identifier := NewIdentifier(nil)

	files := []TorrentFile{
		{Path: "Show.1x01.mkv", Size: 1000000000},      // XxYY format
		{Path: "Show.S01E02.mkv", Size: 1000000000},    // SxxExx format
		{Path: "Show.Season.1.Episode.3.mkv", Size: 1000000000}, // Season Episode format
	}

	result := identifier.Identify(nil, files, "Show.Season.1")

	require.Equal(3, result.TotalFiles)
	require.Equal(3, result.IdentifiedCount)

	// Verify all were identified correctly
	for _, f := range result.IdentifiedFiles {
		require.Equal(1, f.Season)
		switch f.FilePath {
		case "Show.1x01.mkv":
			require.Equal([]int{1}, f.Episodes)
			require.Equal("XxYY", f.PatternUsed)
		case "Show.S01E02.mkv":
			require.Equal([]int{2}, f.Episodes)
			require.Equal("SxxExx", f.PatternUsed)
		case "Show.Season.1.Episode.3.mkv":
			require.Equal([]int{3}, f.Episodes)
			require.Equal("Season X Episode Y", f.PatternUsed)
		}
	}
}

func TestIdentifyMovieNoEpisodes(t *testing.T) {
	t.Parallel()
	require := require.New(t)

	identifier := NewIdentifier(nil)

	files := []TorrentFile{
		{Path: "Movie.2024.1080p.BluRay.mkv", Size: 5000000000},
		{Path: "Movie.2024.1080p.BluRay.srt", Size: 50000},
	}

	result := identifier.Identify(nil, files, "Movie.2024.1080p.BluRay")

	require.Equal(2, result.TotalFiles)
	require.Equal(0, result.IdentifiedCount)
	require.Len(result.UnidentifiedFiles, 2)
}

func TestIdentifyMultiEpisodeFile(t *testing.T) {
	t.Parallel()
	require := require.New(t)

	identifier := NewIdentifier(nil)

	files := []TorrentFile{
		{Path: "Show.S01E01E02.mkv", Size: 2000000000},
		{Path: "Show.S01E03-E05.mkv", Size: 3000000000},
	}

	result := identifier.Identify(nil, files, "Show.S01")

	require.Equal(2, result.TotalFiles)
	require.Equal(2, result.IdentifiedCount)

	for _, f := range result.IdentifiedFiles {
		switch f.FilePath {
		case "Show.S01E01E02.mkv":
			require.Equal([]int{1, 2}, f.Episodes)
		case "Show.S01E03-E05.mkv":
			require.Equal([]int{3, 4, 5}, f.Episodes)
		}
	}
}

func TestIdentifySpecialEpisodes(t *testing.T) {
	t.Parallel()
	require := require.New(t)

	identifier := NewIdentifier(nil)

	files := []TorrentFile{
		{Path: "Show.S00E01.Special.mkv", Size: 1000000000},
		{Path: "Show.OVA.mkv", Size: 500000000},
	}

	result := identifier.Identify(nil, files, "Show.Specials")

	require.Equal(2, result.TotalFiles)
	require.Equal(2, result.IdentifiedCount)

	for _, f := range result.IdentifiedFiles {
		require.True(f.IsSpecial)
		require.Equal(0, f.Season)
	}
}

func TestIdentifyWithFolderSeasonContext(t *testing.T) {
	t.Parallel()
	require := require.New(t)

	identifier := NewIdentifier(nil)

	files := []TorrentFile{
		{Path: "Show/Season 02/Episode.01.mkv", Size: 1000000000},
		{Path: "Show/Season 02/Episode.02.mkv", Size: 1000000000},
	}

	result := identifier.Identify(nil, files, "Show.Complete")

	require.Equal(2, result.TotalFiles)
	require.Equal(2, result.IdentifiedCount)

	for _, f := range result.IdentifiedFiles {
		require.Equal(2, f.Season)
		require.True(f.SeasonFromFolder)
	}
}

func TestIdentifyQualityExtraction(t *testing.T) {
	t.Parallel()
	require := require.New(t)

	identifier := NewIdentifier(nil)

	files := []TorrentFile{
		{Path: "Show.S01E01.2160p.BluRay.x265.HDR.mkv", Size: 10000000000},
	}

	result := identifier.Identify(nil, files, "Show.S01.2160p")

	require.Equal(1, result.IdentifiedCount)

	f := result.IdentifiedFiles[0]
	require.Equal("2160p", f.Quality.Resolution)
	require.Equal("BluRay", f.Quality.Source)
	require.Equal("HEVC", f.Quality.Codec)
	require.True(f.Quality.HDR)
}

// ============================================================
// Edge Case Tests
// ============================================================

func TestIdentifyEmptyFileList(t *testing.T) {
	t.Parallel()
	require := require.New(t)

	identifier := NewIdentifier(nil)
	result := identifier.Identify(nil, []TorrentFile{}, "Show")

	require.Equal(0, result.TotalFiles)
	require.Equal(0, result.IdentifiedCount)
	require.Len(result.IdentifiedFiles, 0)
	require.Len(result.UnidentifiedFiles, 0)
}

func TestIdentifyAllNonMediaFiles(t *testing.T) {
	t.Parallel()
	require := require.New(t)

	identifier := NewIdentifier(nil)

	files := []TorrentFile{
		{Path: "readme.txt", Size: 1000},
		{Path: "info.nfo", Size: 500},
		{Path: "cover.jpg", Size: 100000},
	}

	result := identifier.Identify(nil, files, "Show")

	require.Equal(0, result.TotalFiles) // Non-media files not counted
	require.Equal(0, result.IdentifiedCount)
}

func TestIdentifyAllSamples(t *testing.T) {
	t.Parallel()
	require := require.New(t)

	identifier := NewIdentifier(nil)

	files := []TorrentFile{
		{Path: "Sample/video.mkv", Size: 50000000},
		{Path: "sample.mkv", Size: 50000000},
		{Path: "trailer.mp4", Size: 30000000},
	}

	result := identifier.Identify(nil, files, "Show")

	require.Equal(0, result.TotalFiles) // All skipped
	require.Equal(0, result.IdentifiedCount)
}

func TestIdentifyLongFilename(t *testing.T) {
	t.Parallel()
	require := require.New(t)

	identifier := NewIdentifier(nil)

	longName := "Very.Long.Show.Name.With.Many.Words.S01E01.This.Is.Episode.Title.1080p.BluRay.x265.HEVC.10bit.AAC.5.1-GROUP.mkv"
	files := []TorrentFile{
		{Path: longName, Size: 1000000000},
	}

	result := identifier.Identify(nil, files, "Very.Long.Show.Name")

	require.Equal(1, result.IdentifiedCount)
	require.Equal(1, result.IdentifiedFiles[0].Season)
	require.Equal([]int{1}, result.IdentifiedFiles[0].Episodes)
}

func TestIdentifyNestedFolders(t *testing.T) {
	t.Parallel()
	require := require.New(t)

	identifier := NewIdentifier(nil)

	files := []TorrentFile{
		{Path: "Show/Complete/Season 01/Disc 1/Show.S01E01.mkv", Size: 1000000000},
		{Path: "Show/Complete/Season 01/Disc 2/Show.S01E05.mkv", Size: 1000000000},
		{Path: "Show/Complete/Season 02/Show.S02E01.mkv", Size: 1000000000},
	}

	result := identifier.Identify(nil, files, "Show.Complete.Series")

	require.Equal(3, result.TotalFiles)
	require.Equal(3, result.IdentifiedCount)

	// Verify folder season extraction
	for _, f := range result.IdentifiedFiles {
		if f.Season == 1 {
			require.True(f.SeasonFromFolder || f.PatternUsed == "SxxExx")
		}
	}
}

func TestIdentifyNeedsReviewFlag(t *testing.T) {
	t.Parallel()
	require := require.New(t)

	identifier := NewIdentifier(nil)

	files := []TorrentFile{
		// Low confidence pattern - needs review
		{Path: "Show/Season 01/Show.0105.mkv", Size: 1000000000},
		// High confidence - no review needed
		{Path: "Show/Season 01/Show.S01E06.mkv", Size: 1000000000},
	}

	result := identifier.Identify(nil, files, "Show")

	for _, f := range result.IdentifiedFiles {
		if f.Confidence == ConfidenceLow {
			require.True(f.NeedsReview)
		} else {
			require.False(f.NeedsReview)
		}
	}
}

// ============================================================
// Fallback Handler Tests
// ============================================================

// MockFallback is a test fallback handler
type MockFallback struct {
	called      bool
	filesCount  int
	returnFiles map[string]*IdentifiedFile
}

func (m *MockFallback) IdentifyBatch(files []UnidentifiedFile, metadata *loader.TMDBMetadata) (map[string]*IdentifiedFile, error) {
	m.called = true
	m.filesCount = len(files)
	return m.returnFiles, nil
}

func TestIdentifyWithFallbackHandler(t *testing.T) {
	t.Parallel()
	require := require.New(t)

	fallback := &MockFallback{
		returnFiles: map[string]*IdentifiedFile{
			"Movie.2024.mkv": {
				FilePath:   "Movie.2024.mkv",
				FileSize:   5000000000,
				FileType:   FileTypeVideo,
				Season:     0,
				Episodes:   []int{},
				Confidence: ConfidenceMedium,
			},
		},
	}

	identifier := NewIdentifier(fallback)

	files := []TorrentFile{
		{Path: "Show.S01E01.mkv", Size: 1000000000},   // Will be identified by regex
		{Path: "Movie.2024.mkv", Size: 5000000000},    // Will be passed to fallback
	}

	result := identifier.Identify(nil, files, "Mixed.Content")

	require.True(fallback.called)
	require.Equal(1, fallback.filesCount) // Only unidentified file
	require.Equal(2, result.TotalFiles)
	require.Equal(2, result.IdentifiedCount) // Both identified (one by regex, one by fallback)
	require.Len(result.UnidentifiedFiles, 0)
}

func TestNoOpFallback(t *testing.T) {
	t.Parallel()
	require := require.New(t)

	fallback := &NoOpFallback{}
	result, err := fallback.IdentifyBatch([]UnidentifiedFile{}, nil)

	require.NoError(err)
	require.Nil(result)
}

func TestNewIdentifierWithNilFallback(t *testing.T) {
	t.Parallel()
	require := require.New(t)

	identifier := NewIdentifier(nil)
	require.NotNil(identifier)
	require.NotNil(identifier.patterns)
	require.NotNil(identifier.fallback) // Should default to NoOpFallback
}

// ============================================================
// Helper Functions
// ============================================================

func intPtr(i int) *int {
	return &i
}
