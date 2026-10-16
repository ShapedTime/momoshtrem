package api

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/metadata"
	"github.com/shapedtime/momoshtrem/internal/service"
	"github.com/shapedtime/momoshtrem/internal/tmdb"
)

// Movie request/response types
type CreateMovieRequest struct {
	TMDBID int `json:"tmdb_id" binding:"required"`
}

type MovieResponse struct {
	ID            int64               `json:"id"`
	TMDBID        int                 `json:"tmdb_id"`
	Title         string              `json:"title"`
	Year          int                 `json:"year"`
	HasAssignment bool                `json:"has_assignment"`
	Assignment    *AssignmentResponse `json:"assignment,omitempty"`
}

type AssignmentResponse struct {
	ID         int64  `json:"id"`
	InfoHash   string `json:"info_hash"`
	FilePath   string `json:"file_path"`
	FileSize   int64  `json:"file_size"`
	Resolution string `json:"resolution,omitempty"`
	Source     string `json:"source,omitempty"`
}

// Show request/response types
type CreateShowRequest struct {
	TMDBID  int   `json:"tmdb_id" binding:"required"`
	Seasons []int `json:"seasons,omitempty"` // Optional: specific seasons to add
}

type ShowResponse struct {
	ID      int64            `json:"id"`
	TMDBID  int              `json:"tmdb_id"`
	Title   string           `json:"title"`
	Year    int              `json:"year"`
	Seasons []SeasonResponse `json:"seasons,omitempty"`
}

type SeasonResponse struct {
	ID           int64             `json:"id"`
	SeasonNumber int               `json:"season_number"`
	Episodes     []EpisodeResponse `json:"episodes,omitempty"`
}

type EpisodeResponse struct {
	ID            int64               `json:"id"`
	EpisodeNumber int                 `json:"episode_number"`
	Name          string              `json:"name"`
	HasAssignment bool                `json:"has_assignment"`
	Assignment    *AssignmentResponse `json:"assignment,omitempty"`
}

// MovieAssignmentResponse is the legacy movie assign's answer
// (?wait=true), with the job that made it.
type MovieAssignmentResponse struct {
	Success    bool                `json:"success"`
	Assignment *AssignmentResponse `json:"assignment,omitempty"`
	Error      string              `json:"error,omitempty"`
	JobID      int64               `json:"job_id"`
}

// Refresh response
type RefreshShowResponse struct {
	Show          ShowResponse `json:"show"`
	SeasonsAdded  int          `json:"seasons_added"`
	EpisodesAdded int          `json:"episodes_added"`
	SeasonErrors  []string     `json:"season_errors,omitempty"`
}

// ShowAssignmentResponse is the legacy show assign's answer (?wait=true),
// with the job that made it.
type ShowAssignmentResponse struct {
	Success   bool                          `json:"success"`
	Summary   service.AssignmentSummary     `json:"summary"`
	Matched   []service.MatchedAssignment   `json:"matched"`
	Unmatched []service.UnmatchedAssignment `json:"unmatched,omitempty"`
	Error     string                        `json:"error,omitempty"`
	JobID     int64                         `json:"job_id"`
}

// parseID parses and validates an ID parameter
func parseID(c *gin.Context, param string) (int64, bool) {
	id, err := strconv.ParseInt(c.Param(param), 10, 64)
	if err != nil {
		errorResponse(c, http.StatusBadRequest, "Invalid ID format")
		return 0, false
	}
	if id <= 0 {
		errorResponse(c, http.StatusBadRequest, "ID must be positive")
		return 0, false
	}
	return id, true
}

// Movie handlers

func (s *Server) listMovies(c *gin.Context) {
	// Movies and their active assignments in one query.
	movies, err := s.movieRepo.ListWithActiveAssignment(c.Request.Context())
	if err != nil {
		errorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}

	response := make([]MovieResponse, len(movies))
	for i, movie := range movies {
		response[i] = toMovieResponse(movie, movie.Assignment)
	}

	c.JSON(http.StatusOK, response)
}

