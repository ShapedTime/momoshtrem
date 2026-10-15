package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/shapedtime/momoshtrem/internal/catalog"
	"github.com/shapedtime/momoshtrem/internal/hdrezka"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/tmdb"
)

// Error codes of the HDRezka routes (PLAN §6 "HDRezka (#63)"). A show
// assign's failed episodes carry them too. None is ever a 401 or 403: the
// web proxy reads those as a bad backend token.
const (
	HDRezkaBadRequest  = "bad_request"         // 400: a malformed request
	HDRezkaNotFound    = "not_found"           // 404
	HDRezkaPremiumOnly = "premium_only"        // 409: it needs an HDRezka Premium account
	HDRezkaInvalid     = "validation_error"    // 422: well-formed, but it can't be done
	HDRezkaUnavailable = "hdrezka_unavailable" // 502: HDRezka or its CDN failed
	HDRezkaTMDBDown    = "tmdb_unavailable"    // 502: TMDB failed and nothing was cached
	HDRezkaBlocked     = "hdrezka_blocked"     // 503: HDRezka answered with its anti-bot page
	HDRezkaOff         = "unavailable"         // 503: a dependency isn't configured
	HDRezkaTimeout     = "timeout"             // 504
)

// HDRezkaError is an HDRezka request that failed for a reason the API
// answers with Code. Message is shown to the user: it never carries a
// signed link.
type HDRezkaError struct {
	Code    string
	Message string
	Err     error // the cause, for logs; nil for validation
}

func (e *HDRezkaError) Error() string { return e.Message }
func (e *HDRezkaError) Unwrap() error { return e.Err }

func hdrezkaErr(code, format string, args ...any) *HDRezkaError {
	return &HDRezkaError{Code: code, Message: fmt.Sprintf(format, args...)}
}

const (
	// MaxHDRezkaItems caps a show assign; the web sends bigger seasons in
	// chunks.
	MaxHDRezkaItems = 30
	// hdrezkaAssignBudget bounds an assign's network work, under the web
	// proxy's 30 s.
	hdrezkaAssignBudget = 25 * time.Second
	// hdrezkaAssignWorkers is how many episodes an assign resolves at once.
	hdrezkaAssignWorkers = 4
	// hdrezkaSizeBudget bounds measuring a stream's qualities for
	// GET /hdrezka/streams; a size not in by then is null.
	hdrezkaSizeBudget = 8 * time.Second
	// hdrezkaWriteTimeout bounds the write after an assign, which runs
	// even when the client has gone: the work is done.
	hdrezkaWriteTimeout = 10 * time.Second
	// maxHDRezkaQuery caps a search query.
	maxHDRezkaQuery = 200
)

// HDRezkaSite is the HDRezka client. *hdrezka.Client implements it.
type HDRezkaSite interface {
	BaseURL() string
	QuickSearch(ctx context.Context, query string) ([]hdrezka.SearchResult, error)
	Search(ctx context.Context, query string) ([]hdrezka.SearchResult, error)
	Title(ctx context.Context, pathOrURL string) (*hdrezka.Title, error)
	Episodes(ctx context.Context, ref hdrezka.Ref, translatorID int) ([]hdrezka.Season, error)
	ContentLength(ctx context.Context, link string) (int64, error)
	Ping(ctx context.Context) error
}

var _ HDRezkaSite = (*hdrezka.Client)(nil)

// HDRezkaResolver resolves streams to their links, cached.
// *hdrezka.Resolver implements it.
type HDRezkaResolver interface {
	Resolve(ctx context.Context, key hdrezka.StreamKey) (*hdrezka.Stream, error)
	Invalidate(key hdrezka.StreamKey)
}

var _ HDRezkaResolver = (*hdrezka.Resolver)(nil)

// HDRezkaTMDB is the cached TMDB client, for building and ranking
// searches. *tmdb.Cached implements it.
type HDRezkaTMDB interface {
	MovieDetails(ctx context.Context, id int) (*tmdb.MovieDetails, error)
	ShowDetails(ctx context.Context, id int) (*tmdb.ShowDetails, error)
}

var _ HDRezkaTMDB = (*tmdb.Cached)(nil)

