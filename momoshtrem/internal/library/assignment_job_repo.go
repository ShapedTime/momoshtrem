package library

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// AssignmentJobRepository stores assignment jobs and their proposals. Every
// read and write takes a fixed number of queries, however many files a
// torrent has.
//
// A job's updated_at never goes backwards: every UPDATE writes
// GREATEST(clock_timestamp(), updated_at). NOW() is the transaction's
// start, and two transactions can take a job's lock in the other order
// from their BEGINs; the web client drops a poll older than the one it
// has, so an older stamp on a newer write would hide every later change.
type AssignmentJobRepository struct {
	db *DB
}

// NewAssignmentJobRepository creates a new assignment job repository.
func NewAssignmentJobRepository(db *DB) *AssignmentJobRepository {
	return &AssignmentJobRepository{db: db}
}

// queryer is what *DB and *sql.Tx share.
type queryer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// jobColumns selects a job (alias j) with its title (s or m) in the order
// scanJob reads them.
const jobColumns = `j.id, j.media_type, j.tmdb_id, j.show_id, j.movie_id,
	j.info_hash, j.magnet_uri, j.torrent_name, COALESCE(j.indexer, ''), COALESCE(j.release_group, ''),
	j.size_bytes, j.file_count, j.video_count, COALESCE(j.file_path, ''),
	j.status, j.dry_run, j.hint, j.suggested_hints, j.ai_files, COALESCE(j.error, ''), COALESCE(j.error_code, ''), COALESCE(j.ai_error, ''),
	j.created_at, j.updated_at, j.applied_at,
	COALESCE(s.title, m.title, ''), COALESCE(s.year, m.year, 0), COALESCE(s.poster_path, m.poster_path, ''),
	COALESCE(s.classifier_mode, 'auto'),
	CASE WHEN j.show_id IS NULL THEN 0 ELSE (SELECT count(*) FROM seasons sn WHERE sn.show_id = j.show_id) END,
	CASE WHEN j.show_id IS NULL THEN 0 ELSE (
		SELECT count(*) FROM episodes e JOIN seasons sn ON sn.id = e.season_id WHERE sn.show_id = j.show_id) END`

const jobFrom = `FROM assignment_jobs j
	LEFT JOIN shows s ON s.id = j.show_id
	LEFT JOIN movies m ON m.id = j.movie_id`

func scanJob(sc scanner) (*AssignmentJob, error) {
	j := &AssignmentJob{}
	var showID, movieID, size sql.NullInt64
	var files, videos, aiFiles sql.NullInt32
	var hint, hints []byte
	var applied sql.NullTime
	err := sc.Scan(
		&j.ID, &j.MediaType, &j.TMDBID, &showID, &movieID,
		&j.InfoHash, &j.MagnetURI, &j.TorrentName, &j.Indexer, &j.ReleaseGroup,
		&size, &files, &videos, &j.FilePath,
		&j.Status, &j.DryRun, &hint, &hints, &aiFiles, &j.Error, &j.ErrorCode, &j.AIError,
		&j.CreatedAt, &j.UpdatedAt, &applied,
		&j.Title, &j.Year, &j.PosterPath, &j.ClassifierMode, &j.SeasonCount, &j.EpisodeCount,
	)
	if err != nil {
		return nil, err
	}
	j.ShowID = nullInt64Ptr(showID)
	j.MovieID = nullInt64Ptr(movieID)
	j.SizeBytes = nullInt64Ptr(size)
	j.FileCount = nullIntPtr(files)
	j.VideoCount = nullIntPtr(videos)
	j.AIFiles = nullIntPtr(aiFiles)
	if applied.Valid {
		j.AppliedAt = &applied.Time
	}
	if len(hint) > 0 && string(hint) != "null" {
		j.Hint = &JobHint{}
		if err := json.Unmarshal(hint, j.Hint); err != nil {
			return nil, fmt.Errorf("job %d hint: %w", j.ID, err)
		}
	}
	if j.SuggestedHints, err = decodeStrings(hints); err != nil {
		return nil, fmt.Errorf("job %d suggested_hints: %w", j.ID, err)
	}
	return j, nil
}

// openStatuses and workingStatuses are the status sets as SQL parameters.
func openStatuses() []string    { return statusStrings(OpenJobStatuses) }
func workingStatuses() []string { return statusStrings(WorkingJobStatuses) }

func statusStrings(set []JobStatus) []string {
	out := make([]string, len(set))
	for i, s := range set {
		out[i] = string(s)
	}
	return out
}

