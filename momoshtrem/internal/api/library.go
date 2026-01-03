package api

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/shapedtime/momoshtrem/internal/identify"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/torrent"
)

// Movie request/response types
type CreateMovieRequest struct {
	TMDBID int `json:"tmdb_id" binding:"required"`
}

type MovieResponse struct {
	ID            int64  `json:"id"`
	TMDBID        int    `json:"tmdb_id"`
	Title         string `json:"title"`
	Year          int    `json:"year"`
	HasAssignment bool   `json:"has_assignment"`
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

// Assignment request types - auto-detection API
type AssignTorrentRequest struct {
	MagnetURI string `json:"magnet_uri" binding:"required"`
}

// Movie assignment response
type MovieAssignmentResponse struct {
	Success    bool                `json:"success"`
	Assignment *AssignmentResponse `json:"assignment,omitempty"`
	Error      string              `json:"error,omitempty"`
}

// Show assignment response
type ShowAssignmentResponse struct {
	Success   bool                   `json:"success"`
	Summary   AssignmentSummary      `json:"summary"`
	Matched   []MatchedAssignment    `json:"matched"`
	Unmatched []UnmatchedAssignment  `json:"unmatched,omitempty"`
	Error     string                 `json:"error,omitempty"`
}

type AssignmentSummary struct {
	TotalFiles int `json:"total_files"`
	Matched    int `json:"matched"`
	Unmatched  int `json:"unmatched"`
	Skipped    int `json:"skipped"`
}

type MatchedAssignment struct {
	EpisodeID  int64  `json:"episode_id"`
	Season     int    `json:"season"`
	Episode    int    `json:"episode"`
	FilePath   string `json:"file_path"`
	FileSize   int64  `json:"file_size"`
	Resolution string `json:"resolution,omitempty"`
	Confidence string `json:"confidence"`
}

type UnmatchedAssignment struct {
	FilePath string `json:"file_path"`
	Reason   string `json:"reason"`
	Season   int    `json:"season"`
	Episode  int    `json:"episode"`
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
	movies, err := s.movieRepo.List()
	if err != nil {
		errorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}

	// Get assignments for each movie
	response := make([]MovieResponse, len(movies))
	for i, movie := range movies {
		assignment, err := s.assignmentRepo.GetActiveForItem(library.ItemTypeMovie, movie.ID)
		if err != nil {
			slog.Error("Failed to get assignment for movie", "movie_id", movie.ID, "error", err)
		}
		response[i] = toMovieResponse(movie, assignment)
	}

	c.JSON(http.StatusOK, response)
}

func (s *Server) createMovie(c *gin.Context) {
	var req CreateMovieRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	// Check if already exists
	existing, err := s.movieRepo.GetByTMDBID(req.TMDBID)
	if err != nil {
		errorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}
	if existing != nil {
		c.JSON(http.StatusOK, toMovieResponse(existing, nil))
		return
	}

	// Fetch from TMDB
	tmdbMovie, err := s.tmdbClient.GetMovie(req.TMDBID)
	if err != nil {
		errorResponse(c, http.StatusBadRequest, "Failed to fetch from TMDB: "+err.Error())
		return
	}

	movie := &library.Movie{
		TMDBID: tmdbMovie.ID,
		Title:  tmdbMovie.Title,
		Year:   tmdbMovie.Year(),
	}

	if err := s.movieRepo.Create(movie); err != nil {
		errorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}

	c.JSON(http.StatusCreated, toMovieResponse(movie, nil))
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
		slog.Error("Failed to get assignment for movie", "movie_id", movie.ID, "error", err)
	}
	c.JSON(http.StatusOK, toMovieResponse(movie, assignment))
}

func (s *Server) deleteMovie(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}

	// Deactivate assignments first
	if err := s.assignmentRepo.DeactivateForItem(library.ItemTypeMovie, id); err != nil {
		slog.Error("Failed to deactivate assignments for movie", "movie_id", id, "error", err)
	}

	if err := s.movieRepo.Delete(id); err != nil {
		errorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}

	c.Status(http.StatusNoContent)
}

