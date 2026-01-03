package vfs

import (
	"io/fs"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/shapedtime/momoshtrem/internal/library"
)

// LibraryFS implements Filesystem backed by the library database
type LibraryFS struct {
	mu             sync.RWMutex
	movieRepo      *library.MovieRepository
	showRepo       *library.ShowRepository
	assignmentRepo *library.AssignmentRepository

	// Cached tree structure
	tree        *DirectoryTree
	treeBuiltAt time.Time
	treeTTL     time.Duration
}

// DirectoryTree represents the virtual directory structure
type DirectoryTree struct {
	root    *VirtualDir
	pathMap map[string]Entry // Fast lookup by path
}

// Entry represents an entry in the virtual filesystem
type Entry interface {
	Name() string
	IsDir() bool
	Size() int64
}

// NewLibraryFS creates a new library-backed filesystem
func NewLibraryFS(
	movieRepo *library.MovieRepository,
	showRepo *library.ShowRepository,
	assignmentRepo *library.AssignmentRepository,
) *LibraryFS {
	return &LibraryFS{
		movieRepo:      movieRepo,
		showRepo:       showRepo,
		assignmentRepo: assignmentRepo,
		treeTTL:        30 * time.Second,
	}
}

// Open returns a file handle for reading
func (fs *LibraryFS) Open(filepath string) (File, error) {
	fs.ensureTree()
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	filepath = cleanPath(filepath)

	entry, exists := fs.tree.pathMap[filepath]
	if !exists {
		return nil, os.ErrNotExist
	}

	switch e := entry.(type) {
	case *VirtualDir:
		return &DirFile{dir: e}, nil
	case *PlaceholderFile:
		// Stage 1: Return placeholder (no torrent backend yet)
		return e, nil
	default:
		return nil, os.ErrNotExist
	}
}

// ReadDir returns directory contents
func (fs *LibraryFS) ReadDir(dirPath string) (map[string]File, error) {
	fs.ensureTree()
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	dirPath = cleanPath(dirPath)

	// Handle root
	if dirPath == "/" {
		result := make(map[string]File)
		for name, child := range fs.tree.root.children {
			result[name] = entryToFile(child)
		}
		return result, nil
	}

	entry, exists := fs.tree.pathMap[dirPath]
	if !exists {
		return nil, os.ErrNotExist
	}

	dir, ok := entry.(*VirtualDir)
	if !ok {
		return nil, os.ErrNotExist // Not a directory
	}

	result := make(map[string]File)
	for name, child := range dir.children {
		result[name] = entryToFile(child)
	}

	return result, nil
}

// ensureTree rebuilds tree if stale
func (fs *LibraryFS) ensureTree() {
	fs.mu.RLock()
	needsRebuild := fs.tree == nil || time.Since(fs.treeBuiltAt) > fs.treeTTL
	fs.mu.RUnlock()

	if needsRebuild {
		fs.rebuildTree()
	}
}

// rebuildTree constructs the VFS tree from database
func (fs *LibraryFS) rebuildTree() {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	tree := &DirectoryTree{
		root:    NewVirtualDir("/"),
		pathMap: make(map[string]Entry),
	}

	// Create root directories
	moviesDir := NewVirtualDir("Movies")
	tvDir := NewVirtualDir("TV Shows")
	tree.root.children["Movies"] = moviesDir
	tree.root.children["TV Shows"] = tvDir
	tree.pathMap["/Movies"] = moviesDir
	tree.pathMap["/TV Shows"] = tvDir

	// Add movies with active assignments
	movies, _ := fs.movieRepo.ListWithAssignments()
	for _, movie := range movies {
		if movie.Assignment == nil {
			continue // Hide movies without torrents
		}

		// Create movie folder: /Movies/Title (Year)/
		folderName := library.SanitizeFilename(movie.Title) + " (" + itoa(movie.Year) + ")"
		folderPath := "/Movies/" + folderName

		movieDir := NewVirtualDir(folderName)
		moviesDir.children[folderName] = movieDir
		tree.pathMap[folderPath] = movieDir

		// Add video file
		ext := path.Ext(movie.Assignment.FilePath)
		if ext == "" {
			ext = ".mkv" // Default extension
		}
		fileName := library.SanitizeFilename(movie.Title) + " (" + itoa(movie.Year) + ")" + ext
		filePath := folderPath + "/" + fileName

		videoFile := NewPlaceholderFile(fileName, movie.Assignment.FileSize, movie.Assignment)
		movieDir.children[fileName] = videoFile
		tree.pathMap[filePath] = videoFile
	}

	// Add TV shows with assigned episodes
	shows, _ := fs.showRepo.GetShowsWithAssignedEpisodes()
	for _, show := range shows {
		// Create show folder: /TV Shows/Title (Year)/
		showFolderName := library.SanitizeFilename(show.Title) + " (" + itoa(show.Year) + ")"
		showPath := "/TV Shows/" + showFolderName

		showDir := NewVirtualDir(showFolderName)
		tvDir.children[showFolderName] = showDir
		tree.pathMap[showPath] = showDir

		for _, season := range show.Seasons {
			// Create season folder: /TV Shows/Title (Year)/Season 01/
			seasonFolderName := "Season " + padZero(season.SeasonNumber, 2)
			seasonPath := showPath + "/" + seasonFolderName

			seasonDir := NewVirtualDir(seasonFolderName)
			showDir.children[seasonFolderName] = seasonDir
			tree.pathMap[seasonPath] = seasonDir

			for _, episode := range season.Episodes {
				if episode.Assignment == nil {
					continue
				}

				// Episode file: Show - S01E05 - Name.ext
				ext := path.Ext(episode.Assignment.FilePath)
				if ext == "" {
					ext = ".mkv"
				}

				episodeName := episode.Name
				if episodeName == "" {
					episodeName = "Episode " + itoa(episode.EpisodeNumber)
				}

				fileName := library.SanitizeFilename(show.Title) + " - S" +
					padZero(season.SeasonNumber, 2) + "E" + padZero(episode.EpisodeNumber, 2) +
					" - " + library.SanitizeFilename(episodeName) + ext
				filePath := seasonPath + "/" + fileName

				videoFile := NewPlaceholderFile(fileName, episode.Assignment.FileSize, episode.Assignment)
				seasonDir.children[fileName] = videoFile
				tree.pathMap[filePath] = videoFile
			}
		}
	}

	fs.tree = tree
	fs.treeBuiltAt = time.Now()
}

