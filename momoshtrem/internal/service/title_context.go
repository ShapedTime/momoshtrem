package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/shapedtime/momoshtrem/internal/catalog"
	"github.com/shapedtime/momoshtrem/internal/identify"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/metadata"
	"github.com/shapedtime/momoshtrem/internal/tmdb"
)

// seasonFetchers caps concurrent TMDB season reads for one show.
const seasonFetchers = 4

// TMDBShowSource is what TMDBShowCatalog reads. *tmdb.Cached implements it.
type TMDBShowSource interface {
	ShowDetails(ctx context.Context, id int) (*tmdb.ShowDetails, error)
	Season(ctx context.Context, showID, seasonNumber int, showStatus string) (*tmdb.Season, error)
	CatalogSource
}

// TitleContextTMDB is the cached TMDB client. *tmdb.Cached implements it.
type TitleContextTMDB interface {
	MovieDetails(ctx context.Context, id int) (*tmdb.MovieDetails, error)
	TMDBShowSource
}

// TitleStore finds titles in the library by TMDB id; nil when they aren't
// there. *library.CatalogRepository implements it.
type TitleStore interface {
	MovieByTMDBID(ctx context.Context, tmdbID int) (*library.Movie, error)
	ShowByTMDBID(ctx context.Context, tmdbID int) (*library.Show, error)
}

// ShowLoader loads a library show with every season, episode and active
// file; nil when it's gone. *library.ShowRepository implements it.
type ShowLoader interface {
	GetWithSeasonsAndEpisodesContext(ctx context.Context, id int64) (*library.Show, error)
}

var (
	_ TitleContextTMDB = (*tmdb.Cached)(nil)
	_ TitleStore       = (*library.CatalogRepository)(nil)
	_ ShowLoader       = (*library.ShowRepository)(nil)
)

// ErrTitleNotFound: TMDB has no such title and it isn't in the library
// (404 not_found). It wraps tmdb.ErrNotFound.
//
// Other TMDB failures for a title that isn't in the library wrap
// library.ErrTMDBFetch (502 tmdb_unavailable) and their cause, so
// tmdb.ErrNoAPIKey still matches. A library title never fails on TMDB: it
// falls back to what the library stores.
var ErrTitleNotFound = errors.New("title not found")

// EpisodeIDKind says what a context's episode ids are.
type EpisodeIDKind string

const (
	// EpisodeIDsLibrary: library episode ids (the show is in the library).
	EpisodeIDsLibrary EpisodeIDKind = "library"
	// EpisodeIDsTMDB: TMDB episode ids (it isn't).
	EpisodeIDsTMDB EpisodeIDKind = "tmdb"
)

// TitleContext is what a torrent search knows about a title (PLAN §6
// "Search"): its names and ids and, for a show, its episodes, which of them
// have a file and the first missing run.
type TitleContext struct {
	MediaType library.JobMediaType
	TMDBID    int
	// LibraryID is the movie's or show's library id; nil when the title
	// isn't in the library.
	LibraryID *int64

	Title         string
	OriginalTitle string   // "" when it's the title
	Aliases       []string // TMDB's alternative titles in Latin letters, the romaji included
	// Romaji is TMDB's romaji alternative title ("Sousou no Frieren"),
	// "" when there's none. An anime's original title is in Japanese
	// script, so it isn't this.
	Romaji  string
	Year    int    // 0 when unknown
	IMDbID  string // "" when unknown
	TVDBID  int    // shows; 0 when unknown
	Runtime int    // minutes: the movie's, or a show's typical episode; 0 when unknown
	IsAnime bool

	// Shows only.
	//
	// Catalog is the classifier's view of the show: the library's, or one
	// built from TMDB (TMDBShowCatalog). EpisodeIDs says which ids its
	// episodes, Files and Missing use.
	Catalog    *identify.ShowCatalog
	EpisodeIDs EpisodeIDKind
	// Files are the episodes with an active file, by episode id: the
	// file's source, "torrent" or "hdrezka" (EpisodeSummary.source).
	Files map[int64]string
	// Missing is the title's library.missing: the first run of aired
	// episodes after the last one with a file. Nil when there's none or
	// the show isn't in the library.
	Missing *catalog.MissingRun

	// MovieFile is a library movie's file source, "torrent" or "hdrezka";
	// "" without one.
	MovieFile string
}

// TitleContextDeps are a TitleContexts' dependencies, all required but Now.
type TitleContextDeps struct {
	Titles TitleStore
	Shows  ShowLoader
	TMDB   TitleContextTMDB
	Now    func() time.Time // nil: time.Now
}

