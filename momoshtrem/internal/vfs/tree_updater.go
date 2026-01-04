package vfs

import "github.com/shapedtime/momoshtrem/internal/library"

// EpisodeWithContext bundles episode data needed for tree operations
type EpisodeWithContext struct {
	ShowTitle    string
	ShowYear     int
	SeasonNumber int
	Episode      *library.Episode
	Assignment   *library.TorrentAssignment
}

// TreeUpdater provides methods to perform partial updates to the VFS tree.
// This interface is implemented by LibraryFS and used by the API server
// to update the tree immediately when assignments change.
type TreeUpdater interface {
	// AddMovieToTree adds a movie and its file to the tree
	AddMovieToTree(movie *library.Movie, assignment *library.TorrentAssignment)

	// RemoveMovieFromTree removes a movie folder and file from the tree
	RemoveMovieFromTree(title string, year int)

	// AddEpisodesToTree adds episodes (with show/season folders as needed)
	AddEpisodesToTree(episodes []EpisodeWithContext)

	// RemoveEpisodeFromTree removes an episode file (and empty parent folders)
	RemoveEpisodeFromTree(showTitle string, showYear int, seasonNumber int, episodeNumber int)

	// RemoveShowFromTree removes an entire show subtree
	RemoveShowFromTree(title string, year int)
}