func (s *Server) createMovie(c *gin.Context) {
	var req CreateMovieRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errorResponse(c, http.StatusBadRequest, err.Error())
		return
	}
	if s.opts.Metadata == nil {
		errorResponse(c, http.StatusServiceUnavailable, "Metadata service not configured")
		return
	}

	// An unknown TMDB ID is the caller's mistake; any other TMDB failure is
	// TMDB's.
	result, err := s.opts.Metadata.CreateMovie(c.Request.Context(), req.TMDBID)
	if err != nil {
		metadataErrorResponse(c, err, http.StatusBadRequest)
		return
	}

	status := http.StatusCreated
	if result.Existing {
		status = http.StatusOK
	}
	c.JSON(status, toMovieResponse(result.Movie, nil))
}

// refreshMovie re-fetches a movie's metadata from TMDB. Title and year (the
// WebDAV folder name) are kept.
func (s *Server) refreshMovie(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if s.opts.Metadata == nil {
		errorResponse(c, http.StatusServiceUnavailable, "Metadata service not configured")
		return
	}

	movie, err := s.opts.Metadata.RefreshMovie(c.Request.Context(), id)
	if err != nil {
		metadataErrorResponse(c, err, http.StatusNotFound)
		return
	}

	assignment, err := s.assignmentRepo.GetActiveForItem(library.ItemTypeMovie, movie.ID)
	if err != nil {
		errorResponse(c, http.StatusInternalServerError, "Failed to get assignment for movie")
		return
	}
	c.JSON(http.StatusOK, toMovieResponse(movie, assignment))
}

// metadataErrorResponse maps a metadata.Syncer error to a response.
// tmdbNotFound is the status for a TMDB ID TMDB doesn't know: 400 when the
// client sent it, 404 when it's already in the library.
func metadataErrorResponse(c *gin.Context, err error, tmdbNotFound int) {
	switch {
	case errors.Is(err, library.ErrMovieNotFound):
		errorResponse(c, http.StatusNotFound, "Movie not found")
	case errors.Is(err, library.ErrShowNotFound):
		errorResponse(c, http.StatusNotFound, "Show not found")
	case errors.Is(err, tmdb.ErrNoAPIKey):
		errorResponse(c, http.StatusServiceUnavailable, "TMDB is not configured (set TMDB_API_KEY)")
	case errors.Is(err, tmdb.ErrNotFound):
		errorResponse(c, tmdbNotFound, err.Error())
	case errors.Is(err, library.ErrTMDBFetch):
		errorCodeResponse(c, http.StatusBadGateway, CodeTMDBUnavailable, err.Error())
	case errors.Is(err, library.ErrInvalidShowSettings):
		errorResponse(c, http.StatusUnprocessableEntity, err.Error())
	default:
		errorResponse(c, http.StatusInternalServerError, err.Error())
	}
}

func (s *Server) getMovie(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}

	movie, err := s.movieRepo.GetByID(id)
	if err != nil {
		errorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}
	if movie == nil {
		errorResponse(c, http.StatusNotFound, "Movie not found")
		return
	}

	assignment, err := s.assignmentRepo.GetActiveForItem(library.ItemTypeMovie, movie.ID)
	if err != nil {
		errorResponse(c, http.StatusInternalServerError, "Failed to get assignment for movie")
		return
	}
	c.JSON(http.StatusOK, toMovieResponse(movie, assignment))
}

