-- B7 subtitles: keep downloaded and torrent subtitles apart, and remember
-- which release a subtitle was made for. Idempotent, like 003.
--
-- Before this, UNIQUE(item_type, item_id, language_code) let a torrent's
-- English subtitle overwrite the one downloaded from OpenSubtitles, and two
-- "unknown"-language files in one torrent overwrote each other.
--
-- Not reversible by deploying the old binary: its upsert names the dropped
-- constraint in ON CONFLICT and fails. Roll back from a pg_dump.

-- The release the subtitle was made for: the OpenSubtitles release name, or
-- the torrent's for subtitles that came with it. NULL when unknown.
ALTER TABLE subtitles ADD COLUMN IF NOT EXISTS release_name TEXT;

-- Drop the old unique constraint by its definition: its name is whatever
-- Postgres generated when 001 ran.
DO $$
DECLARE
    con record;
BEGIN
    FOR con IN
        SELECT conname
        FROM pg_constraint
        WHERE conrelid = 'subtitles'::regclass
          AND contype = 'u'
          AND pg_get_constraintdef(oid) = 'UNIQUE (item_type, item_id, language_code)'
    LOOP
        EXECUTE format('ALTER TABLE subtitles DROP CONSTRAINT %I', con.conname);
    END LOOP;
END $$;

-- One downloaded subtitle per language: a new download replaces it.
CREATE UNIQUE INDEX IF NOT EXISTS subtitles_downloaded_language_key
    ON subtitles (item_type, item_id, language_code)
    WHERE source = 'opensubtitles';

-- Torrent subtitles are keyed by the file they stream from, so several files
-- in one language (or of unknown language) coexist.
CREATE UNIQUE INDEX IF NOT EXISTS subtitles_torrent_file_key
    ON subtitles (item_type, item_id, info_hash, file_path)
    WHERE source = 'torrent';

-- A torrent subtitle streams from its torrent, so it needs the info hash.
-- Rows without one can't be opened (the VFS finds the torrent by hash) and
-- were never written by the app; drop them so the CHECK holds.
DELETE FROM subtitles WHERE source = 'torrent' AND COALESCE(info_hash, '') = '';

ALTER TABLE subtitles DROP CONSTRAINT IF EXISTS subtitles_torrent_info_hash_check;
ALTER TABLE subtitles ADD CONSTRAINT subtitles_torrent_info_hash_check
    CHECK (source <> 'torrent' OR COALESCE(info_hash, '') <> '');
