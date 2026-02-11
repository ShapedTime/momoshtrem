-- Core schema for momoshtrem (PostgreSQL)

CREATE TABLE IF NOT EXISTS movies (
    id BIGSERIAL PRIMARY KEY,
    tmdb_id INTEGER UNIQUE NOT NULL,
    title TEXT NOT NULL,
    year INTEGER NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_movies_tmdb ON movies(tmdb_id);

CREATE TABLE IF NOT EXISTS shows (
    id BIGSERIAL PRIMARY KEY,
    tmdb_id INTEGER UNIQUE NOT NULL,
    title TEXT NOT NULL,
    year INTEGER NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_shows_tmdb ON shows(tmdb_id);

CREATE TABLE IF NOT EXISTS seasons (
    id BIGSERIAL PRIMARY KEY,
    show_id BIGINT NOT NULL REFERENCES shows(id) ON DELETE CASCADE,
    season_number INTEGER NOT NULL,
    UNIQUE(show_id, season_number)
);
CREATE INDEX IF NOT EXISTS idx_seasons_show ON seasons(show_id);

CREATE TABLE IF NOT EXISTS episodes (
    id BIGSERIAL PRIMARY KEY,
    season_id BIGINT NOT NULL REFERENCES seasons(id) ON DELETE CASCADE,
    episode_number INTEGER NOT NULL,
    name TEXT,
    air_date TEXT,
    UNIQUE(season_id, episode_number)
);
CREATE INDEX IF NOT EXISTS idx_episodes_season ON episodes(season_id);
CREATE INDEX IF NOT EXISTS idx_episodes_air_date ON episodes(air_date);

CREATE TABLE IF NOT EXISTS torrent_assignments (
    id BIGSERIAL PRIMARY KEY,
    item_type TEXT NOT NULL CHECK(item_type IN ('movie', 'episode')),
    item_id BIGINT NOT NULL,
    info_hash TEXT NOT NULL,
    magnet_uri TEXT NOT NULL,
    file_path TEXT NOT NULL,
    file_size BIGINT NOT NULL,
    resolution TEXT,
    source TEXT,
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(item_type, item_id, info_hash, file_path)
);
CREATE INDEX IF NOT EXISTS idx_assignments_item ON torrent_assignments(item_type, item_id);
CREATE INDEX IF NOT EXISTS idx_assignments_hash ON torrent_assignments(info_hash);

CREATE TABLE IF NOT EXISTS subtitles (
    id BIGSERIAL PRIMARY KEY,
    item_type TEXT NOT NULL CHECK(item_type IN ('movie', 'episode')),
    item_id BIGINT NOT NULL,
    language_code TEXT NOT NULL,
    language_name TEXT NOT NULL,
    format TEXT NOT NULL DEFAULT 'srt',
    file_path TEXT NOT NULL,
    file_size BIGINT DEFAULT 0,
    source TEXT NOT NULL DEFAULT 'opensubtitles',
    info_hash TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(item_type, item_id, language_code)
);
CREATE INDEX IF NOT EXISTS idx_subtitles_item ON subtitles(item_type, item_id);
CREATE INDEX IF NOT EXISTS idx_subtitles_source ON subtitles(source);

CREATE TABLE IF NOT EXISTS sync_metadata (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at TIMESTAMPTZ DEFAULT NOW()
);
