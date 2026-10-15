package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/service"
)

// AssignmentReview is the mapping review (PLAN §6 "Assignment"): proposal
// edits, the episode picker's list, manual overrides and a torrent's files.
// *service.JobService implements it.
type AssignmentReview interface {
	PatchProposals(ctx context.Context, jobID int64, changes []service.ProposalChange) (*service.JobDetail, error)
	ShowEpisodes(ctx context.Context, showID int64) ([]service.ShowEpisodesSeason, error)
	OverrideEpisode(ctx context.Context, episodeID int64, infoHash, filePath string) error
	OverrideMovie(ctx context.Context, movieID int64, infoHash, filePath string) error
	TorrentFiles(ctx context.Context, infoHash string) ([]service.TorrentFileView, error)
}

var _ AssignmentReview = (*service.JobService)(nil)

// registerReviewRoutes adds the review endpoints.
func (s *Server) registerReviewRoutes(api *gin.RouterGroup) {
	api.PATCH("/assignment-jobs/:id/proposals", s.patchProposals)
	api.GET("/shows/:id/episodes", s.listShowEpisodes)
	api.PUT("/episodes/:id/assignment", s.overrideAssignment(library.ItemTypeEpisode))
	api.PUT("/movies/:id/assignment", s.overrideAssignment(library.ItemTypeMovie))
	api.GET("/torrents/:hash/files", s.listTorrentFiles)
}

// reviewReady answers 503 when no review service is configured.
func (s *Server) reviewReady(c *gin.Context) bool {
	if s.opts.Review == nil {
		errorResponse(c, http.StatusServiceUnavailable, "Torrent assignment not configured")
		return false
	}
	return true
}

// proposalChangeBody is one item of PATCH /assignment-jobs/{id}/proposals.
// Absent (or null) fields stay as they are.
type proposalChangeBody struct {
	ID         int64    `json:"id"`
	Decision   *string  `json:"decision"`
	EpisodeIDs *[]int64 `json:"episode_ids"`
}

type proposalsPatchBody struct {
	Items []proposalChangeBody `json:"items"`
}

const patchUsage = "Send {items: [{id, decision?, episode_ids?}]}, each with a decision or a non-empty episode_ids."

// parseChanges reads a well-formed PATCH body: items, each with an id and
// a known decision or non-empty episode_ids (or both).
func parseChanges(body proposalsPatchBody) ([]service.ProposalChange, bool) {
	if len(body.Items) == 0 {
		return nil, false
	}
	changes := make([]service.ProposalChange, 0, len(body.Items))
	for _, item := range body.Items {
		if item.ID <= 0 || (item.Decision == nil && item.EpisodeIDs == nil) {
			return nil, false
		}
		change := service.ProposalChange{ID: item.ID}
		if item.Decision != nil {
			d := library.Decision(*item.Decision)
			switch d {
			case library.DecisionPending, library.DecisionAccepted, library.DecisionIgnored:
			default:
				return nil, false
			}
			change.Decision = &d
		}
		if item.EpisodeIDs != nil {
			if len(*item.EpisodeIDs) == 0 {
				return nil, false
			}
			change.EpisodeIDs = *item.EpisodeIDs
		}
		changes = append(changes, change)
	}
	return changes, true
}

// patchProposals changes a job's proposals, all or nothing, and answers
// the job's detail.
func (s *Server) patchProposals(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok || !s.reviewReady(c) {
		return
	}
	var body proposalsPatchBody
	if err := c.ShouldBindJSON(&body); err != nil {
		errorResponse(c, http.StatusBadRequest, patchUsage)
		return
	}
	changes, ok := parseChanges(body)
	if !ok {
		errorResponse(c, http.StatusBadRequest, patchUsage)
		return
	}
	detail, err := s.opts.Review.PatchProposals(c.Request.Context(), id, changes)
	if err != nil {
		reviewErrorResponse(c, err)
		return
	}
	c.JSON(http.StatusOK, detail)
}

