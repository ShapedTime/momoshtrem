-- Add air_date column to episodes for tracking when episodes air
-- This enables the "Recently Aired" feature without hitting TMDB on every login

ALTER TABLE episodes ADD COLUMN air_date TEXT;

CREATE INDEX IF NOT EXISTS idx_episodes_air_date ON episodes(air_date);

-- Sync metadata table for tracking background sync state
CREATE TABLE IF NOT EXISTS sync_metadata (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
