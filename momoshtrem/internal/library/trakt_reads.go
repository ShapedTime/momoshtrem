package library

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// What the title pages, /library and /me read from the Trakt tables (PLAN
// §6 "Trakt (B5)", "Watch state rules"). Only the sync writes them, and a
// disconnect empties them, so without Trakt every read finds nothing.

// episodeCols names an episode's TMDB episode id, season and number in SQL.
type episodeCols struct{ tmdbID, season, number string }

// traktEpisode is a Trakt table's episode, as alias.episode_tmdb_id,
// alias.season and alias.episode.
func traktEpisode(alias string) episodeCols {
	return episodeCols{alias + ".episode_tmdb_id", alias + ".season", alias + ".episode"}
}

// libraryEpisode is a library episode, e joined to its season sn.
var libraryEpisode = episodeCols{"e.tmdb_episode_id", "sn.season_number", "e.episode_number"}

// The matching rule (PLAN §6 "Watch state rules"): a Trakt episode is a
// library episode by TMDB's episode id when both have one, else by season
// and number, so an episode TMDB numbers differently never borrows
// another's watched state. sameEpisodeByID and sameEpisodeByNumber are its
// two halves; every query that matches the two goes through sameEpisode or
// episodeWatchedSQL.

func sameEpisodeByID(a, b episodeCols) string {
	return a.tmdbID + ` = ` + b.tmdbID
}

func sameEpisodeByNumber(a, b episodeCols) string {
	return `(` + a.tmdbID + ` IS NULL OR ` + b.tmdbID + ` IS NULL) AND ` + a.season + ` = ` + b.season +
		` AND ` + a.number + ` = ` + b.number
}

// sameEpisode is the matching rule as one condition.
func sameEpisode(a, b episodeCols) string {
	return `(` + sameEpisodeByID(a, b) + ` OR (` + sameEpisodeByNumber(a, b) + `))`
}

// episodeWatchedSQL is true when Trakt has a play of a library episode of
// the show with TMDB id showTMDB. It's sameEpisode as two lookups: as one
// OR, Postgres reads every play of the show for each episode. plays is a
// condition on w that doesn't refer to the outer query and keeps the plays
// of the shows it covers (TRUE: every show), so when Postgres hashes the
// plays it reads only those.
func episodeWatchedSQL(showTMDB string, lib episodeCols, plays string) string {
	w := traktEpisode("w")
	from := `SELECT 1 FROM trakt_watched_episodes w WHERE (` + plays + `) AND w.show_tmdb_id = ` + showTMDB
	return `(EXISTS (` + from + ` AND ` + sameEpisodeByID(w, lib) + `)
		OR EXISTS (` + from + ` AND ` + sameEpisodeByNumber(w, lib) + `))`
}

// libraryEpisodeLateral joins, as le(episode_id, season_number), the
// episode of the library show showID (an SQL expression) that the matching
// rule finds for the Trakt episode t, an id match first. No row when the
// show doesn't list it.
func libraryEpisodeLateral(showID string, t episodeCols) string {
	return `LEFT JOIN LATERAL (
			SELECT e.id AS episode_id, sn.season_number
			FROM seasons sn JOIN episodes e ON e.season_id = sn.id
			WHERE sn.show_id = ` + showID + ` AND ` + sameEpisode(t, libraryEpisode) + `
			ORDER BY ` + sameEpisodeByID(t, libraryEpisode) + ` DESC NULLS LAST, sn.season_number, e.episode_number
			LIMIT 1
		) le ON TRUE`
}