// Create inserts a queued job and sets j.ID. When the title already has an
// open job for the same torrent (not a dry run), it sets j.ID to that job
// instead and returns created false.
func (r *AssignmentJobRepository) Create(ctx context.Context, j *AssignmentJob) (created bool, err error) {
	var hint any
	if j.Hint != nil {
		raw, err := json.Marshal(j.Hint)
		if err != nil {
			return false, fmt.Errorf("failed to encode hint: %w", err)
		}
		hint = string(raw)
	}
	// The partial unique index makes a concurrent second assign of the same
	// torrent find the first one's job.
	for attempt := 0; attempt < 3; attempt++ {
		err = r.db.QueryRowContext(ctx, `
			INSERT INTO assignment_jobs (media_type, tmdb_id, show_id, movie_id, info_hash, magnet_uri,
			                             torrent_name, indexer, file_path, dry_run, hint)
			VALUES ($1, $2, $3, $4, $5, $6, $7, NULLIF($8, ''), NULLIF($9, ''), $10, $11::jsonb)
			ON CONFLICT (media_type, tmdb_id, info_hash)
				WHERE NOT dry_run AND status IN ('queued', 'fetching_metadata', 'classifying', 'resolving', 'needs_review', 'ready')
				DO NOTHING
			RETURNING id
		`, j.MediaType, j.TMDBID, j.ShowID, j.MovieID, j.InfoHash, j.MagnetURI,
			j.TorrentName, j.Indexer, j.FilePath, j.DryRun, hint).Scan(&j.ID)
		if err == nil {
			return true, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return false, fmt.Errorf("failed to create assignment job: %w", err)
		}
		err = r.db.QueryRowContext(ctx, `
			SELECT id FROM assignment_jobs
			WHERE media_type = $1 AND tmdb_id = $2 AND info_hash = $3 AND NOT dry_run AND status = ANY($4)
			ORDER BY id DESC LIMIT 1
		`, j.MediaType, j.TMDBID, j.InfoHash, openStatuses()).Scan(&j.ID)
		if err == nil {
			return false, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return false, fmt.Errorf("failed to find the open assignment job: %w", err)
		}
		// The open job closed in between: try the insert again.
	}
	return false, fmt.Errorf("failed to create assignment job: the open job kept changing")
}

// Get returns a job with its title, or ErrJobNotFound.
func (r *AssignmentJobRepository) Get(ctx context.Context, id int64) (*AssignmentJob, error) {
	return getJob(ctx, r.db, id, "")
}

func getJob(ctx context.Context, q queryer, id int64, lock string) (*AssignmentJob, error) {
	j, err := scanJob(q.QueryRowContext(ctx, `SELECT `+jobColumns+` `+jobFrom+` WHERE j.id = $1 `+lock, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrJobNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get assignment job: %w", err)
	}
	return j, nil
}

// List returns the jobs in one of statuses, newest first, without dry runs.
func (r *AssignmentJobRepository) List(ctx context.Context, statuses []JobStatus, limit int) ([]*AssignmentJob, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+jobColumns+` `+jobFrom+`
		WHERE NOT j.dry_run AND j.status = ANY($1)
		ORDER BY j.id DESC LIMIT $2`, statusStrings(statuses), limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list assignment jobs: %w", err)
	}
	defer rows.Close()
	var jobs []*AssignmentJob
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan assignment job: %w", err)
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}

// proposalColumns selects a proposal (alias p) in the order scanProposal
// reads it, with the season of its first target.
const proposalColumns = `p.id, p.job_id, p.file_path, p.file_size, p.kind, p.review_group, COALESCE(p.extra_type, ''),
	to_jsonb(p.episode_ids), COALESCE(p.confidence, ''), p.source, p.reason,
	to_jsonb(p.suggested_episode_ids), COALESCE(p.suggested_confidence, ''), p.suggested_source, p.suggested_reason,
	p.decision, p.user_decided, p.updated, p.hinted, p.superseded_by, p.linked_proposal_id,
	(SELECT sn.season_number FROM episodes e JOIN seasons sn ON sn.id = e.season_id WHERE e.id = p.episode_ids[1])`

