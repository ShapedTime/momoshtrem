package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

func main() {
	sqlitePath := flag.String("sqlite-path", "", "Path to SQLite database file")
	pgURL := flag.String("pg-url", "", "PostgreSQL connection URL")
	flag.Parse()

	if *sqlitePath == "" || *pgURL == "" {
		fmt.Fprintf(os.Stderr, "Usage: migrate-to-pg --sqlite-path /path/to/momoshtrem.db --pg-url postgres://...\n")
		os.Exit(1)
	}

	// Open SQLite
	sqliteDB, err := sql.Open("sqlite", *sqlitePath)
	if err != nil {
		log.Fatalf("Failed to open SQLite: %v", err)
	}
	defer sqliteDB.Close()

	// Verify SQLite is accessible
	if err := sqliteDB.Ping(); err != nil {
		log.Fatalf("Failed to ping SQLite: %v", err)
	}
	log.Println("Connected to SQLite")

	// Open PostgreSQL
	pgDB, err := sql.Open("pgx", *pgURL)
	if err != nil {
		log.Fatalf("Failed to open PostgreSQL: %v", err)
	}
	defer pgDB.Close()

	if err := pgDB.Ping(); err != nil {
		log.Fatalf("Failed to ping PostgreSQL: %v", err)
	}
	log.Println("Connected to PostgreSQL")

	// Start transaction
	tx, err := pgDB.Begin()
	if err != nil {
		log.Fatalf("Failed to start transaction: %v", err)
	}
	defer tx.Rollback()

	// Truncate all target tables for idempotent re-runs (reverse FK order)
	truncateOrder := []string{"subtitles", "torrent_assignments", "episodes", "seasons", "shows", "movies", "sync_metadata"}
	for _, table := range truncateOrder {
		if _, err := tx.Exec(fmt.Sprintf("TRUNCATE TABLE %s CASCADE", table)); err != nil {
			log.Fatalf("Failed to truncate %s: %v", table, err)
		}
	}
	log.Println("Truncated all target tables")

	// Migrate tables in FK-dependency order
	tables := []struct {
		name    string
		columns string
	}{
		{"movies", "id, tmdb_id, title, year, created_at"},
		{"shows", "id, tmdb_id, title, year, created_at"},
		{"seasons", "id, show_id, season_number"},
		{"episodes", "id, season_id, episode_number, name, air_date"},
		{"torrent_assignments", "id, item_type, item_id, info_hash, magnet_uri, file_path, file_size, resolution, source, is_active, created_at"},
		{"subtitles", "id, item_type, item_id, language_code, language_name, format, file_path, file_size, source, info_hash, created_at"},
		{"sync_metadata", "key, value, updated_at"},
	}

	for _, table := range tables {
		count, err := migrateTable(sqliteDB, tx, table.name, table.columns)
		if err != nil {
			log.Fatalf("Failed to migrate table %s: %v", table.name, err)
		}
		log.Printf("Migrated %s: %d rows", table.name, count)
	}

	// Reset sequences for tables with BIGSERIAL
	sequenceTables := []string{"movies", "shows", "seasons", "episodes", "torrent_assignments", "subtitles"}
	for _, table := range sequenceTables {
		_, err := tx.Exec(fmt.Sprintf(
			"SELECT setval('%s_id_seq', COALESCE((SELECT MAX(id) FROM %s), 1), (SELECT COUNT(*) > 0 FROM %s))",
			table, table, table,
		))
		if err != nil {
			log.Fatalf("Failed to reset sequence for %s: %v", table, err)
		}
		log.Printf("Reset sequence for %s", table)
	}

	// Verify row counts
	log.Println("Verifying row counts...")
	for _, table := range tables {
		var sqliteCount, pgCount int64

		err := sqliteDB.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s", table.name)).Scan(&sqliteCount)
		if err != nil {
			log.Fatalf("Failed to count SQLite rows for %s: %v", table.name, err)
		}

		err = tx.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s", table.name)).Scan(&pgCount)
		if err != nil {
			log.Fatalf("Failed to count PG rows for %s: %v", table.name, err)
		}

		if sqliteCount != pgCount {
			log.Fatalf("Row count mismatch for %s: SQLite=%d, PG=%d", table.name, sqliteCount, pgCount)
		}
		log.Printf("Verified %s: %d rows match", table.name, sqliteCount)
	}

	// Commit
	if err := tx.Commit(); err != nil {
		log.Fatalf("Failed to commit transaction: %v", err)
	}

	log.Println("Migration completed successfully!")
}

func migrateTable(sqliteDB *sql.DB, tx *sql.Tx, tableName, columns string) (int64, error) {
	// Check if table exists in SQLite (subtitles or sync_metadata might not exist in old DBs)
	var tableExists int
	err := sqliteDB.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?",
		tableName,
	).Scan(&tableExists)
	if err != nil {
		return 0, fmt.Errorf("failed to check table existence: %w", err)
	}
	if tableExists == 0 {
		log.Printf("Table %s does not exist in SQLite, skipping", tableName)
		return 0, nil
	}

	// Read from SQLite
	rows, err := sqliteDB.Query(fmt.Sprintf("SELECT %s FROM %s", columns, tableName))
	if err != nil {
		return 0, fmt.Errorf("failed to query SQLite: %w", err)
	}
	defer rows.Close()

	colNames, err := rows.Columns()
	if err != nil {
		return 0, fmt.Errorf("failed to get columns: %w", err)
	}

	// Build INSERT statement with numbered placeholders
	placeholders := make([]string, len(colNames))
	for i := range colNames {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
	}

	// Use OVERRIDING SYSTEM VALUE for BIGSERIAL tables to preserve IDs
	insertSQL := fmt.Sprintf(
		"INSERT INTO %s (%s) OVERRIDING SYSTEM VALUE VALUES (%s)",
		tableName, columns, strings.Join(placeholders, ", "),
	)

	// sync_metadata doesn't have a SERIAL column, so don't use OVERRIDING SYSTEM VALUE
	if tableName == "sync_metadata" {
		insertSQL = fmt.Sprintf(
			"INSERT INTO %s (%s) VALUES (%s)",
			tableName, columns, strings.Join(placeholders, ", "),
		)
	}

	stmt, err := tx.Prepare(insertSQL)
	if err != nil {
		return 0, fmt.Errorf("failed to prepare insert: %w", err)
	}
	defer stmt.Close()

	var count int64
	for rows.Next() {
		// Create a slice of interface{} to hold the values
		values := make([]interface{}, len(colNames))
		valuePtrs := make([]interface{}, len(colNames))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			return 0, fmt.Errorf("failed to scan row: %w", err)
		}

		// Convert SQLite boolean (0/1) to PostgreSQL boolean for is_active column
		for i, colName := range colNames {
			if colName == "is_active" {
				if v, ok := values[i].(int64); ok {
					values[i] = v != 0
				}
			}
		}

		if _, err := stmt.Exec(values...); err != nil {
			return 0, fmt.Errorf("failed to insert row: %w", err)
		}
		count++
	}

	return count, rows.Err()
}