// traktNextCTE is trakt_next: the Trakt progress rows that match scope (on
// p), each with the episode Up next names for it (PLAN §6 "Trakt (B5)"):
// Trakt's next episode, else, caught up, the first regular calendar entry
// after the last watched one. Specials are never next. The episode's
// columns are NULL when there's none; the calendar fills in what progress
// lacks, and first_aired is the calendar's.
func traktNextCTE(scope string) string {
	return `trakt_next AS (
		SELECT p.show_trakt_id, p.show_tmdb_id, p.title, p.poster_path, p.backdrop_path,
		       p.completed, p.aired, p.last_watched_at,
		       n.from_calendar, n.season, n.episode, n.episode_tmdb_id, n.episode_title, n.still_path, n.first_aired
		FROM trakt_show_progress p
		LEFT JOIN LATERAL (
			SELECT FALSE AS from_calendar, p.next_season AS season, p.next_episode AS episode,
			       COALESCE(p.next_episode_tmdb_id, c.episode_tmdb_id) AS episode_tmdb_id,
			       COALESCE(p.next_title, c.episode_title) AS episode_title,
			       COALESCE(p.next_still_path, c.still_path) AS still_path, c.first_aired
			FROM (VALUES (1)) one(x)
			LEFT JOIN trakt_calendar c
			       ON c.show_trakt_id = p.show_trakt_id AND c.season = p.next_season AND c.episode = p.next_episode
			WHERE p.next_season > 0 AND p.next_episode IS NOT NULL
			UNION ALL
			SELECT TRUE, c.season, c.episode, c.episode_tmdb_id, c.episode_title, c.still_path, c.first_aired
			FROM trakt_calendar c
			WHERE c.show_trakt_id = p.show_trakt_id AND c.season > 0
			  AND (c.season, c.episode) > (COALESCE(p.last_season, 0), COALESCE(p.last_episode, 0))
			ORDER BY from_calendar, season, episode
			LIMIT 1
		) n ON TRUE
		WHERE ` + scope + `
	)`
}

// TraktWatchState is what Trakt imported about one title; nil when it
// knows nothing (no play, rating or progress).
type TraktWatchState struct {
	Plays         int        // a show: the sum of its episodes' plays
	LastWatchedAt *time.Time // a show: its last watched episode
	Rating        *int
	// Shows only. Progress is Trakt's (regular episodes), nil without it;
	// Next is the episode Up next names, nil when there's none.
	Progress *TraktProgress
	Next     *TraktNext
}

// TraktProgress is a show's regular episodes watched of aired.
type TraktProgress struct {
	Completed, Aired int
}

// TraktNext is a show's next episode, in the library's numbering and with
// its absolute number, name and air date when the library lists it, else
// as Trakt names it.
type TraktNext struct {
	Season, Episode int
	AbsoluteNumber  *int
	Name            string
	AirDate         string // YYYY-MM-DD; "" when unknown
}

// WatchState returns what Trakt imported about a title: mediaType "movie"
// or "tv", by TMDB id. Nil when Trakt knows nothing about it. One query.
func (r *CatalogRepository) WatchState(ctx context.Context, mediaType string, tmdbID int) (*TraktWatchState, error) {
	switch mediaType {
	case string(JobMovie):
		return r.movieWatchState(ctx, tmdbID)
	case string(JobTV):
		return r.showWatchState(ctx, tmdbID)
	}
	return nil, fmt.Errorf("unknown media type %q", mediaType)
}

func (r *CatalogRepository) movieWatchState(ctx context.Context, tmdbID int) (*TraktWatchState, error) {
	var plays, rating sql.NullInt32
	var last sql.NullTime
	err := r.db.QueryRowContext(ctx, `
		SELECT wm.plays, wm.last_watched_at,
		       (SELECT r.rating FROM trakt_ratings r WHERE r.media_type = 'movie' AND r.tmdb_id = $1 ORDER BY r.rated_at DESC LIMIT 1)
		FROM (VALUES (1)) one(x) LEFT JOIN trakt_watched_movies wm ON wm.tmdb_id = $1
	`, tmdbID).Scan(&plays, &last, &rating)
	if err != nil {
		return nil, fmt.Errorf("failed to read the movie's watch state: %w", err)
	}
	if !plays.Valid && !rating.Valid {
		return nil, nil
	}
	return &TraktWatchState{Plays: int(plays.Int32), LastWatchedAt: nullTimePtr(last), Rating: nullIntPtr(rating)}, nil
}

