package subtitle

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// ItemType represents the type of library item
type ItemType string

const (
	ItemTypeMovie   ItemType = "movie"
	ItemTypeEpisode ItemType = "episode"
)

// Source represents where the subtitle came from
type Source string

const (
	SourceOpenSubtitles Source = "opensubtitles"
	SourceTorrent       Source = "torrent"
)

// ParseItemType converts a string to ItemType with validation.
func ParseItemType(s string) (ItemType, error) {
	switch strings.ToLower(s) {
	case "movie":
		return ItemTypeMovie, nil
	case "episode":
		return ItemTypeEpisode, nil
	default:
		return "", fmt.Errorf("invalid item type: %q (must be 'movie' or 'episode')", s)
	}
}

// Item is a library movie or episode that subtitles belong to.
type Item struct {
	Type ItemType
	ID   int64
}

// Subtitle represents a subtitle file for a library item
type Subtitle struct {
	ID           int64
	ItemType     ItemType
	ItemID       int64
	LanguageCode string // ISO 639-1 (en, ru, tr, az); "unknown" for undetected torrent files
	LanguageName string // Display name (English, Russian, etc.)
	Format       string // srt, vtt, ass, ssa, sub
	FilePath     string // Local storage path OR torrent file path (for Source=torrent)
	FileSize     int64
	Source       Source // "opensubtitles" or "torrent"
	InfoHash     string // For torrent subtitles: the torrent info hash
	ReleaseName  string // The release it was made for; "" when unknown
	CreatedAt    time.Time
}

// Item returns the library item the subtitle belongs to.
func (s *Subtitle) Item() Item { return Item{Type: s.ItemType, ID: s.ItemID} }

// Supported subtitle formats
var SupportedFormats = map[string]bool{
	"srt": true,
	"vtt": true,
	"ass": true,
	"ssa": true,
	"sub": true,
}

// ParseFormat extracts subtitle format from filename extension.
// Returns "srt" as default if format is not recognized.
func ParseFormat(filename string) string {
	lower := strings.ToLower(filename)
	for ext := range SupportedFormats {
		if strings.HasSuffix(lower, "."+ext) {
			return ext
		}
	}
	return "srt" // default
}

// SubtitleRepository defines the interface for subtitle storage operations.
type SubtitleRepository interface {
	// UpsertDownloaded stores an OpenSubtitles download, replacing the
	// item's downloaded subtitle in that language. Torrent subtitles stay.
	UpsertDownloaded(ctx context.Context, sub *Subtitle) error
	// UpsertTorrent stores a subtitle that came with the item's torrent and
	// drops the item's torrent subtitles from any other torrent.
	UpsertTorrent(ctx context.Context, sub *Subtitle) error
	GetByID(ctx context.Context, id int64) (*Subtitle, error)
	GetByItem(ctx context.Context, itemType ItemType, itemID int64) ([]*Subtitle, error)
	// GetByItems loads the subtitles of many items in one query.
	GetByItems(ctx context.Context, items []Item) (map[Item][]*Subtitle, error)
	// GetDownloaded returns the item's downloaded subtitle in a language, or nil.
	GetDownloaded(ctx context.Context, itemType ItemType, itemID int64, languageCode string) (*Subtitle, error)
	Delete(ctx context.Context, id int64) error
	DeleteByItem(ctx context.Context, itemType ItemType, itemID int64) error
}
