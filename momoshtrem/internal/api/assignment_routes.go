package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/service"
)

// AssignmentJobs runs assignment jobs (PLAN §6 "Assignment").
// *service.JobService implements it.
type AssignmentJobs interface {
	Assign(ctx context.Context, mediaType library.JobMediaType, libraryID int64, req service.AssignRequest) (jobID int64, created bool, err error)
	AssignShowAndWait(ctx context.Context, showID int64, req service.AssignRequest) (*service.ShowAssignmentResult, error)
	AssignMovieAndWait(ctx context.Context, movieID int64, req service.AssignRequest) (*service.MovieAssignmentResult, error)
	Detail(ctx context.Context, id int64) (*service.JobDetail, error)
	List(ctx context.Context, statuses []library.JobStatus) ([]service.JobView, error)
	Apply(ctx context.Context, id int64) (*service.JobDetail, error)
	Discard(ctx context.Context, id int64) (*service.JobDetail, error)
}

var _ AssignmentJobs = (*service.JobService)(nil)

// registerAssignmentRoutes adds assigning torrents and the job endpoints.
func (s *Server) registerAssignmentRoutes(api *gin.RouterGroup) {
	api.POST("/shows/:id/assign-torrent", s.assignTorrent(library.JobTV))
	api.POST("/movies/:id/assign-torrent", s.assignTorrent(library.JobMovie))
	api.GET("/assignment-jobs", s.listAssignmentJobs)
	api.GET("/assignment-jobs/:id", s.getAssignmentJob)
	api.POST("/assignment-jobs/:id/apply", s.applyAssignmentJob)
	api.POST("/assignment-jobs/:id/discard", s.discardAssignmentJob)
}

// assignmentsReady answers 503 when no job service is configured.
func (s *Server) assignmentsReady(c *gin.Context) bool {
	if s.opts.Assignments == nil {
		errorResponse(c, http.StatusServiceUnavailable, "Torrent assignment not configured")
		return false
	}
	return true
}

// assignTorrentRequest is POST /shows/{id}/assign-torrent and
// /movies/{id}/assign-torrent. hint is for shows, file_path for movies.
type assignTorrentRequest struct {
	MagnetURI string           `json:"magnet_uri"`
	Hint      *library.JobHint `json:"hint"`
	Indexer   string           `json:"indexer"`
	FilePath  string           `json:"file_path"`
}

// jobAccepted is {job_id}: 202 for a new job, 200 for the open one.
type jobAccepted struct {
	JobID int64 `json:"job_id"`
}

// assignTorrent creates a job for the torrent: 202 {job_id}, or 200 with
// the open job for the same title and torrent. ?wait=true is the legacy
// synchronous assign: it waits, applies and answers 201 with the old
// response plus job_id.
func (s *Server) assignTorrent(mediaType library.JobMediaType) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := parseID(c, "id")
		if !ok || !s.assignmentsReady(c) {
			return
		}
		var body assignTorrentRequest
		if err := c.ShouldBindJSON(&body); err != nil || body.MagnetURI == "" {
			errorResponse(c, http.StatusBadRequest, "Send {magnet_uri, hint?, indexer?}.")
			return
		}
		req := service.AssignRequest{MagnetURI: body.MagnetURI, Hint: body.Hint, Indexer: body.Indexer, FilePath: body.FilePath}
		ctx := c.Request.Context()

		if c.Query("wait") == "true" {
			s.assignAndWait(c, mediaType, id, req)
			return
		}
		jobID, created, err := s.opts.Assignments.Assign(ctx, mediaType, id, req)
		if err != nil {
			assignmentErrorResponse(c, err)
			return
		}
		status := http.StatusAccepted
		if !created {
			status = http.StatusOK
		}
		c.JSON(status, jobAccepted{JobID: jobID})
	}
}

// assignAndWait is the legacy synchronous assign.
func (s *Server) assignAndWait(c *gin.Context, mediaType library.JobMediaType, id int64, req service.AssignRequest) {
	ctx := c.Request.Context()
	if mediaType == library.JobTV {
		res, err := s.opts.Assignments.AssignShowAndWait(ctx, id, req)
		if err != nil {
			assignmentErrorResponse(c, err)
			return
		}
		c.JSON(http.StatusCreated, ShowAssignmentResponse{
			Success: true, Summary: res.Summary, Matched: res.Matched, Unmatched: res.Unmatched, JobID: res.JobID,
		})
		return
	}
	res, err := s.opts.Assignments.AssignMovieAndWait(ctx, id, req)
	if err != nil {
		assignmentErrorResponse(c, err)
		return
	}
	c.JSON(http.StatusCreated, MovieAssignmentResponse{
		Success: true, Assignment: toAssignmentResponse(res.Assignment), JobID: res.JobID,
	})
}

