-- B5 Trakt, read-only (#8; PLAN §5, §6 "Trakt (B5)"): the connection, what
-- the sync imports, and the sync's own state. Every table here is Trakt's
-- and a disconnect purges them all in one transaction; the settings
-- (app_settings key 'trakt') stay. Movies and shows without a TMDB id are
-- skipped on import, so every row maps to TMDB. Idempotent, like 003.
--
-- Reversible by deploying the old binary: nothing else reads these tables.

-- The one connection. The tokens are AES-GCM ciphertexts (SECRET_KEY).
-- connected_at is the connection's generation: every sync write checks it,
-- so nothing lands after a disconnect (or a reconnect) replaced it.
CREATE TABLE IF NOT EXISTS trakt_auth (
    id                SMALLINT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    username          TEXT NOT NULL,
    user_slug         TEXT NOT NULL,
    access_token_enc  BYTEA NOT NULL,
    refresh_token_enc BYTEA NOT NULL,
    token_created_at  TIMESTAMPTZ NOT NULL,   -- Trakt's created_at
    expires_at        TIMESTAMPTZ NOT NULL,   -- created_at + expires_in
    needs_reauth      BOOLEAN NOT NULL DEFAULT FALSE,
    reauth_reason     TEXT,
    connected_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- The sync's cursors: the last_activities snapshot, last_sync_at,
-- last_reconcile_at, the calendar's fetch time.
CREATE TABLE IF NOT EXISTS trakt_sync_state (
    key        TEXT PRIMARY KEY,
    value      JSONB NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Every play, keyed by Trakt's history event id (an incremental sync's
-- overlap upserts onto the same rows). tmdb_id and trakt_id are the movie's
-- or the show's.
CREATE TABLE IF NOT EXISTS trakt_history (
    id              BIGINT PRIMARY KEY,
    media_type      TEXT NOT NULL CHECK (media_type IN ('movie', 'episode')),
    tmdb_id         INTEGER NOT NULL,
    trakt_id        BIGINT NOT NULL,
    title           TEXT NOT NULL,
    year            INTEGER,
    season          INTEGER,
    episode         INTEGER,
    episode_tmdb_id INTEGER,
    episode_title   TEXT,
    watched_at      TIMESTAMPTZ NOT NULL,
    CONSTRAINT trakt_history_episode_check CHECK (
        (media_type = 'movie' AND season IS NULL AND episode IS NULL)
     OR (media_type = 'episode' AND season IS NOT NULL AND episode IS NOT NULL))
);
CREATE INDEX IF NOT EXISTS idx_trakt_history_watched ON trakt_history (watched_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_trakt_history_title ON trakt_history (media_type, tmdb_id);

-- Derived from trakt_history in the same transaction that writes it: what
-- the title pages, /library and Up next read.
CREATE TABLE IF NOT EXISTS trakt_watched_movies (
    tmdb_id         INTEGER PRIMARY KEY,
    plays           INTEGER NOT NULL,
    last_watched_at TIMESTAMPTZ NOT NULL
);

-- Keyed by Trakt's numbering; library episodes match on episode_tmdb_id
-- first, and on season and number only when either id is missing.
CREATE TABLE IF NOT EXISTS trakt_watched_episodes (
    show_tmdb_id    INTEGER NOT NULL,
    season          INTEGER NOT NULL,
    episode         INTEGER NOT NULL,
    episode_tmdb_id INTEGER,
    plays           INTEGER NOT NULL,
    last_watched_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (show_tmdb_id, season, episode)
);
CREATE INDEX IF NOT EXISTS idx_trakt_watched_episodes_tmdb
    ON trakt_watched_episodes (episode_tmdb_id) WHERE episode_tmdb_id IS NOT NULL;

-- tmdb_id is the rated item's own (a season's or an episode's too, NULL
-- when Trakt has none); show_tmdb_id, season and episode place a season or
-- an episode.
CREATE TABLE IF NOT EXISTS trakt_ratings (
    media_type   TEXT NOT NULL CHECK (media_type IN ('movie', 'show', 'season', 'episode')),
    trakt_id     BIGINT NOT NULL,
    tmdb_id      INTEGER,
    show_tmdb_id INTEGER,
    season       INTEGER,
    episode      INTEGER,
    rating       SMALLINT NOT NULL CHECK (rating BETWEEN 1 AND 10),
    rated_at     TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (media_type, trakt_id)
);
CREATE INDEX IF NOT EXISTS idx_trakt_ratings_tmdb ON trakt_ratings (media_type, tmdb_id);

-- media_type is the library's (movie|tv). Artwork comes from Trakt's item
-- or, for titles not in the library, the details step (tmdb_cache).
CREATE TABLE IF NOT EXISTS trakt_watchlist (
    media_type    TEXT NOT NULL CHECK (media_type IN ('movie', 'tv')),
    trakt_id      BIGINT NOT NULL,
    tmdb_id       INTEGER NOT NULL,
    title         TEXT NOT NULL,
    year          INTEGER,
    poster_path   TEXT,
    backdrop_path TEXT,
    rank          INTEGER,
    listed_at     TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (media_type, trakt_id)
);
CREATE INDEX IF NOT EXISTS idx_trakt_watchlist_tmdb ON trakt_watchlist (media_type, tmdb_id);
CREATE INDEX IF NOT EXISTS idx_trakt_watchlist_listed ON trakt_watchlist (listed_at DESC);

-- One row per show from the bulk /sync/progress/watched: aired and
-- completed count regular episodes; last_* and next_* are Trakt's
-- (next_* NULL when caught up).
CREATE TABLE IF NOT EXISTS trakt_show_progress (
    show_trakt_id        BIGINT PRIMARY KEY,
    show_tmdb_id         INTEGER NOT NULL,
    title                TEXT NOT NULL,
    year                 INTEGER,
    poster_path          TEXT,
    backdrop_path        TEXT,
    aired                INTEGER NOT NULL,
    completed            INTEGER NOT NULL,
    last_watched_at      TIMESTAMPTZ,
    reset_at             TIMESTAMPTZ,
    last_season          INTEGER,
    last_episode         INTEGER,
    next_season          INTEGER,
    next_episode         INTEGER,
    next_episode_tmdb_id INTEGER,
    next_title           TEXT,
    next_still_path      TEXT,
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_trakt_show_progress_tmdb ON trakt_show_progress (show_tmdb_id);

-- Shows the user dropped, or hid from progress on trakt.tv: Up next, new
-- episodes and the Watching filter leave them out.
CREATE TABLE IF NOT EXISTS trakt_hidden (
    section  TEXT NOT NULL CHECK (section IN ('dropped', 'progress_watched')),
    trakt_id BIGINT NOT NULL,
    tmdb_id  INTEGER NOT NULL,
    PRIMARY KEY (section, trakt_id)
);
CREATE INDEX IF NOT EXISTS idx_trakt_hidden_tmdb ON trakt_hidden (tmdb_id);

-- /calendars/my/shows from 7 days ago, 33 days long: Up next's next
-- episode once a show is caught up, and new episodes of shows outside the
-- library.
CREATE TABLE IF NOT EXISTS trakt_calendar (
    show_trakt_id   BIGINT NOT NULL,
    season          INTEGER NOT NULL,
    episode         INTEGER NOT NULL,
    show_tmdb_id    INTEGER NOT NULL,
    episode_tmdb_id INTEGER,
    show_title      TEXT NOT NULL,
    year            INTEGER,
    episode_title   TEXT,
    first_aired     TIMESTAMPTZ NOT NULL,
    still_path      TEXT,
    poster_path     TEXT,
    backdrop_path   TEXT,
    PRIMARY KEY (show_trakt_id, season, episode)
);
CREATE INDEX IF NOT EXISTS idx_trakt_calendar_aired ON trakt_calendar (first_aired);
CREATE INDEX IF NOT EXISTS idx_trakt_calendar_show ON trakt_calendar (show_tmdb_id);
