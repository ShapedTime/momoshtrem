package vfs

// TreeUpdater provides methods to perform partial updates to the VFS tree.
// This interface is implemented by LibraryFS and used by the API server
// to update the tree immediately when assignments change. Assigning goes
// through assignment jobs, which rebuild the tree (InvalidateTree): a
// targeted add couldn't rename a grouped multi-episode entry.
type TreeUpdater interface {
	// RemoveMovieFromTree removes a movie folder and file from the tree
	RemoveMovieFromTree(title string, year int)

	// RemoveEpisodeFromTree removes an episode file (and empty parent folders)
	RemoveEpisodeFromTree(showTitle string, showYear int, seasonNumber int, episodeNumber int)

	// RemoveShowFromTree removes an entire show subtree
	RemoveShowFromTree(title string, year int)

	// InvalidateTree forces a full tree rebuild on next access
	InvalidateTree()

	// InvalidateFileHandles closes cached file handles for a torrent.
	// Call before removing a torrent or unassigning files.
	InvalidateFileHandles(infoHash string)
}
