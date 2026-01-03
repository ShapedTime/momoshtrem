package identify

import (
	"github.com/shapedtime/momoshtrem/internal/library"
)

// UnmatchedReason describes why a file couldn't be matched
type UnmatchedReason string

const (
	ReasonNoLibraryEpisode  UnmatchedReason = "no_library_episode"
	ReasonNotVideo          UnmatchedReason = "not_video"
	ReasonSample            UnmatchedReason = "sample"
	ReasonCouldNotIdentify  UnmatchedReason = "could_not_identify"
	ReasonSpecialNotSupport UnmatchedReason = "special_not_supported"
)

// MatchResult contains the results of matching identified files to library episodes
type MatchResult struct {
	Matched   []MatchedEpisode
	Unmatched []UnmatchedFile
}

// MatchedEpisode represents a successful match between a torrent file and a library episode
type MatchedEpisode struct {
	Episode      *library.Episode
	Season       *library.Season
	FilePath     string
	FileSize     int64
	Quality      QualityInfo
	Confidence   Confidence
	NeedsReview  bool
	PatternUsed  string
}

// UnmatchedFile represents a file that couldn't be matched to a library episode
type UnmatchedFile struct {
	FilePath string
	Reason   UnmatchedReason
	Season   int // -1 if unknown
	Episode  int // -1 if unknown, only first episode for ranges
}

// MatchToShow maps identified files to library episodes
// It builds a lookup of existing episodes and matches identified files against it
func MatchToShow(show *library.Show, result *IdentificationResult) *MatchResult {
	matchResult := &MatchResult{
		Matched:   make([]MatchedEpisode, 0),
		Unmatched: make([]UnmatchedFile, 0),
	}

	// Build episode lookup: map[seasonNumber][episodeNumber] -> (Episode, Season)
	type episodeKey struct {
		season  int
		episode int
	}
	episodeLookup := make(map[episodeKey]struct {
		episode *library.Episode
		season  *library.Season
	})

	for i := range show.Seasons {
		season := &show.Seasons[i]
		for j := range season.Episodes {
			ep := &season.Episodes[j]
			key := episodeKey{season: season.SeasonNumber, episode: ep.EpisodeNumber}
			episodeLookup[key] = struct {
				episode *library.Episode
				season  *library.Season
			}{
				episode: ep,
				season:  season,
			}
		}
	}

	// Process identified files
	for _, identified := range result.IdentifiedFiles {
		// Skip non-video files
		if identified.FileType != FileTypeVideo {
			matchResult.Unmatched = append(matchResult.Unmatched, UnmatchedFile{
				FilePath: identified.FilePath,
				Reason:   ReasonNotVideo,
				Season:   identified.Season,
				Episode:  firstEpisode(identified.Episodes),
			})
			continue
		}

		// Skip special episodes for now
		if identified.IsSpecial {
			matchResult.Unmatched = append(matchResult.Unmatched, UnmatchedFile{
				FilePath: identified.FilePath,
				Reason:   ReasonSpecialNotSupport,
				Season:   0,
				Episode:  firstEpisode(identified.Episodes),
			})
			continue
		}

		// For each episode in the identified file (handles multi-episode files)
		for _, epNum := range identified.Episodes {
			key := episodeKey{season: identified.Season, episode: epNum}
			if entry, ok := episodeLookup[key]; ok {
				matchResult.Matched = append(matchResult.Matched, MatchedEpisode{
					Episode:     entry.episode,
					Season:      entry.season,
					FilePath:    identified.FilePath,
					FileSize:    identified.FileSize,
					Quality:     identified.Quality,
					Confidence:  identified.Confidence,
					NeedsReview: identified.NeedsReview,
					PatternUsed: identified.PatternUsed,
				})
			} else {
				matchResult.Unmatched = append(matchResult.Unmatched, UnmatchedFile{
					FilePath: identified.FilePath,
					Reason:   ReasonNoLibraryEpisode,
					Season:   identified.Season,
					Episode:  epNum,
				})
			}
		}
	}

	// Process unidentified files
	for _, path := range result.UnidentifiedFiles {
		matchResult.Unmatched = append(matchResult.Unmatched, UnmatchedFile{
			FilePath: path,
			Reason:   ReasonCouldNotIdentify,
			Season:   -1,
			Episode:  -1,
		})
	}

	return matchResult
}

// MovieMatchResult contains the result of finding a movie file in a torrent
type MovieMatchResult struct {
	Found       bool
	FilePath    string
	FileSize    int64
	Quality     QualityInfo
	OtherFiles  []string // Other video files that were not selected
}

// FindMovieFile finds the best movie file in a list of torrent files
// It selects the largest video file that isn't a sample/trailer
func FindMovieFile(files []TorrentFile) *MovieMatchResult {
	result := &MovieMatchResult{
		Found:      false,
		OtherFiles: make([]string, 0),
	}

	var bestFile *TorrentFile
	var bestSize int64

	patterns := NewCompiledPatterns()

	for i := range files {
		file := &files[i]

		// Skip non-video files
		if !isVideoFile(file.Path) {
			continue
		}

		// Skip samples, trailers, extras
		if shouldSkip(file.Path) {
			continue
		}

		// Track as potential file
		if file.Size > bestSize {
			// If we had a previous best, add it to other files
			if bestFile != nil {
				result.OtherFiles = append(result.OtherFiles, bestFile.Path)
			}
			bestFile = file
			bestSize = file.Size
		} else {
			result.OtherFiles = append(result.OtherFiles, file.Path)
		}
	}

	if bestFile != nil {
		result.Found = true
		result.FilePath = bestFile.Path
		result.FileSize = bestFile.Size
		result.Quality = extractQualityFromPath(bestFile.Path, patterns)
	}

	return result
}

// extractQualityFromPath extracts quality info from a file path
func extractQualityFromPath(path string, patterns *CompiledPatterns) QualityInfo {
	quality := QualityInfo{}

	// Resolution
	if match := patterns.Resolution.FindString(path); match != "" {
		quality.Resolution = normalizeResolution(match)
	}

	// Source
	if match := patterns.Source.FindString(path); match != "" {
		quality.Source = normalizeSource(match)
	}

	// Codec
	if match := patterns.Codec.FindString(path); match != "" {
		quality.Codec = normalizeCodec(match)
	}

	// HDR
	if patterns.HDR.MatchString(path) {
		quality.HDR = true
	}

	return quality
}

// firstEpisode returns the first episode number from a slice, or -1 if empty
func firstEpisode(episodes []int) int {
	if len(episodes) > 0 {
		return episodes[0]
	}
	return -1
}
