package airdate

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/shapedtime/momoshtrem/internal/config"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/tmdb"
)

// SeasonSource fetches a TMDB season. *tmdb.Cached implements it, so air
// dates come from the shared cache (a day old at most while a show airs).
type SeasonSource interface {
	Season(ctx context.Context, showID, seasonNumber int, showStatus string) (*tmdb.Season, error)
}

var _ SeasonSource = (*tmdb.Cached)(nil)

// SyncService manages background synchronization of episode air dates from TMDB
type SyncService struct {
	mu       sync.RWMutex
	config   config.AirDateSyncConfig
	showRepo *library.ShowRepository
	syncRepo *library.SyncMetadataRepository
	seasons  SeasonSource

	lastSync   time.Time
	syncStatus string // "ok", "pending", "in_progress", "error"
	lastError  error

	stopChan chan struct{}
	stopped  bool
	log      *slog.Logger
}

// NewSyncService creates a new air date sync service
func NewSyncService(
	cfg config.AirDateSyncConfig,
	showRepo *library.ShowRepository,
	syncRepo *library.SyncMetadataRepository,
	seasons SeasonSource,
) *SyncService {
	return &SyncService{
		config:     cfg,
		showRepo:   showRepo,
		syncRepo:   syncRepo,
		seasons:    seasons,
		syncStatus: "pending",
		stopChan:   make(chan struct{}),
		log:        slog.With("component", "airdate-sync"),
	}
}

// Start begins the background sync loop
func (s *SyncService) Start() {
	s.log.Info("Air date sync service started",
		"interval_hours", s.config.SyncIntervalHours,
		"lookback_days", s.config.LookbackDays,
		"batch_size", s.config.BatchSize,
	)
	go s.syncLoop()
}

// Stop halts the background sync
func (s *SyncService) Stop() {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return
	}
	s.stopped = true
	s.mu.Unlock()
	close(s.stopChan)
	s.log.Info("Air date sync service stopped")
}

// SyncSingleShow syncs air dates for a single show (called when a show is added)
func (s *SyncService) SyncSingleShow(show *library.Show) error {
	return s.syncShowAirDates(context.Background(), show)
}

func (s *SyncService) syncLoop() {
	// Check if we need to sync on startup
	s.checkAndSync()

	ticker := time.NewTicker(time.Hour) // Check every hour
	defer ticker.Stop()

	for {
		select {
		case <-s.stopChan:
			return
		case <-ticker.C:
			s.checkAndSync()
		}
	}
}

func (s *SyncService) checkAndSync() {
	lastSync, err := s.syncRepo.GetLastSyncTime("air_date_sync")
	if err != nil {
		s.log.Error("Failed to get last sync time", "error", err)
		return
	}

	interval := time.Duration(s.config.SyncIntervalHours) * time.Hour
	if time.Since(lastSync) >= interval {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		if err := s.doSync(ctx, true); err != nil {
			s.log.Error("Scheduled sync failed", "error", err)
		}
	}
}

func (s *SyncService) doSync(ctx context.Context, setInProgress bool) error {
	if setInProgress {
		s.mu.Lock()
		s.syncStatus = "in_progress"
		s.mu.Unlock()
	}

	s.log.Info("Starting air date sync")

	// Get all shows in library
	shows, err := s.showRepo.List()
	if err != nil {
		s.setError(err)
		return err
	}

	if len(shows) == 0 {
		s.log.Info("No shows in library, skipping sync")
		s.setSuccess()
		return nil
	}

	// Process in batches
	for i := 0; i < len(shows); i += s.config.BatchSize {
		select {
		case <-ctx.Done():
			s.setError(ctx.Err())
			return ctx.Err()
		default:
		}

		end := i + s.config.BatchSize
		if end > len(shows) {
			end = len(shows)
		}
		batch := shows[i:end]

		for _, show := range batch {
			if err := s.syncShowAirDates(ctx, show); err != nil {
				s.log.Warn("Failed to sync show air dates",
					"show_id", show.ID,
					"tmdb_id", show.TMDBID,
					"title", show.Title,
					"error", err,
				)
				// Continue with other shows
			}
		}

		// Delay between batches to respect rate limits
		if i+s.config.BatchSize < len(shows) {
			time.Sleep(time.Duration(s.config.BatchDelayMs) * time.Millisecond)
		}
	}

	s.setSuccess()
	s.log.Info("Air date sync completed", "shows_processed", len(shows))
	return nil
}

func (s *SyncService) syncShowAirDates(ctx context.Context, show *library.Show) error {
	// Get show with seasons
	showWithSeasons, err := s.showRepo.GetWithSeasons(show.ID)
	if err != nil {
		return err
	}
	if showWithSeasons == nil {
		return nil // Deleted since the list was read
	}

	// For each season, specials (season 0) included, fetch episode air
	// dates from TMDB
	for _, season := range showWithSeasons.Seasons {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		tmdbSeason, err := s.seasons.Season(ctx, show.TMDBID, season.SeasonNumber, showWithSeasons.Status)
		if err != nil {
			s.log.Warn("Failed to fetch season from TMDB",
				"show_tmdb_id", show.TMDBID,
				"season", season.SeasonNumber,
				"error", err,
			)
			continue
		}

		// Update air dates for episodes
		for _, tmdbEp := range tmdbSeason.Episodes {
			if tmdbEp.AirDate != "" {
				if err := s.showRepo.UpdateEpisodeAirDate(
					ctx,
					season.ID,
					tmdbEp.EpisodeNumber,
					tmdbEp.AirDate,
				); err != nil {
					s.log.Warn("Failed to update episode air date",
						"season_id", season.ID,
						"episode", tmdbEp.EpisodeNumber,
						"error", err,
					)
				}
			}
		}
	}

	return nil
}

func (s *SyncService) setError(err error) {
	s.mu.Lock()
	s.syncStatus = "error"
	s.lastError = err
	s.mu.Unlock()
}

func (s *SyncService) setSuccess() {
	now := time.Now()
	if err := s.syncRepo.SetLastSyncTime("air_date_sync", now); err != nil {
		s.log.Error("Failed to update sync metadata", "error", err)
	}

	s.mu.Lock()
	s.lastSync = now
	s.syncStatus = "ok"
	s.lastError = nil
	s.mu.Unlock()
}