func (r *CatalogRepository) showWatchState(ctx context.Context, tmdbID int) (*TraktWatchState, error) {
	var (
		plays, rating, completed, aired   sql.NullInt32
		season, episode, libSeason, libEp sql.NullInt32
		absolute                          sql.NullInt32
		last, firstAired                  sql.NullTime
		traktName, libName, libAirDate    sql.NullString
	)
	err := r.db.QueryRowContext(ctx, `
		WITH `+traktNextCTE(`p.show_tmdb_id = $1`)+`,
		progress AS (SELECT * FROM trakt_next ORDER BY last_watched_at DESC NULLS LAST LIMIT 1)
		SELECT (SELECT sum(w.plays) FROM trakt_watched_episodes w WHERE w.show_tmdb_id = $1)::int,
		       COALESCE((SELECT max(w.last_watched_at) FROM trakt_watched_episodes w WHERE w.show_tmdb_id = $1), n.last_watched_at),
		       (SELECT r.rating FROM trakt_ratings r WHERE r.media_type = 'show' AND r.tmdb_id = $1 ORDER BY r.rated_at DESC LIMIT 1),
		       n.completed, n.aired, n.season, n.episode, n.episode_title, n.first_aired,
		       le.season_number, e.episode_number, e.absolute_number, e.name, e.air_date
		FROM (VALUES (1)) one(x)
		LEFT JOIN progress n ON TRUE
		LEFT JOIN shows s ON s.tmdb_id = $1
		`+libraryEpisodeLateral("s.id", traktEpisode("n"))+`
		LEFT JOIN episodes e ON e.id = le.episode_id
	`, tmdbID).Scan(&plays, &last, &rating, &completed, &aired, &season, &episode, &traktName, &firstAired,
		&libSeason, &libEp, &absolute, &libName, &libAirDate)
	if err != nil {
		return nil, fmt.Errorf("failed to read the show's watch state: %w", err)
	}
	if !plays.Valid && !rating.Valid && !completed.Valid {
		return nil, nil
	}
	ws := &TraktWatchState{Plays: int(plays.Int32), LastWatchedAt: nullTimePtr(last), Rating: nullIntPtr(rating)}
	if completed.Valid {
		ws.Progress = &TraktProgress{Completed: int(completed.Int32), Aired: int(aired.Int32)}
	}
	switch {
	case libSeason.Valid:
		ws.Next = &TraktNext{Season: int(libSeason.Int32), Episode: int(libEp.Int32), AbsoluteNumber: nullIntPtr(absolute),
			Name: libName.String, AirDate: libAirDate.String}
		if ws.Next.Name == "" {
			ws.Next.Name = traktName.String
		}
		if ws.Next.AirDate == "" {
			ws.Next.AirDate = utcDate(firstAired)
		}
	case season.Valid:
		ws.Next = &TraktNext{Season: int(season.Int32), Episode: int(episode.Int32), Name: traktName.String, AirDate: utcDate(firstAired)}
	}
	return ws, nil
}

// TraktEpisodeKey names an episode as Trakt does: the show's TMDB id,
// Trakt's season and number, and TMDB's episode id (0 when unknown).
type TraktEpisodeKey struct {
	ShowTMDBID    int
	Season        int
	Episode       int
	EpisodeTMDBID int
}

// KeyedEpisode is what the library knows about a TraktEpisodeKey.
type KeyedEpisode struct {
	Key TraktEpisodeKey
	// Show has its ID, TMDB ID, title, year (its WebDAV folder) and
	// artwork; nil when the show isn't in the library.
	Show *Show
	// SeasonNumber and Episode are the library episode the matching rule
	// finds, with its active Assignment; Episode is nil when the library
	// doesn't list it.
	SeasonNumber int
	Episode      *Episode
	// FileMates are the season's other episodes whose file is the same file
	// of the same torrent, with their Assignment's kind and match source:
	// the VFS may name them one entry (vfs.SeasonEntries).
	FileMates []Episode
	// ReviewJobID is the newest open mapping job with a file under review
	// that targets the episode (PLAN §6 "Review rules"); nil without one.
	ReviewJobID *int64
}

