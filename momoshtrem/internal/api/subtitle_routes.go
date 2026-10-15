package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/opensubtitles"
	"github.com/shapedtime/momoshtrem/internal/subtitle"
)

// Item-scoped subtitle routes (PLAN §6 "Subtitles", B7), under /movies/:id
// and /episodes/:id, where :id is the library movie or episode.

// ItemSubtitle is one stored subtitle (contract: Subtitle).
type ItemSubtitle struct {
	ID           int64   `json:"id"`
	LanguageCode string  `json:"language_code"`
	LanguageName string  `json:"language_name"`
	Format       string  `json:"format"`
	Source       string  `json:"source"`
	ReleaseName  *string `json:"release_name"`
	Match        string  `json:"match"`
	CreatedAt    string  `json:"created_at"`
}

// ItemSubtitleList is GET …/subtitles.
type ItemSubtitleList struct {
	Items []ItemSubtitle `json:"items"`
}

// SubtitleCandidate is one ranked OpenSubtitles result (contract:
// SubtitleCandidate).
type SubtitleCandidate struct {
	FileID            int    `json:"file_id"`
	LanguageCode      string `json:"language_code"`
	LanguageName      string `json:"language_name"`
	ReleaseName       string `json:"release_name"`
	FileName          string `json:"file_name"`
	Downloads         int    `json:"downloads"`
	HearingImpaired   bool   `json:"hearing_impaired"`
	MachineTranslated bool   `json:"machine_translated"`
	Match             string `json:"match"`
	MatchScore        int    `json:"match_score"`
	MatchReason       string `json:"match_reason"`
}

// ItemSubtitleSearch is GET …/subtitles/search (contract: SubtitleSearchResult).
type ItemSubtitleSearch struct {
	Items []SubtitleCandidate `json:"items"`
	// The release the candidates were ranked against; null when nothing is
	// assigned.
	ReleaseName *string `json:"release_name"`
}

// ItemSubtitleDownload is the POST …/subtitles body.
type ItemSubtitleDownload struct {
	FileID       int    `json:"file_id" binding:"required"`
	LanguageCode string `json:"language_code" binding:"required"`
	ReleaseName  string `json:"release_name"`
}

// subtitleItemResolver loads what the subtitle routes need about a library
// item.
type subtitleItemResolver interface {
	// ResolveSubtitleItem returns nil when the item isn't in the library.
	ResolveSubtitleItem(ctx context.Context, item subtitle.Item) (*subtitle.ItemContext, error)
}

// libraryItemResolver resolves items from the library repositories.
type libraryItemResolver struct {
	movies      *library.MovieRepository
	shows       *library.ShowRepository
	assignments *library.AssignmentRepository
}

func (r *libraryItemResolver) ResolveSubtitleItem(_ context.Context, item subtitle.Item) (*subtitle.ItemContext, error) {
	out := &subtitle.ItemContext{Item: item}
	var itemType library.ItemType

	switch item.Type {
	case subtitle.ItemTypeMovie:
		movie, err := r.movies.GetByID(item.ID)
		if err != nil || movie == nil {
			return nil, err
		}
		out.Search = opensubtitles.SearchParams{Type: "movie", TMDBID: movie.TMDBID}
		itemType = library.ItemTypeMovie
	case subtitle.ItemTypeEpisode:
		ep, err := r.shows.GetEpisodeContext(item.ID)
		if err != nil || ep == nil {
			return nil, err
		}
		out.Search = opensubtitles.SearchParams{
			Type:          "episode",
			ParentTMDBID:  ep.ShowTMDBID,
			SeasonNumber:  ep.SeasonNumber,
			EpisodeNumber: ep.EpisodeNumber,
		}
		itemType = library.ItemTypeEpisode
	default:
		return nil, nil
	}

	assignment, err := r.assignments.GetActiveForItem(itemType, item.ID)
	if err != nil {
		return nil, err
	}
	// An HDRezka stream has no torrent and no release name to rank against:
	// its matches are unknown.
	if assignment != nil && assignment.IsTorrent() {
		out.InfoHash = assignment.InfoHash
		out.Release = subtitle.AssignedRelease(assignment.MagnetURI, assignment.FilePath)
	}
	return out, nil
}