// InvalidateTree forces a tree rebuild on next access
func (fs *LibraryFS) InvalidateTree() {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	fs.treeBuiltAt = time.Time{} // Zero time forces rebuild
}

// VirtualDir represents a directory in the VFS
type VirtualDir struct {
	name     string
	children map[string]Entry
}

func NewVirtualDir(name string) *VirtualDir {
	return &VirtualDir{
		name:     name,
		children: make(map[string]Entry),
	}
}

func (d *VirtualDir) Name() string { return d.name }
func (d *VirtualDir) IsDir() bool  { return true }
func (d *VirtualDir) Size() int64  { return 0 }

// DirFile wraps VirtualDir as a File
type DirFile struct {
	dir *VirtualDir
}

func (f *DirFile) Name() string   { return f.dir.name }
func (f *DirFile) IsDir() bool    { return true }
func (f *DirFile) Size() int64    { return 0 }
func (f *DirFile) Read([]byte) (int, error) { return 0, os.ErrInvalid }
func (f *DirFile) ReadAt([]byte, int64) (int, error) { return 0, os.ErrInvalid }
func (f *DirFile) Close() error   { return nil }
func (f *DirFile) Stat() (os.FileInfo, error) {
	return &fileInfo{name: f.dir.name, size: 0, isDir: true, modTime: time.Now()}, nil
}

// PlaceholderFile represents a file that will be backed by a torrent (in Stage 2)
type PlaceholderFile struct {
	name       string
	size       int64
	assignment *library.TorrentAssignment
}

func NewPlaceholderFile(name string, size int64, assignment *library.TorrentAssignment) *PlaceholderFile {
	return &PlaceholderFile{
		name:       name,
		size:       size,
		assignment: assignment,
	}
}

func (f *PlaceholderFile) Name() string { return f.name }
func (f *PlaceholderFile) IsDir() bool  { return false }
func (f *PlaceholderFile) Size() int64  { return f.size }
func (f *PlaceholderFile) Read([]byte) (int, error) {
	// Stage 1: Return error (no torrent backend yet)
	return 0, os.ErrNotExist
}
func (f *PlaceholderFile) ReadAt([]byte, int64) (int, error) {
	return 0, os.ErrNotExist
}
func (f *PlaceholderFile) Close() error { return nil }
func (f *PlaceholderFile) Stat() (os.FileInfo, error) {
	return &fileInfo{name: f.name, size: f.size, isDir: false, modTime: time.Now()}, nil
}

// GetAssignment returns the torrent assignment for this file
func (f *PlaceholderFile) GetAssignment() *library.TorrentAssignment {
	return f.assignment
}

// fileInfo implements os.FileInfo
type fileInfo struct {
	name    string
	size    int64
	isDir   bool
	modTime time.Time
}

func (fi *fileInfo) Name() string       { return fi.name }
func (fi *fileInfo) Size() int64        { return fi.size }
func (fi *fileInfo) Mode() fs.FileMode  {
	if fi.isDir {
		return fs.ModeDir | 0755
	}
	return 0644
}
func (fi *fileInfo) ModTime() time.Time { return fi.modTime }
func (fi *fileInfo) IsDir() bool        { return fi.isDir }
func (fi *fileInfo) Sys() interface{}   { return nil }

// Helper functions

func cleanPath(p string) string {
	p = path.Clean(p)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return p
}

func entryToFile(e Entry) File {
	switch v := e.(type) {
	case *VirtualDir:
		return &DirFile{dir: v}
	case *PlaceholderFile:
		return v
	default:
		return nil
	}
}

func itoa(n int) string {
	return strings.TrimLeft(padZero(n, 1), "0")
}

func padZero(n, width int) string {
	s := ""
	for n > 0 || len(s) < width {
		s = string('0'+byte(n%10)) + s
		n /= 10
	}
	if s == "" {
		s = "0"
	}
	return s
}
