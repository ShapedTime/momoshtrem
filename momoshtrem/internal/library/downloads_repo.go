package library

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

// Downloads (#77, PLAN §5 "010_downloads.sql", §6 "Downloads (#77)"): a
// Keep or a smart pick on a movie, a show, a season or an episode, and
// what was watched here (local_watches). internal/downloads plans them;
// this file stores and resolves them.

// DownloadReason is why a download exists: a Keep, or a smart pick.
type DownloadReason string

const (
	DownloadKept  DownloadReason = "kept"
	DownloadSmart DownloadReason = "smart"
)

// DownloadScope is what a download is on.
type DownloadScope string

const (
	DownloadScopeMovie   DownloadScope = "movie"
	DownloadScopeShow    DownloadScope = "show"
	DownloadScopeSeason  DownloadScope = "season"
	DownloadScopeEpisode DownloadScope = "episode"
)

// Valid reports whether s is one of the four scopes.
func (s DownloadScope) Valid() bool {
	switch s {
	case DownloadScopeMovie, DownloadScopeShow, DownloadScopeSeason, DownloadScopeEpisode:
		return true
	}
	return false
}

// SmartSource is why a smart pick was made: Trakt's Up next, or the
// episodes after the one streaming now.
type SmartSource string

const (
	SmartUpNext    SmartSource = "up_next"
	SmartStreaming SmartSource = "streaming"
)

// ReleaseReason is why a download ended: a DELETE, its expires_at, or an
// until-watched download that was watched.
type ReleaseReason string

const (
	ReleaseDeleted ReleaseReason = "deleted"
	ReleaseExpired ReleaseReason = "expired"
	ReleaseWatched ReleaseReason = "watched"
)

// Errors of the download store.
var (
	// ErrSeasonNotFound is a season target whose show has no such season.
	ErrSeasonNotFound = errors.New("season not found")
	// ErrDownloadNotFound is an id that isn't an active download (unknown
	// or released), or a change that doesn't apply to it.
	ErrDownloadNotFound = errors.New("download not found")
	// ErrDownloadExists is a new download on a target that already has an
	// active one.
	ErrDownloadExists = errors.New("the target already has a download")
)

// DownloadTarget is what a download is on, as the API names it: a movie,
// a show or an episode by library id, or a season by its show's id and
// its number.
type DownloadTarget struct {
	Scope        DownloadScope
	ID           int64
	SeasonNumber int
}

