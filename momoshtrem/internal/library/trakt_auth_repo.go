package library

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ErrTraktConnectionReplaced means a write named a connection that isn't
// the stored one any more: a disconnect or a reconnect replaced it.
var ErrTraktConnectionReplaced = errors.New("the Trakt connection was replaced")

// traktTables are every table of migration 005: a disconnect empties them
// all. The settings (app_settings key 'trakt') aren't among them.
var traktTables = []string{
	"trakt_history", "trakt_watched_movies", "trakt_watched_episodes", "trakt_ratings",
	"trakt_watchlist", "trakt_show_progress", "trakt_hidden", "trakt_calendar",
	"trakt_sync_state", "trakt_auth",
}

// TraktAuth is the one Trakt connection (trakt_auth). The tokens are sealed
// with SECRET_KEY (internal/secret). ConnectedAt is the connection's
// generation: writes that name another one are refused.
type TraktAuth struct {
	Username        string
	UserSlug        string
	AccessTokenEnc  []byte
	RefreshTokenEnc []byte
	TokenCreatedAt  time.Time // Trakt's created_at
	ExpiresAt       time.Time // created_at + expires_in
	NeedsReauth     bool
	ReauthReason    string
	ConnectedAt     time.Time
}

// TraktTokens are the sealed tokens a refresh hands out.
type TraktTokens struct {
	AccessTokenEnc  []byte
	RefreshTokenEnc []byte
	CreatedAt       time.Time
	ExpiresAt       time.Time
}

// TraktCounts are what's imported, for GET /integrations/trakt: plays,
// ratings, watchlist titles and how many of those are in the library.
type TraktCounts struct {
	History            int `json:"history"`
	Ratings            int `json:"ratings"`
	Watchlist          int `json:"watchlist"`
	WatchlistInLibrary int `json:"watchlist_in_library"`
}

// TraktAuthRepository stores the Trakt connection and purges what was
// imported with it.
type TraktAuthRepository struct {
	db *DB
}

// NewTraktAuthRepository creates a Trakt connection repository.
func NewTraktAuthRepository(db *DB) *TraktAuthRepository {
	return &TraktAuthRepository{db: db}
}

// Get returns the stored connection; nil when there's none.
func (r *TraktAuthRepository) Get(ctx context.Context) (*TraktAuth, error) {
	var a TraktAuth
	var reason sql.NullString
	err := r.db.QueryRowContext(ctx, `
		SELECT username, user_slug, access_token_enc, refresh_token_enc, token_created_at, expires_at,
		       needs_reauth, reauth_reason, connected_at
		FROM trakt_auth WHERE id = 1
	`).Scan(&a.Username, &a.UserSlug, &a.AccessTokenEnc, &a.RefreshTokenEnc, &a.TokenCreatedAt, &a.ExpiresAt,
		&a.NeedsReauth, &reason, &a.ConnectedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read the Trakt connection: %w", err)
	}
	a.ReauthReason = reason.String
	return &a, nil
}

// Connect stores a new connection and answers its generation. Connecting
// another account than the stored one (or with none stored) purges every
// Trakt table first, in the same transaction; the same account keeps its
// imported data. Either way the generation is new and reauth is cleared.
func (r *TraktAuthRepository) Connect(ctx context.Context, a TraktAuth) (time.Time, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	var slug string
	err = tx.QueryRowContext(ctx, `SELECT user_slug FROM trakt_auth WHERE id = 1 FOR UPDATE`).Scan(&slug)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, fmt.Errorf("failed to read the Trakt connection: %w", err)
	}
	if errors.Is(err, sql.ErrNoRows) || slug != a.UserSlug {
		if err := purgeTrakt(ctx, tx); err != nil {
			return time.Time{}, err
		}
	}

	var connectedAt time.Time
	err = tx.QueryRowContext(ctx, `
		INSERT INTO trakt_auth (id, username, user_slug, access_token_enc, refresh_token_enc, token_created_at, expires_at,
		                        needs_reauth, reauth_reason, connected_at, updated_at)
		VALUES (1, $1, $2, $3, $4, $5, $6, FALSE, NULL, NOW(), NOW())
		ON CONFLICT (id) DO UPDATE SET
			username = EXCLUDED.username, user_slug = EXCLUDED.user_slug,
			access_token_enc = EXCLUDED.access_token_enc, refresh_token_enc = EXCLUDED.refresh_token_enc,
			token_created_at = EXCLUDED.token_created_at, expires_at = EXCLUDED.expires_at,
			needs_reauth = FALSE, reauth_reason = NULL, connected_at = NOW(), updated_at = NOW()
		RETURNING connected_at
	`, a.Username, a.UserSlug, a.AccessTokenEnc, a.RefreshTokenEnc, a.TokenCreatedAt, a.ExpiresAt).Scan(&connectedAt)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to store the Trakt connection: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return time.Time{}, fmt.Errorf("failed to commit the Trakt connection: %w", err)
	}
	return connectedAt, nil
}

