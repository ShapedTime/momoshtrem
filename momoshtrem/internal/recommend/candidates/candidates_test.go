package candidates

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/recommend/profile"
	"github.com/shapedtime/momoshtrem/internal/tmdb"
)

var now = time.Date(2026, 10, 16, 3, 0, 0, 0, time.UTC)

func item(id int, title string, genres ...int) tmdb.ListItem {
	return tmdb.ListItem{ID: id, Title: title, ReleaseDate: "2021-05-01", OriginalLanguage: "en", GenreIDs: genres, VoteAverage: 7.4, VoteCount: 900}
}

func showItem(id int, name string, genres ...int) tmdb.ListItem {
	return tmdb.ListItem{ID: id, Name: name, FirstAirDate: "2022-02-17", OriginalLanguage: "en", GenreIDs: genres, VoteAverage: 8.2, VoteCount: 1500}
}

func anime(i tmdb.ListItem) tmdb.ListItem {
	i.OriginalLanguage, i.OriginCountry = "ja", []string{"JP"}
	return i
}

func page(mediaType string, items ...tmdb.ListItem) *tmdb.ResultsPage {
	for i := range items {
		items[i].MediaType = mediaType
	}
	return &tmdb.ResultsPage{Page: 1, Results: items, TotalResults: len(items)}
}

// fakeTMDB answers from tables and records the discover queries.
type fakeTMDB struct {
	mu        sync.Mutex
	movies    map[int]*tmdb.MovieDetails
	shows     map[int]*tmdb.ShowDetails
	discover  map[string]*tmdb.ResultsPage // by media type
	trending  map[string]*tmdb.ResultsPage
	upcoming  *tmdb.ResultsPage
	onTheAir  *tmdb.ResultsPage
	fail      bool
	queries   []string
	described int
}

func (f *fakeTMDB) MovieDetails(_ context.Context, id int) (*tmdb.MovieDetails, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.described++
	if d := f.movies[id]; d != nil && !f.fail {
		return d, nil
	}
	return nil, tmdb.ErrNotFound
}

func (f *fakeTMDB) ShowDetails(_ context.Context, id int) (*tmdb.ShowDetails, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.described++
	if d := f.shows[id]; d != nil && !f.fail {
		return d, nil
	}
	return nil, tmdb.ErrNotFound
}

func (f *fakeTMDB) Discover(_ context.Context, mediaType string, q tmdb.DiscoverQuery) (*tmdb.ResultsPage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.queries = append(f.queries, fmt.Sprintf("%s %+v", mediaType, q))
	if f.fail || f.discover[mediaType] == nil {
		return nil, &tmdb.StatusError{StatusCode: 500}
	}
	return f.discover[mediaType], nil
}

func (f *fakeTMDB) Trending(_ context.Context, mediaType, _ string) (*tmdb.ResultsPage, error) {
	if f.fail || f.trending[mediaType] == nil {
		return nil, &tmdb.StatusError{StatusCode: 500}
	}
	return f.trending[mediaType], nil
}

func (f *fakeTMDB) UpcomingMovies(context.Context, int) (*tmdb.ResultsPage, error) {
	if f.fail || f.upcoming == nil {
		return nil, &tmdb.StatusError{StatusCode: 500}
	}
	return f.upcoming, nil
}

func (f *fakeTMDB) OnTheAir(context.Context, int) (*tmdb.ResultsPage, error) {
	if f.fail || f.onTheAir == nil {
		return nil, &tmdb.StatusError{StatusCode: 500}
	}
	return f.onTheAir, nil
}

func (f *fakeTMDB) GenreList(_ context.Context, mediaType string) ([]tmdb.Genre, error) {
	if f.fail {
		return nil, &tmdb.StatusError{StatusCode: 500}
	}
	if mediaType == tmdb.MediaTV {
		return []tmdb.Genre{{ID: 18, Name: "Drama"}, {ID: 9648, Name: "Mystery"}, {ID: 16, Name: "Animation"}, {ID: 10765, Name: "Sci-Fi & Fantasy"}}, nil
	}
	return []tmdb.Genre{{ID: 18, Name: "Drama"}, {ID: 878, Name: "Science Fiction"}, {ID: 16, Name: "Animation"}, {ID: 28, Name: "Action"}}, nil
}

