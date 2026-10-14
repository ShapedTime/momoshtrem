// Package metadata keeps the library's TMDB metadata in step with TMDB: it
// adds movies and shows, refreshes them, applies per-show settings and
// backfills existing rows. Titles, years and (on backfill) episode names are
// never rewritten, so WebDAV paths stay stable.
package metadata

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"sync"

	"golang.org/x/sync/singleflight"

	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/tmdb"
)

// seasonParallelism caps concurrent season fetches per show.
const seasonParallelism = 4

// TMDB is the metadata source. *tmdb.Cached implements it.
type TMDB interface {
	MovieDetails(ctx context.Context, id int) (*tmdb.MovieDetails, error)
	ShowDetails(ctx context.Context, id int) (*tmdb.ShowDetails, error)
	Season(ctx context.Context, showID, seasonNumber int, showStatus string) (*tmdb.Season, error)
	EpisodeGroups(ctx context.Context, showID int) ([]tmdb.EpisodeGroupSummary, error)
	EpisodeGroup(ctx context.Context, id string) (*tmdb.EpisodeGroup, error)
}

var _ TMDB = (*tmdb.Cached)(nil)

// TreeInvalidator rebuilds the WebDAV tree. vfs.TreeUpdater implements it.
type TreeInvalidator interface {
	InvalidateTree()
}

// Syncer writes TMDB metadata into the library.
type Syncer struct {
	tmdb   TMDB
	movies *library.MovieRepository
	shows  *library.ShowRepository
	meta   *library.MetadataRepository
	tree   TreeInvalidator // Optional
	log    *slog.Logger

	refreshGroup singleflight.Group // One refresh per show at a time
}

// Option configures a Syncer.
type Option func(*Syncer)

// WithTreeInvalidator rebuilds the WebDAV tree when a refresh renames an
// assigned episode.
func WithTreeInvalidator(t TreeInvalidator) Option {
	return func(s *Syncer) { s.tree = t }
}

