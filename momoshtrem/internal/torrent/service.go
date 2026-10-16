package torrent

import (
	"errors"
	"time"

	"github.com/anacrolix/torrent"

	"github.com/shapedtime/momoshtrem/internal/identify"
)

// Common errors
var (
	ErrTorrentNotFound = errors.New("torrent not found")
	ErrMetadataTimeout = errors.New("timeout waiting for torrent metadata")
	ErrInvalidMagnet   = errors.New("invalid magnet URI")
	ErrNoFiles         = errors.New("torrent contains no files")
	ErrFileNotFound    = errors.New("file not found in torrent")
)

// TorrentInfo contains information about an added torrent
type TorrentInfo struct {
	InfoHash  string
	Name      string
	Files     []identify.TorrentFile
	TotalSize int64
	AddedAt   time.Time
}

// TorrentStatus contains current status of a loaded torrent
type TorrentStatus struct {
	InfoHash   string
	Name       string
	TotalSize  int64
	Downloaded int64   // bytes on disk
	Progress   float64 // 0.0 to 1.0
	// Complete is true when every piece is on disk. Pieces are fetched on
	// demand and kept in an LRU cache, so it can go back to false.
	Complete bool
	Seeders  int // connected seeders
	Leechers int
	Peers    int // connected peers
	// DownloadSpeed and UploadSpeed are smoothed, in bytes per second (the
	// speed sampler's; 0 without one).
	DownloadSpeed int64
	UploadSpeed   int64
	// IsPaused is true while the torrent's network is off: idle, or paused
	// by the user.
	IsPaused bool
	// Paused is true while the user has it paused (Pause).
	Paused bool
	// LoadedAt is when it was added to the client.
	LoadedAt time.Time
	// LastActive is when it was last read, resumed or loaded.
	LastActive time.Time
}

// FullStats contains complete torrent statistics for Prometheus metrics collection.
type FullStats struct {
	InfoHash          string
	Name              string
	TotalSize         int64
	BytesCompleted    int64
	ActivePeers       int
	ConnectedSeeders  int
	HalfOpenPeers     int
	PiecesComplete    int
	BytesReadData     int64 // Cumulative data bytes downloaded from peers
	BytesWrittenData  int64 // Cumulative data bytes uploaded to peers
	ChunksReadWasted  int64 // Wasted chunks received
	PiecesDirtiedGood int64 // Pieces that passed hash verification
	PiecesDirtiedBad  int64 // Pieces that failed hash verification
	PieceLength       int64 // Piece size in bytes
	NumPieces         int   // Total piece count
}

// FileProgress is how much of one file of a torrent is on disk.
type FileProgress struct {
	Length         int64
	BytesCompleted int64
}

// Complete reports whether every byte of the file is on disk.
func (p FileProgress) Complete() bool {
	return p.BytesCompleted >= p.Length
}

// DebugInfo contains detailed piece-level information for debugging torrent streaming.
type DebugInfo struct {
	InfoHash    string          `json:"info_hash"`
	Name        string          `json:"name"`
	PieceLength int64           `json:"piece_length"`
	NumPieces   int             `json:"num_pieces"`
	Pieces      string          `json:"pieces"` // base64 encoded, 1 byte per piece
	Files       []DebugFileInfo `json:"files"`
	Readers     []DebugReaderInfo `json:"readers"`
	Stats       DebugStats      `json:"stats"`
}

// DebugFileInfo contains per-file progress information.
type DebugFileInfo struct {
	Path           string  `json:"path"`
	Length         int64   `json:"length"`
	BytesCompleted int64   `json:"bytes_completed"`
	Progress       float64 `json:"progress"`
	BeginPiece     int     `json:"begin_piece"`
	EndPiece       int     `json:"end_piece"`
}

// DebugReaderInfo contains information about an active reader.
type DebugReaderInfo struct {
	FilePath      string `json:"file_path"`
	Position      int64  `json:"position"`
	PositionPiece int    `json:"position_piece"`
	FileLength    int64  `json:"file_length"`
}

// DebugStats contains aggregate torrent statistics.
type DebugStats struct {
	PiecesComplete   int   `json:"pieces_complete"`
	PiecesTotal      int   `json:"pieces_total"`
	ActivePeers      int   `json:"active_peers"`
	ConnectedSeeders int   `json:"connected_seeders"`
	BytesCompleted   int64 `json:"bytes_completed"`
	TotalSize        int64 `json:"total_size"`
}