func (s *Server) registerSubtitleRoutes(api *gin.RouterGroup) {
	for _, r := range []struct {
		path     string
		itemType subtitle.ItemType
	}{
		{"/movies/:id", subtitle.ItemTypeMovie},
		{"/episodes/:id", subtitle.ItemTypeEpisode},
	} {
		api.GET(r.path+"/subtitles", s.listItemSubtitles(r.itemType))
		api.GET(r.path+"/subtitles/search", s.searchItemSubtitles(r.itemType))
		api.POST(r.path+"/subtitles", s.downloadItemSubtitle(r.itemType))
		api.DELETE(r.path+"/subtitles/:subtitle_id", s.deleteItemSubtitle(r.itemType))
	}
}

// subtitleItem resolves the route's item, or writes the error and returns
// false: 503 without a subtitle service, 404 for an unknown item.
func (s *Server) subtitleItem(c *gin.Context, itemType subtitle.ItemType) (*subtitle.ItemContext, bool) {
	if s.subtitleService == nil || s.subtitleItems == nil {
		errorResponse(c, http.StatusServiceUnavailable, "Subtitles are not available")
		return nil, false
	}
	id, ok := parseID(c, "id")
	if !ok {
		return nil, false
	}
	item, err := s.subtitleItems.ResolveSubtitleItem(c.Request.Context(), subtitle.Item{Type: itemType, ID: id})
	if err != nil {
		errorResponse(c, http.StatusInternalServerError, err.Error())
		return nil, false
	}
	if item == nil {
		if itemType == subtitle.ItemTypeMovie {
			errorResponse(c, http.StatusNotFound, "Movie not found")
		} else {
			errorResponse(c, http.StatusNotFound, "Episode not found")
		}
		return nil, false
	}
	return item, true
}

func (s *Server) listItemSubtitles(itemType subtitle.ItemType) gin.HandlerFunc {
	return func(c *gin.Context) {
		item, ok := s.subtitleItem(c, itemType)
		if !ok {
			return
		}
		stored, err := s.subtitleService.List(c.Request.Context(), *item)
		if err != nil {
			errorResponse(c, http.StatusInternalServerError, err.Error())
			return
		}
		resp := ItemSubtitleList{Items: make([]ItemSubtitle, len(stored))}
		for i, st := range stored {
			resp.Items[i] = toItemSubtitle(st)
		}
		c.JSON(http.StatusOK, resp)
	}
}

func (s *Server) searchItemSubtitles(itemType subtitle.ItemType) gin.HandlerFunc {
	return func(c *gin.Context) {
		item, ok := s.subtitleItem(c, itemType)
		if !ok {
			return
		}
		if !s.subtitleService.IsConfigured() {
			errorResponse(c, http.StatusServiceUnavailable, "OpenSubtitles is not configured")
			return
		}
		languages, err := subtitle.ParseLanguages(c.Query("languages"))
		if err != nil {
			errorResponse(c, http.StatusBadRequest, err.Error())
			return
		}

		ranked, err := s.subtitleService.SearchRanked(c.Request.Context(), *item, languages)
		if err != nil {
			subtitleErrorResponse(c, err)
			return
		}

		resp := ItemSubtitleSearch{Items: make([]SubtitleCandidate, len(ranked)), ReleaseName: releaseOrNull(item.Release)}
		for i, cand := range ranked {
			resp.Items[i] = SubtitleCandidate{
				FileID:            cand.FileID,
				LanguageCode:      cand.LanguageCode,
				LanguageName:      cand.LanguageName,
				ReleaseName:       cand.ReleaseName,
				FileName:          cand.FileName,
				Downloads:         cand.Downloads,
				HearingImpaired:   cand.HearingImpaired,
				MachineTranslated: cand.MachineTranslated,
				Match:             string(cand.Match),
				MatchScore:        cand.Score,
				MatchReason:       cand.Reason,
			}
		}
		c.JSON(http.StatusOK, resp)
	}
}

