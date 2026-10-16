package api

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/storagecap"
)

// CodeLimitBelowCommitted is a 409 from PUT /settings/storage: the change
// would cap kept downloads below what they already want.
const CodeLimitBelowCommitted = "limit_below_committed"

// Storage serves the storage settings (PLAN §6 "Storage (#77)"): the piece
// cache's limit, and the caps of kept and smart downloads.
// *storagecap.Governor implements it.
type Storage interface {
	// Storage is the limit, where it comes from, the disk and the usage.
	Storage(ctx context.Context) (*storagecap.Report, error)
	// UpdateStorage applies the fields set, answering the new report; a
	// bad value is a *storagecap.FieldError, a kept cap below what's
	// committed a *storagecap.CommittedError.
	UpdateStorage(ctx context.Context, patch library.StorageSettingsPatch) (*storagecap.Report, error)
}

var _ Storage = (*storagecap.Governor)(nil)

// registerStorageRoutes adds GET and PUT /settings/storage. Without
// Options.Storage they answer 503.
func (s *Server) registerStorageRoutes(api *gin.RouterGroup) {
	api.GET("/settings/storage", s.getStorage)
	api.PUT("/settings/storage", s.putStorage)
}

func (s *Server) storageReady(c *gin.Context) bool {
	if s.opts.Storage == nil {
		errorResponse(c, http.StatusServiceUnavailable, "Storage settings aren't available")
		return false
	}
	return true
}

// GET /settings/storage → Report
func (s *Server) getStorage(c *gin.Context) {
	if !s.storageReady(c) {
		return
	}
	report, err := s.opts.Storage.Storage(c.Request.Context())
	if err == nil && report == nil {
		err = errors.New("no storage report")
	}
	if err != nil {
		storageErrorResponse(c, err)
		return
	}
	c.JSON(http.StatusOK, report)
}

// PUT /settings/storage with any of limit_bytes, kept, smart and background
// → 200 Report. Only the fields sent change and the nested objects merge;
// unknown fields are ignored. limit_bytes goes only when the limit control
// changes: GET's null may mean config.yaml (source). A body that isn't a JSON object
// is 400, a bad value 422 naming the field, a kept cap below what kept
// downloads want 409 limit_below_committed.
func (s *Server) putStorage(c *gin.Context) {
	if !s.storageReady(c) {
		return
	}
	raw, err := io.ReadAll(io.LimitReader(c.Request.Body, 64<<10))
	if err != nil {
		errorResponse(c, http.StatusBadRequest, "Couldn't read the body.")
		return
	}
	patch, err := storagecap.ParsePatch(raw)
	if err != nil {
		storageErrorResponse(c, err)
		return
	}
	report, err := s.opts.Storage.UpdateStorage(c.Request.Context(), patch)
	if err == nil && report == nil {
		err = errors.New("no storage report")
	}
	if err != nil {
		storageErrorResponse(c, err)
		return
	}
	c.JSON(http.StatusOK, report)
}

// committedErrorBody is the 409's body: the error with what's committed.
type committedErrorBody struct {
	Error committedErrorDetail `json:"error"`
}

type committedErrorDetail struct {
	Code               string `json:"code"`
	Message            string `json:"message"`
	CommittedKeptBytes int64  `json:"committed_kept_bytes"`
}

func storageErrorResponse(c *gin.Context, err error) {
	var field *storagecap.FieldError
	var committed *storagecap.CommittedError
	switch {
	case errors.As(err, &field):
		c.JSON(http.StatusUnprocessableEntity, ErrorBody{Error: ErrorDetail{Code: CodeValidationError, Message: field.Message, Field: field.Field}})
	case errors.As(err, &committed):
		c.JSON(http.StatusConflict, committedErrorBody{Error: committedErrorDetail{
			Code: CodeLimitBelowCommitted, Message: committed.Error(), CommittedKeptBytes: committed.CommittedKeptBytes,
		}})
	case errors.Is(err, storagecap.ErrNotAnObject):
		errorResponse(c, http.StatusBadRequest, "Send the settings to change as a JSON object.")
	case errors.Is(err, context.Canceled):
		errorCodeResponse(c, StatusClientClosedRequest, CodeClientClosed, "The request was canceled")
	default:
		slog.Error("Storage settings request failed", "path", c.Request.URL.Path, "error", err)
		errorResponse(c, http.StatusInternalServerError, "Couldn't read or save the storage settings.")
	}
}