// showEpisodesList is GET /shows/{id}/episodes.
type showEpisodesList struct {
	Items []service.ShowEpisodesSeason `json:"items"`
}

// listShowEpisodes lists every season of a show, Specials included, for
// the episode picker.
func (s *Server) listShowEpisodes(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok || !s.reviewReady(c) {
		return
	}
	items, err := s.opts.Review.ShowEpisodes(c.Request.Context(), id)
	if err != nil {
		reviewErrorResponse(c, err)
		return
	}
	if items == nil {
		items = []service.ShowEpisodesSeason{}
	}
	c.JSON(http.StatusOK, showEpisodesList{Items: items})
}

// manualAssignmentBody is PUT /episodes/{id}/assignment and
// /movies/{id}/assignment.
type manualAssignmentBody struct {
	InfoHash string `json:"info_hash"`
	FilePath string `json:"file_path"`
}

// overrideAssignment makes a torrent's file the item's file, by hand:
// 204.
func (s *Server) overrideAssignment(itemType library.ItemType) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := parseID(c, "id")
		if !ok || !s.reviewReady(c) {
			return
		}
		var body manualAssignmentBody
		if err := c.ShouldBindJSON(&body); err != nil ||
			strings.TrimSpace(body.InfoHash) == "" || strings.TrimSpace(body.FilePath) == "" {
			errorResponse(c, http.StatusBadRequest, "Send {info_hash, file_path}.")
			return
		}
		ctx := c.Request.Context()
		hash := strings.TrimSpace(body.InfoHash)
		var err error
		if itemType == library.ItemTypeEpisode {
			err = s.opts.Review.OverrideEpisode(ctx, id, hash, body.FilePath)
		} else {
			err = s.opts.Review.OverrideMovie(ctx, id, hash, body.FilePath)
		}
		if err != nil {
			reviewErrorResponse(c, err)
			return
		}
		c.Status(http.StatusNoContent)
	}
}

// torrentFileList is GET /torrents/{hash}/files.
type torrentFileList struct {
	Items []service.TorrentFileView `json:"items"`
}

// listTorrentFiles lists a torrent's files and what each is assigned to.
func (s *Server) listTorrentFiles(c *gin.Context) {
	if !s.reviewReady(c) {
		return
	}
	items, err := s.opts.Review.TorrentFiles(c.Request.Context(), c.Param("hash"))
	if err != nil {
		reviewErrorResponse(c, err)
		return
	}
	if items == nil {
		items = []service.TorrentFileView{}
	}
	c.JSON(http.StatusOK, torrentFileList{Items: items})
}

// reviewErrorResponse maps the review endpoints' errors to the contract's
// status codes; the job errors as for the other job endpoints.
func reviewErrorResponse(c *gin.Context, err error) {
	var invalid *service.ValidationError
	switch {
	case errors.As(err, &invalid):
		errorResponse(c, http.StatusUnprocessableEntity, invalid.Message)
	case errors.Is(err, library.ErrEpisodeNotFound):
		errorResponse(c, http.StatusNotFound, "Episode not found")
	case errors.Is(err, service.ErrUnknownTorrent):
		errorResponse(c, http.StatusNotFound, "No such torrent")
	case errors.Is(err, service.ErrFileNotInTorrent):
		errorResponse(c, http.StatusNotFound, "The torrent has no such file")
	case errors.Is(err, service.ErrFileListTimeout):
		errorResponse(c, http.StatusGatewayTimeout, "No peers sent the torrent's file list in time. Try again in a minute.")
	case errors.Is(err, library.ErrTorrentServiceUnavailable):
		errorResponse(c, http.StatusServiceUnavailable, "Torrents aren't available, so the file list can't be fetched.")
	case errors.Is(err, context.DeadlineExceeded):
		slog.Warn("Review request timed out", "path", c.Request.URL.Path, "error", err)
		errorResponse(c, http.StatusGatewayTimeout, "That took too long; try again.")
	default:
		assignmentErrorResponse(c, err)
	}
}
