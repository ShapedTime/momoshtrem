package library

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

var (
	// ErrTraktConnectionReplaced means a write named a connection that
	// isn't the stored one any more: a disconnect or a reconnect replaced
	// it.
	ErrTraktConnectionReplaced = errors.New("the Trakt connection was replaced")
	// ErrTraktAccountInUse means another member has connected the Trakt
	// account (an account connects to one member: trakt_auth_user_slug_key).
	ErrTraktAccountInUse = errors.New("another member has connected that Trakt account")
	// errUnbound means a member's repository was used before ForMember.
	errUnbound = errors.New("the repository isn't bound to a household member (ForMember)")
)

// boundMember is the member a repository reads and writes for (ForMember);
// 0 until it's bound.
type boundMember int64

// id is the member, or errUnbound.
func (m boundMember) id() (int64, error) {
	if m <= 0 {
		return 0, errUnbound
	}
	return int64(m), nil
}

// traktTables are every table of migration 005: a disconnect empties them
// of the member's rows. The settings (members.trakt_settings) aren't among
// them.
var traktTables = []string{
	"trakt_history", "trakt_watched_movies", "trakt_watched_episodes", "trakt_ratings",
	"trakt_watchlist", "trakt_show_progress", "trakt_hidden", "trakt_calendar",
	"trakt_sync_state", "trakt_auth",
}

// TraktAuth is a member's Trakt connection (trakt_auth). The tokens are
// sealed with SECRET_KEY (internal/secret). ConnectedAt is the
// connection's generation: writes that name another one are refused.
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

// TraktAuthRepository stores a member's Trakt connection and purges what
// was imported with it. Bind it to the member with ForMember: unbound,
// every method fails.
type TraktAuthRepository struct {
	db     *DB
	member boundMember
}

// NewTraktAuthRepository creates a Trakt connection repository, bound to
// no member yet.
func NewTraktAuthRepository(db *DB) *TraktAuthRepository {
	return &TraktAuthRepository{db: db}
}

// ForMember is the repository for member id: every statement reads and
// writes that member's rows only.
func (r *TraktAuthRepository) ForMember(id int64) *TraktAuthRepository {
	return &TraktAuthRepository{db: r.db, member: boundMember(id)}
}

