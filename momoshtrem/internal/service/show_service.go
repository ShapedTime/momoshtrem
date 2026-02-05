package service

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/tmdb"
)

// TMDBClient defines the TMDB operations needed by ShowService.
// Defined at point of use for minimal coupling.
type TMDBClient interface {
	GetShowDetails(id int) (*tmdb.ShowDetails, error)
	GetSeason(showID int, seasonNumber int) (*tmdb.Season, error)
}

// Compile-time verification that tmdb.Client implements TMDBClient
var _ TMDBClient = (*tmdb.Client)(nil)

// ShowService manages show lifecycle operations.
type ShowService struct {
	showRepo   *library.ShowRepository
	tmdbClient TMDBClient
	log        *slog.Logger
}

// NewShowService creates a new ShowService.
func NewShowService(showRepo *library.ShowRepository, tmdbClient TMDBClient) *ShowService {
	return &ShowService{
		showRepo:   showRepo,
		tmdbClient: tmdbClient,
		log:        slog.With("component", "show-service"),
	}
}

// CreateShowInput contains parameters for creating a show.
type CreateShowInput struct {
	TMDBID  int
	Seasons []int // Optional: specific seasons to add. Empty means all.
}

// CreateShowResult contains the result of show creation.
type CreateShowResult struct {
	Show         *library.Show
	IsExisting   bool          // True if show already existed
	SeasonErrors []SeasonError // Non-fatal errors during season/episode creation
}

// SeasonError represents an error that occurred while creating a specific season.
type SeasonError struct {
	SeasonNumber int
	Err          error
}

func (e SeasonError) Error() string {
	return fmt.Sprintf("season %d: %v", e.SeasonNumber, e.Err)
}

// Create creates a new show from TMDB, including seasons and episodes.
// If the show already exists, returns the existing show.
// Season/episode creation failures are collected in SeasonErrors rather than
// aborting the entire operation.
func (s *ShowService) Create(ctx context.Context, input CreateShowInput) (*CreateShowResult, error) {
	result := &CreateShowResult{}

	// 1. Check for existing show
	existing, err := s.showRepo.GetByTMDBID(input.TMDBID)
	if err != nil {
		return nil, fmt.Errorf("failed to check existing show: %w", err)
	}
	if existing != nil {
		show, err := s.showRepo.GetWithSeasonsAndEpisodes(existing.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to load existing show: %w", err)
		}
		result.Show = show
		result.IsExisting = true
		return result, nil
	}

	// 2. Fetch show details from TMDB
	tmdbShow, err := s.tmdbClient.GetShowDetails(input.TMDBID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch show from TMDB: %w", err)
	}

	// 3. Create show record
	show := &library.Show{
		TMDBID: tmdbShow.ID,
		Title:  tmdbShow.Name,
		Year:   tmdbShow.Year(),
	}
	if err := s.showRepo.Create(show); err != nil {
		return nil, fmt.Errorf("failed to create show: %w", err)
	}

	// 4. Determine which seasons to add
	seasonsToAdd := input.Seasons
	if len(seasonsToAdd) == 0 {
		for _, season := range tmdbShow.Seasons {
			if season.SeasonNumber > 0 { // Skip specials (season 0)
				seasonsToAdd = append(seasonsToAdd, season.SeasonNumber)
			}
		}
	}

	// 5. Create seasons and episodes
	for _, seasonNum := range seasonsToAdd {
		if err := s.createSeason(ctx, show.ID, input.TMDBID, seasonNum); err != nil {
			result.SeasonErrors = append(result.SeasonErrors, SeasonError{
				SeasonNumber: seasonNum,
				Err:          err,
			})
			s.log.Warn("Failed to create season",
				"show_id", show.ID,
				"season", seasonNum,
				"error", err,
			)
		}
	}

	// 6. Reload with full hierarchy
	show, err = s.showRepo.GetWithSeasonsAndEpisodes(show.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to reload show: %w", err)
	}

	result.Show = show
	return result, nil
}

// createSeason creates a season and its episodes.
func (s *ShowService) createSeason(ctx context.Context, showID int64, tmdbID int, seasonNum int) error {
	// Create season record
	season := &library.Season{
		ShowID:       showID,
		SeasonNumber: seasonNum,
	}
	if err := s.showRepo.CreateSeason(season); err != nil {
		return fmt.Errorf("failed to create season record: %w", err)
	}

	// Fetch episodes from TMDB
	tmdbSeason, err := s.tmdbClient.GetSeason(tmdbID, seasonNum)
	if err != nil {
		return fmt.Errorf("failed to fetch season from TMDB: %w", err)
	}

	// Create episode records
	for _, ep := range tmdbSeason.Episodes {
		episode := &library.Episode{
			SeasonID:      season.ID,
			EpisodeNumber: ep.EpisodeNumber,
			Name:          ep.Name,
		}
		if err := s.showRepo.CreateEpisode(episode); err != nil {
			s.log.Warn("Failed to create episode",
				"season_id", season.ID,
				"episode", ep.EpisodeNumber,
				"error", err,
			)
			// Continue with other episodes
		}
	}

	return nil
}
