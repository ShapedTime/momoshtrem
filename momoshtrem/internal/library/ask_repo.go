package library

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/shapedtime/momoshtrem/internal/member"
)

// Ask momoshtrem's threads (PLAN §5 006, §6 "Ask momoshtrem"; R3, #158):
// ask_threads, ask_turns and ask_messages. A turn starts in one
// transaction and ends in another; its raw API messages are written only
// when it ends done, so a cancelled or failed turn never replays. Threads
// and turns are a household member's (014, #246): a turn is its thread's
// member's, and each member has one streaming turn at most.

// Ask turn statuses (ask_turns.status).
const (
	AskTurnStreaming = "streaming"
	AskTurnDone      = "done"
	AskTurnError     = "error"
	AskTurnCancelled = "cancelled"
)

// ErrAskTurnStreaming: another of the member's turns is streaming (the
// partial unique index on ask_turns allows one per member, in any of their
// threads).
var ErrAskTurnStreaming = errors.New("another ask turn is streaming")

// AskThread is a conversation.
type AskThread struct {
	ID int64
	// MemberID is whose thread it is: only they read it or follow it up.
	MemberID int64
	Title    string
	// Model and PromptVersion are the first turn's, kept for the thread's
	// life: thinking blocks are bound to the model, and a replay must send
	// the same system prompt and tools.
	Model         string
	PromptVersion string
	CreatedAt     time.Time
	UpdatedAt     time.Time
	TurnCount     int
}

// AskTurn is one ask and its answer. View is what the thread page shows
// (JSON), written when the turn ends; nil while it streams.
type AskTurn struct {
	ID         int64
	ThreadID   int64
	Position   int
	Message    string
	Status     string
	View       []byte
	ErrorCode  string
	CostUSD    float64
	CreatedAt  time.Time
	FinishedAt *time.Time
}

// AskMessage is one API message: its role and its content blocks as a
// JSON array, byte for byte as they were sent (ask_messages.content is
// json, not jsonb, so the text comes back unchanged).
type AskMessage struct {
	Role    string
	Content []byte
}

// NewAskTurn starts the member's turn: in ThreadID (theirs), or in a new
// thread of theirs with Title, Model and PromptVersion when ThreadID is 0.
type NewAskTurn struct {
	MemberID      int64
	ThreadID      int64
	Title         string
	Model         string
	PromptVersion string
	Message       string
}

// AskTurnEnd is how a turn ended. Messages are written only with
// AskTurnDone.
type AskTurnEnd struct {
	TurnID    int64
	Status    string // AskTurnDone, AskTurnError or AskTurnCancelled
	View      []byte // JSON
	ErrorCode string // "" for none
	CostUSD   float64
	Messages  []AskMessage
}

// AskRepository stores the threads.
type AskRepository struct {
	db *DB
}

// NewAskRepository creates an AskRepository.
func NewAskRepository(db *DB) *AskRepository {
	return &AskRepository{db: db}
}

const askThreadColumns = `t.id, t.member_id, t.title, t.model, t.prompt_version, t.created_at, t.updated_at,
	(SELECT count(*) FROM ask_turns n WHERE n.thread_id = t.id)`

func scanAskThread(row interface{ Scan(...any) error }) (AskThread, error) {
	var t AskThread
	err := row.Scan(&t.ID, &t.MemberID, &t.Title, &t.Model, &t.PromptVersion, &t.CreatedAt, &t.UpdatedAt, &t.TurnCount)
	return t, err
}

