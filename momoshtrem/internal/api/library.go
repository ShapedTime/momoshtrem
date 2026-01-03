package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/shapedtime/momoshtrem/internal/library"
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

// Assignment request types
type AssignTorrentRequest struct {
	MagnetURI  string `json:"magnet_uri" binding:"required"`
	FilePath   string `json:"file_path" binding:"required"`
	FileSize   int64  `json:"file_size" binding:"required"`
	Resolution string `json:"resolution,omitempty"`
	Source     string `json:"source,omitempty"`
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
		assignment, _ := s.assignmentRepo.GetActiveForItem(library.ItemTypeMovie, movie.ID)
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
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		errorResponse(c, http.StatusBadRequest, "Invalid ID")
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

	assignment, _ := s.assignmentRepo.GetActiveForItem(library.ItemTypeMovie, movie.ID)
	c.JSON(http.StatusOK, toMovieResponse(movie, assignment))
}

func (s *Server) deleteMovie(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		errorResponse(c, http.StatusBadRequest, "Invalid ID")
		return
	}

	// Deactivate assignments first
	_ = s.assignmentRepo.DeactivateForItem(library.ItemTypeMovie, id)

	if err := s.movieRepo.Delete(id); err != nil {
		errorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}

	c.Status(http.StatusNoContent)
}

func (s *Server) assignMovieTorrent(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		errorResponse(c, http.StatusBadRequest, "Invalid ID")
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
	infoHash := extractInfoHash(req.MagnetURI)
	if infoHash == "" {
		errorResponse(c, http.StatusBadRequest, "Invalid magnet URI")
		return
	}

	assignment := &library.TorrentAssignment{
		ItemType:   library.ItemTypeMovie,
		ItemID:     id,
		InfoHash:   infoHash,
		MagnetURI:  req.MagnetURI,
		FilePath:   req.FilePath,
		FileSize:   req.FileSize,
		Resolution: req.Resolution,
		Source:     req.Source,
	}

	if err := s.assignmentRepo.Create(assignment); err != nil {
		errorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}

	c.JSON(http.StatusCreated, toAssignmentResponse(assignment))
}

func (s *Server) unassignMovieTorrent(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		errorResponse(c, http.StatusBadRequest, "Invalid ID")
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
		show, _ := s.showRepo.GetWithSeasonsAndEpisodes(existing.ID)
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
			continue // Skip on error
		}

		// Fetch episodes from TMDB
		tmdbSeason, err := s.tmdbClient.GetSeason(req.TMDBID, seasonNum)
		if err != nil {
			continue
		}

		for _, ep := range tmdbSeason.Episodes {
			episode := &library.Episode{
				SeasonID:      season.ID,
				EpisodeNumber: ep.EpisodeNumber,
				Name:          ep.Name,
			}
			_ = s.showRepo.CreateEpisode(episode)
		}

		show.Seasons = append(show.Seasons, *season)
	}

	// Reload with full data
	show, _ = s.showRepo.GetWithSeasonsAndEpisodes(show.ID)
	c.JSON(http.StatusCreated, toShowResponse(show))
}

func (s *Server) getShow(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		errorResponse(c, http.StatusBadRequest, "Invalid ID")
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
			assignment, _ := s.assignmentRepo.GetActiveForItem(library.ItemTypeEpisode, ep.ID)
			ep.Assignment = assignment
		}
	}

	c.JSON(http.StatusOK, toShowResponse(show))
}

func (s *Server) deleteShow(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		errorResponse(c, http.StatusBadRequest, "Invalid ID")
		return
	}

	// Get all episodes to deactivate their assignments
	show, _ := s.showRepo.GetWithSeasonsAndEpisodes(id)
	if show != nil {
		for _, season := range show.Seasons {
			for _, ep := range season.Episodes {
				_ = s.assignmentRepo.DeactivateForItem(library.ItemTypeEpisode, ep.ID)
			}
		}
	}

	if err := s.showRepo.Delete(id); err != nil {
		errorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}

	c.Status(http.StatusNoContent)
}

// Episode handlers

func (s *Server) assignEpisodeTorrent(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		errorResponse(c, http.StatusBadRequest, "Invalid ID")
		return
	}

	var req AssignTorrentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		errorResponse(c, http.StatusBadRequest, err.Error())
		return
	}

	// Verify episode exists
	episode, err := s.showRepo.GetEpisodeByID(id)
	if err != nil {
		errorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}
	if episode == nil {
		errorResponse(c, http.StatusNotFound, "Episode not found")
		return
	}

	// Extract info hash from magnet URI
	infoHash := extractInfoHash(req.MagnetURI)
	if infoHash == "" {
		errorResponse(c, http.StatusBadRequest, "Invalid magnet URI")
		return
	}

	assignment := &library.TorrentAssignment{
		ItemType:   library.ItemTypeEpisode,
		ItemID:     id,
		InfoHash:   infoHash,
		MagnetURI:  req.MagnetURI,
		FilePath:   req.FilePath,
		FileSize:   req.FileSize,
		Resolution: req.Resolution,
		Source:     req.Source,
	}

	if err := s.assignmentRepo.Create(assignment); err != nil {
		errorResponse(c, http.StatusInternalServerError, err.Error())
		return
	}

	c.JSON(http.StatusCreated, toAssignmentResponse(assignment))
}

func (s *Server) unassignEpisodeTorrent(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		errorResponse(c, http.StatusBadRequest, "Invalid ID")
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

// extractInfoHash extracts the info hash from a magnet URI
func extractInfoHash(magnetURI string) string {
	// Look for xt=urn:btih: in the URI
	const prefix = "xt=urn:btih:"
	idx := 0
	for i := 0; i < len(magnetURI)-len(prefix); i++ {
		if magnetURI[i:i+len(prefix)] == prefix {
			idx = i + len(prefix)
			break
		}
	}
	if idx == 0 {
		return ""
	}

	// Extract hash until next & or end
	end := idx
	for end < len(magnetURI) && magnetURI[end] != '&' {
		end++
	}

	hash := magnetURI[idx:end]

	// Handle base32 encoded hashes (40 chars) vs hex (40 chars)
	// Normalize to lowercase
	result := make([]byte, len(hash))
	for i := 0; i < len(hash); i++ {
		c := hash[i]
		if c >= 'A' && c <= 'Z' {
			c = c + 32 // lowercase
		}
		result[i] = c
	}

	return string(result)
}
