package library

import (
	"time"

	"github.com/shapedtime/momoshtrem/internal/common"
)

// ItemType represents the type of library item
type ItemType string

const (
	ItemTypeMovie   ItemType = "movie"
	ItemTypeEpisode ItemType = "episode"
)

// Movie represents a movie in the library
type Movie struct {
	ID        int64
	TMDBID    int
	Title     string
	Year      int
	CreatedAt time.Time

	// Loaded on demand
	Assignment *TorrentAssignment
}

// Show represents a TV show in the library
type Show struct {
	ID        int64
	TMDBID    int
	Title     string
	Year      int // First air year
	CreatedAt time.Time

	// Loaded on demand
	Seasons []Season
}

// Season represents a season of a TV show
type Season struct {
	ID           int64
	ShowID       int64
	SeasonNumber int

	// Loaded on demand
	Episodes []Episode
}

// Episode represents an episode of a TV show
type Episode struct {
	ID            int64
	SeasonID      int64
	EpisodeNumber int
	Name          string

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
	IsActive   bool
	CreatedAt  time.Time
}

// VFSPath returns the virtual filesystem path for a movie
func (m *Movie) VFSPath() string {
	return "/" + SanitizeFilename(m.Title) + " (" + common.Itoa(m.Year) + ")"
}

// VFSPath returns the virtual filesystem path for a show
func (s *Show) VFSPath() string {
	return "/" + SanitizeFilename(s.Title) + " (" + common.Itoa(s.Year) + ")"
}

// VFSPath returns the virtual filesystem path for a season
func (sn *Season) VFSPath(showPath string) string {
	return showPath + "/Season " + common.PadZero(sn.SeasonNumber, 2)
}

// VFSPath returns the virtual filesystem path for an episode
func (e *Episode) VFSPath(seasonPath string, showTitle string, seasonNumber int) string {
	name := e.Name
	if name == "" {
		name = "Episode " + common.Itoa(e.EpisodeNumber)
	}
	return seasonPath + "/" + SanitizeFilename(showTitle) + " - S" + common.PadZero(seasonNumber, 2) + "E" + common.PadZero(e.EpisodeNumber, 2) + " - " + SanitizeFilename(name)
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
