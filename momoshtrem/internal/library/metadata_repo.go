package library

import (
	"context"
	"fmt"
	"time"
)

// MetadataRepository writes TMDB metadata and per-show settings. It never
// changes a movie's or show's title or year: those name the WebDAV folders.
type MetadataRepository struct {
	db *DB
}

// NewMetadataRepository creates a new metadata repository.
func NewMetadataRepository(db *DB) *MetadataRepository {
	return &MetadataRepository{db: db}
}

// MovieMetadata is the TMDB metadata stored on a movie row.
type MovieMetadata struct {
	IMDbID       string
	PosterPath   string
	BackdropPath string
	Runtime      int
	Genres       []string
}

// UpdateMovieMetadata stores m on the movie and stamps metadata_updated_at.
func (r *MetadataRepository) UpdateMovieMetadata(ctx context.Context, movieID int64, m MovieMetadata) error {
	genres, err := encodeStrings(m.Genres)
	if err != nil {
		return err
	}
	res, err := r.db.ExecContext(ctx, `
		UPDATE movies SET
			imdb_id = NULLIF($2, ''), poster_path = NULLIF($3, ''), backdrop_path = NULLIF($4, ''),
			runtime = NULLIF($5, 0), genres = $6::jsonb, metadata_updated_at = NOW()
		WHERE id = $1
	`, movieID, m.IMDbID, m.PosterPath, m.BackdropPath, m.Runtime, genres)
	if err != nil {
		return fmt.Errorf("failed to update movie metadata: %w", err)
	}
	return expectOneRow(res, ErrMovieNotFound)
}

// ShowMetadata is the TMDB metadata stored on a show row.
type ShowMetadata struct {
	IMDbID           string
	TVDBID           int
	OriginalName     string
	OriginalLanguage string
	OriginCountry    []string
	IsAnime          bool
	PosterPath       string
	BackdropPath     string
	Status           string
	Genres           []string
}

// UpdateShowMetadata stores m on the show. It doesn't stamp
// metadata_updated_at; call MarkShowMetadataUpdated once the seasons are
// synced too.
func (r *MetadataRepository) UpdateShowMetadata(ctx context.Context, showID int64, m ShowMetadata) error {
	countries, err := encodeStrings(m.OriginCountry)
	if err != nil {
		return err
	}
	genres, err := encodeStrings(m.Genres)
	if err != nil {
		return err
	}
	res, err := r.db.ExecContext(ctx, `
		UPDATE shows SET
			imdb_id = NULLIF($2, ''), tvdb_id = NULLIF($3, 0), original_name = NULLIF($4, ''),
			original_language = NULLIF($5, ''), origin_country = ARRAY(SELECT jsonb_array_elements_text($6::jsonb)),
			is_anime = $7, poster_path = NULLIF($8, ''), backdrop_path = NULLIF($9, ''),
			status = NULLIF($10, ''), genres = $11::jsonb
		WHERE id = $1
	`, showID, m.IMDbID, m.TVDBID, m.OriginalName, m.OriginalLanguage, countries,
		m.IsAnime, m.PosterPath, m.BackdropPath, m.Status, genres)
	if err != nil {
		return fmt.Errorf("failed to update show metadata: %w", err)
	}
	return expectOneRow(res, ErrShowNotFound)
}

// MarkShowMetadataUpdated stamps metadata_updated_at, which tells the
// backfill the show is done.
func (r *MetadataRepository) MarkShowMetadataUpdated(ctx context.Context, showID int64) error {
	res, err := r.db.ExecContext(ctx, `UPDATE shows SET metadata_updated_at = NOW() WHERE id = $1`, showID)
	if err != nil {
		return fmt.Errorf("failed to stamp show metadata: %w", err)
	}
	return expectOneRow(res, ErrShowNotFound)
}

// UpsertSeason creates the season (show ID and number) or updates its name
// and poster. It sets season.ID; created reports whether the row is new.
func (r *MetadataRepository) UpsertSeason(ctx context.Context, season *Season) (created bool, err error) {
	err = r.db.QueryRowContext(ctx, `
		INSERT INTO seasons AS sn (show_id, season_number, name, poster_path)
		VALUES ($1, $2, NULLIF($3, ''), NULLIF($4, ''))
		ON CONFLICT (show_id, season_number) DO UPDATE SET
			name = EXCLUDED.name, poster_path = EXCLUDED.poster_path
		RETURNING sn.id, (sn.xmax = 0)
	`, season.ShowID, season.SeasonNumber, season.Name, season.PosterPath).Scan(&season.ID, &created)
	if err != nil {
		return false, fmt.Errorf("failed to upsert season %d: %w", season.SeasonNumber, err)
	}
	return created, nil
}

