package service

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/shapedtime/momoshtrem/internal/hdrezka"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/tmdb"
)

// Fakes for the HDRezka service tests: a site, a resolver and the library.

const (
	interstellarPath = "/films/fiction/2259-interstellar-2014.html"
	breakingBadPath  = "/series/thriller/646-vo-vse-tyazhkie-2008-latest.html"
	bigShowPath      = "/series/drama/999-big-show-2020.html"
	interstellarTMDB = 157336
	breakingBadTMDB  = 1396
)

// fakeSite is HDRezka: title pages, translators' episodes, searches and
// file sizes, recording every call.
type fakeSite struct {
	mu          sync.Mutex
	titles      map[string]*hdrezka.Title
	titleErr    error
	episodes    map[string][]hdrezka.Season // by "path#translator"
	episodesErr error
	quick       map[string][]hdrezka.SearchResult
	full        map[string][]hdrezka.SearchResult
	searchErr   error
	// size answers ContentLength; nil: sizeOf.
	size    func(ctx context.Context, link string) (int64, error)
	pingErr error
	calls   []string
}

func (f *fakeSite) record(call string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, call)
}

// called counts the calls that start with prefix.
func (f *fakeSite) called(prefix string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.calls {
		if strings.HasPrefix(c, prefix) {
			n++
		}
	}
	return n
}

func (f *fakeSite) BaseURL() string { return "https://rezka.example" }

func (f *fakeSite) QuickSearch(_ context.Context, q string) ([]hdrezka.SearchResult, error) {
	f.record("quick:" + q)
	return f.quick[q], f.searchErr
}

func (f *fakeSite) Search(_ context.Context, q string) ([]hdrezka.SearchResult, error) {
	f.record("full:" + q)
	return f.full[q], f.searchErr
}

func (f *fakeSite) Title(_ context.Context, pathOrURL string) (*hdrezka.Title, error) {
	f.record("title:" + pathOrURL)
	if f.titleErr != nil {
		return nil, f.titleErr
	}
	t := f.titles[pathOrURL]
	if t == nil {
		return nil, hdrezka.ErrNotFound
	}
	return t, nil
}

func (f *fakeSite) Episodes(_ context.Context, ref hdrezka.Ref, translatorID int) ([]hdrezka.Season, error) {
	f.record(fmt.Sprintf("episodes:%d", translatorID))
	if f.episodesErr != nil {
		return nil, f.episodesErr
	}
	return f.episodes[fmt.Sprintf("%s#%d", ref.Path, translatorID)], nil
}

func (f *fakeSite) ContentLength(ctx context.Context, link string) (int64, error) {
	f.record("size:" + link)
	if f.size != nil {
		return f.size(ctx, link)
	}
	return sizeOf(link), nil
}

func (f *fakeSite) Ping(context.Context) error { return f.pingErr }

// sizeOf is a test link's size: by quality, plus the episode.
func sizeOf(link string) int64 {
	var post, translator, season, episode, version int
	var label string
	fmt.Sscanf(strings.ReplaceAll(strings.TrimPrefix(link, "https://cdn.example/"), "/", " "), "%d t%d s%de%d %s v%d.mp4",
		&post, &translator, &season, &episode, &label, &version)
	base := map[string]int64{"360p": 100_000, "720p": 200_000, "1080p": 300_000}[label]
	return base + int64(episode)
}

// fakeStreams is the resolver: every key resolves to testStream, but for
// the errors set by (season, episode). Invalidate bumps the key's links'
// version. It tracks how many resolves run at once.
type fakeStreams struct {
	mu          sync.Mutex
	errs        map[[2]int]error
	versions    map[hdrezka.StreamKey]int
	keys        []hdrezka.StreamKey
	invalidated int
	delay       time.Duration
	running     int
	maxRunning  int
}

