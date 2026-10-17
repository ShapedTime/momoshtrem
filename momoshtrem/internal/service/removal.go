package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/subtitle"
	"github.com/shapedtime/momoshtrem/internal/torrent"
)

// RemovalTorrents is what removals need from the torrent client: whether a
// torrent is loaded, dropping it (with its data when asked), adding it
// again to learn its pieces when its metainfo isn't stored, and clearing a
// deleted torrent's pause (Resume, once it's dropped).
type RemovalTorrents interface {
	GetTorrent(infoHash string) (*torrent.TorrentInfo, error)
	AddTorrent(magnetURI string) (*torrent.TorrentInfo, error)
	RemoveTorrent(infoHash string, deleteData bool) error
	Resume(infoHash string) error
}

var _ RemovalTorrents = (torrent.Service)(nil)

// SubtitleCleaner deletes the subtitles of items that leave the library or
// lose their file. *subtitle.Service implements it.
type SubtitleCleaner interface {
	DeleteForItems(ctx context.Context, items []subtitle.Item) (int, error)
	DeleteFileSubtitles(ctx context.Context, items []subtitle.Item) (int, error)
}

var _ SubtitleCleaner = (*subtitle.Service)(nil)

// RemovalTree is the WebDAV tree: rebuilt after a removal, and told before
// a torrent goes so it can close what it holds of it.
type RemovalTree interface {
	InvalidateTree()
	InvalidateFileHandles(infoHash string)
}

// PauseClearer clears stored torrent pauses (app_settings key "torrents").
// *library.SettingsRepository implements it.
type PauseClearer interface {
	ResumeTorrents(ctx context.Context, hashes []string) error
}

var _ PauseClearer = (*library.SettingsRepository)(nil)

// DownloadsWaker wakes the download manager (*downloads.Service), so a
// removal's downloads drop their pins at once.
type DownloadsWaker interface {
	Wake()
}

// RemovalDeps are a RemovalService's dependencies. Torrents, Subtitles,
// Tree, Pauses and Downloads are optional.
type RemovalDeps struct {
	Jobs        *library.AssignmentJobRepository
	Movies      *library.MovieRepository
	Shows       *library.ShowRepository
	Assignments *library.AssignmentRepository
	Torrents    RemovalTorrents
	Subtitles   SubtitleCleaner
	Tree        RemovalTree
	// Pauses clears a released torrent's stored pause; nil keeps it (it's
	// pruned at the next startup).
	Pauses PauseClearer
	// Downloads is woken after each removal; nil wakes nothing (its next
	// tick catches up).
	Downloads DownloadsWaker
}

// RemovalService deletes torrents and titles and unassigns files, and
// cleans up what they leave behind: subtitles (rows and files), jobs,
// torrents nothing uses any more, and the WebDAV tree (PLAN §6 "Torrents
// and activity (B6)", "Titles and library").
//
// It never touches the job service's per-hash lock: the torrent service
// serialises add, wait and remove per hash itself.
type RemovalService struct {
	jobs        *library.AssignmentJobRepository
	movies      *library.MovieRepository
	shows       *library.ShowRepository
	assignments *library.AssignmentRepository
	torrents    RemovalTorrents
	subtitles   SubtitleCleaner
	tree        RemovalTree
	pauses      PauseClearer
	downloads   DownloadsWaker
	log         *slog.Logger

	// background counts best-effort data deletes still running.
	background sync.WaitGroup
}

// NewRemovalService creates a RemovalService.
func NewRemovalService(d RemovalDeps) *RemovalService {
	return &RemovalService{
		jobs:        d.Jobs,
		movies:      d.Movies,
		shows:       d.Shows,
		assignments: d.Assignments,
		torrents:    d.Torrents,
		subtitles:   d.Subtitles,
		tree:        d.Tree,
		pauses:      d.Pauses,
		downloads:   d.Downloads,
		log:         slog.With("component", "removals"),
	}
}

// DeleteTorrent removes a torrent from the library and the client:
//   - its open jobs are cancelled (dry runs too), its active assignments
//     deactivated (the rows stay, as history), its torrent subtitles
//     deleted and its stored pause cleared, in one transaction;
//   - it's dropped from the client, with its data when deleteData is set
//     (from the live or stored metainfo; without either, the metainfo is
//     fetched again in the background, best effort), and its pause there
//     is cleared too, so it isn't paused if it's added again;
//   - the WebDAV tree is rebuilt.
//
// The hash may be hex in either case, or base32: torrent.ErrInvalidInfoHash
// otherwise. torrent.ErrTorrentNotFound only when nothing knows it: it
// isn't loaded, no active assignment uses it and no open job (dry runs
// aside) has it.
func (s *RemovalService) DeleteTorrent(ctx context.Context, infoHash string, deleteData bool) error {
	hash := torrent.NormalizeInfoHash(infoHash)
	if hash == "" {
		return fmt.Errorf("%w: %q", torrent.ErrInvalidInfoHash, infoHash)
	}
	if !s.loaded(hash) {
		known, err := s.jobs.TorrentKnown(ctx, hash)
		if err != nil {
			return err
		}
		if !known {
			return torrent.ErrTorrentNotFound
		}
	}

	forgotten, err := s.jobs.ForgetTorrent(ctx, hash)
	if err != nil {
		return err
	}
	s.log.Info("Torrent deleted",
		"info_hash", hash,
		"delete_data", deleteData,
		"jobs_cancelled", forgotten.Cancelled,
		"assignments_deactivated", forgotten.Deactivated,
		"subtitles_deleted", forgotten.Subtitles,
		"pause_cleared", forgotten.Unpaused > 0,
	)
	s.drop(library.TorrentRef{InfoHash: hash, MagnetURI: forgotten.MagnetURI}, deleteData)
	s.forgetPause(ctx, hash, false) // ForgetTorrent cleared the stored one
	s.invalidateTree()
	return nil
}

