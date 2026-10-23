package library

import (
	"context"
	"fmt"
	"time"

	"github.com/shapedtime/momoshtrem/internal/member"
)

// RecFeedback is a member's feedback on a title (PLAN §6 "Feedback"):
// like, dislike, not_interested or seen, given on a shelf, the hero, Ask
// or the title page. It isn't Trakt data, so a Trakt disconnect keeps it;
// it goes with the member (013).
type RecFeedback struct {
	TitleRef
	Signal    string
	Source    string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// RecFeedbackRepository stores feedback in rec_feedback, one signal per
// member and title.
type RecFeedbackRepository struct {
	db *DB
}

// NewRecFeedbackRepository creates a RecFeedbackRepository.
func NewRecFeedbackRepository(db *DB) *RecFeedbackRepository {
	return &RecFeedbackRepository{db: db}
}

// Set upserts the member's feedback on a title: the newest signal wins,
// created_at stays and updated_at moves. member.ErrUnknown when the member
// is gone (removed while the request ran).
func (r *RecFeedbackRepository) Set(ctx context.Context, memberID int64, ref TitleRef, signal, source string) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO rec_feedback (member_id, media_type, tmdb_id, signal, source) VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (member_id, media_type, tmdb_id) DO UPDATE SET
			signal = EXCLUDED.signal, source = EXCLUDED.source, updated_at = NOW()
	`, memberID, ref.MediaType, ref.TMDBID, signal, source)
	if isForeignKeyViolation(err) {
		return fmt.Errorf("member %d: %w", memberID, member.ErrUnknown)
	}
	if err != nil {
		return fmt.Errorf("failed to save feedback on %s %d: %w", ref.MediaType, ref.TMDBID, err)
	}
	return nil
}

// Clear removes the member's feedback on a title; clearing none is no
// error.
func (r *RecFeedbackRepository) Clear(ctx context.Context, memberID int64, ref TitleRef) error {
	if _, err := r.db.ExecContext(ctx, `DELETE FROM rec_feedback WHERE member_id = $1 AND media_type = $2 AND tmdb_id = $3`,
		memberID, ref.MediaType, ref.TMDBID); err != nil {
		return fmt.Errorf("failed to clear feedback on %s %d: %w", ref.MediaType, ref.TMDBID, err)
	}
	return nil
}

// List returns the member's feedback on every title, newest first.
func (r *RecFeedbackRepository) List(ctx context.Context, memberID int64) ([]RecFeedback, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT media_type, tmdb_id, signal, source, created_at, updated_at
		FROM rec_feedback WHERE member_id = $1 ORDER BY updated_at DESC, media_type, tmdb_id
	`, memberID)
	if err != nil {
		return nil, fmt.Errorf("failed to list feedback: %w", err)
	}
	defer rows.Close()

	var out []RecFeedback
	for rows.Next() {
		var f RecFeedback
		if err := rows.Scan(&f.MediaType, &f.TMDBID, &f.Signal, &f.Source, &f.CreatedAt, &f.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan feedback: %w", err)
		}
		out = append(out, f)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read feedback: %w", err)
	}
	return out, nil
}