func scanProposal(sc scanner) (*AssignmentProposal, error) {
	p := &AssignmentProposal{}
	var ids, suggested []byte
	var superseded, linked sql.NullInt64
	var firstSeason sql.NullInt32
	err := sc.Scan(
		&p.ID, &p.JobID, &p.FilePath, &p.FileSize, &p.Kind, &p.Group, &p.ExtraType,
		&ids, &p.Confidence, &p.Source, &p.Reason,
		&suggested, &p.SuggestedConfidence, &p.SuggestedSource, &p.SuggestedReason,
		&p.Decision, &p.UserDecided, &p.Updated, &p.Hinted, &superseded, &linked, &firstSeason,
	)
	if err != nil {
		return nil, err
	}
	if p.EpisodeIDs, err = decodeIDs(ids); err != nil {
		return nil, err
	}
	if p.SuggestedEpisodeIDs, err = decodeIDs(suggested); err != nil {
		return nil, err
	}
	p.SupersededBy = nullInt64Ptr(superseded)
	p.LinkedProposalID = nullInt64Ptr(linked)
	p.FirstSeason = nullIntPtr(firstSeason)
	return p, nil
}

// Proposals returns the proposals of jobs by job, each in file order. One
// query.
func (r *AssignmentJobRepository) Proposals(ctx context.Context, jobIDs []int64) (map[int64][]*AssignmentProposal, error) {
	out := make(map[int64][]*AssignmentProposal, len(jobIDs))
	if len(jobIDs) == 0 {
		return out, nil
	}
	ps, err := loadProposals(ctx, r.db, `p.job_id = ANY($1)`, jobIDs)
	if err != nil {
		return nil, err
	}
	for _, p := range ps {
		out[p.JobID] = append(out[p.JobID], p)
	}
	return out, nil
}

