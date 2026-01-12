-- Add source and info_hash columns to subtitles table for torrent-embedded subtitles
-- source: 'opensubtitles' (downloaded from OpenSubtitles API) or 'torrent' (embedded in torrent)
-- info_hash: torrent hash for streaming torrent-embedded subtitles (magnet_uri looked up from torrent_assignments)

ALTER TABLE subtitles ADD COLUMN source TEXT NOT NULL DEFAULT 'opensubtitles';
ALTER TABLE subtitles ADD COLUMN info_hash TEXT;

CREATE INDEX IF NOT EXISTS idx_subtitles_source ON subtitles(source);