// fakeStore is the database's side.
type fakeStore struct {
	movies    []library.WatchedMovie
	shows     []library.WatchedShow
	ratings   []library.TasteRating
	hidden    []library.HiddenShow
	feedback  []library.TasteFeedback
	titles    []library.LibraryTitle
	watchlist []library.TitleRef
}

func (f *fakeStore) WatchedMovies(context.Context) ([]library.WatchedMovie, error) {
	return f.movies, nil
}
func (f *fakeStore) WatchedShows(context.Context) ([]library.WatchedShow, error) { return f.shows, nil }
func (f *fakeStore) TasteRatings(context.Context) ([]library.TasteRating, error) {
	return f.ratings, nil
}
func (f *fakeStore) HiddenShows(context.Context) ([]library.HiddenShow, error) { return f.hidden, nil }
func (f *fakeStore) TasteFeedback(context.Context) ([]library.TasteFeedback, error) {
	return f.feedback, nil
}
func (f *fakeStore) LibraryTitles(context.Context) ([]library.LibraryTitle, error) {
	return f.titles, nil
}
func (f *fakeStore) WatchlistRefs(context.Context) ([]library.TitleRef, error) {
	return f.watchlist, nil
}

func movieRef(id int) library.TitleRef { return library.TitleRef{MediaType: "movie", TMDBID: id} }
func showRef(id int) library.TitleRef  { return library.TitleRef{MediaType: "tv", TMDBID: id} }

// testProfile loves Severance (a show) and Arrival (a movie): drama and
// science fiction, the keyword "time loop", no anime.
func testProfile() *profile.Profile {
	return &profile.Profile{
		Mix:       profile.Mix{Movies: 4, Shows: 6, MovieShare: 0.4, AnimeShare: 0},
		Genres:    []profile.Weighted{{ID: 18, Name: "Drama", W: 0.8}, {ID: 878, Name: "Science Fiction", W: 0.5}, {ID: 9648, Name: "Mystery", W: 0.4}},
		Keywords:  []profile.Weighted{{ID: 4379, Name: "time loop", W: 0.4}},
		Languages: []profile.Weighted{{Name: "en", W: 0.9}, {Name: "ko", W: 0.1}},
		Loved: []profile.Title{
			{MediaType: "tv", TMDBID: 95396, Title: "Severance", Note: "completed"},
			{MediaType: "movie", TMDBID: 329865, Title: "Arrival", Note: "watched"},
		},
	}
}

func testTMDB() *fakeTMDB {
	return &fakeTMDB{
		shows: map[int]*tmdb.ShowDetails{
			95396: {ID: 95396, Name: "Severance",
				Recommendations: tmdb.ResultsPage{Results: []tmdb.ListItem{showItem(1, "Dark", 18, 9648), showItem(2, "Watched Show", 18), showItem(3, "Hidden Show", 18)}},
				Similar:         tmdb.ResultsPage{Results: []tmdb.ListItem{showItem(4, "Started Show", 18), showItem(1, "Dark", 18, 9648)}}},
			1: {ID: 1, Name: "Dark", EpisodeRunTime: []int{55}, Status: "Ended",
				Seasons:  []tmdb.SeasonSummary{{SeasonNumber: 0}, {SeasonNumber: 1}, {SeasonNumber: 2}, {SeasonNumber: 3}},
				Keywords: tmdb.Keywords{Keywords: []tmdb.Keyword{{ID: 4379, Name: "time loop"}}}},
		},
		movies: map[int]*tmdb.MovieDetails{
			329865: {ID: 329865, Title: "Arrival",
				Recommendations: tmdb.ResultsPage{Results: []tmdb.ListItem{item(10, "Annihilation", 878, 18), item(11, "Watched Movie", 878), item(12, "Disliked Movie", 878)}},
				Similar:         tmdb.ResultsPage{Results: []tmdb.ListItem{item(13, "Seen Movie", 878), item(14, "Liked Movie", 878)}}},
			10: {ID: 10, Title: "Annihilation", Runtime: 115},
			20: {ID: 20, Title: "Big Action", Runtime: 140},
		},
		discover: map[string]*tmdb.ResultsPage{
			"movie": page("movie", item(15, "Library Movie", 18), item(10, "Annihilation", 878, 18)),
			"tv":    page("tv", anime(showItem(30, "Anime Show", 16, 18))),
		},
		trending: map[string]*tmdb.ResultsPage{
			"movie": page("movie", item(20, "Big Action", 28), item(21, "Not Interested Movie", 28)),
			"tv":    page("tv", showItem(1, "Dark", 18, 9648)),
		},
		upcoming: page("movie", tmdb.ListItem{ID: 40, Title: "Next Year", ReleaseDate: "2027-01-16", GenreIDs: []int{878}},
			tmdb.ListItem{ID: 41, Title: "Out Last Week", ReleaseDate: "2026-10-09", GenreIDs: []int{18}}),
		onTheAir: page("tv", showItem(50, "Airing", 18), showItem(51, "Dropped Show", 18)),
	}
}