// EpisodeMetadata is one TMDB episode to store.
type EpisodeMetadata struct {
	EpisodeNumber int
	Name          string
	AirDate       string // YYYY-MM-DD; "" stores NULL
	TMDBEpisodeID int
	EpisodeType   string
	Runtime       int
	StillPath     string
	Overview      string
}

// UpsertEpisodesResult summarises an UpsertEpisodes call.
type UpsertEpisodesResult struct {
	Added   int // New episode rows
	Renamed int // Existing episodes whose name changed
	// AssignedRenamed is true when a renamed episode has an active
	// assignment, so its WebDAV file name changed and the tree is stale.
	AssignedRenamed bool
}

// UpsertEpisodes creates or updates a season's episodes in one statement.
// With keepNames, existing episodes keep their stored name (new ones still
// get TMDB's), so WebDAV paths don't move. Everything else is overwritten
// with TMDB's values. Duplicate episode numbers in the input keep the first.
func (r *MetadataRepository) UpsertEpisodes(ctx context.Context, seasonID int64, episodes []EpisodeMetadata, keepNames bool) (UpsertEpisodesResult, error) {
	var result UpsertEpisodesResult
	if len(episodes) == 0 {
		return result, nil
	}

	n := len(episodes)
	numbers := make([]int32, 0, n)
	names := make([]string, 0, n)
	airDates := make([]string, 0, n)
	tmdbIDs := make([]int32, 0, n)
	types := make([]string, 0, n)
	runtimes := make([]int32, 0, n)
	stills := make([]string, 0, n)
	overviews := make([]string, 0, n)
	seen := make(map[int]bool, n)
	for _, ep := range episodes {
		if seen[ep.EpisodeNumber] {
			continue // ON CONFLICT can't touch a row twice in one statement
		}
		seen[ep.EpisodeNumber] = true
		numbers = append(numbers, int32(ep.EpisodeNumber))
		names = append(names, ep.Name)
		airDates = append(airDates, ep.AirDate)
		tmdbIDs = append(tmdbIDs, int32(ep.TMDBEpisodeID))
		types = append(types, ep.EpisodeType)
		runtimes = append(runtimes, int32(ep.Runtime))
		stills = append(stills, ep.StillPath)
		overviews = append(overviews, ep.Overview)
	}

	// prev reads the rows as they were before the upsert: every part of a
	// WITH statement sees the same snapshot.
	err := r.db.QueryRowContext(ctx, `
		WITH input AS (
			SELECT * FROM unnest($2::int[], $3::text[], $4::text[], $5::int[], $6::text[], $7::int[], $8::text[], $9::text[])
				AS t(episode_number, name, air_date, tmdb_episode_id, episode_type, runtime, still_path, overview)
		),
		prev AS (
			SELECT id, COALESCE(name, '') AS name FROM episodes WHERE season_id = $1
		),
		up AS (
			INSERT INTO episodes AS e (season_id, episode_number, name, air_date, tmdb_episode_id,
			                           episode_type, runtime, still_path, overview)
			SELECT $1, episode_number, NULLIF(name, ''), NULLIF(air_date, ''), NULLIF(tmdb_episode_id, 0),
			       NULLIF(episode_type, ''), NULLIF(runtime, 0), NULLIF(still_path, ''), NULLIF(overview, '')
			FROM input
			ON CONFLICT (season_id, episode_number) DO UPDATE SET
				name            = CASE WHEN $10::boolean THEN e.name ELSE EXCLUDED.name END,
				air_date        = EXCLUDED.air_date,
				tmdb_episode_id = EXCLUDED.tmdb_episode_id,
				episode_type    = EXCLUDED.episode_type,
				runtime         = EXCLUDED.runtime,
				still_path      = EXCLUDED.still_path,
				overview        = EXCLUDED.overview
			RETURNING e.id, COALESCE(e.name, '') AS name, (e.xmax = 0) AS inserted
		),
		changes AS (
			SELECT up.id, up.inserted, (NOT up.inserted AND prev.name <> up.name) AS renamed
			FROM up LEFT JOIN prev ON prev.id = up.id
		)
		SELECT
			COUNT(*) FILTER (WHERE inserted),
			COUNT(*) FILTER (WHERE renamed),
			EXISTS (
				SELECT 1 FROM changes c
				JOIN torrent_assignments ta ON ta.item_type = 'episode' AND ta.item_id = c.id AND ta.is_active
				WHERE c.renamed
			)
		FROM changes
	`, seasonID, numbers, names, airDates, tmdbIDs, types, runtimes, stills, overviews, keepNames,
	).Scan(&result.Added, &result.Renamed, &result.AssignedRenamed)
	if err != nil {
		return UpsertEpisodesResult{}, fmt.Errorf("failed to upsert episodes: %w", err)
	}
	return result, nil
}

// AbsoluteNumber is the absolute number of one episode; 0 means none.
type AbsoluteNumber struct {
	EpisodeID int64
	Number    int
}