// EpisodesByKeys looks up what the library knows about Trakt's episodes,
// in the order of keys: the show, the episode the matching rule finds, its
// file and the mapping review. One query.
func (r *CatalogRepository) EpisodesByKeys(ctx context.Context, keys []TraktEpisodeKey) ([]KeyedEpisode, error) {
	if len(keys) == 0 {
		return nil, nil
	}
	shows, seasons, episodes, ids := make([]int32, len(keys)), make([]int32, len(keys)), make([]int32, len(keys)), make([]int32, len(keys))
	for i, k := range keys {
		shows[i], seasons[i], episodes[i], ids[i] = int32(k.ShowTMDBID), int32(k.Season), int32(k.Episode), int32(k.EpisodeTMDBID)
	}
	rows, err := r.db.QueryContext(ctx, `
		WITH keys AS (
			SELECT k.n, k.show_tmdb_id, k.season, k.episode, NULLIF(k.episode_tmdb_id, 0) AS episode_tmdb_id
			FROM unnest($1::int[], $2::int[], $3::int[], $4::int[]) WITH ORDINALITY AS k(show_tmdb_id, season, episode, episode_tmdb_id, n)
		),
		`+reviewCTEs(`j.show_id IN (SELECT s.id FROM shows s WHERE s.tmdb_id = ANY($1::int[]))`)+`,
		review_episodes AS (
			SELECT c.episode_id, max(rf.job_id) AS job_id
			FROM review_files rf CROSS JOIN LATERAL unnest(rf.episode_ids) AS c(episode_id)
			GROUP BY c.episode_id
		)
		SELECT k.n, s.id, s.title, s.year, COALESCE(s.poster_path, ''), COALESCE(s.backdrop_path, ''),
		       COALESCE(le.season_number, 0), `+episodeColumns+`, `+assignmentColumns+`, re.job_id, mates.list
		FROM keys k
		LEFT JOIN shows s ON s.tmdb_id = k.show_tmdb_id
		`+libraryEpisodeLateral("s.id", traktEpisode("k"))+`
		LEFT JOIN episodes e ON e.id = le.episode_id
		`+activeAssignmentLateral("episode", "e.id")+`
		LEFT JOIN review_episodes re ON re.episode_id = e.id
		LEFT JOIN LATERAL (
			SELECT jsonb_agg(jsonb_build_object('id', e2.id, 'number', e2.episode_number, 'name', COALESCE(e2.name, ''),
			                                    'kind', t2.kind, 'match_source', COALESCE(t2.match_source, ''))
			                 ORDER BY e2.episode_number) AS list
			FROM episodes e2
			JOIN torrent_assignments t2 ON t2.item_type = 'episode' AND t2.item_id = e2.id AND t2.is_active
			WHERE e2.season_id = e.season_id AND e2.id <> e.id AND t2.info_hash = ta.info_hash AND t2.file_path = ta.file_path
		) mates ON TRUE
		ORDER BY k.n
	`, shows, seasons, episodes, ids)
	if err != nil {
		return nil, fmt.Errorf("failed to look up Trakt episodes: %w", err)
	}
	defer rows.Close()

	out := make([]KeyedEpisode, 0, len(keys))
	for rows.Next() {
		var (
			n                int
			showID, showYear sql.NullInt64
			showTitle        sql.NullString
			poster, backdrop string
			ke               KeyedEpisode
			ep               Episode
			reviewJob        sql.NullInt64
			mates            []byte
		)
		epDest, epFinish := episodeScanDest(&ep)
		taDest, taFinish := assignmentDest(ItemTypeEpisode)
		dest := append([]any{&n, &showID, &showTitle, &showYear, &poster, &backdrop, &ke.SeasonNumber}, epDest...)
		dest = append(append(dest, taDest...), &reviewJob, &mates)
		if err := rows.Scan(dest...); err != nil {
			return nil, fmt.Errorf("failed to scan Trakt episode: %w", err)
		}
		ke.Key = keys[n-1]
		if showID.Valid {
			ke.Show = &Show{ID: showID.Int64, TMDBID: ke.Key.ShowTMDBID, Title: showTitle.String, Year: int(showYear.Int64),
				PosterPath: poster, BackdropPath: backdrop}
		}
		if epFinish() {
			ep.Assignment = taFinish(ep.ID)
			ke.Episode = &ep
			ke.ReviewJobID = nullInt64Ptr(reviewJob)
			if ke.FileMates, err = decodeFileMates(mates, ep.Assignment); err != nil {
				return nil, err
			}
		}
		out = append(out, ke)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read Trakt episodes: %w", err)
	}
	return out, nil
}

