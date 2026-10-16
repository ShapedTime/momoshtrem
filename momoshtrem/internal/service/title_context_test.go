package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/shapedtime/momoshtrem/internal/catalog"
	"github.com/shapedtime/momoshtrem/internal/identify"
	"github.com/shapedtime/momoshtrem/internal/identify/corpus"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/tmdb"
)

const frierenTMDB = 209867

// fakeTitleStore is the library: movies and shows by TMDB id, and shows
// with their seasons by library id.
type fakeTitleStore struct {
	movies map[int]*library.Movie
	shows  map[int64]*library.Show
	err    error
}

func (f *fakeTitleStore) MovieByTMDBID(_ context.Context, tmdbID int) (*library.Movie, error) {
	return f.movies[tmdbID], f.err
}

func (f *fakeTitleStore) ShowByTMDBID(_ context.Context, tmdbID int) (*library.Show, error) {
	for _, s := range f.shows {
		if s.TMDBID == tmdbID {
			bare := *s
			bare.Seasons = nil
			return &bare, f.err
		}
	}
	return nil, f.err
}

func (f *fakeTitleStore) GetWithSeasonsAndEpisodesContext(_ context.Context, id int64) (*library.Show, error) {
	return f.shows[id], f.err
}

// fakeTitleTMDB is cached TMDB: details and seasons, plus the extras of
// fakeCatalogSource. err fails every details and season read.
type fakeTitleTMDB struct {
	fakeCatalogSource
	movies    map[int]*tmdb.MovieDetails
	shows     map[int]*tmdb.ShowDetails
	seasons   map[int]*tmdb.Season // by season number
	seasonErr map[int]error
	err       error

	mu    sync.Mutex
	calls int
}

func (f *fakeTitleTMDB) count() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
}

func (f *fakeTitleTMDB) MovieDetails(_ context.Context, id int) (*tmdb.MovieDetails, error) {
	f.count()
	if f.err != nil {
		return nil, f.err
	}
	if d := f.movies[id]; d != nil {
		return d, nil
	}
	return nil, tmdb.ErrNotFound
}

func (f *fakeTitleTMDB) ShowDetails(_ context.Context, id int) (*tmdb.ShowDetails, error) {
	f.count()
	if f.err != nil {
		return nil, f.err
	}
	if d := f.shows[id]; d != nil {
		return d, nil
	}
	return nil, tmdb.ErrNotFound
}

func (f *fakeTitleTMDB) Season(_ context.Context, _ int, n int, _ string) (*tmdb.Season, error) {
	f.count()
	if f.err != nil {
		return nil, f.err
	}
	if err := f.seasonErr[n]; err != nil {
		return nil, err
	}
	if s := f.seasons[n]; s != nil {
		c := *s
		return &c, nil
	}
	return nil, tmdb.ErrNotFound
}

// readJSON decodes a TMDB fixture from internal/tmdb/testdata.
func readJSON[T any](t *testing.T, name string) *T {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "tmdb", "testdata", name+".json"))
	if err != nil {
		t.Fatal(err)
	}
	v := new(T)
	if err := json.Unmarshal(data, v); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return v
}

// frierenSeason1 is TMDB's season 1 of Frieren, 38 episodes in two cours,
// from the classifier's show fixture. TMDB episode ids are 5000000 + n.
func frierenSeason1(t *testing.T) *tmdb.Season {
	t.Helper()
	shows, err := corpus.LoadShows(filepath.Join("..", "identify", "testdata"))
	if err != nil {
		t.Fatal(err)
	}
	season := &tmdb.Season{SeasonNumber: 1, Name: "Season 1"}
	for _, ep := range shows["frieren"].Episodes {
		if ep.Season == 1 {
			season.Episodes = append(season.Episodes, tmdb.Episode{
				ID: 5000000 + ep.Number, SeasonNumber: 1, EpisodeNumber: ep.Number, Name: ep.Name,
				AirDate: ep.AirDate, EpisodeType: ep.Type, Runtime: 24,
			})
		}
	}
	return season
}