// SetAbsoluteNumbers replaces the absolute numbers of every episode of the
// show in one statement: listed episodes get their number, unlisted ones
// (and Number 0) get NULL. It returns how many rows changed.
func (r *MetadataRepository) SetAbsoluteNumbers(ctx context.Context, showID int64, numbers []AbsoluteNumber) (int64, error) {
	ids := make([]int64, len(numbers))
	values := make([]int32, len(numbers))
	for i, n := range numbers {
		ids[i] = n.EpisodeID
		values[i] = int32(n.Number)
	}

	res, err := r.db.ExecContext(ctx, `
		UPDATE episodes e SET absolute_number = u.n
		FROM (
			SELECT e2.id, NULLIF(x.n, 0) AS n
			FROM episodes e2
			JOIN seasons sn ON sn.id = e2.season_id
			LEFT JOIN unnest($2::bigint[], $3::int[]) AS x(id, n) ON x.id = e2.id
			WHERE sn.show_id = $1
		) u
		WHERE e.id = u.id AND e.absolute_number IS DISTINCT FROM u.n
	`, showID, ids, values)
	if err != nil {
		return 0, fmt.Errorf("failed to set absolute numbers: %w", err)
	}
	changed, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to count absolute number changes: %w", err)
	}
	return changed, nil
}

// ShowSettings are the user-editable per-show settings.
type ShowSettings struct {
	EpisodeOrdering EpisodeOrdering
	EpisodeGroupID  string // "" stores NULL
	IncludeSpecials bool
	ClassifierMode  ClassifierMode
}

// Settings returns the show's current settings.
func (s *Show) Settings() ShowSettings {
	return ShowSettings{
		EpisodeOrdering: s.EpisodeOrdering,
		EpisodeGroupID:  s.EpisodeGroupID,
		IncludeSpecials: s.IncludeSpecials,
		ClassifierMode:  s.ClassifierMode,
	}
}

// UpdateShowSettings replaces the show's settings. Unknown orderings or
// classifier modes return ErrInvalidShowSettings. It doesn't check that the
// episode group belongs to the show; that needs TMDB (metadata.Syncer does).
func (r *MetadataRepository) UpdateShowSettings(ctx context.Context, showID int64, s ShowSettings) error {
	if !s.EpisodeOrdering.Valid() {
		return fmt.Errorf("%w: episode ordering %q", ErrInvalidShowSettings, s.EpisodeOrdering)
	}
	if !s.ClassifierMode.Valid() {
		return fmt.Errorf("%w: classifier mode %q", ErrInvalidShowSettings, s.ClassifierMode)
	}
	res, err := r.db.ExecContext(ctx, `
		UPDATE shows SET episode_ordering = $2, episode_group_id = NULLIF($3, ''),
		                 include_specials = $4, classifier_mode = $5
		WHERE id = $1
	`, showID, string(s.EpisodeOrdering), s.EpisodeGroupID, s.IncludeSpecials, string(s.ClassifierMode))
	if err != nil {
		return fmt.Errorf("failed to update show settings: %w", err)
	}
	return expectOneRow(res, ErrShowNotFound)
}

// MediaKind selects movies or shows.
type MediaKind string

const (
	MediaMovie MediaKind = "movie"
	MediaShow  MediaKind = "show"
)

// BackfillItem is a movie or show the metadata backfill should process.
type BackfillItem struct {
	ID                int64
	TMDBID            int
	Title             string
	Year              int
	MetadataUpdatedAt *time.Time
}

// ListForBackfill returns the movies or shows that have never had a metadata
// sync (all of them with force), oldest first. limit 0 means no limit.
func (r *MetadataRepository) ListForBackfill(ctx context.Context, kind MediaKind, force bool, limit int) ([]BackfillItem, error) {
	var table string
	switch kind {
	case MediaMovie:
		table = "movies"
	case MediaShow:
		table = "shows"
	default:
		return nil, fmt.Errorf("unknown media kind %q", kind)
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT id, tmdb_id, title, year, metadata_updated_at
		FROM `+table+`
		WHERE $1::boolean OR metadata_updated_at IS NULL
		ORDER BY id
		LIMIT NULLIF($2, 0)
	`, force, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list %s for backfill: %w", table, err)
	}
	defer rows.Close()

	var items []BackfillItem
	for rows.Next() {
		var item BackfillItem
		if err := rows.Scan(&item.ID, &item.TMDBID, &item.Title, &item.Year, &item.MetadataUpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan backfill item: %w", err)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// rowsAffecter is the part of sql.Result expectOneRow needs.
type rowsAffecter interface {
	RowsAffected() (int64, error)
}

// expectOneRow returns notFound when an UPDATE matched no row.
func expectOneRow(res rowsAffecter, notFound error) error {
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check affected rows: %w", err)
	}
	if n == 0 {
		return notFound
	}
	return nil
}
