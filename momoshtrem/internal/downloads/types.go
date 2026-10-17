package downloads

import (
	"time"

	"github.com/shapedtime/momoshtrem/internal/library"
)

// State is a download's state (PLAN §6 "Downloads (#77)"), and a
// torrent's for the states that apply to one (TorrentWanted.state).
type State string

// The states, in the contract's precedence: a download takes the first
// that matches.
const (
	StateUnavailable      State = "unavailable"
	StateNoFile           State = "no_file"
	StateDone             State = "done"
	StatePaused           State = "paused"
	StateDiskFull         State = "disk_full"
	StateStorageFull      State = "storage_full"
	StateWaitingForWindow State = "waiting_for_window"
	StateWaitingForStream State = "waiting_for_stream"
	StateStalled          State = "stalled"
	StateDownloading      State = "downloading"
	StateQueued           State = "queued"
)

// Wants reports whether a file in this state still wants bytes, so it
// reads as downloading on the title pages (PLAN §6 "Availability rules"):
// queued, downloading, stalled, waiting, or held by a full disk. A paused
// one doesn't, nor one that's done or over the cap.
func (s State) Wants() bool {
	switch s {
	case StateQueued, StateDownloading, StateStalled, StateWaitingForWindow, StateWaitingForStream, StateDiskFull:
		return true
	}
	return false
}

// PausedReason is why background downloads wait now (GET /downloads).
type PausedReason string

const (
	PausedStreaming PausedReason = "streaming"
	PausedWindow    PausedReason = "window"
	PausedDisabled  PausedReason = "disabled"
)

// Download is a download on the wire (web/src/lib/api/contract/
// downloads.ts): GET /downloads' items and every POST and PATCH answer.
type Download struct {
	ID          int64                  `json:"id"`
	Reason      library.DownloadReason `json:"reason"`
	SmartSource *library.SmartSource   `json:"smart_source"`
	Scope       library.DownloadScope  `json:"scope"`
	Item        Item                   `json:"item"`
	State       State                  `json:"state"`
	// WantedBytes and DoneBytes are over its admitted torrent files (a
	// file several episodes share counts once); done is on disk, from the
	// piece cache.
	WantedBytes int64 `json:"wanted_bytes"`
	DoneBytes   int64 `json:"done_bytes"`
	// DownloadBps is its downloading torrents' smoothed speed, each once.
	DownloadBps int64  `json:"download_bps"`
	EtaSeconds  *int64 `json:"eta_seconds"`
	// ResumesAt is the next window's start (waiting_for_window) or the
	// next retry (stalled).
	ResumesAt    *time.Time `json:"resumes_at"`
	Files        Files      `json:"files"`
	UntilWatched bool       `json:"until_watched"`
	ExpiresAt    *time.Time `json:"expires_at"`
	CreatedAt    time.Time  `json:"created_at"`
}

// Item is what a download is for: the movie, or the show with the season
// and episode of those scopes.
type Item struct {
	MediaType string `json:"media_type"`
	TMDBID    int    `json:"tmdb_id"`
	// LibraryID is the movie's or the show's.
	LibraryID     int64   `json:"library_id"`
	Title         string  `json:"title"`
	PosterPath    *string `json:"poster_path"`
	SeasonNumber  *int    `json:"season_number"`
	EpisodeNumber *int    `json:"episode_number"`
	EpisodeName   *string `json:"episode_name"`
}

// Files counts the assigned files a download covers: on disk, that can't
// be downloaded (HDRezka, pure v2), and assigned later beyond its cap.
type Files struct {
	Total       int `json:"total"`
	Done        int `json:"done"`
	Unavailable int `json:"unavailable"`
	OverCap     int `json:"over_cap"`
}

// List is GET /downloads.
type List struct {
	Items        []Download    `json:"items"`
	Usage        Usage         `json:"usage"`
	PausedReason *PausedReason `json:"paused_reason"`
	// ResumesAt is the next window's start (window), or a minute after the
	// last stream ended (streaming; null while one streams).
	ResumesAt *time.Time `json:"resumes_at"`
}

// Usage is what downloads hold: on disk now, what kept downloads commit
// (as /settings/storage's), and the effective caps.
type Usage struct {
	KeptBytes          int64 `json:"kept_bytes"`
	SmartBytes         int64 `json:"smart_bytes"`
	CommittedKeptBytes int64 `json:"committed_kept_bytes"`
	KeptMaxBytes       int64 `json:"kept_max_bytes"`
	SmartMaxBytes      int64 `json:"smart_max_bytes"`
}

// Estimate is GET /downloads/estimate: what a Keep would need, before it's
// made.
type Estimate struct {
	WantedBytes    int64 `json:"wanted_bytes"`
	NeededBytes    int64 `json:"needed_bytes"`
	AvailableBytes int64 `json:"available_bytes"`
	Downloadable   bool  `json:"downloadable"`
}

// Keep is the download that holds a title, a season or an episode (PLAN §6
// "Titles and library", P6 #204).
type Keep struct {
	DownloadID   int64                  `json:"download_id"`
	Reason       library.DownloadReason `json:"reason"`
	Scope        library.DownloadScope  `json:"scope"`
	State        State                  `json:"state"`
	Pct          int                    `json:"pct"`
	UntilWatched bool                   `json:"until_watched"`
	ExpiresAt    *time.Time             `json:"expires_at"`
}

// TorrentWanted is what downloads want of a torrent (B6's
// TorrentListItem.wanted).
type TorrentWanted struct {
	Bytes      int64      `json:"bytes"`
	DoneBytes  int64      `json:"done_bytes"`
	EtaSeconds *int64     `json:"eta_seconds"`
	State      State      `json:"state"`
	ResumesAt  *time.Time `json:"resumes_at"`
}

// TorrentHold is a download that wants a torrent (B6's
// TorrentListItem.holds).
type TorrentHold struct {
	DownloadID int64                  `json:"download_id"`
	Reason     library.DownloadReason `json:"reason"`
	Scope      library.DownloadScope  `json:"scope"`
	// Title is the movie's or the show's.
	Title string `json:"title"`
}

// FileKey names a torrent file: its hash (lowercase hex) and its path in
// the torrent, as the client names it.
type FileKey struct {
	Hash string
	Path string
}