// Service manages torrent operations.
// This interface is used by the API handlers to add torrents and get file metadata,
// and by the VFS layer to stream file content.
type Service interface {
	// AddTorrent adds a torrent by magnet URI, waits for metadata,
	// and returns information about the torrent including its files.
	// This operation may take some time as it needs to connect to peers
	// and download the torrent metadata.
	// Returns ErrMetadataTimeout if metadata cannot be retrieved in time.
	// Returns ErrInvalidMagnet if the magnet URI is invalid.
	// A torrent whose metainfo is stored loads without waiting for peers.
	// Adds, waits, drops and removes of one hash are serialised, and
	// concurrent adds of one hash share one attempt.
	AddTorrent(magnetURI string) (*TorrentInfo, error)

	// GetTorrent returns information about an already-added torrent.
	// Returns ErrTorrentNotFound if the torrent is not loaded.
	GetTorrent(infoHash string) (*TorrentInfo, error)

	// GetOrAddTorrent returns an existing torrent or adds it if not present.
	GetOrAddTorrent(magnetURI string) (*TorrentInfo, error)

	// GetFile returns a file handle for streaming a specific file from a torrent.
	// Returns ErrTorrentNotFound if the torrent is not loaded.
	// Returns ErrFileNotFound if the file path doesn't exist in the torrent.
	GetFile(infoHash string, filePath string) (TorrentFileHandle, error)

	// RemoveTorrent removes a torrent from the client (waiting for an add
	// of it in flight) and deletes its stored metainfo. If deleteData is
	// true, also deletes its pieces from the cache, from the live or the
	// stored metainfo. The hash may be hex in either case, or base32.
	// Returns ErrTorrentNotFound when it's neither loaded nor stored.
	RemoveTorrent(infoHash string, deleteData bool) error

	// ListTorrents returns status of all active torrents.
	ListTorrents() ([]TorrentStatus, error)

	// GetStatus returns detailed status of a specific torrent.
	GetStatus(infoHash string) (*TorrentStatus, error)

	// Pause pauses a torrent until Resume, loaded or not: its network goes
	// off and reads don't wake it; one that isn't loaded is paused when it
	// loads. Idempotent. The pause lives in memory: the caller persists it
	// (RestorePauses). The hash may be hex in either case, or base32;
	// ErrInvalidInfoHash otherwise.
	Pause(infoHash string) error

	// Resume clears a pause. A loaded torrent's network comes back on until
	// the idle timeout; one that isn't loaded stays unloaded. Idempotent.
	Resume(infoHash string) error

	// PausedAt returns when the torrent was paused; ok is false when it
	// isn't.
	PausedAt(infoHash string) (at time.Time, ok bool)

	// Pauses returns every paused torrent, loaded or not, with when it was
	// paused.
	Pauses() map[string]time.Time

	// RestorePauses applies pauses kept from before a restart.
	RestorePauses(pauses map[string]time.Time)

	// CollectStats returns complete statistics for all active torrents.
	// Used by the Prometheus metrics collector.
	CollectStats() []FullStats

	// GetDebugInfo returns detailed piece-level debug information for a torrent.
	GetDebugInfo(infoHash string) (*DebugInfo, error)

	// Detail returns what the client knows of a loaded torrent: its piece
	// map (run-length), files, trackers and totals, for Activity's detail.
	// The hash may be hex in either case, or base32. Returns
	// ErrTorrentNotFound if the torrent is not loaded; it never loads one.
	Detail(infoHash string) (*Detail, error)

	// FilesProgress returns how much of each file of a loaded torrent is on
	// disk, keyed by path. It's cheap: no piece map. Returns
	// ErrTorrentNotFound if the torrent is not loaded; the map is empty while
	// the metadata is still arriving.
	FilesProgress(infoHash string) (map[string]FileProgress, error)

	// IsActive reports whether a torrent is loaded with its network on
	// (not idled by the activity manager).
	IsActive(infoHash string) bool

	// Close shuts down the torrent service.
	Close() error
}

// TorrentFileHandle provides access to a file within a torrent for streaming.
type TorrentFileHandle interface {
	// Path returns the file path within the torrent.
	Path() string
	// Length returns the file size in bytes.
	Length() int64
	// NewReader creates a new reader for the file content.
	NewReader() TorrentReader
	// Torrent returns the underlying torrent for piece prioritization.
	Torrent() *torrent.Torrent
	// File returns the underlying file for piece prioritization.
	File() *torrent.File
	// NextFile returns the next file in the torrent (for pre-fetching), or nil.
	NextFile() *torrent.File
}

// TorrentReader provides reading capabilities for torrent file content.
type TorrentReader interface {
	Read(p []byte) (n int, err error)
	Seek(offset int64, whence int) (int64, error)
	Close() error
	// SetResponsive prioritizes the current reading position.
	SetResponsive()
}

// extractInfoHash extracts the info hash from a magnet URI
// This is a utility function used by implementations
func ExtractInfoHash(magnetURI string) string {
	const prefix = "xt=urn:btih:"
	idx := 0
	for i := 0; i < len(magnetURI)-len(prefix); i++ {
		if magnetURI[i:i+len(prefix)] == prefix {
			idx = i + len(prefix)
			break
		}
	}
	if idx == 0 {
		return ""
	}

	// Extract hash until next & or end
	end := idx
	for end < len(magnetURI) && magnetURI[end] != '&' {
		end++
	}

	hash := magnetURI[idx:end]

	// Normalize to lowercase
	result := make([]byte, len(hash))
	for i := 0; i < len(hash); i++ {
		c := hash[i]
		if c >= 'A' && c <= 'Z' {
			c = c + 32 // lowercase
		}
		result[i] = c
	}

	return string(result)
}