func testStore() *fakeStore {
	return &fakeStore{
		movies: []library.WatchedMovie{{TMDBID: 11, Plays: 1}, {TMDBID: 329865, Plays: 2}},
		shows: []library.WatchedShow{
			{TMDBID: 2, Plays: 10, Aired: 10, Completed: 10, HasProgress: true},
			{TMDBID: 4, Plays: 1}, // plays without progress: started
			{TMDBID: 95396, Plays: 19, Aired: 19, Completed: 19, HasProgress: true},
		},
		hidden: []library.HiddenShow{{TMDBID: 3, FromProgress: true}, {TMDBID: 51, Dropped: true}},
		ratings: []library.TasteRating{
			{TitleRef: movieRef(20), Rating: 6},                    // rated on Trakt, never played: seen
			{TitleRef: showRef(50), Rating: 7, FromEpisodes: true}, // its episodes' mean isn't its own rating
		},
		feedback: []library.TasteFeedback{
			{TitleRef: movieRef(12), Signal: "dislike"}, {TitleRef: movieRef(13), Signal: "seen"},
			{TitleRef: movieRef(14), Signal: "like"}, {TitleRef: movieRef(21), Signal: "not_interested"},
		},
		titles:    []library.LibraryTitle{{TitleRef: movieRef(15), Title: "Library Movie"}},
		watchlist: []library.TitleRef{showRef(1)},
	}
}

