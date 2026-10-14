package library

import (
	"context"
	"database/sql"
	"errors"
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

// showColumns selects a show row (alias s) in the order scanShow reads it.
const showColumns = `s.id, s.tmdb_id, s.title, s.year, s.created_at,
	COALESCE(s.imdb_id, ''), COALESCE(s.tvdb_id, 0), COALESCE(s.original_name, ''),
	COALESCE(s.original_language, ''), COALESCE(to_jsonb(s.origin_country), '[]'::jsonb), s.is_anime,
	COALESCE(s.poster_path, ''), COALESCE(s.backdrop_path, ''), COALESCE(s.status, ''),
	COALESCE(s.genres, '[]'::jsonb), s.metadata_updated_at,
	s.episode_ordering, COALESCE(s.episode_group_id, ''), s.include_specials, s.classifier_mode`

// scanShow reads showColumns.
func scanShow(sc scanner) (*Show, error) {
	show := &Show{}
	var countries, genres []byte
	var updatedAt sql.NullTime
	err := sc.Scan(
		&show.ID, &show.TMDBID, &show.Title, &show.Year, &show.CreatedAt,
		&show.IMDbID, &show.TVDBID, &show.OriginalName,
		&show.OriginalLanguage, &countries, &show.IsAnime,
		&show.PosterPath, &show.BackdropPath, &show.Status,
		&genres, &updatedAt,
		&show.EpisodeOrdering, &show.EpisodeGroupID, &show.IncludeSpecials, &show.ClassifierMode,
	)
	if err != nil {
		return nil, err
	}
	if show.OriginCountry, err = decodeStrings(countries); err != nil {
		return nil, fmt.Errorf("show %d origin_country: %w", show.ID, err)
	}
	if show.Genres, err = decodeStrings(genres); err != nil {
		return nil, fmt.Errorf("show %d genres: %w", show.ID, err)
	}
	if updatedAt.Valid {
		show.MetadataUpdatedAt = &updatedAt.Time
	}
	return show, nil
}

// episodeColumns selects an episode row (alias e) in the order
// episodeScanDest reads it. Every column is non-null except absolute_number,
// so it also works on the nullable side of a LEFT JOIN.
const episodeColumns = `e.id, COALESCE(e.season_id, 0), COALESCE(e.episode_number, 0), COALESCE(e.name, ''),
	COALESCE(e.air_date, ''), COALESCE(e.tmdb_episode_id, 0), e.absolute_number,
	COALESCE(e.episode_type, ''), COALESCE(e.runtime, 0), COALESCE(e.still_path, ''), COALESCE(e.overview, '')`

// episodeScanDest returns scan destinations for episodeColumns. Call
// finish after Scan to copy the nullable values into ep.
func episodeScanDest(ep *Episode) (dest []any, finish func() (present bool)) {
	var id, absolute sql.NullInt64
	dest = []any{
		&id, &ep.SeasonID, &ep.EpisodeNumber, &ep.Name,
		&ep.AirDate, &ep.TMDBEpisodeID, &absolute,
		&ep.EpisodeType, &ep.Runtime, &ep.StillPath, &ep.Overview,
	}
	return dest, func() bool {
		ep.ID = id.Int64
		if absolute.Valid {
			n := int(absolute.Int64)
			ep.AbsoluteNumber = &n
		}
		return id.Valid
	}
}

func scanEpisode(sc scanner) (*Episode, error) {
	ep := &Episode{}
	dest, finish := episodeScanDest(ep)
	if err := sc.Scan(dest...); err != nil {
		return nil, err
	}
	finish()
	return ep, nil
}

// Create adds a new show to the library (title, year and TMDB ID only).
func (r *ShowRepository) Create(show *Show) error {
	err := r.db.QueryRow(
		`INSERT INTO shows (tmdb_id, title, year) VALUES ($1, $2, $3)
		 RETURNING id, created_at, episode_ordering, include_specials, classifier_mode`,
		show.TMDBID, show.Title, show.Year,
	).Scan(&show.ID, &show.CreatedAt, &show.EpisodeOrdering, &show.IncludeSpecials, &show.ClassifierMode)
	if err != nil {
		return fmt.Errorf("failed to create show: %w", err)
	}
	return nil
}

// CreateIfAbsent inserts show with its metadata. EpisodeOrdering is written
// when set; the other settings take their column defaults (specials on,
// classifier auto) and are copied back into show. metadata_updated_at stays
// NULL until the caller has synced the seasons (MarkShowMetadataUpdated).
//
// If a show with the same TMDB ID already exists it is left alone, show is
// overwritten with the stored row, and created is false.
func (r *ShowRepository) CreateIfAbsent(ctx context.Context, show *Show) (created bool, err error) {
	if show.EpisodeOrdering != "" && !show.EpisodeOrdering.Valid() {
		return false, fmt.Errorf("%w: episode ordering %q", ErrInvalidShowSettings, show.EpisodeOrdering)
	}
	countries, err := encodeStrings(show.OriginCountry)
	if err != nil {
		return false, err
	}
	genres, err := encodeStrings(show.Genres)
	if err != nil {
		return false, err
	}

	err = r.db.QueryRowContext(ctx, `
		INSERT INTO shows (tmdb_id, title, year, imdb_id, tvdb_id, original_name, original_language,
		                   origin_country, is_anime, poster_path, backdrop_path, status, genres, episode_ordering)
		VALUES ($1, $2, $3, NULLIF($4, ''), NULLIF($5, 0), NULLIF($6, ''), NULLIF($7, ''),
		        ARRAY(SELECT jsonb_array_elements_text($8::jsonb)), $9, NULLIF($10, ''), NULLIF($11, ''),
		        NULLIF($12, ''), $13::jsonb, COALESCE(NULLIF($14, ''), 'aired'))
		ON CONFLICT (tmdb_id) DO NOTHING
		RETURNING id, created_at, episode_ordering, include_specials, classifier_mode
	`, show.TMDBID, show.Title, show.Year, show.IMDbID, show.TVDBID, show.OriginalName, show.OriginalLanguage,
		countries, show.IsAnime, show.PosterPath, show.BackdropPath, show.Status, genres, string(show.EpisodeOrdering),
	).Scan(&show.ID, &show.CreatedAt, &show.EpisodeOrdering, &show.IncludeSpecials, &show.ClassifierMode)
	if err == nil {
		return true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return false, fmt.Errorf("failed to create show: %w", err)
	}

	existing, err := r.GetByTMDBID(show.TMDBID)
	if err != nil {
		return false, err
	}
	if existing == nil {
		return false, fmt.Errorf("show %d conflicted on insert but is gone", show.TMDBID)
	}
	*show = *existing
	return false, nil
}

// GetByID retrieves a show by its ID
func (r *ShowRepository) GetByID(id int64) (*Show, error) {
	return r.getShow(context.Background(), `s.id = $1`, id)
}

// GetByTMDBID retrieves a show by its TMDB ID
func (r *ShowRepository) GetByTMDBID(tmdbID int) (*Show, error) {
	return r.getShow(context.Background(), `s.tmdb_id = $1`, tmdbID)
}

func (r *ShowRepository) getShow(ctx context.Context, where string, arg any) (*Show, error) {
	show, err := scanShow(r.db.QueryRowContext(ctx, `SELECT `+showColumns+` FROM shows s WHERE `+where, arg))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get show: %w", err)
	}
	return show, nil
}

