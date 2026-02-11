package library

import (
	"database/sql"
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

// Create adds a new movie to the library
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

// GetByID retrieves a movie by its ID
func (r *MovieRepository) GetByID(id int64) (*Movie, error) {
	movie := &Movie{}
	err := r.db.QueryRow(
		`SELECT id, tmdb_id, title, year, created_at FROM movies WHERE id = $1`,
		id,
	).Scan(&movie.ID, &movie.TMDBID, &movie.Title, &movie.Year, &movie.CreatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get movie: %w", err)
	}

	return movie, nil
}

// GetByTMDBID retrieves a movie by its TMDB ID
func (r *MovieRepository) GetByTMDBID(tmdbID int) (*Movie, error) {
	movie := &Movie{}
	err := r.db.QueryRow(
		`SELECT id, tmdb_id, title, year, created_at FROM movies WHERE tmdb_id = $1`,
		tmdbID,
	).Scan(&movie.ID, &movie.TMDBID, &movie.Title, &movie.Year, &movie.CreatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get movie: %w", err)
	}

	return movie, nil
}

// List returns all movies in the library
func (r *MovieRepository) List() ([]*Movie, error) {
	rows, err := r.db.Query(
		`SELECT id, tmdb_id, title, year, created_at FROM movies ORDER BY title`,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list movies: %w", err)
	}
	defer rows.Close()

	var movies []*Movie
	for rows.Next() {
		movie := &Movie{}
		if err := rows.Scan(&movie.ID, &movie.TMDBID, &movie.Title, &movie.Year, &movie.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan movie: %w", err)
		}
		movies = append(movies, movie)
	}

	return movies, rows.Err()
}

// ListWithAssignments returns all movies that have active torrent assignments
func (r *MovieRepository) ListWithAssignments() ([]*Movie, error) {
	rows, err := r.db.Query(`
		SELECT m.id, m.tmdb_id, m.title, m.year, m.created_at,
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
		movie := &Movie{}
		assignment := &TorrentAssignment{ItemType: ItemTypeMovie}

		var resolution, source sql.NullString

		if err := rows.Scan(
			&movie.ID, &movie.TMDBID, &movie.Title, &movie.Year, &movie.CreatedAt,
			&assignment.ID, &assignment.InfoHash, &assignment.MagnetURI,
			&assignment.FilePath, &assignment.FileSize,
			&resolution, &source, &assignment.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan movie: %w", err)
		}

		assignment.ItemID = movie.ID
		assignment.Resolution = resolution.String
		assignment.Source = source.String
		assignment.IsActive = true
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

// Update updates a movie's metadata
func (r *MovieRepository) Update(movie *Movie) error {
	_, err := r.db.Exec(
		`UPDATE movies SET title = $1, year = $2 WHERE id = $3`,
		movie.Title, movie.Year, movie.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update movie: %w", err)
	}
	return nil
}
