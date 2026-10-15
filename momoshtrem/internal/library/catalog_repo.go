package library

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
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
// episode rows: ID, number, absolute number, air date, runtime and the active
// assignment. Names, overviews and stills are left out; title summaries don't
// need them. One query.
func (r *CatalogRepository) ShowEpisodes(ctx context.Context, showID int64) ([]Season, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT sn.id, sn.season_number, COALESCE(sn.name, ''),
		       e.id, COALESCE(e.episode_number, 0), e.absolute_number, COALESCE(e.air_date, ''), COALESCE(e.runtime, 0),
		       `+assignmentColumns+`
		FROM seasons sn
		LEFT JOIN episodes e ON e.season_id = sn.id
		`+activeAssignmentLateral("episode", "e.id")+`
		WHERE sn.show_id = $1
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
			&epID, &ep.EpisodeNumber, &absolute, &ep.AirDate, &ep.Runtime}, dest...)
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
// episode rows and active assignments, or nil when the show isn't in the
// library. One query.
func (r *CatalogRepository) SeasonByTMDBID(ctx context.Context, tmdbID, seasonNumber int) (*SeasonView, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT s.id, s.title, s.year, s.include_specials,
		       sn.id, COALESCE(sn.name, ''), COALESCE(sn.poster_path, ''),
		       `+episodeColumns+`,
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

// Library sorts.
const (
	LibrarySortAdded       = "added"
	LibrarySortTitle       = "title"
	LibrarySortLastWatched = "last_watched"
	LibrarySortSize        = "size"
)

// Library status filters the database can answer (watching needs B5).
const (
	LibraryStatusReady       = "ready"
	LibraryStatusIncomplete  = "incomplete"
	LibraryStatusNeedsReview = "needs_review"
)

// LibraryQuery selects library items.
type LibraryQuery struct {
	MediaType string // "", "movie" or "tv"
	Status    string // "" or a LibraryStatus*
	Sort      string // A LibrarySort*; "" is added
	Search    string // Title substring, case-insensitive
	Limit     int    // 0: no limit
	Offset    int

	// Today is the date (YYYY-MM-DD) episodes count as aired by.
	Today string
	// Downloading lists the files the torrent client is fetching right now
	// (loaded, active, incomplete): their items are "downloading".
	Downloading []FileRef

	// OnlyTMDB restricts the result to these TMDB IDs (similar-title
	// annotation); nil means every item.
	OnlyTMDB *TMDBIDs
}

// TMDBIDs are movie and show TMDB IDs.
type TMDBIDs struct {
	Movies []int
	Shows  []int
}

// LibraryItem is a movie or show with its availability, as GET /library
// lists it.
type LibraryItem struct {
	MediaType  string // "movie" or "tv"
	ID         int64
	TMDBID     int
	Title      string
	Year       int
	PosterPath string
	CreatedAt  time.Time
	// Shows only (nil for movies): regular episodes that aired or have a
	// file, and those with a file.
	EpisodesTotal *int
	EpisodesReady *int
	SizeBytes     int64
	// NeedsReview counts the files of the title's open mapping jobs that
	// need a look (PLAN §6 "Review rules").
	NeedsReview int
	// Stage is the availability stage: needs_torrent, downloading or ready.
	Stage string
}

// libraryItemsSQL computes every item's availability in SQL, so /library can
// filter and page on it. It mirrors the stage rules of internal/catalog
// (PLAN §6), which compute the same from loaded rows for title pages; an
// integration test keeps the two in step:
//   - movie: needs_torrent without an active assignment, downloading while
//     its file is being fetched, else ready;
//   - show: needs_torrent with nothing assigned or a non-empty frontier
//     (a regular episode aired after the last assigned one), downloading
//     while any assigned file is being fetched, else ready. Specials never
//     count towards the frontier, and are left out entirely while the show's
//     include_specials is off.
//
// An item has a file when it has an active assignment (its row id): an
// HDRezka row has no info hash. Only torrent files are ever downloading.
// A show's size counts each file once: episodes a multi-episode file plays
// share its (kind, hash, path).
//
// needs_review counts the title's files under review (reviewCTEs), like
// ReviewState.
//
// Episode positions are encoded as season·10⁶ + episode so MAX can compare
// them.
//
// Parameters: $1/$2 downloading hashes/paths, $3 restrict to IDs, $4 movie
// IDs, $5 show IDs, $6 today, $7 media type ("" for both).
var libraryItemsSQL = `
WITH downloading AS (
	SELECT h AS info_hash, p AS file_path FROM unnest($1::text[], $2::text[]) AS d(h, p)
),
` + reviewCTEs("TRUE") + `,
review_counts AS (
	SELECT rj.show_id, rj.movie_id, count(*) AS needs_review
	FROM review_files rf JOIN review_jobs rj ON rj.id = rf.job_id
	GROUP BY rj.show_id, rj.movie_id
),
movie_items AS (
	SELECT 'movie'::text AS media_type, m.id, m.tmdb_id, m.title, m.year, COALESCE(m.poster_path, '') AS poster_path,
	       m.created_at, NULL::bigint AS episodes_total, NULL::bigint AS episodes_ready,
	       COALESCE(ta.file_size, 0)::bigint AS size_bytes,
	       COALESCE((SELECT rc.needs_review FROM review_counts rc WHERE rc.movie_id = m.id), 0)::bigint AS needs_review,
	       CASE WHEN ta.id IS NULL THEN 'needs_torrent'
	            WHEN ta.kind = 'torrent' AND (ta.info_hash, ta.file_path) IN (SELECT info_hash, file_path FROM downloading)
	                 THEN 'downloading'
	            ELSE 'ready' END AS stage
	FROM movies m
	` + `LEFT JOIN LATERAL (
		SELECT t.id, t.kind, t.info_hash, t.file_path, t.file_size
		FROM torrent_assignments t
		WHERE t.item_type = 'movie' AND t.item_id = m.id AND t.is_active
		ORDER BY t.created_at DESC, t.id DESC
		LIMIT 1
	) ta ON TRUE
	WHERE $7 IN ('', 'movie') AND (NOT $3::boolean OR m.tmdb_id = ANY($4::int[]))
),
show_episodes AS (
	SELECT sn.show_id, sn.season_number, e.episode_number, e.air_date,
	       ta.id AS ta_id, ta.kind, ta.info_hash, ta.file_path, ta.file_size,
	       sn.season_number::bigint * 1000000 + e.episode_number AS position
	FROM shows s
	JOIN seasons sn ON sn.show_id = s.id AND (sn.season_number > 0 OR s.include_specials)
	JOIN episodes e ON e.season_id = sn.id
	LEFT JOIN LATERAL (
		SELECT t.id, t.kind, t.info_hash, t.file_path, t.file_size
		FROM torrent_assignments t
		WHERE t.item_type = 'episode' AND t.item_id = e.id AND t.is_active
		ORDER BY t.created_at DESC, t.id DESC
		LIMIT 1
	) ta ON TRUE
	WHERE $7 IN ('', 'tv') AND (NOT $3::boolean OR s.tmdb_id = ANY($5::int[]))
),
show_stats AS (
	SELECT show_id,
	       COUNT(*) FILTER (WHERE season_number > 0 AND (ta_id IS NOT NULL OR air_date <= $6)) AS episodes_total,
	       COUNT(*) FILTER (WHERE season_number > 0 AND ta_id IS NOT NULL) AS episodes_ready,
	       COUNT(ta_id) AS assigned,
	       MAX(position) FILTER (WHERE season_number > 0 AND ta_id IS NOT NULL) AS last_assigned,
	       MAX(position) FILTER (WHERE season_number > 0 AND air_date <= $6) AS last_aired,
	       COALESCE(BOOL_OR(kind = 'torrent' AND (info_hash, file_path) IN (SELECT info_hash, file_path FROM downloading)),
	                FALSE) AS downloading
	FROM show_episodes
	GROUP BY show_id
),
show_sizes AS (
	SELECT show_id, SUM(file_size)::bigint AS size_bytes
	FROM (SELECT DISTINCT show_id, kind, info_hash, file_path, file_size FROM show_episodes WHERE ta_id IS NOT NULL) f
	GROUP BY show_id
),
show_items AS (
	SELECT 'tv'::text, s.id, s.tmdb_id, s.title, s.year, COALESCE(s.poster_path, ''), s.created_at,
	       COALESCE(st.episodes_total, 0), COALESCE(st.episodes_ready, 0), COALESCE(sz.size_bytes, 0),
	       COALESCE((SELECT rc.needs_review FROM review_counts rc WHERE rc.show_id = s.id), 0)::bigint,
	       CASE WHEN COALESCE(st.assigned, 0) = 0 OR st.last_aired > COALESCE(st.last_assigned, -1) THEN 'needs_torrent'
	            WHEN st.downloading THEN 'downloading'
	            ELSE 'ready' END
	FROM shows s
	LEFT JOIN show_stats st ON st.show_id = s.id
	LEFT JOIN show_sizes sz ON sz.show_id = s.id
	WHERE $7 IN ('', 'tv') AND (NOT $3::boolean OR s.tmdb_id = ANY($5::int[]))
)
SELECT * FROM movie_items
UNION ALL
SELECT * FROM show_items`

// librarySorts are the whitelisted ORDER BY clauses, each ending in a unique
// tie-break so pages don't overlap.
var librarySorts = map[string]string{
	LibrarySortAdded: `created_at DESC, media_type, id`,
	LibrarySortTitle: `lower(title), year, media_type, id`,
	// No watch history until B5: newest first, like added.
	LibrarySortLastWatched: `created_at DESC, media_type, id`,
	LibrarySortSize:        `size_bytes DESC, created_at DESC, media_type, id`,
}

// LibraryItems returns one page of library items and how many match in all.
// One query (COUNT(*) OVER ()), plus a count when the page is past the end.
func (r *CatalogRepository) LibraryItems(ctx context.Context, q LibraryQuery) ([]LibraryItem, int, error) {
	if q.MediaType != "" && q.MediaType != "movie" && q.MediaType != "tv" {
		return nil, 0, fmt.Errorf("unknown media type %q", q.MediaType)
	}
	if q.Sort == "" {
		q.Sort = LibrarySortAdded
	}
	order, ok := librarySorts[q.Sort]
	if !ok {
		return nil, 0, fmt.Errorf("unknown library sort %q", q.Sort)
	}
	var statusFilter string
	switch q.Status {
	case "":
	case LibraryStatusReady:
		statusFilter = ` AND stage = 'ready'`
	case LibraryStatusIncomplete:
		statusFilter = ` AND stage <> 'ready'`
	case LibraryStatusNeedsReview:
		statusFilter = ` AND needs_review > 0`
	default:
		return nil, 0, fmt.Errorf("unknown library status %q", q.Status)
	}

	hashes, paths := splitFileRefs(q.Downloading)
	restrict, movieIDs, showIDs := false, []int32{}, []int32{}
	if q.OnlyTMDB != nil {
		restrict, movieIDs, showIDs = true, int32s(q.OnlyTMDB.Movies), int32s(q.OnlyTMDB.Shows)
	}
	var limit any // NULL: no limit
	if q.Limit > 0 {
		limit = q.Limit
	}
	args := []any{hashes, paths, restrict, movieIDs, showIDs, q.Today, q.MediaType, likePattern(q.Search), limit, q.Offset}

	where := `($8 = '' OR title ILIKE $8 ESCAPE '\')` + statusFilter
	rows, err := r.db.QueryContext(ctx, `
		WITH items AS (`+libraryItemsSQL+`)
		SELECT media_type, id, tmdb_id, title, year, poster_path, created_at,
		       episodes_total, episodes_ready, size_bytes, needs_review, stage, COUNT(*) OVER ()
		FROM items
		WHERE `+where+`
		ORDER BY `+order+`
		LIMIT $9 OFFSET $10
	`, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list library: %w", err)
	}
	defer rows.Close()

	items := []LibraryItem{}
	total := 0
	for rows.Next() {
		var item LibraryItem
		var episodesTotal, episodesReady sql.NullInt64
		if err := rows.Scan(&item.MediaType, &item.ID, &item.TMDBID, &item.Title, &item.Year, &item.PosterPath,
			&item.CreatedAt, &episodesTotal, &episodesReady, &item.SizeBytes, &item.NeedsReview, &item.Stage, &total); err != nil {
			return nil, 0, fmt.Errorf("failed to scan library item: %w", err)
		}
		if episodesTotal.Valid {
			n, m := int(episodesTotal.Int64), int(episodesReady.Int64)
			item.EpisodesTotal, item.EpisodesReady = &n, &m
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("failed to read library: %w", err)
	}

	// A page past the end has no rows to carry the total.
	if len(items) == 0 && q.Offset > 0 {
		err := r.db.QueryRowContext(ctx, `
			WITH items AS (`+libraryItemsSQL+`)
			SELECT COUNT(*) FROM items WHERE `+where,
			args[:8]...).Scan(&total)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to count library: %w", err)
		}
	}
	return items, total, nil
}

func splitFileRefs(refs []FileRef) (hashes, paths []string) {
	hashes, paths = make([]string, len(refs)), make([]string, len(refs))
	for i, f := range refs {
		hashes[i], paths[i] = f.InfoHash, f.Path
	}
	return hashes, paths
}

func int32s(ids []int) []int32 {
	out := make([]int32, len(ids))
	for i, id := range ids {
		out[i] = int32(id)
	}
	return out
}

// likePattern turns a search into an ILIKE substring pattern, escaping the
// wildcards; "" stays "" (no filter).
func likePattern(search string) string {
	search = strings.TrimSpace(search)
	if search == "" {
		return ""
	}
	escaped := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(search)
	return "%" + escaped + "%"
}
