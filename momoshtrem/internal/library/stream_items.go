package library

import (
	"context"
	"database/sql"
	"fmt"
)

// StreamItem is the library item a streamed file plays as: a movie, or an
// episode with its show (a streaming session's item, PLAN §6 "Torrents and
// activity (B6)").
type StreamItem struct {
	ItemType ItemType
	ItemID   int64
	// MediaType is movie or tv; TMDBID, LibraryID and Title are the movie's
	// or the episode's show's.
	MediaType JobMediaType
	TMDBID    int
	LibraryID int64
	Title     string
	// SeasonNumber, EpisodeNumber and EpisodeName are an episode's.
	SeasonNumber  *int
	EpisodeNumber *int
	EpisodeName   string
	// RuntimeMinutes is the file's: the movie's or the episode's runtime,
	// or the sum over the episodes that share the episode's torrent file
	// ("S01E01-E02"). 0 when one of them is unknown.
	RuntimeMinutes int
}

// StreamItems returns the movies and episodes with these ids, those that
// still exist. One query.
func (r *CatalogRepository) StreamItems(ctx context.Context, movieIDs, episodeIDs []int64) ([]StreamItem, error) {
	if len(movieIDs) == 0 && len(episodeIDs) == 0 {
		return nil, nil
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT 'movie', m.id, m.tmdb_id, m.id, m.title, NULL::int, NULL::int, '', COALESCE(m.runtime, 0)
		FROM movies m
		WHERE m.id = ANY($1::bigint[])
		UNION ALL
		SELECT 'episode', e.id, s.tmdb_id, s.id, s.title, sn.season_number, e.episode_number, COALESCE(e.name, ''),
		       COALESCE(shared.runtime, e.runtime, 0)
		FROM episodes e
		JOIN seasons sn ON sn.id = e.season_id
		JOIN shows s ON s.id = sn.show_id
		LEFT JOIN LATERAL (
			SELECT CASE WHEN count(*) = count(e2.runtime) THEN sum(e2.runtime) ELSE 0 END AS runtime
			FROM torrent_assignments a
			JOIN torrent_assignments a2 ON a2.kind = 'torrent' AND a2.is_active AND a2.item_type = 'episode'
			     AND a2.info_hash = a.info_hash AND a2.file_path = a.file_path
			JOIN episodes e2 ON e2.id = a2.item_id
			WHERE a.item_type = 'episode' AND a.item_id = e.id AND a.is_active AND a.kind = 'torrent'
		) shared ON TRUE
		WHERE e.id = ANY($2::bigint[])
	`, idsOrEmpty(movieIDs), idsOrEmpty(episodeIDs))
	if err != nil {
		return nil, fmt.Errorf("failed to load the streamed items: %w", err)
	}
	defer rows.Close()
	var out []StreamItem
	for rows.Next() {
		var it StreamItem
		var season, episode sql.NullInt32
		if err := rows.Scan(&it.ItemType, &it.ItemID, &it.TMDBID, &it.LibraryID, &it.Title,
			&season, &episode, &it.EpisodeName, &it.RuntimeMinutes); err != nil {
			return nil, fmt.Errorf("failed to scan a streamed item: %w", err)
		}
		it.MediaType = JobMovie
		if it.ItemType == ItemTypeEpisode {
			it.MediaType = JobTV
			it.SeasonNumber, it.EpisodeNumber = nullIntPtr(season), nullIntPtr(episode)
		}
		out = append(out, it)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read the streamed items: %w", err)
	}
	return out, nil
}
