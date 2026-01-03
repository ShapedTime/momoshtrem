package vfs

import (
	"log/slog"
	"os"
	"path"
	"sync"
	"time"

	"github.com/shapedtime/momoshtrem/internal/common"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/torrent"
)

// LibraryFS implements Filesystem backed by the library database
type LibraryFS struct {
	mu             sync.RWMutex
	movieRepo      *library.MovieRepository
	showRepo       *library.ShowRepository
	assignmentRepo *library.AssignmentRepository

	// Torrent service for file streaming (Stage 2)
	torrentService torrent.Service
	readTimeout    time.Duration
	onActivity     func(hash string)

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
	treeTTLSeconds int,
) *LibraryFS {
	ttl := time.Duration(treeTTLSeconds) * time.Second
	if ttl <= 0 {
		ttl = 30 * time.Second // Default to 30 seconds
	}
	return &LibraryFS{
		movieRepo:      movieRepo,
		showRepo:       showRepo,
		assignmentRepo: assignmentRepo,
		treeTTL:        ttl,
	}
}

// SetTorrentService configures the torrent service for file streaming.
// This should be called after creating the LibraryFS but before serving requests.
func (fs *LibraryFS) SetTorrentService(
	svc torrent.Service,
	readTimeout time.Duration,
	onActivity func(hash string),
) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	fs.torrentService = svc
	fs.readTimeout = readTimeout
	fs.onActivity = onActivity
	slog.Info("VFS torrent service configured", "read_timeout_seconds", readTimeout.Seconds())
}

// Open returns a file handle for reading
func (fs *LibraryFS) Open(filepath string) (File, error) {
	fs.ensureTree()
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	filepath = common.CleanPath(filepath)

	entry, exists := fs.tree.pathMap[filepath]
	if !exists {
		return nil, os.ErrNotExist
	}

	switch e := entry.(type) {
	case *VirtualDir:
		return &DirFile{dir: e}, nil
	case *PlaceholderFile:
		// If torrent service is available and file has assignment, return real torrent file
		if fs.torrentService != nil && e.assignment != nil {
			return fs.openTorrentFile(e)
		}
		// Fallback: return placeholder (Stage 1 behavior)
		return e, nil
	default:
		return nil, os.ErrNotExist
	}
}

// openTorrentFile creates a TorrentFile for streaming from a PlaceholderFile.
func (fs *LibraryFS) openTorrentFile(pf *PlaceholderFile) (File, error) {
	assignment := pf.assignment

	// Ensure torrent is loaded (lazy loading via GetOrAddTorrent)
	_, err := fs.torrentService.GetOrAddTorrent(assignment.MagnetURI)
	if err != nil {
		slog.Error("Failed to load torrent for file",
			"info_hash", assignment.InfoHash,
			"file_path", assignment.FilePath,
			"error", err,
		)
		return nil, err
	}

	// Get the specific file handle from the torrent
	handle, err := fs.torrentService.GetFile(assignment.InfoHash, assignment.FilePath)
	if err != nil {
		slog.Error("Failed to get file from torrent",
			"info_hash", assignment.InfoHash,
			"file_path", assignment.FilePath,
			"error", err,
		)
		return nil, err
	}

	return NewTorrentFile(
		handle,
		pf.name,
		assignment.InfoHash,
		fs.readTimeout,
		fs.onActivity,
	), nil
}

// ReadDir returns directory contents
func (fs *LibraryFS) ReadDir(dirPath string) (map[string]File, error) {
	fs.ensureTree()
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	dirPath = common.CleanPath(dirPath)

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
	movies, err := fs.movieRepo.ListWithAssignments()
	if err != nil {
		slog.Error("Failed to list movies for VFS", "error", err)
	}
	for _, movie := range movies {
		if movie.Assignment == nil {
			continue // Hide movies without torrents
		}

		// Create movie folder: /Movies/Title (Year)/
		folderName := library.SanitizeFilename(movie.Title) + " (" + common.Itoa(movie.Year) + ")"
		folderPath := "/Movies/" + folderName

		movieDir := NewVirtualDir(folderName)
		moviesDir.children[folderName] = movieDir
		tree.pathMap[folderPath] = movieDir

		// Add video file
		ext := path.Ext(movie.Assignment.FilePath)
		if ext == "" {
			ext = ".mkv" // Default extension
		}
		fileName := library.SanitizeFilename(movie.Title) + " (" + common.Itoa(movie.Year) + ")" + ext
		filePath := folderPath + "/" + fileName

		videoFile := NewPlaceholderFile(fileName, movie.Assignment.FileSize, movie.Assignment)
		movieDir.children[fileName] = videoFile
		tree.pathMap[filePath] = videoFile
	}

	// Add TV shows with assigned episodes
	shows, err := fs.showRepo.GetShowsWithAssignedEpisodes()
	if err != nil {
		slog.Error("Failed to list shows for VFS", "error", err)
	}
	for _, show := range shows {
		// Create show folder: /TV Shows/Title (Year)/
		showFolderName := library.SanitizeFilename(show.Title) + " (" + common.Itoa(show.Year) + ")"
		showPath := "/TV Shows/" + showFolderName

		showDir := NewVirtualDir(showFolderName)
		tvDir.children[showFolderName] = showDir
		tree.pathMap[showPath] = showDir

		for _, season := range show.Seasons {
			// Create season folder: /TV Shows/Title (Year)/Season 01/
			seasonFolderName := "Season " + common.PadZero(season.SeasonNumber, 2)
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
					episodeName = "Episode " + common.Itoa(episode.EpisodeNumber)
				}

				fileName := library.SanitizeFilename(show.Title) + " - S" +
					common.PadZero(season.SeasonNumber, 2) + "E" + common.PadZero(episode.EpisodeNumber, 2) +
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
	return common.NewFileInfo(f.dir.name, 0, true, time.Now()), nil
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
	return common.NewFileInfo(f.name, f.size, false, time.Now()), nil
}

// GetAssignment returns the torrent assignment for this file
func (f *PlaceholderFile) GetAssignment() *library.TorrentAssignment {
	return f.assignment
}

// Helper functions

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
