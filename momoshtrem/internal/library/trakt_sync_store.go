package library

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// What the Trakt sync (#132) writes: the rows of migration 005 and the
// sync's cursors (trakt_sync_state); trakt_reads.go reads them. Rows go in as one JSON array per
// statement (jsonb_to_recordset), so the json tags below are column names.
// Zero ints and empty strings are stored as NULL, except season and episode
// numbers: season 0 is specials, so those are pointers.

// The trakt_sync_state keys.
const (
	traktKeyActivities  = "last_activities"
	traktKeySyncAt      = "last_sync_at"
	traktKeyReconcileAt = "last_reconcile_at"
	traktKeyCalendarAt  = "calendar_fetched_at"
	traktKeyLastRun     = "last_run"
)

// TraktPlayRow is one play (trakt_history). TMDBID and TraktID are the
// movie's or the show's.
type TraktPlayRow struct {
	ID            int64     `json:"id"`         // Trakt's history event id
	MediaType     string    `json:"media_type"` // movie|episode
	TMDBID        int       `json:"tmdb_id"`
	TraktID       int64     `json:"trakt_id"`
	Title         string    `json:"title"`
	Year          int       `json:"year"`
	Season        *int      `json:"season"`
	Episode       *int      `json:"episode"`
	EpisodeTMDBID int       `json:"episode_tmdb_id"`
	EpisodeTitle  string    `json:"episode_title"`
	WatchedAt     time.Time `json:"watched_at"`
}

// TraktRatingRow is one rating (trakt_ratings). TMDBID is the rated item's
// own; ShowTMDBID, Season and Episode place a season or an episode.
type TraktRatingRow struct {
	MediaType  string    `json:"media_type"` // movie|show|season|episode
	TraktID    int64     `json:"trakt_id"`
	TMDBID     int       `json:"tmdb_id"`
	ShowTMDBID int       `json:"show_tmdb_id"`
	Season     *int      `json:"season"`
	Episode    *int      `json:"episode"`
	Rating     int       `json:"rating"`
	RatedAt    time.Time `json:"rated_at"`
}

// TraktWatchlistRow is a watchlisted movie or show (trakt_watchlist).
type TraktWatchlistRow struct {
	MediaType string    `json:"media_type"` // movie|tv
	TraktID   int64     `json:"trakt_id"`
	TMDBID    int       `json:"tmdb_id"`
	Title     string    `json:"title"`
	Year      int       `json:"year"`
	Rank      int       `json:"rank"`
	ListedAt  time.Time `json:"listed_at"`
}

// TraktShowProgressRow is a show's watched progress (trakt_show_progress);
// the Next fields are nil when the show is caught up.
type TraktShowProgressRow struct {
	ShowTraktID       int64      `json:"show_trakt_id"`
	ShowTMDBID        int        `json:"show_tmdb_id"`
	Title             string     `json:"title"`
	Year              int        `json:"year"`
	Aired             int        `json:"aired"`
	Completed         int        `json:"completed"`
	LastWatchedAt     *time.Time `json:"last_watched_at"`
	ResetAt           *time.Time `json:"reset_at"`
	LastSeason        *int       `json:"last_season"`
	LastEpisode       *int       `json:"last_episode"`
	NextSeason        *int       `json:"next_season"`
	NextEpisode       *int       `json:"next_episode"`
	NextEpisodeTMDBID int        `json:"next_episode_tmdb_id"`
	NextTitle         string     `json:"next_title"`
}

// TraktHiddenRow is a show the user dropped or hid from progress
// (trakt_hidden).
type TraktHiddenRow struct {
	Section string `json:"section"` // dropped|progress_watched
	TraktID int64  `json:"trakt_id"`
	TMDBID  int    `json:"tmdb_id"`
}

// TraktCalendarRow is an episode of the user's calendar (trakt_calendar).
type TraktCalendarRow struct {
	ShowTraktID   int64     `json:"show_trakt_id"`
	Season        int       `json:"season"`
	Episode       int       `json:"episode"`
	ShowTMDBID    int       `json:"show_tmdb_id"`
	EpisodeTMDBID int       `json:"episode_tmdb_id"`
	ShowTitle     string    `json:"show_title"`
	Year          int       `json:"year"`
	EpisodeTitle  string    `json:"episode_title"`
	FirstAired    time.Time `json:"first_aired"`
}