func frierenTMDBFake(t *testing.T) *fakeTitleTMDB {
	t.Helper()
	return &fakeTitleTMDB{
		fakeCatalogSource: fakeCatalogSource{
			titles: readJSON[struct{ Results []tmdb.AlternativeTitle }](t, "tv_209867_alternative_titles").Results,
		},
		shows: map[int]*tmdb.ShowDetails{frierenTMDB: readJSON[tmdb.ShowDetails](t, "tv_209867")},
		seasons: map[int]*tmdb.Season{
			0: readJSON[tmdb.Season](t, "tv_209867_season_0"),
			1: frierenSeason1(t),
		},
	}
}

// libraryFrieren is Frieren in the library: specials, then twelve weekly
// episodes from 2023-09-29. E01 and E02 come from a torrent, E03 from
// HDRezka.
func libraryFrieren() *library.Show {
	show := &library.Show{
		ID: 7, TMDBID: frierenTMDB, Title: "Frieren: Beyond Journey's End", OriginalName: "葬送のフリーレン", Year: 2023,
		IMDbID: "tt22248376", TVDBID: 424536, IsAnime: true,
		EpisodeOrdering: library.OrderingAbsolute, IncludeSpecials: true,
		Seasons: []library.Season{{SeasonNumber: 0, Episodes: []library.Episode{{ID: 100, EpisodeNumber: 1, AirDate: "2023-10-06", Runtime: 3}}}},
	}
	first := time.Date(2023, 9, 29, 0, 0, 0, 0, time.UTC)
	season := library.Season{SeasonNumber: 1}
	for n := 1; n <= 12; n++ {
		abs := n
		ep := library.Episode{
			ID: int64(100 + n), EpisodeNumber: n, AbsoluteNumber: &abs, Runtime: 24,
			AirDate: first.AddDate(0, 0, 7*(n-1)).Format(time.DateOnly),
		}
		switch n {
		case 1, 2:
			ep.Assignment = &library.TorrentAssignment{Kind: library.AssignmentKindTorrent, InfoHash: "abc", FilePath: fmt.Sprintf("Frieren/E%02d.mkv", n)}
		case 3:
			ep.Assignment = &library.TorrentAssignment{Kind: library.AssignmentKindHDRezka, Stream: &library.StreamRef{Provider: "hdrezka", Season: 1, Episode: 3}}
		case 4:
			ep.Runtime = 25
		}
		season.Episodes = append(season.Episodes, ep)
	}
	show.Seasons = append(show.Seasons, season)
	return show
}

func newTestTitleContexts(store *fakeTitleStore, src *fakeTitleTMDB, today string) *TitleContexts {
	now, _ := time.Parse(time.DateOnly, today)
	return NewTitleContexts(TitleContextDeps{Titles: store, Shows: store, TMDB: src, Now: func() time.Time { return now }})
}

