package library

import (
	"database/sql"
	"fmt"
	"time"
)

// ShowRepository handles show, season, and episode database operations
type ShowRepository struct {
	db *DB
}

// NewShowRepository creates a new show repository
func NewShowRepository(db *DB) *ShowRepository {
	return &ShowRepository{db: db}
}

// Create adds a new show to the library
func (r *ShowRepository) Create(show *Show) error {
	err := r.db.QueryRow(
		`INSERT INTO shows (tmdb_id, title, year) VALUES ($1, $2, $3) RETURNING id, created_at`,
		show.TMDBID, show.Title, show.Year,
	).Scan(&show.ID, &show.CreatedAt)
	if err != nil {
		return fmt.Errorf("failed to create show: %w", err)
	}
	return nil
}

// GetByID retrieves a show by its ID
func (r *ShowRepository) GetByID(id int64) (*Show, error) {
	show := &Show{}
	err := r.db.QueryRow(
		`SELECT id, tmdb_id, title, year, created_at FROM shows WHERE id = $1`,
		id,
	).Scan(&show.ID, &show.TMDBID, &show.Title, &show.Year, &show.CreatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get show: %w", err)
	}

	return show, nil
}

// GetByTMDBID retrieves a show by its TMDB ID
func (r *ShowRepository) GetByTMDBID(tmdbID int) (*Show, error) {
	show := &Show{}
	err := r.db.QueryRow(
		`SELECT id, tmdb_id, title, year, created_at FROM shows WHERE tmdb_id = $1`,
		tmdbID,
	).Scan(&show.ID, &show.TMDBID, &show.Title, &show.Year, &show.CreatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get show: %w", err)
	}

	return show, nil
}

// List returns all shows in the library
func (r *ShowRepository) List() ([]*Show, error) {
	rows, err := r.db.Query(
		`SELECT id, tmdb_id, title, year, created_at FROM shows ORDER BY title`,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list shows: %w", err)
	}
	defer rows.Close()

	var shows []*Show
	for rows.Next() {
		show := &Show{}
		if err := rows.Scan(&show.ID, &show.TMDBID, &show.Title, &show.Year, &show.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan show: %w", err)
		}
		shows = append(shows, show)
	}

	return shows, rows.Err()
}

// GetWithSeasons retrieves a show with all its seasons
func (r *ShowRepository) GetWithSeasons(id int64) (*Show, error) {
	show, err := r.GetByID(id)
	if err != nil || show == nil {
		return show, err
	}

	seasons, err := r.GetSeasons(id)
	if err != nil {
		return nil, err
	}
	show.Seasons = seasons

	return show, nil
}

// GetWithSeasonsAndEpisodes retrieves a show with all seasons and episodes
func (r *ShowRepository) GetWithSeasonsAndEpisodes(id int64) (*Show, error) {
	show, err := r.GetWithSeasons(id)
	if err != nil || show == nil {
		return show, err
	}

	for i := range show.Seasons {
		episodes, err := r.GetEpisodes(show.Seasons[i].ID)
		if err != nil {
			return nil, err
		}
		show.Seasons[i].Episodes = episodes
	}

	return show, nil
}

