package library

import (
	"context"
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

// assignmentSelect lists a torrent_assignments row's columns in the order
// scanAssignment reads them.
const assignmentSelect = `id, item_type, item_id, kind, info_hash, magnet_uri, file_path, file_size,
	resolution, source, confidence, match_source, stream_ref, is_active, created_at`

// scanAssignment scans assignmentSelect into a TorrentAssignment. An HDRezka
// row has no info hash or magnet: they read as "".
func scanAssignment(s scanner) (*TorrentAssignment, error) {
	assignment := &TorrentAssignment{}
	var hash, magnet, resolution, source, confidence, matchSource sql.NullString
	var stream nullStreamRef

	err := s.Scan(
		&assignment.ID, &assignment.ItemType, &assignment.ItemID, &assignment.Kind,
		&hash, &magnet, &assignment.FilePath, &assignment.FileSize,
		&resolution, &source, &confidence, &matchSource, &stream, &assignment.IsActive, &assignment.CreatedAt,
	)
	if err != nil {
		return nil, err
	}

	assignment.InfoHash = hash.String
	assignment.MagnetURI = magnet.String
	assignment.Resolution = resolution.String
	assignment.Source = source.String
	assignment.Confidence = confidence.String
	assignment.MatchSource = matchSource.String
	assignment.Stream = stream.Ref

	return assignment, nil
}

// assignmentUpsert reactivates the row an item already has for the same
// file (UNIQUE(item_type, item_id, info_hash, file_path)) instead of
// failing. A re-assignment is a new assignment, so it's stamped now; a
// row that was active already (the same torrent applied again) keeps its
// time, so a download's admission order doesn't change (#77).
const assignmentUpsert = `ON CONFLICT (item_type, item_id, info_hash, file_path) DO UPDATE SET
		is_active = TRUE, magnet_uri = EXCLUDED.magnet_uri, file_size = EXCLUDED.file_size,
		resolution = EXCLUDED.resolution, source = EXCLUDED.source,
		confidence = EXCLUDED.confidence, match_source = EXCLUDED.match_source,
		created_at = CASE WHEN torrent_assignments.is_active THEN torrent_assignments.created_at ELSE NOW() END`

// WriteStream makes HDRezka streams the items' files (w.Rows: ItemID,
// Stream, FileSize and Resolution), in its own transaction, through the
// same writer as Apply and the overrides: the title's lock, each item's
// other active rows deactivated (a torrent file too), and its torrent and
// HDRezka subtitles dropped when its file changes. A stream the item had
// before is reactivated; another translator or quality is a new row. The
// write is partial: the title's other items keep their files. It returns
// ErrShowNotFound or ErrMovieNotFound when the title is gone.
func (r *AssignmentRepository) WriteStream(ctx context.Context, w AssignmentWrite) error {
	w.Kind, w.Partial = AssignmentKindHDRezka, true
	return r.write(ctx, w)
}

// write runs writeAssignments in its own transaction.
func (r *AssignmentRepository) write(ctx context.Context, w AssignmentWrite) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()
	if err := writeAssignments(ctx, tx, w); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit: %w", err)
	}
	return nil
}

// GetByID retrieves an assignment by its ID
func (r *AssignmentRepository) GetByID(id int64) (*TorrentAssignment, error) {
	row := r.db.QueryRow(
		`SELECT `+assignmentSelect+`
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
		`SELECT `+assignmentSelect+`
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
		`SELECT `+assignmentSelect+`
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
		`SELECT `+assignmentSelect+`
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
		`SELECT DISTINCT info_hash FROM torrent_assignments WHERE is_active = TRUE AND kind = 'torrent'`,
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
