package library

import "errors"

// Sentinel errors for library operations.
var (
	ErrShowNotFound              = errors.New("show not found")
	ErrMovieNotFound             = errors.New("movie not found")
	ErrInvalidMagnet             = errors.New("invalid magnet URI")
	ErrTorrentServiceUnavailable = errors.New("torrent service not available")
	ErrNoVideoFiles              = errors.New("no video files found in torrent")
)
