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

// The picks' runs (R2, #157; PLAN §5 "006_recommendations.sql"): rec_runs
// with their shelves, items and hero picks. Each run is one household
// member's (013, #245), and a member has at most one queued or running
// run (the partial unique index rec_runs_one_active); a run's shelves and
// hero picks are written with it when it succeeds, in one transaction, so
// /home switches from one run's picks to the next at once.

// Run statuses (rec_runs.status).
const (
	RecRunQueued    = "queued"
	RecRunRunning   = "running"
	RecRunSucceeded = "succeeded"
	RecRunFailed    = "failed"
	RecRunSkipped   = "skipped"
)

// ErrRecRunNotActive: the run isn't queued (StartRecRun, SkipRecRun) or
// isn't queued or running any more (the other writes): another process
// finished it, or startup marked it failed.
var ErrRecRunNotActive = errors.New("the run isn't active")

// RecRun is a rec_runs row: one member's run. Progress is its JSON
// document; the usage columns sum its ai_usage rows.
type RecRun struct {
	ID              int64
	MemberID        int64
	Kind            string
	Status          string
	Model           string
	ProfileID       *int64
	Progress        json.RawMessage
	SkipReason      *string
	Error           *string
	APICalls        int
	InputTokens     int64
	OutputTokens    int64
	CacheReadTokens int64
	WebSearches     int
	CostUSD         float64
	CreatedAt       time.Time
	StartedAt       *time.Time
	FinishedAt      *time.Time
}

// RecShelfRow is a shelf as a run writes it: Position orders the run's
// shelves, and Items are in order.
type RecShelfRow struct {
	Slug     string
	Kind     string
	Title    string
	Subtitle *string
	Items    []RecItemRow
}

// RecItemRow is a shelf's title: Meta is its snapshot (title, year,
// poster…), Signals why it was picked (never served).
type RecItemRow struct {
	TitleRef
	Reason  string
	Meta    json.RawMessage
	Signals json.RawMessage
}

// RecHeroPickRow is a hero pick, in order.
type RecHeroPickRow struct {
	TitleRef
	Kicker string
	Reason string
	Meta   json.RawMessage
}

// RecRunRepository stores runs.
type RecRunRepository struct {
	db *DB
}

// NewRecRunRepository creates a RecRunRepository.
func NewRecRunRepository(db *DB) *RecRunRepository {
	return &RecRunRepository{db: db}
}

const recRunColumns = `id, member_id, kind, status, model, profile_id, progress, skip_reason, error, api_calls, input_tokens,
	output_tokens, cache_read_tokens, web_searches, cost_usd::float8, created_at, started_at, finished_at`

func scanRecRun(row interface{ Scan(...any) error }) (*RecRun, error) {
	var (
		r                 RecRun
		profileID         sql.NullInt64
		progress          []byte
		skip, msg         sql.NullString
		started, finished sql.NullTime
	)
	if err := row.Scan(&r.ID, &r.MemberID, &r.Kind, &r.Status, &r.Model, &profileID, &progress, &skip, &msg, &r.APICalls,
		&r.InputTokens, &r.OutputTokens, &r.CacheReadTokens, &r.WebSearches, &r.CostUSD, &r.CreatedAt,
		&started, &finished); err != nil {
		return nil, err
	}
	r.ProfileID = nullInt64Ptr(profileID)
	r.Progress = progress
	if skip.Valid {
		r.SkipReason = &skip.String
	}
	if msg.Valid {
		r.Error = &msg.String
	}
	r.StartedAt, r.FinishedAt = nullTimePtr(started), nullTimePtr(finished)
	return &r, nil
}

func (r *RecRunRepository) one(ctx context.Context, query string, args ...any) (*RecRun, error) {
	run, err := scanRecRun(r.db.QueryRowContext(ctx, query, args...))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read a run: %w", err)
	}
	return run, nil
}

// QueueRecRun queues the member's run of kind on model, unless one of
// theirs is already queued or running: then it answers that one's id and
// queued false (a refresh joins it). The partial unique index makes this
// single-flight across processes too. member.ErrUnknown when the member is
// gone (removed while the request ran).
func (r *RecRunRepository) QueueRecRun(ctx context.Context, memberID int64, kind, model string) (id int64, queued bool, err error) {
	for range 3 {
		err = r.db.QueryRowContext(ctx, `
			INSERT INTO rec_runs (member_id, kind, status, model) VALUES ($1, $2, 'queued', $3)
			ON CONFLICT DO NOTHING RETURNING id
		`, memberID, kind, model).Scan(&id)
		if err == nil {
			return id, true, nil
		}
		if isForeignKeyViolation(err) {
			return 0, false, fmt.Errorf("member %d: %w", memberID, member.ErrUnknown)
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return 0, false, fmt.Errorf("failed to queue a %s run: %w", kind, err)
		}
		// One is active: join it, unless it finished in between.
		active, err := r.ActiveRecRun(ctx, memberID)
		if err != nil {
			return 0, false, err
		}
		if active != nil {
			return active.ID, false, nil
		}
	}
	return 0, false, fmt.Errorf("failed to queue a %s run: the active run kept changing", kind)
}

