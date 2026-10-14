package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/shapedtime/momoshtrem/internal/opensubtitles"
	"github.com/shapedtime/momoshtrem/internal/subtitle"
)

// Subtitle request/response types

type SubtitleSearchResult struct {
	FileID        int     `json:"file_id"`
	LanguageCode  string  `json:"language_code"`
	LanguageName  string  `json:"language_name"`
	ReleaseName   string  `json:"release_name"`
	DownloadCount int     `json:"download_count"`
	FileName      string  `json:"file_name"`
	Ratings       float64 `json:"ratings"`
}

type SubtitleSearchResponse struct {
	Results []SubtitleSearchResult `json:"results"`
}

type DownloadSubtitleRequest struct {
	ItemType     string `json:"item_type" binding:"required"` // "movie" or "episode"
	ItemID       int64  `json:"item_id" binding:"required"`
	FileID       int    `json:"file_id" binding:"required"`
	LanguageCode string `json:"language_code" binding:"required"`
	LanguageName string `json:"language_name" binding:"required"`
}

type SubtitleResponse struct {
	ID           int64  `json:"id"`
	LanguageCode string `json:"language_code"`
	LanguageName string `json:"language_name"`
	Format       string `json:"format"`
	FileSize     int64  `json:"file_size"`
	CreatedAt    string `json:"created_at"`
}

// Legacy subtitle handlers (/subtitles/*). The item-scoped routes in
// subtitle_routes.go replace them; they go when the web drops their callers.

func (s *Server) searchSubtitles(c *gin.Context) {
	if s.subtitleService == nil || !s.subtitleService.IsConfigured() {
		errorResponse(c, http.StatusServiceUnavailable, "Subtitle service not configured")
		return
	}

	// Parse query parameters
	tmdbIDStr := c.Query("tmdb_id")
	if tmdbIDStr == "" {
		errorResponse(c, http.StatusBadRequest, "tmdb_id is required")
		return
	}
	tmdbID, err := strconv.Atoi(tmdbIDStr)
	if err != nil {
		errorResponse(c, http.StatusBadRequest, "Invalid tmdb_id")
		return
	}

	mediaType := c.Query("type")
	if mediaType != "movie" && mediaType != "episode" {
		errorResponse(c, http.StatusBadRequest, "type must be 'movie' or 'episode'")
		return
	}

	languagesStr := c.Query("languages")
	if languagesStr == "" {
		languagesStr = "en" // Default to English
	}
	languages := strings.Split(languagesStr, ",")

	// Parse season/episode for TV
	var seasonNum, episodeNum int
	if mediaType == "episode" {
		if seasonStr := c.Query("season"); seasonStr != "" {
			seasonNum, _ = strconv.Atoi(seasonStr)
		}
		if epStr := c.Query("episode"); epStr != "" {
			episodeNum, _ = strconv.Atoi(epStr)
		}
	}

	// Search via service. For episodes tmdb_id is the show's.
	params := opensubtitles.SearchParams{
		Type:          mediaType,
		Languages:     languages,
		SeasonNumber:  seasonNum,
		EpisodeNumber: episodeNum,
	}
	if mediaType == "episode" {
		params.ParentTMDBID = tmdbID
	} else {
		params.TMDBID = tmdbID
	}

	searchResp, err := s.subtitleService.Search(c.Request.Context(), params)
	if err != nil {
		errorResponse(c, http.StatusInternalServerError, "Subtitle search failed: "+err.Error())
		return
	}

	// Convert to response format
	results := make([]SubtitleSearchResult, 0, len(searchResp.Data))
	for _, sub := range searchResp.Data {
		if len(sub.Attributes.Files) == 0 {
			continue
		}

		// Use the first file in each subtitle entry
		file := sub.Attributes.Files[0]

		results = append(results, SubtitleSearchResult{
			FileID:        file.FileID,
			LanguageCode:  sub.Attributes.Language,
			LanguageName:  subtitle.LanguageName(sub.Attributes.Language),
			ReleaseName:   sub.Attributes.Release,
			DownloadCount: sub.Attributes.DownloadCount,
			FileName:      file.FileName,
			Ratings:       sub.Attributes.Ratings,
		})
	}

	c.JSON(http.StatusOK, SubtitleSearchResponse{Results: results})
}

func (s *Server) downloadSubtitle(c *gin.Context) {
	if s.subtitleService == nil || !s.subtitleService.IsConfigured() {
		errorResponse(c, http.StatusServiceUnavailable, "Subtitle service not configured")
		return
	}

	var req DownloadSubtitleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	// Parse and validate item type
	itemType, err := subtitle.ParseItemType(req.ItemType)
	if err != nil {
		errorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	// Download and store via service
	sub, err := s.subtitleService.DownloadAndStore(
		c.Request.Context(),
		itemType,
		req.ItemID,
		req.FileID,
		req.LanguageCode,
		req.LanguageName,
	)
	if err != nil {
		subtitleErrorResponse(c, err)
		return
	}

	// Update VFS tree to include the new subtitle
	if s.treeUpdater != nil {
		s.treeUpdater.InvalidateTree()
	}

	c.JSON(http.StatusCreated, gin.H{
		"success":  true,
		"subtitle": toSubtitleResponse(sub),
	})
}

func (s *Server) deleteSubtitle(c *gin.Context) {
	if s.subtitleService == nil {
		errorResponse(c, http.StatusServiceUnavailable, "Subtitle service not configured")
		return
	}

	id, ok := parseID(c, "id")
	if !ok {
		return
	}

	if err := s.subtitleService.Delete(c.Request.Context(), id); err != nil {
		subtitleErrorResponse(c, err)
		return
	}

	// Update VFS tree
	if s.treeUpdater != nil {
		s.treeUpdater.InvalidateTree()
	}

	c.Status(http.StatusNoContent)
}

// Helper functions

func toSubtitleResponse(s *subtitle.Subtitle) SubtitleResponse {
	return SubtitleResponse{
		ID:           s.ID,
		LanguageCode: s.LanguageCode,
		LanguageName: s.LanguageName,
		Format:       s.Format,
		FileSize:     s.FileSize,
		CreatedAt:    s.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}
}
