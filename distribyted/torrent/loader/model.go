package loader

// MediaType represents the type of media (movie or TV show)
type MediaType string

const (
	MediaTypeMovie MediaType = "movie"
	MediaTypeTV    MediaType = "tv"
)

// TMDBMetadata contains TMDB information for a torrent
type TMDBMetadata struct {
	Type    MediaType `json:"type"`
	TMDBID  int       `json:"tmdb_id"`
	Title   string    `json:"title"`
	Year    int       `json:"year"`
	Season  *int      `json:"season,omitempty"`  // For TV season packs
	Episode *int      `json:"episode,omitempty"` // For single TV episodes
}

// TorrentWithMetadata wraps a magnet URI with optional metadata
type TorrentWithMetadata struct {
	MagnetURI string        `json:"magnet_uri"`
	Metadata  *TMDBMetadata `json:"metadata,omitempty"`
}