func build(t *testing.T, tm *fakeTMDB, st *fakeStore, p *profile.Profile) *Pool {
	t.Helper()
	pool, err := New(Deps{TMDB: tm, Store: st, Now: func() time.Time { return now }}).Build(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	return pool
}

func byID(pool *Pool) map[string]Candidate {
	out := map[string]Candidate{}
	for _, c := range pool.Items {
		out[fmt.Sprintf("%s:%d", c.MediaType, c.TMDBID)] = c
	}
	return out
}

func TestBuild_Exclusions(t *testing.T) {
	pool := build(t, testTMDB(), testStore(), testProfile())
	got := byID(pool)
	for _, gone := range []string{"tv:2", "tv:3", "tv:4", "movie:11", "movie:12", "movie:13", "movie:20", "movie:21", "tv:51"} {
		if _, ok := got[gone]; ok {
			t.Errorf("%s is in the pool", gone)
		}
	}
	for _, kept := range []string{"tv:1", "movie:10", "movie:14", "movie:15", "tv:30", "movie:40", "movie:41", "tv:50"} {
		if _, ok := got[kept]; !ok {
			t.Errorf("%s isn't in the pool", kept)
		}
	}
	// Library titles stay, marked; so do watchlisted ones
	if !got["movie:15"].InLibrary || !got["tv:1"].OnWatchlist || got["movie:10"].InLibrary {
		t.Errorf("marks: library %+v, watchlist %+v", got["movie:15"], got["tv:1"])
	}
	if want := (Stats{Considered: 17, Excluded: 9, Kept: 8}); pool.Stats != want {
		t.Errorf("stats = %+v, want %+v", pool.Stats, want)
	}
}

func TestBuild_Ranking(t *testing.T) {
	pool := build(t, testTMDB(), testStore(), testProfile())
	// Dark: Severance's recommendation and similar title, trending, drama
	// and mystery, the time-loop keyword once described
	dark := pool.Items[0]
	if dark.TMDBID != 1 {
		t.Fatalf("first = %+v, want Dark", dark)
	}
	want := Candidate{MediaType: "tv", TMDBID: 1, Title: "Dark", Date: "2022-02-17", Genres: []string{"Drama", "Mystery"}, Language: "en",
		Runtime: 55, Seasons: 3, Ended: true, Vote: 8.2, Why: []string{"rec:Severance", "similar:Severance", "trending"}, OnWatchlist: true}
	gotJSON, _ := json.Marshal(dark)
	wantJSON, _ := json.Marshal(want)
	if string(gotJSON) != string(wantJSON) {
		t.Errorf("Dark = %s\nwant %s", gotJSON, wantJSON)
	}
	pos := map[int]int{}
	for i, c := range pool.Items {
		pos[c.TMDBID] = i
		if i > 0 && c.Score() > pool.Items[i-1].Score() {
			t.Errorf("%s (%.3f) after %s (%.3f)", c.Title, c.Score(), pool.Items[i-1].Title, pool.Items[i-1].Score())
		}
	}
	// A loved movie's recommendation beats a trending airing show
	if pos[10] > pos[50] {
		t.Errorf("Annihilation at %d after Airing at %d", pos[10], pos[50])
	}
	byIDs := byID(pool)
	if !byIDs["tv:30"].Anime {
		t.Error("the anime show isn't marked anime")
	}
	// Released already: new, not upcoming
	if w := byIDs["movie:41"].Why; !reflect.DeepEqual(w, []string{"new"}) {
		t.Errorf("released upcoming why = %v", w)
	}
	if w := byIDs["movie:40"].Why; !reflect.DeepEqual(w, []string{"upcoming"}) {
		t.Errorf("upcoming why = %v", w)
	}
}

// The same inputs make the same pool, whichever source answers first.
func TestBuild_Deterministic(t *testing.T) {
	first, _ := json.Marshal(build(t, testTMDB(), testStore(), testProfile()))
	for i := 0; i < 10; i++ {
		again, _ := json.Marshal(build(t, testTMDB(), testStore(), testProfile()))
		if string(again) != string(first) {
			t.Fatalf("pool %d differs:\n%s\n%s", i, again, first)
		}
	}
}

// Discover's windows come from now; a media type under 15% of the profile
// gets none.
func TestBuild_DiscoverQueries(t *testing.T) {
	tm := testTMDB()
	p := testProfile()
	p.Languages = []profile.Weighted{{Name: "en", W: 0.7}, {Name: "ko", W: 0.3}}
	build(t, tm, testStore(), p)
	joined := strings.Join(tm.queries, "\n")
	for _, want := range []string{
		"movie {Genres:[] Keywords:[4379] From:2021-10-16 To: MinVoteAverage:0 MinVoteCount:50 Language: SortBy: Page:0}",
		"movie {Genres:[18 878] Keywords:[] From:2026-06-18 To:2026-10-16 MinVoteAverage:0 MinVoteCount:10 Language: SortBy: Page:0}",
		"movie {Genres:[18 878] Keywords:[] From: To:2024-10-16 MinVoteAverage:7.5 MinVoteCount:300 Language: SortBy:vote_average.desc Page:0}",
		"movie {Genres:[18 878] Keywords:[] From:2023-10-16 To: MinVoteAverage:0 MinVoteCount:20 Language:ko SortBy: Page:0}",
		"tv {Genres:[18 9648] Keywords:[] From:2026-06-18 To:2026-10-16 MinVoteAverage:0 MinVoteCount:10 Language: SortBy: Page:0}",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("no query %q in\n%s", want, joined)
		}
	}

	tm = testTMDB()
	p = testProfile()
	p.Mix.MovieShare = 0.95
	build(t, tm, testStore(), p)
	for _, q := range tm.queries {
		if strings.HasPrefix(q, "tv ") {
			t.Errorf("a movie watcher's pool discovers shows: %s", q)
		}
	}
}

