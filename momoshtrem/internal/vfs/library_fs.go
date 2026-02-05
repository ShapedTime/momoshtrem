package vfs

import (
	"context"
	"log/slog"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/shapedtime/momoshtrem/internal/common"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/streaming"
	"github.com/shapedtime/momoshtrem/internal/subtitle"
	"github.com/shapedtime/momoshtrem/internal/torrent"
)

// VFS path and file constants
const (
	DefaultVideoExt = ".mkv"
	MoviesPath      = "/Movies"
	TVShowsPath     = "/TV Shows"
)

// makeMediaFolderName creates a folder name for movies or shows: "Title (Year)"
func makeMediaFolderName(title string, year int) string {
	return library.SanitizeFilename(title) + " (" + common.Itoa(year) + ")"
}

// makeSeasonFolderName creates a season folder name: "Season 01"
func makeSeasonFolderName(seasonNum int) string {
	return "Season " + common.PadZero(seasonNum, 2)
}

// makeEpisodeFileName creates an episode filename: "Show - S01E05 - Episode Name.ext"
func makeEpisodeFileName(showTitle string, seasonNum, epNum int, epName, ext string) string {
	if epName == "" {
		epName = "Episode " + common.Itoa(epNum)
	}
	return library.SanitizeFilename(showTitle) + " - S" +
		common.PadZero(seasonNum, 2) + "E" + common.PadZero(epNum, 2) +
		" - " + library.SanitizeFilename(epName) + ext
}

// makeEpisodePrefix creates the prefix for matching episodes: "Show - S01E05"
func makeEpisodePrefix(showTitle string, seasonNum, epNum int) string {
	return library.SanitizeFilename(showTitle) + " - S" +
		common.PadZero(seasonNum, 2) + "E" + common.PadZero(epNum, 2)
}

// getVideoExt extracts video extension from path, defaulting to .mkv
func getVideoExt(filePath string) string {
	ext := path.Ext(filePath)
	if ext == "" {
		return DefaultVideoExt
	}
	return ext
}

// LibraryFS implements Filesystem backed by the library database
type LibraryFS struct {
	mu             sync.RWMutex
	movieRepo      *library.MovieRepository
	showRepo       *library.ShowRepository
	assignmentRepo *library.AssignmentRepository
	subtitleRepo   *subtitle.Repository

	// Torrent service for file streaming (Stage 2)
	torrentService    torrent.Service
	readTimeout       time.Duration
	onActivity        func(hash string)
	waitForActivation func(hash string, timeout time.Duration) error

	// Streaming optimization config (Stage 3)
	streamingCfg streaming.Config

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
	waitForActivation func(hash string, timeout time.Duration) error,
	streamingCfg streaming.Config,
) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	fs.torrentService = svc
	fs.readTimeout = readTimeout
	fs.onActivity = onActivity
	fs.waitForActivation = waitForActivation
	fs.streamingCfg = streamingCfg
	slog.Info("VFS torrent service configured",
		"read_timeout_seconds", readTimeout.Seconds(),
		"header_priority_mb", streamingCfg.HeaderPriorityBytes/(1024*1024),
		"readahead_mb", streamingCfg.ReadaheadBytes/(1024*1024),
	)
}

// SetSubtitleRepository configures subtitle support for the VFS.
func (fs *LibraryFS) SetSubtitleRepository(repo *subtitle.Repository) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	fs.subtitleRepo = repo
	slog.Info("VFS subtitle repository configured")
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
	case *SubtitleFile:
		// Subtitle files are backed by local storage
		return e, nil
	case *TorrentSubtitleFile:
		// Torrent-embedded subtitle: stream from torrent
		if fs.torrentService != nil {
			return fs.openTorrentSubtitleFile(e)
		}
		return nil, os.ErrNotExist
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
		fs.waitForActivation,
		fs.streamingCfg,
	), nil
}

