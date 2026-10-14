package library

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"sort"
	"strconv"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// migrationLockID is the pg_advisory_lock key held while migrating, so two
// processes starting at once (server and backfill, or parallel test packages)
// don't apply the same migration twice. ASCII "momomigr".
const migrationLockID int64 = 0x6d6f6d6f6d696772

// DB wraps the SQL database connection
type DB struct {
	*sql.DB
}

// NewDB opens a Postgres connection and runs pending migrations.
func NewDB(connStr string) (*DB, error) {
	sqlDB, err := sql.Open("pgx", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := sqlDB.Ping(); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	db := NewDBFromSQL(sqlDB)
	if err := db.Migrate(context.Background()); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	return db, nil
}

// NewDBFromSQL wraps an already open connection pool, for callers that need
// to configure the driver themselves (e.g. a pgx QueryTracer in tests). It
// does not migrate; call Migrate.
func NewDBFromSQL(sqlDB *sql.DB) *DB {
	return &DB{DB: sqlDB}
}

// migration is one embedded SQL file.
type migration struct {
	Version int
	Name    string
}

// Migrate applies every embedded migration that isn't recorded in
// schema_migrations, in version order, each in its own transaction. It holds
// a Postgres advisory lock for the duration, so concurrent callers wait
// rather than race.
func (db *DB) Migrate(ctx context.Context) error {
	all, err := embeddedMigrations(migrationsFS)
	if err != nil {
		return err
	}

	// Advisory locks belong to a session, so lock, migrate and unlock on one
	// connection.
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("failed to get connection: %w", err)
	}
	defer conn.Close()

	if _, err := conn.ExecContext(ctx, `SELECT pg_advisory_lock($1)`, migrationLockID); err != nil {
		return fmt.Errorf("failed to take migration lock: %w", err)
	}
	defer func() {
		if _, err := conn.ExecContext(context.Background(), `SELECT pg_advisory_unlock($1)`, migrationLockID); err != nil {
			slog.Warn("Failed to release migration lock", "error", err)
		}
	}()

	if _, err := conn.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at TIMESTAMPTZ DEFAULT NOW()
		)
	`); err != nil {
		return fmt.Errorf("failed to create migrations table: %w", err)
	}

	applied, err := appliedVersions(ctx, conn)
	if err != nil {
		return err
	}

	pending, unknown := pendingMigrations(all, applied)
	if len(unknown) > 0 {
		slog.Warn("Database has migrations this binary doesn't know; is it older than the database?",
			"versions", unknown)
	}

	for _, m := range pending {
		slog.Info("Applying migration", "version", m.Version, "name", m.Name)

		content, err := migrationsFS.ReadFile("migrations/" + m.Name)
		if err != nil {
			return fmt.Errorf("failed to read migration %s: %w", m.Name, err)
		}
		if err := applyMigration(ctx, conn, m.Version, string(content)); err != nil {
			return fmt.Errorf("failed to apply migration %s: %w", m.Name, err)
		}
	}

	return nil
}

// embeddedMigrations lists the NNN_name.sql files in fsys/migrations, sorted
// by version. Two files with the same version are an error.
func embeddedMigrations(fsys fs.FS) ([]migration, error) {
	entries, err := fs.ReadDir(fsys, "migrations")
	if err != nil {
		return nil, fmt.Errorf("failed to read migrations directory: %w", err)
	}

	var out []migration
	seen := make(map[int]string)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		// "001_core_schema.sql" -> 1
		prefix, _, ok := strings.Cut(entry.Name(), "_")
		if !ok {
			continue
		}
		version, err := strconv.Atoi(prefix)
		if err != nil {
			continue
		}
		if other, dup := seen[version]; dup {
			return nil, fmt.Errorf("migrations %s and %s share version %d", other, entry.Name(), version)
		}
		seen[version] = entry.Name()
		out = append(out, migration{Version: version, Name: entry.Name()})
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Version < out[j].Version })
	return out, nil
}

// pendingMigrations returns the migrations in all (sorted by version) that
// aren't in applied, and the applied versions that have no embedded file.
//
// Every missing version is pending, not just those above the highest applied
// one: migration numbers are reserved per task, so 004 can land after 005.
func pendingMigrations(all []migration, applied []int) (pending []migration, unknown []int) {
	done := make(map[int]bool, len(applied))
	for _, v := range applied {
		done[v] = true
	}
	embedded := make(map[int]bool, len(all))
	for _, m := range all {
		embedded[m.Version] = true
		if !done[m.Version] {
			pending = append(pending, m)
		}
	}
	for _, v := range applied {
		if !embedded[v] {
			unknown = append(unknown, v)
		}
	}
	sort.Ints(unknown)
	return pending, unknown
}

func appliedVersions(ctx context.Context, conn *sql.Conn) ([]int, error) {
	rows, err := conn.QueryContext(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, fmt.Errorf("failed to read applied migrations: %w", err)
	}
	defer rows.Close()

	var versions []int
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return nil, fmt.Errorf("failed to scan migration version: %w", err)
		}
		versions = append(versions, v)
	}
	return versions, rows.Err()
}

// applyMigration runs one migration and records it, in a single transaction.
func applyMigration(ctx context.Context, conn *sql.Conn, version int, content string) error {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, content); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (version) VALUES ($1)`, version); err != nil {
		return err
	}
	return tx.Commit()
}

// SizeBytes returns the on-disk size of the current database.
func (db *DB) SizeBytes(ctx context.Context) (int64, error) {
	var size int64
	if err := db.QueryRowContext(ctx, `SELECT pg_database_size(current_database())`).Scan(&size); err != nil {
		return 0, fmt.Errorf("failed to get database size: %w", err)
	}
	return size, nil
}

// Close closes the database connection
func (db *DB) Close() error {
	return db.DB.Close()
}
