package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
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

	hash, ok := parseInfoHash(c)
	if !ok {
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

// deleteTorrent removes a torrent from the library and the client (PLAN §6
// "Torrents and activity (B6)").
// DELETE /api/torrents/:hash?delete_data=true|false → 204
func (s *Server) deleteTorrent(c *gin.Context) {
	hash, ok := parseInfoHash(c)
	if !ok {
		return
	}
	deleteData, ok := parseDeleteData(c)
	if !ok || !s.removalsReady(c) {
		return
	}
	if err := s.opts.Removals.DeleteTorrent(c.Request.Context(), hash, deleteData); err != nil {
		removalErrorResponse(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// pauseTorrent pauses a torrent
// POST /api/torrents/:hash/pause
func (s *Server) pauseTorrent(c *gin.Context) {
	if s.torrentService == nil {
		errorResponse(c, http.StatusServiceUnavailable, "Torrent service not available")
		return
	}

	hash, ok := parseInfoHash(c)
	if !ok {
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

	hash, ok := parseInfoHash(c)
	if !ok {
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