// DownloadRecord is a downloads row. The target's id is the scope's: one
// of MovieID, ShowID, SeasonID and EpisodeID is set, the others are 0.
type DownloadRecord struct {
	ID          int64
	Reason      DownloadReason
	SmartSource SmartSource // "" for kept
	Scope       DownloadScope
	MovieID     int64
	ShowID      int64
	SeasonID    int64
	EpisodeID   int64
	// UntilWatched releases watched files one by one, and the download
	// once its scope is watched (internal/downloads).
	UntilWatched bool
	ExpiresAt    *time.Time // nil: no maximum
	ReleasedAt   *time.Time // nil while active
	// ReleaseReason is "" while active.
	ReleaseReason ReleaseReason
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// DownloadTitle is what the API shows of a download's target: the movie,
// or the show with the season and episode of those scopes.
type DownloadTitle struct {
	MediaType string // movie or tv
	TMDBID    int
	// LibraryID is the movie's or the show's id.
	LibraryID     int64
	Title         string
	PosterPath    string
	SeasonNumber  *int // season and episode scopes
	EpisodeNumber *int // episode scope
	EpisodeName   string
	// ShowStatus is TMDB's ("Ended", "Returning Series"…); "" for movies.
	ShowStatus string
	// LaterSeason says the show has a season after this one (season
	// scope): this one has finished airing once its episodes have.
	LaterSeason bool
}

// CoveredItem is a library item a download covers, with its file and when
// it was last watched.
type CoveredItem struct {
	ItemType ItemType
	ItemID   int64
	// SeasonNumber and EpisodeNumber are the episode's; 0 for a movie.
	SeasonNumber  int
	EpisodeNumber int
	AirDate       string // ISO date; "" when unknown
	EpisodeType   string // TMDB's: standard, finale, mid_season
	// WatchedAt is the latest of its Trakt plays and its local watch; nil
	// when neither exists.
	WatchedAt *time.Time
	// File is its active assignment; nil when it has none.
	File *CoveredFile
}

// CoveredFile is an item's active assignment: a file of a torrent, or an
// HDRezka stream (Kind).
type CoveredFile struct {
	Kind      AssignmentKind
	InfoHash  string // "" for HDRezka
	MagnetURI string
	FilePath  string
	FileSize  int64
	// AssignedAt is when the row was written (an Apply's time).
	AssignedAt time.Time
}

// ResolvedDownload is a download with its target and every item its scope
// covers, in episode order, specials last. A movie covers the movie; a
// show every episode of every season the library lists; a season its
// episodes; an episode itself.
type ResolvedDownload struct {
	DownloadRecord
	Title DownloadTitle
	Items []CoveredItem
}

// ItemRef names a movie or an episode.
type ItemRef struct {
	Type ItemType
	ID   int64
}

// DownloadsRepository stores downloads and local watches.
type DownloadsRepository struct {
	db *DB
}

// NewDownloadsRepository creates a DownloadsRepository.
func NewDownloadsRepository(db *DB) *DownloadsRepository {
	return &DownloadsRepository{db: db}
}

const downloadColumns = `id, reason, smart_source, scope, movie_id, show_id, season_id, episode_id,
	until_watched, expires_at, released_at, release_reason, created_at, updated_at`

func scanDownload(s scanner) (DownloadRecord, error) {
	var d DownloadRecord
	var source, release sql.NullString
	var movie, show, season, episode sql.NullInt64
	var expires, released sql.NullTime
	if err := s.Scan(&d.ID, &d.Reason, &source, &d.Scope, &movie, &show, &season, &episode,
		&d.UntilWatched, &expires, &released, &release, &d.CreatedAt, &d.UpdatedAt); err != nil {
		return DownloadRecord{}, err
	}
	d.SmartSource, d.ReleaseReason = SmartSource(source.String), ReleaseReason(release.String)
	d.MovieID, d.ShowID, d.SeasonID, d.EpisodeID = movie.Int64, show.Int64, season.Int64, episode.Int64
	d.ExpiresAt, d.ReleasedAt = nullTimePtr(expires), nullTimePtr(released)
	return d, nil
}

// targetColumn is the column naming a record's target, and its id.
func (d DownloadRecord) targetColumn() (string, int64) {
	switch d.Scope {
	case DownloadScopeMovie:
		return "movie_id", d.MovieID
	case DownloadScopeShow:
		return "show_id", d.ShowID
	case DownloadScopeSeason:
		return "season_id", d.SeasonID
	default:
		return "episode_id", d.EpisodeID
	}
}

func nullID(id int64) sql.NullInt64 { return sql.NullInt64{Int64: id, Valid: id != 0} }

// GetDownload returns the active download with the id;
// ErrDownloadNotFound when there's none (unknown or released).
func (r *DownloadsRepository) GetDownload(ctx context.Context, id int64) (DownloadRecord, error) {
	d, err := scanDownload(r.db.QueryRowContext(ctx,
		`SELECT `+downloadColumns+` FROM downloads WHERE id = $1 AND released_at IS NULL`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return DownloadRecord{}, ErrDownloadNotFound
	}
	if err != nil {
		return DownloadRecord{}, fmt.Errorf("failed to read download %d: %w", id, err)
	}
	return d, nil
}

// ActiveDownloadFor returns the active download on the target of rec (its
// scope and id), kept or smart; nil when there's none.
func (r *DownloadsRepository) ActiveDownloadFor(ctx context.Context, rec DownloadRecord) (*DownloadRecord, error) {
	column, id := rec.targetColumn()
	d, err := scanDownload(r.db.QueryRowContext(ctx,
		`SELECT `+downloadColumns+` FROM downloads WHERE `+column+` = $1 AND released_at IS NULL`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read the download of %s %d: %w", rec.Scope, id, err)
	}
	return &d, nil
}

// CreateDownload inserts rec (its reason, source, scope, target, duration
// and created_at; a zero created_at is now) and answers the row.
// ErrDownloadExists when the target has an active download.
func (r *DownloadsRepository) CreateDownload(ctx context.Context, rec DownloadRecord) (DownloadRecord, error) {
	created := sql.NullTime{Time: rec.CreatedAt, Valid: !rec.CreatedAt.IsZero()}
	d, err := scanDownload(r.db.QueryRowContext(ctx, `
		INSERT INTO downloads (reason, smart_source, scope, movie_id, show_id, season_id, episode_id,
		                       until_watched, expires_at, created_at, updated_at)
		VALUES ($1, NULLIF($2, ''), $3, $4, $5, $6, $7, $8, $9, COALESCE($10, NOW()), COALESCE($10, NOW()))
		RETURNING `+downloadColumns,
		rec.Reason, string(rec.SmartSource), rec.Scope, nullID(rec.MovieID), nullID(rec.ShowID), nullID(rec.SeasonID),
		nullID(rec.EpisodeID), rec.UntilWatched, rec.ExpiresAt, created))
	if isUniqueViolation(err) {
		return DownloadRecord{}, ErrDownloadExists
	}
	if err != nil {
		return DownloadRecord{}, fmt.Errorf("failed to create the download of %s: %w", rec.Scope, err)
	}
	return d, nil
}

// KeepSmartDownload turns an active smart pick into a kept download: the
// same row, created at, with the duration given (PLAN §6: a Keep on a
// smart pick). ErrDownloadNotFound when id isn't an active smart pick.
func (r *DownloadsRepository) KeepSmartDownload(ctx context.Context, id int64, untilWatched bool, expiresAt *time.Time, at time.Time) (DownloadRecord, error) {
	d, err := scanDownload(r.db.QueryRowContext(ctx, `
		UPDATE downloads SET reason = 'kept', smart_source = NULL, until_watched = $2, expires_at = $3,
		       created_at = $4, updated_at = $4
		WHERE id = $1 AND released_at IS NULL AND reason = 'smart'
		RETURNING `+downloadColumns, id, untilWatched, expiresAt, at))
	if errors.Is(err, sql.ErrNoRows) {
		return DownloadRecord{}, ErrDownloadNotFound
	}
	if err != nil {
		return DownloadRecord{}, fmt.Errorf("failed to keep download %d: %w", id, err)
	}
	return d, nil
}

// DownloadDurationPatch changes a kept download's duration: a nil
// UntilWatched keeps it, and SetExpiresAt sends ExpiresAt (nil: no
// maximum).
type DownloadDurationPatch struct {
	UntilWatched *bool
	SetExpiresAt bool
	ExpiresAt    *time.Time
}

// UpdateDownloadDuration applies patch to an active kept download.
// ErrDownloadNotFound when id isn't one.
func (r *DownloadsRepository) UpdateDownloadDuration(ctx context.Context, id int64, patch DownloadDurationPatch) (DownloadRecord, error) {
	var until sql.NullBool
	if patch.UntilWatched != nil {
		until = sql.NullBool{Bool: *patch.UntilWatched, Valid: true}
	}
	d, err := scanDownload(r.db.QueryRowContext(ctx, `
		UPDATE downloads SET until_watched = COALESCE($2, until_watched),
		       expires_at = CASE WHEN $3 THEN $4 ELSE expires_at END, updated_at = NOW()
		WHERE id = $1 AND released_at IS NULL AND reason = 'kept'
		RETURNING `+downloadColumns, id, until, patch.SetExpiresAt, patch.ExpiresAt))
	if errors.Is(err, sql.ErrNoRows) {
		return DownloadRecord{}, ErrDownloadNotFound
	}
	if err != nil {
		return DownloadRecord{}, fmt.Errorf("failed to update download %d: %w", id, err)
	}
	return d, nil
}

// DownloadRelease is a download to release, and why.
type DownloadRelease struct {
	ID     int64
	Reason ReleaseReason
}

// ReleaseDownloads releases the active downloads given, at at, and
// answers the ids it released (an id released already is skipped). The
// rows stay, with released_at and the reason.
func (r *DownloadsRepository) ReleaseDownloads(ctx context.Context, releases []DownloadRelease, at time.Time) ([]int64, error) {
	return r.release(ctx, releases, at, false)
}

// release releases the active downloads given (with smartOnly, only
// those that are still smart picks) and answers the ids it released.
func (r *DownloadsRepository) release(ctx context.Context, releases []DownloadRelease, at time.Time, smartOnly bool) ([]int64, error) {
	if len(releases) == 0 {
		return nil, nil
	}
	ids := make([]int64, len(releases))
	reasons := make([]string, len(releases))
	for i, rel := range releases {
		ids[i], reasons[i] = rel.ID, string(rel.Reason)
	}
	rows, err := r.db.QueryContext(ctx, `
		UPDATE downloads d SET released_at = $3, release_reason = m.reason, updated_at = $3
		FROM unnest($1::bigint[], $2::text[]) AS m(id, reason)
		WHERE d.id = m.id AND d.released_at IS NULL AND (NOT $4 OR d.reason = 'smart')
		RETURNING d.id`, ids, reasons, at, smartOnly)
	if err != nil {
		return nil, fmt.Errorf("failed to release downloads: %w", err)
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("failed to scan a released download: %w", err)
		}
		out = append(out, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to release downloads: %w", err)
	}
	return out, nil
}

// ResolveTarget finds a target in the library and what a download on it
// would cover, without writing anything: its record (ID 0, the target's
// ids set) with its title and items. ErrMovieNotFound, ErrShowNotFound,
// ErrSeasonNotFound or ErrEpisodeNotFound when the library hasn't got it.
func (r *DownloadsRepository) ResolveTarget(ctx context.Context, target DownloadTarget) (*ResolvedDownload, error) {
	var out *ResolvedDownload
	err := r.readTx(ctx, func(q queryer) error {
		rec := DownloadRecord{Scope: target.Scope}
		var err error
		switch target.Scope {
		case DownloadScopeMovie:
			err = exists(ctx, q, `SELECT id FROM movies WHERE id = $1`, ErrMovieNotFound, &rec.MovieID, target.ID)
		case DownloadScopeShow:
			err = exists(ctx, q, `SELECT id FROM shows WHERE id = $1`, ErrShowNotFound, &rec.ShowID, target.ID)
		case DownloadScopeSeason:
			if err = exists(ctx, q, `SELECT id FROM shows WHERE id = $1`, ErrShowNotFound, new(int64), target.ID); err == nil {
				err = exists(ctx, q, `SELECT id FROM seasons WHERE show_id = $1 AND season_number = $2`,
					ErrSeasonNotFound, &rec.SeasonID, target.ID, target.SeasonNumber)
			}
		case DownloadScopeEpisode:
			err = exists(ctx, q, `SELECT id FROM episodes WHERE id = $1`, ErrEpisodeNotFound, &rec.EpisodeID, target.ID)
		default:
			return fmt.Errorf("unknown download scope %q", target.Scope)
		}
		if err != nil {
			return err
		}
		targets := `SELECT 0::bigint AS id, $1::text AS scope, $2::bigint AS movie_id, $3::bigint AS show_id,
			$4::bigint AS season_id, $5::bigint AS episode_id`
		args := []any{rec.Scope, nullID(rec.MovieID), nullID(rec.ShowID), nullID(rec.SeasonID), nullID(rec.EpisodeID)}
		resolved, err := resolve(ctx, q, []DownloadRecord{rec}, targets, args)
		if err != nil {
			return err
		}
		out = &resolved[0]
		return nil
	})
	return out, err
}

func exists(ctx context.Context, q queryer, query string, missing error, id *int64, args ...any) error {
	err := q.QueryRowContext(ctx, query, args...).Scan(id)
	if errors.Is(err, sql.ErrNoRows) {
		return missing
	}
	if err != nil {
		return fmt.Errorf("failed to look up a download target: %w", err)
	}
	return nil
}

// readTx runs f's statements in one read-only REPEATABLE READ transaction:
// one snapshot, so a title deleted between them (its downloads cascading)
// can't leave a download without its title or items.
func (r *DownloadsRepository) readTx(ctx context.Context, f func(q queryer) error) error {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return fmt.Errorf("failed to begin a read: %w", err)
	}
	defer tx.Rollback()
	if err := f(tx); err != nil {
		return err
	}
	return tx.Commit()
}

// ResolveDownloads returns every active download, oldest first, with its
// title and the items it covers: the resolve the download manager runs on
// each tick. Three queries in one snapshot, whatever the number of
// downloads.
func (r *DownloadsRepository) ResolveDownloads(ctx context.Context) ([]ResolvedDownload, error) {
	var out []ResolvedDownload
	err := r.readTx(ctx, func(q queryer) error {
		recs, err := records(ctx, q, `SELECT `+downloadColumns+` FROM downloads WHERE released_at IS NULL ORDER BY created_at, id`)
		if err != nil || len(recs) == 0 {
			return err
		}
		out, err = resolve(ctx, q, recs, `SELECT id, scope, movie_id, show_id, season_id, episode_id FROM downloads WHERE released_at IS NULL`, nil)
		return err
	})
	return out, err
}

// ResolveReleasedWatched returns the downloads released as watched since
// since, with what they covered then: whose pieces the manager demotes
// again after a restart (the cache's demotion lives in memory).
func (r *DownloadsRepository) ResolveReleasedWatched(ctx context.Context, since time.Time) ([]ResolvedDownload, error) {
	const where = ` FROM downloads WHERE release_reason = 'watched' AND released_at >= $1`
	var out []ResolvedDownload
	err := r.readTx(ctx, func(q queryer) error {
		recs, err := records(ctx, q, `SELECT `+downloadColumns+where+` ORDER BY created_at, id`, since)
		if err != nil || len(recs) == 0 {
			return err
		}
		out, err = resolve(ctx, q, recs, `SELECT id, scope, movie_id, show_id, season_id, episode_id`+where, []any{since})
		return err
	})
	return out, err
}

func records(ctx context.Context, q queryer, query string, args ...any) ([]DownloadRecord, error) {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list downloads: %w", err)
	}
	defer rows.Close()
	var out []DownloadRecord
	for rows.Next() {
		d, err := scanDownload(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan a download: %w", err)
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to list downloads: %w", err)
	}
	return out, nil
}

// resolve joins recs to their titles and covered items. targets is the
// SQL of (id, scope, movie_id, show_id, season_id, episode_id) rows, one
// per record, with its args.
func resolve(ctx context.Context, q queryer, recs []DownloadRecord, targets string, args []any) ([]ResolvedDownload, error) {
	out := make([]ResolvedDownload, len(recs))
	byID := make(map[int64]*ResolvedDownload, len(recs))
	for i, rec := range recs {
		out[i].DownloadRecord = rec
		byID[rec.ID] = &out[i]
	}
	if err := titles(ctx, q, byID, targets, args); err != nil {
		return nil, err
	}
	if err := items(ctx, q, byID, targets, args); err != nil {
		return nil, err
	}
	return out, nil
}

func titles(ctx context.Context, q queryer, byID map[int64]*ResolvedDownload, targets string, args []any) error {
	rows, err := q.QueryContext(ctx, `
		WITH t AS (`+targets+`)
		SELECT t.id, CASE WHEN m.id IS NOT NULL THEN 'movie' ELSE 'tv' END,
		       COALESCE(m.tmdb_id, s.tmdb_id), COALESCE(m.id, s.id), COALESCE(m.title, s.title),
		       COALESCE(m.poster_path, s.poster_path, ''),
		       sn.season_number, e.episode_number, COALESCE(e.name, ''), COALESCE(s.status, ''),
		       sn.id IS NOT NULL AND EXISTS (SELECT 1 FROM seasons later
		                                     WHERE later.show_id = s.id AND later.season_number > sn.season_number)
		FROM t
		LEFT JOIN movies m ON m.id = t.movie_id
		LEFT JOIN episodes e ON e.id = t.episode_id
		LEFT JOIN seasons sn ON sn.id = COALESCE(t.season_id, e.season_id)
		LEFT JOIN shows s ON s.id = COALESCE(t.show_id, sn.show_id)
	`, args...)
	if err != nil {
		return fmt.Errorf("failed to read the downloads' titles: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var t DownloadTitle
		var season, episode sql.NullInt32
		if err := rows.Scan(&id, &t.MediaType, &t.TMDBID, &t.LibraryID, &t.Title, &t.PosterPath,
			&season, &episode, &t.EpisodeName, &t.ShowStatus, &t.LaterSeason); err != nil {
			return fmt.Errorf("failed to scan a download's title: %w", err)
		}
		t.SeasonNumber, t.EpisodeNumber = nullIntPtr(season), nullIntPtr(episode)
		if d := byID[id]; d != nil {
			d.Title = t
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("failed to read the downloads' titles: %w", err)
	}
	return nil
}

// episodeWatchedAtSQL is the latest watch of the library episode e (with
// sn and s): Trakt's by the matching rule (its two halves as two lookups,
// as episodeWatchedSQL does), or local; NULL without one.
var episodeWatchedAtSQL = `GREATEST(
	         (SELECT max(w.last_watched_at) FROM trakt_watched_episodes w
	          WHERE w.show_tmdb_id = s.tmdb_id AND ` + sameEpisodeByID(traktEpisode("w"), libraryEpisode) + `),
	         (SELECT max(w.last_watched_at) FROM trakt_watched_episodes w
	          WHERE w.show_tmdb_id = s.tmdb_id AND ` + sameEpisodeByNumber(traktEpisode("w"), libraryEpisode) + `),
	         (SELECT lw.watched_at FROM local_watches lw WHERE lw.episode_id = e.id))`

// episodeItemSQL selects the episodes joined as e (with sn and s) as
// covered items of download t.id, each with its latest watch.
var episodeItemSQL = `
	SELECT t.id AS download_id, 'episode' AS item_type, e.id AS item_id, sn.season_number, e.episode_number,
	       e.air_date, e.episode_type, ` + episodeWatchedAtSQL + ` AS watched_at`

func items(ctx context.Context, q queryer, byID map[int64]*ResolvedDownload, targets string, args []any) error {
	rows, err := q.QueryContext(ctx, `
		WITH t AS (`+targets+`),
		items AS (
			SELECT t.id AS download_id, 'movie' AS item_type, m.id AS item_id, 0 AS season_number, 0 AS episode_number,
			       NULL::text AS air_date, NULL::text AS episode_type,
			       GREATEST((SELECT w.last_watched_at FROM trakt_watched_movies w WHERE w.tmdb_id = m.tmdb_id),
			                (SELECT lw.watched_at FROM local_watches lw WHERE lw.movie_id = m.id)) AS watched_at
			FROM t JOIN movies m ON m.id = t.movie_id
			WHERE t.scope = 'movie'
			UNION ALL`+episodeItemSQL+`
			FROM t JOIN seasons sn ON sn.show_id = t.show_id JOIN episodes e ON e.season_id = sn.id JOIN shows s ON s.id = sn.show_id
			WHERE t.scope = 'show'
			UNION ALL`+episodeItemSQL+`
			FROM t JOIN seasons sn ON sn.id = t.season_id JOIN episodes e ON e.season_id = sn.id JOIN shows s ON s.id = sn.show_id
			WHERE t.scope = 'season'
			UNION ALL`+episodeItemSQL+`
			FROM t JOIN episodes e ON e.id = t.episode_id JOIN seasons sn ON sn.id = e.season_id JOIN shows s ON s.id = sn.show_id
			WHERE t.scope = 'episode'
		)
		SELECT i.download_id, i.item_type, i.item_id, i.season_number, i.episode_number,
		       COALESCE(i.air_date, ''), COALESCE(i.episode_type, ''), i.watched_at,
		       a.kind, a.info_hash, a.magnet_uri, a.file_path, a.file_size, a.created_at
		FROM items i
		LEFT JOIN LATERAL (
			SELECT ta.kind, ta.info_hash, ta.magnet_uri, ta.file_path, ta.file_size, ta.created_at
			FROM torrent_assignments ta
			WHERE ta.item_type = i.item_type AND ta.item_id = i.item_id AND ta.is_active
			ORDER BY ta.created_at DESC, ta.id DESC
			LIMIT 1
		) a ON TRUE
		ORDER BY i.download_id, i.season_number = 0, i.season_number, i.episode_number, i.item_id
	`, args...)
	if err != nil {
		return fmt.Errorf("failed to resolve the downloads' items: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var it CoveredItem
		var watched, assigned sql.NullTime
		var kind, hash, magnet, path sql.NullString
		var size sql.NullInt64
		if err := rows.Scan(&id, &it.ItemType, &it.ItemID, &it.SeasonNumber, &it.EpisodeNumber, &it.AirDate,
			&it.EpisodeType, &watched, &kind, &hash, &magnet, &path, &size, &assigned); err != nil {
			return fmt.Errorf("failed to scan a download's item: %w", err)
		}
		it.WatchedAt = nullTimePtr(watched)
		if path.Valid {
			it.File = &CoveredFile{Kind: AssignmentKind(kind.String), InfoHash: hash.String, MagnetURI: magnet.String,
				FilePath: path.String, FileSize: size.Int64, AssignedAt: assigned.Time}
			if it.File.Kind == "" {
				it.File.Kind = AssignmentKindTorrent
			}
		}
		if d := byID[id]; d != nil {
			d.Items = append(d.Items, it)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("failed to resolve the downloads' items: %w", err)
	}
	return nil
}

// UpsertSmartDownload adds a smart pick of an episode (until watched,
// expiring at expiresAt) unless the episode has an active download of its
// own; created says whether it did, id is the row either way.
func (r *DownloadsRepository) UpsertSmartDownload(ctx context.Context, episodeID int64, source SmartSource, at, expiresAt time.Time) (id int64, created bool, err error) {
	err = r.db.QueryRowContext(ctx, `
		INSERT INTO downloads (reason, smart_source, scope, episode_id, until_watched, expires_at, created_at, updated_at)
		VALUES ('smart', $2, 'episode', $1, TRUE, $4, $3, $3)
		ON CONFLICT (episode_id) WHERE episode_id IS NOT NULL AND released_at IS NULL DO NOTHING
		RETURNING id`, episodeID, source, at, expiresAt).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		err = r.db.QueryRowContext(ctx, `SELECT id FROM downloads WHERE episode_id = $1 AND released_at IS NULL`, episodeID).Scan(&id)
		if err != nil {
			return 0, false, fmt.Errorf("failed to read episode %d's download: %w", episodeID, err)
		}
		return id, false, nil
	}
	if isForeignKeyViolation(err) {
		return 0, false, ErrEpisodeNotFound
	}
	if err != nil {
		return 0, false, fmt.Errorf("failed to add a smart pick of episode %d: %w", episodeID, err)
	}
	return id, true, nil
}

// smartBlockSQL is the re-pick rule (PLAN §6 "Re-picks") for the library
// episode e, joined to its season sn and show s, as an SQL CASE: why it
// can't be a smart pick, else NULL. plays is episodeWatchedSQL's condition
// on the plays.
//   - watched: a Trakt play (the matching rule) or a local watch, ever;
//   - dismissed: a smart pick of it was released deleted;
//   - expired: a smart pick of it ran its whole lifetime (released expired
//     at its expires_at or after), and nothing of the show was played
//     after the last one. A pick released early, as it stopped being a
//     candidate, blocks nothing.
func smartBlockSQL(plays string) string {
	return `CASE
		WHEN EXISTS (SELECT 1 FROM local_watches lw WHERE lw.episode_id = e.id)
		  OR ` + episodeWatchedSQL("s.tmdb_id", libraryEpisode, plays) + `
		  THEN 'watched'
		WHEN EXISTS (SELECT 1 FROM downloads d WHERE d.episode_id = e.id AND d.reason = 'smart' AND d.release_reason = 'deleted')
		  THEN 'dismissed'
		WHEN EXISTS (
			SELECT 1 FROM (SELECT max(d.released_at) AS at FROM downloads d
			               WHERE d.episode_id = e.id AND d.reason = 'smart' AND d.release_reason = 'expired'
			                 AND d.released_at >= d.expires_at) lifetime
			WHERE lifetime.at IS NOT NULL
			  AND NOT EXISTS (SELECT 1 FROM trakt_watched_episodes w
			                  WHERE w.show_tmdb_id = s.tmdb_id AND w.last_watched_at > lifetime.at)
			  AND NOT EXISTS (SELECT 1 FROM local_watches lw JOIN episodes e2 ON e2.id = lw.episode_id
			                  JOIN seasons s2 ON s2.id = e2.season_id
			                  WHERE s2.show_id = s.id AND lw.watched_at > lifetime.at))
		  THEN 'expired'
	END`
}

// SmartPickBlocked says why an episode can't be a smart pick (PLAN §6
// "Re-picks"), "" when it can: watched (a Trakt play or a local watch,
// ever), dismissed (a smart pick of it was released deleted), or expired
// (a smart pick of it ran its whole lifetime, and nothing of the show was
// played after). smartBlockSQL is the rule.
func (r *DownloadsRepository) SmartPickBlocked(ctx context.Context, episodeID int64) (string, error) {
	var reason sql.NullString
	err := r.db.QueryRowContext(ctx, `
		SELECT `+smartBlockSQL("TRUE")+`
		FROM episodes e JOIN seasons sn ON sn.id = e.season_id JOIN shows s ON s.id = sn.show_id
		WHERE e.id = $1`, episodeID).Scan(&reason)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrEpisodeNotFound
	}
	if err != nil {
		return "", fmt.Errorf("failed to check episode %d's smart re-pick: %w", episodeID, err)
	}
	return reason.String, nil
}

// UpsertLocalWatch records that the item was watched here at at: a local
// session reached 90% (PLAN §5 "local_watches"). A later watch moves the
// time; an earlier one doesn't. An item that's gone is skipped.
func (r *DownloadsRepository) UpsertLocalWatch(ctx context.Context, item ItemRef, at time.Time) error {
	var query string
	switch item.Type {
	case ItemTypeMovie:
		query = `INSERT INTO local_watches (movie_id, watched_at) SELECT id, $2 FROM movies WHERE id = $1
			ON CONFLICT (movie_id) WHERE movie_id IS NOT NULL
			DO UPDATE SET watched_at = GREATEST(local_watches.watched_at, EXCLUDED.watched_at)`
	case ItemTypeEpisode:
		query = `INSERT INTO local_watches (episode_id, watched_at) SELECT id, $2 FROM episodes WHERE id = $1
			ON CONFLICT (episode_id) WHERE episode_id IS NOT NULL
			DO UPDATE SET watched_at = GREATEST(local_watches.watched_at, EXCLUDED.watched_at)`
	default:
		return fmt.Errorf("unknown item type %q", item.Type)
	}
	if _, err := r.db.ExecContext(ctx, query, item.ID, at); err != nil {
		return fmt.Errorf("failed to record a local watch of %s %d: %w", item.Type, item.ID, err)
	}
	return nil
}

// FileItems returns the items a torrent file plays as: its active
// assignments (a grouped file is several episodes).
func (r *DownloadsRepository) FileItems(ctx context.Context, infoHash, filePath string) ([]ItemRef, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT item_type, item_id FROM torrent_assignments
		WHERE info_hash = $1 AND file_path = $2 AND is_active AND kind = 'torrent'
		ORDER BY item_type, item_id`, strings.ToLower(infoHash), filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read what %s plays as: %w", filePath, err)
	}
	defer rows.Close()
	var out []ItemRef
	for rows.Next() {
		var ref ItemRef
		if err := rows.Scan(&ref.Type, &ref.ID); err != nil {
			return nil, fmt.Errorf("failed to scan an item: %w", err)
		}
		out = append(out, ref)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read what %s plays as: %w", filePath, err)
	}
	return out, nil
}

// EpisodeIDByNumber returns the show's episode with the season and number;
// 0 when there's none.
func (r *DownloadsRepository) EpisodeIDByNumber(ctx context.Context, showID int64, season, episode int) (int64, error) {
	var id int64
	err := r.db.QueryRowContext(ctx, `
		SELECT e.id FROM episodes e JOIN seasons sn ON sn.id = e.season_id
		WHERE sn.show_id = $1 AND sn.season_number = $2 AND e.episode_number = $3`, showID, season, episode).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("failed to find show %d's S%02dE%02d: %w", showID, season, episode, err)
	}
	return id, nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func isForeignKeyViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23503"
}
