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

// The legacy GET /shows/:id response hides specials when they're switched
// off, but the rows stay.
func TestToShowResponse_Specials(t *testing.T) {
	show := func(includeSpecials bool) *library.Show {
		return &library.Show{
			ID: 1, TMDBID: 2, Title: "Doctor Who", Year: 2005, IncludeSpecials: includeSpecials,
			Seasons: []library.Season{
				{ID: 10, SeasonNumber: 0, Episodes: []library.Episode{{ID: 100, EpisodeNumber: 1, Name: "Special"}}},
				{ID: 11, SeasonNumber: 1, Episodes: []library.Episode{
					{ID: 110, EpisodeNumber: 1, Name: "Rose", Assignment: &library.TorrentAssignment{ID: 5, FilePath: "rose.mkv"}},
				}},
			},
		}
	}
	tests := []struct {
		name            string
		includeSpecials bool
		wantSeasons     []int
	}{
		{"specials on", true, []int{0, 1}},
		{"specials off", false, []int{1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := toShowResponse(show(tt.includeSpecials))
			var got []int
			for _, s := range resp.Seasons {
				got = append(got, s.SeasonNumber)
			}
			if fmt.Sprint(got) != fmt.Sprint(tt.wantSeasons) {
				t.Errorf("seasons = %v, want %v", got, tt.wantSeasons)
			}
			last := resp.Seasons[len(resp.Seasons)-1].Episodes[0]
			if !last.HasAssignment || last.Assignment == nil || last.Assignment.FilePath != "rose.mkv" {
				t.Errorf("episode = %+v, want its assignment", last)
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
