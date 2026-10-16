package vfs

// TestMovie and TestEpisode are videos for NewTestFS: the names tests used
// before NewStaticFS left this file.
type (
	TestMovie   = StaticMovie
	TestEpisode = StaticEpisode
)

// NewTestFS is NewStaticFS, for tests outside the package (WebDAV end to
// end).
func NewTestFS(movies []TestMovie, episodes []TestEpisode) *LibraryFS {
	return NewStaticFS(movies, episodes)
}
