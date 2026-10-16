package library

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// Removals (PLAN §6 "Torrents and activity (B6)", "Titles and library"):
// the database half of deleting a torrent or a title. What lives outside
// the database (subtitle files, the torrent client, the WebDAV tree) is the
// caller's: these return what it needs.

// TorrentRef is a torrent and a magnet to fetch it with.
type TorrentRef struct {
	InfoHash  string
	MagnetURI string
}

// TorrentKnown reports whether the library knows the torrent: an active
// torrent assignment, or an open job that isn't a dry run, uses it.
func (r *AssignmentJobRepository) TorrentKnown(ctx context.Context, infoHash string) (bool, error) {
	var known bool
	err := r.db.QueryRowContext(ctx, `
		SELECT EXISTS (SELECT 1 FROM torrent_assignments WHERE info_hash = $1 AND is_active AND kind = 'torrent')
		    OR EXISTS (SELECT 1 FROM assignment_jobs WHERE info_hash = $1 AND NOT dry_run AND status = ANY($2))
	`, infoHash, openStatuses()).Scan(&known)
	if err != nil {
		return false, fmt.Errorf("failed to check whether the torrent is known: %w", err)
	}
	return known, nil
}

// ForgottenTorrent is what ForgetTorrent changed.
type ForgottenTorrent struct {
	// MagnetURI is the newest magnet the torrent's assignments or jobs
	// had, "" when none did: enough to fetch its metainfo again.
	MagnetURI string
	// Cancelled open jobs (dry runs too), deactivated assignments and
	// deleted torrent subtitles.
	Cancelled, Deactivated, Subtitles int64
}