// TitleContexts builds the context of a torrent search: one per request,
// from the library when the title is there, otherwise from cached TMDB.
type TitleContexts struct {
	titles TitleStore
	shows  ShowLoader
	tmdb   TitleContextTMDB
	now    func() time.Time
	log    *slog.Logger
}

// NewTitleContexts creates a TitleContexts.
func NewTitleContexts(d TitleContextDeps) *TitleContexts {
	now := d.Now
	if now == nil {
		now = time.Now
	}
	return &TitleContexts{
		titles: d.Titles,
		shows:  d.Shows,
		tmdb:   d.TMDB,
		now:    now,
		log:    slog.With("component", "title-context"),
	}
}

// Get returns the context of a movie or show (library.JobMovie or JobTV)
// by TMDB id.
//
// A library show's catalog is the one its mapping jobs use; its aliases
// and episode groups are read from TMDB (cached) and left out when TMDB
// fails. A show that isn't in the library gets TMDBShowCatalog's, whose
// episode ids are TMDB's. A library movie reads TMDB only for its original
// title.
func (s *TitleContexts) Get(ctx context.Context, mediaType library.JobMediaType, tmdbID int) (*TitleContext, error) {
	switch mediaType {
	case library.JobMovie:
		return s.movie(ctx, tmdbID)
	case library.JobTV:
		return s.show(ctx, tmdbID)
	}
	return nil, fmt.Errorf("unknown media type %q", mediaType)
}

func (s *TitleContexts) movie(ctx context.Context, tmdbID int) (*TitleContext, error) {
	movie, err := s.titles.MovieByTMDBID(ctx, tmdbID)
	if err != nil {
		return nil, err
	}
	details, err := s.tmdb.MovieDetails(ctx, tmdbID)
	if err != nil {
		if movie == nil {
			return nil, titleTMDBError(ctx, err)
		}
		s.log.Warn("TMDB unavailable; the movie's search context comes from the library", "tmdb_id", tmdbID, "error", err)
		details = nil
	}

	tc := &TitleContext{MediaType: library.JobMovie, TMDBID: tmdbID}
	if details != nil {
		tc.Title = details.Title
		tc.OriginalTitle = details.OriginalTitle
		tc.Year = details.Year()
		tc.IMDbID = details.IMDbID
		if tc.IMDbID == "" {
			tc.IMDbID = details.ExternalIDs.IMDbID
		}
		tc.Runtime = details.Runtime
		tc.IsAnime = metadata.IsAnime(details.Genres, details.OriginalLanguage, details.OriginCountry, details.Keywords.Keywords)
	}
	if movie != nil {
		id := movie.ID
		tc.LibraryID = &id
		tc.Title, tc.Year = movie.Title, movie.Year
		if movie.IMDbID != "" {
			tc.IMDbID = movie.IMDbID
		}
		if movie.Runtime > 0 {
			tc.Runtime = movie.Runtime
		}
		if movie.Assignment != nil {
			tc.MovieFile = fileSource(movie.Assignment)
		}
	}
	if strings.EqualFold(tc.OriginalTitle, tc.Title) {
		tc.OriginalTitle = ""
	}
	return tc, nil
}

func (s *TitleContexts) show(ctx context.Context, tmdbID int) (*TitleContext, error) {
	show, err := s.titles.ShowByTMDBID(ctx, tmdbID)
	if err != nil {
		return nil, err
	}
	if show != nil {
		full, err := s.shows.GetWithSeasonsAndEpisodesContext(ctx, show.ID)
		if err != nil {
			return nil, err
		}
		if full != nil {
			return s.libraryShow(ctx, full), nil
		}
		// Removed meanwhile: it's a show that isn't in the library.
	}

	ts, err := TMDBShowCatalog(ctx, s.tmdb, tmdbID)
	if err != nil {
		return nil, err
	}
	d, cat := ts.Details, ts.Catalog
	return &TitleContext{
		MediaType:     library.JobTV,
		TMDBID:        tmdbID,
		Title:         cat.Title,
		OriginalTitle: cat.OriginalTitle,
		Aliases:       cat.Aliases,
		Romaji:        romajiTitle(ts.Extras.AlternativeTitles, cat.Title),
		Year:          cat.Year,
		IMDbID:        d.ExternalIDs.IMDbID,
		TVDBID:        d.ExternalIDs.TVDBID,
		Runtime:       ts.Runtime,
		IsAnime:       cat.IsAnime,
		Catalog:       cat,
		EpisodeIDs:    EpisodeIDsTMDB,
	}, nil
}