// TraktSyncState is the sync's cursors (trakt_sync_state). Times are zero
// and the JSON nil until they're first stored.
type TraktSyncState struct {
	// Activities is the last GET /sync/last_activities every fetch of a
	// sync succeeded for, as Trakt answered it.
	Activities        json.RawMessage
	LastSyncAt        time.Time
	LastReconcileAt   time.Time
	CalendarFetchedAt time.Time
	// LastRun is the last sync that ended, as GET /integrations/trakt
	// reports it.
	LastRun json.RawMessage
}

// TraktSyncDone is what a sync that finished stores, in one transaction.
type TraktSyncDone struct {
	Activities json.RawMessage
	At         time.Time
	Reconcile  bool // also sets last_reconcile_at
	Run        json.RawMessage
}

// TraktStillKey is an episode whose still the details step looks up, by
// its show's TMDB id.
type TraktStillKey struct {
	ShowTMDBID int `json:"show_tmdb_id"`
	Season     int `json:"season"`
	Episode    int `json:"episode"`
}

// TraktArtworkWants are the titles and episodes outside the library whose
// imported rows miss artwork.
type TraktArtworkWants struct {
	Movies   []int // TMDB ids: a poster or backdrop
	Shows    []int // TMDB ids: a poster or backdrop
	Episodes []TraktStillKey
}

// TraktImages are a title's TMDB image paths; "" when TMDB has none.
type TraktImages struct {
	Poster   string
	Backdrop string
}

// TraktArtwork is what the details step found. It only fills what's
// missing.
type TraktArtwork struct {
	Movies map[int]TraktImages
	Shows  map[int]TraktImages
	Stills map[TraktStillKey]string
}

// TraktSyncStore writes what the Trakt sync imports. Every write is one
// transaction that first checks the connection it names (trakt_auth's
// connected_at): after a disconnect or a reconnect it's refused with
// ErrTraktConnectionReplaced, so nothing lands after a purge.
type TraktSyncStore struct {
	db *DB
}

// NewTraktSyncStore creates the Trakt sync's store.
func NewTraktSyncStore(db *DB) *TraktSyncStore {
	return &TraktSyncStore{db: db}
}

// State reads the sync's cursors.
func (s *TraktSyncStore) State(ctx context.Context) (TraktSyncState, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT key, value FROM trakt_sync_state`)
	if err != nil {
		return TraktSyncState{}, fmt.Errorf("failed to read the Trakt sync state: %w", err)
	}
	defer rows.Close()
	var st TraktSyncState
	for rows.Next() {
		var key string
		var value []byte
		if err := rows.Scan(&key, &value); err != nil {
			return TraktSyncState{}, fmt.Errorf("failed to scan the Trakt sync state: %w", err)
		}
		var at *time.Time
		switch key {
		case traktKeyActivities:
			st.Activities = value
		case traktKeyLastRun:
			st.LastRun = value
		case traktKeySyncAt:
			at = &st.LastSyncAt
		case traktKeyReconcileAt:
			at = &st.LastReconcileAt
		case traktKeyCalendarAt:
			at = &st.CalendarFetchedAt
		}
		if at != nil {
			if err := json.Unmarshal(value, at); err != nil {
				return TraktSyncState{}, fmt.Errorf("failed to decode the Trakt sync state %q: %w", key, err)
			}
		}
	}
	if err := rows.Err(); err != nil {
		return TraktSyncState{}, fmt.Errorf("failed to read the Trakt sync state: %w", err)
	}
	return st, nil
}

// LatestPlays answers the newest stored play of each kind; zero when
// there's none.
func (s *TraktSyncStore) LatestPlays(ctx context.Context) (movie, episode time.Time, err error) {
	var m, e sql.NullTime
	err = s.db.QueryRowContext(ctx, `
		SELECT MAX(watched_at) FILTER (WHERE media_type = 'movie'), MAX(watched_at) FILTER (WHERE media_type = 'episode')
		FROM trakt_history
	`).Scan(&m, &e)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("failed to read the latest Trakt plays: %w", err)
	}
	return m.Time, e.Time, nil
}

const historyUpsert = `
	INSERT INTO trakt_history (id, media_type, tmdb_id, trakt_id, title, year, season, episode,
	                           episode_tmdb_id, episode_title, watched_at)
	SELECT DISTINCT ON (id) id, media_type, tmdb_id, trakt_id, title, NULLIF(year, 0), season, episode,
	       NULLIF(episode_tmdb_id, 0), NULLIF(episode_title, ''), watched_at
	FROM jsonb_to_recordset($1::jsonb) AS r(id bigint, media_type text, tmdb_id int, trakt_id bigint, title text,
	     year int, season int, episode int, episode_tmdb_id int, episode_title text, watched_at timestamptz)
	ON CONFLICT (id) DO UPDATE SET
		media_type = EXCLUDED.media_type, tmdb_id = EXCLUDED.tmdb_id, trakt_id = EXCLUDED.trakt_id,
		title = EXCLUDED.title, year = EXCLUDED.year, season = EXCLUDED.season, episode = EXCLUDED.episode,
		episode_tmdb_id = EXCLUDED.episode_tmdb_id, episode_title = EXCLUDED.episode_title,
		watched_at = EXCLUDED.watched_at`

// ReplaceHistory replaces every play, and rebuilds trakt_watched_movies and
// trakt_watched_episodes from them.
func (s *TraktSyncStore) ReplaceHistory(ctx context.Context, gen time.Time, plays []TraktPlayRow) error {
	return s.write(ctx, gen, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM trakt_history`); err != nil {
			return fmt.Errorf("failed to clear the Trakt history: %w", err)
		}
		return writeHistory(ctx, tx, plays)
	})
}