// decodeFileMates reads EpisodesByKeys' mates: episodes playing a's file.
func decodeFileMates(raw []byte, a *TorrentAssignment) ([]Episode, error) {
	if len(raw) == 0 || a == nil {
		return nil, nil
	}
	var list []struct {
		ID          int64  `json:"id"`
		Number      int    `json:"number"`
		Name        string `json:"name"`
		Kind        string `json:"kind"`
		MatchSource string `json:"match_source"`
	}
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, fmt.Errorf("failed to decode file mates: %w", err)
	}
	mates := make([]Episode, 0, len(list))
	for _, m := range list {
		mates = append(mates, Episode{ID: m.ID, EpisodeNumber: m.Number, Name: m.Name, Assignment: &TorrentAssignment{
			ItemType: ItemTypeEpisode, ItemID: m.ID, Kind: AssignmentKind(m.Kind), InfoHash: a.InfoHash, FilePath: a.FilePath,
			MatchSource: m.MatchSource, IsActive: true,
		}})
	}
	return mates, nil
}

// Up next's sources.
const (
	UpNextFromProgress  = "progress"
	UpNextFromWatchlist = "watchlist"
)

// UpNextCandidate is a show Up next may list (PLAN §6 "Trakt (B5)"): a
// Trakt progress row, or a watchlisted library show without plays.
type UpNextCandidate struct {
	Source       string // UpNextFromProgress or UpNextFromWatchlist
	ShowTMDBID   int
	Title        string // Trakt's
	PosterPath   string
	BackdropPath string
	// Completed and Aired are Trakt's progress (regular episodes); a
	// watchlist start's are 0 and the library's aired regular episodes.
	Completed, Aired int
	LastWatchedAt    *time.Time // progress; nil for a watchlist start
	ListedAt         *time.Time // a watchlist start's
	Hidden           bool       // dropped, or hidden from progress, on Trakt
	// Next is the episode it names: traktNextCTE's for progress (nil when
	// there's none), S01E01 for a watchlist start.
	Next *TraktNextEpisode
}

// TraktNextEpisode is the episode an Up next candidate names, as Trakt
// knows it.
type TraktNextEpisode struct {
	TraktEpisodeKey
	Title     string
	StillPath string
	// FirstAired is the calendar's, when it lists the episode.
	FirstAired *time.Time
	// FromCalendar: the show is caught up, and the calendar names it.
	FromCalendar bool
}

