-- HDRezka (#63): an item's file comes from a torrent or from an HDRezka
-- stream. One table with a kind column: torrent rows keep their hash and
-- magnet; HDRezka rows have neither and a stream_ref instead (the title,
-- translator and quality: library.StreamRef). Idempotent, like 003.
--
-- An HDRezka row's file_path is made up from its stream_ref
-- ("hdrezka/2259/t56/movie/1080p.mp4"): the VFS takes the .mp4 extension
-- from it, and it keys the row, so another translator or quality is another
-- row. It's derived, never parsed back.
--
-- Not reversible by deploying the old binary once HDRezka rows exist: it
-- scans info_hash and magnet_uri into strings, so one active HDRezka row
-- fails the VFS queries and the Movies folder (or a show) vanishes from
-- WebDAV. Roll back from the pg_dump (ops PLAN §6). As an escape hatch,
-- hide the rows from it:
--   UPDATE torrent_assignments SET is_active = FALSE WHERE kind = 'hdrezka';

ALTER TABLE torrent_assignments
    ADD COLUMN IF NOT EXISTS kind       TEXT NOT NULL DEFAULT 'torrent',
    ADD COLUMN IF NOT EXISTS stream_ref JSONB;

ALTER TABLE torrent_assignments
    ALTER COLUMN info_hash  DROP NOT NULL,
    ALTER COLUMN magnet_uri DROP NOT NULL;

-- A torrent row streams from its torrent; an HDRezka row from its stream,
-- and no classifier mapped it (the VFS never groups it).
ALTER TABLE torrent_assignments DROP CONSTRAINT IF EXISTS torrent_assignments_kind_check;
ALTER TABLE torrent_assignments ADD CONSTRAINT torrent_assignments_kind_check CHECK (
    (kind = 'torrent' AND info_hash IS NOT NULL AND magnet_uri IS NOT NULL AND stream_ref IS NULL)
 OR (kind = 'hdrezka' AND info_hash IS NULL AND magnet_uri IS NULL AND stream_ref IS NOT NULL
     AND match_source IS NULL AND confidence IS NULL));

-- UNIQUE(item_type, item_id, info_hash, file_path) never matches a NULL
-- hash, so HDRezka rows get their own key: the writer's upsert arbiter.
-- Its predicate must stay exactly kind = 'hdrezka' to be inferred.
CREATE UNIQUE INDEX IF NOT EXISTS torrent_assignments_hdrezka_key
    ON torrent_assignments (item_type, item_id, file_path)
    WHERE kind = 'hdrezka';