// Delete removes a show from the library (cascades to seasons and episodes)
func (r *ShowRepository) Delete(id int64) error {
	result, err := r.db.Exec(`DELETE FROM shows WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("failed to delete show: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check delete result: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("show not found")
	}

	return nil
}

// Season operations

// CreateSeason adds a new season to a show
func (r *ShowRepository) CreateSeason(season *Season) error {
	err := r.db.QueryRow(
		`INSERT INTO seasons (show_id, season_number) VALUES ($1, $2)
		 ON CONFLICT(show_id, season_number) DO UPDATE SET season_number = EXCLUDED.season_number
		 RETURNING id`,
		season.ShowID, season.SeasonNumber,
	).Scan(&season.ID)
	if err != nil {
		return fmt.Errorf("failed to create season: %w", err)
	}
	return nil
}

// GetSeason retrieves a season by show ID and season number
func (r *ShowRepository) GetSeason(showID int64, seasonNumber int) (*Season, error) {
	season := &Season{}
	err := r.db.QueryRow(
		`SELECT id, show_id, season_number FROM seasons WHERE show_id = $1 AND season_number = $2`,
		showID, seasonNumber,
	).Scan(&season.ID, &season.ShowID, &season.SeasonNumber)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get season: %w", err)
	}

	return season, nil
}

// GetSeasonByID retrieves a season by its ID
func (r *ShowRepository) GetSeasonByID(id int64) (*Season, error) {
	season := &Season{}
	err := r.db.QueryRow(
		`SELECT id, show_id, season_number FROM seasons WHERE id = $1`,
		id,
	).Scan(&season.ID, &season.ShowID, &season.SeasonNumber)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get season: %w", err)
	}

	return season, nil
}

// GetSeasons retrieves all seasons for a show
func (r *ShowRepository) GetSeasons(showID int64) ([]Season, error) {
	rows, err := r.db.Query(
		`SELECT id, show_id, season_number FROM seasons WHERE show_id = $1 ORDER BY season_number`,
		showID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list seasons: %w", err)
	}
	defer rows.Close()

	var seasons []Season
	for rows.Next() {
		var season Season
		if err := rows.Scan(&season.ID, &season.ShowID, &season.SeasonNumber); err != nil {
			return nil, fmt.Errorf("failed to scan season: %w", err)
		}
		seasons = append(seasons, season)
	}

	return seasons, rows.Err()
}

// Episode operations

// CreateEpisode adds a new episode to a season
func (r *ShowRepository) CreateEpisode(episode *Episode) error {
	err := r.db.QueryRow(
		`INSERT INTO episodes (season_id, episode_number, name) VALUES ($1, $2, $3)
		 ON CONFLICT(season_id, episode_number) DO UPDATE SET name = EXCLUDED.name
		 RETURNING id`,
		episode.SeasonID, episode.EpisodeNumber, episode.Name,
	).Scan(&episode.ID)
	if err != nil {
		return fmt.Errorf("failed to create episode: %w", err)
	}
	return nil
}

// GetEpisode retrieves an episode by season ID and episode number
func (r *ShowRepository) GetEpisode(seasonID int64, episodeNumber int) (*Episode, error) {
	episode := &Episode{}
	var name sql.NullString
	err := r.db.QueryRow(
		`SELECT id, season_id, episode_number, name FROM episodes WHERE season_id = $1 AND episode_number = $2`,
		seasonID, episodeNumber,
	).Scan(&episode.ID, &episode.SeasonID, &episode.EpisodeNumber, &name)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get episode: %w", err)
	}
	episode.Name = name.String

	return episode, nil
}

// GetEpisodeByID retrieves an episode by its ID
func (r *ShowRepository) GetEpisodeByID(id int64) (*Episode, error) {
	episode := &Episode{}
	var name sql.NullString
	err := r.db.QueryRow(
		`SELECT id, season_id, episode_number, name FROM episodes WHERE id = $1`,
		id,
	).Scan(&episode.ID, &episode.SeasonID, &episode.EpisodeNumber, &name)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get episode: %w", err)
	}
	episode.Name = name.String

	return episode, nil
}

// GetEpisodes retrieves all episodes for a season
func (r *ShowRepository) GetEpisodes(seasonID int64) ([]Episode, error) {
	rows, err := r.db.Query(
		`SELECT id, season_id, episode_number, name FROM episodes WHERE season_id = $1 ORDER BY episode_number`,
		seasonID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list episodes: %w", err)
	}
	defer rows.Close()

	var episodes []Episode
	for rows.Next() {
		var episode Episode
		var name sql.NullString
		if err := rows.Scan(&episode.ID, &episode.SeasonID, &episode.EpisodeNumber, &name); err != nil {
			return nil, fmt.Errorf("failed to scan episode: %w", err)
		}
		episode.Name = name.String
		episodes = append(episodes, episode)
	}

	return episodes, rows.Err()
}

// GetShowsWithAssignedEpisodes returns all shows that have at least one episode with an active torrent assignment
func (r *ShowRepository) GetShowsWithAssignedEpisodes() ([]*Show, error) {
	// Get shows that have at least one episode with an assignment
	rows, err := r.db.Query(`
		SELECT DISTINCT s.id, s.tmdb_id, s.title, s.year, s.created_at
		FROM shows s
		INNER JOIN seasons sn ON sn.show_id = s.id
		INNER JOIN episodes e ON e.season_id = sn.id
		INNER JOIN torrent_assignments ta ON ta.item_type = 'episode' AND ta.item_id = e.id AND ta.is_active = TRUE
		ORDER BY s.title
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to list shows with assignments: %w", err)
	}
	defer rows.Close()

	var shows []*Show
	for rows.Next() {
		show := &Show{}
		if err := rows.Scan(&show.ID, &show.TMDBID, &show.Title, &show.Year, &show.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan show: %w", err)
		}

		// Load seasons with assigned episodes
		seasons, err := r.GetSeasonsWithAssignedEpisodes(show.ID)
		if err != nil {
			return nil, err
		}
		show.Seasons = seasons

		shows = append(shows, show)
	}

	return shows, rows.Err()
}