func TestBuild_PoolSize(t *testing.T) {
	tm := testTMDB()
	var many []tmdb.ListItem
	for i := 1000; i < 1000+400; i++ {
		many = append(many, item(i, fmt.Sprintf("Movie %d", i), 18))
	}
	tm.discover["movie"] = page("movie", many...)
	pool := build(t, tm, testStore(), testProfile())
	if len(pool.Items) != PoolSize || pool.Stats.Kept != PoolSize {
		t.Errorf("pool has %d (stats %+v), want %d", len(pool.Items), pool.Stats, PoolSize)
	}
	// Only the best described were asked about, plus the seeds
	if tm.described > described+seeds {
		t.Errorf("TMDB described %d titles", tm.described)
	}
}

func TestBuild_TMDBDown(t *testing.T) {
	tm := testTMDB()
	tm.fail = true
	_, err := New(Deps{TMDB: tm, Store: testStore(), Now: func() time.Time { return now }}).Build(context.Background(), testProfile())
	if !errors.Is(err, ErrNoSources) {
		t.Errorf("err = %v, want ErrNoSources", err)
	}
	// Some sources failing is fine
	tm = testTMDB()
	tm.trending = nil
	if pool := build(t, tm, testStore(), testProfile()); len(pool.Items) == 0 {
		t.Error("empty pool with most sources up")
	}
}