// deleteMovie removes a movie, its subtitles and the torrents nothing else
// uses (PLAN §6 "Titles and library").
// DELETE /api/movies/:id?delete_data=true|false → 204
func (s *Server) deleteMovie(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	deleteData, ok := parseDeleteData(c)
	if !ok || !s.removalsReady(c) {
		return
	}
	if err := s.opts.Removals.DeleteMovie(c.Request.Context(), id, deleteData); err != nil {
		removalErrorResponse(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// unassignMovieTorrent drops the movie's file and the subtitles that came
// with it, and releases its torrent when nothing else uses it.
// DELETE /api/movies/:id/assign → 204
func (s *Server) unassignMovieTorrent(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok || !s.removalsReady(c) {
		return
	}
	if err := s.opts.Removals.UnassignMovie(c.Request.Context(), id); err != nil {
		removalErrorResponse(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// Show handlers

func (s *Server) listShows(c *gin.Context) {
	shows, err := s.showRepo.List()
	if err != nil {
		errorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}

	response := make([]ShowResponse, len(shows))
	for i, show := range shows {
		response[i] = ShowResponse{
			ID:     show.ID,
			TMDBID: show.TMDBID,
			Title:  show.Title,
			Year:   show.Year,
		}
	}

	c.JSON(http.StatusOK, response)
}

func (s *Server) createShow(c *gin.Context) {
	var req CreateShowRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	if s.opts.Metadata == nil {
		errorResponse(c, http.StatusServiceUnavailable, "Metadata service not configured")
		return
	}

	result, err := s.opts.Metadata.CreateShow(c.Request.Context(), metadata.CreateShowInput{
		TMDBID:  req.TMDBID,
		Seasons: req.Seasons,
	})
	if err != nil {
		metadataErrorResponse(c, err, http.StatusBadRequest)
		return
	}

	// Log any season errors (non-fatal)
	for _, se := range result.SeasonErrors {
		slog.Warn("Season creation error", "error", se.Error())
	}

	status := http.StatusCreated
	if result.Existing {
		status = http.StatusOK
	}
	c.JSON(status, toShowResponse(result.Show))
}

func (s *Server) getShow(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}

	// Seasons, episodes and active assignments in two queries.
	show, err := s.showRepo.GetWithSeasonsAndEpisodesContext(c.Request.Context(), id)
	if err != nil {
		errorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}
	if show == nil {
		errorResponse(c, http.StatusNotFound, "Show not found")
		return
	}

	c.JSON(http.StatusOK, toShowResponse(show))
}

func (s *Server) refreshShow(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if s.opts.Metadata == nil {
		errorResponse(c, http.StatusServiceUnavailable, "Metadata service not configured")
		return
	}

	result, err := s.opts.Metadata.RefreshShow(c.Request.Context(), id)
	if err != nil {
		metadataErrorResponse(c, err, http.StatusNotFound)
		return
	}

	seasonErrors := make([]string, 0, len(result.SeasonErrors))
	for _, se := range result.SeasonErrors {
		slog.Warn("Season refresh error", "show_id", id, "error", se.Error())
		seasonErrors = append(seasonErrors, se.Error())
	}

	c.JSON(http.StatusOK, RefreshShowResponse{
		Show:          toShowResponse(result.Show),
		SeasonsAdded:  result.SeasonsAdded,
		EpisodesAdded: result.EpisodesAdded,
		SeasonErrors:  seasonErrors,
	})
}

// deleteShow removes a show with its episodes, their subtitles and the
// torrents nothing else uses (PLAN §6 "Titles and library").
// DELETE /api/shows/:id?delete_data=true|false → 204
func (s *Server) deleteShow(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	deleteData, ok := parseDeleteData(c)
	if !ok || !s.removalsReady(c) {
		return
	}
	if err := s.opts.Removals.DeleteShow(c.Request.Context(), id, deleteData); err != nil {
		removalErrorResponse(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// Episode handlers

// unassignEpisodeTorrent drops the episode's file and the subtitles that
// came with it, and releases its torrent when nothing else uses it.
// DELETE /api/episodes/:id/assign → 204
func (s *Server) unassignEpisodeTorrent(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok || !s.removalsReady(c) {
		return
	}
	if err := s.opts.Removals.UnassignEpisode(c.Request.Context(), id); err != nil {
		removalErrorResponse(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// Helper functions

func toMovieResponse(movie *library.Movie, assignment *library.TorrentAssignment) MovieResponse {
	resp := MovieResponse{
		ID:            movie.ID,
		TMDBID:        movie.TMDBID,
		Title:         movie.Title,
		Year:          movie.Year,
		HasAssignment: assignment != nil,
	}
	if assignment != nil {
		resp.Assignment = toAssignmentResponse(assignment)
	}
	return resp
}

func toShowResponse(show *library.Show) ShowResponse {
	resp := ShowResponse{
		ID:     show.ID,
		TMDBID: show.TMDBID,
		Title:  show.Title,
		Year:   show.Year,
	}

	for _, season := range show.Seasons {
		if season.SeasonNumber == 0 && !show.IncludeSpecials {
			continue // Specials switched off: rows are kept, but hidden
		}
		seasonResp := SeasonResponse{
			ID:           season.ID,
			SeasonNumber: season.SeasonNumber,
		}

		for _, ep := range season.Episodes {
			epResp := EpisodeResponse{
				ID:            ep.ID,
				EpisodeNumber: ep.EpisodeNumber,
				Name:          ep.Name,
				HasAssignment: ep.Assignment != nil,
			}
			if ep.Assignment != nil {
				epResp.Assignment = toAssignmentResponse(ep.Assignment)
			}
			seasonResp.Episodes = append(seasonResp.Episodes, epResp)
		}

		resp.Seasons = append(resp.Seasons, seasonResp)
	}

	return resp
}

func toAssignmentResponse(a *library.TorrentAssignment) *AssignmentResponse {
	return &AssignmentResponse{
		ID:         a.ID,
		InfoHash:   a.InfoHash,
		FilePath:   a.FilePath,
		FileSize:   a.FileSize,
		Resolution: a.Resolution,
		Source:     a.Source,
	}
}

// Recently aired episode response types
type RecentlyAiredEpisodeResponse struct {
	ShowID        int64  `json:"show_id"`
	ShowTMDBID    int    `json:"show_tmdb_id"`
	ShowTitle     string `json:"show_title"`
	ShowYear      int    `json:"show_year"`
	SeasonNumber  int    `json:"season_number"`
	EpisodeID     int64  `json:"episode_id"`
	EpisodeNumber int    `json:"episode_number"`
	EpisodeName   string `json:"episode_name"`
	AirDate       string `json:"air_date"`
	HasAssignment bool   `json:"has_assignment"`
}

type RecentlyAiredResponse struct {
	Episodes     []RecentlyAiredEpisodeResponse `json:"episodes"`
	LastSyncTime string                         `json:"last_sync_time"`
	SyncStatus   string                         `json:"sync_status"`
}

// getRecentlyAiredEpisodes returns episodes that recently aired
func (s *Server) getRecentlyAiredEpisodes(c *gin.Context) {
	// Parse optional lookback_days query param (default: 30)
	lookbackDays := 30
	if s.airDateSync != nil {
		lookbackDays = s.airDateSync.GetLookbackDays()
	}
	if days := c.Query("lookback_days"); days != "" {
		if parsed, err := strconv.Atoi(days); err == nil && parsed > 0 && parsed <= 90 {
			lookbackDays = parsed
		}
	}

	episodes, err := s.showRepo.GetRecentlyAiredEpisodes(lookbackDays)
	if err != nil {
		errorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}

	// Convert to response type
	respEpisodes := make([]RecentlyAiredEpisodeResponse, len(episodes))
	for i, ep := range episodes {
		respEpisodes[i] = RecentlyAiredEpisodeResponse{
			ShowID:        ep.ShowID,
			ShowTMDBID:    ep.ShowTMDBID,
			ShowTitle:     ep.ShowTitle,
			ShowYear:      ep.ShowYear,
			SeasonNumber:  ep.SeasonNumber,
			EpisodeID:     ep.EpisodeID,
			EpisodeNumber: ep.EpisodeNumber,
			EpisodeName:   ep.EpisodeName,
			AirDate:       ep.AirDate,
			HasAssignment: ep.HasAssignment,
		}
	}

	// Get sync status
	var lastSyncTime string
	var syncStatus string
	if s.airDateSync != nil {
		lastSync, status, _ := s.airDateSync.GetStatus()
		if !lastSync.IsZero() {
			lastSyncTime = lastSync.Format("2006-01-02T15:04:05Z07:00")
		}
		syncStatus = status
	} else {
		syncStatus = "disabled"
	}

	c.JSON(http.StatusOK, RecentlyAiredResponse{
		Episodes:     respEpisodes,
		LastSyncTime: lastSyncTime,
		SyncStatus:   syncStatus,
	})
}

// triggerAirDateSync manually triggers an air date sync
func (s *Server) triggerAirDateSync(c *gin.Context) {
	if s.airDateSync == nil {
		errorResponse(c, http.StatusServiceUnavailable, "Air date sync service not configured")
		return
	}

	// Start sync in background
	go func() {
		if err := s.airDateSync.TriggerSync(); err != nil {
			slog.Warn("Manual air date sync failed", "error", err)
		}
	}()

	c.JSON(http.StatusAccepted, gin.H{
		"message": "Air date sync started",
	})
}