// NewSyncer creates a Syncer.
func NewSyncer(client TMDB, movies *library.MovieRepository, shows *library.ShowRepository, meta *library.MetadataRepository, opts ...Option) *Syncer {
	s := &Syncer{
		tmdb:   client,
		movies: movies,
		shows:  shows,
		meta:   meta,
		log:    slog.With("component", "metadata"),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// tmdbError marks err as a TMDB failure. The result matches both
// library.ErrTMDBFetch and the underlying tmdb error (tmdb.ErrNotFound...).
func tmdbError(err error) error {
	return fmt.Errorf("%w: %w", library.ErrTMDBFetch, err)
}

// MovieResult is the outcome of CreateMovie.
type MovieResult struct {
	Movie    *library.Movie
	Existing bool // The movie was already in the library
}

// CreateMovie adds a movie by TMDB ID with its metadata. A movie already in
// the library is returned as is.
func (s *Syncer) CreateMovie(ctx context.Context, tmdbID int) (*MovieResult, error) {
	existing, err := s.movies.GetByTMDBID(tmdbID)
	if err != nil {
		return nil, fmt.Errorf("failed to check existing movie: %w", err)
	}
	if existing != nil {
		return &MovieResult{Movie: existing, Existing: true}, nil
	}

	details, err := s.tmdb.MovieDetails(ctx, tmdbID)
	if err != nil {
		return nil, tmdbError(err)
	}

	m := movieMetadata(details)
	movie := &library.Movie{
		TMDBID:       details.ID,
		Title:        details.Title,
		Year:         details.Year(),
		IMDbID:       m.IMDbID,
		PosterPath:   m.PosterPath,
		BackdropPath: m.BackdropPath,
		Runtime:      m.Runtime,
		Genres:       m.Genres,
	}
	created, err := s.movies.CreateIfAbsent(ctx, movie)
	if err != nil {
		return nil, err
	}
	return &MovieResult{Movie: movie, Existing: !created}, nil
}

// RefreshMovie re-fetches a movie's metadata from TMDB, bypassing the cache.
// Title and year are kept.
func (s *Syncer) RefreshMovie(ctx context.Context, movieID int64) (*library.Movie, error) {
	movie, err := s.movies.GetByID(movieID)
	if err != nil {
		return nil, err
	}
	if movie == nil {
		return nil, library.ErrMovieNotFound
	}

	details, err := s.tmdb.MovieDetails(tmdb.Bypass(ctx), movie.TMDBID)
	if err != nil {
		return nil, tmdbError(err)
	}
	if err := s.meta.UpdateMovieMetadata(ctx, movie.ID, movieMetadata(details)); err != nil {
		return nil, err
	}

	refreshed, err := s.movies.GetByID(movie.ID)
	if err != nil {
		return nil, err
	}
	if refreshed == nil {
		return nil, library.ErrMovieNotFound
	}
	return refreshed, nil
}

// CreateShowInput contains parameters for creating a show.
type CreateShowInput struct {
	TMDBID int
	// Seasons limits the sync to these season numbers. Empty means every
	// season TMDB lists, specials included.
	Seasons []int
}

// SeasonError is a season that failed to sync. The rest of the show still
// syncs.
type SeasonError struct {
	SeasonNumber int
	Err          error
}

func (e SeasonError) Error() string {
	return fmt.Sprintf("season %d: %v", e.SeasonNumber, e.Err)
}

// ShowResult is the outcome of creating or refreshing a show.
type ShowResult struct {
	// Show is reloaded after the sync, with every season, episode and active
	// assignment.
	Show          *library.Show
	Existing      bool // CreateShow found the show already in the library
	SeasonsAdded  int
	EpisodesAdded int
	SeasonErrors  []SeasonError
	// NumberingError is set when absolute numbers couldn't be computed
	// (TMDB episode groups unavailable); the previous numbers are kept.
	NumberingError error
}

// Complete reports whether every season synced and the show was numbered.
// Only then is the show stamped as synced.
func (r *ShowResult) Complete() bool {
	return len(r.SeasonErrors) == 0 && r.NumberingError == nil
}

// CreateShow adds a show by TMDB ID with its seasons (season 0 too: specials
// are on by default) and episodes, and numbers them. Anime gets absolute
// ordering. A show already in the library is returned as is.
func (s *Syncer) CreateShow(ctx context.Context, in CreateShowInput) (*ShowResult, error) {
	existing, err := s.shows.GetByTMDBID(in.TMDBID)
	if err != nil {
		return nil, fmt.Errorf("failed to check existing show: %w", err)
	}
	if existing != nil {
		return s.existingShow(ctx, existing.ID)
	}

	details, err := s.tmdb.ShowDetails(ctx, in.TMDBID)
	if err != nil {
		return nil, tmdbError(err)
	}

	m := showMetadata(details)
	show := &library.Show{
		TMDBID:           details.ID,
		Title:            details.Name,
		Year:             details.Year(),
		IMDbID:           m.IMDbID,
		TVDBID:           m.TVDBID,
		OriginalName:     m.OriginalName,
		OriginalLanguage: m.OriginalLanguage,
		OriginCountry:    m.OriginCountry,
		IsAnime:          m.IsAnime,
		PosterPath:       m.PosterPath,
		BackdropPath:     m.BackdropPath,
		Status:           m.Status,
		Genres:           m.Genres,
		EpisodeOrdering:  DefaultOrdering(m.IsAnime),
	}
	created, err := s.shows.CreateIfAbsent(ctx, show)
	if err != nil {
		return nil, err
	}
	if !created {
		return s.existingShow(ctx, show.ID)
	}

	seasons := in.Seasons
	if len(seasons) == 0 {
		seasons = seasonsToSync(details, show.IncludeSpecials)
	}
	return s.syncShow(ctx, show, details.Status, seasons, false)
}

func (s *Syncer) existingShow(ctx context.Context, id int64) (*ShowResult, error) {
	show, err := s.shows.GetWithSeasonsAndEpisodesContext(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to load existing show: %w", err)
	}
	if show == nil {
		return nil, library.ErrShowNotFound
	}
	return &ShowResult{Show: show, Existing: true}, nil
}

// RefreshShow re-fetches a show, its seasons and episodes from TMDB,
// bypassing the cache. New seasons and episodes are added and names are
// updated; when an assigned episode is renamed the WebDAV tree is rebuilt.
// Title and year are kept. Concurrent refreshes of one show share a result.
func (s *Syncer) RefreshShow(ctx context.Context, showID int64) (*ShowResult, error) {
	v, err, _ := s.refreshGroup.Do(strconv.FormatInt(showID, 10), func() (any, error) {
		return s.refreshShow(ctx, showID)
	})
	if err != nil {
		return nil, err
	}
	return v.(*ShowResult), nil
}

func (s *Syncer) refreshShow(ctx context.Context, showID int64) (*ShowResult, error) {
	show, err := s.shows.GetByID(showID)
	if err != nil {
		return nil, err
	}
	if show == nil {
		return nil, library.ErrShowNotFound
	}

	fresh := tmdb.Bypass(ctx)
	details, err := s.tmdb.ShowDetails(fresh, show.TMDBID)
	if err != nil {
		return nil, tmdbError(err)
	}
	if err := s.updateShowMetadata(ctx, show, details); err != nil {
		return nil, err
	}
	return s.syncShow(fresh, show, details.Status, seasonsToSync(details, show.IncludeSpecials), false)
}

// updateShowMetadata stores the show's TMDB metadata. A show added before
// metadata existed gets anime's default (absolute) ordering, like a newly
// added show, but only on the sync that first detects it as anime: once
// is_anime is stored, a later sync never overrides the user's ordering, even
// if an earlier sync didn't finish.
func (s *Syncer) updateShowMetadata(ctx context.Context, show *library.Show, details *tmdb.ShowDetails) error {
	m := showMetadata(details)
	wasAnime := show.IsAnime
	if err := s.meta.UpdateShowMetadata(ctx, show.ID, m); err != nil {
		return err
	}
	show.IsAnime = m.IsAnime
	show.Status = m.Status

	firstDetected := m.IsAnime && !wasAnime && show.MetadataUpdatedAt == nil
	if firstDetected && show.EpisodeOrdering == library.OrderingAired {
		settings := show.Settings()
		settings.EpisodeOrdering = DefaultOrdering(true)
		if err := s.meta.UpdateShowSettings(ctx, show.ID, settings); err != nil {
			return err
		}
		show.EpisodeOrdering = settings.EpisodeOrdering
	}
	return nil
}

// syncShow syncs seasons, renumbers the show, stamps it as synced when every
// season worked, and returns it reloaded.
func (s *Syncer) syncShow(ctx context.Context, show *library.Show, status string, seasons []int, keepNames bool) (*ShowResult, error) {
	sr := s.syncSeasons(ctx, show, status, seasons, keepNames)
	result := &ShowResult{
		SeasonsAdded:  sr.seasonsAdded,
		EpisodesAdded: sr.episodesAdded,
		SeasonErrors:  sr.errors,
	}

	full, err := s.shows.GetWithSeasonsAndEpisodesContext(ctx, show.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to reload show: %w", err)
	}
	if full == nil {
		return nil, library.ErrShowNotFound
	}
	if err := s.renumber(ctx, full); err != nil {
		// Keep the previous numbers; the next refresh or backfill retries.
		s.log.Warn("Failed to compute absolute numbers", "show_id", show.ID, "error", err)
		result.NumberingError = err
	}

	if result.Complete() {
		if err := s.meta.MarkShowMetadataUpdated(ctx, show.ID); err != nil {
			return nil, err
		}
	}
	if sr.assignedRenamed && s.tree != nil {
		s.log.Info("Assigned episode renamed; rebuilding WebDAV tree", "show_id", show.ID)
		s.tree.InvalidateTree()
	}

	result.Show = full
	return result, nil
}

type seasonSyncResult struct {
	seasonsAdded    int
	episodesAdded   int
	assignedRenamed bool
	errors          []SeasonError
}

// syncSeasons fetches seasons from TMDB, at most seasonParallelism at a
// time, then writes them in order. A failed season is recorded and skipped.
func (s *Syncer) syncSeasons(ctx context.Context, show *library.Show, status string, numbers []int, keepNames bool) seasonSyncResult {
	numbers = uniqueSorted(numbers)
	fetched := make([]*tmdb.Season, len(numbers))
	errs := make([]error, len(numbers))

	var wg sync.WaitGroup
	sem := make(chan struct{}, seasonParallelism)
	for i, n := range numbers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				errs[i] = ctx.Err()
				return
			}
			defer func() { <-sem }()
			fetched[i], errs[i] = s.tmdb.Season(ctx, show.TMDBID, n, status)
		}()
	}
	wg.Wait()

	var result seasonSyncResult
	for i, n := range numbers {
		if errs[i] != nil {
			result.errors = append(result.errors, SeasonError{SeasonNumber: n, Err: tmdbError(errs[i])})
			s.log.Warn("Failed to fetch season", "show_id", show.ID, "season", n, "error", errs[i])
			continue
		}
		added, eps, renamed, err := s.writeSeason(ctx, show.ID, n, fetched[i], keepNames)
		if err != nil {
			result.errors = append(result.errors, SeasonError{SeasonNumber: n, Err: err})
			s.log.Warn("Failed to store season", "show_id", show.ID, "season", n, "error", err)
			continue
		}
		if added {
			result.seasonsAdded++
		}
		result.episodesAdded += eps
		result.assignedRenamed = result.assignedRenamed || renamed
	}
	return result
}

