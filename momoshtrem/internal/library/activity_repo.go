package library

import (
	"cmp"
	"context"
	"database/sql"
	"fmt"
	"slices"
	"time"
)

// What Activity (PLAN §6 "Torrents and activity (B6)") reads about the
// torrents the library knows: those an active torrent assignment or an
// open, non-dry-run job uses, whether the client has them loaded or not.

// KnownTorrent is a torrent the library knows, with the titles it's for.
type KnownTorrent struct {
	InfoHash string
	// JobName is the torrent's name from its newest job that knows its
	// files (else its newest job): the info dict's name once the job saw
	// the file list, the magnet's dn or the hash before. "" without a job.
	JobName string
	// SizeBytes is that job's size, else the sum of its stored files; nil
	// when no job knows it.
	SizeBytes *int64
	// MagnetURI is the newest active assignment's magnet, else the newest
	// open job's.
	MagnetURI string
	// AddedAt is the earliest active assignment or open job.
	AddedAt time.Time
	// OpenJob is the newest open, non-dry-run job; nil without one.
	OpenJob *OpenJobRef
	// Titles are the movies and shows its active assignments and open jobs
	// are for, by title and year.
	Titles []KnownTitle
}

// OpenJobRef is an open mapping job.
type OpenJobRef struct {
	ID     int64
	Status JobStatus
}

// KnownTitle is a library title a torrent is for.
type KnownTitle struct {
	MediaType  JobMediaType // movie or tv
	LibraryID  int64
	TMDBID     int
	Title      string
	Year       int
	PosterPath string // "" when there's none
	// Episodes are the show's episodes the torrent's active assignments
	// fill, by season and number; empty for a movie, or a show only an
	// open job links.
	Episodes []EpisodeNumber
	// SeasonSizes is how many episodes each of those seasons has.
	SeasonSizes map[int]int
}

// EpisodeNumber is an episode's season and in-season number.
type EpisodeNumber struct {
	Season, Episode int
}

