package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/shapedtime/momoshtrem/internal/service"
)

// HDRezka finds titles on HDRezka and assigns their streams (PLAN §6
// "HDRezka (#63)"). *service.HDRezkaService implements it.
type HDRezka interface {
	Search(ctx context.Context, mediaType string, tmdbID int, q string) (*service.HDRezkaSearch, error)
	Title(ctx context.Context, req service.HDRezkaTitleRequest) (*service.HDRezkaTitle, error)
	Streams(ctx context.Context, req service.HDRezkaStreamsRequest) (*service.HDRezkaStreams, error)
	AssignMovie(ctx context.Context, movieID int64, req service.HDRezkaAssign) (*service.HDRezkaAssigned, error)
	AssignShow(ctx context.Context, showID int64, req service.HDRezkaShowAssign) (*service.HDRezkaShowResult, error)
}

var _ HDRezka = (*service.HDRezkaService)(nil)

// registerHDRezkaRoutes adds the HDRezka routes. All of them answer 503
// unavailable while HDRezka is off.
func (s *Server) registerHDRezkaRoutes(api *gin.RouterGroup) {
	api.GET("/hdrezka/search", s.hdrezkaSearch)
	api.GET("/hdrezka/titles", s.hdrezkaTitle)
	api.GET("/hdrezka/streams", s.hdrezkaStreams)
	api.POST("/movies/:id/assign-hdrezka", s.assignMovieHDRezka)
	api.POST("/shows/:id/assign-hdrezka", s.assignShowHDRezka)
}

// hdrezkaReady answers 503 while HDRezka is off.
func (s *Server) hdrezkaReady(c *gin.Context) bool {
	if s.opts.HDRezka == nil {
		errorResponse(c, http.StatusServiceUnavailable, "HDRezka is off (set HDREZKA_ENABLED=true).")
		return false
	}
	return true
}

