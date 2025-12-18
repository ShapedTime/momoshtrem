package episode

import "regexp"

// CompiledPatterns contains all precompiled regex patterns for episode identification
type CompiledPatterns struct {
	// Primary patterns (High confidence)
	SxxExx      *regexp.Regexp // S01E01, s01e01, S1E1
	SxxExxRange *regexp.Regexp // S01E01-E03, S01E01-03
	SxxExxMulti *regexp.Regexp // S01E01E02E03
	XxYY        *regexp.Regexp // 1x01, 01x01

	// Secondary patterns (Medium confidence)
	SeasonEpisode *regexp.Regexp // Season 1 Episode 1
	EpNumber      *regexp.Regexp // Ep 1, Episode 1, E01
	AnimeEpisode  *regexp.Regexp // [Group] Show - 01 [quality]
	DateYMD       *regexp.Regexp // 2024.01.15 (daily shows)

	// Low confidence patterns
	Concatenated4 *regexp.Regexp // 0101 (SSEE format)
	Concatenated3 *regexp.Regexp // 101 (SEE format)

	// Folder/context patterns
	SeasonFolder *regexp.Regexp // Season 01, S01 (in folder path)

	// Quality extraction patterns
	Resolution *regexp.Regexp // 2160p, 4K, 1080p, 720p, 480p
	Source     *regexp.Regexp // BluRay, WEB-DL, HDTV, DVDRip
	Codec      *regexp.Regexp // x264, x265, H.264, H.265, HEVC, AV1
	HDR        *regexp.Regexp // HDR, HDR10, HDR10+, Dolby Vision, DV

	// Special episode patterns
	Special *regexp.Regexp // S00E01, Special, OVA, OAD
}

// NewCompiledPatterns creates and returns all compiled regex patterns
func NewCompiledPatterns() *CompiledPatterns {
	return &CompiledPatterns{
		// Primary patterns (High confidence)
		// S01E01, s01e01, S1E1, S01E1, S1E01
		SxxExx: regexp.MustCompile(`(?i)S(\d{1,2})E(\d{1,3})`),

		// S01E01-E03, S01E01-03, S01E01–E03 (en-dash), S01E01-E10
		SxxExxRange: regexp.MustCompile(`(?i)S(\d{1,2})E(\d{1,3})[-–]E?(\d{1,3})`),

		// S01E01E02E03 - captures season and all episodes
		SxxExxMulti: regexp.MustCompile(`(?i)S(\d{1,2})((?:E\d{1,3})+)`),

		// 1x01, 01x01, 1x001
		XxYY: regexp.MustCompile(`(?i)(\d{1,2})x(\d{2,3})`),

		// Secondary patterns (Medium confidence)
		// Season 1 Episode 1, Season.1.Episode.1
		SeasonEpisode: regexp.MustCompile(`(?i)Season[.\s]*(\d+)[.\s]*Episode[.\s]*(\d+)`),

		// Ep 1, Episode 1, Ep01, Episode.01, E01 (standalone)
		EpNumber: regexp.MustCompile(`(?i)(?:Ep(?:isode)?[.\s]*|(?:^|[.\s_-])E)(\d{1,3})(?:[.\s_\-\[]|$)`),

		// Anime format: [Group] Show - 01 [quality] or Show - 01v2
		// Matches: ] - 01 [ or ] - 01v2 or - 01 [
		AnimeEpisode: regexp.MustCompile(`(?:\]|^)\s*-?\s*(\d{2,3})(?:v\d)?(?:\s*[\[\(]|$)`),

		// Daily shows: 2024.01.15, 2024-01-15
		DateYMD: regexp.MustCompile(`(\d{4})[.\-](\d{2})[.\-](\d{2})`),

		// Low confidence patterns (require season context)
		// SSEE format: 0101, 0112, 1201 (4 digits, SS=01-99, EE=01-99)
		Concatenated4: regexp.MustCompile(`(?:^|[.\s_\-])(\d{2})(\d{2})(?:[.\s_\-]|$)`),

		// SEE format: 101, 112, 923 (3 digits, S=1-9, EE=01-99)
		Concatenated3: regexp.MustCompile(`(?:^|[.\s_\-])(\d)(\d{2})(?:[.\s_\-]|$)`),

		// Folder/context patterns
		// Season 01, Season.01, Season 1, S01, S1
		SeasonFolder: regexp.MustCompile(`(?i)(?:Season[.\s]*|S)(\d{1,2})(?:[/\\]|$)`),

		// Quality extraction patterns
		// 2160p, 1080p, 720p, 480p, 4K, UHD
		Resolution: regexp.MustCompile(`(?i)(2160|1080|720|480)p|4K|UHD`),

		// BluRay, Blu-Ray, WEB-DL, WEBRip, HDTV, DVDRip, BDRip
		Source: regexp.MustCompile(`(?i)(BluRay|Blu-Ray|BDRip|WEB-DL|WEB\.DL|WEBDL|WEBRip|HDTV|DVDRip|PDTV|SDTV)`),

		// x264, x265, H.264, H.265, HEVC, AV1, AVC
		Codec: regexp.MustCompile(`(?i)(x264|x265|H\.?264|H\.?265|HEVC|AV1|AVC)`),

		// HDR, HDR10, HDR10+, Dolby Vision, DV, DoVi
		HDR: regexp.MustCompile(`(?i)(HDR10\+?|HDR|Dolby[\s.]?Vision|DV|DoVi)`),

		// Special episode patterns
		// S00E01, Special, Specials, OVA, OAD, ONA
		Special: regexp.MustCompile(`(?i)S00E(\d+)|(?:^|[.\s_\-])(Special|OVA|OAD|ONA)(?:[.\s_\-]|$)`),
	}
}

// ExtractMultiEpisodes parses the episode portion of multi-episode patterns like "E01E02E03"
// Returns a slice of episode numbers
func ExtractMultiEpisodes(episodePart string) []int {
	re := regexp.MustCompile(`(?i)E(\d{1,3})`)
	matches := re.FindAllStringSubmatch(episodePart, -1)

	episodes := make([]int, 0, len(matches))
	for _, m := range matches {
		if len(m) >= 2 {
			ep := parseInt(m[1])
			if ep > 0 {
				episodes = append(episodes, ep)
			}
		}
	}
	return episodes
}

// ExpandEpisodeRange generates a slice of episode numbers from start to end inclusive
func ExpandEpisodeRange(start, end int) []int {
	if start > end || start < 1 || end > 999 {
		return []int{start}
	}

	episodes := make([]int, 0, end-start+1)
	for i := start; i <= end; i++ {
		episodes = append(episodes, i)
	}
	return episodes
}

// parseInt safely converts a string to int, returns 0 on error
func parseInt(s string) int {
	var result int
	for _, c := range s {
		if c >= '0' && c <= '9' {
			result = result*10 + int(c-'0')
		} else {
			return 0
		}
	}
	return result
}