// MovieGetter finds a library movie; nil when there's none.
// *library.MovieRepository implements it.
type MovieGetter interface {
	GetByID(id int64) (*library.Movie, error)
}

// EpisodeLister lists a show's episodes with their files.
// *library.ShowRepository implements it.
type EpisodeLister interface {
	EpisodeList(ctx context.Context, showID int64) ([]library.PickerSeason, error)
}

// StreamWriter makes HDRezka streams items' files.
// *library.AssignmentRepository implements it.
type StreamWriter interface {
	WriteStream(ctx context.Context, w library.AssignmentWrite) error
}

var (
	_ MovieGetter   = (*library.MovieRepository)(nil)
	_ EpisodeLister = (*library.ShowRepository)(nil)
	_ StreamWriter  = (*library.AssignmentRepository)(nil)
)

// HDRezkaDeps are an HDRezkaService's dependencies, all required.
type HDRezkaDeps struct {
	Site        HDRezkaSite
	Resolver    HDRezkaResolver
	TMDB        HDRezkaTMDB
	Movies      MovieGetter
	Shows       EpisodeLister
	Assignments StreamWriter
	// Tree is rebuilt once after each assign.
	Tree TreeInvalidator
}

// HDRezkaService finds titles on HDRezka and makes their streams library
// files (PLAN §6 "HDRezka (#63)"). Assigning is synchronous: it resolves
// and measures each stream, then writes once and rebuilds the tree once.
type HDRezkaService struct {
	site        HDRezkaSite
	resolver    HDRezkaResolver
	tmdb        HDRezkaTMDB
	movies      MovieGetter
	shows       EpisodeLister
	assignments StreamWriter
	tree        TreeInvalidator
	log         *slog.Logger

	// The budgets; tests shorten them.
	assignBudget time.Duration
	sizeBudget   time.Duration
}

// NewHDRezkaService creates an HDRezkaService.
func NewHDRezkaService(d HDRezkaDeps) *HDRezkaService {
	return &HDRezkaService{
		site: d.Site, resolver: d.Resolver, tmdb: d.TMDB,
		movies: d.Movies, shows: d.Shows, assignments: d.Assignments, tree: d.Tree,
		log:          slog.With("component", "hdrezka"),
		assignBudget: hdrezkaAssignBudget,
		sizeBudget:   hdrezkaSizeBudget,
	}
}

// Ping checks the mirror for GET /status.
func (s *HDRezkaService) Ping(ctx context.Context) error {
	err := s.site.Ping(ctx)
	if errors.Is(err, hdrezka.ErrChallenge) {
		return errors.New("HDRezka asks for a browser check")
	}
	return err
}

// --- Search ---

// HDRezkaSearch is GET /hdrezka/search.
type HDRezkaSearch struct {
	Items []HDRezkaSearchItem `json:"items"`
	Query string              `json:"query"` // the query that found them
}

// HDRezkaSearchItem is one hit, graded against the TMDB title.
type HDRezkaSearchItem struct {
	URL           string   `json:"url"`  // on the configured mirror
	Path          string   `json:"path"` // the title page, without the host
	Title         string   `json:"title"`
	OriginalTitle *string  `json:"original_title"`
	Year          *int     `json:"year"`
	Kind          *string  `json:"kind"` // movie or series; null when the category doesn't tell
	Category      string   `json:"category"`
	Rating        *float64 `json:"rating"`
	Match         string   `json:"match"` // exact, likely or weak
}