// GET /hdrezka/search?media_type=movie|tv&tmdb_id=[&q=]
func (s *Server) hdrezkaSearch(c *gin.Context) {
	if !s.hdrezkaReady(c) {
		return
	}
	tmdbID, ok := queryInt(c, "tmdb_id")
	if !ok {
		return
	}
	res, err := s.opts.HDRezka.Search(c.Request.Context(), c.Query("media_type"), tmdbID, c.Query("q"))
	if err != nil {
		hdrezkaErrorResponse(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

// GET /hdrezka/titles?url=[&translator_id=][&show_id=|&movie_id=]
func (s *Server) hdrezkaTitle(c *gin.Context) {
	if !s.hdrezkaReady(c) {
		return
	}
	translator, ok1 := queryInt(c, "translator_id")
	show, ok2 := queryInt(c, "show_id")
	movie, ok3 := queryInt(c, "movie_id")
	if !ok1 || !ok2 || !ok3 {
		return
	}
	res, err := s.opts.HDRezka.Title(c.Request.Context(), service.HDRezkaTitleRequest{
		URL: c.Query("url"), TranslatorID: translator, ShowID: int64(show), MovieID: int64(movie),
	})
	if err != nil {
		hdrezkaErrorResponse(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

// GET /hdrezka/streams?url=&translator_id=[&camrip=&ads=&director=][&season=&episode=]
func (s *Server) hdrezkaStreams(c *gin.Context) {
	if !s.hdrezkaReady(c) {
		return
	}
	translator, ok1 := queryInt(c, "translator_id")
	season, ok2 := queryOptInt(c, "season")
	episode, ok3 := queryOptInt(c, "episode")
	camrip, ok4 := queryBool(c, "camrip")
	ads, ok5 := queryBool(c, "ads")
	director, ok6 := queryBool(c, "director")
	if !ok1 || !ok2 || !ok3 || !ok4 || !ok5 || !ok6 {
		return
	}
	res, err := s.opts.HDRezka.Streams(c.Request.Context(), service.HDRezkaStreamsRequest{
		URL: c.Query("url"), TranslatorID: translator,
		Cut:    service.TranslatorCut{Camrip: camrip, Ads: ads, Director: director},
		Season: season, Episode: episode,
	})
	if err != nil {
		hdrezkaErrorResponse(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

// POST /movies/{id}/assign-hdrezka {url, translator_id, camrip?, ads?, director?, quality} → 204
func (s *Server) assignMovieHDRezka(c *gin.Context) {
	if !s.hdrezkaReady(c) {
		return
	}
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	var body service.HDRezkaAssign
	if err := c.ShouldBindJSON(&body); err != nil {
		errorResponse(c, http.StatusBadRequest, "Send {url, translator_id, quality} (and camrip, ads, director to pick a cut).")
		return
	}
	if _, err := s.opts.HDRezka.AssignMovie(c.Request.Context(), id, body); err != nil {
		hdrezkaErrorResponse(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// POST /shows/{id}/assign-hdrezka {url, translator_id, quality, items: [{episode_id, season, episode}]}
// → 200 {assigned, failed}
func (s *Server) assignShowHDRezka(c *gin.Context) {
	if !s.hdrezkaReady(c) {
		return
	}
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	var body service.HDRezkaShowAssign
	if err := c.ShouldBindJSON(&body); err != nil {
		errorResponse(c, http.StatusBadRequest, "Send {url, translator_id, quality, items: [{episode_id, season, episode}]}.")
		return
	}
	res, err := s.opts.HDRezka.AssignShow(c.Request.Context(), id, body)
	if err != nil {
		hdrezkaErrorResponse(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

// queryOptInt reads an optional integer query parameter: nil when it's
// absent or empty. A malformed one answers 400 and ok is false.
func queryOptInt(c *gin.Context, name string) (n *int, ok bool) {
	raw := c.Query(name)
	if raw == "" {
		return nil, true
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		errorResponse(c, http.StatusBadRequest, name+" must be a whole number.")
		return nil, false
	}
	return &v, true
}

// queryBool reads an optional true/false (or 1/0) query parameter.
func queryBool(c *gin.Context, name string) (b *bool, ok bool) {
	raw := c.Query(name)
	if raw == "" {
		return nil, true
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		errorResponse(c, http.StatusBadRequest, name+" must be true or false.")
		return nil, false
	}
	return &v, true
}

// hdrezkaErrorResponse answers an HDRezka error with its code's status.
// No code maps to 401 or 403: the web proxy reads those as a bad backend
// token, so even the CDN's 403 is a 502.
func hdrezkaErrorResponse(c *gin.Context, err error) {
	var he *service.HDRezkaError
	switch {
	case errors.As(err, &he):
		status, code := hdrezkaStatus(he.Code)
		if status >= 500 {
			slog.Warn("HDRezka request failed", "path", c.Request.URL.Path, "code", code, "error", err)
		}
		errorCodeResponse(c, status, code, he.Message)
	case errors.Is(err, context.Canceled):
		errorCodeResponse(c, StatusClientClosedRequest, CodeClientClosed, "The request was canceled")
	default:
		slog.Error("HDRezka request failed", "path", c.Request.URL.Path, "error", err)
		errorResponse(c, http.StatusInternalServerError, "Something went wrong with HDRezka")
	}
}

// hdrezkaStatus is the status of an HDRezka error code; an unknown code is
// a 500.
func hdrezkaStatus(code string) (int, string) {
	switch code {
	case CodeBadRequest:
		return http.StatusBadRequest, code
	case CodeNotFound:
		return http.StatusNotFound, code
	case CodePremiumOnly:
		return http.StatusConflict, code
	case CodeValidationError:
		return http.StatusUnprocessableEntity, code
	case CodeHDRezkaUnavailable, CodeTMDBUnavailable:
		return http.StatusBadGateway, code
	case CodeHDRezkaBlocked, CodeUnavailable:
		return http.StatusServiceUnavailable, code
	case CodeTimeout:
		return http.StatusGatewayTimeout, code
	}
	return http.StatusInternalServerError, CodeInternalError
}