// UpsertHistory adds plays by their event id (an overlap lands on the same
// rows), and rebuilds the watched tables.
func (s *TraktSyncStore) UpsertHistory(ctx context.Context, gen time.Time, plays []TraktPlayRow) error {
	return s.write(ctx, gen, func(tx *sql.Tx) error { return writeHistory(ctx, tx, plays) })
}

func writeHistory(ctx context.Context, tx *sql.Tx, plays []TraktPlayRow) error {
	if err := execRows(ctx, tx, historyUpsert, plays); err != nil {
		return fmt.Errorf("failed to store the Trakt history: %w", err)
	}
	for _, q := range []string{
		`DELETE FROM trakt_watched_movies`,
		`INSERT INTO trakt_watched_movies (tmdb_id, plays, last_watched_at)
		 SELECT tmdb_id, COUNT(*), MAX(watched_at) FROM trakt_history WHERE media_type = 'movie' GROUP BY tmdb_id`,
		`DELETE FROM trakt_watched_episodes`,
		`INSERT INTO trakt_watched_episodes (show_tmdb_id, season, episode, episode_tmdb_id, plays, last_watched_at)
		 SELECT tmdb_id, season, episode, MAX(episode_tmdb_id), COUNT(*), MAX(watched_at)
		 FROM trakt_history WHERE media_type = 'episode' GROUP BY tmdb_id, season, episode`,
	} {
		if _, err := tx.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("failed to rebuild the Trakt watched tables: %w", err)
		}
	}
	return nil
}

