package metadata

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/tmdb"
)

// fakeTMDB serves canned TMDB data and counts calls. Unknown IDs return
// tmdb.ErrNotFound.
type fakeTMDB struct {
	mu          sync.Mutex
	movies      map[int]*tmdb.MovieDetails
	shows       map[int]*tmdb.ShowDetails
	seasons     map[[2]int]*tmdb.Season
	seasonErrs  map[[2]int]error
	groupLists  map[int][]tmdb.EpisodeGroupSummary
	groups      map[string]*tmdb.EpisodeGroup
	calls       map[string]int
	seasonDelay time.Duration
	inFlight    int
	maxInFlight int
}

func newFakeTMDB() *fakeTMDB {
	return &fakeTMDB{
		movies:     map[int]*tmdb.MovieDetails{},
		shows:      map[int]*tmdb.ShowDetails{},
		seasons:    map[[2]int]*tmdb.Season{},
		seasonErrs: map[[2]int]error{},
		groupLists: map[int][]tmdb.EpisodeGroupSummary{},
		groups:     map[string]*tmdb.EpisodeGroup{},
		calls:      map[string]int{},
	}
}

var _ TMDB = (*fakeTMDB)(nil)

func (f *fakeTMDB) count(name string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[name]
}

func (f *fakeTMDB) MovieDetails(_ context.Context, id int) (*tmdb.MovieDetails, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls["movie"]++
	m, ok := f.movies[id]
	if !ok {
		return nil, tmdb.ErrNotFound
	}
	c := *m
	return &c, nil
}

func (f *fakeTMDB) ShowDetails(_ context.Context, id int) (*tmdb.ShowDetails, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls["show"]++
	s, ok := f.shows[id]
	if !ok {
		return nil, tmdb.ErrNotFound
	}
	c := *s
	return &c, nil
}

func (f *fakeTMDB) Season(_ context.Context, showID, n int, _ string) (*tmdb.Season, error) {
	f.mu.Lock()
	f.calls["season"]++
	f.inFlight++
	if f.inFlight > f.maxInFlight {
		f.maxInFlight = f.inFlight
	}
	delay := f.seasonDelay
	f.mu.Unlock()

	time.Sleep(delay)

	f.mu.Lock()
	defer f.mu.Unlock()
	f.inFlight--
	if err := f.seasonErrs[[2]int{showID, n}]; err != nil {
		return nil, err
	}
	s, ok := f.seasons[[2]int{showID, n}]
	if !ok {
		return nil, tmdb.ErrNotFound
	}
	c := *s
	c.Episodes = append([]tmdb.Episode(nil), s.Episodes...)
	return &c, nil
}

func (f *fakeTMDB) EpisodeGroups(_ context.Context, showID int) ([]tmdb.EpisodeGroupSummary, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls["groups"]++
	return f.groupLists[showID], nil
}

func (f *fakeTMDB) EpisodeGroup(_ context.Context, id string) (*tmdb.EpisodeGroup, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls["group"]++
	g, ok := f.groups[id]
	if !ok {
		return nil, tmdb.ErrNotFound
	}
	return g, nil
}

// addShow registers a show whose seasons have the given episode names.
// Episode TMDB IDs are id*1000 + season*100 + episode, air dates are in
// 2020, and season 0 is named "Specials".
func (f *fakeTMDB) addShow(details tmdb.ShowDetails, seasons map[int][]string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	details.Seasons = nil
	for n := 0; n <= 20; n++ {
		names, ok := seasons[n]
		if !ok {
			continue
		}
		name := fmt.Sprintf("Season %d", n)
		if n == 0 {
			name = "Specials"
		}
		details.Seasons = append(details.Seasons, tmdb.SeasonSummary{SeasonNumber: n, Name: name, EpisodeCount: len(names)})
		season := &tmdb.Season{SeasonNumber: n, Name: name, PosterPath: fmt.Sprintf("/s%d.jpg", n)}
		for i, epName := range names {
			season.Episodes = append(season.Episodes, tmdb.Episode{
				ID:            episodeTMDBID(details.ID, n, i+1),
				SeasonNumber:  n,
				EpisodeNumber: i + 1,
				Name:          epName,
				AirDate:       fmt.Sprintf("2020-%02d-%02d", n+1, i+1),
				EpisodeType:   "standard",
				Runtime:       24,
				StillPath:     fmt.Sprintf("/still-%d-%d.jpg", n, i+1),
				Overview:      "Overview of " + epName,
			})
		}
		f.seasons[[2]int{details.ID, n}] = season
	}
	f.shows[details.ID] = &details
}

