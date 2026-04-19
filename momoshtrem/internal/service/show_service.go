package service

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"

	"golang.org/x/sync/singleflight"

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
	showRepo     *library.ShowRepository
	tmdbClient   TMDBClient
	log          *slog.Logger
	refreshGroup singleflight.Group // Dedup concurrent Refresh calls by show ID
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
		if _, _, err := s.createSeason(ctx, show.ID, input.TMDBID, seasonNum); err != nil {
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

// RefreshShowResult contains the result of refreshing a show from TMDB.
type RefreshShowResult struct {
	Show          *library.Show
	SeasonsAdded  int
	EpisodesAdded int
	SeasonErrors  []SeasonError
}

// Refresh re-fetches a show's seasons and episodes from TMDB, upserting any
// missing records. Existing seasons, episodes, and torrent assignments are
// preserved; episode names are updated if TMDB has changed them.
//
// Concurrent calls for the same show are deduplicated via singleflight:
// callers arriving while a refresh is in progress share its result instead
// of triggering parallel TMDB fetches and duplicate upserts.
func (s *ShowService) Refresh(ctx context.Context, showID int64) (*RefreshShowResult, error) {
	key := strconv.FormatInt(showID, 10)
	v, err, _ := s.refreshGroup.Do(key, func() (any, error) {
		return s.doRefresh(ctx, showID)
	})
	if err != nil {
		return nil, err
	}
	return v.(*RefreshShowResult), nil
}

func (s *ShowService) doRefresh(ctx context.Context, showID int64) (*RefreshShowResult, error) {
	result := &RefreshShowResult{}

	show, err := s.showRepo.GetByID(showID)
	if err != nil {
		return nil, fmt.Errorf("failed to load show: %w", err)
	}
	if show == nil {
		return nil, library.ErrShowNotFound
	}

	tmdbShow, err := s.tmdbClient.GetShowDetails(show.TMDBID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch show from TMDB: %w", err)
	}

	for _, tmdbSeason := range tmdbShow.Seasons {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if tmdbSeason.SeasonNumber <= 0 {
			continue // Skip specials
		}

		seasonCreated, episodesAdded, err := s.createSeason(ctx, show.ID, show.TMDBID, tmdbSeason.SeasonNumber)
		if err != nil {
			result.SeasonErrors = append(result.SeasonErrors, SeasonError{
				SeasonNumber: tmdbSeason.SeasonNumber,
				Err:          err,
			})
			s.log.Warn("Failed to refresh season",
				"show_id", show.ID,
				"season", tmdbSeason.SeasonNumber,
				"error", err,
			)
			continue
		}
		if seasonCreated {
			result.SeasonsAdded++
		}
		result.EpisodesAdded += episodesAdded
	}

	// Reload with full hierarchy. If this fails after successful mutations,
	// surface the counts with the basic show rather than discarding progress;
	// the client can re-fetch the show to get the nested seasons/episodes.
	reloaded, err := s.showRepo.GetWithSeasonsAndEpisodes(show.ID)
	if err != nil {
		s.log.Error("Failed to reload show after refresh; returning partial result",
			"show_id", show.ID,
			"seasons_added", result.SeasonsAdded,
			"episodes_added", result.EpisodesAdded,
			"error", err,
		)
		result.Show = show
		result.SeasonErrors = append(result.SeasonErrors, SeasonError{
			SeasonNumber: 0,
			Err:          fmt.Errorf("reload after refresh failed: %w", err),
		})
		return result, nil
	}
	result.Show = reloaded
	return result, nil
}

// createSeason upserts a season and its episodes from TMDB.
// Returns whether the season row was newly created and the number of episode
// numbers that did not previously exist for the season. Safe to call on an
// existing season — underlying repo methods use ON CONFLICT DO UPDATE.
func (s *ShowService) createSeason(ctx context.Context, showID int64, tmdbID int, seasonNum int) (seasonCreated bool, episodesAdded int, err error) {
	existingSeason, err := s.showRepo.GetSeason(showID, seasonNum)
	if err != nil {
		return false, 0, fmt.Errorf("failed to check existing season: %w", err)
	}

	var existingEpisodeNums map[int]struct{}
	if existingSeason != nil {
		existing, err := s.showRepo.GetEpisodes(existingSeason.ID)
		if err != nil {
			return false, 0, fmt.Errorf("failed to load existing episodes: %w", err)
		}
		existingEpisodeNums = make(map[int]struct{}, len(existing))
		for _, ep := range existing {
			existingEpisodeNums[ep.EpisodeNumber] = struct{}{}
		}
	}

	season := &library.Season{
		ShowID:       showID,
		SeasonNumber: seasonNum,
	}
	if err := s.showRepo.CreateSeason(season); err != nil {
		return false, 0, fmt.Errorf("failed to create season record: %w", err)
	}
	seasonCreated = existingSeason == nil

	tmdbSeason, err := s.tmdbClient.GetSeason(tmdbID, seasonNum)
	if err != nil {
		return seasonCreated, 0, fmt.Errorf("failed to fetch season from TMDB: %w", err)
	}

	for _, ep := range tmdbSeason.Episodes {
		_, had := existingEpisodeNums[ep.EpisodeNumber]
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
			continue
		}
		if !had {
			episodesAdded++
		}
	}

	return seasonCreated, episodesAdded, nil
}