func (s *Server) assignMovieTorrent(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}

	var req AssignTorrentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	// Verify movie exists
	movie, err := s.movieRepo.GetByID(id)
	if err != nil {
		errorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}
	if movie == nil {
		errorResponse(c, http.StatusNotFound, "Movie not found")
		return
	}

	// Extract info hash from magnet URI
	infoHash := torrent.ExtractInfoHash(req.MagnetURI)
	if infoHash == "" {
		errorResponse(c, http.StatusBadRequest, "Invalid magnet URI")
		return
	}

	// Check if torrent service is available
	if s.torrentService == nil {
		errorResponse(c, http.StatusServiceUnavailable, "Torrent service not available - Stage 2 required")
		return
	}

	// Add torrent and get file list
	torrentInfo, err := s.torrentService.AddTorrent(req.MagnetURI)
	if err != nil {
		errorResponse(c, http.StatusInternalServerError, "Failed to add torrent: "+err.Error())
		return
	}

	// Find the best movie file (largest video file)
	result := identify.FindMovieFile(torrentInfo.Files)
	if !result.Found {
		errorResponse(c, http.StatusBadRequest, "No video files found in torrent")
		return
	}

	// Log other files if any
	if len(result.OtherFiles) > 0 {
		slog.Info("Movie torrent has multiple video files, using largest",
			"movie_id", id,
			"selected", result.FilePath,
			"others", result.OtherFiles,
		)
	}

	assignment := &library.TorrentAssignment{
		ItemType:   library.ItemTypeMovie,
		ItemID:     id,
		InfoHash:   infoHash,
		MagnetURI:  req.MagnetURI,
		FilePath:   result.FilePath,
		FileSize:   result.FileSize,
		Resolution: result.Quality.Resolution,
		Source:     result.Quality.Source,
	}

	if err := s.assignmentRepo.Create(assignment); err != nil {
		errorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}

	c.JSON(http.StatusCreated, MovieAssignmentResponse{
		Success:    true,
		Assignment: toAssignmentResponse(assignment),
	})
}

func (s *Server) unassignMovieTorrent(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}

	if err := s.assignmentRepo.DeactivateForItem(library.ItemTypeMovie, id); err != nil {
		errorResponse(c, http.StatusInternalServerError, err.Error())
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

	// Check if already exists
	existing, err := s.showRepo.GetByTMDBID(req.TMDBID)
	if err != nil {
		errorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}
	if existing != nil {
		// Return existing with seasons
		show, err := s.showRepo.GetWithSeasonsAndEpisodes(existing.ID)
		if err != nil {
			slog.Error("Failed to get show with seasons", "show_id", existing.ID, "error", err)
		}
		c.JSON(http.StatusOK, toShowResponse(show))
		return
	}

	// Fetch from TMDB
	tmdbShow, err := s.tmdbClient.GetShowDetails(req.TMDBID)
	if err != nil {
		errorResponse(c, http.StatusBadRequest, "Failed to fetch from TMDB: "+err.Error())
		return
	}

	show := &library.Show{
		TMDBID: tmdbShow.ID,
		Title:  tmdbShow.Name,
		Year:   tmdbShow.Year(),
	}

	if err := s.showRepo.Create(show); err != nil {
		errorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}

	// Add seasons and episodes
	seasonsToAdd := req.Seasons
	if len(seasonsToAdd) == 0 {
		// Add all seasons
		for _, s := range tmdbShow.Seasons {
			if s.SeasonNumber > 0 { // Skip specials (season 0)
				seasonsToAdd = append(seasonsToAdd, s.SeasonNumber)
			}
		}
	}

	for _, seasonNum := range seasonsToAdd {
		season := &library.Season{
			ShowID:       show.ID,
			SeasonNumber: seasonNum,
		}
		if err := s.showRepo.CreateSeason(season); err != nil {
			slog.Error("Failed to create season", "show_id", show.ID, "season", seasonNum, "error", err)
			continue
		}

		// Fetch episodes from TMDB
		tmdbSeason, err := s.tmdbClient.GetSeason(req.TMDBID, seasonNum)
		if err != nil {
			slog.Error("Failed to fetch season from TMDB", "tmdb_id", req.TMDBID, "season", seasonNum, "error", err)
			continue
		}

		for _, ep := range tmdbSeason.Episodes {
			episode := &library.Episode{
				SeasonID:      season.ID,
				EpisodeNumber: ep.EpisodeNumber,
				Name:          ep.Name,
			}
			if err := s.showRepo.CreateEpisode(episode); err != nil {
				slog.Error("Failed to create episode", "season_id", season.ID, "episode", ep.EpisodeNumber, "error", err)
			}
		}

		show.Seasons = append(show.Seasons, *season)
	}

	// Reload with full data
	show, err = s.showRepo.GetWithSeasonsAndEpisodes(show.ID)
	if err != nil {
		slog.Error("Failed to reload show with seasons", "show_id", show.ID, "error", err)
	}
	c.JSON(http.StatusCreated, toShowResponse(show))
}

func (s *Server) getShow(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}

	show, err := s.showRepo.GetWithSeasonsAndEpisodes(id)
	if err != nil {
		errorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}
	if show == nil {
		errorResponse(c, http.StatusNotFound, "Show not found")
		return
	}

	// Load assignments for episodes
	for i := range show.Seasons {
		for j := range show.Seasons[i].Episodes {
			ep := &show.Seasons[i].Episodes[j]
			assignment, err := s.assignmentRepo.GetActiveForItem(library.ItemTypeEpisode, ep.ID)
			if err != nil {
				slog.Error("Failed to get assignment for episode", "episode_id", ep.ID, "error", err)
			}
			ep.Assignment = assignment
		}
	}

	c.JSON(http.StatusOK, toShowResponse(show))
}