// UpdateTokens replaces both tokens of the connection connectedAt names,
// in one statement: refresh tokens are single-use, so the pair is stored
// before either is used. ErrTraktConnectionReplaced when it isn't stored.
func (r *TraktAuthRepository) UpdateTokens(ctx context.Context, connectedAt time.Time, t TraktTokens) error {
	res, err := r.db.ExecContext(ctx, `
		UPDATE trakt_auth SET access_token_enc = $2, refresh_token_enc = $3, token_created_at = $4, expires_at = $5, updated_at = NOW()
		WHERE id = 1 AND connected_at = $1
	`, connectedAt, t.AccessTokenEnc, t.RefreshTokenEnc, t.CreatedAt, t.ExpiresAt)
	if err != nil {
		return fmt.Errorf("failed to store the Trakt tokens: %w", err)
	}
	return requireConnection(res)
}

// SetReauth records that Trakt stopped accepting the connection's tokens,
// and why (for the user).
func (r *TraktAuthRepository) SetReauth(ctx context.Context, connectedAt time.Time, reason string) error {
	res, err := r.db.ExecContext(ctx, `
		UPDATE trakt_auth SET needs_reauth = TRUE, reauth_reason = $2, updated_at = NOW()
		WHERE id = 1 AND connected_at = $1
	`, connectedAt, reason)
	if err != nil {
		return fmt.Errorf("failed to store the Trakt reauth: %w", err)
	}
	return requireConnection(res)
}

// Purge deletes the connection and everything imported with it, every
// Trakt table, in one transaction. The settings stay.
func (r *TraktAuthRepository) Purge(ctx context.Context) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()
	if err := purgeTrakt(ctx, tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit the Trakt purge: %w", err)
	}
	return nil
}

// Counts reads what's imported.
func (r *TraktAuthRepository) Counts(ctx context.Context) (TraktCounts, error) {
	var c TraktCounts
	err := r.db.QueryRowContext(ctx, `
		SELECT
			(SELECT COUNT(*) FROM trakt_history),
			(SELECT COUNT(*) FROM trakt_ratings),
			(SELECT COUNT(*) FROM trakt_watchlist),
			(SELECT COUNT(*) FROM trakt_watchlist w
			 WHERE (w.media_type = 'movie' AND EXISTS (SELECT 1 FROM movies m WHERE m.tmdb_id = w.tmdb_id))
			    OR (w.media_type = 'tv' AND EXISTS (SELECT 1 FROM shows s WHERE s.tmdb_id = w.tmdb_id)))
	`).Scan(&c.History, &c.Ratings, &c.Watchlist, &c.WatchlistInLibrary)
	if err != nil {
		return TraktCounts{}, fmt.Errorf("failed to count the Trakt data: %w", err)
	}
	return c, nil
}

func purgeTrakt(ctx context.Context, tx *sql.Tx) error {
	for _, table := range traktTables {
		if _, err := tx.ExecContext(ctx, `DELETE FROM `+table); err != nil {
			return fmt.Errorf("failed to purge %s: %w", table, err)
		}
	}
	return nil
}

func requireConnection(res sql.Result) error {
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to read rows affected: %w", err)
	}
	if n == 0 {
		return ErrTraktConnectionReplaced
	}
	return nil
}
