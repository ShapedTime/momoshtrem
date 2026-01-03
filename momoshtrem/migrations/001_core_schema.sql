-- Core schema for momoshtrem
-- Movies, Shows, Seasons, Episodes, and Torrent Assignments

-- Movies (minimal: just ID + title/year for VFS path generation)
CREATE TABLE IF NOT EXISTS movies (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    tmdb_id INTEGER UNIQUE NOT NULL,
    title TEXT NOT NULL,
    year INTEGER NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_movies_tmdb ON movies(tmdb_id);

-- Shows (minimal: just ID + title/year for VFS path)
CREATE TABLE IF NOT EXISTS shows (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    tmdb_id INTEGER UNIQUE NOT NULL,
    title TEXT NOT NULL,
    year INTEGER NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_shows_tmdb ON shows(tmdb_id);

-- Seasons (minimal: just show + season number)
CREATE TABLE IF NOT EXISTS seasons (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    show_id INTEGER NOT NULL REFERENCES shows(id) ON DELETE CASCADE,
    season_number INTEGER NOT NULL,
    UNIQUE(show_id, season_number)
);

CREATE INDEX IF NOT EXISTS idx_seasons_show ON seasons(show_id);

-- Episodes (minimal: just season + episode number + name for VFS)
CREATE TABLE IF NOT EXISTS episodes (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    season_id INTEGER NOT NULL REFERENCES seasons(id) ON DELETE CASCADE,
    episode_number INTEGER NOT NULL,
    name TEXT,
    UNIQUE(season_id, episode_number)
);

CREATE INDEX IF NOT EXISTS idx_episodes_season ON episodes(season_id);

-- Torrent Assignments (links library items to torrent files)
CREATE TABLE IF NOT EXISTS torrent_assignments (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    item_type TEXT NOT NULL CHECK(item_type IN ('movie', 'episode')),
    item_id INTEGER NOT NULL,
    info_hash TEXT NOT NULL,
    magnet_uri TEXT NOT NULL,
    file_path TEXT NOT NULL,
    file_size INTEGER NOT NULL,
    resolution TEXT,
    source TEXT,
    is_active BOOLEAN DEFAULT TRUE,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(item_type, item_id, info_hash, file_path)
);

CREATE INDEX IF NOT EXISTS idx_assignments_item ON torrent_assignments(item_type, item_id);
CREATE INDEX IF NOT EXISTS idx_assignments_hash ON torrent_assignments(info_hash);

-- Schema version tracking
CREATE TABLE IF NOT EXISTS schema_migrations (
    version INTEGER PRIMARY KEY,
    applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
