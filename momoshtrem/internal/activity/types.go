package activity

import "time"

// The JSON shapes of PLAN §6 "Torrents and activity (B6)", field for field
// with web/src/lib/api/contract/torrents.ts. Nullable fields are pointers;
// lists are never null.

// State is a torrent's state on Activity.
type State string

const (
	StateStreaming   State = "streaming"
	StateDownloading State = "downloading"
	StatePaused      State = "paused"
	StateIdle        State = "idle"
)

// Item is one torrent of GET /torrents (TorrentListItem).
type Item struct {
	InfoHash string `json:"info_hash"` // 40 lowercase hex characters
	Name     string `json:"name"`
	State    State  `json:"state"`
	// Loaded is true while the torrent client has it. The fields that need
	// the client are null while it isn't.
	Loaded         bool     `json:"loaded"`
	Complete       *bool    `json:"complete"`
	Progress       *float64 `json:"progress"` // 0–1
	BytesCompleted *int64   `json:"bytes_completed"`
	SizeBytes      *int64   `json:"size_bytes"`
	DownloadBps    int64    `json:"download_bps"` // smoothed; 0 while not loaded
	UploadBps      int64    `json:"upload_bps"`
	Peers          *int     `json:"peers"`   // connected
	Seeders        *int     `json:"seeders"` // connected
	// AddedAt is the earliest active assignment or open job; when the
	// client loaded it, for a torrent nothing references.
	AddedAt time.Time `json:"added_at"`
	// LastActiveAt is the last read since the server started (or the
	// load, or a resume); null while not loaded.
	LastActiveAt *time.Time         `json:"last_active_at"`
	Titles       []Title            `json:"titles"`
	OpenJob      *OpenJob           `json:"open_job"`  // the newest open, non-dry-run job
	Streaming    []StreamingSession `json:"streaming"` // #99
}

// Title is a library title a torrent is for.
type Title struct {
	MediaType  string  `json:"media_type"` // movie or tv
	TMDBID     int     `json:"tmdb_id"`
	LibraryID  int64   `json:"library_id"`
	Title      string  `json:"title"`
	Year       int     `json:"year"`
	PosterPath *string `json:"poster_path"`
	// Scope names the show's episodes the torrent fills ("S01E29–E38",
	// "Season 1", "Seasons 1–3", "S01E01–E05 +2"); null for movies, and for
	// a show only an open job links.
	Scope *string `json:"scope"`
}

// OpenJob is a torrent's open mapping job.
type OpenJob struct {
	ID     int64  `json:"id"`
	Status string `json:"status"`
}

// StreamingSession is a client streaming one of the torrent's files. It's
// empty until #99 fills it; the lists are always empty until then.
type StreamingSession struct{}

// Summary is GET /torrents/summary.
type Summary struct {
	DownloadBps int64              `json:"download_bps"`
	UploadBps   int64              `json:"upload_bps"`
	Peers       int                `json:"peers"`
	History     History            `json:"history"`
	Counts      Counts             `json:"counts"`
	Disk        *Disk              `json:"disk"` // null when it can't be read
	Streaming   []StreamingSession `json:"streaming"`
}

// History is the last minute of total speeds, oldest first: one sample
// every IntervalSeconds, fewer right after startup.
type History struct {
	IntervalSeconds int     `json:"interval_seconds"`
	DownloadBps     []int64 `json:"download_bps"`
	UploadBps       []int64 `json:"upload_bps"`
}

// Counts are the torrents in each state.
type Counts struct {
	Streaming   int `json:"streaming"`
	Downloading int `json:"downloading"`
	Paused      int `json:"paused"`
	Idle        int `json:"idle"`
}

// Disk is the metadata folder's file system and the piece cache on it.
type Disk struct {
	TotalBytes      int64 `json:"total_bytes"`
	FreeBytes       int64 `json:"free_bytes"`
	TorrentsBytes   int64 `json:"torrents_bytes"`    // pieces in the cache
	CacheLimitBytes int64 `json:"cache_limit_bytes"` // the cache's capacity (global_cache_size)
}

// PauseCount is POST /torrents/pause-all and /resume-all.
type PauseCount struct {
	Count int `json:"count"`
}