func (s *Server) deleteShow(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}

	// Get all episodes to deactivate their assignments
	show, err := s.showRepo.GetWithSeasonsAndEpisodes(id)
	if err != nil {
		slog.Error("Failed to get show for deletion", "show_id", id, "error", err)
	}
	if show != nil {
		for _, season := range show.Seasons {
			for _, ep := range season.Episodes {
				if err := s.assignmentRepo.DeactivateForItem(library.ItemTypeEpisode, ep.ID); err != nil {
					slog.Error("Failed to deactivate assignment for episode", "episode_id", ep.ID, "error", err)
				}
			}
		}
	}

	if err := s.showRepo.Delete(id); err != nil {
		errorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}

	c.Status(http.StatusNoContent)
}

// Show assignment handler - auto-detects episodes from torrent

func (s *Server) assignShowTorrent(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}

	var req AssignTorrentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	// Verify show exists and load with all seasons/episodes
	show, err := s.showRepo.GetWithSeasonsAndEpisodes(id)
	if err != nil {
		errorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}
	if show == nil {
		errorResponse(c, http.StatusNotFound, "Show not found")
		return
	}

	// Extract info hash from magnet URI
	infoHash := torrent.ExtractInfoHash(req.MagnetURI)
	if infoHash == "" {
		errorResponse(c, http.StatusBadRequest, "Invalid magnet URI")
		return
	}

	// Check if torrent service is available
	if s.torrentService == nil {
		errorResponse(c, http.StatusServiceUnavailable, "Torrent service not available - Stage 2 required")
		return
	}

	// Add torrent and get file list
	torrentInfo, err := s.torrentService.AddTorrent(req.MagnetURI)
	if err != nil {
		errorResponse(c, http.StatusInternalServerError, "Failed to add torrent: "+err.Error())
		return
	}

	// Identify episodes in the torrent
	identResult := s.identifier.Identify(torrentInfo.Files, torrentInfo.Name)

	// Match identified files to library episodes
	matchResult := identify.MatchToShow(show, identResult)

	// Create assignments for matched episodes
	matched := make([]MatchedAssignment, 0, len(matchResult.Matched))
	for _, m := range matchResult.Matched {
		assignment := &library.TorrentAssignment{
			ItemType:   library.ItemTypeEpisode,
			ItemID:     m.Episode.ID,
			InfoHash:   infoHash,
			MagnetURI:  req.MagnetURI,
			FilePath:   m.FilePath,
			FileSize:   m.FileSize,
			Resolution: m.Quality.Resolution,
			Source:     m.Quality.Source,
		}

		if err := s.assignmentRepo.Create(assignment); err != nil {
			slog.Error("Failed to create assignment",
				"episode_id", m.Episode.ID,
				"error", err,
			)
			continue
		}

		matched = append(matched, MatchedAssignment{
			EpisodeID:  m.Episode.ID,
			Season:     m.Season.SeasonNumber,
			Episode:    m.Episode.EpisodeNumber,
			FilePath:   m.FilePath,
			FileSize:   m.FileSize,
			Resolution: m.Quality.Resolution,
			Confidence: string(m.Confidence),
		})
	}

	// Build unmatched response
	unmatched := make([]UnmatchedAssignment, 0, len(matchResult.Unmatched))
	for _, u := range matchResult.Unmatched {
		unmatched = append(unmatched, UnmatchedAssignment{
			FilePath: u.FilePath,
			Reason:   string(u.Reason),
			Season:   u.Season,
			Episode:  u.Episode,
		})

		// Log unmatched files for investigation
		slog.Warn("Unmatched file in torrent",
			"show_id", id,
			"show_title", show.Title,
			"info_hash", infoHash,
			"file_path", u.FilePath,
			"reason", u.Reason,
			"parsed_season", u.Season,
			"parsed_episode", u.Episode,
		)
	}

	// Calculate skipped count (non-video files that were filtered out)
	skipped := identResult.TotalFiles - len(identResult.IdentifiedFiles) - len(identResult.UnidentifiedFiles)
	if skipped < 0 {
		skipped = 0
	}

	c.JSON(http.StatusCreated, ShowAssignmentResponse{
		Success: true,
		Summary: AssignmentSummary{
			TotalFiles: identResult.TotalFiles,
			Matched:    len(matched),
			Unmatched:  len(unmatched),
			Skipped:    skipped,
		},
		Matched:   matched,
		Unmatched: unmatched,
	})
}

// Episode handlers

func (s *Server) unassignEpisodeTorrent(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}

	if err := s.assignmentRepo.DeactivateForItem(library.ItemTypeEpisode, id); err != nil {
		errorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}

	c.Status(http.StatusNoContent)
}

// Status handler

func (s *Server) getStatus(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "ok",
	})
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