// jobList is GET /assignment-jobs.
type jobList struct {
	Items []service.JobView `json:"items"`
}

// listAssignmentJobs lists jobs, newest first, without dry runs:
// ?status=open (the default) or a comma-separated list of statuses.
func (s *Server) listAssignmentJobs(c *gin.Context) {
	if !s.assignmentsReady(c) {
		return
	}
	statuses, err := service.ParseStatuses(c.Query("status"))
	if err != nil {
		errorResponse(c, http.StatusBadRequest, "Unknown status in \""+c.Query("status")+"\".")
		return
	}
	items, err := s.opts.Assignments.List(c.Request.Context(), statuses)
	if err != nil {
		assignmentErrorResponse(c, err)
		return
	}
	if items == nil {
		items = []service.JobView{}
	}
	c.JSON(http.StatusOK, jobList{Items: items})
}

func (s *Server) getAssignmentJob(c *gin.Context) {
	s.jobAction(c, func(ctx context.Context, id int64) (*service.JobDetail, error) {
		return s.opts.Assignments.Detail(ctx, id)
	})
}

func (s *Server) applyAssignmentJob(c *gin.Context) {
	s.jobAction(c, func(ctx context.Context, id int64) (*service.JobDetail, error) {
		return s.opts.Assignments.Apply(ctx, id)
	})
}

func (s *Server) discardAssignmentJob(c *gin.Context) {
	s.jobAction(c, func(ctx context.Context, id int64) (*service.JobDetail, error) {
		return s.opts.Assignments.Discard(ctx, id)
	})
}

// jobAction runs do on the job in the path and answers its detail.
func (s *Server) jobAction(c *gin.Context, do func(context.Context, int64) (*service.JobDetail, error)) {
	id, ok := parseID(c, "id")
	if !ok || !s.assignmentsReady(c) {
		return
	}
	detail, err := do(c.Request.Context(), id)
	if err != nil {
		assignmentErrorResponse(c, err)
		return
	}
	c.JSON(http.StatusOK, detail)
}

// failedJobStatus is the status a legacy assign (?wait=true) answers for a
// failed job: no video 400 (as the synchronous assign did), no torrent
// client 503, no file list in time 504, anything else 500.
func failedJobStatus(code string) int {
	switch code {
	case service.FailureNoVideos, service.FailureInvalidMagnet:
		return http.StatusBadRequest
	case service.FailureNoTorrents:
		return http.StatusServiceUnavailable
	case service.FailureMetadataTimeout:
		return http.StatusGatewayTimeout
	case service.FailureTitleGone:
		return http.StatusNotFound
	}
	return http.StatusInternalServerError
}

// assignmentErrorResponse maps the job service's errors to the contract's
// status codes.
func assignmentErrorResponse(c *gin.Context, err error) {
	var jobConflict *service.JobConflictError
	var failed *service.JobFailedError
	switch {
	case errors.As(err, &jobConflict):
		errorCodeResponse(c, http.StatusConflict, jobConflict.Code, jobConflict.Message)
	case errors.As(err, &failed):
		errorResponse(c, failedJobStatus(failed.Code), failed.Message)
	case errors.Is(err, library.ErrJobNotFound):
		errorResponse(c, http.StatusNotFound, "No such mapping job")
	case errors.Is(err, library.ErrShowNotFound):
		errorResponse(c, http.StatusNotFound, "Show not found")
	case errors.Is(err, library.ErrMovieNotFound):
		errorResponse(c, http.StatusNotFound, "Movie not found")
	case errors.Is(err, library.ErrInvalidMagnet):
		errorResponse(c, http.StatusBadRequest, "The magnet link has no BitTorrent info hash.")
	case errors.Is(err, service.ErrInvalidHint), errors.Is(err, service.ErrInvalidStatus):
		errorResponse(c, http.StatusBadRequest, err.Error())
	case errors.Is(err, library.ErrNoVideoFiles):
		errorResponse(c, http.StatusBadRequest, "No video files found in torrent")
	case errors.Is(err, context.Canceled):
		errorCodeResponse(c, StatusClientClosedRequest, CodeClientClosed, "The request was canceled")
	case errors.Is(err, context.DeadlineExceeded):
		errorResponse(c, http.StatusGatewayTimeout, "The files are still being matched; check the mapping job.")
	default:
		slog.Error("Assignment request failed", "path", c.Request.URL.Path, "error", err)
		errorResponse(c, http.StatusInternalServerError, "Something went wrong with the mapping job")
	}
}
