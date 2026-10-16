-- AI recommendations and Ask momoshtrem (#9; PLAN §5 "006_recommendations.sql",
-- §6 "Recommendations (R1–R3)"): the taste profile (R1), runs with their
-- shelves and hero picks (R2), feedback on titles, and Ask's threads (R3).
-- New tables only. Idempotent, like 003; it lands after 007–009 (numbers
-- are reserved per task), and the runner applies every missing version.
--
-- Reversible by deploying the old binary: nothing else reads these tables.

-- R1's taste profile, one row per build. counts ({loved, completed, dropped,
-- rated, plays}) drives no_history (fewer than 5 loved or completed
-- titles); profile is compact JSON, about 16 KB at most. Rows no run
-- references are pruned.
CREATE TABLE IF NOT EXISTS taste_profiles (
    id         BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    counts     JSONB NOT NULL,
    profile    JSONB NOT NULL
);

-- One run of the picks agent. progress is {steps: [{step, state, detail}],
-- shelves_done, shelves_target}, written as the run moves; the usage
-- columns sum its ai_usage rows as they land.
CREATE TABLE IF NOT EXISTS rec_runs (
    id                BIGSERIAL PRIMARY KEY,
    kind              TEXT NOT NULL CHECK (kind IN ('scheduled', 'manual', 'first_sync')),
    status            TEXT NOT NULL CHECK (status IN ('queued', 'running', 'succeeded', 'failed', 'skipped')),
    model             TEXT NOT NULL,
    profile_id        BIGINT REFERENCES taste_profiles(id) ON DELETE SET NULL,
    progress          JSONB NOT NULL DEFAULT '{}'::jsonb,
    skip_reason       TEXT CHECK (skip_reason IN ('budget_exceeded', 'no_history')),
    error             TEXT,   -- for the user
    api_calls         INTEGER NOT NULL DEFAULT 0,
    input_tokens      BIGINT NOT NULL DEFAULT 0,
    output_tokens     BIGINT NOT NULL DEFAULT 0,
    cache_read_tokens BIGINT NOT NULL DEFAULT 0,
    web_searches      INTEGER NOT NULL DEFAULT 0,
    cost_usd          NUMERIC(12, 6) NOT NULL DEFAULT 0,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at        TIMESTAMPTZ,
    finished_at       TIMESTAMPTZ
);
-- At most one run is queued or running.
CREATE UNIQUE INDEX IF NOT EXISTS rec_runs_one_active ON rec_runs ((true))
    WHERE status IN ('queued', 'running');
-- The newest succeeded (or finished) run.
CREATE INDEX IF NOT EXISTS idx_rec_runs_status_finished ON rec_runs (status, finished_at DESC);
CREATE INDEX IF NOT EXISTS idx_rec_runs_profile ON rec_runs (profile_id);

CREATE TABLE IF NOT EXISTS rec_shelves (
    id       BIGSERIAL PRIMARY KEY,
    run_id   BIGINT NOT NULL REFERENCES rec_runs(id) ON DELETE CASCADE,
    position INTEGER NOT NULL,
    slug     TEXT NOT NULL,
    kind     TEXT NOT NULL CHECK (kind IN ('because', 'new', 'anime', 'gems', 'coming_soon', 'other')),
    title    TEXT NOT NULL,
    subtitle TEXT,
    UNIQUE (run_id, position),
    UNIQUE (run_id, slug)
);

-- meta is the title's snapshot at write time (title, year, year_end,
-- poster_path, seasons, runtime_minutes, vote_average, release_date, badge),
-- so reads never call TMDB. signals is why the agent picked it (profile
-- keys, candidate source), for debugging; never served.
CREATE TABLE IF NOT EXISTS rec_items (
    id         BIGSERIAL PRIMARY KEY,
    shelf_id   BIGINT NOT NULL REFERENCES rec_shelves(id) ON DELETE CASCADE,
    position   INTEGER NOT NULL,
    media_type TEXT NOT NULL CHECK (media_type IN ('movie', 'tv')),
    tmdb_id    INTEGER NOT NULL,
    reason     TEXT NOT NULL,
    meta       JSONB NOT NULL DEFAULT '{}'::jsonb,
    signals    JSONB,
    UNIQUE (shelf_id, media_type, tmdb_id)
);
-- The title page's reason.
CREATE INDEX IF NOT EXISTS idx_rec_items_title ON rec_items (media_type, tmdb_id);