// Search finds a library title on HDRezka: q, else the TMDB original title,
// else its title (the next is tried when one finds nothing). Each query
// runs the quick search, then the full search when that finds nothing.
// Hits are graded against TMDB's title, original title, year and kind;
// when TMDB fails and q was given, against q alone.
func (s *HDRezkaService) Search(ctx context.Context, mediaType string, tmdbID int, q string) (*HDRezkaSearch, error) {
	var kind hdrezka.Kind
	switch mediaType {
	case "movie":
		kind = hdrezka.KindMovie
	case "tv":
		kind = hdrezka.KindSeries
	default:
		return nil, hdrezkaErr(HDRezkaBadRequest, "media_type must be movie or tv.")
	}
	if tmdbID <= 0 {
		return nil, hdrezkaErr(HDRezkaBadRequest, "tmdb_id must be a positive number.")
	}
	q = strings.TrimSpace(q)
	if len([]rune(q)) > maxHDRezkaQuery {
		return nil, hdrezkaErr(HDRezkaBadRequest, "q is over %d characters.", maxHDRezkaQuery)
	}

	want, err := s.want(ctx, kind, tmdbID)
	if err != nil {
		if q == "" || errors.Is(err, context.Canceled) {
			return nil, tmdbFailure(err)
		}
		s.log.Warn("TMDB unavailable; ranking the HDRezka search against the query", "tmdb_id", tmdbID, "error", err)
		want = hdrezka.Want{Title: q, Kind: kind}
	}
	queries := []string{q}
	if q == "" {
		queries = searchQueries(want.OriginalTitle, want.Title)
	}

	out := &HDRezkaSearch{Items: []HDRezkaSearchItem{}}
	var results []hdrezka.SearchResult
	for _, query := range queries {
		out.Query = query
		results, err = s.site.QuickSearch(ctx, query)
		if err == nil && len(results) == 0 {
			results, err = s.site.Search(ctx, query)
		}
		if err != nil {
			return nil, siteFailure(err)
		}
		if len(results) > 0 {
			break
		}
	}
	for _, m := range hdrezka.Rank(results, want) {
		out.Items = append(out.Items, HDRezkaSearchItem{
			URL:           s.site.BaseURL() + m.Path,
			Path:          m.Path,
			Title:         m.Title,
			OriginalTitle: nonEmpty(m.OriginalTitle),
			Year:          positiveInt(m.Year),
			Kind:          nonEmpty(string(m.Kind)),
			Category:      m.Category,
			Rating:        positiveFloat(m.Rating),
			Match:         string(m.Grade),
		})
	}
	return out, nil
}

// want is the title a search is for, from cached TMDB.
func (s *HDRezkaService) want(ctx context.Context, kind hdrezka.Kind, tmdbID int) (hdrezka.Want, error) {
	if kind == hdrezka.KindMovie {
		d, err := s.tmdb.MovieDetails(ctx, tmdbID)
		if err != nil {
			return hdrezka.Want{}, err
		}
		return hdrezka.Want{Title: d.Title, OriginalTitle: d.OriginalTitle, Year: d.Year(), Kind: kind}, nil
	}
	d, err := s.tmdb.ShowDetails(ctx, tmdbID)
	if err != nil {
		return hdrezka.Want{}, err
	}
	return hdrezka.Want{Title: d.Name, OriginalTitle: d.OriginalName, Year: d.Year(), Kind: kind}, nil
}

// searchQueries is the original title, then the title when it differs.
func searchQueries(titles ...string) []string {
	var out []string
	for _, t := range titles {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		dup := false
		for _, o := range out {
			dup = dup || strings.EqualFold(o, t)
		}
		if !dup {
			out = append(out, t)
		}
	}
	return out
}

// --- Title ---

// HDRezkaTitleRequest is GET /hdrezka/titles.
type HDRezkaTitleRequest struct {
	URL          string
	TranslatorID int   // 0: the page's default (unless it's premium)
	ShowID       int64 // 0: no library show to annotate the episodes with
	MovieID      int64
}

// HDRezkaTitle is a title page with its translators and, for a series,
// the seasons of one translator.
type HDRezkaTitle struct {
	URL           string              `json:"url"`
	Path          string              `json:"path"`
	PostID        int                 `json:"post_id"`
	Kind          string              `json:"kind"` // movie or series
	Title         string              `json:"title"`
	OriginalTitle *string             `json:"original_title"`
	Year          *int                `json:"year"`
	Translators   []HDRezkaTranslator `json:"translators"`
	TranslatorID  int                 `json:"translator_id"` // the one picked, and the seasons'
	Seasons       []HDRezkaSeason     `json:"seasons"`       // [] for a movie
}

// HDRezkaTranslator is a translation: a dub, a voice-over or the original
// audio.
type HDRezkaTranslator struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Default  bool   `json:"default"`
	Premium  bool   `json:"premium"`
	Camrip   bool   `json:"camrip"`
	Ads      bool   `json:"ads"`
	Director bool   `json:"director"`
}

// HDRezkaSeason is a season of the translator, in the site's order.
type HDRezkaSeason struct {
	Number   int              `json:"number"`
	Episodes []HDRezkaEpisode `json:"episodes"`
}