// GetSeasonsWithAssignedEpisodes returns all seasons for a show that have at least one episode with an active torrent assignment
func (r *ShowRepository) GetSeasonsWithAssignedEpisodes(showID int64) ([]Season, error) {
	rows, err := r.db.Query(`
		SELECT DISTINCT sn.id, sn.show_id, sn.season_number
		FROM seasons sn
		INNER JOIN episodes e ON e.season_id = sn.id
		INNER JOIN torrent_assignments ta ON ta.item_type = 'episode' AND ta.item_id = e.id AND ta.is_active = TRUE
		WHERE sn.show_id = $1
		ORDER BY sn.season_number
	`, showID)
	if err != nil {
		return nil, fmt.Errorf("failed to list seasons with assignments: %w", err)
	}
	defer rows.Close()

	var seasons []Season
	for rows.Next() {
		var season Season
		if err := rows.Scan(&season.ID, &season.ShowID, &season.SeasonNumber); err != nil {
			return nil, fmt.Errorf("failed to scan season: %w", err)
		}

		// Load episodes with assignments
		episodes, err := r.GetEpisodesWithAssignments(season.ID)
		if err != nil {
			return nil, err
		}
		season.Episodes = episodes

		seasons = append(seasons, season)
	}

	return seasons, rows.Err()
}

// GetEpisodesWithAssignments returns all episodes for a season that have active torrent assignments
func (r *ShowRepository) GetEpisodesWithAssignments(seasonID int64) ([]Episode, error) {
	rows, err := r.db.Query(`
		SELECT e.id, e.season_id, e.episode_number, e.name,
		       ta.id, ta.info_hash, ta.magnet_uri, ta.file_path, ta.file_size,
		       ta.resolution, ta.source, ta.created_at
		FROM episodes e
		INNER JOIN torrent_assignments ta ON ta.item_type = 'episode' AND ta.item_id = e.id AND ta.is_active = TRUE
		WHERE e.season_id = $1
		ORDER BY e.episode_number
	`, seasonID)
	if err != nil {
		return nil, fmt.Errorf("failed to list episodes with assignments: %w", err)
	}
	defer rows.Close()

	var episodes []Episode
	for rows.Next() {
		var episode Episode
		var name, resolution, source sql.NullString
		assignment := &TorrentAssignment{ItemType: ItemTypeEpisode}

		if err := rows.Scan(
			&episode.ID, &episode.SeasonID, &episode.EpisodeNumber, &name,
			&assignment.ID, &assignment.InfoHash, &assignment.MagnetURI,
			&assignment.FilePath, &assignment.FileSize,
			&resolution, &source, &assignment.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan episode: %w", err)
		}

		episode.Name = name.String
		assignment.ItemID = episode.ID
		assignment.Resolution = resolution.String
		assignment.Source = source.String
		assignment.IsActive = true
		episode.Assignment = assignment

		episodes = append(episodes, episode)
	}

	return episodes, rows.Err()
}

