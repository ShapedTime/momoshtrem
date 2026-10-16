package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/service"
	"github.com/shapedtime/momoshtrem/internal/torznab"
)

// Inspector creates Inspect's dry runs (PLAN §6 "Search" › inspect).
// *service.JobService implements it.
type Inspector interface {
	Inspect(ctx context.Context, req service.InspectRequest) (jobID int64, created bool, err error)
}

var _ Inspector = (*service.JobService)(nil)

// ReleaseResolver turns a search result's release_id into its magnet and
// the indexer that found it (PLAN §6 "Search" › resolve). The torrent
// search implements it. Its errors:
//   - service.ErrReleaseNotFound: the id isn't indexed (any more), 404;
//   - torznab.ErrTimeout or context.DeadlineExceeded: 504 timeout;
//   - anything else: the indexer's link gave no torrent, 502
//     torrent_unavailable.
type ReleaseResolver interface {
	Resolve(ctx context.Context, releaseID string) (magnetURI, indexer string, err error)
}

// registerInspectRoutes adds POST /search/torrents/inspect.
func (s *Server) registerInspectRoutes(api *gin.RouterGroup) {
	api.POST("/search/torrents/inspect", s.inspectTorrent)
}

// inspectRequest is POST /search/torrents/inspect. The hint takes the
// search's season and episode only.
type inspectRequest struct {
	MediaType string `json:"media_type"`
	TMDBID    int    `json:"tmdb_id"`
	ReleaseID string `json:"release_id"`
	MagnetURI string `json:"magnet_uri"`
	Hint      *struct {
		Season  *int `json:"season"`
		Episode *int `json:"episode"`
	} `json:"hint"`
}

// inspectTorrent creates a dry run for a search result or a magnet:
// 202 {job_id}, or 200 with the dry run of the same title and torrent from
// the last hour. A magnet_uri is used as it is; a release_id is resolved
// first, after everything that needs no network is checked.
func (s *Server) inspectTorrent(c *gin.Context) {
	if s.opts.Inspect == nil {
		errorResponse(c, http.StatusServiceUnavailable, "Inspect isn't configured")
		return
	}
	var body inspectRequest
	if err := c.ShouldBindJSON(&body); err != nil || (body.ReleaseID == "" && body.MagnetURI == "") {
		errorResponse(c, http.StatusBadRequest, "Send {media_type, tmdb_id, release_id | magnet_uri, hint?: {season?, episode?}}.")
		return
	}
	req := service.InspectRequest{
		MediaType: library.JobMediaType(body.MediaType),
		TMDBID:    body.TMDBID,
		MagnetURI: body.MagnetURI,
	}
	if body.Hint != nil {
		req.Hint = &library.JobHint{Season: body.Hint.Season, Episode: body.Hint.Episode}
	}
	if err := req.Validate(); err != nil {
		errorResponse(c, http.StatusBadRequest, err.Error())
		return
	}
	ctx := c.Request.Context()

	if req.MagnetURI == "" {
		if s.opts.Releases == nil {
			errorResponse(c, http.StatusNotFound, "This result has expired. Search again.")
			return
		}
		magnet, indexer, err := s.opts.Releases.Resolve(ctx, body.ReleaseID)
		if err != nil {
			resolveErrorResponse(c, err)
			return
		}
		req.MagnetURI, req.Indexer = magnet, indexer
	}

	jobID, created, err := s.opts.Inspect.Inspect(ctx, req)
	if err != nil {
		inspectErrorResponse(c, err)
		return
	}
	status := http.StatusAccepted
	if !created {
		status = http.StatusOK
	}
	c.JSON(status, jobAccepted{JobID: jobID})
}

// resolveErrorResponse maps a release resolver's errors (ReleaseResolver).
func resolveErrorResponse(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrReleaseNotFound):
		errorResponse(c, http.StatusNotFound, "This result has expired. Search again.")
	case errors.Is(err, context.Canceled):
		errorCodeResponse(c, StatusClientClosedRequest, CodeClientClosed, "The request was canceled")
	case errors.Is(err, torznab.ErrTimeout), errors.Is(err, context.DeadlineExceeded):
		errorResponse(c, http.StatusGatewayTimeout, "The indexer didn't send the torrent in time.")
	default:
		errorCodeResponse(c, http.StatusBadGateway, CodeTorrentUnavailable, "The indexer didn't give a torrent for this result.")
	}
}

// inspectErrorResponse maps Inspect's errors; the job service's others
// (library.ErrTMDBFetch included) map as for assigning.
func inspectErrorResponse(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrTitleNotFound):
		errorResponse(c, http.StatusNotFound, "TMDB has no such title.")
	case errors.Is(err, service.ErrInvalidInspect):
		errorResponse(c, http.StatusBadRequest, err.Error())
	default:
		assignmentErrorResponse(c, err)
	}
}
