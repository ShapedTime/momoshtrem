-- B2 assignment jobs (#5): assigning a torrent creates a job that a worker
-- classifies in the background; the files it finds are stored as proposals
-- the user reviews before they're applied (PLAN §5, §6 "Assignment").
-- Idempotent, like 003.

-- How an assignment was made. Both are NULL for rows written before jobs
-- (and confidence for manual picks). The VFS groups a multi-episode file
-- into one entry only for rows with a match_source, so legacy paths stay.
ALTER TABLE torrent_assignments
    ADD COLUMN IF NOT EXISTS confidence TEXT
        CONSTRAINT torrent_assignments_confidence_check
        CHECK (confidence IN ('high', 'medium', 'low')),
    ADD COLUMN IF NOT EXISTS match_source TEXT
        CONSTRAINT torrent_assignments_match_source_check
        CHECK (match_source IN ('rules', 'ai', 'manual'));

CREATE TABLE IF NOT EXISTS assignment_jobs (
    id            BIGSERIAL PRIMARY KEY,
    -- The title. Both FKs are NULL for a dry run on a title that isn't in
    -- the library (#6 Inspect); deleting the title deletes its jobs.
    media_type    TEXT NOT NULL CHECK (media_type IN ('movie', 'tv')),
    tmdb_id       INTEGER NOT NULL,
    show_id       BIGINT REFERENCES shows(id) ON DELETE CASCADE,
    movie_id      BIGINT REFERENCES movies(id) ON DELETE CASCADE,
    -- The torrent. torrent_name is the magnet's dn (or the info hash) until
    -- the file list arrives; sizes and counts are NULL until then.
    info_hash     TEXT NOT NULL,
    magnet_uri    TEXT NOT NULL,
    torrent_name  TEXT NOT NULL,
    indexer       TEXT,
    release_group TEXT,
    size_bytes    BIGINT,
    file_count    INTEGER,
    video_count   INTEGER,           -- video files that aren't extras
    -- Movies: the file the assign request picked up front.
    file_path     TEXT,
    status        TEXT NOT NULL DEFAULT 'queued' CHECK (status IN (
                      'queued', 'fetching_metadata', 'classifying', 'resolving',
                      'needs_review', 'ready', 'applied', 'failed', 'cancelled')),
    dry_run       BOOLEAN NOT NULL DEFAULT FALSE,
    hint            JSONB,           -- {season?, episode?, text?}
    suggested_hints JSONB NOT NULL DEFAULT '[]'::jsonb,
    ai_files      INTEGER,           -- files sent in the current or latest AI run
    error         TEXT,              -- why it failed
    ai_error      TEXT,              -- the latest AI run failed
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    applied_at    TIMESTAMPTZ,
    CONSTRAINT assignment_jobs_item_check CHECK (
        (media_type = 'tv' AND movie_id IS NULL) OR (media_type = 'movie' AND show_id IS NULL))
);

-- The worker claims the oldest queued job.
CREATE INDEX IF NOT EXISTS idx_assignment_jobs_working
    ON assignment_jobs(id) WHERE status IN ('queued', 'fetching_metadata', 'classifying', 'resolving');
CREATE INDEX IF NOT EXISTS idx_assignment_jobs_show ON assignment_jobs(show_id);
CREATE INDEX IF NOT EXISTS idx_assignment_jobs_movie ON assignment_jobs(movie_id);
CREATE INDEX IF NOT EXISTS idx_assignment_jobs_hash ON assignment_jobs(info_hash);

-- One open job per title and torrent: assigning the same torrent again
-- answers the job that's already open.
CREATE UNIQUE INDEX IF NOT EXISTS assignment_jobs_open_key
    ON assignment_jobs(media_type, tmdb_id, info_hash)
    WHERE NOT dry_run AND status IN ('queued', 'fetching_metadata', 'classifying', 'resolving', 'needs_review', 'ready');

-- One row per file of the torrent, subtitles too: a subtitle that belongs
-- to a video links to it and isn't listed by the API.
CREATE TABLE IF NOT EXISTS assignment_proposals (
    id                    BIGSERIAL PRIMARY KEY,
    job_id                BIGINT NOT NULL REFERENCES assignment_jobs(id) ON DELETE CASCADE,
    file_path             TEXT NOT NULL,
    file_size             BIGINT NOT NULL,
    kind                  TEXT NOT NULL CHECK (kind IN ('episode', 'movie', 'extra', 'subtitle')),
    -- Set when the job is classified and never changed, so rows don't jump.
    review_group          TEXT NOT NULL CHECK (review_group IN ('matched', 'needs_review', 'specials', 'extras', 'unmatched')),
    extra_type            TEXT CHECK (extra_type IN (
                              'opening', 'ending', 'sample', 'trailer', 'featurette',
                              'font', 'info', 'image', 'superseded', 'other')),
    -- Target episodes, in order; re-validated when applied.
    episode_ids           BIGINT[] NOT NULL DEFAULT '{}',
    -- What the classifier or AI last suggested: Undo restores it.
    suggested_episode_ids BIGINT[] NOT NULL DEFAULT '{}',
    suggested_confidence  TEXT CHECK (suggested_confidence IN ('high', 'medium', 'low')),
    suggested_source      TEXT NOT NULL CHECK (suggested_source IN ('rules', 'ai', 'manual')),
    suggested_reason      TEXT NOT NULL DEFAULT '',
    confidence            TEXT CHECK (confidence IN ('high', 'medium', 'low')),
    source                TEXT NOT NULL CHECK (source IN ('rules', 'ai', 'manual')),
    reason                TEXT NOT NULL DEFAULT '',   -- at most 120 characters
    decision              TEXT NOT NULL CHECK (decision IN ('pending', 'accepted', 'ignored')),
    user_decided          BOOLEAN NOT NULL DEFAULT FALSE,
    updated               BOOLEAN NOT NULL DEFAULT FALSE,
    hinted                BOOLEAN NOT NULL DEFAULT FALSE,
    superseded_by         BIGINT REFERENCES assignment_proposals(id) ON DELETE SET NULL,
    linked_proposal_id    BIGINT REFERENCES assignment_proposals(id) ON DELETE SET NULL,
    UNIQUE (job_id, file_path)
);

-- Every AI call, shared by the classifier, recommendations and Ask.
CREATE TABLE IF NOT EXISTS ai_usage (
    id                BIGSERIAL PRIMARY KEY,
    feature           TEXT NOT NULL CHECK (feature IN ('classifier', 'recs', 'ask')),
    model             TEXT NOT NULL,
    input_tokens      INTEGER NOT NULL DEFAULT 0,
    output_tokens     INTEGER NOT NULL DEFAULT 0,
    cache_read_tokens INTEGER NOT NULL DEFAULT 0,
    web_searches      INTEGER NOT NULL DEFAULT 0,
    cost_usd          NUMERIC(12, 6) NOT NULL DEFAULT 0,
    ref_id            BIGINT,        -- the job or run it was for
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_ai_usage_created ON ai_usage(created_at);

-- AI answers, keyed by the feature (the classifier: info hash, show,
-- metadata_updated_at and hint). expires_at NULL never expires.
CREATE TABLE IF NOT EXISTS ai_cache (
    key        TEXT PRIMARY KEY,
    feature    TEXT NOT NULL,
    payload    JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_ai_cache_expires ON ai_cache(expires_at);