// ForgetTorrent drops the library's use of a torrent, in one transaction:
// its open jobs (dry runs too) are cancelled, its active assignments
// deactivated (the rows stay, as history) and its torrent subtitles
// deleted. Jobs go first: Apply locks the job before it writes assignments,
// so taking them in the same order can't deadlock with it.
func (r *AssignmentJobRepository) ForgetTorrent(ctx context.Context, infoHash string) (*ForgottenTorrent, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	out := &ForgottenTorrent{}
	err = tx.QueryRowContext(ctx, `
		SELECT magnet_uri FROM (
			SELECT magnet_uri, created_at FROM torrent_assignments WHERE info_hash = $1 AND kind = 'torrent'
			UNION ALL
			SELECT magnet_uri, created_at FROM assignment_jobs WHERE info_hash = $1
		) m ORDER BY created_at DESC LIMIT 1
	`, infoHash).Scan(&out.MagnetURI)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("failed to find the torrent's magnet: %w", err)
	}

	steps := []struct {
		n     *int64
		what  string
		query string
		args  []any
	}{
		{&out.Cancelled, "cancel the torrent's jobs", `
			UPDATE assignment_jobs SET status = 'cancelled', updated_at = GREATEST(clock_timestamp(), updated_at)
			WHERE info_hash = $1 AND status = ANY($2)`, []any{infoHash, openStatuses()}},
		{&out.Deactivated, "deactivate the torrent's assignments", `
			UPDATE torrent_assignments SET is_active = FALSE WHERE info_hash = $1 AND is_active`, []any{infoHash}},
		{&out.Subtitles, "delete the torrent's subtitles", `
			DELETE FROM subtitles WHERE source = 'torrent' AND info_hash = $1`, []any{infoHash}},
	}
	for _, step := range steps {
		res, err := tx.ExecContext(ctx, step.query, step.args...)
		if err != nil {
			return nil, fmt.Errorf("failed to %s: %w", step.what, err)
		}
		if *step.n, err = res.RowsAffected(); err != nil {
			return nil, fmt.Errorf("failed to %s: %w", step.what, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit: %w", err)
	}
	return out, nil
}

// TitleDeletion is what a deleted title leaves to clean up outside the
// database.
type TitleDeletion struct {
	Title string
	Year  int
	// ItemIDs are the title's items: the movie, or every episode of the
	// show. Their subtitles are still stored.
	ItemIDs []int64
	// Torrents are every torrent the title used, in its assignments
	// (active or not) and its jobs, one entry per hash with its newest
	// magnet. Whether something else still uses one is the caller's to ask
	// (AssignmentJobRepository.TorrentInUse).
	Torrents []TorrentRef
}

// DeleteWithAssignments deletes the movie (its jobs cascade) and
// deactivates its assignments, in one transaction, and returns what's left
// to clean up. ErrMovieNotFound when it doesn't exist.
func (r *MovieRepository) DeleteWithAssignments(ctx context.Context, id int64) (*TitleDeletion, error) {
	return deleteTitle(ctx, r.db, titleDelete{
		itemType: ItemTypeMovie,
		items:    `SELECT $1::bigint`,
		jobKey:   "movie_id",
		delete:   `DELETE FROM movies WHERE id = $1 RETURNING title, year`,
		missing:  ErrMovieNotFound,
	}, id)
}

// DeleteWithAssignments deletes the show (its seasons, episodes and jobs
// cascade) and deactivates every episode's assignments in one query, all in
// one transaction, and returns what's left to clean up. ErrShowNotFound
// when it doesn't exist.
func (r *ShowRepository) DeleteWithAssignments(ctx context.Context, id int64) (*TitleDeletion, error) {
	return deleteTitle(ctx, r.db, titleDelete{
		itemType: ItemTypeEpisode,
		items:    `SELECT e.id FROM episodes e JOIN seasons sn ON sn.id = e.season_id WHERE sn.show_id = $1 ORDER BY e.id`,
		jobKey:   "show_id",
		delete:   `DELETE FROM shows WHERE id = $1 RETURNING title, year`,
		missing:  ErrShowNotFound,
	}, id)
}

// titleDelete is the per-kind SQL of deleteTitle. jobKey is a column name,
// never user input.
type titleDelete struct {
	itemType ItemType
	items    string // $1: the title id → its item ids
	jobKey   string // assignment_jobs' column for the title
	delete   string // $1: the title id → RETURNING title, year
	missing  error
}

// deleteTitle runs a title delete in one transaction. The assignments are
// deactivated after the title is gone: deleting it waits for an Apply in
// flight on its jobs (the cascade locks their rows), so the rows that
// Apply wrote are deactivated too.
func deleteTitle(ctx context.Context, db *DB, d titleDelete, id int64) (*TitleDeletion, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	out := &TitleDeletion{}
	rows, err := tx.QueryContext(ctx, d.items, id)
	if err != nil {
		return nil, fmt.Errorf("failed to list the title's items: %w", err)
	}
	for rows.Next() {
		var itemID int64
		if err := rows.Scan(&itemID); err != nil {
			rows.Close()
			return nil, fmt.Errorf("failed to scan an item id: %w", err)
		}
		out.ItemIDs = append(out.ItemIDs, itemID)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to list the title's items: %w", err)
	}

	rows, err = tx.QueryContext(ctx, `
		SELECT DISTINCT ON (info_hash) info_hash, magnet_uri FROM (
			SELECT info_hash, magnet_uri, created_at FROM torrent_assignments
			WHERE item_type = $1 AND item_id = ANY($2::bigint[]) AND kind = 'torrent'
			UNION ALL
			SELECT info_hash, magnet_uri, created_at FROM assignment_jobs WHERE `+d.jobKey+` = $3
		) t ORDER BY info_hash, created_at DESC
	`, d.itemType, out.ItemIDs, id)
	if err != nil {
		return nil, fmt.Errorf("failed to list the title's torrents: %w", err)
	}
	for rows.Next() {
		var ref TorrentRef
		if err := rows.Scan(&ref.InfoHash, &ref.MagnetURI); err != nil {
			rows.Close()
			return nil, fmt.Errorf("failed to scan a torrent: %w", err)
		}
		out.Torrents = append(out.Torrents, ref)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to list the title's torrents: %w", err)
	}

	err = tx.QueryRowContext(ctx, d.delete, id).Scan(&out.Title, &out.Year)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, d.missing
	}
	if err != nil {
		return nil, fmt.Errorf("failed to delete the title: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE torrent_assignments SET is_active = FALSE
		WHERE item_type = $1 AND item_id = ANY($2::bigint[]) AND is_active
	`, d.itemType, out.ItemIDs); err != nil {
		return nil, fmt.Errorf("failed to deactivate the title's assignments: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit: %w", err)
	}
	return out, nil
}
