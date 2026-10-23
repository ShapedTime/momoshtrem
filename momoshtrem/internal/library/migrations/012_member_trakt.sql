-- Trakt per household member (#239, B2a #243; PLAN §5, §6 "Household
-- members"): every Trakt table's rows are one member's, trakt_auth holds
-- one connection per member and an account connects to one member, and the
-- Trakt settings move to the member. What was there is the lowest member's
-- (not member 1's: it may be gone by this deploy). member_id has no
-- default, so a writer that forgets the member fails. Idempotent, like 006.
--
-- Not reversible by the old binary: roll back from the pg_dump (ops PLAN
-- §6).

-- The member's Trakt settings, read over their defaults and deleted with
-- the member.
ALTER TABLE members ADD COLUMN IF NOT EXISTS trakt_settings JSONB NOT NULL DEFAULT '{}'::jsonb;
UPDATE members m SET trakt_settings = s.value FROM app_settings s
WHERE s.key = 'trakt' AND jsonb_typeof(s.value) = 'object' AND m.id = (SELECT min(id) FROM members);
DELETE FROM app_settings WHERE key = 'trakt';

-- A member's rows go with the member (DELETE /members/{id}).
DO $$
DECLARE
    t text;
    owner bigint := (SELECT min(id) FROM members);
BEGIN
    FOREACH t IN ARRAY ARRAY['trakt_auth', 'trakt_sync_state', 'trakt_history', 'trakt_watched_movies',
        'trakt_watched_episodes', 'trakt_ratings', 'trakt_watchlist', 'trakt_show_progress', 'trakt_hidden',
        'trakt_calendar']
    LOOP
        EXECUTE format('ALTER TABLE %I ADD COLUMN IF NOT EXISTS member_id BIGINT', t);
        EXECUTE format('UPDATE %I SET member_id = $1 WHERE member_id IS NULL', t) USING owner;
        EXECUTE format('ALTER TABLE %I ALTER COLUMN member_id SET NOT NULL', t);
        EXECUTE format('ALTER TABLE %I DROP CONSTRAINT IF EXISTS %I', t, t || '_member_id_fkey');
        EXECUTE format('ALTER TABLE %I ADD CONSTRAINT %I FOREIGN KEY (member_id) REFERENCES members (id) ON DELETE CASCADE',
            t, t || '_member_id_fkey');
    END LOOP;
END $$;

-- One connection per member; dropping id takes its CHECK (id = 1) and the
-- old primary key with it. An account connects to one member: connecting
-- it for another fails on trakt_auth_user_slug_key (account_in_use).
ALTER TABLE trakt_auth DROP COLUMN IF EXISTS id;
ALTER TABLE trakt_auth DROP CONSTRAINT IF EXISTS trakt_auth_pkey,
    ADD CONSTRAINT trakt_auth_pkey PRIMARY KEY (member_id);
CREATE UNIQUE INDEX IF NOT EXISTS trakt_auth_user_slug_key ON trakt_auth (user_slug);

-- The keys lead with the member, but the watched tables': they lead with
-- the title, for the household's reads (any member's play), and have an
-- index on the member for the member's own.
ALTER TABLE trakt_sync_state DROP CONSTRAINT IF EXISTS trakt_sync_state_pkey,
    ADD CONSTRAINT trakt_sync_state_pkey PRIMARY KEY (member_id, key);
ALTER TABLE trakt_history DROP CONSTRAINT IF EXISTS trakt_history_pkey,
    ADD CONSTRAINT trakt_history_pkey PRIMARY KEY (member_id, id);
ALTER TABLE trakt_watched_movies DROP CONSTRAINT IF EXISTS trakt_watched_movies_pkey,
    ADD CONSTRAINT trakt_watched_movies_pkey PRIMARY KEY (tmdb_id, member_id);
CREATE INDEX IF NOT EXISTS idx_trakt_watched_movies_member ON trakt_watched_movies (member_id);
ALTER TABLE trakt_watched_episodes DROP CONSTRAINT IF EXISTS trakt_watched_episodes_pkey,
    ADD CONSTRAINT trakt_watched_episodes_pkey PRIMARY KEY (show_tmdb_id, season, episode, member_id);
CREATE INDEX IF NOT EXISTS idx_trakt_watched_episodes_member ON trakt_watched_episodes (member_id);
ALTER TABLE trakt_ratings DROP CONSTRAINT IF EXISTS trakt_ratings_pkey,
    ADD CONSTRAINT trakt_ratings_pkey PRIMARY KEY (member_id, media_type, trakt_id);
ALTER TABLE trakt_watchlist DROP CONSTRAINT IF EXISTS trakt_watchlist_pkey,
    ADD CONSTRAINT trakt_watchlist_pkey PRIMARY KEY (member_id, media_type, trakt_id);
ALTER TABLE trakt_show_progress DROP CONSTRAINT IF EXISTS trakt_show_progress_pkey,
    ADD CONSTRAINT trakt_show_progress_pkey PRIMARY KEY (member_id, show_trakt_id);
ALTER TABLE trakt_hidden DROP CONSTRAINT IF EXISTS trakt_hidden_pkey,
    ADD CONSTRAINT trakt_hidden_pkey PRIMARY KEY (member_id, section, trakt_id);
ALTER TABLE trakt_calendar DROP CONSTRAINT IF EXISTS trakt_calendar_pkey,
    ADD CONSTRAINT trakt_calendar_pkey PRIMARY KEY (member_id, show_trakt_id, season, episode);

-- The personal reads' indexes lead with the member. The by-title indexes
-- of progress, hidden shows and the calendar's show
-- (idx_trakt_show_progress_tmdb, idx_trakt_hidden_tmdb,
-- idx_trakt_calendar_show) and idx_trakt_watched_episodes_tmdb stay, for
-- the household's lookups.
DROP INDEX IF EXISTS idx_trakt_history_watched;
CREATE INDEX idx_trakt_history_watched ON trakt_history (member_id, watched_at DESC, id DESC);
DROP INDEX IF EXISTS idx_trakt_history_title;
CREATE INDEX idx_trakt_history_title ON trakt_history (member_id, media_type, tmdb_id);
DROP INDEX IF EXISTS idx_trakt_ratings_tmdb;
CREATE INDEX idx_trakt_ratings_tmdb ON trakt_ratings (member_id, media_type, tmdb_id);
DROP INDEX IF EXISTS idx_trakt_watchlist_tmdb;
CREATE INDEX idx_trakt_watchlist_tmdb ON trakt_watchlist (member_id, media_type, tmdb_id);
DROP INDEX IF EXISTS idx_trakt_watchlist_listed;
CREATE INDEX idx_trakt_watchlist_listed ON trakt_watchlist (member_id, listed_at DESC);
DROP INDEX IF EXISTS idx_trakt_calendar_aired;
CREATE INDEX idx_trakt_calendar_aired ON trakt_calendar (member_id, first_aired);
