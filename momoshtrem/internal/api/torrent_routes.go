package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/shapedtime/momoshtrem/internal/activity"
	"github.com/shapedtime/momoshtrem/internal/torrent"
)

// Activity lists the torrents, sums them up, details one, and pauses and
// resumes them (PLAN §6 "Torrents and activity (B6)"). *activity.Service
// implements it.
type Activity interface {
	List(ctx context.Context) ([]activity.Item, error)
	Summary(ctx context.Context) (*activity.Summary, error)
	Detail(ctx context.Context, infoHash string) (*activity.Detail, error)
	Pause(ctx context.Context, infoHash string) (*activity.Item, error)
	Resume(ctx context.Context, infoHash string) (*activity.Item, error)
	PauseAll(ctx context.Context) (int, error)
	ResumeAll(ctx context.Context) (int, error)
}

var _ Activity = (*activity.Service)(nil)

// registerTorrentRoutes registers /torrents. The static routes sit beside
// /torrents/:hash: Gin matches them first. GET /torrents/:hash/files is the
// mapping review's (review_routes.go).
func (s *Server) registerTorrentRoutes(api *gin.RouterGroup) {
	api.GET("/torrents", s.listTorrents)
	api.GET("/torrents/summary", s.torrentsSummary)
	api.POST("/torrents/pause-all", s.pauseAllTorrents)
	api.POST("/torrents/resume-all", s.resumeAllTorrents)
	api.GET("/torrents/:hash", s.getTorrentDetail)
	api.DELETE("/torrents/:hash", s.deleteTorrent)
	api.POST("/torrents/:hash/pause", s.pauseTorrent)
	api.POST("/torrents/:hash/resume", s.resumeTorrent)
	api.GET("/torrents/:hash/debug", s.getTorrentDebug) // until #102
}

// activityReady answers 503 when no activity service is configured.
func (s *Server) activityReady(c *gin.Context) bool {
	if s.opts.Activity == nil {
		errorResponse(c, http.StatusServiceUnavailable, "Torrent activity not configured")
		return false
	}
	return true
}

// listTorrents lists every torrent the library knows, loaded or not.
// GET /api/torrents → 200 {items: TorrentListItem[]}
func (s *Server) listTorrents(c *gin.Context) {
	if !s.activityReady(c) {
		return
	}
	items, err := s.opts.Activity.List(c.Request.Context())
	if err != nil {
		activityErrorResponse(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

// torrentsSummary sums the torrents up: speeds, history, counts and disk.
// GET /api/torrents/summary → 200 TorrentsSummary
func (s *Server) torrentsSummary(c *gin.Context) {
	if !s.activityReady(c) {
		return
	}
	sum, err := s.opts.Activity.Summary(c.Request.Context())
	if err != nil {
		activityErrorResponse(c, err)
		return
	}
	c.JSON(http.StatusOK, sum)
}

// pauseTorrent pauses a torrent, loaded or not, across restarts.
// POST /api/torrents/:hash/pause → 200 TorrentListItem
func (s *Server) pauseTorrent(c *gin.Context) {
	s.setTorrentPaused(c, true)
}

// resumeTorrent clears a torrent's pause.
// POST /api/torrents/:hash/resume → 200 TorrentListItem
func (s *Server) resumeTorrent(c *gin.Context) {
	s.setTorrentPaused(c, false)
}

func (s *Server) setTorrentPaused(c *gin.Context, pause bool) {
	hash, ok := parseInfoHash(c)
	if !ok || !s.activityReady(c) {
		return
	}
	set := s.opts.Activity.Resume
	if pause {
		set = s.opts.Activity.Pause
	}
	item, err := set(c.Request.Context(), hash)
	if err != nil {
		activityErrorResponse(c, err)
		return
	}
	c.JSON(http.StatusOK, item)
}

// pauseAllTorrents pauses every streaming and downloading torrent.
// POST /api/torrents/pause-all → 200 {count}
func (s *Server) pauseAllTorrents(c *gin.Context) {
	if !s.activityReady(c) {
		return
	}
	n, err := s.opts.Activity.PauseAll(c.Request.Context())
	if err != nil {
		activityErrorResponse(c, err)
		return
	}
	c.JSON(http.StatusOK, activity.PauseCount{Count: n})
}

// resumeAllTorrents resumes every paused torrent.
// POST /api/torrents/resume-all → 200 {count}
func (s *Server) resumeAllTorrents(c *gin.Context) {
	if !s.activityReady(c) {
		return
	}
	n, err := s.opts.Activity.ResumeAll(c.Request.Context())
	if err != nil {
		activityErrorResponse(c, err)
		return
	}
	c.JSON(http.StatusOK, activity.PauseCount{Count: n})
}

// activityErrorResponse maps an Activity error to a response.
func activityErrorResponse(c *gin.Context, err error) {
	switch {
	case errors.Is(err, torrent.ErrInvalidInfoHash):
		errorResponse(c, http.StatusBadRequest, "Not a BitTorrent info hash: send 40 hex characters or 32 base32 ones")
	case errors.Is(err, torrent.ErrTorrentNotFound):
		errorResponse(c, http.StatusNotFound, "Torrent not found")
	default:
		slog.Error("Torrent activity failed", "path", c.Request.URL.Path, "error", err)
		errorResponse(c, http.StatusInternalServerError, "Couldn't read or change the torrents")
	}
}

// deleteTorrent removes a torrent from the library and the client.
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

// getTorrentDetail returns a torrent's list item plus its detail: piece
// map, files, trackers and totals. It never loads the torrent.
// GET /api/torrents/:hash → 200 TorrentDetail
func (s *Server) getTorrentDetail(c *gin.Context) {
	hash, ok := parseInfoHash(c)
	if !ok || !s.activityReady(c) {
		return
	}
	d, err := s.opts.Activity.Detail(c.Request.Context(), hash)
	if err != nil {
		activityErrorResponse(c, err)
		return
	}
	c.JSON(http.StatusOK, d)
}
