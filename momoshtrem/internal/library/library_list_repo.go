package library

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Library sorts.
const (
	LibrarySortAdded       = "added"
	LibrarySortTitle       = "title"
	LibrarySortLastWatched = "last_watched"
	LibrarySortSize        = "size"
)

// Library status filters the database can answer. Watching needs B5's
// watch history: until then it matches nothing.
const (
	LibraryStatusReady       = "ready"
	LibraryStatusIncomplete  = "incomplete"
	LibraryStatusNeedsReview = "needs_review"
	LibraryStatusWatching    = "watching"
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

	// IDs keeps only these titles (GET /library?ids=), ANDed with the other
	// filters. The counts honour it; the summary doesn't. nil: every title.
	IDs *TMDBIDs
	// OnlyTMDB computes only these titles (similar-title annotation): the
	// rest of the library is never looked at, so the summary covers just
	// them too. nil: every title.
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
	Title      string // The WebDAV folder name, fixed at creation
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
	// ReviewJobID is the newest of those jobs; nil without one.
	ReviewJobID *int64
	// Stage is the availability stage: needs_torrent, downloading or ready.
	Stage string

	// Movies only: minutes (0 when unknown), and the active assignment's
	// resolution and file inside its torrent ("" without one).
	Runtime    int
	Resolution string
	FilePath   string
	// HasFile: anything is assigned (shows: specials too, even hidden ones),
	// so the VFS lists the title.
	HasFile bool
	// Downloading are the title's files being fetched (shows: of its visible
	// seasons), each once.
	Downloading []FileRef
	// Missing is a show's first frontier run; nil for movies and when the
	// frontier is empty.
	Missing *FrontierRun
}

// FrontierRun is the first run of a show's frontier (PLAN §6 "Availability
// rules"): consecutive aired regular episodes of one season, after the last
// one with a file.
type FrontierRun struct {
	SeasonNumber int
	From, To     int  // in-season episode numbers
	AbsoluteFrom *int // nil without absolute numbers
	AbsoluteTo   *int
	Count        int
}

// LibraryCounts are the filter chips' counts: they honour the search and
// IDs, never the type or status.
type LibraryCounts struct {
	All, Movies, Shows int
	Ready, Incomplete  int
	NeedsReview        int
}

// LibrarySummary describes the whole library, whatever the filters.
type LibrarySummary struct {
	Total     int
	SizeBytes int64
	Ready     int
}