// HDRezkaEpisode is an HDRezka episode and the library episode with the
// same season and episode numbers.
type HDRezkaEpisode struct {
	Number  int                    `json:"number"`
	Library *HDRezkaLibraryEpisode `json:"library"`
}

// HDRezkaLibraryEpisode is a library episode and its current file.
type HDRezkaLibraryEpisode struct {
	EpisodeID int64                  `json:"episode_id"`
	Season    int                    `json:"season"`
	Episode   int                    `json:"episode"`
	Name      string                 `json:"name"`
	AirDate   *string                `json:"air_date"`
	HasFile   bool                   `json:"has_file"`
	Source    *string                `json:"source"` // torrent or hdrezka; null without a file
	Stream    *catalog.EpisodeStream `json:"stream"` // an HDRezka file's
}

// Title reads an HDRezka title page. For a series it lists the seasons of
// the translator asked for, else of the page's default (or, when that's
// premium, of the first that isn't), each episode with the show's library
// episode of the same numbers.
func (s *HDRezkaService) Title(ctx context.Context, req HDRezkaTitleRequest) (*HDRezkaTitle, error) {
	path, err := titlePath(req.URL)
	if err != nil {
		return nil, err
	}
	if req.TranslatorID < 0 || req.ShowID < 0 || req.MovieID < 0 {
		return nil, hdrezkaErr(HDRezkaBadRequest, "translator_id, show_id and movie_id must be positive.")
	}
	if req.ShowID > 0 && req.MovieID > 0 {
		return nil, hdrezkaErr(HDRezkaBadRequest, "Send show_id or movie_id, not both.")
	}
	var lib []library.PickerSeason
	if req.ShowID > 0 {
		if lib, err = s.shows.EpisodeList(ctx, req.ShowID); err != nil {
			return nil, libraryFailure(err)
		}
	}
	if req.MovieID > 0 {
		if err := s.movieExists(req.MovieID); err != nil {
			return nil, err
		}
	}

	t, err := s.site.Title(ctx, path)
	if err != nil {
		return nil, siteFailure(err)
	}
	tr, err := pickTranslator(t, req.TranslatorID, TranslatorCut{})
	if err != nil {
		return nil, err
	}
	out := &HDRezkaTitle{
		URL: s.site.BaseURL() + t.Path, Path: t.Path, PostID: t.PostID, Kind: string(t.Kind),
		Title: t.Title, OriginalTitle: nonEmpty(t.OriginalTitle), Year: positiveInt(t.Year),
		Translators: make([]HDRezkaTranslator, 0, len(t.Translators)), TranslatorID: tr.ID,
		Seasons: []HDRezkaSeason{},
	}
	for _, x := range t.Translators {
		out.Translators = append(out.Translators, HDRezkaTranslator{ID: x.ID, Name: x.Name, Default: x.Default,
			Premium: x.Premium, Camrip: x.Camrip, Ads: x.Ads, Director: x.Director})
	}
	if t.Kind == hdrezka.KindSeries {
		seasons, err := s.seasonsOf(ctx, t, tr)
		if err != nil {
			return nil, siteFailure(err)
		}
		out.Seasons = annotateSeasons(seasons, lib)
	}
	return out, nil
}

// TranslatorCut picks one cut of a translator id: HDRezka can list an id
// several times with different camrip, ads and director flags (a
// director's cut), and a movie's stream depends on them. A nil flag
// matches either way, so an empty cut is the id's first entry.
type TranslatorCut struct {
	Camrip   *bool
	Ads      *bool
	Director *bool
}

func (c TranslatorCut) matches(tr hdrezka.Translator) bool {
	same := func(want *bool, got bool) bool { return want == nil || *want == got }
	return same(c.Camrip, tr.Camrip) && same(c.Ads, tr.Ads) && same(c.Director, tr.Director)
}

func (c TranslatorCut) empty() bool { return c.Camrip == nil && c.Ads == nil && c.Director == nil }

