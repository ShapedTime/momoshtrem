package library

import (
	"context"
	"database/sql"
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