// StartRecRun moves a queued run to running with its first progress.
func (r *RecRunRepository) StartRecRun(ctx context.Context, id int64, progress json.RawMessage) error {
	return r.update(ctx, `
		UPDATE rec_runs SET status = 'running', started_at = NOW(), progress = $2::jsonb
		WHERE id = $1 AND status = 'queued'
	`, id, jsonOrEmpty(progress))
}

// SetRecRunProfile records the taste profile a running run built.
func (r *RecRunRepository) SetRecRunProfile(ctx context.Context, id, profileID int64) error {
	return r.update(ctx, `UPDATE rec_runs SET profile_id = $2 WHERE id = $1 AND status = 'running'`, id, profileID)
}

// SetRecRunProgress writes a running run's progress.
func (r *RecRunRepository) SetRecRunProgress(ctx context.Context, id int64, progress json.RawMessage) error {
	return r.update(ctx, `UPDATE rec_runs SET progress = $2::jsonb WHERE id = $1 AND status = 'running'`, id, jsonOrEmpty(progress))
}

// SyncRecRunUsage sets a run's usage columns to the sums of its ai_usage
// rows (feature recs, ref_id the run), whatever its status: exact however
// many calls landed, and safe to repeat.
func (r *RecRunRepository) SyncRecRunUsage(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE rec_runs SET (api_calls, input_tokens, output_tokens, cache_read_tokens, web_searches, cost_usd) = (
			SELECT COUNT(*), COALESCE(SUM(u.input_tokens), 0), COALESCE(SUM(u.output_tokens), 0),
			       COALESCE(SUM(u.cache_read_tokens), 0), COALESCE(SUM(u.web_searches), 0), COALESCE(SUM(u.cost_usd), 0)
			FROM ai_usage u WHERE u.feature = $2 AND u.ref_id = $1
		)
		WHERE id = $1
	`, id, AIFeatureRecs)
	if err != nil {
		return fmt.Errorf("failed to sum run %d's usage: %w", id, err)
	}
	return nil
}

// PublishRecRun writes a running run's shelves and hero picks and marks it
// succeeded, in one transaction: /home moves to it at once.
func (r *RecRunRepository) PublishRecRun(ctx context.Context, id int64, progress json.RawMessage,
	shelves []RecShelfRow, picks []RecHeroPickRow) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin: %w", err)
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx, `
		UPDATE rec_runs SET status = 'succeeded', finished_at = NOW(), progress = $2::jsonb, error = NULL
		WHERE id = $1 AND status = 'running'
	`, id, jsonOrEmpty(progress))
	if err != nil {
		return fmt.Errorf("failed to finish run %d: %w", id, err)
	}
	if n, err := res.RowsAffected(); err != nil || n == 0 {
		return fmt.Errorf("run %d: %w", id, ErrRecRunNotActive)
	}
	for i, shelf := range shelves {
		var shelfID int64
		if err := tx.QueryRowContext(ctx, `
			INSERT INTO rec_shelves (run_id, position, slug, kind, title, subtitle) VALUES ($1, $2, $3, $4, $5, $6)
			RETURNING id
		`, id, i, shelf.Slug, shelf.Kind, shelf.Title, shelf.Subtitle).Scan(&shelfID); err != nil {
			return fmt.Errorf("failed to store shelf %q: %w", shelf.Slug, err)
		}
		for j, item := range shelf.Items {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO rec_items (shelf_id, position, media_type, tmdb_id, reason, meta, signals)
				VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7::jsonb)
			`, shelfID, j, item.MediaType, item.TMDBID, item.Reason, jsonOrEmpty(item.Meta), jsonOrNil(item.Signals)); err != nil {
				return fmt.Errorf("failed to store %s %d on shelf %q: %w", item.MediaType, item.TMDBID, shelf.Slug, err)
			}
		}
	}
	for i, p := range picks {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO rec_hero_picks (run_id, position, media_type, tmdb_id, kicker, reason, meta)
			VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb)
		`, id, i, p.MediaType, p.TMDBID, p.Kicker, p.Reason, jsonOrEmpty(p.Meta)); err != nil {
			return fmt.Errorf("failed to store hero pick %s %d: %w", p.MediaType, p.TMDBID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit run %d: %w", id, err)
	}
	return nil
}

// FailRecRun marks a queued or running run failed, with a message for the
// user and its progress (nil keeps it).
func (r *RecRunRepository) FailRecRun(ctx context.Context, id int64, message string, progress json.RawMessage) error {
	return r.update(ctx, `
		UPDATE rec_runs SET status = 'failed', finished_at = NOW(), error = $2,
		       progress = COALESCE($3::jsonb, progress)
		WHERE id = $1 AND status IN ('queued', 'running')
	`, id, message, jsonOrNil(progress))
}

// SkipRecRun marks a queued run skipped (budget_exceeded or no_history):
// it never started.
func (r *RecRunRepository) SkipRecRun(ctx context.Context, id int64, reason string) error {
	return r.update(ctx, `
		UPDATE rec_runs SET status = 'skipped', skip_reason = $2, finished_at = NOW()
		WHERE id = $1 AND status = 'queued'
	`, id, reason)
}

// FailActiveRecRuns marks every queued or running run failed (startup
// without a key: none of them can run), answering how many.
func (r *RecRunRepository) FailActiveRecRuns(ctx context.Context, message string) (int64, error) {
	res, err := r.db.ExecContext(ctx, `
		UPDATE rec_runs SET status = 'failed', finished_at = NOW(), error = $1
		WHERE status IN ('queued', 'running')
	`, message)
	if err != nil {
		return 0, fmt.Errorf("failed to fail the runs left active: %w", err)
	}
	return res.RowsAffected()
}

// FailRunningRecRuns marks every running run failed (startup: a crash
// stopped it), answering how many. Queued ones stay, for the worker.
func (r *RecRunRepository) FailRunningRecRuns(ctx context.Context, message string) (int64, error) {
	res, err := r.db.ExecContext(ctx, `
		UPDATE rec_runs SET status = 'failed', finished_at = NOW(), error = $1
		WHERE status = 'running'
	`, message)
	if err != nil {
		return 0, fmt.Errorf("failed to fail the runs left running: %w", err)
	}
	return res.RowsAffected()
}

// PruneRecRuns deletes each member's runs beyond their newest keep (by
// id), with their shelves and hero picks. It never deletes an active run,
// a member's newest succeeded one (whose picks /home serves), or a run
// that finished since monthStart, the budget month's start (the usage
// page finds whose the month's spend is through its runs).
func (r *RecRunRepository) PruneRecRuns(ctx context.Context, keep int, monthStart time.Time) (int64, error) {
	res, err := r.db.ExecContext(ctx, `
		DELETE FROM rec_runs r
		WHERE r.status NOT IN ('queued', 'running')
		  AND r.finished_at < $2
		  AND r.id NOT IN (SELECT id FROM (
		          SELECT id, ROW_NUMBER() OVER (PARTITION BY member_id ORDER BY id DESC) AS n FROM rec_runs
		      ) newest WHERE n <= $1)
		  AND r.id NOT IN (SELECT DISTINCT ON (member_id) id FROM rec_runs WHERE status = 'succeeded'
		                   ORDER BY member_id, finished_at DESC, id DESC)
	`, keep, monthStart)
	if err != nil {
		return 0, fmt.Errorf("failed to prune runs: %w", err)
	}
	return res.RowsAffected()
}

// ActiveRecRun is the member's queued or running run, nil when there's
// none.
func (r *RecRunRepository) ActiveRecRun(ctx context.Context, memberID int64) (*RecRun, error) {
	return r.one(ctx, `SELECT `+recRunColumns+` FROM rec_runs WHERE member_id = $1 AND status IN ('queued', 'running')
		ORDER BY id DESC LIMIT 1`, memberID)
}

// ActiveRecRuns are every member's queued and running runs (one a member
// at most), oldest queued first.
func (r *RecRunRepository) ActiveRecRuns(ctx context.Context) ([]RecRun, error) {
	return r.list(ctx, `SELECT `+recRunColumns+` FROM rec_runs WHERE status IN ('queued', 'running')
		ORDER BY created_at, id`)
}

// NextQueuedRecRun is the oldest queued run, whoever's it is: the worker's
// next. Nil when none is queued.
func (r *RecRunRepository) NextQueuedRecRun(ctx context.Context) (*RecRun, error) {
	return r.one(ctx, `SELECT `+recRunColumns+` FROM rec_runs WHERE status = 'queued' ORDER BY created_at, id LIMIT 1`)
}

// GetRecRun is one run, whoever's it is; nil when it doesn't exist.
func (r *RecRunRepository) GetRecRun(ctx context.Context, id int64) (*RecRun, error) {
	return r.one(ctx, `SELECT `+recRunColumns+` FROM rec_runs WHERE id = $1`, id)
}

// LatestSucceededRecRun is the member's newest succeeded run (by when it
// finished), nil before their first.
func (r *RecRunRepository) LatestSucceededRecRun(ctx context.Context, memberID int64) (*RecRun, error) {
	return r.one(ctx, `SELECT `+recRunColumns+` FROM rec_runs WHERE member_id = $1 AND status = 'succeeded'
		ORDER BY finished_at DESC, id DESC LIMIT 1`, memberID)
}

// LatestFinishedRecRun is the member's newest run that succeeded, failed
// or was skipped, nil before their first.
func (r *RecRunRepository) LatestFinishedRecRun(ctx context.Context, memberID int64) (*RecRun, error) {
	return r.one(ctx, `SELECT `+recRunColumns+` FROM rec_runs
		WHERE member_id = $1 AND status IN ('succeeded', 'failed', 'skipped')
		ORDER BY finished_at DESC NULLS LAST, id DESC LIMIT 1`, memberID)
}

// LatestRecRunCreatedAt is when the newest run of any kind, anyone's, was
// queued; ok is false before the first (the schedule's catch-up).
func (r *RecRunRepository) LatestRecRunCreatedAt(ctx context.Context) (at time.Time, ok bool, err error) {
	var t sql.NullTime
	if err := r.db.QueryRowContext(ctx, `SELECT MAX(created_at) FROM rec_runs`).Scan(&t); err != nil {
		return time.Time{}, false, fmt.Errorf("failed to read the newest run: %w", err)
	}
	return t.Time, t.Valid, nil
}

// ListRecRuns lists the member's newest runs first (id DESC).
func (r *RecRunRepository) ListRecRuns(ctx context.Context, memberID int64, limit int) ([]RecRun, error) {
	return r.list(ctx, `SELECT `+recRunColumns+` FROM rec_runs WHERE member_id = $1 ORDER BY id DESC LIMIT $2`, memberID, limit)
}

func (r *RecRunRepository) list(ctx context.Context, query string, args ...any) ([]RecRun, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list runs: %w", err)
	}
	defer rows.Close()
	var out []RecRun
	for rows.Next() {
		run, err := scanRecRun(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan a run: %w", err)
		}
		out = append(out, *run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read runs: %w", err)
	}
	return out, nil
}

// RecRunShelves is a run's shelves with their items, in order. One query.
func (r *RecRunRepository) RecRunShelves(ctx context.Context, runID int64) ([]RecShelfRow, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT s.id, s.slug, s.kind, s.title, s.subtitle, i.media_type, i.tmdb_id, i.reason, i.meta
		FROM rec_shelves s
		LEFT JOIN rec_items i ON i.shelf_id = s.id
		WHERE s.run_id = $1
		ORDER BY s.position, i.position
	`, runID)
	if err != nil {
		return nil, fmt.Errorf("failed to read run %d's shelves: %w", runID, err)
	}
	defer rows.Close()

	var out []RecShelfRow
	lastID := int64(-1)
	for rows.Next() {
		var (
			shelfID          int64
			slug, kind, name string
			subtitle         sql.NullString
			mediaType        sql.NullString
			tmdbID           sql.NullInt64
			reason           sql.NullString
			meta             []byte
		)
		if err := rows.Scan(&shelfID, &slug, &kind, &name, &subtitle, &mediaType, &tmdbID, &reason, &meta); err != nil {
			return nil, fmt.Errorf("failed to scan a shelf: %w", err)
		}
		if shelfID != lastID {
			shelf := RecShelfRow{Slug: slug, Kind: kind, Title: name}
			if subtitle.Valid {
				shelf.Subtitle = &subtitle.String
			}
			out = append(out, shelf)
			lastID = shelfID
		}
		if mediaType.Valid {
			last := &out[len(out)-1]
			last.Items = append(last.Items, RecItemRow{
				TitleRef: TitleRef{MediaType: mediaType.String, TMDBID: int(tmdbID.Int64)},
				Reason:   reason.String, Meta: meta,
			})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read run %d's shelves: %w", runID, err)
	}
	return out, nil
}

