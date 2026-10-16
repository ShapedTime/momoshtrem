package catalog

import (
	"context"
	"path"
	"time"

	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/tmdb"
	"github.com/shapedtime/momoshtrem/internal/vfs"
)

// maxCast is how many cast members a title page lists.
const maxCast = 20

// MovieTitle builds GET /titles/movie/{tmdb_id}. Queries: the movie with its
// assignment, its mapping review, its watch state, the TMDB cache, the
// similar titles' library state and the colour cache.
//
// TMDB failures for a movie in the library fall back to its stored metadata
// (no cast or similar titles); for one that isn't, they're returned
// (wrapping library.ErrTMDBFetch and the tmdb error).
func (s *Service) MovieTitle(ctx context.Context, tmdbID int) (*MovieTitle, error) {
	movie, err := s.store.MovieByTMDBID(ctx, tmdbID)
	if err != nil {
		return nil, err
	}
	var rev *review
	if movie != nil {
		if rev, err = s.reviewOf(ctx, string(library.JobMovie), movie.ID); err != nil {
			return nil, err
		}
	}
	watch, err := s.movieWatchState(ctx, tmdbID)
	if err != nil {
		return nil, err
	}

	details, err := s.tmdb.MovieDetails(ctx, tmdbID)
	if err != nil {
		if movie == nil || !fallBackToLibrary(err) {
			return nil, tmdbError(err)
		}
		s.log.Warn("TMDB unavailable; serving the movie from the library", "tmdb_id", tmdbID, "error", err)
		details = nil
	}

	var title *MovieTitle
	if details != nil {
		title = movieFromTMDB(details)
	} else {
		title = movieFromLibrary(movie)
	}
	color := s.colorAsync(ctx, deref(title.BackdropPath))

	title.WatchState = watch
	title.Library = s.movieLibrary(movie, s.torrentView())
	rev.setOn(&title.Library)
	if details != nil {
		if title.Similar, err = s.similar(ctx, "movie", details.Recommendations.Results, details.Similar.Results); err != nil {
			return nil, err
		}
	}
	title.RecommendationReason = s.recommendationReason(ctx, "movie", tmdbID)
	title.BackdropDominantColor = color()
	return title, nil
}

func movieFromTMDB(d *tmdb.MovieDetails) *MovieTitle {
	cast := make([]CastMember, 0, min(len(d.Credits.Cast), maxCast))
	for _, c := range d.Credits.Cast {
		if len(cast) == maxCast {
			break
		}
		cast = append(cast, CastMember{Name: c.Name, Character: c.Character, ProfilePath: strPtr(c.ProfilePath)})
	}
	return &MovieTitle{
		TitleBase: TitleBase{
			MediaType:         "movie",
			TMDBID:            d.ID,
			Title:             d.Title,
			OriginalTitle:     originalTitle(d.OriginalTitle, d.Title),
			Year:              yearPtr(d.Year()),
			Overview:          d.Overview,
			PosterPath:        strPtr(d.PosterPath),
			BackdropPath:      strPtr(d.BackdropPath),
			Genres:            genreNames(d.Genres),
			VoteAverage:       voteAverage(d.VoteAverage, d.VoteCount),
			VoteCount:         d.VoteCount,
			OriginalLanguage:  strPtr(d.OriginalLanguage),
			Cast:              cast,
			TrailerYouTubeKey: strPtr(d.Videos.TrailerKey()),
			Similar:           []SimilarTitle{},
		},
		Runtime:   positive(d.Runtime),
		Directors: nonNil(d.Credits.Directors()),
	}
}

// movieFromLibrary is the title from stored metadata, when TMDB is down.
func movieFromLibrary(m *library.Movie) *MovieTitle {
	return &MovieTitle{
		TitleBase: TitleBase{
			MediaType:    "movie",
			TMDBID:       m.TMDBID,
			Title:        m.Title,
			Year:         yearPtr(m.Year),
			PosterPath:   strPtr(m.PosterPath),
			BackdropPath: strPtr(m.BackdropPath),
			Genres:       nonNil(m.Genres),
			Cast:         []CastMember{},
			Similar:      []SimilarTitle{},
		},
		Runtime:   positive(m.Runtime),
		Directors: []string{},
	}
}

// movieLibrary is a movie's library state; movie is nil when it isn't in
// the library.
func (s *Service) movieLibrary(movie *library.Movie, tv *torrentView) TitleLibraryState {
	if movie == nil {
		return notInLibrary()
	}
	state := inLibrary(movie.ID, movie.CreatedAt)
	a := movie.Assignment
	downloading, done, length, seeders := tv.fileProgress(a)
	state.AvailabilityStage = MovieStage(true, a != nil, downloading)
	if a == nil {
		return state
	}

	state.WebDAVPath = strPtr(vfs.MovieFilePath(movie.Title, movie.Year, a.FilePath))
	state.WebDAVURL = s.webdavURL(state.WebDAVPath)
	if downloading {
		state.Download = &DownloadProgress{Pct: pct(done, length), SpeedBps: tv.state(a.InfoHash).speedBps, Seeders: seeders}
	}
	if !a.IsTorrent() {
		state.Stream = s.titleStream(a) // torrent stays null
		return state
	}

	torrentState := TorrentNotLoaded
	var torrentSeeders *int
	st := tv.state(a.InfoHash)
	if st.loaded {
		torrentState = TorrentIdle
		if st.active {
			torrentState = TorrentActive
		}
		torrentSeeders = intPtr(st.seeders)
	}
	if st.paused {
		torrentState = TorrentPaused
	}
	state.Torrent = &TitleTorrent{
		InfoHash:    a.InfoHash,
		ReleaseName: releaseName(a),
		FileName:    path.Base(a.FilePath),
		SizeBytes:   a.FileSize,
		Resolution:  strPtr(a.Resolution),
		Source:      strPtr(a.Source),
		AddedAt:     a.CreatedAt,
		State:       torrentState,
		Seeders:     torrentSeeders,
	}
	return state
}

func notInLibrary() TitleLibraryState {
	return TitleLibraryState{AvailabilityStage: StageNone}
}

// inLibrary is the state of a library title before its stage is known.
func inLibrary(id int64, addedAt time.Time) TitleLibraryState {
	state := TitleLibraryState{InLibrary: true, LibraryID: &id, AvailabilityStage: StageNeedsTorrent}
	if !addedAt.IsZero() {
		state.AddedAt = &addedAt
	}
	return state
}

func (s *Service) webdavURL(p *string) *string {
	if p == nil {
		return nil
	}
	u := vfs.PublicURL(s.publicURL, *p)
	return &u
}

func originalTitle(original, title string) *string {
	if original == title {
		return nil
	}
	return strPtr(original)
}

func voteAverage(avg float64, count int) *float64 {
	if count == 0 {
		return nil
	}
	return &avg
}

func genreNames(genres []tmdb.Genre) []string {
	names := make([]string, 0, len(genres))
	for _, g := range genres {
		names = append(names, g.Name)
	}
	return names
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
