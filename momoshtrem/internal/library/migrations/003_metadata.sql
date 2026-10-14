-- B1 metadata foundation: rich TMDB metadata, specials (season 0), episode
-- ordering, and a cache for TMDB responses. Idempotent, so a database that
-- already has some of it (a re-run after a partial apply) still migrates.

ALTER TABLE movies
    ADD COLUMN IF NOT EXISTS imdb_id             TEXT,
    ADD COLUMN IF NOT EXISTS poster_path         TEXT,
    ADD COLUMN IF NOT EXISTS backdrop_path       TEXT,
    ADD COLUMN IF NOT EXISTS runtime             INTEGER,
    ADD COLUMN IF NOT EXISTS genres              JSONB,
    -- NULL until the first metadata sync; backfill-metadata resumes from it.
    ADD COLUMN IF NOT EXISTS metadata_updated_at TIMESTAMPTZ;

ALTER TABLE shows
    ADD COLUMN IF NOT EXISTS imdb_id             TEXT,
    ADD COLUMN IF NOT EXISTS tvdb_id             INTEGER,
    ADD COLUMN IF NOT EXISTS original_name       TEXT,
    ADD COLUMN IF NOT EXISTS original_language   TEXT,
    ADD COLUMN IF NOT EXISTS origin_country      TEXT[],
    ADD COLUMN IF NOT EXISTS is_anime            BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS poster_path         TEXT,
    ADD COLUMN IF NOT EXISTS backdrop_path       TEXT,
    ADD COLUMN IF NOT EXISTS status              TEXT,
    ADD COLUMN IF NOT EXISTS genres              JSONB,
    ADD COLUMN IF NOT EXISTS episode_ordering    TEXT NOT NULL DEFAULT 'aired'
        CONSTRAINT shows_episode_ordering_check
        CHECK (episode_ordering IN ('aired', 'absolute', 'episode_group')),
    ADD COLUMN IF NOT EXISTS episode_group_id    TEXT,
    ADD COLUMN IF NOT EXISTS include_specials    BOOLEAN NOT NULL DEFAULT TRUE,
    ADD COLUMN IF NOT EXISTS classifier_mode     TEXT NOT NULL DEFAULT 'auto'
        CONSTRAINT shows_classifier_mode_check
        CHECK (classifier_mode IN ('auto', 'rules_only', 'ai_always')),
    ADD COLUMN IF NOT EXISTS metadata_updated_at TIMESTAMPTZ;

-- Season 0 holds specials (PLAN D8); nothing stopped it before, the sync
-- just skipped it.
ALTER TABLE seasons
    ADD COLUMN IF NOT EXISTS name        TEXT,
    ADD COLUMN IF NOT EXISTS poster_path TEXT;

ALTER TABLE episodes
    ADD COLUMN IF NOT EXISTS tmdb_episode_id INTEGER,
    ADD COLUMN IF NOT EXISTS absolute_number INTEGER,
    ADD COLUMN IF NOT EXISTS episode_type    TEXT,
    ADD COLUMN IF NOT EXISTS runtime         INTEGER,
    ADD COLUMN IF NOT EXISTS still_path      TEXT,
    ADD COLUMN IF NOT EXISTS overview        TEXT;

-- air_date stays TEXT: ISO dates compare correctly as strings. Unknown is
-- NULL, never ''.
UPDATE episodes SET air_date = NULL WHERE air_date = '';

CREATE INDEX IF NOT EXISTS idx_episodes_tmdb_episode ON episodes(tmdb_episode_id);

-- Every page load looks up the active assignment of each item.
CREATE INDEX IF NOT EXISTS idx_assignments_active_item
    ON torrent_assignments(item_type, item_id) WHERE is_active;

-- TMDB responses, keyed by request. expires_at NULL means it never expires
-- (derived data such as artwork colours).
CREATE TABLE IF NOT EXISTS tmdb_cache (
    key        TEXT PRIMARY KEY,
    payload    JSONB NOT NULL,
    fetched_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_tmdb_cache_expires ON tmdb_cache(expires_at);
