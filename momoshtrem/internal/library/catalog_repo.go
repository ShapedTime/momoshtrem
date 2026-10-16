package library

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// CatalogRepository reads what the title pages and GET /library need, in a
// fixed number of queries however big the library or show (PLAN §6 query
// budgets).
type CatalogRepository struct {
	db *DB
}

// NewCatalogRepository creates a new catalog repository.
func NewCatalogRepository(db *DB) *CatalogRepository {
	return &CatalogRepository{db: db}
}

// activeAssignmentLateral joins an item's newest active assignment as "ta".
// itemType is a SQL literal; itemID the outer column.
func activeAssignmentLateral(itemType, itemID string) string {
	return `LEFT JOIN LATERAL (
			SELECT t.id, t.kind, t.info_hash, t.magnet_uri, t.file_path, t.file_size, t.resolution, t.source,
			       t.confidence, t.match_source, t.stream_ref, t.created_at
			FROM torrent_assignments t
			WHERE t.item_type = '` + itemType + `' AND t.item_id = ` + itemID + ` AND t.is_active
			ORDER BY t.created_at DESC, t.id DESC
			LIMIT 1
		) ta ON TRUE`
}

// assignmentDest scans assignmentColumns: the ta.* columns of
// activeAssignmentLateral, or of an active torrent_assignments row joined
// as ta. Call finish after Scan: it returns the assignment, or nil when
// there is none. Every column is nullable: an HDRezka row has no hash or
// magnet, and the lateral none at all without a row.
func assignmentDest(itemType ItemType) (dest []any, finish func(itemID int64) *TorrentAssignment) {
	var id, size sql.NullInt64
	var kind, hash, magnet, path, resolution, source, confidence, matchSource sql.NullString
	var stream nullStreamRef
	var created sql.NullTime
	dest = []any{&id, &kind, &hash, &magnet, &path, &size, &resolution, &source, &confidence, &matchSource, &stream, &created}
	return dest, func(itemID int64) *TorrentAssignment {
		if !id.Valid {
			return nil
		}
		return &TorrentAssignment{
			ID: id.Int64, ItemType: itemType, ItemID: itemID, Kind: AssignmentKind(kind.String),
			InfoHash: hash.String, MagnetURI: magnet.String, FilePath: path.String, FileSize: size.Int64,
			Resolution: resolution.String, Source: source.String,
			Confidence: confidence.String, MatchSource: matchSource.String, Stream: stream.Ref,
			IsActive: true, CreatedAt: created.Time,
		}
	}
}

const assignmentColumns = `ta.id, ta.kind, ta.info_hash, ta.magnet_uri, ta.file_path, ta.file_size, ta.resolution, ta.source,
	ta.confidence, ta.match_source, ta.stream_ref, ta.created_at`

// MovieByTMDBID returns the movie with that TMDB ID and its active
// assignment (Movie.Assignment, nil when none), or nil when the movie isn't
// in the library. One query.
func (r *CatalogRepository) MovieByTMDBID(ctx context.Context, tmdbID int) (*Movie, error) {
	dest, finish := assignmentDest(ItemTypeMovie)
	movie, err := scanMovie(r.db.QueryRowContext(ctx, `
		SELECT `+movieColumns+`, `+assignmentColumns+`
		FROM movies m
		`+activeAssignmentLateral("movie", "m.id")+`
		WHERE m.tmdb_id = $1
	`, tmdbID), dest...)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get movie: %w", err)
	}
	movie.Assignment = finish(movie.ID)
	return movie, nil
}

// ShowByTMDBID returns the show with that TMDB ID, without seasons, or nil
// when it isn't in the library. One query.
func (r *CatalogRepository) ShowByTMDBID(ctx context.Context, tmdbID int) (*Show, error) {
	return r.show(ctx, `s.tmdb_id = $1`, tmdbID)
}

// ShowByID returns a show without seasons, or nil. One query.
func (r *CatalogRepository) ShowByID(ctx context.Context, id int64) (*Show, error) {
	return r.show(ctx, `s.id = $1`, id)
}

func (r *CatalogRepository) show(ctx context.Context, where string, arg any) (*Show, error) {
	show, err := scanShow(r.db.QueryRowContext(ctx, `SELECT `+showColumns+` FROM shows s WHERE `+where, arg))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get show: %w", err)
	}
	return show, nil
}