func (f *fakeStreams) Resolve(ctx context.Context, key hdrezka.StreamKey) (*hdrezka.Stream, error) {
	f.mu.Lock()
	f.keys = append(f.keys, key)
	f.running++
	f.maxRunning = max(f.maxRunning, f.running)
	err := f.errs[[2]int{key.Season, key.Episode}]
	version := f.versions[key]
	f.mu.Unlock()
	defer func() {
		f.mu.Lock()
		f.running--
		f.mu.Unlock()
	}()
	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if err != nil {
		return nil, err
	}
	return testStream(key, version), nil
}

func (f *fakeStreams) Invalidate(key hdrezka.StreamKey) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.versions == nil {
		f.versions = map[hdrezka.StreamKey]int{}
	}
	f.versions[key]++
	f.invalidated++
}

// testStream offers 360p, 720p and 1080p, and premium 1080p Ultra and 4K.
func testStream(key hdrezka.StreamKey, version int) *hdrezka.Stream {
	link := func(label string) string {
		return fmt.Sprintf("https://cdn.example/%d/t%d/s%de%d/%s/v%d.mp4", key.PostID, key.TranslatorID, key.Season, key.Episode, label, version)
	}
	return &hdrezka.Stream{
		Qualities: []hdrezka.Quality{
			{Label: "360p", Resolution: 360, MP4: link("360p")},
			{Label: "720p", Resolution: 720, MP4: link("720p")},
			{Label: "1080p", Resolution: 1080, MP4: link("1080p")},
			{Label: "1080p Ultra", Resolution: 1080, Premium: true, MP4: link("ultra")},
			{Label: "4K", Resolution: 2160, Premium: true, MP4: link("4k")},
		},
		Subtitles: []hdrezka.Subtitle{
			{Language: "English", Code: "en", URL: "https://cdn.example/sub/en.vtt"},
			{Language: "Ukrainian?", URL: "https://cdn.example/sub/x.vtt"},
		},
	}
}

type fakeMovieGetter map[int64]*library.Movie

func (f fakeMovieGetter) GetByID(id int64) (*library.Movie, error) { return f[id], nil }

type fakeEpisodeLister map[int64][]library.PickerSeason

func (f fakeEpisodeLister) EpisodeList(_ context.Context, showID int64) ([]library.PickerSeason, error) {
	seasons, ok := f[showID]
	if !ok {
		return nil, library.ErrShowNotFound
	}
	return seasons, nil
}

// fakeStreamWriter records writes.
type fakeStreamWriter struct {
	mu     sync.Mutex
	writes []library.AssignmentWrite
	err    error
}

func (f *fakeStreamWriter) WriteStream(_ context.Context, w library.AssignmentWrite) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	f.writes = append(f.writes, w)
	return nil
}

type fakeHDRezkaTMDB struct {
	movies map[int]*tmdb.MovieDetails
	shows  map[int]*tmdb.ShowDetails
	err    error
}

func (f *fakeHDRezkaTMDB) MovieDetails(_ context.Context, id int) (*tmdb.MovieDetails, error) {
	if f.err != nil {
		return nil, f.err
	}
	if d := f.movies[id]; d != nil {
		return d, nil
	}
	return nil, tmdb.ErrNotFound
}

func (f *fakeHDRezkaTMDB) ShowDetails(_ context.Context, id int) (*tmdb.ShowDetails, error) {
	if f.err != nil {
		return nil, f.err
	}
	if d := f.shows[id]; d != nil {
		return d, nil
	}
	return nil, tmdb.ErrNotFound
}

// hdrezkaEnv is an HDRezkaService over the fakes: Interstellar (a movie
// with a director's cut of translator 56 and a premium translator),
// Breaking Bad (show 5 in the library) and a 40-episode show (9).
type hdrezkaEnv struct {
	site    *fakeSite
	streams *fakeStreams
	writer  *fakeStreamWriter
	tree    *countingTree
	tmdb    *fakeHDRezkaTMDB
	shows   fakeEpisodeLister
	svc     *HDRezkaService
}

