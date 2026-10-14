package api

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/tmdb"
)

func TestMetadataErrorResponse(t *testing.T) {
	// The Syncer wraps TMDB failures in library.ErrTMDBFetch.
	tmdbErr := func(err error) error { return fmt.Errorf("%w: %w", library.ErrTMDBFetch, err) }

	tests := []struct {
		name         string
		err          error
		tmdbNotFound int
		wantStatus   int
		wantCode     string
	}{
		{"movie missing", library.ErrMovieNotFound, http.StatusNotFound, http.StatusNotFound, CodeNotFound},
		{"show missing", fmt.Errorf("load: %w", library.ErrShowNotFound), http.StatusNotFound, http.StatusNotFound, CodeNotFound},
		{"no TMDB key", tmdbErr(tmdb.ErrNoAPIKey), http.StatusBadRequest, http.StatusServiceUnavailable, CodeUnavailable},
		{"unknown TMDB ID on add", tmdbErr(tmdb.ErrNotFound), http.StatusBadRequest, http.StatusBadRequest, CodeBadRequest},
		{"unknown TMDB ID on refresh", tmdbErr(tmdb.ErrNotFound), http.StatusNotFound, http.StatusNotFound, CodeNotFound},
		{"TMDB down", tmdbErr(&tmdb.StatusError{StatusCode: 503}), http.StatusBadRequest, http.StatusBadGateway, CodeTMDBUnavailable},
		{"invalid settings", fmt.Errorf("%w: bad", library.ErrInvalidShowSettings), http.StatusBadRequest, http.StatusUnprocessableEntity, CodeValidationError},
		{"database", errors.New("connection reset"), http.StatusBadRequest, http.StatusInternalServerError, CodeInternalError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			metadataErrorResponse(c, tt.err, tt.tmdbNotFound)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if got := decodeError(t, rec); got.Code != tt.wantCode {
				t.Errorf("code = %q, want %q", got.Code, tt.wantCode)
			}
		})
	}
}

func TestMetadataRoutes_WithoutSyncer(t *testing.T) {
	s := newTestServer(Options{})
	tests := []struct {
		method, path, body string
	}{
		{http.MethodPost, "/api/movies", `{"tmdb_id": 1}`},
		{http.MethodPost, "/api/movies/1/refresh", ``},
		{http.MethodPost, "/api/shows", `{"tmdb_id": 1}`},
		{http.MethodPost, "/api/shows/1/refresh", ``},
	}
	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			s.Handler().ServeHTTP(rec, req)
			if rec.Code != http.StatusServiceUnavailable {
				t.Errorf("status = %d, want 503 (body %s)", rec.Code, rec.Body.String())
			}
		})
	}
}