// libraryShow is the context of a show loaded with its seasons, episodes
// and files. Missing is computed as the title page computes it.
func (s *TitleContexts) libraryShow(ctx context.Context, show *library.Show) *TitleContext {
	extras, _ := loadCatalogExtras(ctx, s.tmdb, show.TMDBID, show.EpisodeGroupID, s.log.With("show_id", show.ID))
	cat := ShowCatalog(show, extras)
	id := show.ID
	tc := &TitleContext{
		MediaType:     library.JobTV,
		TMDBID:        show.TMDBID,
		LibraryID:     &id,
		Title:         cat.Title,
		OriginalTitle: cat.OriginalTitle,
		Aliases:       cat.Aliases,
		Romaji:        romajiTitle(extras.AlternativeTitles, cat.Title),
		Year:          show.Year,
		IMDbID:        show.IMDbID,
		TVDBID:        show.TVDBID,
		IsAnime:       show.IsAnime,
		Catalog:       cat,
		EpisodeIDs:    EpisodeIDsLibrary,
		Files:         map[int64]string{},
	}

	var runtimes []int
	facts := make([]catalog.SeasonFacts, 0, len(show.Seasons))
	for _, season := range show.Seasons {
		f := catalog.SeasonFacts{Number: season.SeasonNumber, Name: season.Name}
		for _, ep := range season.Episodes {
			if ep.Assignment != nil {
				tc.Files[ep.ID] = fileSource(ep.Assignment)
			}
			if season.SeasonNumber > 0 && ep.Runtime > 0 {
				runtimes = append(runtimes, ep.Runtime)
			}
			f.Episodes = append(f.Episodes, catalog.EpisodeFact{
				Number:   ep.EpisodeNumber,
				Absolute: ep.AbsoluteNumber,
				AirDate:  ep.AirDate,
				HasFile:  ep.Assignment != nil,
			})
		}
		facts = append(facts, f)
	}
	tc.Runtime = median(runtimes)
	_, tc.Missing = catalog.FirstMissingRun(facts, s.now().Format(time.DateOnly))
	return tc
}

// TMDBShow is a show read from TMDB alone, for one that isn't in the
// library.
type TMDBShow struct {
	Details *tmdb.ShowDetails
	// Catalog's episode ids are TMDB episode ids.
	Catalog *identify.ShowCatalog
	// Extras are the alternative titles and episode groups in Catalog.
	Extras CatalogExtras
	// Runtime is the median runtime of the regular episodes in minutes,
	// else TMDB's episode_run_time; 0 when unknown.
	Runtime int
}

// TMDBShowCatalog builds the classifier's catalog of a show from cached
// TMDB, the way the library would hold it once added: every season TMDB
// lists (specials included), absolute numbers from metadata.AbsoluteNumbers
// (anime from TMDB's absolute episode group, others cumulative), and the
// aliases and alternative orderings ShowCatalog takes. Episode ids are
// TMDB's.
//
// TMDB calls, all cached: the details, each season (seasonFetchers at a
// time), the alternative titles, the episode group list, up to
// maxEpisodeGroups season-like groups and, for anime, the absolute group.
// A season TMDB doesn't have is skipped; any other failure of the details
// or a season fails the build (see ErrTitleNotFound). Missing extras or an
// unavailable absolute group are left out, as for a library show.
func TMDBShowCatalog(ctx context.Context, src TMDBShowSource, tmdbID int) (*TMDBShow, error) {
	log := slog.With("component", "title-context", "tmdb_id", tmdbID)
	details, err := src.ShowDetails(ctx, tmdbID)
	if err != nil {
		return nil, titleTMDBError(ctx, err)
	}
	isAnime := metadata.IsAnime(details.Genres, details.OriginalLanguage, details.OriginCountry, details.Keywords.Keywords)
	ordering := metadata.DefaultOrdering(isAnime)

	var (
		wg     sync.WaitGroup
		extras CatalogExtras
		groups []tmdb.EpisodeGroupSummary
	)
	wg.Add(1)
	go func() {
		defer wg.Done()
		extras, groups = loadCatalogExtras(ctx, src, tmdbID, "", log)
	}()
	seasons, err := tmdbSeasons(ctx, src, details, log)
	wg.Wait()
	if err != nil {
		return nil, err
	}

	var group *tmdb.EpisodeGroup
	if id := metadata.NumberingGroup(ordering, "", groups); id != "" {
		if group, err = src.EpisodeGroup(ctx, id); err != nil {
			log.Warn("Absolute episode group unavailable; numbering cumulatively", "group", id, "error", err)
			group = nil
		}
	}

	show := &library.Show{
		TMDBID:          details.ID,
		Title:           details.Name,
		Year:            details.Year(),
		IMDbID:          details.ExternalIDs.IMDbID,
		TVDBID:          details.ExternalIDs.TVDBID,
		OriginalName:    details.OriginalName,
		OriginCountry:   details.OriginCountry,
		IsAnime:         isAnime,
		EpisodeOrdering: ordering,
		IncludeSpecials: true,
	}
	var refs []metadata.NumberedEpisode
	var runtimes []int
	for _, season := range seasons {
		ls := library.Season{SeasonNumber: season.SeasonNumber, Name: season.Name}
		for _, ep := range season.Episodes {
			ls.Episodes = append(ls.Episodes, library.Episode{
				ID:            int64(ep.ID),
				EpisodeNumber: ep.EpisodeNumber,
				Name:          ep.Name,
				AirDate:       ep.AirDate,
				TMDBEpisodeID: ep.ID,
				EpisodeType:   ep.EpisodeType,
				Runtime:       ep.Runtime,
			})
			refs = append(refs, metadata.NumberedEpisode{
				ID: int64(ep.ID), SeasonNumber: season.SeasonNumber, EpisodeNumber: ep.EpisodeNumber, TMDBEpisodeID: ep.ID,
			})
			if season.SeasonNumber > 0 && ep.Runtime > 0 {
				runtimes = append(runtimes, ep.Runtime)
			}
		}
		show.Seasons = append(show.Seasons, ls)
	}
	numbers := metadata.AbsoluteNumbers(refs, group)
	for i := range show.Seasons {
		for j := range show.Seasons[i].Episodes {
			ep := &show.Seasons[i].Episodes[j]
			if n, ok := numbers[ep.ID]; ok {
				ep.AbsoluteNumber = &n
			}
		}
	}

	runtime := median(runtimes)
	if runtime == 0 {
		for _, m := range details.EpisodeRunTime {
			if m > 0 {
				runtime = m
				break
			}
		}
	}
	return &TMDBShow{Details: details, Catalog: ShowCatalog(show, extras), Extras: extras, Runtime: runtime}, nil
}