// RecRunHeroPicks is a run's hero picks, in order.
func (r *RecRunRepository) RecRunHeroPicks(ctx context.Context, runID int64) ([]RecHeroPickRow, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT media_type, tmdb_id, kicker, reason, meta FROM rec_hero_picks WHERE run_id = $1 ORDER BY position
	`, runID)
	if err != nil {
		return nil, fmt.Errorf("failed to read run %d's hero picks: %w", runID, err)
	}
	defer rows.Close()
	var out []RecHeroPickRow
	for rows.Next() {
		var p RecHeroPickRow
		var meta []byte
		if err := rows.Scan(&p.MediaType, &p.TMDBID, &p.Kicker, &p.Reason, &meta); err != nil {
			return nil, fmt.Errorf("failed to scan a hero pick: %w", err)
		}
		p.Meta = meta
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read run %d's hero picks: %w", runID, err)
	}
	return out, nil
}

// RecommendationReason is the member's newest succeeded run's reason for
// a title: a shelf item's (its first shelf's), else a hero pick's. Nil
// without one, and after the member's negative feedback (dislike,
// not_interested, seen). One query.
func (r *RecRunRepository) RecommendationReason(ctx context.Context, memberID int64, ref TitleRef) (*string, error) {
	var reason sql.NullString
	err := r.db.QueryRowContext(ctx, `
		WITH run AS (
			SELECT id FROM rec_runs WHERE member_id = $3 AND status = 'succeeded'
			ORDER BY finished_at DESC, id DESC LIMIT 1
		)
		SELECT COALESCE(
			(SELECT i.reason FROM rec_items i JOIN rec_shelves s ON s.id = i.shelf_id
			 WHERE s.run_id = (SELECT id FROM run) AND i.media_type = $1 AND i.tmdb_id = $2
			 ORDER BY s.position, i.position LIMIT 1),
			(SELECT h.reason FROM rec_hero_picks h
			 WHERE h.run_id = (SELECT id FROM run) AND h.media_type = $1 AND h.tmdb_id = $2
			 ORDER BY h.position LIMIT 1))
		WHERE NOT EXISTS (
			SELECT 1 FROM rec_feedback f
			WHERE f.member_id = $3 AND f.media_type = $1 AND f.tmdb_id = $2
			  AND f.signal IN ('dislike', 'not_interested', 'seen'))
	`, ref.MediaType, ref.TMDBID, memberID).Scan(&reason)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && !reason.Valid) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read the reason for %s %d: %w", ref.MediaType, ref.TMDBID, err)
	}
	return &reason.String, nil
}

// RecRunCostMeans is, per model, the mean cost of its newest n succeeded
// runs, anyone's; a model without any has no entry.
func (r *RecRunRepository) RecRunCostMeans(ctx context.Context, n int) (map[string]float64, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT model, AVG(cost_usd)::float8 FROM (
			SELECT model, cost_usd, ROW_NUMBER() OVER (PARTITION BY model ORDER BY finished_at DESC, id DESC) AS n
			FROM rec_runs WHERE status = 'succeeded'
		) recent
		WHERE n <= $1 GROUP BY model
	`, n)
	if err != nil {
		return nil, fmt.Errorf("failed to average run costs: %w", err)
	}
	defer rows.Close()
	out := map[string]float64{}
	for rows.Next() {
		var model string
		var mean float64
		if err := rows.Scan(&model, &mean); err != nil {
			return nil, fmt.Errorf("failed to scan a run cost: %w", err)
		}
		out[model] = mean
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read run costs: %w", err)
	}
	return out, nil
}

// update runs a write that must touch the run, else ErrRecRunNotActive.
func (r *RecRunRepository) update(ctx context.Context, query string, id int64, args ...any) error {
	res, err := r.db.ExecContext(ctx, query, append([]any{id}, args...)...)
	if err != nil {
		return fmt.Errorf("failed to update run %d: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to update run %d: %w", id, err)
	}
	if n == 0 {
		return fmt.Errorf("run %d: %w", id, ErrRecRunNotActive)
	}
	return nil
}

// jsonOrEmpty is a JSON document for a NOT NULL jsonb column: {} when
// empty.
func jsonOrEmpty(raw json.RawMessage) string {
	if len(raw) == 0 {
		return "{}"
	}
	return string(raw)
}

// jsonOrNil is a JSON document for a nullable jsonb parameter: NULL when
// empty.
func jsonOrNil(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	return string(raw)
}
