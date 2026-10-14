package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/shapedtime/momoshtrem/internal/catalog"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/metadata"
	"github.com/shapedtime/momoshtrem/internal/tmdb"
)

// CodeNotInLibrary is a 404 for a show that isn't in the library, so it has
// no seasons to list.
const CodeNotInLibrary = "not_in_library"

// Catalog builds title pages and the library list. *catalog.Service
// implements it.
type Catalog interface {
	MovieTitle(ctx context.Context, tmdbID int) (*catalog.MovieTitle, error)
	ShowTitle(ctx context.Context, tmdbID int) (*catalog.ShowTitle, error)
	Season(ctx context.Context, tmdbID, seasonNumber int) (*catalog.SeasonDetail, error)
	Library(ctx context.Context, p catalog.LibraryParams) (*catalog.LibraryPage, error)
	PatchShow(ctx context.Context, showID int64, patch metadata.SettingsPatch) (*catalog.ShowTitle, error)
	EpisodeGroups(ctx context.Context, showID int64) ([]catalog.EpisodeGroup, error)
}

var _ Catalog = (*catalog.Service)(nil)

// registerTitleRoutes adds the title pages and show settings (PLAN §6
// "Titles and library").
func (s *Server) registerTitleRoutes(api *gin.RouterGroup) {
	api.GET("/titles/movie/:tmdb_id", s.getMovieTitle)
	api.GET("/titles/tv/:tmdb_id", s.getShowTitle)
	api.GET("/titles/tv/:tmdb_id/seasons/:season", s.getShowSeason)
	api.PATCH("/shows/:id", s.patchShow)
	api.GET("/shows/:id/episode-groups", s.getEpisodeGroups)
}

// catalogReady answers 503 when no catalog is configured.
func (s *Server) catalogReady(c *gin.Context) bool {
	if s.opts.Catalog == nil {
		errorResponse(c, http.StatusServiceUnavailable, "Title service not configured")
		return false
	}
	return true
}

func (s *Server) getMovieTitle(c *gin.Context) {
	tmdbID, ok := parseID(c, "tmdb_id")
	if !ok || !s.catalogReady(c) {
		return
	}
	title, err := s.opts.Catalog.MovieTitle(c.Request.Context(), int(tmdbID))
	if err != nil {
		catalogErrorResponse(c, err)
		return
	}
	c.JSON(http.StatusOK, title)
}

func (s *Server) getShowTitle(c *gin.Context) {
	tmdbID, ok := parseID(c, "tmdb_id")
	if !ok || !s.catalogReady(c) {
		return
	}
	title, err := s.opts.Catalog.ShowTitle(c.Request.Context(), int(tmdbID))
	if err != nil {
		catalogErrorResponse(c, err)
		return
	}
	c.JSON(http.StatusOK, title)
}

func (s *Server) getShowSeason(c *gin.Context) {
	tmdbID, ok := parseID(c, "tmdb_id")
	if !ok {
		return
	}
	season, err := strconv.Atoi(c.Param("season"))
	if err != nil || season < 0 {
		errorResponse(c, http.StatusBadRequest, "Season must be a number, 0 for specials")
		return
	}
	if !s.catalogReady(c) {
		return
	}
	detail, err := s.opts.Catalog.Season(c.Request.Context(), int(tmdbID), season)
	if err != nil {
		catalogErrorResponse(c, err)
		return
	}
	c.JSON(http.StatusOK, detail)
}

// patchShowRequest is PATCH /shows/{id}. Absent fields are kept;
// episode_group_id null clears the group.
type patchShowRequest struct {
	EpisodeOrdering *string        `json:"episode_ordering"`
	EpisodeGroupID  optionalString `json:"episode_group_id"`
	IncludeSpecials *bool          `json:"include_specials"`
	ClassifierMode  *string        `json:"classifier_mode"`
}

// optionalString tells an absent field from null: Set is true when the field
// is present; null leaves Value "".
type optionalString struct {
	Set   bool
	Value string
}

func (o *optionalString) UnmarshalJSON(data []byte) error {
	o.Set = true
	if bytes.Equal(data, []byte("null")) {
		o.Value = ""
		return nil
	}
	return json.Unmarshal(data, &o.Value)
}

func (r patchShowRequest) toPatch() metadata.SettingsPatch {
	var p metadata.SettingsPatch
	if r.EpisodeOrdering != nil {
		o := library.EpisodeOrdering(*r.EpisodeOrdering)
		p.EpisodeOrdering = &o
	}
	if r.EpisodeGroupID.Set {
		p.EpisodeGroupID = &r.EpisodeGroupID.Value
	}
	p.IncludeSpecials = r.IncludeSpecials
	if r.ClassifierMode != nil {
		m := library.ClassifierMode(*r.ClassifierMode)
		p.ClassifierMode = &m
	}
	return p
}

func (s *Server) patchShow(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	var req patchShowRequest
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		errorResponse(c, http.StatusBadRequest, "Failed to read the request body")
		return
	}
	if err := json.Unmarshal(body, &req); err != nil {
		var typeErr *json.UnmarshalTypeError
		if errors.As(err, &typeErr) {
			errorResponse(c, http.StatusUnprocessableEntity, "Wrong type for "+typeErr.Field)
			return
		}
		errorResponse(c, http.StatusBadRequest, "Send a JSON object of show settings")
		return
	}
	if !s.catalogReady(c) {
		return
	}

	title, err := s.opts.Catalog.PatchShow(c.Request.Context(), id, req.toPatch())
	if err != nil {
		catalogErrorResponse(c, err)
		return
	}
	c.JSON(http.StatusOK, title)
}

func (s *Server) getEpisodeGroups(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok || !s.catalogReady(c) {
		return
	}
	groups, err := s.opts.Catalog.EpisodeGroups(c.Request.Context(), id)
	if err != nil {
		catalogErrorResponse(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": groups})
}

// catalogErrorResponse maps a catalog error to a response: the catalog's
// own 404s and 400s, then the metadata errors (TMDB 502/503, unknown title
// 404, invalid settings 422, unknown show 404).
func catalogErrorResponse(c *gin.Context, err error) {
	switch {
	case errors.Is(err, context.Canceled):
		// The browser moved on (a new season, a closed tab): nobody reads this answer
		errorCodeResponse(c, StatusClientClosedRequest, CodeClientClosed, "The request was canceled")
	case errors.Is(err, catalog.ErrNotInLibrary):
		errorCodeResponse(c, http.StatusNotFound, CodeNotInLibrary, "Add the show to the library to see its episodes")
	case errors.Is(err, catalog.ErrSeasonNotFound):
		errorResponse(c, http.StatusNotFound, "Season not found")
	case errors.Is(err, catalog.ErrInvalidQuery):
		errorResponse(c, http.StatusBadRequest, err.Error())
	case errors.Is(err, catalog.ErrSettingsUnavailable):
		errorResponse(c, http.StatusServiceUnavailable, "Show settings not configured")
	case errors.Is(err, tmdb.ErrNotFound):
		errorResponse(c, http.StatusNotFound, "TMDB has no such title")
	default:
		metadataErrorResponse(c, err, http.StatusNotFound)
	}
}
