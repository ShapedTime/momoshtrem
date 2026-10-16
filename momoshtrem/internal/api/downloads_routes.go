package api

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/shapedtime/momoshtrem/internal/downloads"
	"github.com/shapedtime/momoshtrem/internal/library"
)

// Error codes of /downloads (PLAN §6 "Downloads (#77)"): 409 storage_full,
// a Keep the kept cap can't take; 422 not_downloadable, a Keep on files
// that all play from HDRezka.
const (
	CodeStorageFull     = "storage_full"
	CodeNotDownloadable = "not_downloadable"
)

// Downloads serves /downloads: the Keeps and smart picks, the estimate, and
// a kept download's duration. *downloads.Service implements it.
type Downloads interface {
	List(ctx context.Context) (downloads.List, error)
	Estimate(ctx context.Context, target library.DownloadTarget) (downloads.Estimate, error)
	// Keep is POST's body checked and kept; created says it's new (201).
	Keep(ctx context.Context, body []byte) (downloads.Download, bool, error)
	Update(ctx context.Context, id int64, body []byte) (downloads.Download, error)
	Delete(ctx context.Context, id int64) error
}

var _ Downloads = (*downloads.Service)(nil)

// registerDownloadRoutes adds /downloads. Without Options.Downloads they
// answer 503.
func (s *Server) registerDownloadRoutes(api *gin.RouterGroup) {
	api.GET("/downloads", s.listDownloads)
	api.GET("/downloads/estimate", s.estimateDownload)
	api.POST("/downloads", s.keepDownload)
	api.PATCH("/downloads/:id", s.patchDownload)
	api.DELETE("/downloads/:id", s.deleteDownload)
}

func (s *Server) downloadsReady(c *gin.Context) bool {
	if s.opts.Downloads == nil {
		errorResponse(c, http.StatusServiceUnavailable, "Downloads aren't available")
		return false
	}
	return true
}

// GET /downloads → 200 DownloadList
func (s *Server) listDownloads(c *gin.Context) {
	if !s.downloadsReady(c) {
		return
	}
	list, err := s.opts.Downloads.List(c.Request.Context())
	if err != nil {
		downloadsErrorResponse(c, err)
		return
	}
	c.JSON(http.StatusOK, list)
}

// GET /downloads/estimate?scope=&id=[&season_number=] → 200
// DownloadEstimate; 400 for a malformed target, 404 when the library
// hasn't got it.
func (s *Server) estimateDownload(c *gin.Context) {
	if !s.downloadsReady(c) {
		return
	}
	target, err := downloads.ParseTarget(c.Query("scope"), c.Query("id"), c.Query("season_number"))
	if err != nil {
		errorResponse(c, http.StatusBadRequest, err.Error())
		return
	}
	est, err := s.opts.Downloads.Estimate(c.Request.Context(), target)
	if err != nil {
		downloadsErrorResponse(c, err)
		return
	}
	c.JSON(http.StatusOK, est)
}

// POST /downloads with {scope, id, season_number?, until_watched?,
// expires_at?} → 201 Download (a new Keep), or 200 (already kept, or a
// smart pick turned kept).
func (s *Server) keepDownload(c *gin.Context) {
	if !s.downloadsReady(c) {
		return
	}
	body, ok := readDownloadBody(c)
	if !ok {
		return
	}
	d, created, err := s.opts.Downloads.Keep(c.Request.Context(), body)
	if err != nil {
		downloadsErrorResponse(c, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	c.JSON(status, d)
}

// PATCH /downloads/:id with {until_watched?, expires_at?} → 200 Download;
// kept downloads only.
func (s *Server) patchDownload(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok || !s.downloadsReady(c) {
		return
	}
	body, ok := readDownloadBody(c)
	if !ok {
		return
	}
	d, err := s.opts.Downloads.Update(c.Request.Context(), id, body)
	if err != nil {
		downloadsErrorResponse(c, err)
		return
	}
	c.JSON(http.StatusOK, d)
}

// DELETE /downloads/:id → 204: the download is released (deleted).
func (s *Server) deleteDownload(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok || !s.downloadsReady(c) {
		return
	}
	if err := s.opts.Downloads.Delete(c.Request.Context(), id); err != nil {
		downloadsErrorResponse(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func readDownloadBody(c *gin.Context) ([]byte, bool) {
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, 64<<10))
	if err != nil {
		errorResponse(c, http.StatusBadRequest, "Couldn't read the body.")
		return nil, false
	}
	return body, true
}

// storageFullBody is the 409's body: the error with what the Keep needs
// and what's left.
type storageFullBody struct {
	Error storageFullDetail `json:"error"`
}

type storageFullDetail struct {
	Code           string `json:"code"`
	Message        string `json:"message"`
	NeededBytes    int64  `json:"needed_bytes"`
	AvailableBytes int64  `json:"available_bytes"`
}

func downloadsErrorResponse(c *gin.Context, err error) {
	var field *downloads.FieldError
	var full *downloads.StorageFullError
	switch {
	case errors.Is(err, downloads.ErrNotAnObject):
		errorResponse(c, http.StatusBadRequest, "Send the download as a JSON object.")
	case errors.As(err, &field):
		c.JSON(http.StatusUnprocessableEntity, ErrorBody{Error: ErrorDetail{Code: CodeValidationError, Message: field.Message, Field: field.Field}})
	case errors.Is(err, downloads.ErrSmartPick):
		errorResponse(c, http.StatusUnprocessableEntity, "A smart pick has no duration of its own: keep it first.")
	case errors.Is(err, downloads.ErrTargetNotFound):
		errorResponse(c, http.StatusNotFound, "That isn't in the library.")
	case errors.Is(err, downloads.ErrNotFound):
		errorResponse(c, http.StatusNotFound, "No such download.")
	case errors.Is(err, downloads.ErrNotDownloadable):
		errorCodeResponse(c, http.StatusUnprocessableEntity, CodeNotDownloadable,
			"This plays from HDRezka, so it can't be kept on disk. Pick a torrent for it to keep it.")
	case errors.As(err, &full):
		c.JSON(http.StatusConflict, storageFullBody{Error: storageFullDetail{
			Code: CodeStorageFull, Message: full.Error(), NeededBytes: full.NeededBytes, AvailableBytes: full.AvailableBytes,
		}})
	case errors.Is(err, context.Canceled):
		errorCodeResponse(c, StatusClientClosedRequest, CodeClientClosed, "The request was canceled")
	default:
		slog.Error("Downloads request failed", "method", c.Request.Method, "path", c.Request.URL.Path, "error", err)
		errorResponse(c, http.StatusInternalServerError, "Couldn't read or change the downloads.")
	}
}