func loadProposals(ctx context.Context, q queryer, where string, arg any) ([]*AssignmentProposal, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+proposalColumns+` FROM assignment_proposals p
		WHERE `+where+` ORDER BY p.job_id, p.id`, arg)
	if err != nil {
		return nil, fmt.Errorf("failed to load proposals: %w", err)
	}
	defer rows.Close()
	var out []*AssignmentProposal
	for rows.Next() {
		p, err := scanProposal(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan proposal: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// EpisodeRefs returns the episodes among ids that belong to the show, each
// with its active assignment. One query.
func (r *AssignmentJobRepository) EpisodeRefs(ctx context.Context, showID int64, ids []int64) (map[int64]*EpisodeRef, error) {
	return episodeRefs(ctx, r.db, showID, ids)
}

func episodeRefs(ctx context.Context, q queryer, showID int64, ids []int64) (map[int64]*EpisodeRef, error) {
	out := make(map[int64]*EpisodeRef, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := q.QueryContext(ctx, `
		SELECT e.id, sn.show_id, sn.season_number, COALESCE(e.episode_number, 0), e.absolute_number, COALESCE(e.name, ''),
		       `+assignmentColumns+`
		FROM episodes e
		JOIN seasons sn ON sn.id = e.season_id
		`+activeAssignmentLateral("episode", "e.id")+`
		WHERE e.id = ANY($1::bigint[]) AND sn.show_id = $2
	`, ids, showID)
	if err != nil {
		return nil, fmt.Errorf("failed to load episodes: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		ref := &EpisodeRef{}
		var absolute sql.NullInt32
		dest, finish := assignmentDest(ItemTypeEpisode)
		dest = append([]any{&ref.ID, &ref.ShowID, &ref.Season, &ref.Number, &absolute, &ref.Name}, dest...)
		if err := rows.Scan(dest...); err != nil {
			return nil, fmt.Errorf("failed to scan episode: %w", err)
		}
		ref.AbsoluteNumber = nullIntPtr(absolute)
		ref.Active = finish(ref.ID)
		out[ref.ID] = ref
	}
	return out, rows.Err()
}

// Claim takes the oldest queued job for the worker: it moves to
// fetching_metadata. It returns ErrJobNotFound when none is queued. Two
// workers never claim the same job (FOR UPDATE SKIP LOCKED).
func (r *AssignmentJobRepository) Claim(ctx context.Context) (*AssignmentJob, error) {
	var id int64
	err := r.db.QueryRowContext(ctx, `
		UPDATE assignment_jobs SET status = 'fetching_metadata', updated_at = GREATEST(clock_timestamp(), updated_at)
		WHERE id = (
			SELECT id FROM assignment_jobs WHERE status = 'queued'
			ORDER BY id FOR UPDATE SKIP LOCKED LIMIT 1
		)
		RETURNING id
	`).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrJobNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to claim assignment job: %w", err)
	}
	return r.Get(ctx, id)
}

// Requeue puts jobs a stopped worker left in fetching_metadata or
// classifying back in the queue, and returns how many.
func (r *AssignmentJobRepository) Requeue(ctx context.Context) (int64, error) {
	res, err := r.db.ExecContext(ctx, `
		UPDATE assignment_jobs SET status = 'queued', updated_at = GREATEST(clock_timestamp(), updated_at)
		WHERE status IN ('fetching_metadata', 'classifying')
	`)
	if err != nil {
		return 0, fmt.Errorf("failed to requeue assignment jobs: %w", err)
	}
	return res.RowsAffected()
}

// IDsWithStatus lists the jobs in status, oldest first.
func (r *AssignmentJobRepository) IDsWithStatus(ctx context.Context, status JobStatus) ([]int64, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id FROM assignment_jobs WHERE status = $1 ORDER BY id`, status)
	if err != nil {
		return nil, fmt.Errorf("failed to list assignment jobs: %w", err)
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("failed to scan assignment job id: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// FileList is what the torrent's metadata says.
type FileList struct {
	Name       string
	SizeBytes  int64
	FileCount  int
	VideoCount int
}

// SetFileList stores the torrent's file list and moves the job from
// fetching_metadata to classifying. It reports false when the job isn't in
// fetching_metadata any more (discarded, or gone).
func (r *AssignmentJobRepository) SetFileList(ctx context.Context, id int64, f FileList) (bool, error) {
	res, err := r.db.ExecContext(ctx, `
		UPDATE assignment_jobs
		SET torrent_name = $2, size_bytes = $3, file_count = $4, video_count = $5,
		    status = 'classifying', updated_at = GREATEST(clock_timestamp(), updated_at)
		WHERE id = $1 AND status = 'fetching_metadata'
	`, id, f.Name, f.SizeBytes, f.FileCount, f.VideoCount)
	if err != nil {
		return false, fmt.Errorf("failed to store the file list: %w", err)
	}
	return affected(res)
}

// Fail ends a working job with an error: code tells code why, message the
// user. It reports false when the job isn't working any more.
func (r *AssignmentJobRepository) Fail(ctx context.Context, id int64, code, message string) (bool, error) {
	res, err := r.db.ExecContext(ctx, `
		UPDATE assignment_jobs SET status = 'failed', error_code = $2, error = $3, updated_at = GREATEST(clock_timestamp(), updated_at)
		WHERE id = $1 AND status = ANY($4)
	`, id, code, message, workingStatuses())
	if err != nil {
		return false, fmt.Errorf("failed to fail assignment job: %w", err)
	}
	return affected(res)
}

// Cancel moves an open job to cancelled. It reports false when the job
// isn't open.
func (r *AssignmentJobRepository) Cancel(ctx context.Context, id int64) (bool, error) {
	res, err := r.db.ExecContext(ctx, `
		UPDATE assignment_jobs SET status = 'cancelled', updated_at = GREATEST(clock_timestamp(), updated_at)
		WHERE id = $1 AND status = ANY($2)
	`, id, openStatuses())
	if err != nil {
		return false, fmt.Errorf("failed to cancel assignment job: %w", err)
	}
	return affected(res)
}

// TorrentInUse reports whether an active assignment or an open job other
// than exceptJob uses the torrent.
func (r *AssignmentJobRepository) TorrentInUse(ctx context.Context, infoHash string, exceptJob int64) (bool, error) {
	var used bool
	err := r.db.QueryRowContext(ctx, `
		SELECT EXISTS (SELECT 1 FROM torrent_assignments WHERE info_hash = $1 AND is_active)
		    OR EXISTS (SELECT 1 FROM assignment_jobs WHERE info_hash = $1 AND id <> $2 AND status = ANY($3))
	`, infoHash, exceptJob, openStatuses()).Scan(&used)
	if err != nil {
		return false, fmt.Errorf("failed to check torrent use: %w", err)
	}
	return used, nil
}

// DeleteDryRuns removes dry-run jobs created before olderThan ago (#6
// Inspect keeps them for a day).
func (r *AssignmentJobRepository) DeleteDryRuns(ctx context.Context, olderThan time.Duration) (int64, error) {
	res, err := r.db.ExecContext(ctx, `
		DELETE FROM assignment_jobs WHERE dry_run AND created_at < NOW() - make_interval(secs => $1)
	`, olderThan.Seconds())
	if err != nil {
		return 0, fmt.Errorf("failed to delete dry runs: %w", err)
	}
	return res.RowsAffected()
}

// JobTx is a job locked for a change (FOR UPDATE), with its proposals.
// Changes made through it commit together.
type JobTx struct {
	ctx       context.Context
	tx        *sql.Tx
	Job       *AssignmentJob
	Proposals []*AssignmentProposal
}

// Locked runs fn on the job, locked, in a transaction that commits when fn
// returns nil. It returns ErrJobNotFound for an unknown job. PATCH, apply,
// the worker's writes and the AI's take the same lock, so they serialize.
func (r *AssignmentJobRepository) Locked(ctx context.Context, id int64, fn func(*JobTx) error) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	job, err := getJob(ctx, tx, id, "FOR UPDATE OF j")
	if err != nil {
		return err
	}
	t := &JobTx{ctx: ctx, tx: tx, Job: job}
	if t.Proposals, err = loadProposals(ctx, tx, `p.job_id = $1`, id); err != nil {
		return err
	}
	if err := fn(t); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit: %w", err)
	}
	return nil
}

// SaveJob writes the job's mutable fields: status, hint, release group,
// video count, suggested hints, AI files, errors and applied_at.
func (t *JobTx) SaveJob() error {
	var hint any
	if t.Job.Hint != nil {
		raw, err := json.Marshal(t.Job.Hint)
		if err != nil {
			return fmt.Errorf("failed to encode hint: %w", err)
		}
		hint = string(raw)
	}
	hints, err := encodeStrings(t.Job.SuggestedHints)
	if err != nil {
		return err
	}
	var applied any
	if t.Job.AppliedAt != nil {
		applied = *t.Job.AppliedAt
	}
	err = t.tx.QueryRowContext(t.ctx, `
		UPDATE assignment_jobs
		SET status = $2, hint = $3::jsonb, release_group = NULLIF($4, ''), video_count = $5,
		    suggested_hints = $6::jsonb, ai_files = $7, error = NULLIF($8, ''), ai_error = NULLIF($9, ''),
		    applied_at = $10, updated_at = GREATEST(clock_timestamp(), updated_at)
		WHERE id = $1
		RETURNING updated_at
	`, t.Job.ID, t.Job.Status, hint, t.Job.ReleaseGroup, t.Job.VideoCount,
		hints, t.Job.AIFiles, t.Job.Error, t.Job.AIError, applied).Scan(&t.Job.UpdatedAt)
	if err != nil {
		return fmt.Errorf("failed to save assignment job: %w", err)
	}
	return nil
}

// MarkApplied moves the job to applied, guarded by its status (UPDATE …
// WHERE status IN from). It reports false when the job isn't in one of
// from.
func (t *JobTx) MarkApplied(from []JobStatus) (bool, error) {
	var applied, updated time.Time
	err := t.tx.QueryRowContext(t.ctx, `
		UPDATE assignment_jobs SET status = 'applied', applied_at = clock_timestamp(),
		       updated_at = GREATEST(clock_timestamp(), updated_at)
		WHERE id = $1 AND status = ANY($2)
		RETURNING applied_at, updated_at
	`, t.Job.ID, statusStrings(from)).Scan(&applied, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to mark the job applied: %w", err)
	}
	t.Job.Status, t.Job.AppliedAt, t.Job.UpdatedAt = JobApplied, &applied, updated
	return true, nil
}

// proposalRecord is a proposal as jsonb_to_recordset reads it.
type proposalRecord struct {
	ID                  int64   `json:"id,omitempty"`
	FilePath            string  `json:"file_path"`
	FileSize            int64   `json:"file_size"`
	Kind                string  `json:"kind"`
	Group               string  `json:"review_group"`
	ExtraType           string  `json:"extra_type"`
	EpisodeIDs          []int64 `json:"episode_ids"`
	Confidence          string  `json:"confidence"`
	Source              string  `json:"source"`
	Reason              string  `json:"reason"`
	SuggestedEpisodeIDs []int64 `json:"suggested_episode_ids"`
	SuggestedConfidence string  `json:"suggested_confidence"`
	SuggestedSource     string  `json:"suggested_source"`
	SuggestedReason     string  `json:"suggested_reason"`
	Decision            string  `json:"decision"`
	UserDecided         bool    `json:"user_decided"`
	Updated             bool    `json:"updated"`
	Hinted              bool    `json:"hinted"`
	LinkedPath          string  `json:"linked_path,omitempty"`
	SupersededByPath    string  `json:"superseded_by_path,omitempty"`
	Ord                 int     `json:"ord"` // file order
}

func idsOrEmpty(ids []int64) []int64 {
	if ids == nil {
		return []int64{}
	}
	return ids
}

// ReplaceProposals stores the job's proposals from drafts, in order,
// replacing any it had (a job classified again after a restart), and
// reloads t.Proposals. Each starts on its own suggestion. Four queries.
func (t *JobTx) ReplaceProposals(drafts []ProposalDraft) error {
	if _, err := t.tx.ExecContext(t.ctx, `DELETE FROM assignment_proposals WHERE job_id = $1`, t.Job.ID); err != nil {
		return fmt.Errorf("failed to delete proposals: %w", err)
	}
	records := make([]proposalRecord, len(drafts))
	links := false
	for i, d := range drafts {
		records[i] = proposalRecord{
			FilePath: d.FilePath, FileSize: d.FileSize, Kind: string(d.Kind), Group: string(d.Group),
			ExtraType: d.ExtraType, EpisodeIDs: idsOrEmpty(d.EpisodeIDs), Confidence: d.Confidence,
			Source: d.Source, Reason: d.Reason, Decision: string(d.Decision),
			LinkedPath: d.LinkedPath, SupersededByPath: d.SupersededByPath, Ord: i,
		}
		links = links || d.LinkedPath != "" || d.SupersededByPath != ""
	}
	payload, err := json.Marshal(records)
	if err != nil {
		return fmt.Errorf("failed to encode proposals: %w", err)
	}
	// Rows are inserted in file order, so ids follow it.
	if _, err := t.tx.ExecContext(t.ctx, `
		INSERT INTO assignment_proposals (job_id, file_path, file_size, kind, review_group, extra_type,
		    episode_ids, suggested_episode_ids, suggested_confidence, suggested_source, suggested_reason,
		    confidence, source, reason, decision)
		SELECT $1, d.file_path, d.file_size, d.kind, d.review_group, NULLIF(d.extra_type, ''),
		       d.episode_ids, d.episode_ids, NULLIF(d.confidence, ''), d.source, d.reason,
		       NULLIF(d.confidence, ''), d.source, d.reason, d.decision
		FROM jsonb_to_recordset($2::jsonb) AS d(file_path text, file_size bigint, kind text,
		     review_group text, extra_type text, episode_ids bigint[], confidence text, source text,
		     reason text, decision text, ord int)
		ORDER BY d.ord
	`, t.Job.ID, string(payload)); err != nil {
		return fmt.Errorf("failed to insert proposals: %w", err)
	}
	if links {
		if _, err := t.tx.ExecContext(t.ctx, `
			UPDATE assignment_proposals p
			SET linked_proposal_id = v.id, superseded_by = s.id
			FROM jsonb_to_recordset($2::jsonb) AS d(file_path text, linked_path text, superseded_by_path text)
			LEFT JOIN assignment_proposals v ON v.job_id = $1 AND v.file_path = d.linked_path
			LEFT JOIN assignment_proposals s ON s.job_id = $1 AND s.file_path = d.superseded_by_path
			WHERE p.job_id = $1 AND p.file_path = d.file_path
			  AND (d.linked_path IS NOT NULL OR d.superseded_by_path IS NOT NULL)
		`, t.Job.ID, string(payload)); err != nil {
			return fmt.Errorf("failed to link proposals: %w", err)
		}
	}
	t.Proposals, err = loadProposals(t.ctx, t.tx, `p.job_id = $1`, t.Job.ID)
	return err
}

// UpdateProposals writes the mutable fields of ps (targets, confidence,
// source, reason, the suggestion, decision and flags; kind and group too,
// which only the first AI run may change) and reloads t.Proposals. Two
// queries.
func (t *JobTx) UpdateProposals(ps []*AssignmentProposal) error {
	if len(ps) == 0 {
		return nil
	}
	records := make([]proposalRecord, len(ps))
	for i, p := range ps {
		records[i] = proposalRecord{
			ID: p.ID, Kind: string(p.Kind), Group: string(p.Group), ExtraType: p.ExtraType,
			EpisodeIDs: idsOrEmpty(p.EpisodeIDs), Confidence: p.Confidence,
			Source: p.Source, Reason: p.Reason, SuggestedEpisodeIDs: idsOrEmpty(p.SuggestedEpisodeIDs),
			SuggestedConfidence: p.SuggestedConfidence, SuggestedSource: p.SuggestedSource,
			SuggestedReason: p.SuggestedReason, Decision: string(p.Decision),
			UserDecided: p.UserDecided, Updated: p.Updated, Hinted: p.Hinted,
		}
	}
	payload, err := json.Marshal(records)
	if err != nil {
		return fmt.Errorf("failed to encode proposals: %w", err)
	}
	_, err = t.tx.ExecContext(t.ctx, `
		UPDATE assignment_proposals p
		SET kind = d.kind, review_group = d.review_group, extra_type = NULLIF(d.extra_type, ''),
		    episode_ids = d.episode_ids, confidence = NULLIF(d.confidence, ''),
		    source = d.source, reason = d.reason, suggested_episode_ids = d.suggested_episode_ids,
		    suggested_confidence = NULLIF(d.suggested_confidence, ''), suggested_source = d.suggested_source,
		    suggested_reason = d.suggested_reason, decision = d.decision, user_decided = d.user_decided,
		    updated = d.updated, hinted = d.hinted
		FROM jsonb_to_recordset($2::jsonb) AS d(id bigint, kind text, review_group text, extra_type text,
		     episode_ids bigint[], confidence text, source text, reason text, suggested_episode_ids bigint[],
		     suggested_confidence text, suggested_source text, suggested_reason text, decision text,
		     user_decided boolean, updated boolean, hinted boolean)
		WHERE p.job_id = $1 AND p.id = d.id
	`, t.Job.ID, string(payload))
	if err != nil {
		return fmt.Errorf("failed to update proposals: %w", err)
	}
	t.Proposals, err = loadProposals(t.ctx, t.tx, `p.job_id = $1`, t.Job.ID)
	return err
}

// EpisodeRefs is AssignmentJobRepository.EpisodeRefs inside the
// transaction: apply re-validates its targets with it.
func (t *JobTx) EpisodeRefs(showID int64, ids []int64) (map[int64]*EpisodeRef, error) {
	return episodeRefs(t.ctx, t.tx, showID, ids)
}

// AssignmentWrite is what Apply writes for one torrent: the title's rows,
// one per item (episode or movie).
type AssignmentWrite struct {
	ItemType  ItemType
	InfoHash  string
	MagnetURI string
	// The title the write is authoritative for: the torrent's active rows
	// on its items (the show's episodes, or the movie) that Rows doesn't
	// list are deactivated.
	ShowID  int64
	MovieID int64
	// Rows: ItemID, FilePath, FileSize, Resolution, Source, Confidence and
	// MatchSource. One per item.
	Rows []TorrentAssignment
	// Partial writes Rows' items only: the torrent's other rows on the
	// title stay as they are (a manual override of one episode). Apply's
	// writes aren't partial.
	Partial bool
}

// WriteAssignments applies w: the torrent's rows the job no longer maps
// are deactivated (unless w is partial), each item's other active rows
// too, and every row is upserted active (a file the item had before is
// reactivated). An item whose file changes loses its torrent subtitles:
// they came with the old file, and the caller records the new file's
// after the commit. It locks the title first, so writes to one title
// never interleave. Six queries (five for a partial write).
func (t *JobTx) WriteAssignments(w AssignmentWrite) error {
	return writeAssignments(t.ctx, t.tx, w)
}

// lockTitle serializes writes to the title w writes to until tx ends, and
// checks the title still exists. Under READ COMMITTED two writers can't
// see each other's uncommitted rows, so two applies mapping one episode
// to different torrents would otherwise both leave a row active.
//
// It's an advisory lock keyed on the title ("assign:show:42"), not a lock
// on the show's row: Apply holds the job's row, and deleting the show
// locks the show's row before cascading to the job's, so a row lock here
// would invert that order and deadlock. A DELETE never takes the advisory
// lock; it just waits for the job's row until the apply commits.
func lockTitle(ctx context.Context, tx *sql.Tx, w AssignmentWrite) error {
	var table, key string
	var id int64
	var missing error
	switch w.ItemType {
	case ItemTypeEpisode:
		table, key, id, missing = "shows", "show", w.ShowID, ErrShowNotFound
	case ItemTypeMovie:
		table, key, id, missing = "movies", "movie", w.MovieID, ErrMovieNotFound
	default:
		return fmt.Errorf("unknown item type %q", w.ItemType)
	}
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`,
		fmt.Sprintf("assign:%s:%d", key, id)); err != nil {
		return fmt.Errorf("failed to lock the title: %w", err)
	}
	var exists bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM `+table+` WHERE id = $1)`, id).Scan(&exists); err != nil {
		return fmt.Errorf("failed to check the title: %w", err)
	}
	if !exists {
		return missing
	}
	return nil
}

// writeAssignments is the assignment writer: Apply and the manual
// override (AssignmentRepository.WriteOverride) write through it.
func writeAssignments(ctx context.Context, tx *sql.Tx, w AssignmentWrite) error {
	if err := lockTitle(ctx, tx, w); err != nil {
		return err
	}
	n := len(w.Rows)
	items := make([]int64, n)
	paths := make([]string, n)
	sizes := make([]int64, n)
	resolutions := make([]string, n)
	sources := make([]string, n)
	confidences := make([]string, n)
	matchSources := make([]string, n)
	seen := make(map[int64]bool, n)
	for i, row := range w.Rows {
		if seen[row.ItemID] {
			return fmt.Errorf("item %d is written twice", row.ItemID)
		}
		seen[row.ItemID] = true
		items[i], paths[i], sizes[i] = row.ItemID, row.FilePath, row.FileSize
		resolutions[i], sources[i] = row.Resolution, row.Source
		confidences[i], matchSources[i] = row.Confidence, row.MatchSource
	}

	var err error
	switch {
	case w.Partial:
	case w.ItemType == ItemTypeEpisode:
		_, err = tx.ExecContext(ctx, `
			UPDATE torrent_assignments t SET is_active = FALSE
			FROM episodes e JOIN seasons sn ON sn.id = e.season_id
			WHERE t.item_type = 'episode' AND t.item_id = e.id AND sn.show_id = $1
			  AND t.info_hash = $2 AND t.is_active
			  AND NOT EXISTS (SELECT 1 FROM unnest($3::bigint[], $4::text[]) AS m(item_id, file_path)
			                  WHERE m.item_id = t.item_id AND m.file_path = t.file_path)
		`, w.ShowID, w.InfoHash, items, paths)
	case w.ItemType == ItemTypeMovie:
		_, err = tx.ExecContext(ctx, `
			UPDATE torrent_assignments t SET is_active = FALSE
			WHERE t.item_type = 'movie' AND t.item_id = $1 AND t.info_hash = $2 AND t.is_active
			  AND NOT EXISTS (SELECT 1 FROM unnest($3::bigint[], $4::text[]) AS m(item_id, file_path)
			                  WHERE m.item_id = t.item_id AND m.file_path = t.file_path)
		`, w.MovieID, w.InfoHash, items, paths)
	default:
		return fmt.Errorf("unknown item type %q", w.ItemType)
	}
	if err != nil {
		return fmt.Errorf("failed to deactivate the torrent's earlier rows: %w", err)
	}
	if n == 0 {
		return nil
	}

	// Torrent subtitles belong to the file they came with: an item moving
	// to another file (even of the same torrent) drops them. Downloaded
	// subtitles stay.
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM subtitles s
		USING unnest($1::bigint[], $2::text[]) AS m(item_id, file_path)
		WHERE s.item_type = $3 AND s.item_id = m.item_id AND s.source = 'torrent'
		  AND NOT EXISTS (SELECT 1 FROM torrent_assignments t
		                  WHERE t.item_type = $3 AND t.item_id = m.item_id AND t.is_active
		                    AND t.info_hash = $4 AND t.file_path = m.file_path)
	`, items, paths, w.ItemType, w.InfoHash); err != nil {
		return fmt.Errorf("failed to drop the old files' subtitles: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE torrent_assignments t SET is_active = FALSE
		FROM unnest($1::bigint[], $2::text[]) AS m(item_id, file_path)
		WHERE t.item_type = $3 AND t.item_id = m.item_id AND t.is_active
		  AND NOT (t.info_hash = $4 AND t.file_path = m.file_path)
	`, items, paths, w.ItemType, w.InfoHash); err != nil {
		return fmt.Errorf("failed to deactivate the items' other rows: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO torrent_assignments (item_type, item_id, info_hash, magnet_uri, file_path, file_size,
		                                 resolution, source, confidence, match_source, is_active)
		SELECT $1, m.item_id, $2, $3, m.file_path, m.file_size, NULLIF(m.resolution, ''), NULLIF(m.source, ''),
		       NULLIF(m.confidence, ''), NULLIF(m.match_source, ''), TRUE
		FROM unnest($4::bigint[], $5::text[], $6::bigint[], $7::text[], $8::text[], $9::text[], $10::text[])
		     AS m(item_id, file_path, file_size, resolution, source, confidence, match_source)
		`+assignmentUpsert,
		w.ItemType, w.InfoHash, w.MagnetURI, items, paths, sizes, resolutions, sources, confidences, matchSources,
	); err != nil {
		return fmt.Errorf("failed to write assignments: %w", err)
	}
	return nil
}

func affected(res sql.Result) (bool, error) {
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("failed to read rows affected: %w", err)
	}
	return n > 0, nil
}

func nullInt64Ptr(v sql.NullInt64) *int64 {
	if !v.Valid {
		return nil
	}
	n := v.Int64
	return &n
}

func nullIntPtr(v sql.NullInt32) *int {
	if !v.Valid {
		return nil
	}
	n := int(v.Int32)
	return &n
}
