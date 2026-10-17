package activity

import (
	"time"

	"github.com/shapedtime/momoshtrem/internal/downloads"
	"github.com/shapedtime/momoshtrem/internal/sessions"
	"github.com/shapedtime/momoshtrem/internal/torrent"
)

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
	Streaming    []StreamingSession `json:"streaming"` // its files' sessions, the most recently read first
	// Wanted is what downloads want of it (#77): its wanted files' size and
	// what's on disk of them, the manager's state for it and the ETA; null
	// when no download wants it.
	Wanted *downloads.TorrentWanted `json:"wanted"`
	// Holds are the downloads that want it, kept first, then by id.
	Holds []downloads.TorrentHold `json:"holds"`
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

// StreamingSession is a client streaming a file over WebDAV: one of the
// torrent's on a list item, any source in the summary.
type StreamingSession = sessions.Session

// Detail is GET /torrents/{hash} (TorrentDetail): the list item plus the
// detail page's facts. It never loads the torrent: they come from the
// client when it's loaded, else the stored metainfo, else the newest job's
// file list.
type Detail struct {
	Item
	MagnetURI   *string `json:"magnet_uri"`
	PieceLength *int64  `json:"piece_length"`
	PiecesTotal *int    `json:"pieces_total"`
	PiecesHave  *int    `json:"pieces_have"` // null when not loaded
	// Pieces is the piece map, run-length in piece order; null when not
	// loaded.
	Pieces   []torrent.PieceRun `json:"pieces"`
	Files    []DetailFile       `json:"files"`
	Trackers []Tracker          `json:"trackers"`
	// DownloadedBytes and UploadedBytes count since the torrent loaded;
	// they, Ratio and Swarm are null when it isn't.
	DownloadedBytes *int64   `json:"downloaded_bytes"`
	UploadedBytes   *int64   `json:"uploaded_bytes"`
	Ratio           *float64 `json:"ratio"` // uploaded ÷ downloaded; null before anything arrived
	Swarm           *Swarm   `json:"swarm"`
}

// DetailFile is one file of the torrent, in its order.
type DetailFile struct {
	Path      string   `json:"path"` // the torrent's own path: "{name}/…" in a multi-file torrent
	SizeBytes int64    `json:"size_bytes"`
	Progress  *float64 `json:"progress"` // 0–1; null when not loaded
	// PlaysAs are the library items its active assignments map it to: a
	// movie, or its episodes in order.
	PlaysAs []PlaysAs `json:"plays_as"`
	// Ignored says why the newest job left it out ("Ignored (NCOP)"); null
	// when it plays as something, or no job ignored it.
	Ignored *Ignored `json:"ignored"`
}

// PlaysAs is a library item a file plays as.
type PlaysAs struct {
	MediaType string `json:"media_type"` // movie or tv
	// ItemID is the movie's or the episode's library id.
	ItemID        int64   `json:"item_id"`
	SeasonNumber  *int    `json:"season_number"`
	EpisodeNumber *int    `json:"episode_number"`
	EpisodeName   *string `json:"episode_name"` // null for movies, and episodes without a name
	Label         string  `json:"label"`        // "S01E29" or "Movie"
}

// Ignored is why a file is left out: reason is the extra's type (opening,
// ending, sample, trailer, featurette, font, info, image, superseded,
// other), "subtitle", or "user" for a video the user (or a movie pick)
// left out.
type Ignored struct {
	Reason string `json:"reason"`
	Label  string `json:"label"`
}

// Tracker is one of the torrent's trackers. anacrolix keeps its announce
// results to itself, so the status is "unknown" and the rest null.
type Tracker struct {
	URL            string     `json:"url"`
	Status         string     `json:"status"` // working, error or unknown
	Error          *string    `json:"error"`
	Seeders        *int       `json:"seeders"`
	Peers          *int       `json:"peers"`
	LastAnnounceAt *time.Time `json:"last_announce_at"`
}

// Swarm is who the client is connected to; TotalSeeders needs a tracker
// scrape, which nothing does yet, so it's null.
type Swarm struct {
	ConnectedPeers   int  `json:"connected_peers"`
	ConnectedSeeders int  `json:"connected_seeders"`
	TotalSeeders     *int `json:"total_seeders"`
}

// Summary is GET /torrents/summary.
type Summary struct {
	DownloadBps int64              `json:"download_bps"`
	UploadBps   int64              `json:"upload_bps"`
	Peers       int                `json:"peers"`
	History     History            `json:"history"`
	Counts      Counts             `json:"counts"`
	Disk        *Disk              `json:"disk"`      // null when it can't be read
	Streaming   []StreamingSession `json:"streaming"` // every session (HDRezka too), the most recently read first
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
	TorrentsBytes   int64 `json:"torrents_bytes"`    // pieces in the cache: kept, smart and streaming cache together
	CacheLimitBytes int64 `json:"cache_limit_bytes"` // the cap the cache applies now
	// KeptBytes and SmartBytes are what's pinned on disk for kept and smart
	// downloads (#77), as /settings/storage's usage.
	KeptBytes  int64 `json:"kept_bytes"`
	SmartBytes int64 `json:"smart_bytes"`
}

// PauseCount is POST /torrents/pause-all and /resume-all.
type PauseCount struct {
	Count int `json:"count"`
}