func TestTitleContextLibraryShow(t *testing.T) {
	store := &fakeTitleStore{shows: map[int64]*library.Show{7: libraryFrieren()}}
	src := frierenTMDBFake(t)
	tc, err := newTestTitleContexts(store, src, "2023-12-01").Get(t.Context(), library.JobTV, frierenTMDB)
	if err != nil {
		t.Fatal(err)
	}

	// The library has the show: TMDB is read for the aliases and episode
	// groups only.
	if src.calls != 0 {
		t.Errorf("%d TMDB details or season reads, want none", src.calls)
	}
	if tc.LibraryID == nil || *tc.LibraryID != 7 || tc.EpisodeIDs != EpisodeIDsLibrary {
		t.Errorf("library id %v, episode ids %q", tc.LibraryID, tc.EpisodeIDs)
	}
	if tc.Title != "Frieren: Beyond Journey's End" || tc.OriginalTitle != "葬送のフリーレン" || tc.Year != 2023 ||
		tc.IMDbID != "tt22248376" || tc.TVDBID != 424536 || !tc.IsAnime || tc.Runtime != 24 {
		t.Errorf("context = %+v", tc)
	}
	// The romaji is an alternative title: TMDB's original name is Japanese.
	if tc.Romaji != "Sousou no Frieren" || !reflect.DeepEqual(tc.Aliases, []string{"Sousou no Frieren", "Frieren"}) {
		t.Errorf("romaji %q, aliases %q", tc.Romaji, tc.Aliases)
	}
	// HDRezka streams are files too.
	wantFiles := map[int64]string{101: "torrent", 102: "torrent", 103: "hdrezka"}
	if !reflect.DeepEqual(tc.Files, wantFiles) {
		t.Errorf("files = %v, want %v", tc.Files, wantFiles)
	}
	// E04 to E10 aired after the last file (E03); E11 airs a week later.
	abs := func(n int) *int { return &n }
	wantMissing := &catalog.MissingRun{SeasonNumber: 1, From: 4, To: 10, AbsoluteFrom: abs(4), AbsoluteTo: abs(10), Count: 7}
	if !reflect.DeepEqual(tc.Missing, wantMissing) {
		t.Errorf("missing = %+v, want %+v", tc.Missing, wantMissing)
	}
	// The catalog is the jobs' one: library episode ids.
	if tc.Catalog == nil || len(tc.Catalog.Episodes) != 13 || tc.Catalog.Episodes[1].ID != 101 || tc.Catalog.Episodes[1].Absolute != 1 {
		t.Fatalf("catalog = %+v", tc.Catalog)
	}
}

// A library show never fails on TMDB: aliases are left out.
func TestTitleContextLibraryShowWithoutTMDB(t *testing.T) {
	store := &fakeTitleStore{shows: map[int64]*library.Show{7: libraryFrieren()}}
	tc, err := newTestTitleContexts(store, &fakeTitleTMDB{err: errors.New("TMDB is down")}, "2023-12-01").Get(t.Context(), library.JobTV, frierenTMDB)
	if err != nil {
		t.Fatal(err)
	}
	if tc.Romaji != "" || len(tc.Aliases) != 0 || tc.Missing == nil || len(tc.Files) != 3 {
		t.Errorf("context = %+v", tc)
	}
}

func TestTitleContextTMDBShow(t *testing.T) {
	src := frierenTMDBFake(t)
	tc, err := newTestTitleContexts(&fakeTitleStore{}, src, "2026-10-16").Get(t.Context(), library.JobTV, frierenTMDB)
	if err != nil {
		t.Fatal(err)
	}

	if tc.LibraryID != nil || tc.EpisodeIDs != EpisodeIDsTMDB || len(tc.Files) != 0 || tc.Missing != nil {
		t.Errorf("library state: id %v, ids %q, files %v, missing %+v", tc.LibraryID, tc.EpisodeIDs, tc.Files, tc.Missing)
	}
	if tc.Title != "Frieren: Beyond Journey's End" || tc.OriginalTitle != "葬送のフリーレン" || tc.Year != 2023 ||
		tc.IMDbID != "tt22248376" || tc.TVDBID != 424536 || !tc.IsAnime || tc.Runtime != 24 {
		t.Errorf("context = %+v", tc)
	}
	if tc.Romaji != "Sousou no Frieren" || !reflect.DeepEqual(tc.Aliases, []string{"Sousou no Frieren", "Frieren"}) {
		t.Errorf("romaji %q, aliases %q", tc.Romaji, tc.Aliases)
	}

	cat := tc.Catalog
	if cat == nil || len(cat.Episodes) != 40 || cat.Ordering != identify.OrderingAbsolute {
		t.Fatalf("catalog = %+v", cat)
	}
	var e29 identify.Episode
	for _, ep := range cat.Episodes {
		if ep.Season == 1 && ep.Number == 29 {
			e29 = ep
		}
	}
	// TMDB ids, cumulative absolute numbers (no absolute group), and the
	// cours the classifier remaps through.
	if e29.ID != 5000029 || e29.Absolute != 29 {
		t.Errorf("S01E29 = %+v", e29)
	}
	nums := identify.ReleaseNumbers(cat, 1, 29)
	if len(nums) != 3 || nums[2].Label != identify.LabelScene || nums[2].Season != 2 || nums[2].Episode != 1 {
		t.Errorf("release numbers of S01E29 = %+v", nums)
	}
}

