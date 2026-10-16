package sessions

import "time"

// Session is a client streaming a file (StreamingSession, PLAN §6
// "Torrents and activity (B6)"), field for field with
// web/src/lib/api/contract/torrents.ts. Nullable fields are pointers.
type Session struct {
	ID     string `json:"id"`
	Source Source `json:"source"` // torrent or hdrezka
	// InfoHash and FilePath are a torrent file's hash and its path inside
	// the torrent (the detail's files[].path); null for HDRezka.
	InfoHash *string `json:"info_hash"`
	Item     *Item   `json:"item"` // null when the library doesn't know the file
	FileName string  `json:"file_name"`
	FilePath *string `json:"file_path"`
	// Client is Infuse, VLC, Safari, Kodi or "Unknown", from the
	// User-Agent; null without one. Device is the User-Agent's (Apple TV,
	// iPhone, Mac…), else null.
	Client *string `json:"client"`
	Device *string `json:"device"`
	// OffsetBytes is where the client reads now: the end of its last read.
	OffsetBytes int64 `json:"offset_bytes"`
	SizeBytes   int64 `json:"size_bytes"`
	// PositionSeconds and DurationSeconds take the file to play at a
	// constant bitrate over the runtime: offset ÷ size × runtime. Null
	// without a runtime.
	PositionSeconds *int64 `json:"position_seconds"`
	DurationSeconds *int64 `json:"duration_seconds"`
	RateBps         int64  `json:"rate_bps"` // smoothed (EWMA, 5 s)
	// BufferedAheadBytes are the complete bytes contiguous from the offset
	// (torrents; null for HDRezka); the seconds need the runtime too.
	BufferedAheadBytes   *int64    `json:"buffered_ahead_bytes"`
	BufferedAheadSeconds *int64    `json:"buffered_ahead_seconds"`
	StartedAt            time.Time `json:"started_at"`
	LastReadAt           time.Time `json:"last_read_at"`
}

// Item is the library item a session plays: a movie, or a show's episode.
type Item struct {
	MediaType string `json:"media_type"` // movie or tv
	TMDBID    int    `json:"tmdb_id"`
	// LibraryID is the movie's or the show's library id.
	LibraryID     int64   `json:"library_id"`
	Title         string  `json:"title"`
	SeasonNumber  *int    `json:"season_number"`
	EpisodeNumber *int    `json:"episode_number"`
	EpisodeName   *string `json:"episode_name"`
}
