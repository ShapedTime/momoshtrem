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

// Subtitle represents a downloaded subtitle file for a library item
type Subtitle struct {
	ID           int64
	ItemType     ItemType
	ItemID       int64
	LanguageCode string // ISO 639-1 (en, ru, tr, az)
	LanguageName string // Display name (English, Russian, etc.)
	Format       string // srt, vtt, ass, ssa, sub
	FilePath     string // Local storage path
	FileSize     int64
	CreatedAt    time.Time
}

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
	Create(ctx context.Context, sub *Subtitle) error
	GetByID(ctx context.Context, id int64) (*Subtitle, error)
	GetByItem(ctx context.Context, itemType ItemType, itemID int64) ([]*Subtitle, error)
	GetByItemAndLanguage(ctx context.Context, itemType ItemType, itemID int64, languageCode string) (*Subtitle, error)
	Delete(ctx context.Context, id int64) error
	DeleteByItem(ctx context.Context, itemType ItemType, itemID int64) error
}