-- Tonight's picks: a run writes 3–9 and /home shows three a day.
CREATE TABLE IF NOT EXISTS rec_hero_picks (
    id         BIGSERIAL PRIMARY KEY,
    run_id     BIGINT NOT NULL REFERENCES rec_runs(id) ON DELETE CASCADE,
    position   INTEGER NOT NULL,
    media_type TEXT NOT NULL CHECK (media_type IN ('movie', 'tv')),
    tmdb_id    INTEGER NOT NULL,
    kicker     TEXT NOT NULL,
    reason     TEXT NOT NULL,
    meta       JSONB NOT NULL DEFAULT '{}'::jsonb,
    UNIQUE (run_id, position),
    UNIQUE (run_id, media_type, tmdb_id)
);

-- Feedback on a title: the newest signal wins. dislike, not_interested and
-- seen drop it from every AI surface and from the candidate pools; seen
-- also counts as a completed, neutral title in the profile. Not Trakt
-- data: a Trakt disconnect keeps it.
CREATE TABLE IF NOT EXISTS rec_feedback (
    media_type TEXT NOT NULL CHECK (media_type IN ('movie', 'tv')),
    tmdb_id    INTEGER NOT NULL,
    signal     TEXT NOT NULL CHECK (signal IN ('like', 'dislike', 'not_interested', 'seen')),
    source     TEXT NOT NULL CHECK (source IN ('shelf', 'hero', 'ask', 'title')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (media_type, tmdb_id)
);

-- An Ask conversation. model is the first turn's and stays for the thread's
-- life (thinking blocks are bound to the model); prompt_version names the
-- system prompt it started with.
CREATE TABLE IF NOT EXISTS ask_threads (
    id             BIGSERIAL PRIMARY KEY,
    title          TEXT NOT NULL,
    model          TEXT NOT NULL,
    prompt_version TEXT NOT NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- One ask and its answer. view is what the thread page shows, written when
-- the turn ends: {steps, intro, items, refinements, error, elapsed_ms}.
CREATE TABLE IF NOT EXISTS ask_turns (
    id          BIGSERIAL PRIMARY KEY,
    thread_id   BIGINT NOT NULL REFERENCES ask_threads(id) ON DELETE CASCADE,
    position    INTEGER NOT NULL,
    message     TEXT NOT NULL,
    status      TEXT NOT NULL CHECK (status IN ('streaming', 'done', 'error', 'cancelled')),
    view        JSONB,
    error_code  TEXT,
    cost_usd    NUMERIC(12, 6) NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at TIMESTAMPTZ,
    UNIQUE (thread_id, position)
);
-- The daily limit counts turns started since the server-local midnight.
CREATE INDEX IF NOT EXISTS idx_ask_turns_created ON ask_turns (created_at);
-- One turn streams at a time, in any thread (ask_busy).
CREATE UNIQUE INDEX IF NOT EXISTS ask_turns_one_streaming ON ask_turns ((true))
    WHERE status = 'streaming';

-- The raw API content blocks of a thread, append-only: a turn's messages
-- are written in one transaction, and only when it ends done, so a
-- cancelled or failed turn never replays (no orphaned tool_use). content
-- is json, not jsonb: jsonb reorders keys and drops whitespace, and a
-- replay must be byte-identical (preserved thinking and prompt caching
-- compare the request prefix).
CREATE TABLE IF NOT EXISTS ask_messages (
    id         BIGSERIAL PRIMARY KEY,
    thread_id  BIGINT NOT NULL REFERENCES ask_threads(id) ON DELETE CASCADE,
    turn_id    BIGINT NOT NULL REFERENCES ask_turns(id) ON DELETE CASCADE,
    position   INTEGER NOT NULL,
    role       TEXT NOT NULL CHECK (role IN ('user', 'assistant')),
    content    JSON NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (thread_id, position)
);
CREATE INDEX IF NOT EXISTS idx_ask_messages_turn ON ask_messages (turn_id);