// forgetPause clears the pause of a torrent nothing uses any more, so it
// isn't paused if it's assigned again: the client's, and the stored one
// when stored is set. It's dropped by then, so nothing wakes.
func (s *RemovalService) forgetPause(ctx context.Context, hash string, stored bool) {
	if stored && s.pauses != nil {
		if err := s.pauses.ResumeTorrents(ctx, []string{hash}); err != nil {
			s.log.Warn("Failed to clear a released torrent's stored pause; the next start prunes it", "info_hash", hash, "error", err)
		}
	}
	if s.torrents != nil {
		if err := s.torrents.Resume(hash); err != nil {
			s.log.Warn("Failed to clear a torrent's pause", "info_hash", hash, "error", err)
		}
	}
}

// DeleteMovie deletes a movie: its assignments are deactivated, its jobs
// go with it, every subtitle it had is deleted (rows and files), and each
// torrent it used that nothing else uses is dropped, with its data when
// deleteData is set. library.ErrMovieNotFound when it doesn't exist.
func (s *RemovalService) DeleteMovie(ctx context.Context, id int64, deleteData bool) error {
	deletion, err := s.movies.DeleteWithAssignments(ctx, id)
	if err != nil {
		return err
	}
	s.afterTitleDelete(ctx, subtitle.ItemTypeMovie, deletion, deleteData)
	s.log.Info("Movie deleted", "movie_id", id, "title", deletion.Title, "year", deletion.Year,
		"torrents", len(deletion.Torrents), "delete_data", deleteData)
	return nil
}

// DeleteShow deletes a show like DeleteMovie, for every episode: one query
// deactivates their assignments, one deletes their subtitles.
// library.ErrShowNotFound when it doesn't exist.
func (s *RemovalService) DeleteShow(ctx context.Context, id int64, deleteData bool) error {
	deletion, err := s.shows.DeleteWithAssignments(ctx, id)
	if err != nil {
		return err
	}
	s.afterTitleDelete(ctx, subtitle.ItemTypeEpisode, deletion, deleteData)
	s.log.Info("Show deleted", "show_id", id, "title", deletion.Title, "year", deletion.Year,
		"episodes", len(deletion.ItemIDs), "torrents", len(deletion.Torrents), "delete_data", deleteData)
	return nil
}

// afterTitleDelete cleans up after a title's rows are gone: its subtitles,
// its unused torrents and the tree. It outlives a cancelled request: the
// delete is committed.
func (s *RemovalService) afterTitleDelete(ctx context.Context, itemType subtitle.ItemType, d *library.TitleDeletion, deleteData bool) {
	ctx = context.WithoutCancel(ctx)
	if s.subtitles != nil && len(d.ItemIDs) > 0 {
		items := make([]subtitle.Item, len(d.ItemIDs))
		for i, id := range d.ItemIDs {
			items[i] = subtitle.Item{Type: itemType, ID: id}
		}
		if _, err := s.subtitles.DeleteForItems(ctx, items); err != nil {
			s.log.Warn("Failed to delete a deleted title's subtitles", "title", d.Title, "error", err)
		}
	}
	s.releaseUnused(ctx, d.Torrents, deleteData)
	s.invalidateTree()
}

// UnassignMovie drops the movie's file: its assignment is deactivated, the
// subtitles that came with the file (torrent and HDRezka) are deleted, and
// the torrent is dropped when nothing else uses it (never its data).
// Downloaded subtitles stay. library.ErrMovieNotFound for an unknown movie.
func (s *RemovalService) UnassignMovie(ctx context.Context, id int64) error {
	movie, err := s.movies.GetByID(id)
	if err != nil {
		return err
	}
	if movie == nil {
		return library.ErrMovieNotFound
	}
	return s.unassign(ctx, library.ItemTypeMovie, id)
}

// UnassignEpisode drops the episode's file like UnassignMovie. The
// episode's grouped entry ("S01E01-E02") is renamed by the tree rebuild.
// library.ErrEpisodeNotFound for an unknown episode.
func (s *RemovalService) UnassignEpisode(ctx context.Context, id int64) error {
	if _, err := s.shows.EpisodeShowID(ctx, id); err != nil {
		return err
	}
	return s.unassign(ctx, library.ItemTypeEpisode, id)
}

