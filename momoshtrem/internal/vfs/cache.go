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
	cacheVersion = 1
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
	InfoHash   string
	MagnetURI  string
	FilePath   string
}

type cachedShow struct {
	FolderName string
	Seasons    []cachedSeason
}

type cachedSeason struct {
	FolderName string
	Episodes   []cachedEpisode
}

type cachedEpisode struct {
	FileName  string
	FileSize  int64
	InfoHash  string
	MagnetURI string
	FilePath  string
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
			InfoHash:  cm.InfoHash,
			MagnetURI: cm.MagnetURI,
			FilePath:  cm.FilePath,
			FileSize:  cm.FileSize,
		}
		videoFile := NewPlaceholderFile(cm.FileName, cm.FileSize, assignment)
		filePath := folderPath + "/" + cm.FileName
		movieDir.children[cm.FileName] = videoFile
		tree.pathMap[filePath] = videoFile
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
					InfoHash:  ce.InfoHash,
					MagnetURI: ce.MagnetURI,
					FilePath:  ce.FilePath,
					FileSize:  ce.FileSize,
				}
				videoFile := NewPlaceholderFile(ce.FileName, ce.FileSize, assignment)
				filePath := seasonPath + "/" + ce.FileName
				seasonDir.children[ce.FileName] = videoFile
				tree.pathMap[filePath] = videoFile
			}
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
					InfoHash:   pf.assignment.InfoHash,
					MagnetURI:  pf.assignment.MagnetURI,
					FilePath:   pf.assignment.FilePath,
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
						InfoHash:  pf.assignment.InfoHash,
						MagnetURI: pf.assignment.MagnetURI,
						FilePath:  pf.assignment.FilePath,
					})
				}
				if len(csn.Episodes) > 0 {
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