// ReplaceRatings replaces the ratings of the given media types.
func (s *TraktSyncStore) ReplaceRatings(ctx context.Context, gen time.Time, types []string, ratings []TraktRatingRow) error {
	return s.write(ctx, gen, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM trakt_ratings WHERE media_type = ANY($1::text[])`, types); err != nil {
			return fmt.Errorf("failed to clear the Trakt ratings: %w", err)
		}
		err := execRows(ctx, tx, `
			INSERT INTO trakt_ratings (media_type, trakt_id, tmdb_id, show_tmdb_id, season, episode, rating, rated_at)
			SELECT DISTINCT ON (media_type, trakt_id) media_type, trakt_id, NULLIF(tmdb_id, 0), NULLIF(show_tmdb_id, 0),
			       season, episode, rating, rated_at
			FROM jsonb_to_recordset($1::jsonb) AS r(media_type text, trakt_id bigint, tmdb_id int, show_tmdb_id int,
			     season int, episode int, rating int, rated_at timestamptz)
			WHERE media_type = ANY($2::text[])
		`, ratings, types)
		if err != nil {
			return fmt.Errorf("failed to store the Trakt ratings: %w", err)
		}
		return nil
	})
}

// ReplaceWatchlist replaces the watchlist. A title that stays keeps the
// artwork the details step found for it.
func (s *TraktSyncStore) ReplaceWatchlist(ctx context.Context, gen time.Time, items []TraktWatchlistRow) error {
	return s.write(ctx, gen, func(tx *sql.Tx) error {
		err := execRows(ctx, tx, `
			DELETE FROM trakt_watchlist w WHERE NOT EXISTS (
				SELECT 1 FROM jsonb_to_recordset($1::jsonb) AS r(media_type text, trakt_id bigint)
				WHERE r.media_type = w.media_type AND r.trakt_id = w.trakt_id)
		`, items)
		if err == nil {
			err = execRows(ctx, tx, `
				INSERT INTO trakt_watchlist AS w (media_type, trakt_id, tmdb_id, title, year, rank, listed_at)
				SELECT DISTINCT ON (media_type, trakt_id) media_type, trakt_id, tmdb_id, title, NULLIF(year, 0),
				       NULLIF(rank, 0), listed_at
				FROM jsonb_to_recordset($1::jsonb) AS r(media_type text, trakt_id bigint, tmdb_id int, title text,
				     year int, rank int, listed_at timestamptz)
				ON CONFLICT (media_type, trakt_id) DO UPDATE SET
					tmdb_id = EXCLUDED.tmdb_id, title = EXCLUDED.title, year = EXCLUDED.year,
					rank = EXCLUDED.rank, listed_at = EXCLUDED.listed_at,
					poster_path = CASE WHEN w.tmdb_id = EXCLUDED.tmdb_id THEN w.poster_path END,
					backdrop_path = CASE WHEN w.tmdb_id = EXCLUDED.tmdb_id THEN w.backdrop_path END
			`, items)
		}
		if err != nil {
			return fmt.Errorf("failed to store the Trakt watchlist: %w", err)
		}
		return nil
	})
}

// ReplaceHidden replaces the dropped and hidden shows.
func (s *TraktSyncStore) ReplaceHidden(ctx context.Context, gen time.Time, shows []TraktHiddenRow) error {
	return s.write(ctx, gen, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM trakt_hidden`); err != nil {
			return fmt.Errorf("failed to clear the Trakt hidden shows: %w", err)
		}
		err := execRows(ctx, tx, `
			INSERT INTO trakt_hidden (section, trakt_id, tmdb_id)
			SELECT DISTINCT ON (section, trakt_id) section, trakt_id, tmdb_id
			FROM jsonb_to_recordset($1::jsonb) AS r(section text, trakt_id bigint, tmdb_id int)
		`, shows)
		if err != nil {
			return fmt.Errorf("failed to store the Trakt hidden shows: %w", err)
		}
		return nil
	})
}

// ReplaceShowProgress replaces every show's progress. A show keeps its
// poster and backdrop, and its next episode's still while it's the same
// episode.
func (s *TraktSyncStore) ReplaceShowProgress(ctx context.Context, gen time.Time, shows []TraktShowProgressRow) error {
	return s.write(ctx, gen, func(tx *sql.Tx) error {
		err := execRows(ctx, tx, `
			DELETE FROM trakt_show_progress p WHERE NOT EXISTS (
				SELECT 1 FROM jsonb_to_recordset($1::jsonb) AS r(show_trakt_id bigint) WHERE r.show_trakt_id = p.show_trakt_id)
		`, shows)
		if err == nil {
			err = execRows(ctx, tx, `
				INSERT INTO trakt_show_progress AS p (show_trakt_id, show_tmdb_id, title, year, aired, completed,
				       last_watched_at, reset_at, last_season, last_episode, next_season, next_episode,
				       next_episode_tmdb_id, next_title, updated_at)
				SELECT DISTINCT ON (show_trakt_id) show_trakt_id, show_tmdb_id, title, NULLIF(year, 0), aired, completed,
				       last_watched_at, reset_at, last_season, last_episode, next_season, next_episode,
				       NULLIF(next_episode_tmdb_id, 0), NULLIF(next_title, ''), NOW()
				FROM jsonb_to_recordset($1::jsonb) AS r(show_trakt_id bigint, show_tmdb_id int, title text, year int,
				     aired int, completed int, last_watched_at timestamptz, reset_at timestamptz, last_season int,
				     last_episode int, next_season int, next_episode int, next_episode_tmdb_id int, next_title text)
				ON CONFLICT (show_trakt_id) DO UPDATE SET
					show_tmdb_id = EXCLUDED.show_tmdb_id, title = EXCLUDED.title, year = EXCLUDED.year,
					aired = EXCLUDED.aired, completed = EXCLUDED.completed,
					last_watched_at = EXCLUDED.last_watched_at, reset_at = EXCLUDED.reset_at,
					last_season = EXCLUDED.last_season, last_episode = EXCLUDED.last_episode,
					next_season = EXCLUDED.next_season, next_episode = EXCLUDED.next_episode,
					next_episode_tmdb_id = EXCLUDED.next_episode_tmdb_id, next_title = EXCLUDED.next_title,
					updated_at = NOW(),
					poster_path = CASE WHEN p.show_tmdb_id = EXCLUDED.show_tmdb_id THEN p.poster_path END,
					backdrop_path = CASE WHEN p.show_tmdb_id = EXCLUDED.show_tmdb_id THEN p.backdrop_path END,
					next_still_path = CASE WHEN p.show_tmdb_id = EXCLUDED.show_tmdb_id
						AND p.next_season IS NOT DISTINCT FROM EXCLUDED.next_season
						AND p.next_episode IS NOT DISTINCT FROM EXCLUDED.next_episode THEN p.next_still_path END
			`, shows)
		}
		if err != nil {
			return fmt.Errorf("failed to store the Trakt show progress: %w", err)
		}
		return nil
	})
}

