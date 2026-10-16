package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/shapedtime/momoshtrem/internal/catalog"
)

// registerLibraryRoutes adds GET /library.
func (s *Server) registerLibraryRoutes(api *gin.RouterGroup) {
	api.GET("/library", s.getLibrary)
}

// getLibrary lists the library a page at a time:
// ?type=movie|tv&status=ready|incomplete|needs_review|watching
// &sort=added|title|last_watched|size&q=&page=&page_size= (50, max 200),
// or looks titles up with &ids=movie:603,tv:1399 (at most 200). Malformed
// ids, like unknown filters, answer 400.
func (s *Server) getLibrary(c *gin.Context) {
	page, ok := queryInt(c, "page")
	if !ok {
		return
	}
	pageSize, ok := queryInt(c, "page_size")
	if !ok || !s.catalogReady(c) {
		return
	}

	result, err := s.opts.Catalog.Library(c.Request.Context(), catalog.LibraryParams{
		MediaType: c.Query("type"),
		Status:    c.Query("status"),
		Sort:      c.Query("sort"),
		Query:     c.Query("q"),
		Page:      page,
		PageSize:  pageSize,
		IDs:       c.Query("ids"),
	})
	if err != nil {
		catalogErrorResponse(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

// queryInt reads an optional positive integer query parameter; 0 when it's
// absent. Anything else answers 400.
func queryInt(c *gin.Context, name string) (int, bool) {
	raw := c.Query(name)
	if raw == "" {
		return 0, true
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		errorResponse(c, http.StatusBadRequest, name+" must be a positive number")
		return 0, false
	}
	return n, true
}
