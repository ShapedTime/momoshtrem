package library

import "errors"

// Sentinel errors for library operations.
var (
	ErrShowNotFound              = errors.New("show not found")
	ErrMovieNotFound             = errors.New("movie not found")
	ErrEpisodeNotFound           = errors.New("episode not found")
	ErrInvalidMagnet             = errors.New("invalid magnet URI")
	ErrTorrentServiceUnavailable = errors.New("torrent service not available")
	ErrNoVideoFiles              = errors.New("no video files found in torrent")
	ErrTMDBFetch                 = errors.New("failed to fetch from TMDB")
	// ErrInvalidShowSettings is returned for an unknown episode ordering or
	// classifier mode. The API maps it to 422.
	ErrInvalidShowSettings = errors.New("invalid show settings")
)
