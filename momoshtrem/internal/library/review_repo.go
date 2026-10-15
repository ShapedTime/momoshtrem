package library

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// What the mapping review (#5) reads and writes outside a job: the review
// state the title pages and /library show, the episode picker's list, a
// torrent's file list and mappings, and manual overrides.

// reviewCTEs are the CTEs behind PLAN §6 "Review rules", for the jobs that
// match scope (a condition on assignment_jobs j):
//   - review_jobs (id, show_id, movie_id): open, non-dry-run jobs past
//     classification (resolving, needs_review or ready);
//   - review_files (id, job_id, episode_ids): their listed proposals that
//     need a look, as service.countsOf counts needs_review: pending and
//     mapping (targets, or a movie candidate), or in a conflict (sharing
//     an episode with another non-ignored proposal of the job; movie
//     candidates and subtitles never conflict).
func reviewCTEs(scope string) string {
	return `review_jobs AS (
		SELECT j.id, j.show_id, j.movie_id FROM assignment_jobs j
		WHERE NOT j.dry_run AND j.status IN ('resolving', 'needs_review', 'ready') AND (` + scope + `)
	),
	review_props AS (
		SELECT p.id, p.job_id, p.kind, p.decision, p.episode_ids
		FROM assignment_proposals p JOIN review_jobs rj ON rj.id = p.job_id
		WHERE p.linked_proposal_id IS NULL
	),
	review_claims AS (
		SELECT rp.job_id, c.episode_id
		FROM review_props rp CROSS JOIN LATERAL unnest(rp.episode_ids) AS c(episode_id)
		WHERE rp.decision <> 'ignored' AND rp.kind NOT IN ('movie', 'subtitle')
		GROUP BY rp.job_id, c.episode_id
		HAVING count(DISTINCT rp.id) > 1
	),
	review_files AS (
		SELECT rp.id, rp.job_id, rp.episode_ids FROM review_props rp
		WHERE (rp.decision = 'pending' AND (rp.kind = 'movie' OR cardinality(rp.episode_ids) > 0))
		   OR (rp.decision <> 'ignored' AND rp.kind NOT IN ('movie', 'subtitle') AND EXISTS (
		        SELECT 1 FROM review_claims rc WHERE rc.job_id = rp.job_id AND rc.episode_id = ANY(rp.episode_ids)))
	)`
}

// ReviewState is a library title's mapping review (PLAN §6 "Review
// rules"), from its open, non-dry-run jobs past classification.
type ReviewState struct {
	// JobID is the newest such job; nil without one. It stays set while a
	// ready job waits for Apply.
	JobID *int64
	// NeedsReview counts those jobs' files that need a look.
	NeedsReview int
	// EpisodeIDs are the episodes those files target (shows), ascending.
	EpisodeIDs []int64
}

// ReviewState returns the review state of a library title: mediaType "tv"
// (a show) or "movie". One query.
func (r *CatalogRepository) ReviewState(ctx context.Context, mediaType string, libraryID int64) (*ReviewState, error) {
	var scope string
	switch mediaType {
	case string(JobTV):
		scope = `j.show_id = $1`
	case string(JobMovie):
		scope = `j.movie_id = $1`
	default:
		return nil, fmt.Errorf("unknown media type %q", mediaType)
	}
	var jobID sql.NullInt64
	var ids []byte
	state := &ReviewState{}
	err := r.db.QueryRowContext(ctx, `
		WITH `+reviewCTEs(scope)+`
		SELECT (SELECT max(id) FROM review_jobs),
		       (SELECT count(*) FROM review_files),
		       (SELECT to_jsonb(array_agg(DISTINCT e.id ORDER BY e.id))
		        FROM review_files rf CROSS JOIN LATERAL unnest(rf.episode_ids) AS e(id))
	`, libraryID).Scan(&jobID, &state.NeedsReview, &ids)
	if err != nil {
		return nil, fmt.Errorf("failed to read the review state: %w", err)
	}
	state.JobID = nullInt64Ptr(jobID)
	if state.EpisodeIDs, err = decodeIDs(ids); err != nil {
		return nil, err
	}
	return state, nil
}

// PickerSeason is a season of a show's episode list (the episode picker).
type PickerSeason struct {
	Number   int
	Name     string // "" when TMDB has none
	Episodes []PickerEpisode
}

// PickerEpisode is one episode of the picker's list.
type PickerEpisode struct {
	ID             int64
	Number         int
	AbsoluteNumber *int
	Name           string
	AirDate        string // "" when unknown
	HasFile        bool   // it has an active assignment
}

