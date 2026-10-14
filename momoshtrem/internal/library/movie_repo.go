package library

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// MovieRepository handles movie database operations
type MovieRepository struct {
	db *DB
}

// NewMovieRepository creates a new movie repository
func NewMovieRepository(db *DB) *MovieRepository {
	return &MovieRepository{db: db}
}

// movieColumns selects a movie row (alias m) in the order scanMovie reads it.
const movieColumns = `m.id, m.tmdb_id, m.title, m.year, m.created_at,
	COALESCE(m.imdb_id, ''), COALESCE(m.poster_path, ''), COALESCE(m.backdrop_path, ''),
	COALESCE(m.runtime, 0), COALESCE(m.genres, '[]'::jsonb), m.metadata_updated_at`

// scanMovie reads movieColumns, followed by any extra destinations.
func scanMovie(s scanner, extra ...any) (*Movie, error) {
	movie := &Movie{}
	var genres []byte
	var updatedAt sql.NullTime
	dest := append([]any{
		&movie.ID, &movie.TMDBID, &movie.Title, &movie.Year, &movie.CreatedAt,
		&movie.IMDbID, &movie.PosterPath, &movie.BackdropPath,
		&movie.Runtime, &genres, &updatedAt,
	}, extra...)
	if err := s.Scan(dest...); err != nil {
		return nil, err
	}
	var err error
	if movie.Genres, err = decodeStrings(genres); err != nil {
		return nil, fmt.Errorf("movie %d genres: %w", movie.ID, err)
	}
	if updatedAt.Valid {
		movie.MetadataUpdatedAt = &updatedAt.Time
	}
	return movie, nil
}

// Create adds a new movie to the library (title, year and TMDB ID only).
func (r *MovieRepository) Create(movie *Movie) error {
	err := r.db.QueryRow(
		`INSERT INTO movies (tmdb_id, title, year) VALUES ($1, $2, $3) RETURNING id, created_at`,
		movie.TMDBID, movie.Title, movie.Year,
	).Scan(&movie.ID, &movie.CreatedAt)
	if err != nil {
		return fmt.Errorf("failed to create movie: %w", err)
	}
	return nil
}

// CreateIfAbsent inserts movie with its metadata and stamps
// metadata_updated_at. If a movie with the same TMDB ID already exists it is
// left alone, movie is overwritten with the stored row, and created is false.
func (r *MovieRepository) CreateIfAbsent(ctx context.Context, movie *Movie) (created bool, err error) {
	genres, err := encodeStrings(movie.Genres)
	if err != nil {
		return false, err
	}
	err = r.db.QueryRowContext(ctx, `
		INSERT INTO movies (tmdb_id, title, year, imdb_id, poster_path, backdrop_path, runtime, genres, metadata_updated_at)
		VALUES ($1, $2, $3, NULLIF($4, ''), NULLIF($5, ''), NULLIF($6, ''), NULLIF($7, 0), $8::jsonb, NOW())
		ON CONFLICT (tmdb_id) DO NOTHING
		RETURNING id, created_at, metadata_updated_at
	`, movie.TMDBID, movie.Title, movie.Year, movie.IMDbID, movie.PosterPath, movie.BackdropPath,
		movie.Runtime, genres,
	).Scan(&movie.ID, &movie.CreatedAt, &movie.MetadataUpdatedAt)
	if err == nil {
		return true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return false, fmt.Errorf("failed to create movie: %w", err)
	}

	existing, err := r.GetByTMDBID(movie.TMDBID)
	if err != nil {
		return false, err
	}
	if existing == nil {
		return false, fmt.Errorf("movie %d conflicted on insert but is gone", movie.TMDBID)
	}
	*movie = *existing
	return false, nil
}

// GetByID retrieves a movie by its ID
func (r *MovieRepository) GetByID(id int64) (*Movie, error) {
	movie, err := scanMovie(r.db.QueryRow(`SELECT `+movieColumns+` FROM movies m WHERE m.id = $1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get movie: %w", err)
	}
	return movie, nil
}

// GetByTMDBID retrieves a movie by its TMDB ID
func (r *MovieRepository) GetByTMDBID(tmdbID int) (*Movie, error) {
	movie, err := scanMovie(r.db.QueryRow(`SELECT `+movieColumns+` FROM movies m WHERE m.tmdb_id = $1`, tmdbID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get movie: %w", err)
	}
	return movie, nil
}

// List returns all movies in the library
func (r *MovieRepository) List() ([]*Movie, error) {
	rows, err := r.db.Query(`SELECT ` + movieColumns + ` FROM movies m ORDER BY m.title`)
	if err != nil {
		return nil, fmt.Errorf("failed to list movies: %w", err)
	}
	defer rows.Close()

	var movies []*Movie
	for rows.Next() {
		movie, err := scanMovie(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan movie: %w", err)
		}
		movies = append(movies, movie)
	}

	return movies, rows.Err()
}

// ListWithActiveAssignment returns every movie, by title, each with its
// active assignment (Movie.Assignment, nil when none). One query.
func (r *MovieRepository) ListWithActiveAssignment(ctx context.Context) ([]*Movie, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT `+movieColumns+`, `+assignmentColumns+`
		FROM movies m
		`+activeAssignmentLateral("movie", "m.id")+`
		ORDER BY m.title
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to list movies: %w", err)
	}
	defer rows.Close()

	var movies []*Movie
	for rows.Next() {
		dest, finish := assignmentDest(ItemTypeMovie)
		movie, err := scanMovie(rows, dest...)
		if err != nil {
			return nil, fmt.Errorf("failed to scan movie: %w", err)
		}
		movie.Assignment = finish(movie.ID)
		movies = append(movies, movie)
	}
	return movies, rows.Err()
}

// ListWithAssignments returns all movies that have active torrent assignments
func (r *MovieRepository) ListWithAssignments() ([]*Movie, error) {
	rows, err := r.db.Query(`
		SELECT ` + movieColumns + `,
		       ta.id, ta.info_hash, ta.magnet_uri, ta.file_path, ta.file_size,
		       ta.resolution, ta.source, ta.created_at
		FROM movies m
		INNER JOIN torrent_assignments ta ON ta.item_type = 'movie' AND ta.item_id = m.id AND ta.is_active = TRUE
		ORDER BY m.title
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to list movies with assignments: %w", err)
	}
	defer rows.Close()

	var movies []*Movie
	for rows.Next() {
		assignment := &TorrentAssignment{ItemType: ItemTypeMovie, IsActive: true}
		var resolution, source sql.NullString

		movie, err := scanMovie(rows,
			&assignment.ID, &assignment.InfoHash, &assignment.MagnetURI,
			&assignment.FilePath, &assignment.FileSize,
			&resolution, &source, &assignment.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan movie: %w", err)
		}

		assignment.ItemID = movie.ID
		assignment.Resolution = resolution.String
		assignment.Source = source.String
		movie.Assignment = assignment

		movies = append(movies, movie)
	}

	return movies, rows.Err()
}

// Delete removes a movie from the library
func (r *MovieRepository) Delete(id int64) error {
	result, err := r.db.Exec(`DELETE FROM movies WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("failed to delete movie: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check delete result: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("movie not found")
	}

	return nil
}
