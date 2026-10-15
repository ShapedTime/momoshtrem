package subtitle

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

var (
	// ErrNotFound is returned when a subtitle doesn't exist (or isn't the
	// item's).
	ErrNotFound = errors.New("subtitle not found")
	// ErrStaleStream: the HDRezka stream whose subtitles were being stored
	// is no longer the item's file; another was assigned meanwhile.
	ErrStaleStream = errors.New("the item's HDRezka stream has changed")
)

// Repository handles subtitle database operations
type Repository struct {
	db *sql.DB
}

// NewRepository creates a new subtitle repository
func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

var _ SubtitleRepository = (*Repository)(nil)

// subtitleColumns is the SELECT list scanSubtitle reads.
const subtitleColumns = `id, item_type, item_id, language_code, language_name, format, file_path,
	file_size, source, info_hash, release_name, created_at`

// subtitleOrder lists an item's subtitles by language, downloaded before
// those that came with the file (torrent or HDRezka), then oldest first.
// The VFS names them in this order.
const subtitleOrder = `language_code, (source <> 'opensubtitles'), id`

// scanner is an interface for sql.Row and sql.Rows
type scanner interface {
	Scan(dest ...interface{}) error
}

// scanSubtitle scans a row into a Subtitle
func scanSubtitle(s scanner) (*Subtitle, error) {
	sub := &Subtitle{}
	var infoHash, releaseName sql.NullString
	err := s.Scan(
		&sub.ID, &sub.ItemType, &sub.ItemID,
		&sub.LanguageCode, &sub.LanguageName,
		&sub.Format, &sub.FilePath, &sub.FileSize,
		&sub.Source, &infoHash, &releaseName,
		&sub.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	sub.InfoHash = infoHash.String
	sub.ReleaseName = releaseName.String
	return sub, nil
}

// nullString converts an empty string to sql.NullString for nullable columns
func nullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

// UpsertDownloaded stores an OpenSubtitles download. A second download in the
// same language replaces the first (same id, fresh created_at); the item's
// torrent subtitles are untouched.
func (r *Repository) UpsertDownloaded(ctx context.Context, sub *Subtitle) error {
	sub.Source = SourceOpenSubtitles
	sub.InfoHash = ""
	err := r.db.QueryRowContext(ctx,
		`INSERT INTO subtitles (item_type, item_id, language_code, language_name, format, file_path,
		                        file_size, source, info_hash, release_name)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, 'opensubtitles', NULL, $8)
		 ON CONFLICT (item_type, item_id, language_code) WHERE source = 'opensubtitles' DO UPDATE SET
		     language_name = EXCLUDED.language_name,
		     format        = EXCLUDED.format,
		     file_path     = EXCLUDED.file_path,
		     file_size     = EXCLUDED.file_size,
		     release_name  = EXCLUDED.release_name,
		     created_at    = NOW()
		 RETURNING id, created_at`,
		sub.ItemType, sub.ItemID, sub.LanguageCode, sub.LanguageName,
		sub.Format, sub.FilePath, sub.FileSize, nullString(sub.ReleaseName),
	).Scan(&sub.ID, &sub.CreatedAt)
	if err != nil {
		return fmt.Errorf("failed to upsert downloaded subtitle: %w", err)
	}
	return nil
}

// UpsertTorrent stores a subtitle file from the item's torrent, keyed by the
// torrent and file. In the same transaction it deletes the item's torrent
// subtitles from other torrents, so a reassignment leaves none behind.
func (r *Repository) UpsertTorrent(ctx context.Context, sub *Subtitle) error {
	if sub.InfoHash == "" {
		return errors.New("torrent subtitle needs an info hash")
	}
	sub.Source = SourceTorrent

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM subtitles
		 WHERE item_type = $1 AND item_id = $2 AND source = 'torrent' AND info_hash <> $3`,
		sub.ItemType, sub.ItemID, sub.InfoHash,
	); err != nil {
		return fmt.Errorf("failed to drop subtitles from other torrents: %w", err)
	}

	err = tx.QueryRowContext(ctx,
		`INSERT INTO subtitles (item_type, item_id, language_code, language_name, format, file_path,
		                        file_size, source, info_hash, release_name)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, 'torrent', $8, $9)
		 ON CONFLICT (item_type, item_id, info_hash, file_path) WHERE source = 'torrent' DO UPDATE SET
		     language_code = EXCLUDED.language_code,
		     language_name = EXCLUDED.language_name,
		     format        = EXCLUDED.format,
		     file_size     = EXCLUDED.file_size,
		     release_name  = EXCLUDED.release_name
		 RETURNING id, created_at`,
		sub.ItemType, sub.ItemID, sub.LanguageCode, sub.LanguageName,
		sub.Format, sub.FilePath, sub.FileSize, sub.InfoHash, nullString(sub.ReleaseName),
	).Scan(&sub.ID, &sub.CreatedAt)
	if err != nil {
		return fmt.Errorf("failed to upsert torrent subtitle: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit torrent subtitle: %w", err)
	}
	return nil
}

// UpsertHDRezka stores the subtitles that came with the item's HDRezka
// stream, one per language: each replaces the item's HDRezka subtitle in
// its language (same id, fresh created_at), so assigning the stream again
// refreshes them. Every sub must be the item's.
//
// It writes only while streamFile is the item's active file, and holds
// that row (FOR SHARE) until it commits. The assignment writer deactivates
// an item's old row before it drops the old file's subtitles, so an assign
// that comes meanwhile waits and then drops these too; one that came first
// leaves no row to hold, and this returns ErrStaleStream without storing
// anything. place runs after the rows are written and before the commit:
// it moves the files into place, so a later assign's files always land
// after this one's. When it fails, nothing is stored.
func (r *Repository) UpsertHDRezka(ctx context.Context, item Item, streamFile string, subs []*Subtitle, place func() error) error {
	for _, sub := range subs {
		if sub.Item() != item {
			return fmt.Errorf("subtitle for %s %d stored as %s %d's", sub.ItemType, sub.ItemID, item.Type, item.ID)
		}
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	var assignmentID int64
	err = tx.QueryRowContext(ctx,
		`SELECT id FROM torrent_assignments
		 WHERE item_type = $1 AND item_id = $2 AND is_active AND kind = 'hdrezka' AND file_path = $3
		 FOR SHARE`,
		item.Type, item.ID, streamFile,
	).Scan(&assignmentID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrStaleStream
	}
	if err != nil {
		return fmt.Errorf("failed to hold the item's stream: %w", err)
	}

	for _, sub := range subs {
		sub.Source = SourceHDRezka
		sub.InfoHash = ""
		err := tx.QueryRowContext(ctx,
			`INSERT INTO subtitles (item_type, item_id, language_code, language_name, format, file_path,
			                        file_size, source, info_hash, release_name)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, 'hdrezka', NULL, $8)
			 ON CONFLICT (item_type, item_id, language_code) WHERE source = 'hdrezka' DO UPDATE SET
			     language_name = EXCLUDED.language_name,
			     format        = EXCLUDED.format,
			     file_path     = EXCLUDED.file_path,
			     file_size     = EXCLUDED.file_size,
			     release_name  = EXCLUDED.release_name,
			     created_at    = NOW()
			 RETURNING id, created_at`,
			sub.ItemType, sub.ItemID, sub.LanguageCode, sub.LanguageName,
			sub.Format, sub.FilePath, sub.FileSize, nullString(sub.ReleaseName),
		).Scan(&sub.ID, &sub.CreatedAt)
		if err != nil {
			return fmt.Errorf("failed to upsert HDRezka subtitle: %w", err)
		}
	}

	if place != nil {
		if err := place(); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit HDRezka subtitles: %w", err)
	}
	return nil
}

// GetByID retrieves a subtitle by its ID; nil when it doesn't exist.
func (r *Repository) GetByID(ctx context.Context, id int64) (*Subtitle, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT `+subtitleColumns+` FROM subtitles WHERE id = $1`,
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

