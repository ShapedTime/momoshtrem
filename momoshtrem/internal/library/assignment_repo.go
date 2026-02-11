package library

import (
	"database/sql"
	"fmt"
)

// AssignmentRepository handles torrent assignment database operations
type AssignmentRepository struct {
	db *DB
}

// NewAssignmentRepository creates a new assignment repository
func NewAssignmentRepository(db *DB) *AssignmentRepository {
	return &AssignmentRepository{db: db}
}

// scanner is an interface for sql.Row and sql.Rows
type scanner interface {
	Scan(dest ...interface{}) error
}

// scanAssignment scans a row into a TorrentAssignment
func scanAssignment(s scanner) (*TorrentAssignment, error) {
	assignment := &TorrentAssignment{}
	var resolution, source sql.NullString

	err := s.Scan(
		&assignment.ID, &assignment.ItemType, &assignment.ItemID,
		&assignment.InfoHash, &assignment.MagnetURI, &assignment.FilePath, &assignment.FileSize,
		&resolution, &source, &assignment.IsActive, &assignment.CreatedAt,
	)
	if err != nil {
		return nil, err
	}

	assignment.Resolution = resolution.String
	assignment.Source = source.String

	return assignment, nil
}

// Create adds a new torrent assignment
func (r *AssignmentRepository) Create(assignment *TorrentAssignment) error {
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Deactivate any existing active assignments for this item
	_, err = tx.Exec(
		`UPDATE torrent_assignments SET is_active = FALSE WHERE item_type = $1 AND item_id = $2 AND is_active = TRUE`,
		assignment.ItemType, assignment.ItemID,
	)
	if err != nil {
		return fmt.Errorf("failed to deactivate existing assignments: %w", err)
	}

	// Create new assignment
	err = tx.QueryRow(
		`INSERT INTO torrent_assignments (item_type, item_id, info_hash, magnet_uri, file_path, file_size, resolution, source, is_active)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, TRUE) RETURNING id, created_at`,
		assignment.ItemType, assignment.ItemID, assignment.InfoHash, assignment.MagnetURI,
		assignment.FilePath, assignment.FileSize, nullString(assignment.Resolution), nullString(assignment.Source),
	).Scan(&assignment.ID, &assignment.CreatedAt)
	if err != nil {
		return fmt.Errorf("failed to create assignment: %w", err)
	}
	assignment.IsActive = true

	return tx.Commit()
}

// GetByID retrieves an assignment by its ID
func (r *AssignmentRepository) GetByID(id int64) (*TorrentAssignment, error) {
	row := r.db.QueryRow(
		`SELECT id, item_type, item_id, info_hash, magnet_uri, file_path, file_size, resolution, source, is_active, created_at
		 FROM torrent_assignments WHERE id = $1`,
		id,
	)

	assignment, err := scanAssignment(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get assignment: %w", err)
	}

	return assignment, nil
}

// GetActiveForItem retrieves the active assignment for a library item
func (r *AssignmentRepository) GetActiveForItem(itemType ItemType, itemID int64) (*TorrentAssignment, error) {
	row := r.db.QueryRow(
		`SELECT id, item_type, item_id, info_hash, magnet_uri, file_path, file_size, resolution, source, is_active, created_at
		 FROM torrent_assignments WHERE item_type = $1 AND item_id = $2 AND is_active = TRUE`,
		itemType, itemID,
	)

	assignment, err := scanAssignment(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get active assignment: %w", err)
	}

	return assignment, nil
}

// GetByInfoHash retrieves all assignments using a specific torrent
func (r *AssignmentRepository) GetByInfoHash(infoHash string) ([]*TorrentAssignment, error) {
	rows, err := r.db.Query(
		`SELECT id, item_type, item_id, info_hash, magnet_uri, file_path, file_size, resolution, source, is_active, created_at
		 FROM torrent_assignments WHERE info_hash = $1`,
		infoHash,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get assignments by hash: %w", err)
	}
	defer rows.Close()

	var assignments []*TorrentAssignment
	for rows.Next() {
		assignment, err := scanAssignment(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan assignment: %w", err)
		}
		assignments = append(assignments, assignment)
	}

	return assignments, rows.Err()
}

// GetActiveByInfoHash retrieves all active assignments using a specific torrent
func (r *AssignmentRepository) GetActiveByInfoHash(infoHash string) ([]*TorrentAssignment, error) {
	rows, err := r.db.Query(
		`SELECT id, item_type, item_id, info_hash, magnet_uri, file_path, file_size, resolution, source, is_active, created_at
		 FROM torrent_assignments WHERE info_hash = $1 AND is_active = TRUE`,
		infoHash,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get active assignments by hash: %w", err)
	}
	defer rows.Close()

	var assignments []*TorrentAssignment
	for rows.Next() {
		assignment, err := scanAssignment(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan assignment: %w", err)
		}
		assignments = append(assignments, assignment)
	}

	return assignments, rows.Err()
}

// Deactivate deactivates an assignment
func (r *AssignmentRepository) Deactivate(id int64) error {
	result, err := r.db.Exec(
		`UPDATE torrent_assignments SET is_active = FALSE WHERE id = $1`,
		id,
	)
	if err != nil {
		return fmt.Errorf("failed to deactivate assignment: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check deactivate result: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("assignment not found")
	}

	return nil
}

// DeactivateForItem deactivates all assignments for a library item
func (r *AssignmentRepository) DeactivateForItem(itemType ItemType, itemID int64) error {
	_, err := r.db.Exec(
		`UPDATE torrent_assignments SET is_active = FALSE WHERE item_type = $1 AND item_id = $2`,
		itemType, itemID,
	)
	if err != nil {
		return fmt.Errorf("failed to deactivate assignments: %w", err)
	}

	return nil
}

// Delete removes an assignment
func (r *AssignmentRepository) Delete(id int64) error {
	result, err := r.db.Exec(`DELETE FROM torrent_assignments WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("failed to delete assignment: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check delete result: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("assignment not found")
	}

	return nil
}

// ListDistinctTorrents returns all unique torrents that have active assignments
func (r *AssignmentRepository) ListDistinctTorrents() ([]string, error) {
	rows, err := r.db.Query(
		`SELECT DISTINCT info_hash FROM torrent_assignments WHERE is_active = TRUE`,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list torrents: %w", err)
	}
	defer rows.Close()

	var hashes []string
	for rows.Next() {
		var hash string
		if err := rows.Scan(&hash); err != nil {
			return nil, fmt.Errorf("failed to scan hash: %w", err)
		}
		hashes = append(hashes, hash)
	}

	return hashes, rows.Err()
}

// DeleteByInfoHash removes all assignments using a specific torrent
func (r *AssignmentRepository) DeleteByInfoHash(infoHash string) (int64, error) {
	result, err := r.db.Exec(`DELETE FROM torrent_assignments WHERE info_hash = $1`, infoHash)
	if err != nil {
		return 0, fmt.Errorf("failed to delete assignments by hash: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get rows affected: %w", err)
	}
	return affected, nil
}

// Helper to convert empty string to NULL
func nullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}
