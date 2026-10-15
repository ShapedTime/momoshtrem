package vfs

import (
	"bytes"
	"encoding/gob"
	"errors"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/shapedtime/momoshtrem/internal/library"
)

const (
	// cacheVersion 5 stores each file's source (Kind) and HDRezka stream,
	// so a stream restored from the cache still streams; v4 split a
	// grouped file at gaps in its episodes (E07 and E09 are two entries);
	// v3 named a file a job mapped to several episodes once
	// ("S01E01-E02"); v2 added subtitle files. An older cache is ignored
	// and the tree rebuilt from the database.
	cacheVersion = 5
	cacheFile    = "vfs_tree.gob"
)

// treeCache is the serializable representation of the VFS tree.
type treeCache struct {
	Version int
	Movies  []cachedMovie
	Shows   []cachedShow
}

type cachedMovie struct {
	FolderName string
	FileName   string
	FileSize   int64
	Kind       library.AssignmentKind
	InfoHash   string
	MagnetURI  string
	FilePath   string
	Stream     *library.StreamRef // HDRezka streams only
	Subtitles  []cachedSubtitle
}

type cachedShow struct {
	FolderName string
	Seasons    []cachedSeason
}

type cachedSeason struct {
	FolderName string
	Episodes   []cachedEpisode
	Subtitles  []cachedSubtitle // Every episode's, named after it
}

// cachedSubtitle is a subtitle file next to a video: downloaded (LocalPath)
// or streamed from a torrent (TorrentPath and InfoHash).
type cachedSubtitle struct {
	FileName    string
	FileSize    int64
	LocalPath   string
	TorrentPath string
	InfoHash    string
}

// cacheSubtitles lists the subtitle files in dir.
func cacheSubtitles(dir *VirtualDir) []cachedSubtitle {
	var out []cachedSubtitle
	for name, entry := range dir.children {
		switch e := entry.(type) {
		case *SubtitleFile:
			out = append(out, cachedSubtitle{FileName: name, FileSize: e.size, LocalPath: e.localPath})
		case *TorrentSubtitleFile:
			out = append(out, cachedSubtitle{FileName: name, FileSize: e.size, TorrentPath: e.torrentPath, InfoHash: e.infoHash})
		}
	}
	return out
}

// restoreSubtitles puts cached subtitle files back in dir.
func restoreSubtitles(tree *DirectoryTree, dir *VirtualDir, dirPath string, subs []cachedSubtitle) {
	for _, cs := range subs {
		var entry Entry
		if cs.InfoHash != "" {
			entry = NewTorrentSubtitleFile(cs.FileName, cs.TorrentPath, cs.FileSize, cs.InfoHash)
		} else {
			entry = NewSubtitleFile(cs.FileName, cs.LocalPath, cs.FileSize)
		}
		dir.children[cs.FileName] = entry
		tree.pathMap[dirPath+"/"+cs.FileName] = entry
	}
}

type cachedEpisode struct {
	FileName  string
	FileSize  int64
	Kind      library.AssignmentKind
	InfoHash  string
	MagnetURI string
	FilePath  string
	Stream    *library.StreamRef // HDRezka streams only
}

// loadTreeFromCache attempts to load the VFS tree from disk cache.
// Returns error if cache doesn't exist, is invalid, or version mismatches.
func (fs *LibraryFS) loadTreeFromCache() error {
	if fs.cacheDir == "" {
		return errors.New("no cache directory configured")
	}

	cachePath := filepath.Join(fs.cacheDir, cacheFile)
	data, err := os.ReadFile(cachePath)
	if err != nil {
		return err
	}

	var cache treeCache
	if err := gob.NewDecoder(bytes.NewReader(data)).Decode(&cache); err != nil {
		return err
	}

	if cache.Version != cacheVersion {
		return errors.New("cache version mismatch")
	}

	// Reconstruct tree from cache
	tree, moviesDir, tvDir := newEmptyTree()

	// Restore movies
	for _, cm := range cache.Movies {
		movieDir := NewVirtualDir(cm.FolderName)
		folderPath := MoviesPath + "/" + cm.FolderName
		moviesDir.children[cm.FolderName] = movieDir
		tree.pathMap[folderPath] = movieDir

		// Restore video file
		assignment := &library.TorrentAssignment{
			ItemType:  library.ItemTypeMovie,
			Kind:      cm.Kind,
			InfoHash:  cm.InfoHash,
			MagnetURI: cm.MagnetURI,
			FilePath:  cm.FilePath,
			FileSize:  cm.FileSize,
			Stream:    cm.Stream,
			IsActive:  true,
		}
		videoFile := NewPlaceholderFile(cm.FileName, cm.FileSize, assignment)
		filePath := folderPath + "/" + cm.FileName
		movieDir.children[cm.FileName] = videoFile
		tree.pathMap[filePath] = videoFile
		restoreSubtitles(tree, movieDir, folderPath, cm.Subtitles)
	}

	// Restore shows
	for _, cs := range cache.Shows {
		showDir := NewVirtualDir(cs.FolderName)
		showPath := TVShowsPath + "/" + cs.FolderName
		tvDir.children[cs.FolderName] = showDir
		tree.pathMap[showPath] = showDir

		for _, csn := range cs.Seasons {
			seasonDir := NewVirtualDir(csn.FolderName)
			seasonPath := showPath + "/" + csn.FolderName
			showDir.children[csn.FolderName] = seasonDir
			tree.pathMap[seasonPath] = seasonDir

			for _, ce := range csn.Episodes {
				assignment := &library.TorrentAssignment{
					ItemType:  library.ItemTypeEpisode,
					Kind:      ce.Kind,
					InfoHash:  ce.InfoHash,
					MagnetURI: ce.MagnetURI,
					FilePath:  ce.FilePath,
					FileSize:  ce.FileSize,
					Stream:    ce.Stream,
					IsActive:  true,
				}
				videoFile := NewPlaceholderFile(ce.FileName, ce.FileSize, assignment)
				filePath := seasonPath + "/" + ce.FileName
				seasonDir.children[ce.FileName] = videoFile
				tree.pathMap[filePath] = videoFile
			}
			restoreSubtitles(tree, seasonDir, seasonPath, csn.Subtitles)
		}
	}

	// Atomic swap
	fs.mu.Lock()
	fs.tree = tree
	fs.mu.Unlock()

	slog.Info("VFS tree loaded from cache", "entries", len(tree.pathMap))
	return nil
}

