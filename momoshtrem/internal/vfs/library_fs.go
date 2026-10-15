package vfs

import (
	"log/slog"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/shapedtime/momoshtrem/internal/common"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/metrics"
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

// makeMultiEpisodeFileName names a file that plays several episodes, after
// the first: "Show - S01E01-E02 - First Name.ext".
func makeMultiEpisodeFileName(showTitle string, seasonNum, firstEp, lastEp int, firstName, ext string) string {
	if firstName == "" {
		firstName = "Episode " + common.Itoa(firstEp)
	}
	return library.SanitizeFilename(showTitle) + " - S" +
		common.PadZero(seasonNum, 2) + "E" + common.PadZero(firstEp, 2) + "-E" + common.PadZero(lastEp, 2) +
		" - " + library.SanitizeFilename(firstName) + ext
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
	subtitleRepo   subtitleLister

	// Torrent service for file streaming (Stage 2)
	torrentService    torrent.Service
	readTimeout       time.Duration
	onActivity        func(hash string)
	waitForActivation func(hash string, timeout time.Duration) error

	// Streaming optimization config (Stage 3)
	streamingCfg streaming.Config

	// Prometheus streaming metrics (nil when metrics disabled)
	metrics *metrics.Metrics

	// Reader tracking for debug endpoint (optional)
	readerTracker *streaming.ReaderTracker

	// Cached tree structure
	tree       *DirectoryTree
	rebuilding sync.Mutex // Coordinates rebuild operations to prevent concurrent rebuilds
	cacheDir   string     // Directory for persistent VFS cache (optional)
}

// DirectoryTree represents the virtual directory structure
type DirectoryTree struct {
	root    *VirtualDir
	pathMap map[string]Entry // Fast lookup by path
}

// newEmptyTree creates a DirectoryTree with root and standard directories (Movies, TV Shows).
// Used by both buildTreeFromDB and loadTreeFromCache to avoid duplication.
func newEmptyTree() (*DirectoryTree, *VirtualDir, *VirtualDir) {
	tree := &DirectoryTree{
		root:    NewVirtualDir("/"),
		pathMap: make(map[string]Entry),
	}
	tree.pathMap["/"] = tree.root

	moviesDir := NewVirtualDir("Movies")
	tvDir := NewVirtualDir("TV Shows")
	tree.root.children["Movies"] = moviesDir
	tree.root.children["TV Shows"] = tvDir
	tree.pathMap[MoviesPath] = moviesDir
	tree.pathMap[TVShowsPath] = tvDir

	return tree, moviesDir, tvDir
}

// Entry represents an entry in the virtual filesystem
type Entry interface {
	Name() string
	IsDir() bool
	Size() int64
}

// NewLibraryFS creates a new library-backed filesystem.
// Note: treeTTLSeconds parameter is deprecated and ignored - updates are now event-driven.
func NewLibraryFS(
	movieRepo *library.MovieRepository,
	showRepo *library.ShowRepository,
	assignmentRepo *library.AssignmentRepository,
	treeTTLSeconds int,
) *LibraryFS {
	if treeTTLSeconds > 0 {
		slog.Warn("VFS tree_ttl is deprecated - updates are now event-driven",
			"configured_ttl_seconds", treeTTLSeconds)
	}
	return &LibraryFS{
		movieRepo:      movieRepo,
		showRepo:       showRepo,
		assignmentRepo: assignmentRepo,
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
func (fs *LibraryFS) SetSubtitleRepository(repo subtitleLister) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	fs.subtitleRepo = repo
	slog.Info("VFS subtitle repository configured")
}

// SetCacheDir configures the directory for persistent VFS tree caching.
// When set, the tree will be saved to disk after rebuilds and loaded on startup.
func (fs *LibraryFS) SetCacheDir(dir string) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	fs.cacheDir = dir
	if dir != "" {
		slog.Info("VFS cache directory configured", "path", dir)
	}
}

// SetMetrics configures Prometheus streaming metrics for the VFS.
func (fs *LibraryFS) SetMetrics(m *metrics.Metrics) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	fs.metrics = m
	slog.Info("VFS metrics configured")
}

// SetReaderTracker configures reader tracking for debug visualization.
func (fs *LibraryFS) SetReaderTracker(rt *streaming.ReaderTracker) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	fs.readerTracker = rt
}