// List returns all shows in the library
func (r *ShowRepository) List() ([]*Show, error) {
	rows, err := r.db.Query(`SELECT ` + showColumns + ` FROM shows s ORDER BY s.title`)
	if err != nil {
		return nil, fmt.Errorf("failed to list shows: %w", err)
	}
	defer rows.Close()

	var shows []*Show
	for rows.Next() {
		show, err := scanShow(rows)
		if err != nil {
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

// GetWithSeasonsAndEpisodes retrieves a show with every season (season 0
// included), every episode and each episode's active assignment, in two
// queries however long the show is.
func (r *ShowRepository) GetWithSeasonsAndEpisodes(id int64) (*Show, error) {
	return r.GetWithSeasonsAndEpisodesContext(context.Background(), id)
}

// GetWithSeasonsAndEpisodesContext is GetWithSeasonsAndEpisodes with a
// context.
func (r *ShowRepository) GetWithSeasonsAndEpisodesContext(ctx context.Context, id int64) (*Show, error) {
	show, err := r.getShow(ctx, `s.id = $1`, id)
	if err != nil || show == nil {
		return show, err
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT sn.id, sn.season_number, COALESCE(sn.name, ''), COALESCE(sn.poster_path, ''),
		       `+episodeColumns+`,
		       ta.id, ta.info_hash, ta.magnet_uri, ta.file_path, ta.file_size,
		       ta.resolution, ta.source, ta.created_at
		FROM seasons sn
		LEFT JOIN episodes e ON e.season_id = sn.id
		LEFT JOIN LATERAL (
			SELECT t.id, t.info_hash, t.magnet_uri, t.file_path, t.file_size, t.resolution, t.source, t.created_at
			FROM torrent_assignments t
			WHERE t.item_type = 'episode' AND t.item_id = e.id AND t.is_active
			ORDER BY t.created_at DESC, t.id DESC
			LIMIT 1
		) ta ON TRUE
		WHERE sn.show_id = $1
		ORDER BY sn.season_number, e.episode_number
	`, id)
	if err != nil {
		return nil, fmt.Errorf("failed to load seasons and episodes: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var season Season
		var ep Episode
		var taID, taSize sql.NullInt64
		var taHash, taMagnet, taPath, taResolution, taSource sql.NullString
		var taCreated sql.NullTime

		epDest, finish := episodeScanDest(&ep)
		dest := append([]any{&season.ID, &season.SeasonNumber, &season.Name, &season.PosterPath}, epDest...)
		dest = append(dest, &taID, &taHash, &taMagnet, &taPath, &taSize, &taResolution, &taSource, &taCreated)
		if err := rows.Scan(dest...); err != nil {
			return nil, fmt.Errorf("failed to scan season row: %w", err)
		}

		// Rows arrive grouped by season; start a new one when it changes.
		if n := len(show.Seasons); n == 0 || show.Seasons[n-1].ID != season.ID {
			season.ShowID = show.ID
			show.Seasons = append(show.Seasons, season)
		}
		if !finish() {
			continue // season without episodes
		}
		if taID.Valid {
			ep.Assignment = &TorrentAssignment{
				ID:         taID.Int64,
				ItemType:   ItemTypeEpisode,
				ItemID:     ep.ID,
				InfoHash:   taHash.String,
				MagnetURI:  taMagnet.String,
				FilePath:   taPath.String,
				FileSize:   taSize.Int64,
				Resolution: taResolution.String,
				Source:     taSource.String,
				IsActive:   true,
				CreatedAt:  taCreated.Time,
			}
		}
		last := &show.Seasons[len(show.Seasons)-1]
		last.Episodes = append(last.Episodes, ep)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read seasons and episodes: %w", err)
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

const seasonColumns = `sn.id, sn.show_id, sn.season_number, COALESCE(sn.name, ''), COALESCE(sn.poster_path, '')`

func scanSeason(sc scanner) (*Season, error) {
	season := &Season{}
	if err := sc.Scan(&season.ID, &season.ShowID, &season.SeasonNumber, &season.Name, &season.PosterPath); err != nil {
		return nil, err
	}
	return season, nil
}

// GetSeason retrieves a season by show ID and season number
func (r *ShowRepository) GetSeason(showID int64, seasonNumber int) (*Season, error) {
	season, err := scanSeason(r.db.QueryRow(
		`SELECT `+seasonColumns+` FROM seasons sn WHERE sn.show_id = $1 AND sn.season_number = $2`,
		showID, seasonNumber,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get season: %w", err)
	}
	return season, nil
}

// GetSeasonByID retrieves a season by its ID
func (r *ShowRepository) GetSeasonByID(id int64) (*Season, error) {
	season, err := scanSeason(r.db.QueryRow(`SELECT `+seasonColumns+` FROM seasons sn WHERE sn.id = $1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get season: %w", err)
	}
	return season, nil
}

// GetSeasons retrieves all seasons for a show, season 0 (specials) first.
func (r *ShowRepository) GetSeasons(showID int64) ([]Season, error) {
	rows, err := r.db.Query(
		`SELECT `+seasonColumns+` FROM seasons sn WHERE sn.show_id = $1 ORDER BY sn.season_number`,
		showID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list seasons: %w", err)
	}
	defer rows.Close()

	var seasons []Season
	for rows.Next() {
		season, err := scanSeason(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan season: %w", err)
		}
		seasons = append(seasons, *season)
	}

	return seasons, rows.Err()
}

// Episode operations

// GetEpisode retrieves an episode by season ID and episode number
func (r *ShowRepository) GetEpisode(seasonID int64, episodeNumber int) (*Episode, error) {
	episode, err := scanEpisode(r.db.QueryRow(
		`SELECT `+episodeColumns+` FROM episodes e WHERE e.season_id = $1 AND e.episode_number = $2`,
		seasonID, episodeNumber,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get episode: %w", err)
	}
	return episode, nil
}

// GetEpisodeByID retrieves an episode by its ID
func (r *ShowRepository) GetEpisodeByID(id int64) (*Episode, error) {
	episode, err := scanEpisode(r.db.QueryRow(`SELECT `+episodeColumns+` FROM episodes e WHERE e.id = $1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get episode: %w", err)
	}
	return episode, nil
}

// GetEpisodes retrieves all episodes for a season
func (r *ShowRepository) GetEpisodes(seasonID int64) ([]Episode, error) {
	rows, err := r.db.Query(
		`SELECT `+episodeColumns+` FROM episodes e WHERE e.season_id = $1 ORDER BY e.episode_number`,
		seasonID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list episodes: %w", err)
	}
	defer rows.Close()

	var episodes []Episode
	for rows.Next() {
		episode, err := scanEpisode(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan episode: %w", err)
		}
		episodes = append(episodes, *episode)
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

// UpdateEpisodeAirDate sets the air date for a specific episode. An empty
// airDate stores NULL.
func (r *ShowRepository) UpdateEpisodeAirDate(ctx context.Context, seasonID int64, episodeNumber int, airDate string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE episodes SET air_date = NULLIF($1, '') WHERE season_id = $2 AND episode_number = $3`,
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
	ShowTMDBID    int
	SeasonNumber  int
	EpisodeNumber int
}

// GetEpisodeContext returns the full context for an episode in a single query
func (r *ShowRepository) GetEpisodeContext(episodeID int64) (*EpisodeContext, error) {
	ctx := &EpisodeContext{}
	err := r.db.QueryRow(`
		SELECT s.title, s.year, s.tmdb_id, sn.season_number, e.episode_number
		FROM episodes e
		INNER JOIN seasons sn ON sn.id = e.season_id
		INNER JOIN shows s ON s.id = sn.show_id
		WHERE e.id = $1
	`, episodeID).Scan(&ctx.ShowTitle, &ctx.ShowYear, &ctx.ShowTMDBID, &ctx.SeasonNumber, &ctx.EpisodeNumber)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get episode context: %w", err)
	}

	return ctx, nil
}
