package library

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/shapedtime/momoshtrem/internal/member"
)

// memberLockID is the pg_advisory_xact_lock key Create and Delete hold, so
// neither the cap nor "never zero members" can race, from any process.
// ASCII "momomemb".
const memberLockID int64 = 0x6d6f6d6f6d656d62

// MemberRepository stores the household's members (migration 011). It
// implements member.Store.
type MemberRepository struct {
	db *DB
}

var _ member.Store = (*MemberRepository)(nil)

// NewMemberRepository creates a members repository.
func NewMemberRepository(db *DB) *MemberRepository {
	return &MemberRepository{db: db}
}

// List is every member, by id.
func (r *MemberRepository) List(ctx context.Context) ([]member.Member, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, name, color, created_at FROM members ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("failed to list the members: %w", err)
	}
	defer rows.Close()
	var out []member.Member
	for rows.Next() {
		var m member.Member
		if err := rows.Scan(&m.ID, &m.Name, &m.Color, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan a member: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// Create adds a member, under the members' lock: member.ErrLimit with
// member.MaxMembers already, member.ErrNameTaken for a name another member
// has (ignoring case). An empty color is member.DefaultColor of the
// others'. The name and colour are the caller's to check (member.Service).
func (r *MemberRepository) Create(ctx context.Context, name, color string) (member.Member, error) {
	tx, err := r.lockMembers(ctx)
	if err != nil {
		return member.Member{}, err
	}
	defer tx.Rollback()

	used, err := memberColors(ctx, tx)
	if err != nil {
		return member.Member{}, err
	}
	if len(used) >= member.MaxMembers {
		return member.Member{}, member.ErrLimit
	}
	if color == "" {
		color = member.DefaultColor(used)
	}

	m := member.Member{Name: name, Color: color}
	err = tx.QueryRowContext(ctx, `INSERT INTO members (name, color) VALUES ($1, $2) RETURNING id, created_at`,
		name, color).Scan(&m.ID, &m.CreatedAt)
	if isUniqueViolation(err) {
		return member.Member{}, member.ErrNameTaken
	}
	if err != nil {
		return member.Member{}, fmt.Errorf("failed to add the member: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return member.Member{}, fmt.Errorf("failed to commit the member: %w", err)
	}
	return m, nil
}

// Update changes the fields set: member.ErrNotFound, member.ErrNameTaken.
func (r *MemberRepository) Update(ctx context.Context, id int64, p member.Patch) (member.Member, error) {
	m := member.Member{ID: id}
	err := r.db.QueryRowContext(ctx, `
		UPDATE members SET name = COALESCE($2, name), color = COALESCE($3, color), updated_at = NOW()
		WHERE id = $1
		RETURNING name, color, created_at
	`, id, p.Name, p.Color).Scan(&m.Name, &m.Color, &m.CreatedAt)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return member.Member{}, member.ErrNotFound
	case isUniqueViolation(err):
		return member.Member{}, member.ErrNameTaken
	case err != nil:
		return member.Member{}, fmt.Errorf("failed to update member %d: %w", id, err)
	}
	return m, nil
}

// Delete removes a member, under the members' lock: member.ErrNotFound,
// member.ErrLastMember when it's the only one. What's theirs goes with the
// row (the member_id foreign keys cascade, from 012).
func (r *MemberRepository) Delete(ctx context.Context, id int64) error {
	tx, err := r.lockMembers(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var count int
	var exists bool
	if err := tx.QueryRowContext(ctx, `SELECT count(*), COALESCE(bool_or(id = $1), FALSE) FROM members`, id).
		Scan(&count, &exists); err != nil {
		return fmt.Errorf("failed to read the members: %w", err)
	}
	switch {
	case !exists:
		return member.ErrNotFound
	case count == 1:
		return member.ErrLastMember
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM members WHERE id = $1`, id); err != nil {
		return fmt.Errorf("failed to remove member %d: %w", id, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit removing member %d: %w", id, err)
	}
	return nil
}

// lockMembers begins a transaction holding the members' lock until it
// ends.
func (r *MemberRepository) lockMembers(ctx context.Context) (*sql.Tx, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, memberLockID); err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to lock the members: %w", err)
	}
	return tx, nil
}

// TraktSettings are what momoshtrem takes from a member's Trakt
// (members.trakt_settings, 012): one JSON document shaped like PUT
// /integrations/trakt/settings answers (PLAN §6 "Trakt (B5)"). Fields it
// doesn't hold take their defaults; a disconnect keeps it, and it goes
// with the member.
type TraktSettings struct {
	// UseRatings lets recommendations (#9) weigh Trakt ratings.
	UseRatings bool `json:"use_ratings"`
	// ShowWatchlist shows the watchlist on For You, and Up next's
	// watchlist starts.
	ShowWatchlist bool `json:"show_watchlist"`
	// IncludePaused keeps shows idle for over 180 days in Up next.
	IncludePaused bool `json:"include_paused"`
}

// DefaultTraktSettings are the settings before anyone changes them.
func DefaultTraktSettings() TraktSettings {
	return TraktSettings{UseRatings: true, ShowWatchlist: true}
}

// TraktSettingsPatch is a partial update: a nil field keeps its value.
type TraktSettingsPatch struct {
	UseRatings    *bool `json:"use_ratings,omitempty"`
	ShowWatchlist *bool `json:"show_watchlist,omitempty"`
	IncludePaused *bool `json:"include_paused,omitempty"`
}

// TraktSettings reads member id's Trakt settings over their defaults: a
// field the stored document doesn't have keeps its default. An unknown
// member gives the defaults and member.ErrNotFound, a document that isn't
// the settings the defaults and the error.
func (r *MemberRepository) TraktSettings(ctx context.Context, id int64) (TraktSettings, error) {
	var raw []byte
	err := r.db.QueryRowContext(ctx, `SELECT trakt_settings FROM members WHERE id = $1`, id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return DefaultTraktSettings(), member.ErrNotFound
	}
	if err != nil {
		return DefaultTraktSettings(), fmt.Errorf("failed to read member %d's Trakt settings: %w", id, err)
	}
	return decodeTraktSettings(raw)
}

// UpdateTraktSettings stores the fields patch sets for member id and
// answers the settings it leaves; member.ErrNotFound for an unknown
// member. It's one statement (jsonb ||), so two updates of different
// fields never lose each other's change.
func (r *MemberRepository) UpdateTraktSettings(ctx context.Context, id int64, patch TraktSettingsPatch) (TraktSettings, error) {
	set, err := json.Marshal(patch)
	if err != nil {
		return DefaultTraktSettings(), fmt.Errorf("failed to encode the Trakt settings: %w", err)
	}
	var raw []byte
	err = r.db.QueryRowContext(ctx, `
		UPDATE members SET
			trakt_settings = CASE WHEN jsonb_typeof(trakt_settings) = 'object' THEN trakt_settings ELSE '{}'::jsonb END || $2::jsonb,
			updated_at = NOW()
		WHERE id = $1
		RETURNING trakt_settings
	`, id, string(set)).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return DefaultTraktSettings(), member.ErrNotFound
	}
	if err != nil {
		return DefaultTraktSettings(), fmt.Errorf("failed to update member %d's Trakt settings: %w", id, err)
	}
	return decodeTraktSettings(raw)
}

// TraktSettingsFor is member id's Trakt settings, for what's built for one
// member: their trakt.Auth and taste profile.
func (r *MemberRepository) TraktSettingsFor(id int64) MemberTraktSettings {
	return MemberTraktSettings{repo: r, member: id}
}

// MemberTraktSettings are one member's Trakt settings (TraktSettingsFor).
type MemberTraktSettings struct {
	repo   *MemberRepository
	member int64
}

// TraktSettings reads the member's (MemberRepository.TraktSettings).
func (s MemberTraktSettings) TraktSettings(ctx context.Context) (TraktSettings, error) {
	return s.repo.TraktSettings(ctx, s.member)
}

// UpdateTraktSettings stores the member's
// (MemberRepository.UpdateTraktSettings).
func (s MemberTraktSettings) UpdateTraktSettings(ctx context.Context, patch TraktSettingsPatch) (TraktSettings, error) {
	return s.repo.UpdateTraktSettings(ctx, s.member, patch)
}

func decodeTraktSettings(raw []byte) (TraktSettings, error) {
	settings := DefaultTraktSettings()
	// Unmarshal only overwrites the fields present in raw.
	if err := json.Unmarshal(raw, &settings); err != nil {
		return DefaultTraktSettings(), fmt.Errorf("failed to decode the Trakt settings: %w", err)
	}
	return settings, nil
}

// memberColors is every member's colour, one per member.
func memberColors(ctx context.Context, tx *sql.Tx) ([]string, error) {
	rows, err := tx.QueryContext(ctx, `SELECT color FROM members`)
	if err != nil {
		return nil, fmt.Errorf("failed to read the members: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, fmt.Errorf("failed to scan a member: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