func TestExclusion(t *testing.T) {
	six := 6
	tests := []struct {
		name string
		s    library.TitleState
		want string
	}{
		{"unknown movie", library.TitleState{TitleRef: movieRef(1)}, ""},
		{"watched movie", library.TitleState{TitleRef: movieRef(1), Plays: 1, Started: true}, ExcludedWatched},
		{"liked movie", library.TitleState{TitleRef: movieRef(1), Feedback: "like", OnWatchlist: true, InLibrary: true}, ""},
		{"disliked", library.TitleState{TitleRef: movieRef(1), Feedback: "dislike"}, ExcludedDisliked},
		{"not interested", library.TitleState{TitleRef: showRef(1), Feedback: "not_interested"}, ExcludedNotInterested},
		{"seen beats watched", library.TitleState{TitleRef: movieRef(1), Feedback: "seen", Plays: 2, Started: true}, ExcludedSeen},
		{"finished show", library.TitleState{TitleRef: showRef(1), Started: true, Aired: 10, Completed: 10}, ExcludedWatched},
		{"started show", library.TitleState{TitleRef: showRef(1), Started: true, Aired: 10, Completed: 2}, ExcludedStarted},
		{"plays only", library.TitleState{TitleRef: showRef(1), Plays: 1}, ExcludedStarted},
		{"dropped, never started", library.TitleState{TitleRef: showRef(1), Dropped: true}, ExcludedDropped},
		{"hidden from progress", library.TitleState{TitleRef: showRef(1), Hidden: true}, ExcludedHidden},
		{"in the library", library.TitleState{TitleRef: showRef(1), InLibrary: true}, ""},
		{"rated, never played", library.TitleState{TitleRef: movieRef(1), Rating: &six}, ExcludedRated},
		{"rated show, never started", library.TitleState{TitleRef: showRef(1), Rating: &six, Hidden: true}, ExcludedRated},
		{"watched beats rated", library.TitleState{TitleRef: movieRef(1), Plays: 1, Started: true, Rating: &six}, ExcludedWatched},
	}
	for _, tt := range tests {
		if got := Exclusion(tt.s); got != tt.want {
			t.Errorf("%s: %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestPool_Filter(t *testing.T) {
	yes, no := true, false
	pool := &Pool{Items: []Candidate{
		{MediaType: "tv", TMDBID: 1, Title: "Dark", Genres: []string{"Drama", "Sci-Fi & Fantasy"}, Language: "de", Runtime: 55, year: 2017},
		{MediaType: "movie", TMDBID: 2, Title: "Annihilation", Genres: []string{"Science Fiction"}, Language: "en", Runtime: 115, year: 2018},
		{MediaType: "tv", TMDBID: 3, Title: "Frieren", Genres: []string{"Animation"}, Language: "ja", Anime: true, Runtime: 24, year: 2023},
		{MediaType: "movie", TMDBID: 4, Title: "Unknown", Genres: []string{"Drama"}, Language: "en"},
	}}
	ids := func(f Filter) []int {
		items, _ := pool.Filter(f)
		var out []int
		for _, c := range items {
			out = append(out, c.TMDBID)
		}
		return out
	}
	tests := []struct {
		name string
		f    Filter
		want []int
	}{
		{"everything", Filter{}, []int{1, 2, 3, 4}},
		{"shows", Filter{MediaType: "tv"}, []int{1, 3}},
		{"science fiction finds TV's Sci-Fi & Fantasy", Filter{Genres: []string{"science fiction"}}, []int{1, 2}},
		{"any of the genres", Filter{Genres: []string{"Animation", "drama"}}, []int{1, 3, 4}},
		{"short: no runtime is no match", Filter{MaxRuntime: 60}, []int{1, 3}},
		{"long", Filter{MinRuntime: 100}, []int{2}},
		{"recent: no year is no match", Filter{YearFrom: 2018}, []int{2, 3}},
		{"older", Filter{YearTo: 2017}, []int{1}},
		{"language", Filter{Language: "JA"}, []int{3}},
		{"anime only", Filter{Anime: &yes}, []int{3}},
		{"no anime", Filter{Anime: &no}, []int{1, 2, 4}},
		{"limit", Filter{Limit: 2}, []int{1, 2}},
	}
	for _, tt := range tests {
		if got := ids(tt.f); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%s: %v, want %v", tt.name, got, tt.want)
		}
	}
	if _, matched := pool.Filter(Filter{Limit: 1}); matched != 4 {
		t.Errorf("matched = %d, want 4", matched)
	}
	big := &Pool{}
	for i := 0; i < 100; i++ {
		big.Items = append(big.Items, Candidate{MediaType: "movie", TMDBID: i})
	}
	if items, _ := big.Filter(Filter{}); len(items) != DefaultLimit {
		t.Errorf("default limit gave %d", len(items))
	}
	if items, _ := big.Filter(Filter{Limit: 500}); len(items) != MaxLimit {
		t.Errorf("limit 500 gave %d", len(items))
	}
	if items, _ := (&Pool{}).Filter(Filter{}); items == nil {
		t.Error("an empty filter result is nil, not []")
	}
}

// Seeds are keyed by their rank, not their name: two loved titles called
// Dune stay apart, and a title only marked seen seeds nothing.
func TestBuild_Seeds(t *testing.T) {
	tm := testTMDB()
	tm.movies[841] = &tmdb.MovieDetails{ID: 841, Title: "Dune",
		Recommendations: tmdb.ResultsPage{Results: []tmdb.ListItem{item(60, "Flash Gordon", 878)}}}
	tm.movies[438631] = &tmdb.MovieDetails{ID: 438631, Title: "Dune",
		Recommendations: tmdb.ResultsPage{Results: []tmdb.ListItem{item(61, "Arrakis Story", 878)}}}
	tm.movies[949] = &tmdb.MovieDetails{ID: 949, Title: "Heat",
		Recommendations: tmdb.ResultsPage{Results: []tmdb.ListItem{item(62, "From a Seen Seed", 80)}}}
	p := testProfile()
	p.Loved = []profile.Title{
		{MediaType: "movie", TMDBID: 949, Title: "Heat", Year: 1995, Note: profile.NoteSeen},
		{MediaType: "movie", TMDBID: 438631, Title: "Dune", Year: 2021, Note: "watched"},
		{MediaType: "movie", TMDBID: 841, Title: "Dune", Year: 1984, Note: "watched"},
	}
	got := byID(build(t, tm, testStore(), p))
	if w := got["movie:61"].Why; !reflect.DeepEqual(w, []string{"rec:Dune (2021)"}) {
		t.Errorf("2021's pick why = %v", w)
	}
	if w := got["movie:60"].Why; !reflect.DeepEqual(w, []string{"rec:Dune (1984)"}) {
		t.Errorf("1984's pick why = %v", w)
	}
	// The first real seed counts fully, the second 5% less
	if a, b := got["movie:61"], got["movie:60"]; a.Score() <= b.Score() {
		t.Errorf("first seed's pick %.3f, second's %.3f", a.Score(), b.Score())
	}
	if _, ok := got["movie:62"]; ok {
		t.Error("a title only marked seen seeded the pool")
	}
}
