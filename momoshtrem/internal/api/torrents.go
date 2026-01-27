package api

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/torrent"
)

// TorrentListResponse contains a list of torrents
type TorrentListResponse struct {
	Torrents []TorrentResponse `json:"torrents"`
}

// TorrentResponse contains torrent status information
type TorrentResponse struct {
	InfoHash      string  `json:"info_hash"`
	Name          string  `json:"name"`
	TotalSize     int64   `json:"total_size"`
	Downloaded    int64   `json:"downloaded"`
	Progress      float64 `json:"progress"`
	Seeders       int     `json:"seeders"`
	Leechers      int     `json:"leechers"`
	DownloadSpeed int64   `json:"download_speed"`
	UploadSpeed   int64   `json:"upload_speed"`
	IsPaused      bool    `json:"is_paused"`
}

// listTorrents returns all active torrents
// GET /api/torrents
func (s *Server) listTorrents(c *gin.Context) {
	if s.torrentService == nil {
		errorResponse(c, http.StatusServiceUnavailable, "Torrent service not available")
		return
	}

	statuses, err := s.torrentService.ListTorrents()
	if err != nil {
		errorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}

	response := TorrentListResponse{
		Torrents: make([]TorrentResponse, len(statuses)),
	}

	for i, status := range statuses {
		response.Torrents[i] = statusToResponse(status)
	}

	c.JSON(http.StatusOK, response)
}

// getTorrent returns status of a specific torrent
// GET /api/torrents/:hash
func (s *Server) getTorrent(c *gin.Context) {
	if s.torrentService == nil {
		errorResponse(c, http.StatusServiceUnavailable, "Torrent service not available")
		return
	}

	hash := c.Param("hash")
	if hash == "" {
		errorResponse(c, http.StatusBadRequest, "Hash parameter is required")
		return
	}

	status, err := s.torrentService.GetStatus(hash)
	if err != nil {
		if err == torrent.ErrTorrentNotFound {
			errorResponse(c, http.StatusNotFound, "Torrent not found")
			return
		}
		errorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}

	c.JSON(http.StatusOK, statusToResponse(*status))
}

// deleteTorrent removes a torrent
// DELETE /api/torrents/:hash
func (s *Server) deleteTorrent(c *gin.Context) {
	if s.torrentService == nil {
		errorResponse(c, http.StatusServiceUnavailable, "Torrent service not available")
		return
	}

	hash := c.Param("hash")
	if hash == "" {
		errorResponse(c, http.StatusBadRequest, "Hash parameter is required")
		return
	}

	deleteData := c.Query("delete_data") == "true"

	// Get assignments BEFORE deleting so we can update the VFS tree
	assignments, err := s.assignmentRepo.GetByInfoHash(hash)
	if err != nil {
		slog.Error("Failed to get assignments for torrent", "hash", hash, "error", err)
		// Continue anyway - we'll fall back to InvalidateTree if needed
	}

	// Delete all assignments using this torrent
	var assignmentsDeleted int64
	if deleted, err := s.assignmentRepo.DeleteByInfoHash(hash); err != nil {
		slog.Error("Failed to delete assignments for torrent", "hash", hash, "error", err)
		// Continue with torrent deletion even if assignment cleanup fails
	} else if deleted > 0 {
		assignmentsDeleted = deleted
		slog.Info("Deleted assignments for torrent", "hash", hash, "count", deleted)
	}

	if err := s.torrentService.RemoveTorrent(hash, deleteData); err != nil {
		if err == torrent.ErrTorrentNotFound {
			errorResponse(c, http.StatusNotFound, "Torrent not found")
			return
		}
		errorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}

	// Update VFS tree - use targeted removal if we have assignment info
	if assignmentsDeleted > 0 && s.treeUpdater != nil {
		if len(assignments) > 0 {
			s.removeAssignmentsFromTree(assignments)
		} else {
			// Fallback to full invalidation if we couldn't get assignments
			s.treeUpdater.InvalidateTree()
		}
	}

	c.Status(http.StatusNoContent)
}

// removeAssignmentsFromTree removes items from the VFS tree based on their assignments
func (s *Server) removeAssignmentsFromTree(assignments []*library.TorrentAssignment) {
	if s.treeUpdater == nil {
		return
	}

	for _, assignment := range assignments {
		switch assignment.ItemType {
		case library.ItemTypeMovie:
			movie, err := s.movieRepo.GetByID(assignment.ItemID)
			if err != nil || movie == nil {
				slog.Warn("Could not find movie for tree removal", "item_id", assignment.ItemID)
				continue
			}
			s.treeUpdater.RemoveMovieFromTree(movie.Title, movie.Year)
			slog.Debug("Removed movie from tree", "title", movie.Title, "year", movie.Year)

		case library.ItemTypeEpisode:
			ctx, err := s.showRepo.GetEpisodeContext(assignment.ItemID)
			if err != nil || ctx == nil {
				slog.Warn("Could not find episode context for tree removal", "item_id", assignment.ItemID, "error", err)
				continue
			}
			s.treeUpdater.RemoveEpisodeFromTree(ctx.ShowTitle, ctx.ShowYear, ctx.SeasonNumber, ctx.EpisodeNumber)
			slog.Debug("Removed episode from tree", "show", ctx.ShowTitle, "season", ctx.SeasonNumber, "episode", ctx.EpisodeNumber)
		}
	}
}

// pauseTorrent pauses a torrent
// POST /api/torrents/:hash/pause
func (s *Server) pauseTorrent(c *gin.Context) {
	if s.torrentService == nil {
		errorResponse(c, http.StatusServiceUnavailable, "Torrent service not available")
		return
	}

	hash := c.Param("hash")
	if hash == "" {
		errorResponse(c, http.StatusBadRequest, "Hash parameter is required")
		return
	}

	if err := s.torrentService.Pause(hash); err != nil {
		if err == torrent.ErrTorrentNotFound {
			errorResponse(c, http.StatusNotFound, "Torrent not found")
			return
		}
		errorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Torrent paused"})
}

// resumeTorrent resumes a torrent
// POST /api/torrents/:hash/resume
func (s *Server) resumeTorrent(c *gin.Context) {
	if s.torrentService == nil {
		errorResponse(c, http.StatusServiceUnavailable, "Torrent service not available")
		return
	}

	hash := c.Param("hash")
	if hash == "" {
		errorResponse(c, http.StatusBadRequest, "Hash parameter is required")
		return
	}

	if err := s.torrentService.Resume(hash); err != nil {
		if err == torrent.ErrTorrentNotFound {
			errorResponse(c, http.StatusNotFound, "Torrent not found")
			return
		}
		errorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Torrent resumed"})
}

// statusToResponse converts TorrentStatus to TorrentResponse
func statusToResponse(status torrent.TorrentStatus) TorrentResponse {
	return TorrentResponse{
		InfoHash:      status.InfoHash,
		Name:          status.Name,
		TotalSize:     status.TotalSize,
		Downloaded:    status.Downloaded,
		Progress:      status.Progress,
		Seeders:       status.Seeders,
		Leechers:      status.Leechers,
		DownloadSpeed: status.DownloadSpeed,
		UploadSpeed:   status.UploadSpeed,
		IsPaused:      status.IsPaused,
	}
}
