package identify

import (
	"path/filepath"
	"strings"
)

// Video file extensions
var videoExtensions = map[string]bool{
	".mkv": true, ".mp4": true, ".avi": true, ".wmv": true,
	".mov": true, ".m4v": true, ".webm": true, ".ts": true,
	".m2ts": true, ".vob": true, ".flv": true, ".divx": true,
}

// Subtitle file extensions
var subtitleExtensions = map[string]bool{
	".srt": true, ".sub": true, ".ass": true, ".ssa": true,
	".vtt": true, ".idx": true, ".smi": true,
}

// FallbackHandler is an interface for handling unidentified files
// This allows future integration with local LLMs for complex identification
type FallbackHandler interface {
	IdentifyBatch(files []UnidentifiedFile, context *Context) (map[string]*IdentifiedFile, error)
}

// UnidentifiedFile represents a file that couldn't be identified by regex patterns
type UnidentifiedFile struct {
	Path    string
	Size    int64
	Context *Context
}

// Context provides hints extracted from torrent/folder names
type Context struct {
	TorrentName string
	SeasonHint  *int   // From folder like "Season 01"
	IsComplete  bool   // "Complete Series" indicator
	QualityHint string
}

// NoOpFallback is the default fallback that does nothing
type NoOpFallback struct{}

// IdentifyBatch returns nil, leaving files unidentified
func (n *NoOpFallback) IdentifyBatch(files []UnidentifiedFile, context *Context) (map[string]*IdentifiedFile, error) {
	return nil, nil
}

// Identifier is the main episode identification engine
type Identifier struct {
	patterns *CompiledPatterns
	fallback FallbackHandler
}

// NewIdentifier creates a new Identifier with the given fallback handler
// If fallback is nil, NoOpFallback is used
func NewIdentifier(fallback FallbackHandler) *Identifier {
	if fallback == nil {
		fallback = &NoOpFallback{}
	}
	return &Identifier{
		patterns: NewCompiledPatterns(),
		fallback: fallback,
	}
}

// Identify processes torrent files and returns identification results
func (i *Identifier) Identify(files []TorrentFile, torrentName string) *IdentificationResult {
	result := &IdentificationResult{
		TorrentName:       torrentName,
		IdentifiedFiles:   make([]IdentifiedFile, 0),
		UnidentifiedFiles: make([]string, 0),
		TotalFiles:        0,
		IdentifiedCount:   0,
	}

	// Extract global context from torrent name
	ctx := i.extractContext(torrentName)

	// Process each file
	for _, file := range files {
		// Skip non-media files
		if !isVideoFile(file.Path) && !isSubtitleFile(file.Path) {
			continue
		}

		// Skip samples, trailers, extras
		if shouldSkip(file.Path) {
			continue
		}

		result.TotalFiles++

		// Try to identify the file
		identified, ok := i.identifyFile(file, ctx)
		if ok {
			result.IdentifiedFiles = append(result.IdentifiedFiles, *identified)
			result.IdentifiedCount++
		} else {
			result.UnidentifiedFiles = append(result.UnidentifiedFiles, file.Path)
		}
	}

	// If we have unidentified files and a fallback handler, try to identify them
	if len(result.UnidentifiedFiles) > 0 && i.fallback != nil {
		unidentified := make([]UnidentifiedFile, len(result.UnidentifiedFiles))
		for idx, path := range result.UnidentifiedFiles {
			// Find the original file info
			var size int64
			for _, f := range files {
				if f.Path == path {
					size = f.Size
					break
				}
			}
			unidentified[idx] = UnidentifiedFile{
				Path:    path,
				Size:    size,
				Context: ctx,
			}
		}

		fallbackResults, err := i.fallback.IdentifyBatch(unidentified, ctx)
		if err == nil && fallbackResults != nil {
			// Update results with fallback identifications
			newUnidentified := make([]string, 0)
			for _, path := range result.UnidentifiedFiles {
				if identified, ok := fallbackResults[path]; ok {
					result.IdentifiedFiles = append(result.IdentifiedFiles, *identified)
					result.IdentifiedCount++
				} else {
					newUnidentified = append(newUnidentified, path)
				}
			}
			result.UnidentifiedFiles = newUnidentified
		}
	}

	return result
}

// extractContext extracts hints from the torrent name
func (i *Identifier) extractContext(torrentName string) *Context {
	ctx := &Context{
		TorrentName: torrentName,
	}

	// Check for season hint in torrent name
	if match := i.patterns.SeasonFolder.FindStringSubmatch(torrentName); match != nil {
		season := parseInt(match[1])
		if season > 0 {
			ctx.SeasonHint = &season
		}
	}

	// Check for "Complete" series indicator
	lowerName := strings.ToLower(torrentName)
	ctx.IsComplete = strings.Contains(lowerName, "complete") ||
		strings.Contains(lowerName, "full series") ||
		strings.Contains(lowerName, "all seasons")

	// Extract quality hint
	if match := i.patterns.Resolution.FindString(torrentName); match != "" {
		ctx.QualityHint = normalizeResolution(match)
	}

	return ctx
}