// Get returns the member's stored connection; nil when there's none.
func (r *TraktAuthRepository) Get(ctx context.Context) (*TraktAuth, error) {
	member, err := r.member.id()
	if err != nil {
		return nil, err
	}
	var a TraktAuth
	var reason sql.NullString
	err = r.db.QueryRowContext(ctx, `
		SELECT username, user_slug, access_token_enc, refresh_token_enc, token_created_at, expires_at,
		       needs_reauth, reauth_reason, connected_at
		FROM trakt_auth WHERE member_id = $1
	`, member).Scan(&a.Username, &a.UserSlug, &a.AccessTokenEnc, &a.RefreshTokenEnc, &a.TokenCreatedAt, &a.ExpiresAt,
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

// Connect stores the member's new connection and answers its generation.
// Connecting another account than the stored one (or with none stored)
// purges the member's rows of every Trakt table first, in the same
// transaction; the same account keeps its imported data. Either way the
// generation is new and reauth is cleared. ErrTraktAccountInUse, with
// nothing changed, when another member has connected the account.
func (r *TraktAuthRepository) Connect(ctx context.Context, a TraktAuth) (time.Time, error) {
	member, err := r.member.id()
	if err != nil {
		return time.Time{}, err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	var slug string
	err = tx.QueryRowContext(ctx, `SELECT user_slug FROM trakt_auth WHERE member_id = $1 FOR UPDATE`, member).Scan(&slug)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, fmt.Errorf("failed to read the Trakt connection: %w", err)
	}
	if errors.Is(err, sql.ErrNoRows) || slug != a.UserSlug {
		if err := purgeTrakt(ctx, tx, member); err != nil {
			return time.Time{}, err
		}
	}

	var connectedAt time.Time
	err = tx.QueryRowContext(ctx, `
		INSERT INTO trakt_auth (member_id, username, user_slug, access_token_enc, refresh_token_enc, token_created_at, expires_at,
		                        needs_reauth, reauth_reason, connected_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, FALSE, NULL, NOW(), NOW())
		ON CONFLICT (member_id) DO UPDATE SET
			username = EXCLUDED.username, user_slug = EXCLUDED.user_slug,
			access_token_enc = EXCLUDED.access_token_enc, refresh_token_enc = EXCLUDED.refresh_token_enc,
			token_created_at = EXCLUDED.token_created_at, expires_at = EXCLUDED.expires_at,
			needs_reauth = FALSE, reauth_reason = NULL, connected_at = NOW(), updated_at = NOW()
		RETURNING connected_at
	`, member, a.Username, a.UserSlug, a.AccessTokenEnc, a.RefreshTokenEnc, a.TokenCreatedAt, a.ExpiresAt).Scan(&connectedAt)
	if isUniqueViolationOn(err, "trakt_auth_user_slug_key") {
		return time.Time{}, ErrTraktAccountInUse
	}
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
	member, err := r.member.id()
	if err != nil {
		return err
	}
	res, err := r.db.ExecContext(ctx, `
		UPDATE trakt_auth SET access_token_enc = $3, refresh_token_enc = $4, token_created_at = $5, expires_at = $6, updated_at = NOW()
		WHERE member_id = $1 AND connected_at = $2
	`, member, connectedAt, t.AccessTokenEnc, t.RefreshTokenEnc, t.CreatedAt, t.ExpiresAt)
	if err != nil {
		return fmt.Errorf("failed to store the Trakt tokens: %w", err)
	}
	return requireConnection(res)
}

// SetReauth records that Trakt stopped accepting the connection's tokens,
// and why (for the user).
func (r *TraktAuthRepository) SetReauth(ctx context.Context, connectedAt time.Time, reason string) error {
	member, err := r.member.id()
	if err != nil {
		return err
	}
	res, err := r.db.ExecContext(ctx, `
		UPDATE trakt_auth SET needs_reauth = TRUE, reauth_reason = $3, updated_at = NOW()
		WHERE member_id = $1 AND connected_at = $2
	`, member, connectedAt, reason)
	if err != nil {
		return fmt.Errorf("failed to store the Trakt reauth: %w", err)
	}
	return requireConnection(res)
}

// Purge deletes the member's connection and everything imported with it,
// their rows of every Trakt table, in one transaction. The settings and
// the other members' rows stay.
func (r *TraktAuthRepository) Purge(ctx context.Context) error {
	member, err := r.member.id()
	if err != nil {
		return err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()
	// The connection's row first, as a sync write takes it
	// (TraktSyncStore): a write in flight finishes before the purge.
	if _, err := tx.ExecContext(ctx, `SELECT 1 FROM trakt_auth WHERE member_id = $1 FOR UPDATE`, member); err != nil {
		return fmt.Errorf("failed to lock the Trakt connection: %w", err)
	}
	if err := purgeTrakt(ctx, tx, member); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit the Trakt purge: %w", err)
	}
	return nil
}

// Counts reads what's imported for the member.
func (r *TraktAuthRepository) Counts(ctx context.Context) (TraktCounts, error) {
	member, err := r.member.id()
	if err != nil {
		return TraktCounts{}, err
	}
	var c TraktCounts
	err = r.db.QueryRowContext(ctx, `
		SELECT
			(SELECT COUNT(*) FROM trakt_history WHERE member_id = $1),
			(SELECT COUNT(*) FROM trakt_ratings WHERE member_id = $1),
			(SELECT COUNT(*) FROM trakt_watchlist WHERE member_id = $1),
			(SELECT COUNT(*) FROM trakt_watchlist w
			 WHERE w.member_id = $1
			   AND ((w.media_type = 'movie' AND EXISTS (SELECT 1 FROM movies m WHERE m.tmdb_id = w.tmdb_id))
			     OR (w.media_type = 'tv' AND EXISTS (SELECT 1 FROM shows s WHERE s.tmdb_id = w.tmdb_id))))
	`, member).Scan(&c.History, &c.Ratings, &c.Watchlist, &c.WatchlistInLibrary)
	if err != nil {
		return TraktCounts{}, fmt.Errorf("failed to count the Trakt data: %w", err)
	}
	return c, nil
}

// purgeTrakt deletes member's rows of every Trakt table.
func purgeTrakt(ctx context.Context, tx *sql.Tx, member int64) error {
	for _, table := range traktTables {
		if _, err := tx.ExecContext(ctx, `DELETE FROM `+table+` WHERE member_id = $1`, member); err != nil {
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

// isUniqueViolationOn reports a unique violation of constraint (or unique
// index).
func isUniqueViolationOn(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == constraint
}
