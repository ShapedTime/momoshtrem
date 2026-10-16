package subtitle

import (
	"context"
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
	// SourceOpenSubtitles: downloaded from OpenSubtitles to local storage.
	SourceOpenSubtitles Source = "opensubtitles"
	// SourceTorrent: a file in the item's torrent, streamed from it.
	SourceTorrent Source = "torrent"
	// SourceHDRezka: came with the item's HDRezka stream, converted to SRT
	// and stored locally when the stream was assigned.
	SourceHDRezka Source = "hdrezka"
)

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
	Source       Source
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
	// UpsertHDRezka stores the subtitles that came with an item's HDRezka
	// stream, one per language, each replacing the item's HDRezka subtitle
	// in its language. It writes only while streamFile is the item's active
	// file, and holds that file until it commits, so an assign can't change
	// it meanwhile; place runs just before the commit (it moves the files
	// into place). ErrStaleStream when the item's file has changed: nothing
	// is stored and place doesn't run.
	UpsertHDRezka(ctx context.Context, item Item, streamFile string, subs []*Subtitle, place func() error) error
	GetByID(ctx context.Context, id int64) (*Subtitle, error)
	GetByItem(ctx context.Context, itemType ItemType, itemID int64) ([]*Subtitle, error)
	// GetByItems loads the subtitles of many items in one query.
	GetByItems(ctx context.Context, items []Item) (map[Item][]*Subtitle, error)
	// GetDownloaded returns the item's downloaded subtitle in a language, or nil.
	GetDownloaded(ctx context.Context, itemType ItemType, itemID int64, languageCode string) (*Subtitle, error)
	Delete(ctx context.Context, id int64) error
	DeleteByItem(ctx context.Context, itemType ItemType, itemID int64) error
	// DeleteByItems deletes the items' subtitles from the given sources
	// (every source when none) in one query and returns the deleted rows.
	DeleteByItems(ctx context.Context, items []Item, sources ...Source) ([]*Subtitle, error)
}