// pickTranslator is the translator asked for: the first entry with the id
// whose flags match cut (a series ignores the cut: its episodes don't take
// the flags); 422 when the page has none. Without an id, the page's
// default unless it's premium, else the first that isn't, else the
// default.
func pickTranslator(t *hdrezka.Title, id int, cut TranslatorCut) (hdrezka.Translator, error) {
	if id != 0 {
		if t.Kind == hdrezka.KindSeries {
			cut = TranslatorCut{}
		}
		for _, tr := range t.Translators {
			if tr.ID == id && cut.matches(tr) {
				return tr, nil
			}
		}
		if cut.empty() {
			return hdrezka.Translator{}, hdrezkaErr(HDRezkaInvalid, "This HDRezka title has no translator %d.", id)
		}
		return hdrezka.Translator{}, hdrezkaErr(HDRezkaInvalid, "This HDRezka title has no translator %d with those camrip, ads and director flags.", id)
	}
	def, hasDefault := t.DefaultTranslator()
	if hasDefault && !def.Premium {
		return def, nil
	}
	for _, tr := range t.Translators {
		if !tr.Premium {
			return tr, nil
		}
	}
	if hasDefault || len(t.Translators) == 0 {
		return def, nil
	}
	return t.Translators[0], nil
}

// seasonsOf is a series' seasons and episodes in one translation: the
// page's own for its default, else asked from the CDN.
func (s *HDRezkaService) seasonsOf(ctx context.Context, t *hdrezka.Title, tr hdrezka.Translator) ([]hdrezka.Season, error) {
	if tr.Default && len(t.Seasons) > 0 {
		return t.Seasons, nil
	}
	return s.site.Episodes(ctx, t.Ref(), tr.ID)
}

// annotateSeasons pairs each HDRezka episode with the library episode of
// the same season and episode numbers.
func annotateSeasons(seasons []hdrezka.Season, lib []library.PickerSeason) []HDRezkaSeason {
	byNumber := map[[2]int]*HDRezkaLibraryEpisode{}
	for _, ls := range lib {
		for _, e := range ls.Episodes {
			le := &HDRezkaLibraryEpisode{EpisodeID: e.ID, Season: ls.Number, Episode: e.Number, Name: e.Name,
				AirDate: nonEmpty(e.AirDate), HasFile: e.HasFile}
			if e.HasFile {
				source := string(e.Source)
				if source == "" {
					source = catalog.SourceTorrent
				}
				le.Source = &source
			}
			if e.Stream != nil {
				le.Stream = &catalog.EpisodeStream{Translator: e.Stream.TranslatorName, Quality: e.Stream.Quality}
			}
			byNumber[[2]int{ls.Number, e.Number}] = le
		}
	}
	out := make([]HDRezkaSeason, 0, len(seasons))
	for _, season := range seasons {
		eps := make([]HDRezkaEpisode, 0, len(season.Episodes))
		for _, n := range season.Episodes {
			eps = append(eps, HDRezkaEpisode{Number: n, Library: byNumber[[2]int{season.Number, n}]})
		}
		out = append(out, HDRezkaSeason{Number: season.Number, Episodes: eps})
	}
	return out
}

// --- Streams ---

// HDRezkaStreamsRequest is GET /hdrezka/streams. Season and Episode pick
// a series' episode; without them it's the translator's first.
type HDRezkaStreamsRequest struct {
	URL          string
	TranslatorID int
	Cut          TranslatorCut // movies only
	Season       *int
	Episode      *int
}

// HDRezkaStreams is what a movie or an episode offers in one translation.
type HDRezkaStreams struct {
	Qualities []HDRezkaQuality  `json:"qualities"`
	Subtitles []HDRezkaSubtitle `json:"subtitles"`
}

// HDRezkaQuality is one rendition.
type HDRezkaQuality struct {
	Label      string `json:"label"`      // e.g. "1080p", "1080p Ultra"
	Resolution *int   `json:"resolution"` // lines; null when the label doesn't say
	Premium    bool   `json:"premium"`    // a teaser without an HDRezka Premium account
	// Available is whether it can be assigned: it has a progressive MP4
	// and isn't premium.
	Available bool   `json:"available"`
	SizeBytes *int64 `json:"size_bytes"` // null when premium or not measured in time
}

// HDRezkaSubtitle is a subtitle track of the stream.
type HDRezkaSubtitle struct {
	LanguageCode *string `json:"language_code"` // ISO 639-1; null when the site gives none
	LanguageName string  `json:"language_name"`
}