// renameEpisode changes an episode's name on "TMDB".
func (f *fakeTMDB) renameEpisode(showID, season, episode int, name string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s := f.seasons[[2]int{showID, season}]
	for i := range s.Episodes {
		if s.Episodes[i].EpisodeNumber == episode {
			s.Episodes[i].Name = name
		}
	}
}

func episodeTMDBID(showID, season, episode int) int {
	return (showID%1_000_000)*1000 + season*100 + episode
}

// fakeTree counts tree rebuilds.
type fakeTree struct{ n atomic.Int32 }

func (t *fakeTree) InvalidateTree() { t.n.Add(1) }

var idSeq atomic.Int64

// uniqueTMDBID returns a TMDB ID no other test (or rerun) uses.
func uniqueTMDBID() int {
	return 1_000_000_000 + int((time.Now().UnixNano()/1000+idSeq.Add(7))%1_000_000_000)
}

// openTestDB connects to MOMOSHTREM_TEST_DATABASE_URL and runs migrations.
func openTestDB(t *testing.T) *library.DB {
	t.Helper()
	url := os.Getenv("MOMOSHTREM_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("MOMOSHTREM_TEST_DATABASE_URL not set; skipping Postgres integration test")
	}
	db, err := library.NewDB(url)
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

type harness struct {
	db     *library.DB
	tmdb   *fakeTMDB
	tree   *fakeTree
	syncer *Syncer
	movies *library.MovieRepository
	shows  *library.ShowRepository
	meta   *library.MetadataRepository
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	db := openTestDB(t)
	h := &harness{
		db:     db,
		tmdb:   newFakeTMDB(),
		tree:   &fakeTree{},
		movies: library.NewMovieRepository(db),
		shows:  library.NewShowRepository(db),
		meta:   library.NewMetadataRepository(db),
	}
	h.syncer = NewSyncer(h.tmdb, h.movies, h.shows, h.meta, WithTreeInvalidator(h.tree))
	return h
}

// cleanupShow deletes a show (and its assignments) after the test.
func (h *harness) cleanupShow(t *testing.T, tmdbID int) {
	t.Helper()
	t.Cleanup(func() {
		_, _ = h.db.Exec(`
			DELETE FROM torrent_assignments WHERE item_type = 'episode' AND item_id IN (
				SELECT e.id FROM episodes e JOIN seasons sn ON sn.id = e.season_id
				JOIN shows s ON s.id = sn.show_id WHERE s.tmdb_id = $1)`, tmdbID)
		_, _ = h.db.Exec(`DELETE FROM shows WHERE tmdb_id = $1`, tmdbID)
	})
}

func (h *harness) cleanupMovie(t *testing.T, tmdbID int) {
	t.Helper()
	t.Cleanup(func() { _, _ = h.db.Exec(`DELETE FROM movies WHERE tmdb_id = $1`, tmdbID) })
}

// assign gives an episode an active assignment.
func (h *harness) assign(t *testing.T, episodeID int64) {
	t.Helper()
	err := h.db.QueryRow(`
		INSERT INTO torrent_assignments (item_type, item_id, info_hash, magnet_uri, file_path, file_size, is_active)
		VALUES ('episode', $1, 'abcdef0123456789abcdef0123456789abcdef01', 'magnet:?xt=urn:btih:abc', 'file.mkv', 1, TRUE)
		RETURNING id`, episodeID).Scan(new(int64))
	if err != nil {
		t.Fatalf("assign: %v", err)
	}
}

// episode returns a stored episode by season and number.
func episodeOf(t *testing.T, show *library.Show, season, number int) library.Episode {
	t.Helper()
	for _, s := range show.Seasons {
		if s.SeasonNumber != season {
			continue
		}
		for _, e := range s.Episodes {
			if e.EpisodeNumber == number {
				return e
			}
		}
	}
	t.Fatalf("no S%02dE%02d in show %d", season, number, show.ID)
	return library.Episode{}
}

func absoluteOf(e library.Episode) int {
	if e.AbsoluteNumber == nil {
		return 0
	}
	return *e.AbsoluteNumber
}

func seasonNumbers(show *library.Show) []int {
	var out []int
	for _, s := range show.Seasons {
		out = append(out, s.SeasonNumber)
	}
	return out
}
