package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestErrorCodeForStatus(t *testing.T) {
	tests := []struct {
		status int
		want   string
	}{
		{http.StatusBadRequest, "bad_request"},
		{http.StatusUnauthorized, "unauthorized"},
		{http.StatusForbidden, "forbidden"},
		{http.StatusNotFound, "not_found"},
		{http.StatusConflict, "conflict"},
		{http.StatusUnprocessableEntity, "validation_error"},
		{http.StatusTooManyRequests, "rate_limited"},
		{http.StatusInternalServerError, "internal_error"},
		{http.StatusBadGateway, "internal_error"},
		{http.StatusServiceUnavailable, "unavailable"},
		{http.StatusGatewayTimeout, "timeout"},
		{http.StatusMethodNotAllowed, "bad_request"},
	}
	for _, tt := range tests {
		t.Run(http.StatusText(tt.status), func(t *testing.T) {
			if got := errorCodeForStatus(tt.status); got != tt.want {
				t.Errorf("errorCodeForStatus(%d) = %q, want %q", tt.status, got, tt.want)
			}
		})
	}
}

func TestErrorResponseShape(t *testing.T) {
	tests := []struct {
		name     string
		write    func(c *gin.Context)
		status   int
		wantBody string
	}{
		{
			name:     "derived code",
			write:    func(c *gin.Context) { errorResponse(c, http.StatusNotFound, "Movie not found") },
			status:   http.StatusNotFound,
			wantBody: `{"error":{"code":"not_found","message":"Movie not found"}}`,
		},
		{
			name:     "explicit code",
			write:    func(c *gin.Context) { errorCodeResponse(c, http.StatusConflict, "already_assigned", "Torrent already assigned") },
			status:   http.StatusConflict,
			wantBody: `{"error":{"code":"already_assigned","message":"Torrent already assigned"}}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			tt.write(c)

			if rec.Code != tt.status {
				t.Errorf("status = %d, want %d", rec.Code, tt.status)
			}
			if got := rec.Body.String(); got != tt.wantBody {
				t.Errorf("body = %s, want %s", got, tt.wantBody)
			}
		})
	}
}