// EpisodeList returns every season of the show (Specials included, as
// stored, whatever include_specials says) in number order, each with its
// episodes in order. It returns ErrShowNotFound for an unknown show. One
// query.
func (r *ShowRepository) EpisodeList(ctx context.Context, showID int64) ([]PickerSeason, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT sn.id, sn.season_number, COALESCE(sn.name, ''),
		       e.id, COALESCE(e.episode_number, 0), e.absolute_number, COALESCE(e.name, ''), COALESCE(e.air_date, ''),
		       EXISTS (SELECT 1 FROM torrent_assignments t
		               WHERE t.item_type = 'episode' AND t.item_id = e.id AND t.is_active)
		FROM shows s
		LEFT JOIN seasons sn ON sn.show_id = s.id
		LEFT JOIN episodes e ON e.season_id = sn.id
		WHERE s.id = $1
		ORDER BY sn.season_number, e.episode_number, e.id
	`, showID)
	if err != nil {
		return nil, fmt.Errorf("failed to list the show's episodes: %w", err)
	}
	defer rows.Close()

	found := false
	seasons := []PickerSeason{}
	var lastSeason int64
	for rows.Next() {
		found = true
		var seasonID, epID sql.NullInt64
		var number sql.NullInt32
		var seasonName string
		var ep PickerEpisode
		var absolute sql.NullInt32
		if err := rows.Scan(&seasonID, &number, &seasonName,
			&epID, &ep.Number, &absolute, &ep.Name, &ep.AirDate, &ep.HasFile); err != nil {
			return nil, fmt.Errorf("failed to scan an episode: %w", err)
		}
		if !seasonID.Valid {
			continue // a show without seasons
		}
		if len(seasons) == 0 || lastSeason != seasonID.Int64 {
			seasons = append(seasons, PickerSeason{Number: int(number.Int32), Name: seasonName, Episodes: []PickerEpisode{}})
			lastSeason = seasonID.Int64
		}
		if !epID.Valid {
			continue // a season without episodes
		}
		ep.ID = epID.Int64
		ep.AbsoluteNumber = nullIntPtr(absolute)
		last := &seasons[len(seasons)-1]
		last.Episodes = append(last.Episodes, ep)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read the show's episodes: %w", err)
	}
	if !found {
		return nil, ErrShowNotFound
	}
	return seasons, nil
}

// EpisodeShowID returns the show an episode belongs to, or
// ErrEpisodeNotFound. One query.
func (r *ShowRepository) EpisodeShowID(ctx context.Context, episodeID int64) (int64, error) {
	var showID int64
	err := r.db.QueryRowContext(ctx, `
		SELECT sn.show_id FROM episodes e JOIN seasons sn ON sn.id = e.season_id WHERE e.id = $1
	`, episodeID).Scan(&showID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrEpisodeNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("failed to get the episode: %w", err)
	}
	return showID, nil
}

// TorrentFile is one file of a torrent.
type TorrentFile struct {
	Path string
	Size int64
}

// StoredTorrent is a torrent as a job stored it: its file list (every
// file, as proposals), the subtitles the classifier linked to each video,
// and how to fetch it again.
type StoredTorrent struct {
	Name      string
	MagnetURI string
	Files     []TorrentFile
	// Subtitles are the subtitle files that belong to a video, by the
	// video's path.
	Subtitles map[string][]TorrentFile
}

// StoredTorrent returns the file list of the newest job for the torrent
// that has proposals (any status, dry runs too: the files are the same),
// or nil when no job has one. One query.
func (r *AssignmentJobRepository) StoredTorrent(ctx context.Context, infoHash string) (*StoredTorrent, error) {
	rows, err := r.db.QueryContext(ctx, `
		WITH job AS (
			SELECT j.id, j.torrent_name, j.magnet_uri FROM assignment_jobs j
			WHERE j.info_hash = $1 AND EXISTS (SELECT 1 FROM assignment_proposals p WHERE p.job_id = j.id)
			ORDER BY j.id DESC LIMIT 1
		)
		SELECT job.torrent_name, job.magnet_uri, p.file_path, p.file_size, COALESCE(v.file_path, '')
		FROM job JOIN assignment_proposals p ON p.job_id = job.id
		LEFT JOIN assignment_proposals v ON v.id = p.linked_proposal_id
		ORDER BY p.id
	`, infoHash)
	if err != nil {
		return nil, fmt.Errorf("failed to load the torrent's files: %w", err)
	}
	defer rows.Close()
	var out *StoredTorrent
	for rows.Next() {
		var name, magnet, video string
		var f TorrentFile
		if err := rows.Scan(&name, &magnet, &f.Path, &f.Size, &video); err != nil {
			return nil, fmt.Errorf("failed to scan a torrent file: %w", err)
		}
		if out == nil {
			out = &StoredTorrent{Name: name, MagnetURI: magnet, Subtitles: map[string][]TorrentFile{}}
		}
		out.Files = append(out.Files, f)
		if video != "" {
			out.Subtitles[video] = append(out.Subtitles[video], f)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read the torrent's files: %w", err)
	}
	return out, nil
}

// TorrentMagnet returns a magnet for the torrent: an assignment's (active
// first, newest first), else a job's (newest first). "" when neither knows
// the torrent. One query.
func (r *AssignmentJobRepository) TorrentMagnet(ctx context.Context, infoHash string) (string, error) {
	var magnet string
	err := r.db.QueryRowContext(ctx, `
		SELECT magnet_uri FROM (
			SELECT t.magnet_uri, 0 AS source, COALESCE(t.is_active, FALSE) AS active, t.created_at, t.id
			FROM torrent_assignments t WHERE t.info_hash = $1
			UNION ALL
			SELECT j.magnet_uri, 1, FALSE, j.created_at, j.id
			FROM assignment_jobs j WHERE j.info_hash = $1
		) known
		ORDER BY source, active DESC, created_at DESC NULLS LAST, id DESC
		LIMIT 1
	`, infoHash).Scan(&magnet)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("failed to find the torrent's magnet: %w", err)
	}
	return magnet, nil
}

// FileMapping is an active assignment of a torrent's file: to a movie, or
// to an episode (with its season, number and name).
type FileMapping struct {
	FilePath string
	MovieID  *int64      // a movie's file
	Episode  *EpisodeRef // an episode's file (Active is nil)
}

// ActiveMappings returns the torrent's active assignments, by file path
// and then episode order. Rows of episodes or movies that are gone are
// left out. One query.
func (r *AssignmentJobRepository) ActiveMappings(ctx context.Context, infoHash string) ([]FileMapping, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT t.file_path, t.item_type, t.item_id, sn.show_id, sn.season_number,
		       e.episode_number, e.absolute_number, COALESCE(e.name, '')
		FROM torrent_assignments t
		LEFT JOIN episodes e ON t.item_type = 'episode' AND e.id = t.item_id
		LEFT JOIN seasons sn ON sn.id = e.season_id
		WHERE t.info_hash = $1 AND t.is_active
		  AND (e.id IS NOT NULL OR (t.item_type = 'movie' AND EXISTS (SELECT 1 FROM movies m WHERE m.id = t.item_id)))
		ORDER BY t.file_path, sn.season_number, e.episode_number, t.item_id
	`, infoHash)
	if err != nil {
		return nil, fmt.Errorf("failed to load the torrent's assignments: %w", err)
	}
	defer rows.Close()
	var out []FileMapping
	for rows.Next() {
		var m FileMapping
		var itemType ItemType
		var itemID int64
		var showID sql.NullInt64
		var season, number, absolute sql.NullInt32
		var name string
		if err := rows.Scan(&m.FilePath, &itemType, &itemID, &showID, &season, &number, &absolute, &name); err != nil {
			return nil, fmt.Errorf("failed to scan an assignment: %w", err)
		}
		if itemType == ItemTypeMovie {
			id := itemID
			m.MovieID = &id
		} else {
			m.Episode = &EpisodeRef{ID: itemID, ShowID: showID.Int64, Season: int(season.Int32), Number: int(number.Int32),
				AbsoluteNumber: nullIntPtr(absolute), Name: name}
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read the torrent's assignments: %w", err)
	}
	return out, nil
}

// WriteOverride writes a manual override in its own transaction, through
// the same writer as Apply (the title's lock, the upsert, the item's other
// rows deactivated, its torrent subtitles dropped when its file changes).
// The write is partial: the torrent's other rows on the title stay. It
// returns ErrShowNotFound or ErrMovieNotFound when the title is gone.
func (r *AssignmentRepository) WriteOverride(ctx context.Context, w AssignmentWrite) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()
	w.Partial = true
	if err := writeAssignments(ctx, tx, w); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit: %w", err)
	}
	return nil
}
