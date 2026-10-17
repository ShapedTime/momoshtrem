package library

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// What smart downloads read and write (#77 P5 #203, PLAN §6 "Downloads
// (#77)" › "Smart picks"): a show's next episodes with torrent files, the
// active smart picks, and their releases. internal/downloads decides; this
// file reads.

// NextEpisodeFile is an episode smart downloads may pick, with its file:
// the active assignment's torrent file.
type NextEpisodeFile struct {
	EpisodeID     int64
	SeasonNumber  int
	EpisodeNumber int
	InfoHash      string
	FilePath      string
	FileSize      int64
}

// NextEpisodesWithFiles returns the show's first k episodes from season
// and episode on (that one included) that smart downloads may pick: in
// regular seasons, never watched (no Trakt play by the matching rule, no
// local watch), and whose active assignment is a torrent file. In episode
// order. One query.
func (r *DownloadsRepository) NextEpisodesWithFiles(ctx context.Context, showID int64, season, episode, k int) ([]NextEpisodeFile, error) {
	if k <= 0 {
		return nil, nil
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT e.id, sn.season_number, e.episode_number, ta.info_hash, ta.file_path, ta.file_size
		FROM seasons sn
		JOIN shows s ON s.id = sn.show_id
		JOIN episodes e ON e.season_id = sn.id
		`+activeAssignmentLateral("episode", "e.id")+`
		WHERE sn.show_id = $1 AND sn.season_number > 0
		  AND (sn.season_number, e.episode_number) >= ($2, $3)
		  AND ta.kind = 'torrent'
		  AND NOT EXISTS (SELECT 1 FROM local_watches lw WHERE lw.episode_id = e.id)
		  AND NOT `+episodeWatchedSQL("s.tmdb_id", libraryEpisode, `w.show_tmdb_id = (SELECT tmdb_id FROM shows WHERE id = $1)`)+`
		ORDER BY sn.season_number, e.episode_number
		LIMIT $4
	`, showID, season, episode, k)
	if err != nil {
		return nil, fmt.Errorf("failed to read show %d's next episodes: %w", showID, err)
	}
	defer rows.Close()
	var out []NextEpisodeFile
	for rows.Next() {
		var f NextEpisodeFile
		if err := rows.Scan(&f.EpisodeID, &f.SeasonNumber, &f.EpisodeNumber, &f.InfoHash, &f.FilePath, &f.FileSize); err != nil {
			return nil, fmt.Errorf("failed to scan a next episode: %w", err)
		}
		out = append(out, f)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read show %d's next episodes: %w", showID, err)
	}
	return out, nil
}

// SmartPick is an active smart pick (always an episode), with what the
// smart rules ask of it.
type SmartPick struct {
	ID            int64
	EpisodeID     int64
	ShowID        int64
	SeasonNumber  int
	EpisodeNumber int
	Source        SmartSource
	CreatedAt     time.Time
	// WatchedAt is the episode's latest Trakt play or local watch; nil
	// when it has neither.
	WatchedAt *time.Time
	// HasFile says its active assignment is a torrent file.
	HasFile bool
}

// ActiveSmartPicks returns the active smart picks, oldest first. One
// query.
func (r *DownloadsRepository) ActiveSmartPicks(ctx context.Context) ([]SmartPick, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT d.id, d.episode_id, sn.show_id, sn.season_number, e.episode_number, d.smart_source, d.created_at,
		       GREATEST(
		         (SELECT max(w.last_watched_at) FROM trakt_watched_episodes w
		          WHERE w.show_tmdb_id = s.tmdb_id AND w.episode_tmdb_id = e.tmdb_episode_id),
		         (SELECT max(w.last_watched_at) FROM trakt_watched_episodes w
		          WHERE w.show_tmdb_id = s.tmdb_id AND (w.episode_tmdb_id IS NULL OR e.tmdb_episode_id IS NULL)
		            AND w.season = sn.season_number AND w.episode = e.episode_number),
		         (SELECT lw.watched_at FROM local_watches lw WHERE lw.episode_id = e.id)),
		       COALESCE(ta.kind = 'torrent', FALSE)
		FROM downloads d
		JOIN episodes e ON e.id = d.episode_id
		JOIN seasons sn ON sn.id = e.season_id
		JOIN shows s ON s.id = sn.show_id
		`+activeAssignmentLateral("episode", "e.id")+`
		WHERE d.reason = 'smart' AND d.released_at IS NULL
		ORDER BY d.created_at, d.id
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to list the smart picks: %w", err)
	}
	defer rows.Close()
	var out []SmartPick
	for rows.Next() {
		var p SmartPick
		var watched sql.NullTime
		if err := rows.Scan(&p.ID, &p.EpisodeID, &p.ShowID, &p.SeasonNumber, &p.EpisodeNumber, &p.Source, &p.CreatedAt,
			&watched, &p.HasFile); err != nil {
			return nil, fmt.Errorf("failed to scan a smart pick: %w", err)
		}
		p.WatchedAt = nullTimePtr(watched)
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to list the smart picks: %w", err)
	}
	return out, nil
}

// ReleaseSmartPicks releases the active smart picks given, at at, and
// answers the ids it released. A row that isn't an active smart pick any
// more (released, or kept since) is skipped, so a Keep that lands between
// a smart pass's read and its release stays kept.
func (r *DownloadsRepository) ReleaseSmartPicks(ctx context.Context, releases []DownloadRelease, at time.Time) ([]int64, error) {
	if len(releases) == 0 {
		return nil, nil
	}
	ids := make([]int64, len(releases))
	reasons := make([]string, len(releases))
	for i, rel := range releases {
		ids[i], reasons[i] = rel.ID, string(rel.Reason)
	}
	rows, err := r.db.QueryContext(ctx, `
		UPDATE downloads d SET released_at = $3, release_reason = m.reason, updated_at = $3
		FROM unnest($1::bigint[], $2::text[]) AS m(id, reason)
		WHERE d.id = m.id AND d.released_at IS NULL AND d.reason = 'smart'
		RETURNING d.id`, ids, reasons, at)
	if err != nil {
		return nil, fmt.Errorf("failed to release smart picks: %w", err)
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("failed to scan a released smart pick: %w", err)
		}
		out = append(out, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to release smart picks: %w", err)
	}
	return out, nil
}