// ReplaceCalendar replaces the calendar and records when it was fetched.
// An episode that stays keeps its artwork.
func (s *TraktSyncStore) ReplaceCalendar(ctx context.Context, gen time.Time, entries []TraktCalendarRow, fetchedAt time.Time) error {
	return s.write(ctx, gen, func(tx *sql.Tx) error {
		err := execRows(ctx, tx, `
			DELETE FROM trakt_calendar c WHERE NOT EXISTS (
				SELECT 1 FROM jsonb_to_recordset($1::jsonb) AS r(show_trakt_id bigint, season int, episode int)
				WHERE r.show_trakt_id = c.show_trakt_id AND r.season = c.season AND r.episode = c.episode)
		`, entries)
		if err == nil {
			err = execRows(ctx, tx, `
				INSERT INTO trakt_calendar AS c (show_trakt_id, season, episode, show_tmdb_id, episode_tmdb_id,
				       show_title, year, episode_title, first_aired)
				SELECT DISTINCT ON (show_trakt_id, season, episode) show_trakt_id, season, episode, show_tmdb_id,
				       NULLIF(episode_tmdb_id, 0), show_title, NULLIF(year, 0), NULLIF(episode_title, ''), first_aired
				FROM jsonb_to_recordset($1::jsonb) AS r(show_trakt_id bigint, season int, episode int, show_tmdb_id int,
				     episode_tmdb_id int, show_title text, year int, episode_title text, first_aired timestamptz)
				ON CONFLICT (show_trakt_id, season, episode) DO UPDATE SET
					show_tmdb_id = EXCLUDED.show_tmdb_id, episode_tmdb_id = EXCLUDED.episode_tmdb_id,
					show_title = EXCLUDED.show_title, year = EXCLUDED.year, episode_title = EXCLUDED.episode_title,
					first_aired = EXCLUDED.first_aired,
					still_path = CASE WHEN c.show_tmdb_id = EXCLUDED.show_tmdb_id THEN c.still_path END,
					poster_path = CASE WHEN c.show_tmdb_id = EXCLUDED.show_tmdb_id THEN c.poster_path END,
					backdrop_path = CASE WHEN c.show_tmdb_id = EXCLUDED.show_tmdb_id THEN c.backdrop_path END
			`, entries)
		}
		if err != nil {
			return fmt.Errorf("failed to store the Trakt calendar: %w", err)
		}
		return setTraktState(ctx, tx, traktKeyCalendarAt, fetchedAt)
	})
}