// GetByItem retrieves all subtitles for a library item, by language with
// downloaded ones first.
func (r *Repository) GetByItem(ctx context.Context, itemType ItemType, itemID int64) ([]*Subtitle, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+subtitleColumns+` FROM subtitles
		 WHERE item_type = $1 AND item_id = $2
		 ORDER BY `+subtitleOrder,
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

// GetByItems loads the subtitles of many items in one query, each item's in
// GetByItem order. Items without subtitles are absent from the map.
func (r *Repository) GetByItems(ctx context.Context, items []Item) (map[Item][]*Subtitle, error) {
	out := make(map[Item][]*Subtitle)
	if len(items) == 0 {
		return out, nil
	}
	types := make([]string, len(items))
	ids := make([]int64, len(items))
	for i, it := range items {
		types[i] = string(it.Type)
		ids[i] = it.ID
	}

	rows, err := r.db.QueryContext(ctx,
		`SELECT `+subtitleColumns+` FROM subtitles
		 WHERE (item_type, item_id) IN (SELECT * FROM unnest($1::text[], $2::bigint[]))
		 ORDER BY item_type, item_id, `+subtitleOrder,
		types, ids,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get subtitles for items: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		sub, err := scanSubtitle(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan subtitle: %w", err)
		}
		out[sub.Item()] = append(out[sub.Item()], sub)
	}
	return out, rows.Err()
}

// GetDownloaded returns the item's downloaded subtitle in a language; nil
// when there is none.
func (r *Repository) GetDownloaded(ctx context.Context, itemType ItemType, itemID int64, languageCode string) (*Subtitle, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT `+subtitleColumns+` FROM subtitles
		 WHERE item_type = $1 AND item_id = $2 AND language_code = $3 AND source = 'opensubtitles'`,
		itemType, itemID, languageCode,
	)

	sub, err := scanSubtitle(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get downloaded subtitle: %w", err)
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
		return ErrNotFound
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
