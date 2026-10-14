package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Error codes used in the {"error": {"code", "message"}} response body.
const (
	CodeBadRequest      = "bad_request"
	CodeUnauthorized    = "unauthorized"
	CodeForbidden       = "forbidden"
	CodeNotFound        = "not_found"
	CodeConflict        = "conflict"
	CodeValidationError = "validation_error"
	CodeRateLimited     = "rate_limited"
	CodeInternalError   = "internal_error"
	CodeUnavailable     = "unavailable"
	CodeTimeout         = "timeout"
)

// ErrorBody is the JSON shape of every API error response.
type ErrorBody struct {
	Error ErrorDetail `json:"error"`
}

// ErrorDetail carries a stable machine-readable code and a human message.
type ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// errorResponse writes an error with a code derived from the HTTP status.
func errorResponse(c *gin.Context, status int, message string) {
	errorCodeResponse(c, status, errorCodeForStatus(status), message)
}

// errorCodeResponse writes an error with an explicit code, for cases where the
// status alone doesn't tell the client what went wrong.
func errorCodeResponse(c *gin.Context, status int, code, message string) {
	c.JSON(status, ErrorBody{Error: ErrorDetail{Code: code, Message: message}})
}

// errorCodeForStatus maps an HTTP status to the default error code.
func errorCodeForStatus(status int) string {
	switch status {
	case http.StatusBadRequest:
		return CodeBadRequest
	case http.StatusUnauthorized:
		return CodeUnauthorized
	case http.StatusForbidden:
		return CodeForbidden
	case http.StatusNotFound:
		return CodeNotFound
	case http.StatusConflict:
		return CodeConflict
	case http.StatusUnprocessableEntity:
		return CodeValidationError
	case http.StatusTooManyRequests:
		return CodeRateLimited
	case http.StatusServiceUnavailable:
		return CodeUnavailable
	case http.StatusGatewayTimeout:
		return CodeTimeout
	}
	if status >= 400 && status < 500 {
		return CodeBadRequest
	}
	return CodeInternalError
}
