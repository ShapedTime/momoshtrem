package library

import (
	"database/sql"
	"embed"
	"fmt"
	"log/slog"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// DB wraps the SQL database connection
type DB struct {
	*sql.DB
}

// NewDB creates a new database connection and runs migrations
func NewDB(path string) (*DB, error) {
	// Ensure parent directory exists (handled by config.EnsureDirectories)

	// Connect to SQLite database
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Enable foreign keys
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	// Enable WAL mode for better concurrent access
	if _, err := db.Exec("PRAGMA journal_mode = WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to enable WAL mode: %w", err)
	}

	wrapper := &DB{DB: db}

	// Run migrations
	if err := wrapper.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	return wrapper, nil
}

// migrate runs all pending migrations
func (db *DB) migrate() error {
	// Create migrations table if not exists
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create migrations table: %w", err)
	}

	// Get current version
	var currentVersion int
	row := db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_migrations")
	if err := row.Scan(&currentVersion); err != nil {
		return fmt.Errorf("failed to get current version: %w", err)
	}

	// Read migration files
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		// Fallback: try reading from filesystem directly for development
		return db.migrateFromFilesystem(currentVersion)
	}

	// Sort migrations by version number
	var migrations []struct {
		version int
		name    string
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		// Parse version from filename (e.g., "001_core_schema.sql" -> 1)
		parts := strings.SplitN(entry.Name(), "_", 2)
		if len(parts) < 2 {
			continue
		}
		version, err := strconv.Atoi(parts[0])
		if err != nil {
			continue
		}
		migrations = append(migrations, struct {
			version int
			name    string
		}{version, entry.Name()})
	}

	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].version < migrations[j].version
	})

	// Apply pending migrations
	for _, m := range migrations {
		if m.version <= currentVersion {
			continue
		}

		slog.Info("Applying migration", "version", m.version, "name", m.name)

		content, err := migrationsFS.ReadFile(filepath.Join("migrations", m.name))
		if err != nil {
			return fmt.Errorf("failed to read migration %s: %w", m.name, err)
		}

		if err := db.applyMigration(m.version, string(content)); err != nil {
			return fmt.Errorf("failed to apply migration %s: %w", m.name, err)
		}
	}

	return nil
}

// migrateFromFilesystem handles migrations when embedded files aren't available
func (db *DB) migrateFromFilesystem(currentVersion int) error {
	// For development/debugging, just run the initial schema directly
	if currentVersion < 1 {
		slog.Info("Applying initial schema")
		schema := `
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
`
		if err := db.applyMigration(1, schema); err != nil {
			return err
		}
	}
	return nil
}

// applyMigration runs a migration within a transaction
func (db *DB) applyMigration(version int, content string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Execute migration
	if _, err := tx.Exec(content); err != nil {
		return err
	}

	// Record migration
	if _, err := tx.Exec("INSERT INTO schema_migrations (version) VALUES (?)", version); err != nil {
		return err
	}

	return tx.Commit()
}

// Close closes the database connection
func (db *DB) Close() error {
	return db.DB.Close()
}