// tmdbSeasons reads every season the details list, in number order, with
// their season number set. A season TMDB doesn't have is skipped.
func tmdbSeasons(ctx context.Context, src TMDBShowSource, details *tmdb.ShowDetails, log *slog.Logger) ([]*tmdb.Season, error) {
	numbers := details.SeasonNumbers()
	sort.Ints(numbers)
	fetched := make([]*tmdb.Season, len(numbers))
	errs := make([]error, len(numbers))

	var wg sync.WaitGroup
	sem := make(chan struct{}, seasonFetchers)
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
			fetched[i], errs[i] = src.Season(ctx, details.ID, n, details.Status)
		}()
	}
	wg.Wait()

	seasons := make([]*tmdb.Season, 0, len(numbers))
	for i, n := range numbers {
		switch {
		case errors.Is(errs[i], tmdb.ErrNotFound):
			log.Warn("TMDB lists a season it doesn't have; skipping it", "season", n)
			continue
		case errs[i] != nil:
			return nil, titleTMDBError(ctx, fmt.Errorf("season %d: %w", n, errs[i]))
		case fetched[i] == nil:
			continue
		}
		fetched[i].SeasonNumber = n
		seasons = append(seasons, fetched[i])
	}
	return seasons, nil
}

// titleTMDBError classifies a TMDB failure for a title that isn't in the
// library. The caller's own cancellation stays as it is.
func titleTMDBError(ctx context.Context, err error) error {
	switch {
	case ctx.Err() != nil:
		return ctx.Err()
	case errors.Is(err, tmdb.ErrNotFound):
		return fmt.Errorf("%w: %w", ErrTitleNotFound, err)
	}
	return fmt.Errorf("%w: %w", library.ErrTMDBFetch, err)
}

// romajiTitle picks the romaji among TMDB's alternative titles: the one
// typed "Romaji", else the first Japanese one in Latin letters. "" when
// there's none or it's the title.
func romajiTitle(titles []tmdb.AlternativeTitle, title string) string {
	pick := ""
	for _, t := range titles {
		if strings.EqualFold(t.Type, "romaji") && hasLatinLetter(t.Title) {
			pick = t.Title
			break
		}
	}
	if pick == "" {
		for _, t := range titles {
			if t.Country == "JP" && hasLatinLetter(t.Title) {
				pick = t.Title
				break
			}
		}
	}
	pick = strings.TrimSpace(pick)
	if strings.EqualFold(pick, strings.TrimSpace(title)) {
		return ""
	}
	return pick
}

// fileSource is where an active file comes from: "torrent" or "hdrezka".
func fileSource(a *library.TorrentAssignment) string {
	if a.IsTorrent() {
		return catalog.SourceTorrent
	}
	return string(a.Kind)
}

// median is the median of runtimes (the upper one of an even count), 0
// without any.
func median(runtimes []int) int {
	if len(runtimes) == 0 {
		return 0
	}
	sorted := append([]int(nil), runtimes...)
	sort.Ints(sorted)
	return sorted[len(sorted)/2]
}
