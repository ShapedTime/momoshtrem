-- Subtitles table: stores downloaded subtitles for library items
CREATE TABLE IF NOT EXISTS subtitles (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    item_type TEXT NOT NULL CHECK(item_type IN ('movie', 'episode')),
    item_id INTEGER NOT NULL,
    language_code TEXT NOT NULL,
    language_name TEXT NOT NULL,
    format TEXT NOT NULL DEFAULT 'srt',
    file_path TEXT NOT NULL,
    file_size INTEGER DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(item_type, item_id, language_code)
);

CREATE INDEX IF NOT EXISTS idx_subtitles_item ON subtitles(item_type, item_id);