// Thread returns a thread, whoever's it is (the caller checks MemberID),
// with its turn count; nil for an unknown one.
func (r *AskRepository) Thread(ctx context.Context, id int64) (*AskThread, error) {
	t, err := scanAskThread(r.db.QueryRowContext(ctx, `SELECT `+askThreadColumns+` FROM ask_threads t WHERE t.id = $1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read ask thread %d: %w", id, err)
	}
	return &t, nil
}

// Threads lists the member's threads, the newest updated first, at most
// limit.
func (r *AskRepository) Threads(ctx context.Context, memberID int64, limit int) ([]AskThread, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+askThreadColumns+`
		FROM ask_threads t WHERE t.member_id = $1 ORDER BY t.updated_at DESC, t.id DESC LIMIT $2`, memberID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list member %d's ask threads: %w", memberID, err)
	}
	defer rows.Close()
	var out []AskThread
	for rows.Next() {
		t, err := scanAskThread(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan an ask thread: %w", err)
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to list ask threads: %w", err)
	}
	return out, nil
}

// Turns returns a thread's turns in order.
func (r *AskRepository) Turns(ctx context.Context, threadID int64) ([]AskTurn, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, thread_id, position, message, status, view, COALESCE(error_code, ''), cost_usd::float8, created_at, finished_at
		FROM ask_turns WHERE thread_id = $1 ORDER BY position
	`, threadID)
	if err != nil {
		return nil, fmt.Errorf("failed to read the turns of ask thread %d: %w", threadID, err)
	}
	defer rows.Close()
	var out []AskTurn
	for rows.Next() {
		var (
			t        AskTurn
			finished sql.NullTime
		)
		if err := rows.Scan(&t.ID, &t.ThreadID, &t.Position, &t.Message, &t.Status, &t.View, &t.ErrorCode, &t.CostUSD,
			&t.CreatedAt, &finished); err != nil {
			return nil, fmt.Errorf("failed to scan an ask turn: %w", err)
		}
		t.FinishedAt = nullTimePtr(finished)
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read the turns of ask thread %d: %w", threadID, err)
	}
	return out, nil
}

// Messages returns a thread's API messages in order, each exactly as it
// was stored.
func (r *AskRepository) Messages(ctx context.Context, threadID int64) ([]AskMessage, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT role, content::text FROM ask_messages WHERE thread_id = $1 ORDER BY position
	`, threadID)
	if err != nil {
		return nil, fmt.Errorf("failed to read the messages of ask thread %d: %w", threadID, err)
	}
	defer rows.Close()
	var out []AskMessage
	for rows.Next() {
		var (
			m       AskMessage
			content string
		)
		if err := rows.Scan(&m.Role, &content); err != nil {
			return nil, fmt.Errorf("failed to scan an ask message: %w", err)
		}
		m.Content = []byte(content)
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read the messages of ask thread %d: %w", threadID, err)
	}
	return out, nil
}

// StartTurn creates the member's streaming turn (and their thread when
// it's new) in one transaction, and bumps the thread's updated_at.
// ErrAskTurnStreaming when another of their turns is streaming;
// member.ErrUnknown when the member is gone, their threads with them. A
// follow-up in another member's thread is refused the same way (the
// composite FK: a turn is its thread's member's).
func (r *AskRepository) StartTurn(ctx context.Context, in NewAskTurn) (*AskThread, *AskTurn, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to start an ask turn: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	threadID := in.ThreadID
	if threadID == 0 {
		err = tx.QueryRowContext(ctx, `
			INSERT INTO ask_threads (member_id, title, model, prompt_version) VALUES ($1, $2, $3, $4) RETURNING id
		`, in.MemberID, in.Title, in.Model, in.PromptVersion).Scan(&threadID)
		if isForeignKeyViolation(err) {
			return nil, nil, fmt.Errorf("member %d: %w", in.MemberID, member.ErrUnknown)
		}
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create an ask thread: %w", err)
		}
	} else if _, err := tx.ExecContext(ctx, `SELECT 1 FROM ask_threads WHERE id = $1 FOR UPDATE`, threadID); err != nil {
		return nil, nil, fmt.Errorf("failed to lock ask thread %d: %w", threadID, err)
	}

	turn := AskTurn{ThreadID: threadID, Message: in.Message, Status: AskTurnStreaming}
	// The one-streaming index (and the thread's positions) are the
	// arbiters: no row back means another of the member's turns is
	// streaming.
	err = tx.QueryRowContext(ctx, `
		INSERT INTO ask_turns (member_id, thread_id, position, message, status)
		VALUES ($1, $2, (SELECT COALESCE(MAX(position) + 1, 0) FROM ask_turns WHERE thread_id = $2), $3, 'streaming')
		ON CONFLICT DO NOTHING
		RETURNING id, position, created_at
	`, in.MemberID, threadID, in.Message).Scan(&turn.ID, &turn.Position, &turn.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, ErrAskTurnStreaming
	}
	if isForeignKeyViolation(err) {
		return nil, nil, fmt.Errorf("member %d, ask thread %d: %w", in.MemberID, threadID, member.ErrUnknown)
	}
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create an ask turn: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE ask_threads SET updated_at = NOW() WHERE id = $1`, threadID); err != nil {
		return nil, nil, fmt.Errorf("failed to touch ask thread %d: %w", threadID, err)
	}
	thread, err := scanAskThread(tx.QueryRowContext(ctx, `SELECT `+askThreadColumns+` FROM ask_threads t WHERE t.id = $1`, threadID))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read ask thread %d: %w", threadID, err)
	}
	if err := tx.Commit(); err != nil {
		return nil, nil, fmt.Errorf("failed to start an ask turn: %w", err)
	}
	return &thread, &turn, nil
}

// CancelTurn marks a streaming turn cancelled at once, which frees the
// one-streaming slot before the turn's usage and view are written
// (EndTurn). A turn that isn't streaming is left as it is.
func (r *AskRepository) CancelTurn(ctx context.Context, turnID int64) error {
	if _, err := r.db.ExecContext(ctx, `
		UPDATE ask_turns SET status = 'cancelled', finished_at = NOW() WHERE id = $1 AND status = 'streaming'
	`, turnID); err != nil {
		return fmt.Errorf("failed to cancel ask turn %d: %w", turnID, err)
	}
	return nil
}

