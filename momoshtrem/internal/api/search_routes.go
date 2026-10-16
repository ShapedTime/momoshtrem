package api

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/search"
	"github.com/shapedtime/momoshtrem/internal/service"
	"github.com/shapedtime/momoshtrem/internal/torrent"
)

// CodeJackettUnavailable is a 502: Jackett's indexer list can't be
// fetched, or Jackett rejects the key.
const CodeJackettUnavailable = "jackett_unavailable"

// TorrentSearch searches Jackett's indexers and resolves results to
// magnets (PLAN §6 "Search"). *search.Service implements it.
type TorrentSearch interface {
	Search(ctx context.Context, req search.SearchRequest) (*search.SearchResponse, error)
	ReleaseResolver
}

var _ TorrentSearch = (*search.Service)(nil)

// registerSearchRoutes adds torrent search. Both routes answer 503
// unavailable without Jackett.
func (s *Server) registerSearchRoutes(api *gin.RouterGroup) {
	api.GET("/search/torrents", s.searchTorrents)
	api.POST("/search/torrents/resolve", s.resolveRelease)
}

// searchReady answers 503 when Jackett isn't configured.
func (s *Server) searchReady(c *gin.Context) bool {
	if s.opts.Search == nil {
		errorResponse(c, http.StatusServiceUnavailable, "Torrent search needs Jackett (set JACKETT_URL and JACKETT_API_KEY).")
		return false
	}
	return true
}

