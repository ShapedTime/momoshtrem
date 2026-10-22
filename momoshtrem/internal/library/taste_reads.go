package library

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"time"
)

// What recommendations (R1, #153) read about your taste: Trakt's history,
// ratings, progress, hidden shows and watchlist, the library's titles, and
// feedback (PLAN §7 R1). Each reader is one query over everything; the
// rules that weigh them live in internal/recommend/profile and candidates,
// and every time boundary is theirs, computed in Go.

// A title's media type, as recommendations name it.
const (
	TitleMovie = "movie"
	TitleTV    = "tv"
)

// TitleRef names a title: TitleMovie or TitleTV, and its TMDB id.
type TitleRef struct {
	MediaType string
	TMDBID    int
}

// TasteRepository reads one household member's taste inputs: their Trakt
// rows only (PLAN §6 "Household members": a title another member watched
// can still be picked), and the library's titles. Bind it to the member
// with ForMember: unbound, the Trakt reads fail.
type TasteRepository struct {
	db     *DB
	member boundMember
}

// NewTasteRepository creates a TasteRepository, bound to no member yet.
func NewTasteRepository(db *DB) *TasteRepository {
	return &TasteRepository{db: db}
}

// ForMember is the repository for member id's taste.
func (r *TasteRepository) ForMember(id int64) *TasteRepository {
	return &TasteRepository{db: r.db, member: boundMember(id)}
}

// knownTitleLateral joins, as kt(title, year), the name the database knows
// a title by: the library's, else its newest play's, Trakt progress's or
// the watchlist's, any member's (only names). mediaType ('movie' or 'tv')
// and tmdbID are SQL expressions; kt's columns are NULL when nothing names
// it.
func knownTitleLateral(mediaType, tmdbID string) string {
	return `LEFT JOIN LATERAL (
		SELECT n.title, n.year FROM (
			SELECT m.title, m.year, 1 AS pri FROM movies m WHERE ` + mediaType + ` = 'movie' AND m.tmdb_id = ` + tmdbID + `
			UNION ALL
			SELECT s.title, s.year, 1 FROM shows s WHERE ` + mediaType + ` = 'tv' AND s.tmdb_id = ` + tmdbID + `
			UNION ALL
			(SELECT h.title, h.year, 2 FROM trakt_history h
			 WHERE h.media_type = CASE WHEN ` + mediaType + ` = 'movie' THEN 'movie' ELSE 'episode' END AND h.tmdb_id = ` + tmdbID + `
			 ORDER BY h.watched_at DESC, h.id DESC LIMIT 1)
			UNION ALL
			(SELECT p.title, p.year, 3 FROM trakt_show_progress p
			 WHERE ` + mediaType + ` = 'tv' AND p.show_tmdb_id = ` + tmdbID + ` ORDER BY p.show_trakt_id LIMIT 1)
			UNION ALL
			(SELECT w.title, w.year, 4 FROM trakt_watchlist w
			 WHERE w.media_type = ` + mediaType + ` AND w.tmdb_id = ` + tmdbID + ` ORDER BY w.trakt_id LIMIT 1)
		) n ORDER BY n.pri LIMIT 1
	) kt ON TRUE`
}

// WatchedMovie is a movie with a play on Trakt.
type WatchedMovie struct {
	TMDBID        int
	Title         string // the newest play's
	Year          int    // 0 when unknown
	Plays         int
	LastWatchedAt time.Time
}