// EndTurn writes how a turn ended, in one transaction: its status, view,
// error code and cost, and, when it ended done, its messages after the
// thread's. A streaming turn takes any end; a cancelled one (CancelTurn)
// only a cancelled end, its view and cost. It reports whether the turn
// took the end: false when it had already ended otherwise, and then
// nothing is written.
func (r *AskRepository) EndTurn(ctx context.Context, end AskTurnEnd) (bool, error) {
	if end.Status != AskTurnDone && len(end.Messages) > 0 {
		return false, fmt.Errorf("ask turn %d ended %s: only a done turn's messages are stored", end.TurnID, end.Status)
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("failed to end ask turn %d: %w", end.TurnID, err)
	}
	defer func() { _ = tx.Rollback() }()

	var errorCode *string
	if end.ErrorCode != "" {
		errorCode = &end.ErrorCode
	}
	var threadID int64
	err = tx.QueryRowContext(ctx, `
		UPDATE ask_turns
		SET status = $2, view = $3::jsonb, error_code = $4, cost_usd = $5, finished_at = COALESCE(finished_at, NOW())
		WHERE id = $1 AND (status = 'streaming' OR (status = 'cancelled' AND $2 = 'cancelled'))
		RETURNING thread_id
	`, end.TurnID, end.Status, jsonParam(end.View), errorCode, end.CostUSD).Scan(&threadID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to end ask turn %d: %w", end.TurnID, err)
	}
	if len(end.Messages) > 0 {
		// The thread row's lock orders two writers (there's one turn at a
		// time, but a crash's leftovers must never interleave).
		if _, err := tx.ExecContext(ctx, `SELECT 1 FROM ask_threads WHERE id = $1 FOR UPDATE`, threadID); err != nil {
			return false, fmt.Errorf("failed to lock ask thread %d: %w", threadID, err)
		}
		var next int
		if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(position) + 1, 0) FROM ask_messages WHERE thread_id = $1`,
			threadID).Scan(&next); err != nil {
			return false, fmt.Errorf("failed to read ask thread %d's messages: %w", threadID, err)
		}
		for i, m := range end.Messages {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO ask_messages (thread_id, turn_id, position, role, content) VALUES ($1, $2, $3, $4, $5::json)
			`, threadID, end.TurnID, next+i, m.Role, string(m.Content)); err != nil {
				return false, fmt.Errorf("failed to store ask turn %d's messages: %w", end.TurnID, err)
			}
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE ask_threads SET updated_at = NOW() WHERE id = $1`, threadID); err != nil {
		return false, fmt.Errorf("failed to touch ask thread %d: %w", threadID, err)
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("failed to end ask turn %d: %w", end.TurnID, err)
	}
	return true, nil
}

// AbandonTurn ends a streaming turn as an error, writing nothing else: the
// last resort when its end can't be written (EndTurn keeps failing), so
// the one-streaming index is free. A turn that isn't streaming is left as
// it is.
func (r *AskRepository) AbandonTurn(ctx context.Context, turnID int64) error {
	if _, err := r.db.ExecContext(ctx, `
		UPDATE ask_turns SET status = 'error', error_code = 'internal', finished_at = NOW() WHERE id = $1 AND status = 'streaming'
	`, turnID); err != nil {
		return fmt.Errorf("failed to abandon ask turn %d: %w", turnID, err)
	}
	return nil
}

// FailStreaming ends every streaming turn, everyone's, as an error with
// code and view: at startup, the turns a crash left streaming. It returns
// how many.
func (r *AskRepository) FailStreaming(ctx context.Context, code string, view []byte) (int64, error) {
	res, err := r.db.ExecContext(ctx, `
		UPDATE ask_turns SET status = 'error', error_code = $1, view = $2::jsonb, finished_at = NOW()
		WHERE status = 'streaming'
	`, code, jsonParam(view))
	if err != nil {
		return 0, fmt.Errorf("failed to end the ask turns left streaming: %w", err)
	}
	return res.RowsAffected()
}

// TurnsSince counts the member's turns started since a time, whatever
// their status (Ask's daily limit).
func (r *AskRepository) TurnsSince(ctx context.Context, memberID int64, since time.Time) (int, error) {
	var n int
	if err := r.db.QueryRowContext(ctx, `SELECT count(*) FROM ask_turns WHERE member_id = $1 AND created_at >= $2`,
		memberID, since).Scan(&n); err != nil {
		return 0, fmt.Errorf("failed to count member %d's ask turns: %w", memberID, err)
	}
	return n, nil
}

// MeanDoneCost is the mean cost of the newest n done turns, anyone's; nil
// without any.
func (r *AskRepository) MeanDoneCost(ctx context.Context, n int) (*float64, error) {
	var mean sql.NullFloat64
	if err := r.db.QueryRowContext(ctx, `
		SELECT avg(cost_usd)::float8 FROM (
			SELECT cost_usd FROM ask_turns WHERE status = 'done' ORDER BY finished_at DESC NULLS LAST, id DESC LIMIT $1
		) recent
	`, n).Scan(&mean); err != nil {
		return nil, fmt.Errorf("failed to average the ask costs: %w", err)
	}
	if !mean.Valid {
		return nil, nil
	}
	return &mean.Float64, nil
}

// jsonParam is a JSON document as a query parameter: NULL when empty.
func jsonParam(doc []byte) any {
	if len(doc) == 0 {
		return nil
	}
	return string(doc)
}
