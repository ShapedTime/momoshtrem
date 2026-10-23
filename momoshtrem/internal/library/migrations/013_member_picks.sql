-- Picks per household member (#239, B3a #245; PLAN §5, §6 "Household
-- members"): each member has their own taste profiles, runs and feedback,
-- and one active run. What was there is the lowest member's (not member
-- 1's: it may be gone by this deploy). member_id has no default, so a
-- writer that forgets the member fails. Shelves, items and hero picks
-- don't change: they belong to their run. Idempotent, like 006.
--
-- Not reversible by the old binary: roll back from the pg_dump (ops PLAN
-- §6).

-- A member's rows go with the member (DELETE /members/{id}), and a run's
-- shelves and hero picks with the run.
DO $$
DECLARE
    t text;
    owner bigint := (SELECT min(id) FROM members);
BEGIN
    FOREACH t IN ARRAY ARRAY['taste_profiles', 'rec_runs', 'rec_feedback']
    LOOP
        EXECUTE format('ALTER TABLE %I ADD COLUMN IF NOT EXISTS member_id BIGINT', t);
        EXECUTE format('UPDATE %I SET member_id = $1 WHERE member_id IS NULL', t) USING owner;
        EXECUTE format('ALTER TABLE %I ALTER COLUMN member_id SET NOT NULL', t);
        EXECUTE format('ALTER TABLE %I DROP CONSTRAINT IF EXISTS %I', t, t || '_member_id_fkey');
        EXECUTE format('ALTER TABLE %I ADD CONSTRAINT %I FOREIGN KEY (member_id) REFERENCES members (id) ON DELETE CASCADE',
            t, t || '_member_id_fkey');
    END LOOP;
END $$;

-- At most one run per member is queued or running.
DROP INDEX IF EXISTS rec_runs_one_active;
CREATE UNIQUE INDEX rec_runs_one_active ON rec_runs (member_id) WHERE status IN ('queued', 'running');
-- A member's newest succeeded (or finished) run, and their runs newest
-- first.
DROP INDEX IF EXISTS idx_rec_runs_status_finished;
CREATE INDEX IF NOT EXISTS idx_rec_runs_member_finished ON rec_runs (member_id, status, finished_at DESC);
CREATE INDEX IF NOT EXISTS idx_rec_runs_member ON rec_runs (member_id, id DESC);
-- The worker's next run: the oldest queued, whoever's it is.
CREATE INDEX IF NOT EXISTS idx_rec_runs_queued ON rec_runs (created_at, id) WHERE status = 'queued';
-- A member's newest profile.
CREATE INDEX IF NOT EXISTS idx_taste_profiles_member ON taste_profiles (member_id, id DESC);

-- Feedback is each member's: one signal per member and title.
ALTER TABLE rec_feedback DROP CONSTRAINT IF EXISTS rec_feedback_pkey,
    ADD CONSTRAINT rec_feedback_pkey PRIMARY KEY (member_id, media_type, tmdb_id);
