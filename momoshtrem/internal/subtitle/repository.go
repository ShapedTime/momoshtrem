package subtitle

import (
	"context"
	"database/sql"
	"fmt"
)

// Repository handles subtitle database operations
type Repository struct {
	db *sql.DB
}

// NewRepository creates a new subtitle repository
func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

// scanner is an interface for sql.Row and sql.Rows
type scanner interface {
	Scan(dest ...interface{}) error
}

// scanSubtitle scans a row into a Subtitle
func scanSubtitle(s scanner) (*Subtitle, error) {
	sub := &Subtitle{}
	err := s.Scan(
		&sub.ID, &sub.ItemType, &sub.ItemID,
		&sub.LanguageCode, &sub.LanguageName,
		&sub.Format, &sub.FilePath, &sub.FileSize,
		&sub.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return sub, nil
}

// Create adds or updates a subtitle record (upsert).
// After upsert, it queries the actual row to get correct ID and CreatedAt.
func (r *Repository) Create(ctx context.Context, sub *Subtitle) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO subtitles (item_type, item_id, language_code, language_name, format, file_path, file_size)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(item_type, item_id, language_code) DO UPDATE SET
		 language_name = excluded.language_name,
		 format = excluded.format,
		 file_path = excluded.file_path,
		 file_size = excluded.file_size`,
		sub.ItemType, sub.ItemID, sub.LanguageCode, sub.LanguageName,
		sub.Format, sub.FilePath, sub.FileSize,
	)
	if err != nil {
		return fmt.Errorf("failed to create subtitle: %w", err)
	}

	// Query the actual row to get correct ID and CreatedAt (LastInsertId unreliable after UPSERT)
	row := r.db.QueryRowContext(ctx,
		`SELECT id, created_at FROM subtitles WHERE item_type = ? AND item_id = ? AND language_code = ?`,
		sub.ItemType, sub.ItemID, sub.LanguageCode,
	)
	if err := row.Scan(&sub.ID, &sub.CreatedAt); err != nil {
		return fmt.Errorf("failed to get subtitle after upsert: %w", err)
	}

	return nil
}

// GetByID retrieves a subtitle by its ID
func (r *Repository) GetByID(ctx context.Context, id int64) (*Subtitle, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, item_type, item_id, language_code, language_name, format, file_path, file_size, created_at
		 FROM subtitles WHERE id = ?`,
		id,
	)

	sub, err := scanSubtitle(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get subtitle: %w", err)
	}

	return sub, nil
}

// GetByItem retrieves all subtitles for a library item
func (r *Repository) GetByItem(ctx context.Context, itemType ItemType, itemID int64) ([]*Subtitle, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, item_type, item_id, language_code, language_name, format, file_path, file_size, created_at
		 FROM subtitles WHERE item_type = ? AND item_id = ?
		 ORDER BY language_code`,
		itemType, itemID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get subtitles for item: %w", err)
	}
	defer rows.Close()

	var subtitles []*Subtitle
	for rows.Next() {
		sub, err := scanSubtitle(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan subtitle: %w", err)
		}
		subtitles = append(subtitles, sub)
	}

	return subtitles, rows.Err()
}

// GetByItemAndLanguage retrieves a specific subtitle by item and language
func (r *Repository) GetByItemAndLanguage(ctx context.Context, itemType ItemType, itemID int64, languageCode string) (*Subtitle, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, item_type, item_id, language_code, language_name, format, file_path, file_size, created_at
		 FROM subtitles WHERE item_type = ? AND item_id = ? AND language_code = ?`,
		itemType, itemID, languageCode,
	)

	sub, err := scanSubtitle(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get subtitle by language: %w", err)
	}

	return sub, nil
}

// Delete removes a subtitle by ID
func (r *Repository) Delete(ctx context.Context, id int64) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM subtitles WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("failed to delete subtitle: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check delete result: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("subtitle not found")
	}

	return nil
}

// DeleteByItem removes all subtitles for a library item
func (r *Repository) DeleteByItem(ctx context.Context, itemType ItemType, itemID int64) error {
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM subtitles WHERE item_type = ? AND item_id = ?`,
		itemType, itemID,
	)
	if err != nil {
		return fmt.Errorf("failed to delete subtitles for item: %w", err)
	}

	return nil
}