// Stat returns file metadata without creating a TorrentFile.
// This avoids the overhead of Open() → openTorrentFile() → Close() for
// WebDAV PROPFIND/HEAD requests that only need name/size/isDir.
func (fs *LibraryFS) Stat(filepath string) (os.FileInfo, error) {
	fs.ensureTree()
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	filepath = common.CleanPath(filepath)

	entry, exists := fs.tree.pathMap[filepath]
	if !exists {
		return nil, os.ErrNotExist
	}

	return common.NewFileInfo(entry.Name(), entry.Size(), entry.IsDir(), time.Now()), nil
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
		// Subtitle files are backed by local storage. A handle per open:
		// the tree entry is shared by every request.
		return NewSubtitleFile(e.name, e.localPath, e.size), nil
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

// openTorrentFile creates a new TorrentFile for streaming.
// Each WebDAV connection gets its own reader — multiple anacrolix readers
// on the same torrent share piece storage and deduplicate piece requests,
// so parallel readers at different positions (header + footer probing)
// are more efficient than a single reader thrashing between positions.
func (fs *LibraryFS) openTorrentFile(pf *PlaceholderFile) (File, error) {
	assignment := pf.assignment

	_, err := fs.torrentService.GetOrAddTorrent(assignment.MagnetURI)
	if err != nil {
		slog.Error("Failed to load torrent for file",
			"info_hash", assignment.InfoHash,
			"file_path", assignment.FilePath,
			"error", err,
		)
		return nil, err
	}

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
		fs.metrics,
		fs.readerTracker,
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
		fs.metrics,
		fs.readerTracker,
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

// ensureTree builds tree on first access only.
// Updates are event-driven via TreeUpdater methods, no periodic rebuilds.
// On first access, tries to load from persistent cache before rebuilding.
func (fs *LibraryFS) ensureTree() {
	fs.mu.RLock()
	needsBuild := fs.tree == nil
	cacheDir := fs.cacheDir
	fs.mu.RUnlock()

	if needsBuild {
		// Try loading from persistent cache first
		if cacheDir != "" {
			if err := fs.loadTreeFromCache(); err == nil {
				return // Successfully loaded from cache
			}
			// Cache miss or invalid - fall through to rebuild
		}
		fs.rebuildTree()
	}
}

// rebuildTree constructs the VFS tree from database.
// Uses lock-free building with atomic swap for minimal lock duration.
// Callers block if another rebuild is in progress (ensures tree is non-nil on return).
func (fs *LibraryFS) rebuildTree() {
	// Serialize rebuilds - callers wait rather than skip
	fs.rebuilding.Lock()
	defer fs.rebuilding.Unlock()

	// Double-check: tree may have been built while we waited for the lock
	fs.mu.RLock()
	alreadyBuilt := fs.tree != nil
	fs.mu.RUnlock()
	if alreadyBuilt {
		return
	}

	// Build tree completely outside the RWMutex lock
	tree := fs.buildTreeFromDB()

	// Atomic swap - only lock for pointer assignment
	fs.mu.Lock()
	fs.tree = tree
	cacheDir := fs.cacheDir
	fs.mu.Unlock()

	slog.Debug("VFS tree rebuilt", "entries", len(tree.pathMap))

	// Save to persistent cache (synchronous to avoid race with DeleteCache)
	if cacheDir != "" {
		fs.saveTreeToCache()
	}
}

// buildTreeFromDB constructs the VFS tree from database without holding locks.
// This allows concurrent reads to continue during tree construction.
func (fs *LibraryFS) buildTreeFromDB() *DirectoryTree {
	tree, moviesDir, tvDir := newEmptyTree()
	var subDirs []subtitleDir // Subtitles are added after the walk, in one query

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
		folderPath := MovieDir(movie.Title, movie.Year)
		folderName := path.Base(folderPath)

		movieDir := NewVirtualDir(folderName)
		moviesDir.children[folderName] = movieDir
		tree.pathMap[folderPath] = movieDir

		// Add video file
		filePath := MovieFilePath(movie.Title, movie.Year, movie.Assignment.FilePath)
		fileName := path.Base(filePath)

		videoFile := NewPlaceholderFile(fileName, movie.Assignment.FileSize, movie.Assignment)
		movieDir.children[fileName] = videoFile
		tree.pathMap[filePath] = videoFile

		subDirs = append(subDirs, subtitleDir{dir: movieDir, dirPath: folderPath, base: folderName,
			item: subtitle.Item{Type: subtitle.ItemTypeMovie, ID: movie.ID}, infoHash: movie.Assignment.InfoHash})
	}

	// Add TV shows with assigned episodes
	shows, err := fs.showRepo.GetShowsWithAssignedEpisodes()
	if err != nil {
		slog.Error("Failed to list shows for VFS", "error", err)
	}
	for _, show := range shows {
		// Create show folder: /TV Shows/Title (Year)/
		showPath := ShowDir(show.Title, show.Year)
		showFolderName := path.Base(showPath)

		showDir := NewVirtualDir(showFolderName)
		tvDir.children[showFolderName] = showDir
		tree.pathMap[showPath] = showDir

		for _, season := range show.Seasons {
			// Create season folder: /TV Shows/Title (Year)/Season 01/
			seasonPath := SeasonDir(show.Title, show.Year, season.SeasonNumber)
			seasonFolderName := path.Base(seasonPath)

			seasonDir := NewVirtualDir(seasonFolderName)
			showDir.children[seasonFolderName] = seasonDir
			tree.pathMap[seasonPath] = seasonDir

			subDirs = append(subDirs, addSeasonEntries(tree, seasonDir, seasonPath, show.Title, season)...)
		}
	}

	subs := fs.loadSubtitles(subDirs)
	for _, d := range subDirs {
		addSubtitleFiles(tree, d, d.subtitles(subs))
	}

	return tree
}

// addSeasonEntries puts a season's assigned episodes in its folder, named
// by SeasonEntries: "Show - S01E05 - Name.ext", or one "Show - S01E01-E02 -
// Name.ext" for episodes a job mapped to the same file. It returns where
// each video's subtitles go: a grouped entry gets every one of its
// episodes' subtitles, named after it.
func addSeasonEntries(tree *DirectoryTree, seasonDir *VirtualDir, seasonPath, showTitle string, season library.Season) []subtitleDir {
	var assigned []library.Episode
	var eps []SeasonEpisode
	for _, ep := range season.Episodes {
		if ep.Assignment == nil {
			continue
		}
		assigned = append(assigned, ep)
		eps = append(eps, SeasonEpisode{
			Number: ep.EpisodeNumber, Name: ep.Name,
			InfoHash: ep.Assignment.InfoHash, FilePath: ep.Assignment.FilePath,
			Grouped: ep.Assignment.Groupable(),
		})
	}

	// A grouped file split at a gap is several entries: each gets the
	// file's torrent subtitles, wherever Apply recorded them.
	type fileKey struct{ hash, path string }
	byFile := map[fileKey][]int{}
	for i, ep := range eps {
		if ep.Grouped {
			key := fileKey{ep.InfoHash, ep.FilePath}
			byFile[key] = append(byFile[key], i)
		}
	}

	var dirs []subtitleDir
	for _, entry := range SeasonEntries(showTitle, season.SeasonNumber, eps) {
		first := assigned[entry.Episodes[0]]
		filePath := seasonPath + "/" + entry.FileName
		videoFile := NewPlaceholderFile(entry.FileName, first.Assignment.FileSize, first.Assignment)
		seasonDir.children[entry.FileName] = videoFile
		tree.pathMap[filePath] = videoFile

		d := subtitleDir{
			dir: seasonDir, dirPath: seasonPath,
			base:     strings.TrimSuffix(entry.FileName, getVideoExt(first.Assignment.FilePath)),
			item:     subtitle.Item{Type: subtitle.ItemTypeEpisode, ID: first.ID},
			infoHash: first.Assignment.InfoHash,
		}
		inEntry := map[int]bool{}
		for _, i := range entry.Episodes {
			inEntry[i] = true
		}
		for _, i := range entry.Episodes[1:] {
			d.more = append(d.more, subtitle.Item{Type: subtitle.ItemTypeEpisode, ID: assigned[i].ID})
		}
		if lead := eps[entry.Episodes[0]]; lead.Grouped {
			for _, i := range byFile[fileKey{lead.InfoHash, lead.FilePath}] {
				if !inEntry[i] {
					d.sameFile = append(d.sameFile, subtitle.Item{Type: subtitle.ItemTypeEpisode, ID: assigned[i].ID})
				}
			}
		}
		dirs = append(dirs, d)
	}
	return dirs
}

// InvalidateFileHandles is a no-op retained for the TreeUpdater interface.
// With per-connection readers (no shared cache), there are no cached handles
// to invalidate — each reader is owned by its HTTP request and closed when
// the request completes.
func (fs *LibraryFS) InvalidateFileHandles(infoHash string) {}

// ensureTreeForMutation guarantees the in-memory tree reflects current DB
// state before a targeted mutation runs. If the tree hasn't been loaded yet,
// it rebuilds from the DB (bypassing the on-disk cache, which may be stale)
// and returns true to signal the caller that the targeted update is now
// redundant — the rebuild already captured the row the mutation is about
// to add or remove, because callers write to the DB before invoking the
// tree updater.
//
// Must be called without holding fs.mu; rebuildTree acquires it internally.
func (fs *LibraryFS) ensureTreeForMutation() (rebuilt bool) {
	fs.mu.RLock()
	loaded := fs.tree != nil
	fs.mu.RUnlock()
	if loaded {
		return false
	}
	fs.rebuildTree()
	return true
}

// InvalidateTree forces an immediate tree rebuild.
// This is used as a fallback when targeted updates aren't possible (e.g., subtitles added).
func (fs *LibraryFS) InvalidateTree() {
	slog.Info("VFS tree invalidation requested")
	fs.DeleteCache()
	// Clear the in-memory tree so rebuildTree() will actually rebuild
	// (its guard checks fs.tree != nil and returns early otherwise).
	fs.mu.Lock()
	fs.tree = nil
	fs.mu.Unlock()
	fs.rebuildTree()
}

// RemoveMovieFromTree removes a movie folder and its contents from the VFS tree.
// If the tree hasn't been built yet, rebuild from DB (which no longer has the
// active assignment) instead of no-opping.
func (fs *LibraryFS) RemoveMovieFromTree(title string, year int) {
	if fs.ensureTreeForMutation() {
		return
	}

	fs.mu.Lock()

	if fs.tree == nil {
		fs.mu.Unlock()
		return
	}

	folderPath := MovieDir(title, year)
	folderName := path.Base(folderPath)

	movieDir, exists := fs.tree.pathMap[folderPath]
	if !exists {
		fs.mu.Unlock()
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

	fs.mu.Unlock()

	slog.Debug("Removed movie from VFS tree", "path", folderPath)
	fs.saveTreeToCache()
}

// RemoveEpisodeFromTree removes an episode file and cleans up empty parent folders.
// If the tree hasn't been built yet, rebuild from DB (which no longer has the
// active assignment) instead of no-opping.
func (fs *LibraryFS) RemoveEpisodeFromTree(showTitle string, showYear int, seasonNumber int, episodeNumber int) {
	if fs.ensureTreeForMutation() {
		return
	}

	fs.mu.Lock()

	if fs.tree == nil {
		fs.mu.Unlock()
		return
	}

	tvDir, ok := fs.tree.pathMap[TVShowsPath].(*VirtualDir)
	if !ok {
		fs.mu.Unlock()
		return
	}

	showPath := ShowDir(showTitle, showYear)
	showFolderName := path.Base(showPath)

	showDirEntry, exists := fs.tree.pathMap[showPath]
	if !exists {
		fs.mu.Unlock()
		return
	}
	showDir, ok := showDirEntry.(*VirtualDir)
	if !ok {
		fs.mu.Unlock()
		return
	}

	seasonPath := SeasonDir(showTitle, showYear, seasonNumber)
	seasonFolderName := path.Base(seasonPath)

	seasonDirEntry, exists := fs.tree.pathMap[seasonPath]
	if !exists {
		fs.mu.Unlock()
		return
	}
	seasonDir, ok := seasonDirEntry.(*VirtualDir)
	if !ok {
		fs.mu.Unlock()
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
		fs.mu.Unlock()
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

	fs.mu.Unlock()
	fs.saveTreeToCache()
}

// RemoveShowFromTree removes an entire show subtree from the VFS tree.
// If the tree hasn't been built yet, rebuild from DB (which no longer has the
// show's active assignments) instead of no-opping.
func (fs *LibraryFS) RemoveShowFromTree(title string, year int) {
	if fs.ensureTreeForMutation() {
		return
	}

	fs.mu.Lock()

	if fs.tree == nil {
		fs.mu.Unlock()
		return
	}

	tvDir, ok := fs.tree.pathMap[TVShowsPath].(*VirtualDir)
	if !ok {
		fs.mu.Unlock()
		return
	}

	showPath := ShowDir(title, year)
	showFolderName := path.Base(showPath)

	showDirEntry, exists := fs.tree.pathMap[showPath]
	if !exists {
		fs.mu.Unlock()
		return
	}
	showDir, ok := showDirEntry.(*VirtualDir)
	if !ok {
		fs.mu.Unlock()
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

	fs.mu.Unlock()

	slog.Debug("Removed show from VFS tree", "path", showPath)
	fs.saveTreeToCache()
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

// The tree entry only lists the file; LibraryFS.Open streams it from the
// torrent.
func (f *TorrentSubtitleFile) Read([]byte) (int, error)          { return 0, os.ErrInvalid }
func (f *TorrentSubtitleFile) ReadAt([]byte, int64) (int, error) { return 0, os.ErrInvalid }
func (f *TorrentSubtitleFile) Close() error                      { return nil }

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
		// Listed as is; LibraryFS.Open() streams it from the torrent. A nil
		// here made WebDAV directory listings panic.
		return v
	default:
		return nil
	}
}
