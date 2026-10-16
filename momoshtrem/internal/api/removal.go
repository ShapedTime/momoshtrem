package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/service"
	"github.com/shapedtime/momoshtrem/internal/torrent"
)

// Removals deletes torrents and titles and unassigns files, cleaning up
// what they leave behind (PLAN §6 "Torrents and activity (B6)", "Titles
// and library"). *service.RemovalService implements it.
type Removals interface {
	DeleteTorrent(ctx context.Context, infoHash string, deleteData bool) error
	DeleteMovie(ctx context.Context, id int64, deleteData bool) error
	DeleteShow(ctx context.Context, id int64, deleteData bool) error
	UnassignMovie(ctx context.Context, id int64) error
	UnassignEpisode(ctx context.Context, id int64) error
}

var _ Removals = (*service.RemovalService)(nil)

// removalsReady answers 503 when no removal service is configured.
func (s *Server) removalsReady(c *gin.Context) bool {
	if s.opts.Removals == nil {
		errorResponse(c, http.StatusServiceUnavailable, "Removals not configured")
		return false
	}
	return true
}

// parseDeleteData reads ?delete_data=: "true" or "false", false when
// absent. Anything else is a 400.
func parseDeleteData(c *gin.Context) (deleteData, ok bool) {
	switch c.Query("delete_data") {
	case "", "false":
		return false, true
	case "true":
		return true, true
	}
	errorResponse(c, http.StatusBadRequest, "delete_data must be true or false")
	return false, false
}

// parseInfoHash reads the :hash parameter, normalised (hex in either case,
// or base32). Anything else is a 400.
func parseInfoHash(c *gin.Context) (string, bool) {
	hash := torrent.NormalizeInfoHash(c.Param("hash"))
	if hash == "" {
		errorResponse(c, http.StatusBadRequest, "Not a BitTorrent info hash: send 40 hex characters or 32 base32 ones")
		return "", false
	}
	return hash, true
}

// removalErrorResponse maps a Removals error to a response.
func removalErrorResponse(c *gin.Context, err error) {
	switch {
	case errors.Is(err, torrent.ErrInvalidInfoHash):
		errorResponse(c, http.StatusBadRequest, "Not a BitTorrent info hash: send 40 hex characters or 32 base32 ones")
	case errors.Is(err, torrent.ErrTorrentNotFound):
		errorResponse(c, http.StatusNotFound, "Torrent not found")
	case errors.Is(err, library.ErrMovieNotFound):
		errorResponse(c, http.StatusNotFound, "Movie not found")
	case errors.Is(err, library.ErrShowNotFound):
		errorResponse(c, http.StatusNotFound, "Show not found")
	case errors.Is(err, library.ErrEpisodeNotFound):
		errorResponse(c, http.StatusNotFound, "Episode not found")
	default:
		slog.Error("Removal failed", "path", c.Request.URL.Path, "error", err)
		errorResponse(c, http.StatusInternalServerError, "The removal failed")
	}
}