// extractSeasonFromPath extracts season number from folder path
func (i *Identifier) extractSeasonFromPath(filePath string) (int, bool) {
	// Split path into directory components
	dir := filepath.Dir(filePath)
	parts := strings.Split(dir, string(filepath.Separator))

	// Check each directory component for season patterns
	for _, part := range parts {
		if match := i.patterns.SeasonFolder.FindStringSubmatch(part); match != nil {
			season := parseInt(match[1])
			if season > 0 {
				return season, true
			}
		}
	}

	return 0, false
}

// identifyFile attempts to identify a single file
func (i *Identifier) identifyFile(file TorrentFile, ctx *Context) (*IdentifiedFile, bool) {
	filename := filepath.Base(file.Path)
	ext := strings.ToLower(filepath.Ext(file.Path))

	// Determine file type
	var fileType FileType
	if videoExtensions[ext] {
		fileType = FileTypeVideo
	} else if subtitleExtensions[ext] {
		fileType = FileTypeSubtitle
	} else {
		return nil, false
	}

	// Try to extract season from folder path
	folderSeason, hasFolderSeason := i.extractSeasonFromPath(file.Path)

	// Try patterns in order of confidence
	season, episodes, confidence, pattern, isSpecial, ok := i.tryPatterns(filename, folderSeason, hasFolderSeason)
	if !ok {
		return nil, false
	}

	// Extract quality info
	quality := i.extractQuality(filename, ctx)

	// Determine if review is needed
	needsReview := confidence == ConfidenceLow

	return &IdentifiedFile{
		FilePath:         file.Path,
		FileSize:         file.Size,
		FileType:         fileType,
		Season:           season,
		Episodes:         episodes,
		IsSpecial:        isSpecial,
		Quality:          quality,
		Confidence:       confidence,
		PatternUsed:      pattern,
		NeedsReview:      needsReview,
		SeasonFromFolder: hasFolderSeason && season == folderSeason,
	}, true
}

// tryPatterns tries all patterns in order of confidence and returns the first match
func (i *Identifier) tryPatterns(filename string, folderSeason int, hasFolderSeason bool) (season int, episodes []int, confidence Confidence, pattern string, isSpecial bool, ok bool) {
	// Check for special episodes first
	if match := i.patterns.Special.FindStringSubmatch(filename); match != nil {
		// S00E01 format
		if match[1] != "" {
			ep := parseInt(match[1])
			return 0, []int{ep}, ConfidenceHigh, "S00Exx", true, true
		}
		// Special/OVA/OAD keyword
		if match[2] != "" {
			return 0, []int{1}, ConfidenceMedium, "Special keyword", true, true
		}
	}

	// HIGH CONFIDENCE PATTERNS

	// Try SxxExx range first (S01E01-E03)
	if match := i.patterns.SxxExxRange.FindStringSubmatch(filename); match != nil {
		s := parseInt(match[1])
		startEp := parseInt(match[2])
		endEp := parseInt(match[3])
		return s, ExpandEpisodeRange(startEp, endEp), ConfidenceHigh, "SxxExx-Exx", false, true
	}

	// Try SxxExx multi-episode (S01E01E02E03)
	if match := i.patterns.SxxExxMulti.FindStringSubmatch(filename); match != nil {
		s := parseInt(match[1])
		eps := ExtractMultiEpisodes(match[2])
		if len(eps) > 1 {
			return s, eps, ConfidenceHigh, "SxxExxExx", false, true
		}
	}

	// Try standard SxxExx
	if match := i.patterns.SxxExx.FindStringSubmatch(filename); match != nil {
		s := parseInt(match[1])
		ep := parseInt(match[2])
		return s, []int{ep}, ConfidenceHigh, "SxxExx", false, true
	}

	// Try XxYY format (1x01)
	if match := i.patterns.XxYY.FindStringSubmatch(filename); match != nil {
		s := parseInt(match[1])
		ep := parseInt(match[2])
		return s, []int{ep}, ConfidenceHigh, "XxYY", false, true
	}

	// MEDIUM CONFIDENCE PATTERNS

	// Try Season X Episode Y
	if match := i.patterns.SeasonEpisode.FindStringSubmatch(filename); match != nil {
		s := parseInt(match[1])
		ep := parseInt(match[2])
		return s, []int{ep}, ConfidenceMedium, "Season X Episode Y", false, true
	}

	// Try Episode/Ep number (needs folder season context)
	if match := i.patterns.EpNumber.FindStringSubmatch(filename); match != nil {
		ep := parseInt(match[1])
		if hasFolderSeason {
			return folderSeason, []int{ep}, ConfidenceMedium, "Ep/Episode + folder", false, true
		}
	}

	// Try Anime format
	if match := i.patterns.AnimeEpisode.FindStringSubmatch(filename); match != nil {
		ep := parseInt(match[1])
		if ep > 0 {
			if hasFolderSeason {
				return folderSeason, []int{ep}, ConfidenceMedium, "Anime + folder", false, true
			}
			// Without folder context, assume season 1
			return 1, []int{ep}, ConfidenceLow, "Anime (assumed S1)", false, true
		}
	}

	// LOW CONFIDENCE PATTERNS (only use with folder context)
	if hasFolderSeason {
		// Try 4-digit concatenated format (0101 = S01E01)
		if match := i.patterns.Concatenated4.FindStringSubmatch(filename); match != nil {
			s := parseInt(match[1])
			ep := parseInt(match[2])
			// Validate that parsed season matches folder season for higher confidence
			if s == folderSeason && ep > 0 && ep <= 99 {
				return s, []int{ep}, ConfidenceLow, "SSEE + folder", false, true
			}
		}

		// Try 3-digit concatenated format (101 = S1E01)
		if match := i.patterns.Concatenated3.FindStringSubmatch(filename); match != nil {
			s := parseInt(match[1])
			ep := parseInt(match[2])
			if s == folderSeason && ep > 0 && ep <= 99 {
				return s, []int{ep}, ConfidenceLow, "SEE + folder", false, true
			}
		}
	}

	return 0, nil, ConfidenceNone, "", false, false
}

