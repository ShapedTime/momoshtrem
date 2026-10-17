-- Downloads (#77, P4 #202; PLAN §5 "010_downloads.sql", §6 "Downloads
-- (#77)"): a Keep or a smart pick on a movie, show, season or episode,
-- and what was watched here. New tables only. Idempotent, like 003.
--
-- What a download wants isn't stored: the download manager resolves it on
-- every tick from the active torrent assignments, so a show or season
-- keep picks up episodes assigned later. Its pins live in the piece cache
-- and are rebuilt from this table at startup.
--
-- Reversible by deploying the old binary: nothing else reads these
-- tables, so pinned data becomes ordinary cache.

-- One row per Keep (kept) or smart pick (smart). The target is one of the
-- four ids, by scope; the API names a season by its show and number, and
-- the row stores the seasons row. Releases are soft: a released row keeps
-- released_at and its reason (deleted: a DELETE; expired: expires_at
-- passed; watched: an until-watched download ended), and every read but
-- the smart re-pick rule and history skips it.
CREATE TABLE IF NOT EXISTS downloads (
    id             BIGSERIAL PRIMARY KEY,
    reason         TEXT NOT NULL CHECK (reason IN ('kept', 'smart')),
    smart_source   TEXT CHECK (smart_source IN ('up_next', 'streaming')),
    scope          TEXT NOT NULL CHECK (scope IN ('movie', 'show', 'season', 'episode')),
    movie_id       BIGINT REFERENCES movies(id) ON DELETE CASCADE,
    show_id        BIGINT REFERENCES shows(id) ON DELETE CASCADE,
    season_id      BIGINT REFERENCES seasons(id) ON DELETE CASCADE,
    episode_id     BIGINT REFERENCES episodes(id) ON DELETE CASCADE,
    until_watched  BOOLEAN NOT NULL,
    expires_at     TIMESTAMPTZ,   -- NULL: no maximum
    released_at    TIMESTAMPTZ,   -- NULL while active
    release_reason TEXT CHECK (release_reason IN ('deleted', 'expired', 'watched')),
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    -- A smart pick says why it was picked; a kept download never does.
    CONSTRAINT downloads_source_reason_check CHECK ((reason = 'smart') = (smart_source IS NOT NULL)),
    -- The scope's id is the only one set.
    CONSTRAINT downloads_target_check CHECK (
        (scope = 'movie'   AND movie_id IS NOT NULL AND show_id IS NULL AND season_id IS NULL AND episode_id IS NULL)
     OR (scope = 'show'    AND show_id IS NOT NULL AND movie_id IS NULL AND season_id IS NULL AND episode_id IS NULL)
     OR (scope = 'season'  AND season_id IS NOT NULL AND movie_id IS NULL AND show_id IS NULL AND episode_id IS NULL)
     OR (scope = 'episode' AND episode_id IS NOT NULL AND movie_id IS NULL AND show_id IS NULL AND season_id IS NULL)),
    CONSTRAINT downloads_release_check CHECK ((released_at IS NULL) = (release_reason IS NULL))
);

-- One active download per target, so a Keep on a smart pick turns that row
-- kept. Released rows stay out of them.
CREATE UNIQUE INDEX IF NOT EXISTS downloads_active_movie ON downloads (movie_id)
    WHERE movie_id IS NOT NULL AND released_at IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS downloads_active_show ON downloads (show_id)
    WHERE show_id IS NOT NULL AND released_at IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS downloads_active_season ON downloads (season_id)
    WHERE season_id IS NOT NULL AND released_at IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS downloads_active_episode ON downloads (episode_id)
    WHERE episode_id IS NOT NULL AND released_at IS NULL;
-- Expiry, over the active rows.
CREATE INDEX IF NOT EXISTS idx_downloads_expires ON downloads (expires_at)
    WHERE released_at IS NULL;
-- Every row by target, released ones too: the FK cascades (a title's
-- delete) and the smart re-pick rule read them.
CREATE INDEX IF NOT EXISTS idx_downloads_movie ON downloads (movie_id);
CREATE INDEX IF NOT EXISTS idx_downloads_show ON downloads (show_id);
CREATE INDEX IF NOT EXISTS idx_downloads_season ON downloads (season_id);
CREATE INDEX IF NOT EXISTS idx_downloads_episode ON downloads (episode_id);

-- What was watched here, not on Trakt: a local session that reaches 90%
-- upserts its item's row. Watched is a Trakt play or a row here, after the
-- download's created_at.
CREATE TABLE IF NOT EXISTS local_watches (
    id         BIGSERIAL PRIMARY KEY,
    movie_id   BIGINT REFERENCES movies(id) ON DELETE CASCADE,
    episode_id BIGINT REFERENCES episodes(id) ON DELETE CASCADE,
    watched_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT local_watches_target_check CHECK ((movie_id IS NULL) <> (episode_id IS NULL))
);
CREATE UNIQUE INDEX IF NOT EXISTS local_watches_movie ON local_watches (movie_id)
    WHERE movie_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS local_watches_episode ON local_watches (episode_id)
    WHERE episode_id IS NOT NULL;