func (s *Server) downloadItemSubtitle(itemType subtitle.ItemType) gin.HandlerFunc {
	return func(c *gin.Context) {
		item, ok := s.subtitleItem(c, itemType)
		if !ok {
			return
		}
		if !s.subtitleService.IsConfigured() {
			errorResponse(c, http.StatusServiceUnavailable, "OpenSubtitles is not configured")
			return
		}
		var req ItemSubtitleDownload
		if err := c.ShouldBindJSON(&req); err != nil {
			errorResponse(c, http.StatusBadRequest, "Send {file_id, language_code, release_name?}")
			return
		}

		stored, err := s.subtitleService.Download(c.Request.Context(), *item, req.FileID, req.LanguageCode, req.ReleaseName)
		if err != nil {
			subtitleErrorResponse(c, err)
			return
		}

		if s.treeUpdater != nil {
			s.treeUpdater.InvalidateTree()
		}
		c.JSON(http.StatusCreated, toItemSubtitle(*stored))
	}
}

func (s *Server) deleteItemSubtitle(itemType subtitle.ItemType) gin.HandlerFunc {
	return func(c *gin.Context) {
		item, ok := s.subtitleItem(c, itemType)
		if !ok {
			return
		}
		subtitleID, err := strconv.ParseInt(c.Param("subtitle_id"), 10, 64)
		if err != nil || subtitleID <= 0 {
			errorResponse(c, http.StatusBadRequest, "Invalid subtitle ID")
			return
		}

		if err := s.subtitleService.DeleteFromItem(c.Request.Context(), *item, subtitleID); err != nil {
			subtitleErrorResponse(c, err)
			return
		}

		if s.treeUpdater != nil {
			s.treeUpdater.InvalidateTree()
		}
		c.Status(http.StatusNoContent)
	}
}

// subtitleErrorResponse maps subtitle service errors to API errors.
func subtitleErrorResponse(c *gin.Context, err error) {
	switch {
	case errors.Is(err, subtitle.ErrNotFound):
		errorResponse(c, http.StatusNotFound, "Subtitle not found")
	case errors.Is(err, subtitle.ErrTorrentSubtitle):
		errorResponse(c, http.StatusConflict, "This subtitle came with the torrent and stays with it")
	case errors.Is(err, subtitle.ErrInvalidLanguage):
		errorResponse(c, http.StatusBadRequest, err.Error())
	case errors.Is(err, subtitle.ErrNotConfigured):
		errorResponse(c, http.StatusServiceUnavailable, "OpenSubtitles is not configured")
	case errors.Is(err, subtitle.ErrFetch):
		// The detail (OpenSubtitles' body, a download link) is for the log.
		slog.Warn("OpenSubtitles request failed", "path", c.Request.URL.Path, "error", err)
		errorResponse(c, http.StatusBadGateway, "OpenSubtitles request failed; try again later")
	default:
		errorResponse(c, http.StatusInternalServerError, err.Error())
	}
}

func toItemSubtitle(st subtitle.Stored) ItemSubtitle {
	return ItemSubtitle{
		ID:           st.ID,
		LanguageCode: st.LanguageCode,
		LanguageName: st.LanguageName,
		Format:       st.Format,
		Source:       string(st.Source),
		ReleaseName:  releaseOrNull(st.ReleaseName),
		Match:        string(st.Match),
		CreatedAt:    st.CreatedAt.UTC().Format(time.RFC3339),
	}
}

// releaseOrNull is s, or nil (JSON null) when it's empty.
func releaseOrNull(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