func (s *RemovalService) unassign(ctx context.Context, itemType library.ItemType, id int64) error {
	active, err := s.assignments.GetActiveForItem(itemType, id)
	if err != nil {
		return err
	}
	if active != nil && active.IsTorrent() && s.tree != nil {
		s.tree.InvalidateFileHandles(active.InfoHash)
	}
	// Deactivating first waits for an HDRezka subtitle write that holds
	// the stream's row, so the delete below sees its subtitles.
	if err := s.assignments.DeactivateForItem(itemType, id); err != nil {
		return err
	}

	ctx = context.WithoutCancel(ctx)
	if s.subtitles != nil {
		item := subtitle.Item{Type: subtitle.ItemType(itemType), ID: id}
		if _, err := s.subtitles.DeleteFileSubtitles(ctx, []subtitle.Item{item}); err != nil {
			s.log.Warn("Failed to delete an unassigned file's subtitles", "item_type", itemType, "item_id", id, "error", err)
		}
	}
	if active == nil {
		return nil
	}
	if active.IsTorrent() {
		s.releaseUnused(ctx, []library.TorrentRef{{InfoHash: active.InfoHash, MagnetURI: active.MagnetURI}}, false)
	}
	s.invalidateTree()
	s.log.Info("File unassigned", "item_type", itemType, "item_id", id, "kind", active.Kind)
	return nil
}

// releaseUnused drops each torrent that no active assignment or open job
// uses any more, with its data when deleteData is set, and clears its
// pause (stored and in the client), so it isn't paused if it's assigned
// again.
func (s *RemovalService) releaseUnused(ctx context.Context, refs []library.TorrentRef, deleteData bool) {
	if s.torrents == nil {
		return
	}
	for _, ref := range refs {
		used, err := s.jobs.TorrentInUse(ctx, ref.InfoHash, 0)
		if err != nil {
			s.log.Warn("Couldn't check whether the torrent is in use; keeping it", "info_hash", ref.InfoHash, "error", err)
			continue
		}
		if used {
			continue
		}
		if s.drop(ref, deleteData) {
			s.forgetPause(ctx, ref.InfoHash, true)
		}
	}
}

// drop removes the torrent from the client. With deleteData, a torrent
// that's neither loaded nor stored has its data deleted in the background.
// It reports whether the torrent is gone from the client: removed, or it
// wasn't there.
func (s *RemovalService) drop(ref library.TorrentRef, deleteData bool) bool {
	if s.torrents == nil {
		return false
	}
	if s.tree != nil {
		s.tree.InvalidateFileHandles(ref.InfoHash)
	}
	err := s.torrents.RemoveTorrent(ref.InfoHash, deleteData)
	switch {
	case err == nil:
	case errors.Is(err, torrent.ErrTorrentNotFound):
		if deleteData {
			s.deleteDataLater(ref)
		}
	default:
		s.log.Warn("Failed to remove a torrent", "info_hash", ref.InfoHash, "error", err)
		return false
	}
	return true
}

// deleteDataLater deletes the data of a torrent whose metainfo isn't
// stored, best effort: it adds the torrent again from its magnet (within
// the client's add timeout) to learn its pieces, then removes it with its
// data, unless something uses it again by then. Without peers, the data is
// left to the cache's LRU.
func (s *RemovalService) deleteDataLater(ref library.TorrentRef) {
	log := s.log.With("info_hash", ref.InfoHash)
	if ref.MagnetURI == "" {
		log.Warn("No magnet to fetch the torrent's metainfo; its data is left to the cache")
		return
	}
	s.background.Add(1)
	go func() {
		defer s.background.Done()
		log.Info("Fetching a deleted torrent's metainfo to delete its data")
		if _, err := s.torrents.AddTorrent(ref.MagnetURI); err != nil {
			log.Warn("Couldn't fetch the torrent's metainfo; its data is left to the cache", "error", err)
			return
		}
		used, err := s.jobs.TorrentInUse(context.Background(), ref.InfoHash, 0)
		if err != nil || used {
			log.Info("The torrent is in use again; keeping it and its data", "error", err)
			return
		}
		if err := s.torrents.RemoveTorrent(ref.InfoHash, true); err != nil {
			log.Warn("Failed to delete the torrent's data", "error", err)
		}
	}()
}

// loaded reports whether the client has the torrent loaded.
func (s *RemovalService) loaded(hash string) bool {
	if s.torrents == nil {
		return false
	}
	_, err := s.torrents.GetTorrent(hash)
	return err == nil
}

// invalidateTree rebuilds the WebDAV tree after a removal, and wakes the
// download manager: a title's downloads cascaded away, or a torrent's
// files left, so their pins go (#77).
func (s *RemovalService) invalidateTree() {
	if s.tree != nil {
		s.tree.InvalidateTree()
	}
	if s.downloads != nil {
		s.downloads.Wake()
	}
}