func (s *Syncer) writeSeason(ctx context.Context, showID int64, number int, season *tmdb.Season, keepNames bool) (created bool, episodesAdded int, assignedRenamed bool, err error) {
	row := &library.Season{ShowID: showID, SeasonNumber: number, Name: season.Name, PosterPath: season.PosterPath}
	created, err = s.meta.UpsertSeason(ctx, row)
	if err != nil {
		return false, 0, false, err
	}

	episodes := make([]library.EpisodeMetadata, 0, len(season.Episodes))
	for _, ep := range season.Episodes {
		episodes = append(episodes, library.EpisodeMetadata{
			EpisodeNumber: ep.EpisodeNumber,
			Name:          ep.Name,
			AirDate:       ep.AirDate,
			TMDBEpisodeID: ep.ID,
			EpisodeType:   ep.EpisodeType,
			Runtime:       ep.Runtime,
			StillPath:     ep.StillPath,
			Overview:      ep.Overview,
		})
	}
	res, err := s.meta.UpsertEpisodes(ctx, row.ID, episodes, keepNames)
	if err != nil {
		return created, 0, false, err
	}
	return created, res.Added, res.AssignedRenamed, nil
}

// renumber recomputes and stores the show's absolute numbers, and updates
// show (loaded with seasons and episodes) to match.
func (s *Syncer) renumber(ctx context.Context, show *library.Show) error {
	group, err := s.numberingGroup(ctx, show)
	if err != nil {
		return err
	}

	var refs []NumberedEpisode
	for _, season := range show.Seasons {
		for _, ep := range season.Episodes {
			refs = append(refs, NumberedEpisode{
				ID:            ep.ID,
				SeasonNumber:  season.SeasonNumber,
				EpisodeNumber: ep.EpisodeNumber,
				TMDBEpisodeID: ep.TMDBEpisodeID,
			})
		}
	}
	numbers := AbsoluteNumbers(refs, group)

	list := make([]library.AbsoluteNumber, 0, len(numbers))
	for id, n := range numbers {
		list = append(list, library.AbsoluteNumber{EpisodeID: id, Number: n})
	}
	if _, err := s.meta.SetAbsoluteNumbers(ctx, show.ID, list); err != nil {
		return err
	}

	for i := range show.Seasons {
		for j := range show.Seasons[i].Episodes {
			ep := &show.Seasons[i].Episodes[j]
			ep.AbsoluteNumber = nil
			if n, ok := numbers[ep.ID]; ok {
				ep.AbsoluteNumber = &n
			}
		}
	}
	return nil
}