// An anime's absolute numbers follow TMDB's absolute episode group, as the
// library numbers a newly added show; a season-like group becomes an
// alternative ordering.
func TestTMDBShowCatalogGroups(t *testing.T) {
	details := &tmdb.ShowDetails{
		ID: 1, Name: "Recap Show", FirstAirDate: "2020-01-01", OriginalLanguage: "ja",
		Genres:  []tmdb.Genre{{ID: tmdb.GenreAnimation}},
		Seasons: []tmdb.SeasonSummary{{SeasonNumber: 1}, {SeasonNumber: 2}},
	}
	season := &tmdb.Season{}
	for n := 1; n <= 6; n++ {
		season.Episodes = append(season.Episodes, tmdb.Episode{ID: 900 + n, EpisodeNumber: n, Runtime: 20 + n})
	}
	group := func(name string, typ int, parts ...[]int) *tmdb.EpisodeGroup {
		g := &tmdb.EpisodeGroup{ID: name, Name: name, Type: typ}
		for i, eps := range parts {
			part := tmdb.EpisodeGroupGroup{Name: "Season " + string(rune('1'+i)), Order: i}
			for j, e := range eps {
				part.Episodes = append(part.Episodes, tmdb.GroupEpisode{SeasonNumber: 1, EpisodeNumber: e, Order: j})
			}
			g.Groups = append(g.Groups, part)
		}
		return g
	}
	src := &fakeTitleTMDB{
		fakeCatalogSource: fakeCatalogSource{
			summaries: []tmdb.EpisodeGroupSummary{
				{ID: "absolute", Type: tmdb.GroupAbsolute, EpisodeCount: 5},
				{ID: "dvd", Type: tmdb.GroupDVD},
			},
			groups: map[string]*tmdb.EpisodeGroup{
				"absolute": group("absolute", tmdb.GroupAbsolute, []int{1, 2, 4, 5, 6}), // E03 is a recap
				"dvd":      group("dvd", tmdb.GroupDVD, []int{1, 2, 3}, []int{4, 5, 6}),
			},
		},
		shows:   map[int]*tmdb.ShowDetails{1: details},
		seasons: map[int]*tmdb.Season{1: season}, // season 2: TMDB doesn't have it
	}

	ts, err := TMDBShowCatalog(t.Context(), src, 1)
	if err != nil {
		t.Fatal(err)
	}
	absolute := map[int]int{}
	for _, ep := range ts.Catalog.Episodes {
		absolute[ep.Number] = ep.Absolute
	}
	if want := map[int]int{1: 1, 2: 2, 3: 0, 4: 3, 5: 4, 6: 5}; !reflect.DeepEqual(absolute, want) {
		t.Errorf("absolute numbers = %v, want %v", absolute, want)
	}
	if len(ts.Catalog.AltOrderings) != 1 || len(ts.Catalog.AltOrderings[0].Groups) != 2 {
		t.Errorf("alternative orderings = %+v", ts.Catalog.AltOrderings)
	}
	if ts.Runtime != 24 {
		t.Errorf("runtime = %d, want 24", ts.Runtime)
	}

	src.seasonErr = map[int]error{1: errors.New("TMDB answered 500")}
	if _, err := TMDBShowCatalog(t.Context(), src, 1); !errors.Is(err, library.ErrTMDBFetch) {
		t.Errorf("a failed season: err = %v, want ErrTMDBFetch", err)
	}
}