// UpdateEpisodeAirDate sets the air date for a specific episode
func (r *ShowRepository) UpdateEpisodeAirDate(seasonID int64, episodeNumber int, airDate string) error {
	_, err := r.db.Exec(
		`UPDATE episodes SET air_date = $1 WHERE season_id = $2 AND episode_number = $3`,
		airDate, seasonID, episodeNumber,
	)
	if err != nil {
		return fmt.Errorf("failed to update episode air date: %w", err)
	}
	return nil
}

// GetRecentlyAiredEpisodes returns episodes that aired within the lookback period
func (r *ShowRepository) GetRecentlyAiredEpisodes(lookbackDays int) ([]RecentlyAiredEpisode, error) {
	cutoffDate := time.Now().AddDate(0, 0, -lookbackDays).Format("2006-01-02")
	today := time.Now().Format("2006-01-02")

	rows, err := r.db.Query(`
		SELECT
			s.id, s.tmdb_id, s.title, s.year,
			sn.season_number,
			e.id, e.episode_number, e.name, e.air_date,
			CASE WHEN ta.id IS NOT NULL THEN 1 ELSE 0 END as has_assignment
		FROM episodes e
		INNER JOIN seasons sn ON sn.id = e.season_id
		INNER JOIN shows s ON s.id = sn.show_id
		LEFT JOIN torrent_assignments ta ON ta.item_type = 'episode' AND ta.item_id = e.id AND ta.is_active = TRUE
		WHERE e.air_date IS NOT NULL
		  AND e.air_date >= $1
		  AND e.air_date <= $2
		ORDER BY e.air_date DESC, s.title ASC, sn.season_number ASC, e.episode_number ASC
	`, cutoffDate, today)
	if err != nil {
		return nil, fmt.Errorf("failed to get recently aired episodes: %w", err)
	}
	defer rows.Close()

	var episodes []RecentlyAiredEpisode
	for rows.Next() {
		var ep RecentlyAiredEpisode
		var name sql.NullString
		var hasAssignment int

		err := rows.Scan(
			&ep.ShowID, &ep.ShowTMDBID, &ep.ShowTitle, &ep.ShowYear,
			&ep.SeasonNumber,
			&ep.EpisodeID, &ep.EpisodeNumber, &name, &ep.AirDate,
			&hasAssignment,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan recently aired episode: %w", err)
		}

		ep.EpisodeName = name.String
		ep.HasAssignment = hasAssignment == 1
		episodes = append(episodes, ep)
	}

	return episodes, rows.Err()
}

// EpisodeContext contains the full context needed for VFS tree operations
type EpisodeContext struct {
	ShowTitle     string
	ShowYear      int
	SeasonNumber  int
	EpisodeNumber int
}

// GetEpisodeContext returns the full context for an episode in a single query
func (r *ShowRepository) GetEpisodeContext(episodeID int64) (*EpisodeContext, error) {
	ctx := &EpisodeContext{}
	err := r.db.QueryRow(`
		SELECT s.title, s.year, sn.season_number, e.episode_number
		FROM episodes e
		INNER JOIN seasons sn ON sn.id = e.season_id
		INNER JOIN shows s ON s.id = sn.show_id
		WHERE e.id = $1
	`, episodeID).Scan(&ctx.ShowTitle, &ctx.ShowYear, &ctx.SeasonNumber, &ctx.EpisodeNumber)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get episode context: %w", err)
	}

	return ctx, nil
}
