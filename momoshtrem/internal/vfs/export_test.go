package vfs

import (
	"path"

	"github.com/shapedtime/momoshtrem/internal/library"
)

// TestMovie and TestEpisode are videos for NewTestFS.
type TestMovie struct {
	Title      string
	Year       int
	Assignment *library.TorrentAssignment
}

type TestEpisode struct {
	Show       string
	Year       int
	Season     int
	Episode    int
	Name       string
	Assignment *library.TorrentAssignment
}

// NewTestFS is a LibraryFS whose tree holds the given videos, named as the
// tree builder names them, for tests outside the package (WebDAV end to
// end).
func NewTestFS(movies []TestMovie, episodes []TestEpisode) *LibraryFS {
	tree, moviesDir, tvDir := newEmptyTree()
	dir := func(parent *VirtualDir, dirPath string) *VirtualDir {
		if d, ok := tree.pathMap[dirPath].(*VirtualDir); ok {
			return d
		}
		d := NewVirtualDir(path.Base(dirPath))
		parent.children[d.name] = d
		tree.pathMap[dirPath] = d
		return d
	}
	file := func(d *VirtualDir, filePath string, a *library.TorrentAssignment) {
		pf := NewPlaceholderFile(path.Base(filePath), a.FileSize, a)
		d.children[pf.name] = pf
		tree.pathMap[filePath] = pf
	}
	for _, m := range movies {
		d := dir(moviesDir, MovieDir(m.Title, m.Year))
		file(d, MovieFilePath(m.Title, m.Year, m.Assignment.FilePath), m.Assignment)
	}
	for _, e := range episodes {
		show := dir(tvDir, ShowDir(e.Show, e.Year))
		season := dir(show, SeasonDir(e.Show, e.Year, e.Season))
		file(season, EpisodeFilePath(e.Show, e.Year, e.Season, e.Episode, e.Name, e.Assignment.FilePath), e.Assignment)
	}
	return &LibraryFS{tree: tree}
}
