package subtitle

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
)

// Cleanup when items leave the library or lose their file (PLAN §6
// "Titles and library"). Downloaded subtitles live in
// {download_path}/{item_type}/{item_id}/, HDRezka ones in
// {download_path}/hdrezka/{item_type}/{item_id}/; torrent subtitles are
// rows only (their files are in the torrent).

// DeleteForItems deletes every subtitle of the items, from every source:
// the rows in one query, their files, and each item's subtitle folders,
// with any file no row names. For items leaving the library. A file that
// can't be removed is logged; the rows are gone either way.
func (s *Service) DeleteForItems(ctx context.Context, items []Item) (int, error) {
	subs, err := s.repo.DeleteByItems(ctx, items)
	if err != nil {
		return 0, err
	}
	removeFiles(subs)
	for _, item := range items {
		for _, dir := range s.itemDirs(item) {
			if err := os.RemoveAll(dir); err != nil {
				slog.Warn("Failed to remove a subtitle folder", "path", dir, "error", err)
			}
		}
	}
	if len(subs) > 0 {
		slog.Info("Deleted the subtitles of items leaving the library", "items", len(items), "subtitles", len(subs))
	}
	return len(subs), nil
}

// DeleteFileSubtitles deletes the subtitles that came with the items'
// file: torrent rows, and HDRezka rows with their SRT files. Downloaded
// subtitles stay. For items whose file was unassigned. Files no row names
// are left to the periodic sweep (#82).
func (s *Service) DeleteFileSubtitles(ctx context.Context, items []Item) (int, error) {
	subs, err := s.repo.DeleteByItems(ctx, items, SourceTorrent, SourceHDRezka)
	if err != nil {
		return 0, err
	}
	removeFiles(subs)
	for _, item := range items {
		if dir := s.hdrezkaDir(item); dir != "" {
			_ = os.Remove(dir) // only when it's empty now
		}
	}
	return len(subs), nil
}

// removeFiles removes the local files of deleted subtitles: downloaded and
// HDRezka ones. A torrent subtitle's FilePath is a path in the torrent.
func removeFiles(subs []*Subtitle) {
	for _, sub := range subs {
		if sub.Source == SourceTorrent || sub.FilePath == "" {
			continue
		}
		if err := os.Remove(sub.FilePath); err != nil && !errors.Is(err, os.ErrNotExist) {
			slog.Warn("Failed to delete a subtitle file", "path", sub.FilePath, "error", err)
		}
	}
}

// itemDirs are the item's subtitle folders: downloaded, then HDRezka. None
// without a download path or for an item that isn't a movie or an episode,
// so a folder outside the subtitles is never named.
func (s *Service) itemDirs(item Item) []string {
	dir := s.downloadDir(item)
	if dir == "" {
		return nil
	}
	return []string{dir, s.hdrezkaDir(item)}
}

func (s *Service) downloadDir(item Item) string {
	if !s.ownsFolders(item) {
		return ""
	}
	return filepath.Join(s.subtitlesPath, string(item.Type), strconv.FormatInt(item.ID, 10))
}

func (s *Service) hdrezkaDir(item Item) string {
	if !s.ownsFolders(item) {
		return ""
	}
	return filepath.Join(s.subtitlesPath, string(SourceHDRezka), string(item.Type), strconv.FormatInt(item.ID, 10))
}

func (s *Service) ownsFolders(item Item) bool {
	return s.subtitlesPath != "" && item.ID > 0 && (item.Type == ItemTypeMovie || item.Type == ItemTypeEpisode)
}