// UpNextCandidates lists Up next's candidates: every Trakt progress row
// with its next episode, and the watchlisted library shows nobody has
// started, with their aired regular episodes as of today (YYYY-MM-DD). One
// query.
func (r *CatalogRepository) UpNextCandidates(ctx context.Context, today string) ([]UpNextCandidate, error) {
	rows, err := r.db.QueryContext(ctx, `
		WITH `+traktNextCTE(`TRUE`)+`
		SELECT 'progress', n.show_tmdb_id, n.title, COALESCE(n.poster_path, ''), COALESCE(n.backdrop_path, ''),
		       n.completed, n.aired, n.last_watched_at, NULL::timestamptz,
		       EXISTS (SELECT 1 FROM trakt_hidden h WHERE h.tmdb_id = n.show_tmdb_id),
		       n.season, n.episode, n.episode_tmdb_id, COALESCE(n.episode_title, ''), COALESCE(n.still_path, ''),
		       n.first_aired, COALESCE(n.from_calendar, FALSE)
		FROM trakt_next n
		UNION ALL
		SELECT 'watchlist', w.tmdb_id, w.title, COALESCE(w.poster_path, ''), COALESCE(w.backdrop_path, ''),
		       0, (SELECT count(*) FROM seasons sn JOIN episodes e ON e.season_id = sn.id
		           WHERE sn.show_id = s.id AND sn.season_number > 0 AND e.air_date <= $1)::int,
		       NULL, w.listed_at,
		       EXISTS (SELECT 1 FROM trakt_hidden h WHERE h.tmdb_id = w.tmdb_id),
		       1, 1, NULL, '', '', NULL, FALSE
		FROM trakt_watchlist w JOIN shows s ON s.tmdb_id = w.tmdb_id
		WHERE w.media_type = 'tv' AND NOT EXISTS (SELECT 1 FROM trakt_watched_episodes we WHERE we.show_tmdb_id = w.tmdb_id)
	`, today)
	if err != nil {
		return nil, fmt.Errorf("failed to list Up next candidates: %w", err)
	}
	defer rows.Close()

	var out []UpNextCandidate
	for rows.Next() {
		var (
			c                        UpNextCandidate
			last, listed, firstAired sql.NullTime
			season, episode, epTMDB  sql.NullInt32
			n                        TraktNextEpisode
		)
		if err := rows.Scan(&c.Source, &c.ShowTMDBID, &c.Title, &c.PosterPath, &c.BackdropPath,
			&c.Completed, &c.Aired, &last, &listed, &c.Hidden,
			&season, &episode, &epTMDB, &n.Title, &n.StillPath, &firstAired, &n.FromCalendar); err != nil {
			return nil, fmt.Errorf("failed to scan Up next candidate: %w", err)
		}
		c.LastWatchedAt, c.ListedAt = nullTimePtr(last), nullTimePtr(listed)
		if season.Valid {
			n.TraktEpisodeKey = TraktEpisodeKey{ShowTMDBID: c.ShowTMDBID, Season: int(season.Int32), Episode: int(episode.Int32),
				EpisodeTMDBID: int(epTMDB.Int32)}
			n.FirstAired = nullTimePtr(firstAired)
			c.Next = &n
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read Up next candidates: %w", err)
	}
	return out, nil
}

// NewEpisodeCandidate is a regular episode that aired in /me/new-episodes'
// window and hasn't been watched: a library show's, in the library's
// numbering, or the Trakt calendar's.
type NewEpisodeCandidate struct {
	TraktEpisodeKey
	FromCalendar bool
	// The calendar's: the show's title and artwork, the episode's title,
	// still and first airing. Empty for a library episode.
	ShowTitle, PosterPath, BackdropPath string
	EpisodeTitle, StillPath             string
	FirstAired                          *time.Time
	AirDate                             string // a library episode's (YYYY-MM-DD)
}

// NewEpisodeCandidates lists the regular episodes that aired from from
// (midnight UTC; fromDate is its date) until now (today is its date): the
// library shows' by air date and the calendar's by first airing, each
// without a play. The calendar also lists watchlisted shows, so its
// entries count only for shows the user started (a progress row) or the
// library has. Shows dropped or hidden on Trakt are left out. One query.
func (r *CatalogRepository) NewEpisodeCandidates(ctx context.Context, from, now time.Time, fromDate, today string) ([]NewEpisodeCandidate, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT FALSE, s.tmdb_id, sn.season_number, e.episode_number, COALESCE(e.tmdb_episode_id, 0),
		       '', '', '', '', '', NULL::timestamptz, e.air_date
		FROM shows s
		JOIN seasons sn ON sn.show_id = s.id AND sn.season_number > 0
		JOIN episodes e ON e.season_id = sn.id
		WHERE e.air_date >= $3 AND e.air_date <= $4
		  AND NOT EXISTS (SELECT 1 FROM trakt_hidden h WHERE h.tmdb_id = s.tmdb_id)
		  AND NOT `+episodeWatchedSQL("s.tmdb_id", libraryEpisode, `TRUE`)+`
		UNION ALL
		SELECT TRUE, c.show_tmdb_id, c.season, c.episode, COALESCE(c.episode_tmdb_id, 0),
		       c.show_title, COALESCE(c.poster_path, ''), COALESCE(c.backdrop_path, ''),
		       COALESCE(c.episode_title, ''), COALESCE(c.still_path, ''), c.first_aired, ''
		FROM trakt_calendar c
		WHERE c.season > 0 AND c.first_aired >= $1 AND c.first_aired <= $2
		  AND (EXISTS (SELECT 1 FROM trakt_show_progress p WHERE p.show_tmdb_id = c.show_tmdb_id)
		    OR EXISTS (SELECT 1 FROM shows s WHERE s.tmdb_id = c.show_tmdb_id))
		  AND NOT EXISTS (SELECT 1 FROM trakt_hidden h WHERE h.tmdb_id = c.show_tmdb_id)
		  AND NOT EXISTS (SELECT 1 FROM trakt_watched_episodes w
		                  WHERE w.show_tmdb_id = c.show_tmdb_id AND `+sameEpisode(traktEpisode("w"), traktEpisode("c"))+`)
	`, from, now, fromDate, today)
	if err != nil {
		return nil, fmt.Errorf("failed to list new episodes: %w", err)
	}
	defer rows.Close()

	var out []NewEpisodeCandidate
	for rows.Next() {
		var c NewEpisodeCandidate
		var firstAired sql.NullTime
		if err := rows.Scan(&c.FromCalendar, &c.ShowTMDBID, &c.Season, &c.Episode, &c.EpisodeTMDBID,
			&c.ShowTitle, &c.PosterPath, &c.BackdropPath, &c.EpisodeTitle, &c.StillPath, &firstAired, &c.AirDate); err != nil {
			return nil, fmt.Errorf("failed to scan new episode: %w", err)
		}
		c.FirstAired = nullTimePtr(firstAired)
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read new episodes: %w", err)
	}
	return out, nil
}

// TraktWatchlistItem is a title on the Trakt watchlist.
type TraktWatchlistItem struct {
	MediaType  string // movie or tv
	TMDBID     int
	Title      string
	Year       int // 0 when unknown
	PosterPath string
	ListedAt   time.Time
}

// TraktWatchlist lists the watchlist, newest listed first: mediaType
// "movie" or "tv", or "" for both. One query.
func (r *CatalogRepository) TraktWatchlist(ctx context.Context, mediaType string) ([]TraktWatchlistItem, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT media_type, tmdb_id, title, COALESCE(year, 0), COALESCE(poster_path, ''), listed_at
		FROM trakt_watchlist
		WHERE $1 = '' OR media_type = $1
		ORDER BY listed_at DESC, media_type, trakt_id
	`, mediaType)
	if err != nil {
		return nil, fmt.Errorf("failed to list the watchlist: %w", err)
	}
	defer rows.Close()

	var out []TraktWatchlistItem
	for rows.Next() {
		var w TraktWatchlistItem
		if err := rows.Scan(&w.MediaType, &w.TMDBID, &w.Title, &w.Year, &w.PosterPath, &w.ListedAt); err != nil {
			return nil, fmt.Errorf("failed to scan watchlist item: %w", err)
		}
		out = append(out, w)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read the watchlist: %w", err)
	}
	return out, nil
}

