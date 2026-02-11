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
	var infoHash sql.NullString
	err := s.Scan(
		&sub.ID, &sub.ItemType, &sub.ItemID,
		&sub.LanguageCode, &sub.LanguageName,
		&sub.Format, &sub.FilePath, &sub.FileSize,
		&sub.Source, &infoHash,
		&sub.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	sub.InfoHash = infoHash.String
	return sub, nil
}

// nullString converts an empty string to sql.NullString for nullable columns
func nullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

// Create adds or updates a subtitle record (upsert).
func (r *Repository) Create(ctx context.Context, sub *Subtitle) error {
	source := sub.Source
	if source == "" {
		source = SourceOpenSubtitles
	}

	err := r.db.QueryRowContext(ctx,
		`INSERT INTO subtitles (item_type, item_id, language_code, language_name, format, file_path, file_size, source, info_hash)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 ON CONFLICT(item_type, item_id, language_code) DO UPDATE SET
		 language_name = EXCLUDED.language_name,
		 format = EXCLUDED.format,
		 file_path = EXCLUDED.file_path,
		 file_size = EXCLUDED.file_size,
		 source = EXCLUDED.source,
		 info_hash = EXCLUDED.info_hash
		 RETURNING id, source, created_at`,
		sub.ItemType, sub.ItemID, sub.LanguageCode, sub.LanguageName,
		sub.Format, sub.FilePath, sub.FileSize, source, nullString(sub.InfoHash),
	).Scan(&sub.ID, &sub.Source, &sub.CreatedAt)
	if err != nil {
		return fmt.Errorf("failed to create subtitle: %w", err)
	}
	return nil
}

// GetByID retrieves a subtitle by its ID
func (r *Repository) GetByID(ctx context.Context, id int64) (*Subtitle, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, item_type, item_id, language_code, language_name, format, file_path, file_size, source, info_hash, created_at
		 FROM subtitles WHERE id = $1`,
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
		`SELECT id, item_type, item_id, language_code, language_name, format, file_path, file_size, source, info_hash, created_at
		 FROM subtitles WHERE item_type = $1 AND item_id = $2
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
		`SELECT id, item_type, item_id, language_code, language_name, format, file_path, file_size, source, info_hash, created_at
		 FROM subtitles WHERE item_type = $1 AND item_id = $2 AND language_code = $3`,
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
	result, err := r.db.ExecContext(ctx, `DELETE FROM subtitles WHERE id = $1`, id)
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
		`DELETE FROM subtitles WHERE item_type = $1 AND item_id = $2`,
		itemType, itemID,
	)
	if err != nil {
		return fmt.Errorf("failed to delete subtitles for item: %w", err)
	}

	return nil
}
