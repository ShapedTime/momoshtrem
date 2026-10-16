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
	Reclassify(ctx context.Context, id int64, req service.ReclassifyRequest) error
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
	api.POST("/assignment-jobs/:id/reclassify", s.reclassifyAssignmentJob)
}

// assignmentsReady answers 503 when no job service is configured.
func (s *Server) assignmentsReady(c *gin.Context) bool {
	if s.opts.Assignments == nil {
		errorResponse(c, http.StatusServiceUnavailable, "Torrent assignment not configured")
		return false
	}
	return true
}

// TorrentMagnets finds a magnet for a torrent the library knows: an
// assignment's, else a job's; "" when it knows none.
// *library.AssignmentJobRepository implements it.
type TorrentMagnets interface {
	TorrentMagnet(ctx context.Context, infoHash string) (string, error)
}

var _ TorrentMagnets = (*library.AssignmentJobRepository)(nil)

// assignTorrentRequest is POST /shows/{id}/assign-torrent and
// /movies/{id}/assign-torrent. hint is for shows, file_path for movies.
// info_hash instead of magnet_uri re-runs the mapping of a torrent the
// library knows (magnet_uri wins when both are sent).
type assignTorrentRequest struct {
	MagnetURI string           `json:"magnet_uri"`
	InfoHash  string           `json:"info_hash"`
	Hint      *library.JobHint `json:"hint"`
	Indexer   string           `json:"indexer"`
	FilePath  string           `json:"file_path"`
}

// jobAccepted is {job_id}: 202 for a new job, 200 for the open one.
type jobAccepted struct {
	JobID int64 `json:"job_id"`
}

// assignTorrent creates a job for the torrent: 202 {job_id}, or 200 with
// the open job for the same title and torrent. {info_hash} instead of a
// magnet re-runs the mapping of a torrent the library knows, with its
// stored magnet. ?wait=true is the legacy synchronous assign: it waits,
// applies and answers 201 with the old response plus job_id.
func (s *Server) assignTorrent(mediaType library.JobMediaType) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := parseID(c, "id")
		if !ok || !s.assignmentsReady(c) {
			return
		}
		var body assignTorrentRequest
		if err := c.ShouldBindJSON(&body); err != nil || (body.MagnetURI == "" && body.InfoHash == "") {
			errorResponse(c, http.StatusBadRequest, "Send {magnet_uri | info_hash, hint?, indexer?}.")
			return
		}
		ctx := c.Request.Context()
		magnet := body.MagnetURI
		if magnet == "" {
			if magnet, ok = s.knownMagnet(c, body.InfoHash); !ok {
				return
			}
		}
		req := service.AssignRequest{MagnetURI: magnet, Hint: body.Hint, Indexer: body.Indexer, FilePath: body.FilePath}

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

// knownMagnet finds the magnet of a torrent the library knows, for an
// assign by info_hash: 400 for a malformed hash, 404 for one the library
// doesn't know.
func (s *Server) knownMagnet(c *gin.Context, infoHash string) (string, bool) {
	hash := torrent.NormalizeInfoHash(infoHash)
	if hash == "" {
		errorResponse(c, http.StatusBadRequest, "Not a BitTorrent info hash: send 40 hex characters or 32 base32 ones")
		return "", false
	}
	if s.opts.Magnets == nil {
		errorResponse(c, http.StatusServiceUnavailable, "Re-running a mapping isn't configured")
		return "", false
	}
	magnet, err := s.opts.Magnets.TorrentMagnet(c.Request.Context(), hash)
	if err != nil {
		slog.Error("Failed to find a torrent's magnet", "info_hash", hash, "error", err)
		errorResponse(c, http.StatusInternalServerError, "Couldn't look the torrent up")
		return "", false
	}
	if magnet == "" {
		errorResponse(c, http.StatusNotFound, "Torrent not found")
		return "", false
	}
	return magnet, true
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

// reclassifyRequest is POST /assignment-jobs/{id}/reclassify.
type reclassifyRequest struct {
	Scope string  `json:"scope"`
	Hint  *string `json:"hint"`
}

// reclassifyAssignmentJob starts a re-check with the AI: 202 {job_id}, the
// job in resolving.
func (s *Server) reclassifyAssignmentJob(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok || !s.assignmentsReady(c) {
		return
	}
	var body reclassifyRequest
	if err := c.ShouldBindJSON(&body); err != nil ||
		(body.Scope != string(service.RecheckUnresolved) && body.Scope != string(service.RecheckAll)) {
		errorResponse(c, http.StatusBadRequest, `Send {scope: "unresolved" | "all", hint?}.`)
		return
	}
	req := service.ReclassifyRequest{Scope: service.RecheckScope(body.Scope)}
	if body.Hint != nil {
		req.Hint = *body.Hint
	}
	if err := s.opts.Assignments.Reclassify(c.Request.Context(), id, req); err != nil {
		assignmentErrorResponse(c, err)
		return
	}
	c.JSON(http.StatusAccepted, jobAccepted{JobID: id})
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
	case errors.Is(err, service.ErrInvalidHint), errors.Is(err, service.ErrInvalidStatus), errors.Is(err, service.ErrInvalidRecheck):
		errorResponse(c, http.StatusBadRequest, err.Error())
	case errors.Is(err, service.ErrAIUnavailable):
		errorResponse(c, http.StatusServiceUnavailable, "AI isn't available for this job.")
	case errors.Is(err, library.ErrNoVideoFiles):
		errorResponse(c, http.StatusBadRequest, "No video files found in torrent")
	case errors.Is(err, library.ErrTMDBFetch): // a dry run outside the library reads TMDB
		errorCodeResponse(c, http.StatusBadGateway, CodeTMDBUnavailable, "TMDB couldn't be reached. Try again in a moment.")
	case errors.Is(err, context.Canceled):
		errorCodeResponse(c, StatusClientClosedRequest, CodeClientClosed, "The request was canceled")
	case errors.Is(err, context.DeadlineExceeded):
		errorResponse(c, http.StatusGatewayTimeout, "The files are still being matched; check the mapping job.")
	default:
		slog.Error("Assignment request failed", "path", c.Request.URL.Path, "error", err)
		errorResponse(c, http.StatusInternalServerError, "Something went wrong with the mapping job")
	}
}
