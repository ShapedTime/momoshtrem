-- HDRezka subtitles (#68): the subtitles an HDRezka stream offers are
-- downloaded when it's assigned (their links expire), converted to SRT and
-- stored as local files with source = 'hdrezka'. Idempotent, like 003.
--
-- They belong to the stream, like torrent subtitles to their torrent: the
-- assignment writer drops them when the item's file changes, and only the
-- item's active HDRezka stream shows them.
--
-- Reversible by deploying the old binary: it doesn't know the source, so
-- its subtitle list and WebDAV tree show the rows as downloaded subtitles
-- (the files exist), and the index only constrains rows it never writes.

-- One HDRezka subtitle per language: assigning the stream again replaces
-- it (the upsert's arbiter; its predicate must stay exactly
-- source = 'hdrezka' to be inferred).
CREATE UNIQUE INDEX IF NOT EXISTS subtitles_hdrezka_key
    ON subtitles (item_type, item_id, language_code)
    WHERE source = 'hdrezka';