// TraktPlay is one play of Trakt's history.
type TraktPlay struct {
	ID        int64  // Trakt's history event id
	MediaType string // movie or episode
	TMDBID    int    // the movie's or the show's
	Title     string
	Year      int  // 0 when unknown
	Season    *int // episodes only
	Episode   *int
	// EpisodeTitle is "" for movies and when Trakt has none.
	EpisodeTitle string
	WatchedAt    time.Time
}

// TraktHistory returns a page of plays, newest first, and how many there
// are in all. One query.
func (r *CatalogRepository) TraktHistory(ctx context.Context, limit, offset int) ([]TraktPlay, int, error) {
	rows, err := r.db.QueryContext(ctx, `
		WITH page AS (
			SELECT * FROM trakt_history ORDER BY watched_at DESC, id DESC LIMIT $1 OFFSET $2
		)
		SELECT (SELECT count(*) FROM trakt_history), p.id, p.media_type, p.tmdb_id, p.title, p.year,
		       p.season, p.episode, p.episode_title, p.watched_at
		FROM (VALUES (1)) one(x) LEFT JOIN page p ON TRUE
		ORDER BY p.watched_at DESC, p.id DESC
	`, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list the history: %w", err)
	}
	defer rows.Close()

	var total int
	plays := []TraktPlay{}
	for rows.Next() {
		var (
			id, tmdbID            sql.NullInt64
			mediaType, title      sql.NullString
			episodeTitle          sql.NullString
			year, season, episode sql.NullInt32
			watched               sql.NullTime
		)
		if err := rows.Scan(&total, &id, &mediaType, &tmdbID, &title, &year, &season, &episode, &episodeTitle, &watched); err != nil {
			return nil, 0, fmt.Errorf("failed to scan play: %w", err)
		}
		if !id.Valid {
			continue // the page is empty
		}
		plays = append(plays, TraktPlay{ID: id.Int64, MediaType: mediaType.String, TMDBID: int(tmdbID.Int64), Title: title.String,
			Year: int(year.Int32), Season: nullIntPtr(season), Episode: nullIntPtr(episode), EpisodeTitle: episodeTitle.String,
			WatchedAt: watched.Time})
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("failed to read the history: %w", err)
	}
	return plays, total, nil
}

func nullTimePtr(v sql.NullTime) *time.Time {
	if !v.Valid {
		return nil
	}
	t := v.Time
	return &t
}

// utcDate is a timestamp's date in UTC (YYYY-MM-DD), "" when it's NULL.
func utcDate(v sql.NullTime) string {
	if !v.Valid {
		return ""
	}
	return v.Time.UTC().Format(time.DateOnly)
}