// LibraryListing is one page of library items, how many match in all, and
// the counts and summary.
type LibraryListing struct {
	Items   []LibraryItem
	Total   int
	Counts  LibraryCounts
	Summary LibrarySummary
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
// ReviewState, and review_job_id is the newest of its review jobs.
//
// missing_* is a show's first frontier run (catalog.FirstMissingRun): from
// the first aired regular episode after the last assigned one, up to the
// next one in that season that hasn't aired.
//
// Episode positions are encoded as season·10⁶ + episode so MAX can compare
// them.
//
// Parameters: $1/$2 downloading hashes/paths, $3 restrict to IDs, $4 movie
// IDs, $5 show IDs, $6 today.
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
review_newest AS (
	SELECT show_id, movie_id, max(id) AS job_id FROM review_jobs GROUP BY show_id, movie_id
),
movie_items AS (
	SELECT 'movie'::text AS media_type, m.id, m.tmdb_id, m.title, m.year, COALESCE(m.poster_path, '') AS poster_path,
	       m.created_at, NULL::bigint AS episodes_total, NULL::bigint AS episodes_ready,
	       COALESCE(ta.file_size, 0)::bigint AS size_bytes,
	       COALESCE((SELECT rc.needs_review FROM review_counts rc WHERE rc.movie_id = m.id), 0)::bigint AS needs_review,
	       CASE WHEN ta.id IS NULL THEN 'needs_torrent'
	            WHEN dl.fetching THEN 'downloading'
	            ELSE 'ready' END AS stage,
	       (SELECT rn.job_id FROM review_newest rn WHERE rn.movie_id = m.id) AS review_job_id,
	       COALESCE(m.runtime, 0) AS runtime, COALESCE(ta.resolution, '') AS resolution,
	       COALESCE(ta.file_path, '') AS file_path, ta.id IS NOT NULL AS has_file,
	       CASE WHEN dl.fetching THEN jsonb_build_array(jsonb_build_object('h', ta.info_hash, 'p', ta.file_path))
	            ELSE '[]'::jsonb END AS downloading,
	       NULL::int AS missing_season, NULL::int AS missing_from, NULL::int AS missing_to,
	       NULL::int AS missing_absolute_from, NULL::int AS missing_absolute_to, NULL::bigint AS missing_count
	FROM movies m
	LEFT JOIN LATERAL (
		SELECT t.id, t.kind, t.info_hash, t.file_path, t.file_size, t.resolution
		FROM torrent_assignments t
		WHERE t.item_type = 'movie' AND t.item_id = m.id AND t.is_active
		ORDER BY t.created_at DESC, t.id DESC
		LIMIT 1
	) ta ON TRUE
	CROSS JOIN LATERAL (
		SELECT COALESCE(ta.kind = 'torrent' AND (ta.info_hash, ta.file_path) IN (SELECT info_hash, file_path FROM downloading),
		                FALSE) AS fetching
	) dl
	WHERE NOT $3::boolean OR m.tmdb_id = ANY($4::int[])
),
show_episodes AS (
	SELECT sn.show_id, sn.season_number, e.episode_number, e.absolute_number, e.air_date,
	       ta.id AS ta_id, ta.kind, ta.info_hash, ta.file_path, ta.file_size,
	       COALESCE(ta.kind = 'torrent' AND (ta.info_hash, ta.file_path) IN (SELECT info_hash, file_path FROM downloading),
	                FALSE) AS fetching,
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
	WHERE NOT $3::boolean OR s.tmdb_id = ANY($5::int[])
),
show_stats AS (
	SELECT show_id,
	       COUNT(*) FILTER (WHERE season_number > 0 AND (ta_id IS NOT NULL OR air_date <= $6)) AS episodes_total,
	       COUNT(*) FILTER (WHERE season_number > 0 AND ta_id IS NOT NULL) AS episodes_ready,
	       COUNT(ta_id) AS assigned,
	       MAX(position) FILTER (WHERE season_number > 0 AND ta_id IS NOT NULL) AS last_assigned,
	       MAX(position) FILTER (WHERE season_number > 0 AND air_date <= $6) AS last_aired,
	       BOOL_OR(fetching) AS downloading,
	       COALESCE(jsonb_agg(DISTINCT jsonb_build_object('h', info_hash, 'p', file_path)) FILTER (WHERE fetching),
	                '[]'::jsonb) AS files
	FROM show_episodes
	GROUP BY show_id
),
show_sizes AS (
	SELECT show_id, SUM(file_size)::bigint AS size_bytes
	FROM (SELECT DISTINCT show_id, kind, info_hash, file_path, file_size FROM show_episodes WHERE ta_id IS NOT NULL) f
	GROUP BY show_id
),
frontier AS (
	SELECT se.show_id, se.season_number, se.episode_number, se.absolute_number, se.air_date, se.position
	FROM show_episodes se JOIN show_stats st ON st.show_id = se.show_id
	WHERE se.season_number > 0 AND se.position > COALESCE(st.last_assigned, -1)
),
frontier_start AS (
	SELECT DISTINCT ON (show_id) show_id, season_number, position
	FROM frontier
	WHERE air_date <= $6
	ORDER BY show_id, position
),
frontier_stop AS (
	SELECT fs.show_id, MIN(f.position) AS position
	FROM frontier_start fs
	JOIN frontier f ON f.show_id = fs.show_id AND f.season_number = fs.season_number AND f.position > fs.position
	WHERE f.air_date IS NULL OR f.air_date > $6
	GROUP BY fs.show_id
),
missing_runs AS (
	SELECT fs.show_id, fs.season_number,
	       MIN(f.episode_number) AS episode_from, MAX(f.episode_number) AS episode_to,
	       (array_agg(f.absolute_number ORDER BY f.position))[1] AS absolute_from,
	       (array_agg(f.absolute_number ORDER BY f.position DESC))[1] AS absolute_to,
	       COUNT(*) AS episodes
	FROM frontier_start fs
	LEFT JOIN frontier_stop fp ON fp.show_id = fs.show_id
	JOIN frontier f ON f.show_id = fs.show_id AND f.season_number = fs.season_number
	     AND f.position >= fs.position AND (fp.position IS NULL OR f.position < fp.position)
	GROUP BY fs.show_id, fs.season_number
),
show_items AS (
	SELECT 'tv'::text, s.id, s.tmdb_id, s.title, s.year, COALESCE(s.poster_path, ''), s.created_at,
	       COALESCE(st.episodes_total, 0), COALESCE(st.episodes_ready, 0), COALESCE(sz.size_bytes, 0),
	       COALESCE((SELECT rc.needs_review FROM review_counts rc WHERE rc.show_id = s.id), 0)::bigint,
	       CASE WHEN COALESCE(st.assigned, 0) = 0 OR st.last_aired > COALESCE(st.last_assigned, -1) THEN 'needs_torrent'
	            WHEN st.downloading THEN 'downloading'
	            ELSE 'ready' END,
	       (SELECT rn.job_id FROM review_newest rn WHERE rn.show_id = s.id),
	       0, '', '',
	       EXISTS (
	           SELECT 1 FROM seasons sa
	           JOIN episodes ea ON ea.season_id = sa.id
	           JOIN torrent_assignments a ON a.item_type = 'episode' AND a.item_id = ea.id AND a.is_active
	           WHERE sa.show_id = s.id
	       ),
	       COALESCE(st.files, '[]'::jsonb),
	       mr.season_number, mr.episode_from, mr.episode_to, mr.absolute_from, mr.absolute_to, mr.episodes
	FROM shows s
	LEFT JOIN show_stats st ON st.show_id = s.id
	LEFT JOIN show_sizes sz ON sz.show_id = s.id
	LEFT JOIN missing_runs mr ON mr.show_id = s.id
	WHERE NOT $3::boolean OR s.tmdb_id = ANY($5::int[])
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

// libraryStatusFilters are the whitelisted status conditions.
var libraryStatusFilters = map[string]string{
	"":                       ``,
	LibraryStatusReady:       ` AND stage = 'ready'`,
	LibraryStatusIncomplete:  ` AND stage <> 'ready'`,
	LibraryStatusNeedsReview: ` AND needs_review > 0`,
	LibraryStatusWatching:    ` AND FALSE`, // B5
}

// ListLibrary returns one page of library items, how many match in all, the
// filter counts and the library's summary. Always one query: the counts and
// summary ride on a row of their own, so an empty page (filtered to
// nothing, or past the end) still has them.
func (r *CatalogRepository) ListLibrary(ctx context.Context, q LibraryQuery) (*LibraryListing, error) {
	if q.MediaType != "" && q.MediaType != "movie" && q.MediaType != "tv" {
		return nil, fmt.Errorf("unknown media type %q", q.MediaType)
	}
	if q.Sort == "" {
		q.Sort = LibrarySortAdded
	}
	order, ok := librarySorts[q.Sort]
	if !ok {
		return nil, fmt.Errorf("unknown library sort %q", q.Sort)
	}
	statusFilter, ok := libraryStatusFilters[q.Status]
	if !ok {
		return nil, fmt.Errorf("unknown library status %q", q.Status)
	}

	hashes, paths := splitFileRefs(q.Downloading)
	restrict, movieIDs, showIDs := false, []int32{}, []int32{}
	if q.OnlyTMDB != nil {
		restrict, movieIDs, showIDs = true, int32s(q.OnlyTMDB.Movies), int32s(q.OnlyTMDB.Shows)
	}
	lookup, lookupMovies, lookupShows := false, []int32{}, []int32{}
	if q.IDs != nil {
		lookup, lookupMovies, lookupShows = true, int32s(q.IDs.Movies), int32s(q.IDs.Shows)
	}
	var limit any // NULL: no limit
	if q.Limit > 0 {
		limit = q.Limit
	}
	args := []any{hashes, paths, restrict, movieIDs, showIDs, q.Today,
		q.MediaType, likePattern(q.Search), limit, q.Offset, lookup, lookupMovies, lookupShows}

	// in_scope: the search and IDs, which the counts honour. matches adds
	// the type and status.
	matches := `in_scope AND ($7 = '' OR media_type = $7)` + statusFilter
	rows, err := r.db.QueryContext(ctx, `
		WITH items AS (`+libraryItemsSQL+`),
		scoped AS (
			SELECT items.*,
			       ($8 = '' OR title ILIKE $8 ESCAPE '\')
			       AND (NOT $11::boolean
			            OR (media_type = 'movie' AND tmdb_id = ANY($12::int[]))
			            OR (media_type = 'tv' AND tmdb_id = ANY($13::int[]))) AS in_scope
			FROM items
		),
		stats AS (
			SELECT count(*) FILTER (WHERE `+matches+`) AS total,
			       count(*) FILTER (WHERE in_scope) AS c_all,
			       count(*) FILTER (WHERE in_scope AND media_type = 'movie') AS c_movies,
			       count(*) FILTER (WHERE in_scope AND media_type = 'tv') AS c_shows,
			       count(*) FILTER (WHERE in_scope AND stage = 'ready') AS c_ready,
			       count(*) FILTER (WHERE in_scope AND stage <> 'ready') AS c_incomplete,
			       count(*) FILTER (WHERE in_scope AND needs_review > 0) AS c_needs_review,
			       count(*) AS s_total,
			       COALESCE(sum(size_bytes), 0)::bigint AS s_size,
			       count(*) FILTER (WHERE stage = 'ready') AS s_ready
			FROM scoped
		),
		page AS (
			SELECT scoped.*, row_number() OVER (ORDER BY `+order+`) AS rn
			FROM scoped
			WHERE `+matches+`
			ORDER BY `+order+`
			LIMIT $9 OFFSET $10
		)
		SELECT st.total, st.c_all, st.c_movies, st.c_shows, st.c_ready, st.c_incomplete, st.c_needs_review,
		       st.s_total, st.s_size, st.s_ready,
		       p.media_type, p.id, p.tmdb_id, p.title, p.year, p.poster_path, p.created_at,
		       p.episodes_total, p.episodes_ready, p.size_bytes, p.needs_review, p.stage, p.review_job_id,
		       p.runtime, p.resolution, p.file_path, p.has_file, p.downloading,
		       p.missing_season, p.missing_from, p.missing_to, p.missing_absolute_from, p.missing_absolute_to, p.missing_count
		FROM stats st LEFT JOIN page p ON TRUE
		ORDER BY p.rn
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list library: %w", err)
	}
	defer rows.Close()

	listing := &LibraryListing{Items: []LibraryItem{}}
	for rows.Next() {
		item, err := scanLibraryRow(rows, listing)
		if err != nil {
			return nil, err
		}
		if item != nil {
			listing.Items = append(listing.Items, *item)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read library: %w", err)
	}
	return listing, nil
}

// LibraryItems returns one page of library items and how many match in all
// (ListLibrary without the counts and summary). One query.
func (r *CatalogRepository) LibraryItems(ctx context.Context, q LibraryQuery) ([]LibraryItem, int, error) {
	listing, err := r.ListLibrary(ctx, q)
	if err != nil {
		return nil, 0, err
	}
	return listing.Items, listing.Total, nil
}

// scanLibraryRow reads one row of ListLibrary's query: the totals into
// listing, and the page item, or nil on the lone row of an empty page.
func scanLibraryRow(rows *sql.Rows, listing *LibraryListing) (*LibraryItem, error) {
	var (
		c                               = &listing.Counts
		s                               = &listing.Summary
		mediaType, title, poster, stage sql.NullString
		resolution, filePath            sql.NullString
		id, tmdbID, year, size, review  sql.NullInt64
		episodesTotal, episodesReady    sql.NullInt64
		reviewJob, runtime              sql.NullInt64
		created                         sql.NullTime
		hasFile                         sql.NullBool
		downloading                     []byte
		missingSeason, missingFrom      sql.NullInt64
		missingTo, missingCount         sql.NullInt64
		missingAbsFrom, missingAbsTo    sql.NullInt32
	)
	if err := rows.Scan(&listing.Total, &c.All, &c.Movies, &c.Shows, &c.Ready, &c.Incomplete, &c.NeedsReview,
		&s.Total, &s.SizeBytes, &s.Ready,
		&mediaType, &id, &tmdbID, &title, &year, &poster, &created,
		&episodesTotal, &episodesReady, &size, &review, &stage, &reviewJob,
		&runtime, &resolution, &filePath, &hasFile, &downloading,
		&missingSeason, &missingFrom, &missingTo, &missingAbsFrom, &missingAbsTo, &missingCount); err != nil {
		return nil, fmt.Errorf("failed to scan library item: %w", err)
	}
	if !id.Valid {
		return nil, nil // the page is empty
	}

	item := &LibraryItem{
		MediaType: mediaType.String, ID: id.Int64, TMDBID: int(tmdbID.Int64), Title: title.String,
		Year: int(year.Int64), PosterPath: poster.String, CreatedAt: created.Time,
		SizeBytes: size.Int64, NeedsReview: int(review.Int64), ReviewJobID: nullInt64Ptr(reviewJob), Stage: stage.String,
		Runtime: int(runtime.Int64), Resolution: resolution.String, FilePath: filePath.String, HasFile: hasFile.Bool,
	}
	if episodesTotal.Valid {
		n, m := int(episodesTotal.Int64), int(episodesReady.Int64)
		item.EpisodesTotal, item.EpisodesReady = &n, &m
	}
	var files []struct {
		Hash string `json:"h"`
		Path string `json:"p"`
	}
	if len(downloading) > 0 {
		if err := json.Unmarshal(downloading, &files); err != nil {
			return nil, fmt.Errorf("failed to decode downloading files: %w", err)
		}
	}
	for _, f := range files {
		item.Downloading = append(item.Downloading, FileRef{InfoHash: f.Hash, Path: f.Path})
	}
	if missingSeason.Valid {
		item.Missing = &FrontierRun{
			SeasonNumber: int(missingSeason.Int64), From: int(missingFrom.Int64), To: int(missingTo.Int64),
			AbsoluteFrom: nullIntPtr(missingAbsFrom), AbsoluteTo: nullIntPtr(missingAbsTo), Count: int(missingCount.Int64),
		}
	}
	return item, nil
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
