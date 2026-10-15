package library

import (
	"time"
)

// ItemType represents the type of library item
type ItemType string

const (
	ItemTypeMovie   ItemType = "movie"
	ItemTypeEpisode ItemType = "episode"
)

// EpisodeOrdering is how a show's episodes are numbered for display and
// matching.
type EpisodeOrdering string

const (
	// OrderingAired numbers episodes by TMDB season and episode.
	OrderingAired EpisodeOrdering = "aired"
	// OrderingAbsolute numbers episodes 1..N across the show (anime).
	OrderingAbsolute EpisodeOrdering = "absolute"
	// OrderingEpisodeGroup follows a TMDB episode group the user picked.
	OrderingEpisodeGroup EpisodeOrdering = "episode_group"
)

// Valid reports whether o is one of the known orderings.
func (o EpisodeOrdering) Valid() bool {
	switch o {
	case OrderingAired, OrderingAbsolute, OrderingEpisodeGroup:
		return true
	}
	return false
}

// ClassifierMode controls when the torrent file classifier calls the AI.
type ClassifierMode string

const (
	ClassifierAuto      ClassifierMode = "auto"
	ClassifierRulesOnly ClassifierMode = "rules_only"
	ClassifierAIAlways  ClassifierMode = "ai_always"
)

// Valid reports whether m is one of the known modes.
func (m ClassifierMode) Valid() bool {
	switch m {
	case ClassifierAuto, ClassifierRulesOnly, ClassifierAIAlways:
		return true
	}
	return false
}

// Movie represents a movie in the library
type Movie struct {
	ID        int64
	TMDBID    int
	Title     string // Fixed at creation: it names the WebDAV folder
	Year      int    // Fixed at creation: it names the WebDAV folder
	CreatedAt time.Time

	// TMDB metadata, written on create, refresh and backfill.
	IMDbID            string
	PosterPath        string
	BackdropPath      string
	Runtime           int // Minutes; 0 when unknown
	Genres            []string
	MetadataUpdatedAt *time.Time // nil until the first metadata sync

	// Loaded on demand
	Assignment *TorrentAssignment
}

// Show represents a TV show in the library
type Show struct {
	ID        int64
	TMDBID    int
	Title     string // Fixed at creation: it names the WebDAV folder
	Year      int    // First air year; fixed at creation like Title
	CreatedAt time.Time

	// TMDB metadata, written on create, refresh and backfill.
	IMDbID            string
	TVDBID            int
	OriginalName      string
	OriginalLanguage  string
	OriginCountry     []string
	IsAnime           bool
	PosterPath        string
	BackdropPath      string
	Status            string // TMDB status: "Returning Series", "Ended", ...
	Genres            []string
	MetadataUpdatedAt *time.Time // nil until the first metadata sync

	// User settings (PATCH /shows/{id}).
	EpisodeOrdering EpisodeOrdering
	EpisodeGroupID  string // TMDB episode group; "" when none
	IncludeSpecials bool   // Store and show season 0
	ClassifierMode  ClassifierMode

	// Loaded on demand
	Seasons []Season
}

// Season represents a season of a TV show. Season 0 holds specials.
type Season struct {
	ID           int64
	ShowID       int64
	SeasonNumber int
	Name         string
	PosterPath   string

	// Loaded on demand
	Episodes []Episode
}

// Episode represents an episode of a TV show
type Episode struct {
	ID             int64
	SeasonID       int64
	EpisodeNumber  int
	Name           string
	AirDate        string // ISO date (YYYY-MM-DD); "" when unknown
	TMDBEpisodeID  int
	AbsoluteNumber *int   // nil for specials and when unknown
	EpisodeType    string // TMDB: "standard", "finale", "mid_season"
	Runtime        int    // Minutes; 0 when unknown
	StillPath      string
	Overview       string

	// Loaded on demand
	Assignment *TorrentAssignment
}

// TorrentAssignment links a library item to a torrent file
type TorrentAssignment struct {
	ID         int64
	ItemType   ItemType
	ItemID     int64
	InfoHash   string
	MagnetURI  string
	FilePath   string
	FileSize   int64
	Resolution string // Optional: 1080p, 4K, etc.
	Source     string // Optional: BluRay, WEB-DL, etc.
	// Confidence is the classifier's (high, medium, low); "" for manual
	// picks and rows written before assignment jobs.
	Confidence string
	// MatchSource is who mapped the file: rules, ai or manual; "" for rows
	// written before assignment jobs. The VFS groups a multi-episode file
	// into one entry only when it's set.
	MatchSource string
	IsActive    bool
	CreatedAt   time.Time
}

// RecentlyAiredEpisode represents an episode with show context for the recently-aired API
type RecentlyAiredEpisode struct {
	ShowID        int64
	ShowTMDBID    int
	ShowTitle     string
	ShowYear      int
	SeasonNumber  int
	EpisodeID     int64
	EpisodeNumber int
	EpisodeName   string
	AirDate       string
	HasAssignment bool
}

// SanitizeFilename removes or replaces characters invalid in file paths
func SanitizeFilename(name string) string {
	// Replace problematic characters with safe alternatives
	var result []rune
	for _, r := range name {
		switch r {
		case '/', '\\':
			result = append(result, '-')
		case ':':
			result = append(result, '-')
		case '*', '?', '<', '>':
			// Skip these characters
		case '"':
			result = append(result, '\'')
		case '|':
			result = append(result, '-')
		default:
			result = append(result, r)
		}
	}
	return string(result)
}
