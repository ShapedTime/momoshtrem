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

// TorrentStatus contains current status of a torrent
type TorrentStatus struct {
	InfoHash      string
	Name          string
	TotalSize     int64
	Downloaded    int64
	Progress      float64 // 0.0 to 1.0
	Seeders       int
	Leechers      int
	DownloadSpeed int64   // bytes per second
	UploadSpeed   int64   // bytes per second
	IsPaused      bool
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

	// RemoveTorrent removes a torrent from the client.
	// If deleteData is true, also deletes downloaded data.
	RemoveTorrent(infoHash string, deleteData bool) error

	// ListTorrents returns status of all active torrents.
	ListTorrents() ([]TorrentStatus, error)

	// GetStatus returns detailed status of a specific torrent.
	GetStatus(infoHash string) (*TorrentStatus, error)

	// Pause pauses downloading/uploading for a torrent.
	Pause(infoHash string) error

	// Resume resumes a paused torrent.
	Resume(infoHash string) error

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