// Streams lists a movie's or an episode's qualities with their sizes
// (measured concurrently within 8 s) and its subtitle languages.
func (s *HDRezkaService) Streams(ctx context.Context, req HDRezkaStreamsRequest) (*HDRezkaStreams, error) {
	path, err := titlePath(req.URL)
	if err != nil {
		return nil, err
	}
	if req.TranslatorID <= 0 {
		return nil, hdrezkaErr(HDRezkaBadRequest, "translator_id is required.")
	}
	if (req.Season == nil) != (req.Episode == nil) || (req.Season != nil && (*req.Season < 0 || *req.Episode < 1)) {
		return nil, hdrezkaErr(HDRezkaBadRequest, "Send season and episode together (an episode from 1), or neither.")
	}
	t, err := s.site.Title(ctx, path)
	if err != nil {
		return nil, siteFailure(err)
	}
	tr, err := pickTranslator(t, req.TranslatorID, req.Cut)
	if err != nil {
		return nil, err
	}
	if tr.Premium {
		return nil, premiumErr()
	}
	key := streamKeyOf(t, tr, 0, 0)
	if t.Kind == hdrezka.KindSeries {
		if req.Season != nil {
			key.Season, key.Episode = *req.Season, *req.Episode
		} else {
			seasons, err := s.seasonsOf(ctx, t, tr)
			if err != nil {
				return nil, siteFailure(err)
			}
			if len(seasons) == 0 || len(seasons[0].Episodes) == 0 {
				return nil, hdrezkaErr(HDRezkaNotFound, "HDRezka has no episodes in this voice-over.")
			}
			key.Season, key.Episode = seasons[0].Number, seasons[0].Episodes[0]
		}
	}

	stream, err := s.resolver.Resolve(ctx, key)
	if err != nil {
		return nil, siteFailure(err)
	}
	sctx, cancel := context.WithTimeout(ctx, s.sizeBudget)
	defer cancel()
	sizes, refused := s.measureAll(sctx, stream)
	if refused { // links cached from an earlier resolve: try fresh ones
		s.resolver.Invalidate(key)
		if fresh, err := s.resolver.Resolve(sctx, key); err == nil {
			stream = fresh
			sizes, _ = s.measureAll(sctx, stream)
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	out := &HDRezkaStreams{Qualities: []HDRezkaQuality{}, Subtitles: []HDRezkaSubtitle{}}
	for i, q := range stream.Qualities {
		h := sizes[i]
		out.Qualities = append(out.Qualities, HDRezkaQuality{
			Label: q.Label, Resolution: positiveInt(q.Resolution), Premium: q.Premium,
			Available: !q.Premium && q.MP4 != "",
			SizeBytes: positiveInt64(h.size),
		})
	}
	for _, sub := range stream.Subtitles {
		out.Subtitles = append(out.Subtitles, HDRezkaSubtitle{LanguageCode: nonEmpty(sub.Code), LanguageName: sub.Language})
	}
	return out, nil
}

// sizeResult is a quality's measured size.
type sizeResult struct {
	size int64
	err  error
}

// measureAll measures every quality that isn't premium and has an MP4, at
// once, by index. refused reports a link the CDN refused.
func (s *HDRezkaService) measureAll(ctx context.Context, stream *hdrezka.Stream) (sizes []sizeResult, refused bool) {
	sizes = make([]sizeResult, len(stream.Qualities))
	var wg sync.WaitGroup
	for i, q := range stream.Qualities {
		if q.Premium || q.MP4 == "" {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			size, err := s.site.ContentLength(ctx, q.MP4)
			sizes[i] = sizeResult{size: size, err: err}
		}()
	}
	wg.Wait()
	for _, r := range sizes {
		refused = refused || errors.Is(r.err, hdrezka.ErrLinkExpired)
	}
	return sizes, refused
}

// titlePath validates a title URL (on any mirror) or path and keeps only
// the path: only {HDREZKA_URL}{path} is ever fetched.
func titlePath(raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", hdrezkaErr(HDRezkaBadRequest, "url is required.")
	}
	path, err := hdrezka.TitlePath(raw)
	if err != nil {
		return "", &HDRezkaError{Code: HDRezkaInvalid, Message: "That isn't an HDRezka title link.", Err: err}
	}
	return path, nil
}

func premiumErr() *HDRezkaError {
	return &HDRezkaError{Code: HDRezkaPremiumOnly, Err: hdrezka.ErrPremium,
		Message: "This needs an HDRezka Premium account; pick another voice-over or quality."}
}

// siteFailure turns an error from HDRezka or its CDN into an HDRezkaError,
// leaving a canceled request as it is (the client has gone).
func siteFailure(err error) error {
	if errors.Is(err, context.Canceled) {
		return err
	}
	return failureOf(err)
}

// failureOf classifies an error from HDRezka or its CDN.
func failureOf(err error) *HDRezkaError {
	var he *HDRezkaError
	if errors.As(err, &he) {
		return he
	}
	var netErr net.Error
	var cdn *hdrezka.CDNError
	fail := func(code, message string) *HDRezkaError { return &HDRezkaError{Code: code, Message: message, Err: err} }
	switch {
	case errors.Is(err, context.DeadlineExceeded), errors.As(err, &netErr) && netErr.Timeout():
		return fail(HDRezkaTimeout, "HDRezka didn't answer in time; try again.")
	case errors.Is(err, context.Canceled):
		return fail(HDRezkaTimeout, "The request was canceled.")
	case errors.Is(err, hdrezka.ErrChallenge):
		return fail(HDRezkaBlocked, "HDRezka asks for a browser check (an anti-bot page); try again later.")
	case errors.Is(err, hdrezka.ErrPremium):
		return premiumErr()
	case errors.Is(err, hdrezka.ErrInvalidPath):
		return fail(HDRezkaInvalid, "That isn't an HDRezka title link.")
	case errors.Is(err, hdrezka.ErrNotFound):
		return fail(HDRezkaNotFound, "HDRezka has nothing to play for this.")
	case errors.Is(err, hdrezka.ErrNoMP4):
		return fail(HDRezkaInvalid, "HDRezka offers this only as HLS, which can't be streamed; pick another voice-over.")
	case errors.Is(err, hdrezka.ErrNoQuality):
		return fail(HDRezkaInvalid, "HDRezka doesn't offer this quality here; pick another.")
	case errors.As(err, &cdn):
		return fail(HDRezkaUnavailable, cdn.Error())
	}
	var status *hdrezka.StatusError
	var parse *hdrezka.ParseError
	if errors.As(err, &status) || errors.As(err, &parse) || errors.Is(err, hdrezka.ErrLinkExpired) ||
		errors.Is(err, hdrezka.ErrRangeIgnored) || errors.Is(err, hdrezka.ErrSizeMismatch) {
		return fail(HDRezkaUnavailable, err.Error())
	}
	return fail(HDRezkaUnavailable, "Couldn't reach HDRezka: "+err.Error())
}

// tmdbFailure classifies a TMDB error from building a search.
func tmdbFailure(err error) error {
	switch {
	case errors.Is(err, context.Canceled):
		return err
	case errors.Is(err, tmdb.ErrNoAPIKey):
		return &HDRezkaError{Code: HDRezkaOff, Message: "TMDB isn't configured (TMDB_API_KEY); search with q instead.", Err: err}
	case errors.Is(err, tmdb.ErrNotFound):
		return &HDRezkaError{Code: HDRezkaNotFound, Message: "TMDB has no such title.", Err: err}
	}
	return &HDRezkaError{Code: HDRezkaTMDBDown, Message: "TMDB is unavailable; search with q instead.", Err: err}
}

// libraryFailure turns the library's not-found errors into 404s; other
// errors (the database) stay as they are.
func libraryFailure(err error) error {
	switch {
	case errors.Is(err, library.ErrShowNotFound):
		return &HDRezkaError{Code: HDRezkaNotFound, Message: "Show not found.", Err: err}
	case errors.Is(err, library.ErrMovieNotFound):
		return &HDRezkaError{Code: HDRezkaNotFound, Message: "Movie not found.", Err: err}
	}
	return err
}

func positiveInt64(n int64) *int64 {
	if n <= 0 {
		return nil
	}
	return &n
}

func positiveFloat(f float64) *float64 {
	if f <= 0 {
		return nil
	}
	return &f
}
