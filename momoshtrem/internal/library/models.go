package library

import (
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
	// Watched: Trakt has a play of it (B5; the matching rule of PLAN §6
	// "Watch state rules"). CatalogRepository's episode reads set it.
	Watched bool
}

// AssignmentKind is where an assignment's file comes from.
type AssignmentKind string

const (
	// AssignmentKindTorrent is a file inside a torrent.
	AssignmentKindTorrent AssignmentKind = "torrent"
	// AssignmentKindHDRezka is an HDRezka stream (StreamRef).
	AssignmentKindHDRezka AssignmentKind = "hdrezka"
)

// TorrentAssignment links a library item to its file: a file inside a
// torrent, or an HDRezka stream (Kind). The name predates HDRezka.
type TorrentAssignment struct {
	ID       int64
	ItemType ItemType
	ItemID   int64
	// Kind is the file's source; "" reads as a torrent.
	Kind AssignmentKind
	// InfoHash and MagnetURI are the torrent's; "" for an HDRezka stream.
	InfoHash  string
	MagnetURI string
	// FilePath is the file inside the torrent; for an HDRezka stream,
	// Stream.FilePath().
	FilePath   string
	FileSize   int64
	Resolution string // Optional: 1080p, 4K, etc.
	Source     string // Optional: BluRay, WEB-DL, etc.
	// Confidence is the classifier's (high, medium, low); "" for manual
	// picks, HDRezka streams and rows written before assignment jobs.
	Confidence string
	// MatchSource is who mapped the file: rules, ai or manual; "" for
	// HDRezka streams and rows written before assignment jobs.
	MatchSource string
	// Stream is the HDRezka stream; nil for a torrent file.
	Stream    *StreamRef
	IsActive  bool
	CreatedAt time.Time
}

// IsTorrent reports whether the file is inside a torrent.
func (a *TorrentAssignment) IsTorrent() bool {
	return a.Kind == "" || a.Kind == AssignmentKindTorrent
}

// Groupable reports whether the VFS may group the file with the other
// episodes that share it into one entry ("S01E01-E02"): only a torrent file
// an assignment job (or a manual pick) mapped. Legacy rows keep one entry
// per episode, so their paths never change.
func (a *TorrentAssignment) Groupable() bool {
	return a.IsTorrent() && a.MatchSource != ""
}

// StreamRef is an HDRezka stream: a title's translation in one quality, and
// for a series one episode. It's stored as the assignment's stream_ref; the
// stream's link is resolved from it when the file is opened (links expire).
type StreamRef struct {
	Provider string `json:"provider"` // "hdrezka"
	// Path is the title page's path, without the host (mirrors change):
	// "/films/fiction/2259-interstellar-2014.html".
	Path           string `json:"path"`
	PostID         int64  `json:"post_id"`
	Title          string `json:"title"`
	TranslatorID   int    `json:"translator_id"`
	TranslatorName string `json:"translator_name"`
	// The translator's flags: one translator id can come in several cuts.
	Camrip   bool `json:"camrip"`
	Ads      bool `json:"ads"`
	Director bool `json:"director"`
	// Season and Episode are HDRezka's numbers; 0 for a movie.
	Season  int    `json:"season,omitempty"`
	Episode int    `json:"episode,omitempty"`
	Quality string `json:"quality"` // e.g. "1080p"
}

// FilePath is the made-up path an HDRezka assignment stores as file_path:
// "hdrezka/2259/t56/movie/1080p.mp4", or "hdrezka/646/t1/s01e05/720p.mp4"
// for an episode. The translator gets -c, -a and -d for its camrip, ads and
// director flags. It gives the VFS the .mp4 extension (the stream is always
// an MP4) and keys the row; it's derived, never parsed back.
func (r StreamRef) FilePath() string {
	translator := fmt.Sprintf("t%d", r.TranslatorID)
	for _, flag := range []struct {
		on     bool
		suffix string
	}{{r.Camrip, "-c"}, {r.Ads, "-a"}, {r.Director, "-d"}} {
		if flag.on {
			translator += flag.suffix
		}
	}
	item := "movie"
	if r.Episode > 0 {
		item = fmt.Sprintf("s%02de%02d", r.Season, r.Episode)
	}
	return fmt.Sprintf("hdrezka/%d/%s/%s/%s.mp4", r.PostID, translator, item, qualitySlug(r.Quality))
}

// Label names the stream for people: "HDRezka · LostFilm · 1080p".
func (r StreamRef) Label() string {
	parts := []string{"HDRezka"}
	for _, p := range []string{r.TranslatorName, r.Quality} {
		if p = strings.TrimSpace(p); p != "" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, " · ")
}

// qualitySlug makes a quality label a file name: "1080p Ultra" becomes
// "1080p-ultra".
func qualitySlug(quality string) string {
	var b strings.Builder
	gap := false
	for _, r := range strings.ToLower(quality) {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9') {
			gap = true
			continue
		}
		if gap && b.Len() > 0 {
			b.WriteByte('-')
		}
		b.WriteRune(r)
		gap = false
	}
	if b.Len() == 0 {
		return "unknown"
	}
	return b.String()
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
