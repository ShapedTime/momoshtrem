-- Ask per household member (#239, B3b #246; PLAN §5, §6 "Household
-- members"): each member has their own threads and one streaming turn,
-- and their own daily limit. What was there is the lowest member's (not
-- member 1's: it may be gone by this deploy). member_id has no default, so
-- a writer that forgets the member fails. ask_messages doesn't change.
-- Idempotent, like 006.
--
-- Not reversible by the old binary: roll back from the pg_dump (ops PLAN
-- §6).

-- A member's threads go with the member (DELETE /members/{id}), and a
-- thread's turns and messages with the thread.
ALTER TABLE ask_threads ADD COLUMN IF NOT EXISTS member_id BIGINT;
UPDATE ask_threads SET member_id = (SELECT min(id) FROM members) WHERE member_id IS NULL;
ALTER TABLE ask_threads ALTER COLUMN member_id SET NOT NULL;
ALTER TABLE ask_threads DROP CONSTRAINT IF EXISTS ask_threads_member_id_fkey,
    ADD CONSTRAINT ask_threads_member_id_fkey FOREIGN KEY (member_id) REFERENCES members (id) ON DELETE CASCADE;

-- A turn is its thread's member's: the composite FK replaces the one on
-- thread_id, so the per-member streaming index below can't be bypassed.
ALTER TABLE ask_turns ADD COLUMN IF NOT EXISTS member_id BIGINT;
UPDATE ask_turns t SET member_id = th.member_id FROM ask_threads th WHERE th.id = t.thread_id AND t.member_id IS NULL;
ALTER TABLE ask_turns ALTER COLUMN member_id SET NOT NULL;
ALTER TABLE ask_turns DROP CONSTRAINT IF EXISTS ask_turns_thread_member_fkey;
ALTER TABLE ask_turns DROP CONSTRAINT IF EXISTS ask_turns_thread_id_fkey;
ALTER TABLE ask_threads DROP CONSTRAINT IF EXISTS ask_threads_id_member_key,
    ADD CONSTRAINT ask_threads_id_member_key UNIQUE (id, member_id);
ALTER TABLE ask_turns ADD CONSTRAINT ask_turns_thread_member_fkey
    FOREIGN KEY (thread_id, member_id) REFERENCES ask_threads (id, member_id) ON DELETE CASCADE;

-- One turn streams at a time per member, in any of their threads
-- (ask_busy).
DROP INDEX IF EXISTS ask_turns_one_streaming;
CREATE UNIQUE INDEX ask_turns_one_streaming ON ask_turns (member_id) WHERE status = 'streaming';
-- The daily limit counts a member's turns since the server-local midnight.
DROP INDEX IF EXISTS idx_ask_turns_created;
CREATE INDEX IF NOT EXISTS idx_ask_turns_member_created ON ask_turns (member_id, created_at);
-- A member's threads, the newest updated first.
CREATE INDEX IF NOT EXISTS idx_ask_threads_member_updated ON ask_threads (member_id, updated_at DESC, id DESC);