func newHDRezkaEnv() *hdrezkaEnv {
	seasons := func(counts ...int) []hdrezka.Season {
		var out []hdrezka.Season
		for i, n := range counts {
			s := hdrezka.Season{Number: i + 1}
			for e := 1; e <= n; e++ {
				s.Episodes = append(s.Episodes, e)
			}
			out = append(out, s)
		}
		return out
	}
	site := &fakeSite{
		titles: map[string]*hdrezka.Title{
			interstellarPath: {Path: interstellarPath, PostID: 2259, Kind: hdrezka.KindMovie, Category: "films",
				Title: "Интерстеллар", OriginalTitle: "Interstellar", Year: 2014,
				Translators: []hdrezka.Translator{
					{ID: 56, Name: "Дубляж", Default: true},
					{ID: 56, Name: "Дубляж", Director: true},
					{ID: 238, Name: "Оригинал (+субтитры)"},
					{ID: 111, Name: "Premium Studio", Premium: true},
				}},
			breakingBadPath: {Path: breakingBadPath, PostID: 646, Kind: hdrezka.KindSeries, Category: "series",
				Title: "Во все тяжкие", OriginalTitle: "Breaking Bad", Year: 2008,
				Translators: []hdrezka.Translator{
					{ID: 56, Name: "LostFilm", Default: true},
					{ID: 238, Name: "Оригинал (+субтитры)"},
					{ID: 238, Name: "Оригинал (+субтитры)", Ads: true},
					{ID: 111, Name: "Premium Studio", Premium: true},
				},
				Seasons: seasons(7, 13)},
			bigShowPath: {Path: bigShowPath, PostID: 999, Kind: hdrezka.KindSeries, Category: "series", Title: "Big Show",
				Translators: []hdrezka.Translator{{ID: 1, Name: "Original", Default: true}}, Seasons: seasons(40)},
		},
		episodes: map[string][]hdrezka.Season{
			breakingBadPath + "#238": seasons(3, 1),
		},
	}

	streamOf := func(translator int, name, quality string) *library.StreamRef {
		return &library.StreamRef{Provider: "hdrezka", TranslatorID: translator, TranslatorName: name, Quality: quality}
	}
	bb := []library.PickerSeason{
		{Number: 0, Name: "Specials", Episodes: []library.PickerEpisode{{ID: 500, Number: 1, Name: "Special"}}},
		{Number: 1, Name: "Season 1", Episodes: []library.PickerEpisode{
			{ID: 501, Number: 1, Name: "Pilot", AirDate: "2008-01-20", HasFile: true, Source: library.AssignmentKindHDRezka,
				Stream: streamOf(56, "LostFilm", "720p")},
			{ID: 502, Number: 2, Name: "Cat's in the Bag...", HasFile: true, Source: library.AssignmentKindTorrent},
			{ID: 503, Number: 3, Name: "...And the Bag's in the River"},
		}},
	}
	var big []library.PickerEpisode
	for e := 1; e <= 40; e++ {
		big = append(big, library.PickerEpisode{ID: int64(9000 + e), Number: e})
	}
	shows := fakeEpisodeLister{5: bb, 9: {{Number: 1, Episodes: big}}}

	e := &hdrezkaEnv{
		site:    site,
		streams: &fakeStreams{},
		writer:  &fakeStreamWriter{},
		tree:    &countingTree{},
		tmdb: &fakeHDRezkaTMDB{
			movies: map[int]*tmdb.MovieDetails{interstellarTMDB: {ID: interstellarTMDB, Title: "Interstellar",
				OriginalTitle: "Interstellar", ReleaseDate: "2014-11-05"}},
			shows: map[int]*tmdb.ShowDetails{breakingBadTMDB: {ID: breakingBadTMDB, Name: "Breaking Bad",
				OriginalName: "Breaking Bad", FirstAirDate: "2008-01-20"}},
		},
		shows: shows,
	}
	e.svc = NewHDRezkaService(HDRezkaDeps{
		Site: site, Resolver: e.streams, TMDB: e.tmdb,
		Movies: fakeMovieGetter{7: {ID: 7, Title: "Interstellar", Year: 2014}},
		Shows:  shows, Assignments: e.writer, Tree: e.tree,
	})
	return e
}