// ArtworkWants lists the titles and episodes outside the library whose
// imported rows miss artwork: watchlist titles, shows in progress and on
// the calendar, and the progress's next episodes and the calendar's
// episodes (stills). The library's own titles have their artwork.
func (s *TraktSyncStore) ArtworkWants(ctx context.Context) (TraktArtworkWants, error) {
	var w TraktArtworkWants
	err := scanInts(ctx, s.db, &w.Movies, `
		SELECT DISTINCT tmdb_id FROM trakt_watchlist t
		WHERE media_type = 'movie' AND (poster_path IS NULL OR backdrop_path IS NULL)
		  AND NOT EXISTS (SELECT 1 FROM movies m WHERE m.tmdb_id = t.tmdb_id)
		ORDER BY 1`)
	if err == nil {
		err = scanInts(ctx, s.db, &w.Shows, `
			SELECT tmdb_id FROM (
				SELECT tmdb_id FROM trakt_watchlist
				WHERE media_type = 'tv' AND (poster_path IS NULL OR backdrop_path IS NULL)
				UNION SELECT show_tmdb_id FROM trakt_show_progress WHERE poster_path IS NULL OR backdrop_path IS NULL
				UNION SELECT show_tmdb_id FROM trakt_calendar WHERE poster_path IS NULL OR backdrop_path IS NULL
			) t(tmdb_id)
			WHERE NOT EXISTS (SELECT 1 FROM shows s WHERE s.tmdb_id = t.tmdb_id)
			ORDER BY 1`)
	}
	if err != nil {
		return TraktArtworkWants{}, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT show_tmdb_id, season, episode FROM (
			SELECT show_tmdb_id, next_season, next_episode FROM trakt_show_progress
			WHERE next_season IS NOT NULL AND next_episode IS NOT NULL AND next_still_path IS NULL
			UNION SELECT show_tmdb_id, season, episode FROM trakt_calendar WHERE still_path IS NULL
		) t(show_tmdb_id, season, episode)
		WHERE NOT EXISTS (SELECT 1 FROM shows s WHERE s.tmdb_id = t.show_tmdb_id)
		ORDER BY 1, 2, 3`)
	if err != nil {
		return TraktArtworkWants{}, fmt.Errorf("failed to list the Trakt episodes without stills: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var k TraktStillKey
		if err := rows.Scan(&k.ShowTMDBID, &k.Season, &k.Episode); err != nil {
			return TraktArtworkWants{}, fmt.Errorf("failed to scan a Trakt episode: %w", err)
		}
		w.Episodes = append(w.Episodes, k)
	}
	if err := rows.Err(); err != nil {
		return TraktArtworkWants{}, fmt.Errorf("failed to list the Trakt episodes without stills: %w", err)
	}
	return w, nil
}

// SetArtwork fills the artwork the details step found, only where it's
// missing.
func (s *TraktSyncStore) SetArtwork(ctx context.Context, gen time.Time, a TraktArtwork) error {
	type images struct {
		TMDBID   int    `json:"tmdb_id"`
		Poster   string `json:"poster"`
		Backdrop string `json:"backdrop"`
	}
	type still struct {
		TraktStillKey
		Still string `json:"still"`
	}
	list := func(m map[int]TraktImages) []images {
		out := make([]images, 0, len(m))
		for id, im := range m {
			out = append(out, images{id, im.Poster, im.Backdrop})
		}
		return out
	}
	movies, shows := list(a.Movies), list(a.Shows)
	stills := make([]still, 0, len(a.Stills))
	for k, path := range a.Stills {
		stills = append(stills, still{k, path})
	}

	const imagesFrom = `jsonb_to_recordset($1::jsonb) AS a(tmdb_id int, poster text, backdrop text)`
	const fill = `poster_path = COALESCE(t.poster_path, NULLIF(a.poster, '')),
	              backdrop_path = COALESCE(t.backdrop_path, NULLIF(a.backdrop, ''))`
	const stillsFrom = `jsonb_to_recordset($1::jsonb) AS a(show_tmdb_id int, season int, episode int, still text)`
	return s.write(ctx, gen, func(tx *sql.Tx) error {
		for _, u := range []struct {
			rows  any
			query string
		}{
			{movies, `UPDATE trakt_watchlist t SET ` + fill + ` FROM ` + imagesFrom + `
				WHERE t.media_type = 'movie' AND t.tmdb_id = a.tmdb_id`},
			{shows, `UPDATE trakt_watchlist t SET ` + fill + ` FROM ` + imagesFrom + `
				WHERE t.media_type = 'tv' AND t.tmdb_id = a.tmdb_id`},
			{shows, `UPDATE trakt_show_progress t SET ` + fill + ` FROM ` + imagesFrom + ` WHERE t.show_tmdb_id = a.tmdb_id`},
			{shows, `UPDATE trakt_calendar t SET ` + fill + ` FROM ` + imagesFrom + ` WHERE t.show_tmdb_id = a.tmdb_id`},
			{stills, `UPDATE trakt_show_progress t SET next_still_path = COALESCE(t.next_still_path, NULLIF(a.still, ''))
				FROM ` + stillsFrom + `
				WHERE t.show_tmdb_id = a.show_tmdb_id AND t.next_season = a.season AND t.next_episode = a.episode`},
			{stills, `UPDATE trakt_calendar t SET still_path = COALESCE(t.still_path, NULLIF(a.still, ''))
				FROM ` + stillsFrom + `
				WHERE t.show_tmdb_id = a.show_tmdb_id AND t.season = a.season AND t.episode = a.episode`},
		} {
			if err := execRows(ctx, tx, u.query, u.rows); err != nil {
				return fmt.Errorf("failed to store the Trakt artwork: %w", err)
			}
		}
		return nil
	})
}

// FinishSync stores what a finished sync leaves: the last_activities
// snapshot, last_sync_at, last_reconcile_at for a reconcile, and the run.
func (s *TraktSyncStore) FinishSync(ctx context.Context, gen time.Time, d TraktSyncDone) error {
	return s.write(ctx, gen, func(tx *sql.Tx) error {
		if err := setTraktState(ctx, tx, traktKeyActivities, d.Activities); err != nil {
			return err
		}
		if err := setTraktState(ctx, tx, traktKeySyncAt, d.At); err != nil {
			return err
		}
		if d.Reconcile {
			if err := setTraktState(ctx, tx, traktKeyReconcileAt, d.At); err != nil {
				return err
			}
		}
		return setTraktState(ctx, tx, traktKeyLastRun, d.Run)
	})
}

// SaveRun stores the last run, for a sync that ended without finishing.
func (s *TraktSyncStore) SaveRun(ctx context.Context, gen time.Time, run json.RawMessage) error {
	return s.write(ctx, gen, func(tx *sql.Tx) error { return setTraktState(ctx, tx, traktKeyLastRun, run) })
}

// write runs fn in a transaction once the stored connection is gen. The
// connection's row is held (FOR SHARE) until the commit, so a purge
// (TraktAuthRepository.Purge locks it first) either waits for the commit
// and deletes what it wrote, or runs first and the check fails.
func (s *TraktSyncStore) write(ctx context.Context, gen time.Time, fn func(tx *sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()
	var connectedAt time.Time
	err = tx.QueryRowContext(ctx, `SELECT connected_at FROM trakt_auth WHERE id = 1 FOR SHARE`).Scan(&connectedAt)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && !connectedAt.Equal(gen)) {
		return ErrTraktConnectionReplaced
	}
	if err != nil {
		return fmt.Errorf("failed to read the Trakt connection: %w", err)
	}
	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit the Trakt sync: %w", err)
	}
	return nil
}

// execRows runs query with rows as its first parameter, a JSON array, then
// args.
func execRows(ctx context.Context, tx *sql.Tx, query string, rows any, args ...any) error {
	b, err := json.Marshal(rows)
	if err != nil {
		return fmt.Errorf("failed to encode the rows: %w", err)
	}
	if string(b) == "null" { // a nil slice
		b = []byte("[]")
	}
	_, err = tx.ExecContext(ctx, query, append([]any{string(b)}, args...)...)
	return err
}

func setTraktState(ctx context.Context, tx *sql.Tx, key string, value any) error {
	raw, ok := value.(json.RawMessage)
	if ok && len(raw) == 0 {
		raw = json.RawMessage("null")
	} else if !ok {
		var err error
		if raw, err = json.Marshal(value); err != nil {
			return fmt.Errorf("failed to encode the Trakt sync state %q: %w", key, err)
		}
	}
	_, err := tx.ExecContext(ctx, `
		INSERT INTO trakt_sync_state (key, value, updated_at) VALUES ($1, $2::jsonb, NOW())
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()
	`, key, string(raw))
	if err != nil {
		return fmt.Errorf("failed to store the Trakt sync state %q: %w", key, err)
	}
	return nil
}

func scanInts(ctx context.Context, db *DB, dst *[]int, query string) error {
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to list the Trakt titles without artwork: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var n int
		if err := rows.Scan(&n); err != nil {
			return fmt.Errorf("failed to scan a Trakt title: %w", err)
		}
		*dst = append(*dst, n)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("failed to list the Trakt titles without artwork: %w", err)
	}
	return nil
}