// extractQuality extracts quality information from filename
func (i *Identifier) extractQuality(filename string, ctx *Context) QualityInfo {
	quality := QualityInfo{}

	// Resolution
	if match := i.patterns.Resolution.FindString(filename); match != "" {
		quality.Resolution = normalizeResolution(match)
	} else if ctx.QualityHint != "" {
		quality.Resolution = ctx.QualityHint
	}

	// Source
	if match := i.patterns.Source.FindString(filename); match != "" {
		quality.Source = normalizeSource(match)
	}

	// Codec
	if match := i.patterns.Codec.FindString(filename); match != "" {
		quality.Codec = normalizeCodec(match)
	}

	// HDR
	if i.patterns.HDR.MatchString(filename) {
		quality.HDR = true
	}

	return quality
}

// normalizeResolution converts resolution to standard format
func normalizeResolution(match string) string {
	upper := strings.ToUpper(match)
	switch {
	case strings.Contains(upper, "2160") || upper == "4K" || upper == "UHD":
		return "2160p"
	case strings.Contains(upper, "1080"):
		return "1080p"
	case strings.Contains(upper, "720"):
		return "720p"
	case strings.Contains(upper, "480"):
		return "480p"
	default:
		return match
	}
}

// normalizeSource converts source to standard format
func normalizeSource(match string) string {
	upper := strings.ToUpper(match)
	switch {
	case strings.Contains(upper, "BLURAY") || strings.Contains(upper, "BLU-RAY") || strings.Contains(upper, "BDRIP"):
		return "BluRay"
	case strings.Contains(upper, "WEB-DL") || strings.Contains(upper, "WEBDL") || strings.Contains(upper, "WEB.DL"):
		return "WEB-DL"
	case strings.Contains(upper, "WEBRIP"):
		return "WEBRip"
	case strings.Contains(upper, "HDTV"):
		return "HDTV"
	case strings.Contains(upper, "DVDRIP"):
		return "DVDRip"
	default:
		return match
	}
}

// normalizeCodec converts codec to standard format
func normalizeCodec(match string) string {
	upper := strings.ToUpper(match)
	switch {
	case strings.Contains(upper, "265") || strings.Contains(upper, "HEVC"):
		return "HEVC"
	case strings.Contains(upper, "264") || strings.Contains(upper, "AVC"):
		return "H.264"
	case strings.Contains(upper, "AV1"):
		return "AV1"
	default:
		return match
	}
}

// IsVideoFile checks if the file is a video file based on extension
func IsVideoFile(path string) bool {
	return isVideoFile(path)
}

// isVideoFile checks if the file is a video file based on extension
func isVideoFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return videoExtensions[ext]
}

// isSubtitleFile checks if the file is a subtitle file based on extension
func isSubtitleFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return subtitleExtensions[ext]
}

// ShouldSkip returns true if the file should be skipped (samples, trailers, extras)
func ShouldSkip(path string) bool {
	return shouldSkip(path)
}

// shouldSkip returns true if the file should be skipped (samples, trailers, extras)
func shouldSkip(path string) bool {
	lower := strings.ToLower(path)

	skipPatterns := []string{
		"sample",
		"trailer",
		"preview",
		"extras/",
		"extras\\",
		"featurette",
		"deleted.scene",
		"deleted_scene",
		"deleted-scene",
		"behind.the.scene",
		"behind_the_scene",
		"behind-the-scene",
		"bonus/",
		"bonus\\",
		"/extra/",
		"\\extra\\",
	}

	for _, pattern := range skipPatterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}

	return false
}
