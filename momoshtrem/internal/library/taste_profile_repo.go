package library

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/shapedtime/momoshtrem/internal/member"
)

// StoredTasteProfile is a taste_profiles row: R1's profile as one build
// stored it. Counts and Profile are its JSON documents.
type StoredTasteProfile struct {
	ID        int64
	CreatedAt time.Time
	Counts    json.RawMessage
	Profile   json.RawMessage
}

// TasteProfileRepository stores one household member's taste profiles
// (013). Bind it to the member with ForMember: unbound, Insert and Latest
// fail.
type TasteProfileRepository struct {
	db     *DB
	member boundMember
}

// NewTasteProfileRepository creates a TasteProfileRepository, bound to no
// member yet.
func NewTasteProfileRepository(db *DB) *TasteProfileRepository {
	return &TasteProfileRepository{db: db}
}

// ForMember is the repository for member id's profiles.
func (r *TasteProfileRepository) ForMember(id int64) *TasteProfileRepository {
	return &TasteProfileRepository{db: r.db, member: boundMember(id)}
}

// Insert stores the member's profile and, in the same transaction, prunes
// their ones no run references that were created before pruneBefore (the
// caller's clock, so a profile a run is about to reference survives).
// member.ErrUnknown when the member is gone.
func (r *TasteProfileRepository) Insert(ctx context.Context, counts, profile json.RawMessage, pruneBefore time.Time) (*StoredTasteProfile, error) {
	memberID, err := r.member.id()
	if err != nil {
		return nil, err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin: %w", err)
	}
	defer tx.Rollback()

	p := &StoredTasteProfile{Counts: counts, Profile: profile}
	err = tx.QueryRowContext(ctx, `
		INSERT INTO taste_profiles (member_id, counts, profile) VALUES ($1, $2::jsonb, $3::jsonb) RETURNING id, created_at
	`, memberID, string(counts), string(profile)).Scan(&p.ID, &p.CreatedAt)
	if isForeignKeyViolation(err) {
		return nil, fmt.Errorf("member %d: %w", memberID, member.ErrUnknown)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to store the taste profile: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM taste_profiles t
		WHERE t.member_id = $1 AND t.id <> $2 AND t.created_at < $3
		  AND NOT EXISTS (SELECT 1 FROM rec_runs r WHERE r.profile_id = t.id)
	`, memberID, p.ID, pruneBefore); err != nil {
		return nil, fmt.Errorf("failed to prune taste profiles: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit the taste profile: %w", err)
	}
	return p, nil
}

// Latest returns the member's newest profile, nil when there's none.
func (r *TasteProfileRepository) Latest(ctx context.Context) (*StoredTasteProfile, error) {
	memberID, err := r.member.id()
	if err != nil {
		return nil, err
	}
	return r.one(ctx, `SELECT id, created_at, counts, profile FROM taste_profiles WHERE member_id = $1
		ORDER BY id DESC LIMIT 1`, memberID)
}

// Get returns one profile, whoever's it is; nil when it doesn't exist.
func (r *TasteProfileRepository) Get(ctx context.Context, id int64) (*StoredTasteProfile, error) {
	return r.one(ctx, `SELECT id, created_at, counts, profile FROM taste_profiles WHERE id = $1`, id)
}

func (r *TasteProfileRepository) one(ctx context.Context, query string, args ...any) (*StoredTasteProfile, error) {
	p := &StoredTasteProfile{}
	var counts, profile []byte
	err := r.db.QueryRowContext(ctx, query, args...).Scan(&p.ID, &p.CreatedAt, &counts, &profile)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read the taste profile: %w", err)
	}
	p.Counts, p.Profile = counts, profile
	return p, nil
}