func TestTitleContextMovie(t *testing.T) {
	interstellar := readJSON[tmdb.MovieDetails](t, "movie_157336")
	amelie := &tmdb.MovieDetails{ID: 194, Title: "Amélie", OriginalTitle: "Le Fabuleux Destin d'Amélie Poulain", ReleaseDate: "2001-04-25", Runtime: 122}
	src := &fakeTitleTMDB{movies: map[int]*tmdb.MovieDetails{157336: interstellar, 194: amelie}}
	store := &fakeTitleStore{movies: map[int]*library.Movie{157336: {
		ID: 3, TMDBID: 157336, Title: "Interstellar", Year: 2014, IMDbID: "tt0816692", Runtime: 169,
		Assignment: &library.TorrentAssignment{Kind: library.AssignmentKindHDRezka},
	}}}
	tcs := newTestTitleContexts(store, src, "2026-10-16")

	tc, err := tcs.Get(t.Context(), library.JobMovie, 157336)
	if err != nil {
		t.Fatal(err)
	}
	if tc.LibraryID == nil || *tc.LibraryID != 3 || tc.MovieFile != "hdrezka" || tc.Title != "Interstellar" ||
		tc.OriginalTitle != "" || tc.Year != 2014 || tc.IMDbID != "tt0816692" || tc.Runtime != 169 || tc.Catalog != nil {
		t.Errorf("library movie = %+v", tc)
	}

	tc, err = tcs.Get(t.Context(), library.JobMovie, 194)
	if err != nil {
		t.Fatal(err)
	}
	if tc.LibraryID != nil || tc.MovieFile != "" || tc.Title != "Amélie" || tc.OriginalTitle != "Le Fabuleux Destin d'Amélie Poulain" ||
		tc.Year != 2001 || tc.Runtime != 122 {
		t.Errorf("TMDB movie = %+v", tc)
	}

	// A library movie never fails on TMDB.
	src.err = errors.New("TMDB is down")
	if tc, err := tcs.Get(t.Context(), library.JobMovie, 157336); err != nil || tc.Title != "Interstellar" || tc.MovieFile != "hdrezka" {
		t.Errorf("library movie without TMDB = %+v, %v", tc, err)
	}
}

func TestTitleContextErrors(t *testing.T) {
	down := errors.New("TMDB answered 503")
	tests := []struct {
		name      string
		mediaType library.JobMediaType
		store     *fakeTitleStore
		tmdbErr   error
		want      []error
	}{
		{"unknown show", library.JobTV, &fakeTitleStore{}, nil, []error{ErrTitleNotFound, tmdb.ErrNotFound}},
		{"unknown movie", library.JobMovie, &fakeTitleStore{}, nil, []error{ErrTitleNotFound}},
		{"TMDB down, show not in the library", library.JobTV, &fakeTitleStore{}, down, []error{library.ErrTMDBFetch, down}},
		{"no TMDB key, movie not in the library", library.JobMovie, &fakeTitleStore{}, tmdb.ErrNoAPIKey, []error{library.ErrTMDBFetch, tmdb.ErrNoAPIKey}},
		{"the library fails", library.JobTV, &fakeTitleStore{err: down}, nil, []error{down}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := &fakeTitleTMDB{err: tt.tmdbErr}
			_, err := newTestTitleContexts(tt.store, src, "2026-10-16").Get(t.Context(), tt.mediaType, 42)
			for _, want := range tt.want {
				if !errors.Is(err, want) {
					t.Errorf("err = %v, want it to match %v", err, want)
				}
			}
			if errors.Is(err, ErrTitleNotFound) && errors.Is(err, library.ErrTMDBFetch) {
				t.Errorf("err = %v is both not found and unavailable", err)
			}
		})
	}

	if _, err := newTestTitleContexts(&fakeTitleStore{}, &fakeTitleTMDB{}, "2026-10-16").Get(t.Context(), "person", 1); err == nil {
		t.Error("unknown media type: no error")
	}
}