// GET /search/torrents?media_type=movie|tv&tmdb_id=[&season=&episode=][&scope=][&q=][&<filter>=…][&sort=][&page=&page_size=][&refresh=1]
func (s *Server) searchTorrents(c *gin.Context) {
	if !s.searchReady(c) {
		return
	}
	req, ok := parseSearchRequest(c)
	if !ok {
		return
	}
	res, err := s.opts.Search.Search(c.Request.Context(), req)
	if err != nil {
		searchErrorResponse(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

// parseSearchRequest reads GET /search/torrents' query. A malformed number
// answers 400 and ok is false; the service checks the rest.
func parseSearchRequest(c *gin.Context) (req search.SearchRequest, ok bool) {
	raw := c.Query("tmdb_id")
	if raw == "" {
		errorResponse(c, http.StatusBadRequest, "tmdb_id is required.")
		return req, false
	}
	tmdbID, err := strconv.Atoi(raw)
	if err != nil || tmdbID < 1 {
		errorResponse(c, http.StatusBadRequest, "tmdb_id must be a positive number.")
		return req, false
	}
	req = search.SearchRequest{
		MediaType: c.Query("media_type"),
		TMDBID:    tmdbID,
		Scope:     search.Scope(c.Query("scope")),
		Q:         c.Query("q"),
		Sort:      search.Sort(c.Query("sort")),
		Filters: search.Filters{
			Resolution: queryValues(c, "resolution"),
			Source:     queryValues(c, "source"),
			Codec:      queryValues(c, "codec"),
			HDR:        queryValues(c, "hdr"),
			Audio:      queryValues(c, "audio"),
			Pack:       queryValues(c, "pack"),
			Indexer:    queryValues(c, "indexer"),
			Group:      queryValues(c, "group"),
		},
	}

	var page, pageSize *int
	if req.Season, ok = queryOptInt(c, "season"); !ok {
		return req, false
	}
	if req.Episode, ok = queryOptInt(c, "episode"); !ok {
		return req, false
	}
	if req.Filters.MinSeeders, ok = queryOptInt(c, "min_seeders"); !ok {
		return req, false
	}
	if page, ok = queryOptInt(c, "page"); !ok {
		return req, false
	}
	if pageSize, ok = queryOptInt(c, "page_size"); !ok {
		return req, false
	}
	if req.Filters.MinSizeGB, ok = queryOptFloat(c, "min_size_gb"); !ok {
		return req, false
	}
	if req.Filters.MaxSizeGB, ok = queryOptFloat(c, "max_size_gb"); !ok {
		return req, false
	}
	for _, p := range []struct {
		name string
		n    *int
	}{{"page", page}, {"page_size", pageSize}} {
		if p.n != nil && *p.n < 1 {
			errorResponse(c, http.StatusBadRequest, p.name+" must be 1 or more.")
			return req, false
		}
	}
	if page != nil {
		req.Page = *page
	}
	if pageSize != nil {
		req.PageSize = *pageSize
	}
	switch c.Query("refresh") {
	case "", "0", "false":
	case "1", "true":
		req.Refresh = true
	default:
		errorResponse(c, http.StatusBadRequest, "refresh must be 1 or left out.")
		return req, false
	}
	return req, true
}

// queryValues are a repeatable parameter's values, without empty ones.
func queryValues(c *gin.Context, name string) []string {
	var out []string
	for _, v := range c.QueryArray(name) {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// queryOptFloat reads an optional number: nil when it's absent or empty. A
// malformed one answers 400 and ok is false.
func queryOptFloat(c *gin.Context, name string) (f *float64, ok bool) {
	raw := c.Query(name)
	if raw == "" {
		return nil, true
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		errorResponse(c, http.StatusBadRequest, name+" must be a number.")
		return nil, false
	}
	return &v, true
}

// searchErrorResponse maps the search's errors. Nothing maps to 401 or
// 403: the web proxy reads those as a bad backend token.
func searchErrorResponse(c *gin.Context, err error) {
	switch {
	case errors.Is(err, search.ErrBadRequest):
		errorResponse(c, http.StatusBadRequest, sentence(strings.TrimPrefix(err.Error(), search.ErrBadRequest.Error()+": ")))
	case errors.Is(err, service.ErrTitleNotFound):
		errorResponse(c, http.StatusNotFound, "TMDB has no such title.")
	case errors.Is(err, library.ErrTMDBFetch):
		errorCodeResponse(c, http.StatusBadGateway, CodeTMDBUnavailable, "TMDB is unavailable and the title isn't in your library.")
	case errors.Is(err, search.ErrJackettUnavailable):
		errorCodeResponse(c, http.StatusBadGateway, CodeJackettUnavailable, sentence(err.Error()))
	case errors.Is(err, context.Canceled):
		errorCodeResponse(c, StatusClientClosedRequest, CodeClientClosed, "The request was canceled")
	default:
		slog.Error("Torrent search failed", "path", c.Request.URL.Path, "error", err)
		errorResponse(c, http.StatusInternalServerError, "The search failed.")
	}
}

// sentence capitalises s and ends it with a full stop.
func sentence(s string) string {
	if s == "" {
		return s
	}
	s = strings.ToUpper(s[:1]) + s[1:]
	if !strings.HasSuffix(s, ".") {
		s += "."
	}
	return s
}

// resolveResponse is POST /search/torrents/resolve's answer.
type resolveResponse struct {
	MagnetURI string `json:"magnet_uri"`
	InfoHash  string `json:"info_hash"`
}

// POST /search/torrents/resolve {release_id} → 200 {magnet_uri, info_hash}
func (s *Server) resolveRelease(c *gin.Context) {
	if !s.searchReady(c) {
		return
	}
	var body struct {
		ReleaseID string `json:"release_id"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || strings.TrimSpace(body.ReleaseID) == "" {
		errorResponse(c, http.StatusBadRequest, "Send {release_id}.")
		return
	}
	magnet, _, err := s.opts.Search.Resolve(c.Request.Context(), strings.TrimSpace(body.ReleaseID))
	if err != nil {
		resolveErrorResponse(c, err)
		return
	}
	c.JSON(http.StatusOK, resolveResponse{
		MagnetURI: magnet,
		InfoHash:  torrent.NormalizeInfoHash(torrent.ExtractInfoHash(magnet)),
	})
}