// WatchedMovies lists every movie with a play. One query.
func (r *TasteRepository) WatchedMovies(ctx context.Context) ([]WatchedMovie, error) {
	member, err := r.member.id()
	if err != nil {
		return nil, err
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT wm.tmdb_id, COALESCE(h.title, ''), COALESCE(h.year, 0), wm.plays, wm.last_watched_at
		FROM trakt_watched_movies wm
		LEFT JOIN LATERAL (
			SELECT title, year FROM trakt_history
			WHERE member_id = $1 AND media_type = 'movie' AND tmdb_id = wm.tmdb_id
			ORDER BY watched_at DESC, id DESC LIMIT 1
		) h ON TRUE
		WHERE wm.member_id = $1
		ORDER BY wm.last_watched_at DESC, wm.tmdb_id
	`, member)
	if err != nil {
		return nil, fmt.Errorf("failed to list watched movies: %w", err)
	}
	defer rows.Close()

	var out []WatchedMovie
	for rows.Next() {
		var m WatchedMovie
		if err := rows.Scan(&m.TMDBID, &m.Title, &m.Year, &m.Plays, &m.LastWatchedAt); err != nil {
			return nil, fmt.Errorf("failed to scan watched movie: %w", err)
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read watched movies: %w", err)
	}
	return out, nil
}

// WatchedShow is a show you started: one with a Trakt progress row, plays,
// or both.
type WatchedShow struct {
	TMDBID int
	Title  string // progress's, else the newest play's
	Year   int    // 0 when unknown
	// Plays counts every episode play; EpisodesWatched the regular
	// episodes with one.
	Plays           int
	EpisodesWatched int
	// Aired and Completed are Trakt's progress (regular episodes), 0
	// without a progress row (HasProgress false).
	Aired, Completed int
	HasProgress      bool
	// LastWatchedAt is the newest play or progress's, nil when neither
	// says.
	LastWatchedAt *time.Time
}

// WatchedShows lists every show you started. One query.
func (r *TasteRepository) WatchedShows(ctx context.Context) ([]WatchedShow, error) {
	member, err := r.member.id()
	if err != nil {
		return nil, err
	}
	rows, err := r.db.QueryContext(ctx, `
		WITH eps AS (
			SELECT show_tmdb_id, sum(plays)::int AS plays, count(*) FILTER (WHERE season > 0)::int AS watched,
			       max(last_watched_at) AS last_watched_at
			FROM trakt_watched_episodes WHERE member_id = $1 GROUP BY show_tmdb_id
		),
		prog AS (
			SELECT DISTINCT ON (show_tmdb_id) show_tmdb_id, title, year, aired, completed, last_watched_at
			FROM trakt_show_progress WHERE member_id = $1
			ORDER BY show_tmdb_id, last_watched_at DESC NULLS LAST, show_trakt_id
		)
		SELECT COALESCE(p.show_tmdb_id, e.show_tmdb_id), COALESCE(p.title, h.title, ''), COALESCE(p.year, h.year, 0),
		       COALESCE(e.plays, 0), COALESCE(e.watched, 0), COALESCE(p.aired, 0), COALESCE(p.completed, 0),
		       p.show_tmdb_id IS NOT NULL, GREATEST(e.last_watched_at, p.last_watched_at)
		FROM prog p
		FULL JOIN eps e ON e.show_tmdb_id = p.show_tmdb_id
		LEFT JOIN LATERAL (
			SELECT title, year FROM trakt_history
			WHERE p.show_tmdb_id IS NULL AND member_id = $1 AND media_type = 'episode' AND tmdb_id = e.show_tmdb_id
			ORDER BY watched_at DESC, id DESC LIMIT 1
		) h ON TRUE
		ORDER BY 9 DESC NULLS LAST, 1
	`, member)
	if err != nil {
		return nil, fmt.Errorf("failed to list watched shows: %w", err)
	}
	defer rows.Close()

	var out []WatchedShow
	for rows.Next() {
		var s WatchedShow
		var last sql.NullTime
		if err := rows.Scan(&s.TMDBID, &s.Title, &s.Year, &s.Plays, &s.EpisodesWatched, &s.Aired, &s.Completed,
			&s.HasProgress, &last); err != nil {
			return nil, fmt.Errorf("failed to scan watched show: %w", err)
		}
		s.LastWatchedAt = nullTimePtr(last)
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read watched shows: %w", err)
	}
	return out, nil
}

// TasteRating is a movie's or a show's rating on Trakt (1–10).
type TasteRating struct {
	TitleRef
	Title string // "" and 0 when nothing names it
	Year  int
	// Rating is the title's own; a show without one gets the mean of its
	// season and episode ratings, rounded (FromEpisodes).
	Rating       int
	FromEpisodes bool
	RatedAt      time.Time // the newest of them
}

// TasteRatings lists every rated movie and show, with the name the
// database knows it by. One query.
func (r *TasteRepository) TasteRatings(ctx context.Context) ([]TasteRating, error) {
	member, err := r.member.id()
	if err != nil {
		return nil, err
	}
	rows, err := r.db.QueryContext(ctx, `
		WITH own AS (
			SELECT DISTINCT ON (r.media_type, r.tmdb_id)
			       CASE r.media_type WHEN 'movie' THEN 'movie' ELSE 'tv' END AS media_type, r.tmdb_id,
			       r.rating::float8 AS rating, r.rated_at, FALSE AS from_episodes
			FROM trakt_ratings r
			WHERE r.member_id = $1 AND r.media_type IN ('movie', 'show') AND r.tmdb_id IS NOT NULL
			ORDER BY r.media_type, r.tmdb_id, r.rated_at DESC
		),
		parts AS (
			SELECT 'tv' AS media_type, r.show_tmdb_id AS tmdb_id, avg(r.rating)::float8 AS rating,
			       max(r.rated_at) AS rated_at, TRUE AS from_episodes
			FROM trakt_ratings r
			WHERE r.member_id = $1 AND r.media_type IN ('season', 'episode') AND r.show_tmdb_id IS NOT NULL
			  AND NOT EXISTS (SELECT 1 FROM own o WHERE o.media_type = 'tv' AND o.tmdb_id = r.show_tmdb_id)
			GROUP BY r.show_tmdb_id
		),
		rated AS (SELECT * FROM own UNION ALL SELECT * FROM parts)
		SELECT x.media_type, x.tmdb_id, COALESCE(kt.title, ''), COALESCE(kt.year, 0), x.rating, x.from_episodes, x.rated_at
		FROM rated x
		`+knownTitleLateral("x.media_type", "x.tmdb_id")+`
		ORDER BY x.rated_at DESC, x.media_type, x.tmdb_id
	`, member)
	if err != nil {
		return nil, fmt.Errorf("failed to list ratings: %w", err)
	}
	defer rows.Close()

	var out []TasteRating
	for rows.Next() {
		var t TasteRating
		var rating float64
		if err := rows.Scan(&t.MediaType, &t.TMDBID, &t.Title, &t.Year, &rating, &t.FromEpisodes, &t.RatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan rating: %w", err)
		}
		t.Rating = int(math.Round(rating))
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read ratings: %w", err)
	}
	return out, nil
}

// HiddenShow is a show dropped, or hidden from progress, on Trakt.
type HiddenShow struct {
	TMDBID       int
	Dropped      bool // trakt_hidden section dropped
	FromProgress bool // section progress_watched
}

// HiddenShows lists the shows dropped or hidden on Trakt. One query.
func (r *TasteRepository) HiddenShows(ctx context.Context) ([]HiddenShow, error) {
	member, err := r.member.id()
	if err != nil {
		return nil, err
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT tmdb_id, bool_or(section = 'dropped'), bool_or(section = 'progress_watched')
		FROM trakt_hidden WHERE member_id = $1 GROUP BY tmdb_id ORDER BY tmdb_id
	`, member)
	if err != nil {
		return nil, fmt.Errorf("failed to list hidden shows: %w", err)
	}
	defer rows.Close()

	var out []HiddenShow
	for rows.Next() {
		var h HiddenShow
		if err := rows.Scan(&h.TMDBID, &h.Dropped, &h.FromProgress); err != nil {
			return nil, fmt.Errorf("failed to scan hidden show: %w", err)
		}
		out = append(out, h)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read hidden shows: %w", err)
	}
	return out, nil
}