// ShowEpisodes returns every season of a show (season 0 included) with lean
// episode rows: ID, number, absolute number, air date, runtime, whether
// Trakt has a play of it and the active assignment. Names, overviews and
// stills are left out; title summaries don't need them. One query.
func (r *CatalogRepository) ShowEpisodes(ctx context.Context, showID int64) ([]Season, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT sn.id, sn.season_number, COALESCE(sn.name, ''),
		       e.id, COALESCE(e.episode_number, 0), e.absolute_number, COALESCE(e.air_date, ''), COALESCE(e.runtime, 0),
		       `+episodeWatchedSQL("s.tmdb_id", libraryEpisode, `w.show_tmdb_id = (SELECT tmdb_id FROM shows WHERE id = $1)`)+`,
		       `+assignmentColumns+`
		FROM shows s
		JOIN seasons sn ON sn.show_id = s.id
		LEFT JOIN episodes e ON e.season_id = sn.id
		`+activeAssignmentLateral("episode", "e.id")+`
		WHERE s.id = $1
		ORDER BY sn.season_number, e.episode_number
	`, showID)
	if err != nil {
		return nil, fmt.Errorf("failed to load show episodes: %w", err)
	}
	defer rows.Close()

	var seasons []Season
	for rows.Next() {
		var season Season
		var ep Episode
		var epID, absolute sql.NullInt64
		dest, finish := assignmentDest(ItemTypeEpisode)
		dest = append([]any{&season.ID, &season.SeasonNumber, &season.Name,
			&epID, &ep.EpisodeNumber, &absolute, &ep.AirDate, &ep.Runtime, &ep.Watched}, dest...)
		if err := rows.Scan(dest...); err != nil {
			return nil, fmt.Errorf("failed to scan show episode: %w", err)
		}
		if n := len(seasons); n == 0 || seasons[n-1].ID != season.ID {
			season.ShowID = showID
			seasons = append(seasons, season)
		}
		if !epID.Valid {
			continue // season without episodes
		}
		ep.ID = epID.Int64
		ep.SeasonID = season.ID
		if absolute.Valid {
			n := int(absolute.Int64)
			ep.AbsoluteNumber = &n
		}
		ep.Assignment = finish(ep.ID)
		last := &seasons[len(seasons)-1]
		last.Episodes = append(last.Episodes, ep)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read show episodes: %w", err)
	}
	return seasons, nil
}

// SeasonView is one season of a library show, for the season endpoint.
type SeasonView struct {
	ShowID          int64
	ShowTitle       string // The WebDAV folder name, fixed at creation
	ShowYear        int
	IncludeSpecials bool
	// Season holds every episode with its active assignment; nil when the
	// show has no such season.
	Season *Season
}

// SeasonByTMDBID returns a season of the show with that TMDB ID, with full
// episode rows, whether Trakt has a play of each and active assignments, or
// nil when the show isn't in the library. One query.
func (r *CatalogRepository) SeasonByTMDBID(ctx context.Context, tmdbID, seasonNumber int) (*SeasonView, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT s.id, s.title, s.year, s.include_specials,
		       sn.id, COALESCE(sn.name, ''), COALESCE(sn.poster_path, ''),
		       `+episodeColumns+`,
		       `+episodeWatchedSQL("s.tmdb_id", libraryEpisode, `w.show_tmdb_id = $1`)+`,
		       `+assignmentColumns+`
		FROM shows s
		LEFT JOIN seasons sn ON sn.show_id = s.id AND sn.season_number = $2
		LEFT JOIN episodes e ON e.season_id = sn.id
		`+activeAssignmentLateral("episode", "e.id")+`
		WHERE s.tmdb_id = $1
		ORDER BY e.episode_number
	`, tmdbID, seasonNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to load season: %w", err)
	}
	defer rows.Close()

	var view *SeasonView
	for rows.Next() {
		var v SeasonView
		var seasonID sql.NullInt64
		var seasonName, seasonPoster string
		var ep Episode
		epDest, epFinish := episodeScanDest(&ep)
		taDest, taFinish := assignmentDest(ItemTypeEpisode)
		dest := append([]any{&v.ShowID, &v.ShowTitle, &v.ShowYear, &v.IncludeSpecials,
			&seasonID, &seasonName, &seasonPoster}, epDest...)
		dest = append(dest, &ep.Watched)
		if err := rows.Scan(append(dest, taDest...)...); err != nil {
			return nil, fmt.Errorf("failed to scan season row: %w", err)
		}
		if view == nil {
			view = &v
			if seasonID.Valid {
				view.Season = &Season{ID: seasonID.Int64, ShowID: v.ShowID, SeasonNumber: seasonNumber,
					Name: seasonName, PosterPath: seasonPoster}
			}
		}
		if !epFinish() {
			continue
		}
		ep.Assignment = taFinish(ep.ID)
		view.Season.Episodes = append(view.Season.Episodes, ep)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read season: %w", err)
	}
	return view, nil
}

// FileRef names one file of a torrent.
type FileRef struct {
	InfoHash string
	Path     string
}