// KnownTorrents lists the torrents the library knows (one entry per hash,
// by hash), or just infoHash when it isn't "": an empty list then means the
// library doesn't know it. An active assignment whose movie or episode is
// gone still makes its torrent known, without a title. One query.
func (r *AssignmentJobRepository) KnownTorrents(ctx context.Context, infoHash string) ([]KnownTorrent, error) {
	rows, err := r.db.QueryContext(ctx, `
		WITH assigned AS (
			SELECT t.info_hash, t.magnet_uri, t.created_at, t.item_type, t.item_id
			FROM torrent_assignments t
			WHERE t.is_active AND t.kind = 'torrent' AND ($1::text = '' OR t.info_hash = $1)
		),
		open_jobs AS (
			SELECT j.id, j.info_hash, j.magnet_uri, j.created_at, j.status, j.media_type, j.show_id, j.movie_id
			FROM assignment_jobs j
			WHERE NOT j.dry_run AND j.status = ANY($2) AND ($1::text = '' OR j.info_hash = $1)
		),
		hashes AS (SELECT info_hash FROM assigned UNION SELECT info_hash FROM open_jobs),
		newest AS (
			SELECT DISTINCT ON (j.info_hash) j.info_hash, j.torrent_name,
			       COALESCE(j.size_bytes, (SELECT sum(p.file_size) FROM assignment_proposals p WHERE p.job_id = j.id))::bigint AS size_bytes
			FROM assignment_jobs j JOIN hashes h ON h.info_hash = j.info_hash
			ORDER BY j.info_hash,
			         (j.size_bytes IS NOT NULL OR EXISTS (SELECT 1 FROM assignment_proposals p WHERE p.job_id = j.id)) DESC,
			         j.id DESC
		),
		season_sizes AS (
			SELECT e.season_id, count(*) AS n FROM episodes e
			WHERE e.season_id IN (
				SELECT ae.season_id FROM assigned a JOIN episodes ae ON a.item_type = 'episode' AND ae.id = a.item_id)
			GROUP BY e.season_id
		)
		SELECT 'assignment' AS kind, a.info_hash, a.magnet_uri, a.created_at,
		       CASE WHEN a.item_type = 'movie' THEN 'movie' ELSE 'tv' END AS media_type,
		       COALESCE(s.id, m.id) AS library_id, COALESCE(s.tmdb_id, m.tmdb_id) AS tmdb_id,
		       COALESCE(s.title, m.title) AS title, COALESCE(s.year, m.year) AS year,
		       COALESCE(s.poster_path, m.poster_path, '') AS poster_path,
		       sn.season_number, e.episode_number, ss.n AS season_size,
		       NULL::bigint AS job_id, NULL::text AS job_status, NULL::text AS torrent_name, NULL::bigint AS size_bytes
		FROM assigned a
		LEFT JOIN episodes e ON a.item_type = 'episode' AND e.id = a.item_id
		LEFT JOIN seasons sn ON sn.id = e.season_id
		LEFT JOIN shows s ON s.id = sn.show_id
		LEFT JOIN movies m ON a.item_type = 'movie' AND m.id = a.item_id
		LEFT JOIN season_sizes ss ON ss.season_id = sn.id
		UNION ALL
		SELECT 'job', j.info_hash, j.magnet_uri, j.created_at, j.media_type,
		       COALESCE(s.id, m.id), COALESCE(s.tmdb_id, m.tmdb_id), COALESCE(s.title, m.title), COALESCE(s.year, m.year),
		       COALESCE(s.poster_path, m.poster_path, ''),
		       NULL, NULL, NULL,
		       j.id, j.status, NULL, NULL
		FROM open_jobs j
		LEFT JOIN shows s ON s.id = j.show_id
		LEFT JOIN movies m ON m.id = j.movie_id
		UNION ALL
		SELECT 'newest', n.info_hash, NULL, NULL, NULL,
		       NULL, NULL, NULL, NULL, NULL,
		       NULL, NULL, NULL,
		       NULL, NULL, n.torrent_name, n.size_bytes
		FROM newest n
	`, infoHash, openStatuses())
	if err != nil {
		return nil, fmt.Errorf("failed to list the known torrents: %w", err)
	}
	defer rows.Close()

	byHash := map[string]*knownBuilder{}
	for rows.Next() {
		var (
			kind, hash                    string
			magnet, mediaType, title      sql.NullString
			posterPath, jobStatus, tName  sql.NullString
			createdAt                     sql.NullTime
			libraryID, jobID, sizeBytes   sql.NullInt64
			seasonSize                    sql.NullInt64
			tmdbID, year, season, episode sql.NullInt32
		)
		if err := rows.Scan(&kind, &hash, &magnet, &createdAt, &mediaType,
			&libraryID, &tmdbID, &title, &year, &posterPath,
			&season, &episode, &seasonSize,
			&jobID, &jobStatus, &tName, &sizeBytes); err != nil {
			return nil, fmt.Errorf("failed to scan a known torrent: %w", err)
		}
		b := byHash[hash]
		if b == nil {
			b = &knownBuilder{KnownTorrent: KnownTorrent{InfoHash: hash}, titles: map[titleKey]*KnownTitle{}}
			byHash[hash] = b
		}
		switch kind {
		case "newest":
			b.JobName = tName.String
			b.SizeBytes = nullInt64Ptr(sizeBytes)
			continue
		case "assignment":
			b.assignmentMagnet.see(magnet.String, createdAt)
		case "job":
			b.jobMagnet.see(magnet.String, createdAt)
			if b.OpenJob == nil || jobID.Int64 > b.OpenJob.ID {
				b.OpenJob = &OpenJobRef{ID: jobID.Int64, Status: JobStatus(jobStatus.String)}
			}
		}
		if createdAt.Valid && (b.AddedAt.IsZero() || createdAt.Time.Before(b.AddedAt)) {
			b.AddedAt = createdAt.Time
		}
		if !libraryID.Valid {
			continue // the item or title is gone
		}
		key := titleKey{JobMediaType(mediaType.String), libraryID.Int64}
		t := b.titles[key]
		if t == nil {
			t = &KnownTitle{MediaType: key.mediaType, LibraryID: key.id, TMDBID: int(tmdbID.Int32),
				Title: title.String, Year: int(year.Int32), PosterPath: posterPath.String}
			b.titles[key] = t
		}
		if season.Valid && episode.Valid {
			t.Episodes = append(t.Episodes, EpisodeNumber{Season: int(season.Int32), Episode: int(episode.Int32)})
			if t.SeasonSizes == nil {
				t.SeasonSizes = map[int]int{}
			}
			t.SeasonSizes[int(season.Int32)] = int(seasonSize.Int64)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read the known torrents: %w", err)
	}

	out := make([]KnownTorrent, 0, len(byHash))
	for _, b := range byHash {
		b.MagnetURI = cmp.Or(b.assignmentMagnet.uri, b.jobMagnet.uri)
		for _, t := range b.titles {
			slices.SortFunc(t.Episodes, func(x, y EpisodeNumber) int {
				return cmp.Or(cmp.Compare(x.Season, y.Season), cmp.Compare(x.Episode, y.Episode))
			})
			t.Episodes = slices.Compact(t.Episodes) // two of its files on one episode count once
			b.Titles = append(b.Titles, *t)
		}
		slices.SortFunc(b.Titles, func(x, y KnownTitle) int {
			return cmp.Or(cmp.Compare(x.Title, y.Title), cmp.Compare(x.Year, y.Year), cmp.Compare(x.LibraryID, y.LibraryID))
		})
		out = append(out, b.KnownTorrent)
	}
	// A hash with only a "newest" row can't happen: newest joins hashes.
	slices.SortFunc(out, func(x, y KnownTorrent) int { return cmp.Compare(x.InfoHash, y.InfoHash) })
	return out, nil
}

type titleKey struct {
	mediaType JobMediaType
	id        int64
}

// knownBuilder gathers one hash's rows.
type knownBuilder struct {
	KnownTorrent
	titles                      map[titleKey]*KnownTitle
	assignmentMagnet, jobMagnet newestMagnet
}

// newestMagnet keeps the newest magnet of the rows it sees.
type newestMagnet struct {
	uri string
	at  time.Time
}

func (m *newestMagnet) see(uri string, at sql.NullTime) {
	if uri == "" {
		return
	}
	if m.uri == "" || (at.Valid && at.Time.After(m.at)) {
		m.uri = uri
		m.at = at.Time
	}
}
