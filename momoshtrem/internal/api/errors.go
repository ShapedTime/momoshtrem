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
	// CodeTMDBUnavailable is a 502: TMDB failed and nothing was cached.
	CodeTMDBUnavailable = "tmdb_unavailable"
	// CodeClientClosed goes with StatusClientClosedRequest: the client went away first.
	CodeClientClosed = "client_closed"
	// CodeHDRezkaUnavailable is a 502: HDRezka or its CDN failed (a CDN
	// 403 included: the API never answers 401 or 403 for HDRezka).
	CodeHDRezkaUnavailable = "hdrezka_unavailable"
	// CodeHDRezkaBlocked is a 503: HDRezka answered with its anti-bot page.
	CodeHDRezkaBlocked = "hdrezka_blocked"
	// CodePremiumOnly is a 409: the translation or quality needs an
	// HDRezka Premium account.
	CodePremiumOnly = "premium_only"
)

// StatusClientClosedRequest (nginx's 499) answers a request the client
// canceled, so it isn't logged or counted as a server error.
const StatusClientClosedRequest = 499

// ErrorBody is the JSON shape of every API error response.
type ErrorBody struct {
	Error ErrorDetail `json:"error"`
}

// ErrorDetail carries a stable machine-readable code and a human message.
type ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	// Field is the JSON name of the field a 422 is about, where the
	// endpoint says so (the quality profile), so the UI can show the
	// message next to it.
	Field string `json:"field,omitempty"`
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