// openTorrentSubtitleFile creates a TorrentFile for streaming a subtitle from a torrent.
func (fs *LibraryFS) openTorrentSubtitleFile(tsf *TorrentSubtitleFile) (File, error) {
	// Look up magnet_uri from torrent_assignments using the info_hash
	assignments, err := fs.assignmentRepo.GetByInfoHash(tsf.infoHash)
	if err != nil || len(assignments) == 0 {
		slog.Error("Failed to find torrent assignment for subtitle",
			"info_hash", tsf.infoHash,
			"subtitle_path", tsf.torrentPath,
			"error", err,
		)
		return nil, os.ErrNotExist
	}

	// Use the first assignment's magnet URI (all assignments for same hash have same magnet)
	magnetURI := assignments[0].MagnetURI

	// Ensure torrent is loaded (lazy loading via GetOrAddTorrent)
	_, err = fs.torrentService.GetOrAddTorrent(magnetURI)
	if err != nil {
		slog.Error("Failed to load torrent for subtitle",
			"info_hash", tsf.infoHash,
			"subtitle_path", tsf.torrentPath,
			"error", err,
		)
		return nil, err
	}

	// Get the specific file handle from the torrent
	handle, err := fs.torrentService.GetFile(tsf.infoHash, tsf.torrentPath)
	if err != nil {
		slog.Error("Failed to get subtitle file from torrent",
			"info_hash", tsf.infoHash,
			"subtitle_path", tsf.torrentPath,
			"error", err,
		)
		return nil, err
	}

	return NewTorrentFile(
		handle,
		tsf.name,
		tsf.infoHash,
		fs.readTimeout,
		fs.onActivity,
		fs.waitForActivation,
		fs.streamingCfg,
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

	// Add root to pathMap so Open("/") works for WebDAV PROPFIND
	tree.pathMap["/"] = tree.root

	// Create root directories
	moviesDir := NewVirtualDir("Movies")
	tvDir := NewVirtualDir("TV Shows")
	tree.root.children["Movies"] = moviesDir
	tree.root.children["TV Shows"] = tvDir
	tree.pathMap[MoviesPath] = moviesDir
	tree.pathMap[TVShowsPath] = tvDir

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
		folderName := makeMediaFolderName(movie.Title, movie.Year)
		folderPath := MoviesPath + "/" + folderName

		movieDir := NewVirtualDir(folderName)
		moviesDir.children[folderName] = movieDir
		tree.pathMap[folderPath] = movieDir

		// Add video file
		ext := getVideoExt(movie.Assignment.FilePath)
		fileName := folderName + ext
		filePath := folderPath + "/" + fileName

		videoFile := NewPlaceholderFile(fileName, movie.Assignment.FileSize, movie.Assignment)
		movieDir.children[fileName] = videoFile
		tree.pathMap[filePath] = videoFile

		// Add subtitle files for this movie
		fs.addSubtitlesToDir(tree, movieDir, folderPath, folderName, subtitle.ItemTypeMovie, movie.ID)
	}

	// Add TV shows with assigned episodes
	shows, err := fs.showRepo.GetShowsWithAssignedEpisodes()
	if err != nil {
		slog.Error("Failed to list shows for VFS", "error", err)
	}
	for _, show := range shows {
		// Create show folder: /TV Shows/Title (Year)/
		showFolderName := makeMediaFolderName(show.Title, show.Year)
		showPath := TVShowsPath + "/" + showFolderName

		showDir := NewVirtualDir(showFolderName)
		tvDir.children[showFolderName] = showDir
		tree.pathMap[showPath] = showDir

		for _, season := range show.Seasons {
			// Create season folder: /TV Shows/Title (Year)/Season 01/
			seasonFolderName := makeSeasonFolderName(season.SeasonNumber)
			seasonPath := showPath + "/" + seasonFolderName

			seasonDir := NewVirtualDir(seasonFolderName)
			showDir.children[seasonFolderName] = seasonDir
			tree.pathMap[seasonPath] = seasonDir

			for _, episode := range season.Episodes {
				if episode.Assignment == nil {
					continue
				}

				// Episode file: Show - S01E05 - Name.ext
				ext := getVideoExt(episode.Assignment.FilePath)
				fileName := makeEpisodeFileName(show.Title, season.SeasonNumber, episode.EpisodeNumber, episode.Name, ext)
				filePath := seasonPath + "/" + fileName

				videoFile := NewPlaceholderFile(fileName, episode.Assignment.FileSize, episode.Assignment)
				seasonDir.children[fileName] = videoFile
				tree.pathMap[filePath] = videoFile

				// Add subtitle files for this episode
				videoBaseName := strings.TrimSuffix(fileName, ext)
				fs.addSubtitlesToDir(tree, seasonDir, seasonPath, videoBaseName, subtitle.ItemTypeEpisode, episode.ID)
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

// AddMovieToTree adds a movie with its assignment to the VFS tree.
// If the tree hasn't been built yet, this is a no-op.
func (fs *LibraryFS) AddMovieToTree(movie *library.Movie, assignment *library.TorrentAssignment) {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	if fs.tree == nil {
		return // Tree not built yet, will be included on first build
	}

	// Build paths
	folderName := makeMediaFolderName(movie.Title, movie.Year)
	folderPath := MoviesPath + "/" + folderName

	moviesDir, ok := fs.tree.pathMap[MoviesPath].(*VirtualDir)
	if !ok {
		slog.Error("Movies directory not found in tree")
		return
	}

	// Check if movie folder already exists (re-assignment case)
	if existing, exists := fs.tree.pathMap[folderPath]; exists {
		// Update existing folder with new file
		if dir, ok := existing.(*VirtualDir); ok {
			// Remove old file if any
			for name := range dir.children {
				delete(fs.tree.pathMap, folderPath+"/"+name)
			}
			dir.children = make(map[string]Entry)
		}
	} else {
		// Create new movie folder
		movieDir := NewVirtualDir(folderName)
		moviesDir.children[folderName] = movieDir
		fs.tree.pathMap[folderPath] = movieDir
	}

	// Add video file
	movieDir, ok := fs.tree.pathMap[folderPath].(*VirtualDir)
	if !ok {
		slog.Error("Movie directory not found after creation", "path", folderPath)
		return
	}
	ext := getVideoExt(assignment.FilePath)
	fileName := folderName + ext
	filePath := folderPath + "/" + fileName

	videoFile := NewPlaceholderFile(fileName, assignment.FileSize, assignment)
	movieDir.children[fileName] = videoFile
	fs.tree.pathMap[filePath] = videoFile

	slog.Debug("Added movie to VFS tree", "path", filePath)
}

// RemoveMovieFromTree removes a movie folder and its contents from the VFS tree.
// If the tree hasn't been built yet, this is a no-op.
func (fs *LibraryFS) RemoveMovieFromTree(title string, year int) {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	if fs.tree == nil {
		return
	}

	folderName := makeMediaFolderName(title, year)
	folderPath := MoviesPath + "/" + folderName

	movieDir, exists := fs.tree.pathMap[folderPath]
	if !exists {
		return // Already removed or never existed
	}

	// Remove all children from pathMap
	if dir, ok := movieDir.(*VirtualDir); ok {
		for name := range dir.children {
			delete(fs.tree.pathMap, folderPath+"/"+name)
		}
	}

	// Remove folder from pathMap
	delete(fs.tree.pathMap, folderPath)

	// Remove from parent's children
	if moviesDir, ok := fs.tree.pathMap[MoviesPath].(*VirtualDir); ok {
		delete(moviesDir.children, folderName)
	}

	slog.Debug("Removed movie from VFS tree", "path", folderPath)
}

// AddEpisodesToTree adds episodes to the VFS tree, creating show/season folders as needed.
// If the tree hasn't been built yet, this is a no-op.
func (fs *LibraryFS) AddEpisodesToTree(episodes []EpisodeWithContext) {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	if fs.tree == nil || len(episodes) == 0 {
		return
	}

	tvDir, ok := fs.tree.pathMap[TVShowsPath].(*VirtualDir)
	if !ok {
		slog.Error("TV Shows directory not found in tree")
		return
	}

	for _, ep := range episodes {
		// Get or create show folder
		showFolderName := makeMediaFolderName(ep.ShowTitle, ep.ShowYear)
		showPath := TVShowsPath + "/" + showFolderName

		showDirEntry, exists := fs.tree.pathMap[showPath]
		if !exists {
			showDirEntry = NewVirtualDir(showFolderName)
			tvDir.children[showFolderName] = showDirEntry
			fs.tree.pathMap[showPath] = showDirEntry
		}
		showDir, ok := showDirEntry.(*VirtualDir)
		if !ok {
			slog.Error("Show directory type assertion failed", "path", showPath)
			continue
		}

		// Get or create season folder
		seasonFolderName := makeSeasonFolderName(ep.SeasonNumber)
		seasonPath := showPath + "/" + seasonFolderName

		seasonDirEntry, exists := fs.tree.pathMap[seasonPath]
		if !exists {
			seasonDirEntry = NewVirtualDir(seasonFolderName)
			showDir.children[seasonFolderName] = seasonDirEntry
			fs.tree.pathMap[seasonPath] = seasonDirEntry
		}
		seasonDir, ok := seasonDirEntry.(*VirtualDir)
		if !ok {
			slog.Error("Season directory type assertion failed", "path", seasonPath)
			continue
		}

		// Add episode file
		ext := getVideoExt(ep.Assignment.FilePath)
		fileName := makeEpisodeFileName(ep.ShowTitle, ep.SeasonNumber, ep.Episode.EpisodeNumber, ep.Episode.Name, ext)
		filePath := seasonPath + "/" + fileName

		videoFile := NewPlaceholderFile(fileName, ep.Assignment.FileSize, ep.Assignment)
		seasonDir.children[fileName] = videoFile
		fs.tree.pathMap[filePath] = videoFile

		slog.Debug("Added episode to VFS tree", "path", filePath)
	}
}

// RemoveEpisodeFromTree removes an episode file and cleans up empty parent folders.
// If the tree hasn't been built yet, this is a no-op.
func (fs *LibraryFS) RemoveEpisodeFromTree(showTitle string, showYear int, seasonNumber int, episodeNumber int) {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	if fs.tree == nil {
		return
	}

	tvDir, ok := fs.tree.pathMap[TVShowsPath].(*VirtualDir)
	if !ok {
		return
	}

	showFolderName := makeMediaFolderName(showTitle, showYear)
	showPath := TVShowsPath + "/" + showFolderName

	showDirEntry, exists := fs.tree.pathMap[showPath]
	if !exists {
		return
	}
	showDir, ok := showDirEntry.(*VirtualDir)
	if !ok {
		return
	}

	seasonFolderName := makeSeasonFolderName(seasonNumber)
	seasonPath := showPath + "/" + seasonFolderName

	seasonDirEntry, exists := fs.tree.pathMap[seasonPath]
	if !exists {
		return
	}
	seasonDir, ok := seasonDirEntry.(*VirtualDir)
	if !ok {
		return
	}

	// Find and remove the episode file (match by episode number since extension may vary)
	prefix := makeEpisodePrefix(showTitle, seasonNumber, episodeNumber)

	var fileNameToRemove string
	for name := range seasonDir.children {
		if strings.HasPrefix(name, prefix) {
			fileNameToRemove = name
			break
		}
	}

	if fileNameToRemove == "" {
		return // File not found
	}

	filePath := seasonPath + "/" + fileNameToRemove
	delete(fs.tree.pathMap, filePath)
	delete(seasonDir.children, fileNameToRemove)

	slog.Debug("Removed episode from VFS tree", "path", filePath)

	// Cleanup empty season folder
	if len(seasonDir.children) == 0 {
		delete(fs.tree.pathMap, seasonPath)
		delete(showDir.children, seasonFolderName)

		// Cleanup empty show folder
		if len(showDir.children) == 0 {
			delete(fs.tree.pathMap, showPath)
			delete(tvDir.children, showFolderName)
		}
	}
}

// RemoveShowFromTree removes an entire show subtree from the VFS tree.
// If the tree hasn't been built yet, this is a no-op.
func (fs *LibraryFS) RemoveShowFromTree(title string, year int) {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	if fs.tree == nil {
		return
	}

	tvDir, ok := fs.tree.pathMap[TVShowsPath].(*VirtualDir)
	if !ok {
		return
	}

	showFolderName := makeMediaFolderName(title, year)
	showPath := TVShowsPath + "/" + showFolderName

	showDirEntry, exists := fs.tree.pathMap[showPath]
	if !exists {
		return
	}
	showDir, ok := showDirEntry.(*VirtualDir)
	if !ok {
		return
	}

	// Remove all files and seasons from pathMap
	for seasonName, seasonEntry := range showDir.children {
		seasonPath := showPath + "/" + seasonName
		if seasonDir, ok := seasonEntry.(*VirtualDir); ok {
			for fileName := range seasonDir.children {
				delete(fs.tree.pathMap, seasonPath+"/"+fileName)
			}
		}
		delete(fs.tree.pathMap, seasonPath)
	}

	// Remove show folder
	delete(fs.tree.pathMap, showPath)
	delete(tvDir.children, showFolderName)

	slog.Debug("Removed show from VFS tree", "path", showPath)
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

// SubtitleFile represents a subtitle file backed by local storage
type SubtitleFile struct {
	name      string
	localPath string
	size      int64
	file      *os.File // Opened file handle
}

func NewSubtitleFile(name, localPath string, size int64) *SubtitleFile {
	return &SubtitleFile{
		name:      name,
		localPath: localPath,
		size:      size,
	}
}

func (f *SubtitleFile) Name() string { return f.name }
func (f *SubtitleFile) IsDir() bool  { return false }
func (f *SubtitleFile) Size() int64  { return f.size }

// ensureOpen lazily opens the underlying file if not already open.
func (f *SubtitleFile) ensureOpen() error {
	if f.file != nil {
		return nil
	}
	file, err := os.Open(f.localPath)
	if err != nil {
		return err
	}
	f.file = file
	return nil
}

func (f *SubtitleFile) Read(p []byte) (int, error) {
	if err := f.ensureOpen(); err != nil {
		return 0, err
	}
	return f.file.Read(p)
}

func (f *SubtitleFile) ReadAt(p []byte, off int64) (int, error) {
	if err := f.ensureOpen(); err != nil {
		return 0, err
	}
	return f.file.ReadAt(p, off)
}

func (f *SubtitleFile) Seek(offset int64, whence int) (int64, error) {
	if err := f.ensureOpen(); err != nil {
		return 0, err
	}
	return f.file.Seek(offset, whence)
}

func (f *SubtitleFile) Close() error {
	if f.file != nil {
		err := f.file.Close()
		f.file = nil
		return err
	}
	return nil
}

func (f *SubtitleFile) Stat() (os.FileInfo, error) {
	return common.NewFileInfo(f.name, f.size, false, time.Now()), nil
}

// TorrentSubtitleFile represents a subtitle file embedded in a torrent
type TorrentSubtitleFile struct {
	name        string
	torrentPath string // Path within the torrent
	size        int64
	infoHash    string
}

func NewTorrentSubtitleFile(name, torrentPath string, size int64, infoHash string) *TorrentSubtitleFile {
	return &TorrentSubtitleFile{
		name:        name,
		torrentPath: torrentPath,
		size:        size,
		infoHash:    infoHash,
	}
}

func (f *TorrentSubtitleFile) Name() string { return f.name }
func (f *TorrentSubtitleFile) IsDir() bool  { return false }
func (f *TorrentSubtitleFile) Size() int64  { return f.size }

func (f *TorrentSubtitleFile) Stat() (os.FileInfo, error) {
	return common.NewFileInfo(f.name, f.size, false, time.Now()), nil
}

// makeSubtitleFileName creates a subtitle filename: "VideoName.lang.format"
func makeSubtitleFileName(videoBaseName, langCode, format string) string {
	return videoBaseName + "." + langCode + "." + format
}

// addSubtitlesToDir adds subtitle files for a media item to the given directory.
// This is a helper to avoid duplicate code for movies and episodes.
func (fs *LibraryFS) addSubtitlesToDir(tree *DirectoryTree, dir *VirtualDir, dirPath, videoBaseName string, itemType subtitle.ItemType, itemID int64) {
	if fs.subtitleRepo == nil {
		return
	}

	subtitles, err := fs.subtitleRepo.GetByItem(context.Background(), itemType, itemID)
	if err != nil {
		slog.Error("Failed to get subtitles", "item_type", itemType, "item_id", itemID, "error", err)
		return
	}

	for _, sub := range subtitles {
		subFileName := makeSubtitleFileName(videoBaseName, sub.LanguageCode, sub.Format)
		subFilePath := dirPath + "/" + subFileName

		var subFile Entry
		if sub.Source == subtitle.SourceTorrent {
			// Torrent-embedded subtitle: will be streamed from torrent
			subFile = NewTorrentSubtitleFile(subFileName, sub.FilePath, sub.FileSize, sub.InfoHash)
		} else {
			// Local subtitle: backed by local storage (OpenSubtitles download)
			subFile = NewSubtitleFile(subFileName, sub.FilePath, sub.FileSize)
		}

		dir.children[subFileName] = subFile
		tree.pathMap[subFilePath] = subFile
	}
}

// Helper functions

func entryToFile(e Entry) File {
	switch v := e.(type) {
	case *VirtualDir:
		return &DirFile{dir: v}
	case *PlaceholderFile:
		return v
	case *SubtitleFile:
		return v
	case *TorrentSubtitleFile:
		// Note: TorrentSubtitleFile needs to be opened via LibraryFS.Open() to get actual torrent file
		return nil
	default:
		return nil
	}
}