// saveTreeToCache serializes the current VFS tree to disk.
// This runs synchronously after tree rebuild.
func (fs *LibraryFS) saveTreeToCache() {
	if fs.cacheDir == "" {
		return
	}

	fs.mu.RLock()
	tree := fs.tree
	fs.mu.RUnlock()

	if tree == nil {
		return
	}

	cache := treeCache{
		Version: cacheVersion,
	}

	// Extract movies
	moviesDir, ok := tree.pathMap[MoviesPath].(*VirtualDir)
	if ok {
		for folderName, entry := range moviesDir.children {
			movieDir, ok := entry.(*VirtualDir)
			if !ok {
				continue
			}
			for fileName, fileEntry := range movieDir.children {
				pf, ok := fileEntry.(*PlaceholderFile)
				if !ok || pf.assignment == nil {
					continue
				}
				cache.Movies = append(cache.Movies, cachedMovie{
					FolderName: folderName,
					FileName:   fileName,
					FileSize:   pf.assignment.FileSize,
					Kind:       pf.assignment.Kind,
					InfoHash:   pf.assignment.InfoHash,
					MagnetURI:  pf.assignment.MagnetURI,
					FilePath:   pf.assignment.FilePath,
					Stream:     pf.assignment.Stream,
					Subtitles:  cacheSubtitles(movieDir),
				})
				break // Only one video file per movie folder
			}
		}
	}

	// Extract shows
	tvDir, ok := tree.pathMap[TVShowsPath].(*VirtualDir)
	if ok {
		for showFolderName, showEntry := range tvDir.children {
			showDir, ok := showEntry.(*VirtualDir)
			if !ok {
				continue
			}
			cs := cachedShow{FolderName: showFolderName}

			for seasonFolderName, seasonEntry := range showDir.children {
				seasonDir, ok := seasonEntry.(*VirtualDir)
				if !ok {
					continue
				}
				csn := cachedSeason{FolderName: seasonFolderName}

				for fileName, fileEntry := range seasonDir.children {
					pf, ok := fileEntry.(*PlaceholderFile)
					if !ok || pf.assignment == nil {
						continue
					}
					csn.Episodes = append(csn.Episodes, cachedEpisode{
						FileName:  fileName,
						FileSize:  pf.assignment.FileSize,
						Kind:      pf.assignment.Kind,
						InfoHash:  pf.assignment.InfoHash,
						MagnetURI: pf.assignment.MagnetURI,
						FilePath:  pf.assignment.FilePath,
						Stream:    pf.assignment.Stream,
					})
				}
				if len(csn.Episodes) > 0 {
					csn.Subtitles = cacheSubtitles(seasonDir)
					cs.Seasons = append(cs.Seasons, csn)
				}
			}
			if len(cs.Seasons) > 0 {
				cache.Shows = append(cache.Shows, cs)
			}
		}
	}

	// Ensure cache directory exists
	if err := os.MkdirAll(fs.cacheDir, 0755); err != nil {
		slog.Error("Failed to create VFS cache directory", "error", err)
		return
	}

	// Serialize to buffer
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(cache); err != nil {
		slog.Error("Failed to encode VFS cache", "error", err)
		return
	}

	// Write to file atomically
	cachePath := filepath.Join(fs.cacheDir, cacheFile)
	tmpPath := cachePath + ".tmp"
	if err := os.WriteFile(tmpPath, buf.Bytes(), 0644); err != nil {
		slog.Error("Failed to write VFS cache", "error", err)
		return
	}
	if err := os.Rename(tmpPath, cachePath); err != nil {
		slog.Error("Failed to rename VFS cache", "error", err)
		os.Remove(tmpPath)
		return
	}

	slog.Debug("VFS tree saved to cache",
		"movies", len(cache.Movies),
		"shows", len(cache.Shows),
	)
}

// DeleteCache removes the cached VFS tree file.
// Call this when the cache needs to be invalidated (e.g., schema change).
func (fs *LibraryFS) DeleteCache() error {
	if fs.cacheDir == "" {
		return nil
	}
	cachePath := filepath.Join(fs.cacheDir, cacheFile)
	err := os.Remove(cachePath)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