// numberingGroup fetches the episode group the show's numbers come from, or
// returns nil for cumulative numbering.
func (s *Syncer) numberingGroup(ctx context.Context, show *library.Show) (*tmdb.EpisodeGroup, error) {
	var groups []tmdb.EpisodeGroupSummary
	if show.EpisodeOrdering == library.OrderingAbsolute && show.EpisodeGroupID == "" {
		var err error
		if groups, err = s.tmdb.EpisodeGroups(ctx, show.TMDBID); err != nil {
			return nil, tmdbError(err)
		}
	}

	id := NumberingGroup(show.EpisodeOrdering, show.EpisodeGroupID, groups)
	if id == "" {
		return nil, nil
	}
	group, err := s.tmdb.EpisodeGroup(ctx, id)
	if err != nil {
		return nil, tmdbError(err)
	}
	return group, nil
}

// seasonsToSync lists the TMDB seasons to store: every regular season, and
// season 0 when the show includes specials.
func seasonsToSync(details *tmdb.ShowDetails, includeSpecials bool) []int {
	var numbers []int
	for _, n := range details.SeasonNumbers() {
		if n > 0 || (n == 0 && includeSpecials) {
			numbers = append(numbers, n)
		}
	}
	return numbers
}

func uniqueSorted(numbers []int) []int {
	seen := make(map[int]bool, len(numbers))
	out := make([]int, 0, len(numbers))
	for _, n := range numbers {
		if !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	sort.Ints(out)
	return out
}

func movieMetadata(d *tmdb.MovieDetails) library.MovieMetadata {
	imdb := d.IMDbID
	if imdb == "" {
		imdb = d.ExternalIDs.IMDbID
	}
	return library.MovieMetadata{
		IMDbID:       imdb,
		PosterPath:   d.PosterPath,
		BackdropPath: d.BackdropPath,
		Runtime:      d.Runtime,
		Genres:       genreNames(d.Genres),
	}
}

func showMetadata(d *tmdb.ShowDetails) library.ShowMetadata {
	return library.ShowMetadata{
		IMDbID:           d.ExternalIDs.IMDbID,
		TVDBID:           d.ExternalIDs.TVDBID,
		OriginalName:     d.OriginalName,
		OriginalLanguage: d.OriginalLanguage,
		OriginCountry:    d.OriginCountry,
		IsAnime:          IsAnime(d.Genres, d.OriginalLanguage, d.OriginCountry, d.Keywords.Keywords),
		PosterPath:       d.PosterPath,
		BackdropPath:     d.BackdropPath,
		Status:           d.Status,
		Genres:           genreNames(d.Genres),
	}
}

func genreNames(genres []tmdb.Genre) []string {
	names := make([]string, 0, len(genres))
	for _, g := range genres {
		names = append(names, g.Name)
	}
	return names
}