// WatchlistRefs lists the titles on the Trakt watchlist, newest listed
// first. One query.
func (r *TasteRepository) WatchlistRefs(ctx context.Context) ([]TitleRef, error) {
	member, err := r.member.id()
	if err != nil {
		return nil, err
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT media_type, tmdb_id FROM trakt_watchlist WHERE member_id = $1 ORDER BY listed_at DESC, media_type, trakt_id
	`, member)
	if err != nil {
		return nil, fmt.Errorf("failed to list the watchlist: %w", err)
	}
	defer rows.Close()

	var out []TitleRef
	for rows.Next() {
		var ref TitleRef
		if err := rows.Scan(&ref.MediaType, &ref.TMDBID); err != nil {
			return nil, fmt.Errorf("failed to scan watchlist title: %w", err)
		}
		out = append(out, ref)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read the watchlist: %w", err)
	}
	return out, nil
}

// LibraryTitle is a movie or show in the library with what TMDB said about
// it when it was added or refreshed.
type LibraryTitle struct {
	TitleRef
	Title  string
	Year   int
	Genres []string
	// OriginalLanguage, OriginCountry and IsAnime are shows' only; the
	// library doesn't store them for movies.
	OriginalLanguage string
	OriginCountry    []string
	IsAnime          bool
	// Runtime is a movie's, or a show's mean regular episode runtime, in
	// minutes; 0 when unknown.
	Runtime int
}

// LibraryTitles lists every movie and show in the library. One query.
func (r *TasteRepository) LibraryTitles(ctx context.Context) ([]LibraryTitle, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT 'movie', m.tmdb_id, m.title, m.year, COALESCE(m.genres, '[]'::jsonb), '', '[]'::jsonb, FALSE,
		       COALESCE(m.runtime, 0)
		FROM movies m
		UNION ALL
		SELECT 'tv', s.tmdb_id, s.title, s.year, COALESCE(s.genres, '[]'::jsonb), COALESCE(s.original_language, ''),
		       COALESCE(to_jsonb(s.origin_country), '[]'::jsonb), s.is_anime,
		       COALESCE((SELECT round(avg(e.runtime))::int FROM seasons sn JOIN episodes e ON e.season_id = sn.id
		                 WHERE sn.show_id = s.id AND sn.season_number > 0 AND e.runtime > 0), 0)
		FROM shows s
		ORDER BY 1, 2
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to list library titles: %w", err)
	}
	defer rows.Close()

	var out []LibraryTitle
	for rows.Next() {
		var t LibraryTitle
		var genres, countries []byte
		if err := rows.Scan(&t.MediaType, &t.TMDBID, &t.Title, &t.Year, &genres, &t.OriginalLanguage, &countries,
			&t.IsAnime, &t.Runtime); err != nil {
			return nil, fmt.Errorf("failed to scan library title: %w", err)
		}
		if t.Genres, err = decodeStrings(genres); err != nil {
			return nil, fmt.Errorf("%s %d genres: %w", t.MediaType, t.TMDBID, err)
		}
		if t.OriginCountry, err = decodeStrings(countries); err != nil {
			return nil, fmt.Errorf("%s %d origin_country: %w", t.MediaType, t.TMDBID, err)
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read library titles: %w", err)
	}
	return out, nil
}

// TasteFeedback is feedback on a title, with the name the database knows
// it by ("" and 0 when nothing names it: a title only ever seen on a
// shelf).
type TasteFeedback struct {
	TitleRef
	Signal    string
	Title     string
	Year      int
	UpdatedAt time.Time
}

// TasteFeedback lists every title's feedback, newest first. One query.
func (r *TasteRepository) TasteFeedback(ctx context.Context) ([]TasteFeedback, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT f.media_type, f.tmdb_id, f.signal, COALESCE(kt.title, ''), COALESCE(kt.year, 0), f.updated_at
		FROM rec_feedback f
		`+knownTitleLateral("f.media_type", "f.tmdb_id")+`
		ORDER BY f.updated_at DESC, f.media_type, f.tmdb_id
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to list feedback: %w", err)
	}
	defer rows.Close()

	var out []TasteFeedback
	for rows.Next() {
		var f TasteFeedback
		if err := rows.Scan(&f.MediaType, &f.TMDBID, &f.Signal, &f.Title, &f.Year, &f.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan feedback: %w", err)
		}
		out = append(out, f)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read feedback: %w", err)
	}
	return out, nil
}

// TitleState is what's known about one title: whether you watched it,
// your rating and feedback, and whether it's hidden on Trakt, on the
// watchlist or in the library (check_seen, and R2's validation of a run's
// items).
type TitleState struct {
	TitleRef
	// Plays: a movie's, or every play of a show's episodes. Started is a
	// show's Trakt progress row or plays (a movie: a play).
	Plays   int
	Started bool
	// Aired and Completed are a show's Trakt progress (regular episodes).
	Aired, Completed int
	LastWatchedAt    *time.Time
	Rating           *int   // the title's own Trakt rating
	Dropped          bool   // a show dropped on Trakt
	Hidden           bool   // a show hidden from progress on Trakt
	Feedback         string // "" without
	OnWatchlist      bool
	InLibrary        bool
}

// TitleStates looks up refs, in their order. One query.
func (r *TasteRepository) TitleStates(ctx context.Context, refs []TitleRef) ([]TitleState, error) {
	if len(refs) == 0 {
		return nil, nil
	}
	member, err := r.member.id()
	if err != nil {
		return nil, err
	}
	types, ids := make([]string, len(refs)), make([]int32, len(refs))
	for i, ref := range refs {
		if ref.TMDBID <= 0 || ref.TMDBID > math.MaxInt32 {
			return nil, fmt.Errorf("TMDB id %d is out of range", ref.TMDBID)
		}
		types[i], ids[i] = ref.MediaType, int32(ref.TMDBID)
	}
	rows, err := r.db.QueryContext(ctx, `
		WITH refs AS (
			SELECT k.n, k.media_type, k.tmdb_id
			FROM unnest($1::text[], $2::int[]) WITH ORDINALITY AS k(media_type, tmdb_id, n)
		)
		SELECT k.n,
		       CASE WHEN k.media_type = 'movie' THEN COALESCE(wm.plays, 0) ELSE COALESCE(we.plays, 0) END,
		       CASE WHEN k.media_type = 'movie' THEN wm.tmdb_id IS NOT NULL
		            ELSE p.show_tmdb_id IS NOT NULL OR COALESCE(we.plays, 0) > 0 END,
		       COALESCE(p.aired, 0), COALESCE(p.completed, 0),
		       CASE WHEN k.media_type = 'movie' THEN wm.last_watched_at ELSE GREATEST(we.last_watched_at, p.last_watched_at) END,
		       (SELECT r.rating FROM trakt_ratings r
		        WHERE r.member_id = $3 AND r.media_type = CASE WHEN k.media_type = 'movie' THEN 'movie' ELSE 'show' END
		          AND r.tmdb_id = k.tmdb_id
		        ORDER BY r.rated_at DESC LIMIT 1),
		       k.media_type = 'tv' AND EXISTS (SELECT 1 FROM trakt_hidden h
		                                       WHERE h.member_id = $3 AND h.tmdb_id = k.tmdb_id AND h.section = 'dropped'),
		       k.media_type = 'tv' AND EXISTS (SELECT 1 FROM trakt_hidden h
		                                       WHERE h.member_id = $3 AND h.tmdb_id = k.tmdb_id AND h.section = 'progress_watched'),
		       COALESCE(f.signal, ''),
		       EXISTS (SELECT 1 FROM trakt_watchlist w WHERE w.member_id = $3 AND w.media_type = k.media_type AND w.tmdb_id = k.tmdb_id),
		       CASE WHEN k.media_type = 'movie' THEN EXISTS (SELECT 1 FROM movies m WHERE m.tmdb_id = k.tmdb_id)
		            ELSE EXISTS (SELECT 1 FROM shows s WHERE s.tmdb_id = k.tmdb_id) END
		FROM refs k
		LEFT JOIN trakt_watched_movies wm ON k.media_type = 'movie' AND wm.tmdb_id = k.tmdb_id AND wm.member_id = $3
		LEFT JOIN LATERAL (
			SELECT sum(w.plays)::int AS plays, max(w.last_watched_at) AS last_watched_at
			FROM trakt_watched_episodes w WHERE k.media_type = 'tv' AND w.show_tmdb_id = k.tmdb_id AND w.member_id = $3
		) we ON TRUE
		LEFT JOIN LATERAL (
			SELECT pr.show_tmdb_id, pr.aired, pr.completed, pr.last_watched_at FROM trakt_show_progress pr
			WHERE k.media_type = 'tv' AND pr.member_id = $3 AND pr.show_tmdb_id = k.tmdb_id
			ORDER BY pr.last_watched_at DESC NULLS LAST, pr.show_trakt_id LIMIT 1
		) p ON TRUE
		LEFT JOIN rec_feedback f ON f.media_type = k.media_type AND f.tmdb_id = k.tmdb_id
		ORDER BY k.n
	`, types, ids, member)
	if err != nil {
		return nil, fmt.Errorf("failed to look up titles: %w", err)
	}
	defer rows.Close()

	out := make([]TitleState, 0, len(refs))
	for rows.Next() {
		var (
			n      int
			s      TitleState
			last   sql.NullTime
			rating sql.NullInt32
		)
		if err := rows.Scan(&n, &s.Plays, &s.Started, &s.Aired, &s.Completed, &last, &rating, &s.Dropped, &s.Hidden,
			&s.Feedback, &s.OnWatchlist, &s.InLibrary); err != nil {
			return nil, fmt.Errorf("failed to scan title state: %w", err)
		}
		s.TitleRef = refs[n-1]
		s.LastWatchedAt = nullTimePtr(last)
		s.Rating = nullIntPtr(rating)
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read title states: %w", err)
	}
	return out, nil
}
