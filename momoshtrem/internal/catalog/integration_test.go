package catalog

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/shapedtime/momoshtrem/internal/artwork"
	"github.com/shapedtime/momoshtrem/internal/downloads"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/tmdb"
)

// queryCounter counts the queries pgx runs.
type queryCounter struct{ n atomic.Int32 }

func (q *queryCounter) TraceQueryStart(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryStartData) context.Context {
	q.n.Add(1)
	return ctx
}

func (q *queryCounter) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

// openTracedDB opens MOMOSHTREM_TEST_DATABASE_URL (migrated) with a query
// counter, or skips without one.
func openTracedDB(t *testing.T) (*library.DB, *queryCounter) {
	t.Helper()
	url := os.Getenv("MOMOSHTREM_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("MOMOSHTREM_TEST_DATABASE_URL not set; skipping Postgres integration test")
	}
	migrated, err := library.NewDB(url)
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	migrated.Close()

	cfg, err := pgx.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	counter := &queryCounter{}
	cfg.Tracer = counter
	db := library.NewDBFromSQL(stdlib.OpenDB(*cfg))
	t.Cleanup(func() { db.Close() })
	return db, counter
}

// fakeTMDBServer serves TMDB JSON from Go values at their API paths.
func fakeTMDBServer(t *testing.T, routes map[string]any) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("api_key") == "" {
			http.Error(w, "no key", http.StatusUnauthorized)
			return
		}
		v, ok := routes[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(v)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func imageServer(t *testing.T) *httptest.Server {
	img := image.NewRGBA(image.Rect(0, 0, 300, 169))
	for y := 0; y < 169; y++ {
		for x := 0; x < 300; x++ {
			img.Set(x, y, color.RGBA{40, 90, 160, 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(buf.Bytes())
	}))
	t.Cleanup(srv.Close)
	return srv
}

// integration is a catalog over the real repository, TMDB cache and colour
// extractor, with seeded rows that are removed afterwards.
type integration struct {
	t         *testing.T
	db        *library.DB
	counter   *queryCounter
	svc       *Service
	torrents  *fakeTorrents
	downloads *fakeDownloads
	tag       string // unique per run: titles, TMDB IDs and backdrops
	baseID    int
}

// A file a download wants, on disk, and one it's still fetching.
var (
	onDisk   = downloads.FileStatus{Bytes: 10, DoneBytes: 10, Complete: true, State: downloads.StateDone}
	fetching = downloads.FileStatus{Bytes: 10, DoneBytes: 3, State: downloads.StateDownloading}
)

func newIntegration(t *testing.T) *integration {
	db, counter := openTracedDB(t)
	nanos := time.Now().UnixNano()
	return &integration{t: t, db: db, counter: counter, torrents: &fakeTorrents{loaded: map[string]fakeTorrent{}},
		downloads: newFakeDownloads(), tag: fmt.Sprintf("It%d", nanos), baseID: 1_500_000_000 + int(nanos/1000%400_000_000)}
}

func (it *integration) id(n int) int { return it.baseID + n }

func (it *integration) exec(query string, args ...any) {
	it.t.Helper()
	if _, err := it.db.Exec(query, args...); err != nil {
		it.t.Fatalf("%s: %v", query, err)
	}
}

func (it *integration) movie(tmdbID int, title, backdrop string) *library.Movie {
	m := &library.Movie{TMDBID: tmdbID, Title: title, Year: 2024, BackdropPath: backdrop}
	if _, err := library.NewMovieRepository(it.db).CreateIfAbsent(context.Background(), m); err != nil {
		it.t.Fatal(err)
	}
	it.t.Cleanup(func() { _, _ = it.db.Exec(`DELETE FROM movies WHERE id = $1`, m.ID) })
	return m
}

func (it *integration) show(tmdbID int, title string) *library.Show {
	s := &library.Show{TMDBID: tmdbID, Title: title, Year: 2023, EpisodeOrdering: library.OrderingAbsolute}
	if _, err := library.NewShowRepository(it.db).CreateIfAbsent(context.Background(), s); err != nil {
		it.t.Fatal(err)
	}
	it.t.Cleanup(func() { _, _ = it.db.Exec(`DELETE FROM shows WHERE id = $1`, s.ID) })
	return s
}

// season stores a season of weekly episodes and returns their IDs by number.
func (it *integration) season(show *library.Show, number, count int, start string) map[int]int64 {
	ctx := context.Background()
	meta := library.NewMetadataRepository(it.db)
	row := &library.Season{ShowID: show.ID, SeasonNumber: number}
	if _, err := meta.UpsertSeason(ctx, row); err != nil {
		it.t.Fatal(err)
	}
	t0, _ := time.Parse(time.DateOnly, start)
	eps := make([]library.EpisodeMetadata, count)
	for i := range eps {
		eps[i] = library.EpisodeMetadata{EpisodeNumber: i + 1, Name: fmt.Sprintf("Episode %d", i+1),
			AirDate: t0.AddDate(0, 0, 7*i).Format(time.DateOnly), Runtime: 24}
	}
	if _, err := meta.UpsertEpisodes(ctx, row.ID, eps, false); err != nil {
		it.t.Fatal(err)
	}
	ids := map[int]int64{}
	rows, err := it.db.Query(`SELECT episode_number, id FROM episodes WHERE season_id = $1`, row.ID)
	if err != nil {
		it.t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var n int
		var id int64
		if err := rows.Scan(&n, &id); err != nil {
			it.t.Fatal(err)
		}
		ids[n] = id
	}
	// Absolute numbers 100·season + episode (none for specials).
	if number > 0 {
		it.exec(`UPDATE episodes SET absolute_number = $2 + episode_number WHERE season_id = $1`, row.ID, 100*number)
	}
	return ids
}

// assign makes the file the item's only active assignment.
func (it *integration) assign(itemType library.ItemType, itemID int64, hash, path string) {
	it.exec(`UPDATE torrent_assignments SET is_active = FALSE WHERE item_type = $1 AND item_id = $2`, itemType, itemID)
	var id int64
	if err := it.db.QueryRow(`
		INSERT INTO torrent_assignments (item_type, item_id, info_hash, magnet_uri, file_path, file_size, resolution, is_active)
		VALUES ($1, $2, $3, $4, $5, 1000, '1080p', TRUE) RETURNING id`,
		itemType, itemID, hash, "magnet:?xt=urn:btih:"+hash+"&dn=Release."+it.tag, path).Scan(&id); err != nil {
		it.t.Fatal(err)
	}
	it.t.Cleanup(func() { _, _ = it.db.Exec(`DELETE FROM torrent_assignments WHERE id = $1`, id) })
}

// streamSize is the size of every HDRezka stream stream() assigns.
const streamSize = 700

// stream makes an HDRezka stream the item's file through the writer:
// titleID is the show (for an episode) or the movie, episode the stream's
// episode (0 for a movie).
func (it *integration) stream(itemType library.ItemType, titleID, itemID int64, episode int) {
	it.t.Helper()
	ref := &library.StreamRef{Provider: "hdrezka", Path: "/series/646-show.html", PostID: 646, Title: it.tag,
		TranslatorID: 56, TranslatorName: "LostFilm", Season: 1, Episode: episode, Quality: "1080p"}
	w := library.AssignmentWrite{ItemType: itemType,
		Rows: []library.TorrentAssignment{{ItemID: itemID, Stream: ref, FileSize: streamSize, Resolution: "1080p"}}}
	if itemType == library.ItemTypeEpisode {
		w.ShowID = titleID
	} else {
		w.MovieID = titleID
	}
	if err := library.NewAssignmentRepository(it.db).WriteStream(context.Background(), w); err != nil {
		it.t.Fatal(err)
	}
	it.t.Cleanup(func() {
		_, _ = it.db.Exec(`DELETE FROM torrent_assignments WHERE item_type = $1 AND item_id = $2 AND kind = 'hdrezka'`, itemType, itemID)
	})
}

// watched records a Trakt play of an episode (episodeTMDB 0: Trakt has no
// TMDB id for it).
func (it *integration) watched(showTMDB, season, episode, episodeTMDB int) {
	it.t.Helper()
	it.exec(`INSERT INTO trakt_watched_episodes (show_tmdb_id, season, episode, episode_tmdb_id, plays, last_watched_at)
		VALUES ($1, $2, $3, NULLIF($4, 0), 1, NOW())`, showTMDB, season, episode, episodeTMDB)
	it.t.Cleanup(func() {
		_, _ = it.db.Exec(`DELETE FROM trakt_watched_episodes WHERE show_tmdb_id = $1 AND season = $2 AND episode = $3`,
			showTMDB, season, episode)
	})
}

func (it *integration) queries(fn func()) int {
	before := it.counter.n.Load()
	fn()
	return int(it.counter.n.Load() - before)
}

func TestIntegration_QueryBudgets(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()

	movieID, showID, otherShowID := it.id(1), it.id(2), it.id(3)
	movieBackdrop, showBackdrop := "/movie-"+it.tag+".jpg", "/show-"+it.tag+".jpg"

	movie := it.movie(movieID, "Movie "+it.tag, movieBackdrop)
	it.assign(library.ItemTypeMovie, movie.ID, "hash-movie-"+it.tag, "Movie/movie.mkv")

	show := it.show(showID, "Show "+it.tag)
	s1 := it.season(show, 1, 30, "2023-01-06") // long: ranges, arcs
	it.season(show, 2, 8, "2024-01-05")
	it.season(show, 0, 2, "2023-06-01")
	for n := 1; n <= 12; n++ {
		it.assign(library.ItemTypeEpisode, s1[n], "hash-show-"+it.tag, fmt.Sprintf("S01/E%02d.mkv", n))
	}
	other := it.show(otherShowID, "Other "+it.tag)
	it.season(other, 1, 3, "2020-01-03")
	// Trakt's plays: E13 of the show, and the movie.
	it.watched(showID, 1, 13, 0)
	it.exec(`INSERT INTO trakt_watched_movies (tmdb_id, plays, last_watched_at) VALUES ($1, 1, NOW())`, movieID)
	t.Cleanup(func() { _, _ = it.db.Exec(`DELETE FROM trakt_watched_movies WHERE tmdb_id = $1`, movieID) })

	arcsID := "arcs-" + it.tag
	var firstArc []tmdb.GroupEpisode // S01E01–24: 80% of the season, so arcs win
	for n := 1; n <= 24; n++ {
		firstArc = append(firstArc, tmdb.GroupEpisode{SeasonNumber: 1, EpisodeNumber: n})
	}
	tmdbSrv := fakeTMDBServer(t, map[string]any{
		fmt.Sprintf("/movie/%d", movieID): tmdb.MovieDetails{ID: movieID, Title: "Movie " + it.tag, ReleaseDate: "2024-01-01",
			BackdropPath: movieBackdrop, Recommendations: tmdb.ResultsPage{Results: []tmdb.ListItem{
				{ID: movieID, MediaType: "movie", Title: "Itself"}, {ID: it.id(9), MediaType: "movie", Title: "Elsewhere"},
			}}},
		fmt.Sprintf("/tv/%d", showID): tmdb.ShowDetails{ID: showID, Name: "Show " + it.tag, FirstAirDate: "2023-01-06",
			BackdropPath: showBackdrop, Status: "Returning Series",
			Seasons: []tmdb.SeasonSummary{{SeasonNumber: 0}, {SeasonNumber: 1}, {SeasonNumber: 2}},
			Recommendations: tmdb.ResultsPage{Results: []tmdb.ListItem{
				{ID: otherShowID, MediaType: "tv", Name: "Other"}, {ID: it.id(8), MediaType: "tv", Name: "Not added"},
			}}},
		fmt.Sprintf("/tv/%d/episode_groups", showID): map[string]any{"results": []tmdb.EpisodeGroupSummary{
			{ID: arcsID, Type: tmdb.GroupStoryArc, EpisodeCount: len(firstArc)},
		}},
		"/tv/episode_group/" + arcsID: tmdb.EpisodeGroup{ID: arcsID, Groups: []tmdb.EpisodeGroupGroup{
			{Name: "First Arc", Order: 0, Episodes: firstArc},
		}},
	})
	t.Cleanup(func() {
		_, _ = it.db.Exec(`DELETE FROM tmdb_cache WHERE key LIKE '%' || $1 || '%' OR key LIKE ANY($2)`, it.tag,
			[]string{fmt.Sprintf("%%/%d", movieID), fmt.Sprintf("%%/%d", showID), fmt.Sprintf("%%/%d/%%", showID)})
	})

	cache := library.NewTMDBCacheRepository(it.db)
	colors := artwork.New(cache, artwork.WithBaseURL(imageServer(t).URL))
	t.Cleanup(colors.Close)
	it.svc = New(Deps{
		Store:           library.NewCatalogRepository(it.db),
		TMDB:            tmdb.NewCached(tmdb.NewClient("test-key", tmdb.WithBaseURL(tmdbSrv.URL), tmdb.WithRetries(1, 0)), cache),
		Torrents:        it.torrents,
		Colors:          colors,
		WebDAVPublicURL: "http://localhost:36911/",
	})

	// Cold: fill the TMDB and colour caches.
	movieTitle, err := it.svc.MovieTitle(ctx, movieID)
	if err != nil {
		t.Fatal(err)
	}
	showTitle, err := it.svc.ShowTitle(ctx, showID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := it.svc.Season(ctx, showID, 1); err != nil {
		t.Fatal(err)
	}
	if movieTitle.BackdropDominantColor == nil || showTitle.BackdropDominantColor == nil {
		t.Errorf("colours = %v %v, want them computed inline", movieTitle.BackdropDominantColor, showTitle.BackdropDominantColor)
	}

	// Warm: the budgets of PLAN §6. The mapping review (#5) adds one
	// query to the title pages and seasons, the watch state (B5) one to the
	// title pages; /library reads both in its one, seasons the plays in
	// theirs.
	budgets := []struct {
		name string
		max  int
		fn   func() error
	}{
		{"movie title", 6, func() error { _, err := it.svc.MovieTitle(ctx, movieID); return err }},
		{"show title", 7, func() error { _, err := it.svc.ShowTitle(ctx, showID); return err }},
		{"long season (arcs)", 4, func() error { _, err := it.svc.Season(ctx, showID, 1); return err }},
		{"short season", 2, func() error { _, err := it.svc.Season(ctx, showID, 2); return err }},
		{"library", 1, func() error {
			_, err := it.svc.Library(ctx, LibraryParams{Query: it.tag})
			return err
		}},
		{"library needing review", 1, func() error {
			_, err := it.svc.Library(ctx, LibraryParams{Query: it.tag, Status: StatusNeedsReview})
			return err
		}},
		{"library watching", 1, func() error {
			_, err := it.svc.Library(ctx, LibraryParams{Status: StatusWatching})
			return err
		}},
		{"library by last watched", 1, func() error {
			_, err := it.svc.Library(ctx, LibraryParams{Query: it.tag, Sort: "last_watched"})
			return err
		}},
		{"library lookup", 1, func() error {
			_, err := it.svc.Library(ctx, LibraryParams{IDs: fmt.Sprintf("movie:%d,tv:%d,tv:%d", movieID, showID, it.id(8))})
			return err
		}},
		{"library past the end", 1, func() error {
			_, err := it.svc.Library(ctx, LibraryParams{Query: it.tag, Page: 9})
			return err
		}},
	}
	for _, b := range budgets {
		var err error
		n := it.queries(func() { err = b.fn() })
		if err != nil {
			t.Errorf("%s: %v", b.name, err)
		}
		if n > b.max {
			t.Errorf("%s: %d queries, budget %d", b.name, n, b.max)
		}
		t.Logf("%s: %d queries (budget %d)", b.name, n, b.max)
	}

	// The answers themselves, end to end.
	season, err := it.svc.Season(ctx, showID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(season.Ranges) == 0 || season.Ranges[0].Kind != "arc" || season.Ranges[0].Label != "First Arc" || season.Ranges[1].Label != "25–30" {
		t.Errorf("ranges = %+v", season.Ranges)
	}
	if e := season.Episodes[0]; e.State != EpisodeReady || e.AbsoluteNumber == nil || *e.AbsoluteNumber != 101 ||
		*e.WebDAVPath != "/TV Shows/Show "+it.tag+" (2023)/Season 01/Show "+it.tag+" - S01E01 - Episode 1.mkv" {
		t.Errorf("E1 = %+v", e)
	}
	// E13 is watched: the frontier starts after it.
	if e := season.Episodes[12]; e.State != EpisodeWatched || e.WebDAVPath != nil {
		t.Errorf("E13 = %+v", e)
	}
	lib := showTitle.Library
	if lib.AvailabilityStage != StageNeedsTorrent || lib.Missing == nil || lib.Missing.From != 14 || lib.Missing.To != 30 ||
		*lib.Missing.AbsoluteFrom != 114 || *lib.EpisodesTotal != 38 || *lib.EpisodesReady != 12 {
		t.Errorf("show library = %+v, missing %+v", lib, lib.Missing)
	}
	if ws := showTitle.WatchState; ws == nil || ws.Plays != 1 || ws.WatchedAt == nil || ws.Progress != nil || ws.NextEpisode != nil {
		t.Errorf("show watch state = %+v", ws)
	}
	if ws := movieTitle.WatchState; ws == nil || ws.Plays != 1 {
		t.Errorf("movie watch state = %+v", ws)
	}
	if len(showTitle.Similar) != 2 || showTitle.Similar[0].Library == nil || showTitle.Similar[0].Library.AvailabilityStage != StageNeedsTorrent ||
		showTitle.Similar[1].Library != nil {
		t.Errorf("similar = %+v", showTitle.Similar)
	}
	if !strings.HasSuffix(*movieTitle.Library.WebDAVURL, "/Movies/Movie%20"+it.tag+"%20%282024%29/Movie%20"+it.tag+"%20%282024%29.mkv") {
		t.Errorf("movie url = %s", *movieTitle.Library.WebDAVURL)
	}

	// The lookup Discover and Search use: the two titles in the library,
	// with what their title pages say.
	lookup, err := it.svc.Library(ctx, LibraryParams{IDs: fmt.Sprintf("movie:%d,tv:%d,tv:%d", movieID, showID, it.id(8))})
	if err != nil || lookup.Total != 2 || len(lookup.Items) != 2 || lookup.PageSize != 3 || lookup.Counts.All != 2 || lookup.Summary.Total < 3 {
		t.Fatalf("lookup = %+v, %v", lookup, err)
	}
	for _, item := range lookup.Items {
		switch item.MediaType {
		case "movie":
			if !reflect.DeepEqual(item.WebDAVURL, movieTitle.Library.WebDAVURL) || item.Resolution == nil || *item.Resolution != "1080p" {
				t.Errorf("movie item = %+v", item)
			}
		case "tv":
			if !reflect.DeepEqual(item.Missing, lib.Missing) || !reflect.DeepEqual(item.WebDAVURL, lib.WebDAVURL) || item.Runtime != nil ||
				*item.EpisodesWatched != 1 || item.LastWatchedAt == nil {
				t.Errorf("show item = %+v, missing %+v", item, item.Missing)
			}
		}
	}
}

// The SQL behind /library and "More like this" and the Go rules behind title
// pages must agree on every show's stage and counts.
func TestIntegration_LibraryStagesMatchTitles(t *testing.T) {
	it := newIntegration(t)
	ctx := context.Background()

	type variant struct {
		name  string
		build func(*library.Show)
		// The files of the show's torrent (hash+activeHash) a download
		// wants: those not all on disk, in a state that wants bytes, are
		// downloading.
		active     map[string]downloads.FileStatus
		activeHash string
		size       int64  // the library's size_bytes: each file once
		want       string // "stage ready/total", when set
	}
	hash := "hash-variant-" + it.tag
	variants := []variant{
		{"nothing assigned", func(s *library.Show) { it.season(s, 1, 4, "2020-01-03") }, nil, "", 0, ""},
		{"frontier", func(s *library.Show) {
			ids := it.season(s, 1, 4, "2020-01-03")
			it.assign(library.ItemTypeEpisode, ids[1], hash+"-f", "E01.mkv")
		}, nil, "", 1000, ""},
		{"gap before the last file", func(s *library.Show) {
			ids := it.season(s, 1, 4, "2020-01-03")
			it.assign(library.ItemTypeEpisode, ids[1], hash+"-g", "E01.mkv")
			it.assign(library.ItemTypeEpisode, ids[4], hash+"-g", "E04.mkv")
		}, nil, "", 2000, ""},
		{"upcoming after the last file", func(s *library.Show) {
			ids := it.season(s, 1, 3, time.Now().AddDate(0, 0, -7).Format(time.DateOnly))
			it.assign(library.ItemTypeEpisode, ids[1], hash+"-u", "E01.mkv")
			it.assign(library.ItemTypeEpisode, ids[2], hash+"-u", "E02.mkv")
		}, nil, "", 2000, ""},
		{"downloading", func(s *library.Show) {
			ids := it.season(s, 1, 2, "2020-01-03")
			it.assign(library.ItemTypeEpisode, ids[1], hash+"-d", "E01.mkv")
			it.assign(library.ItemTypeEpisode, ids[2], hash+"-d", "E02.mkv")
		}, map[string]downloads.FileStatus{"E01.mkv": onDisk, "E02.mkv": fetching}, "-d", 2000, ""},
		// A paused download wants nothing now: the show is ready.
		{"download paused", func(s *library.Show) {
			ids := it.season(s, 1, 2, "2020-01-03")
			it.assign(library.ItemTypeEpisode, ids[1], hash+"-p", "E01.mkv")
			it.assign(library.ItemTypeEpisode, ids[2], hash+"-p", "E02.mkv")
		}, map[string]downloads.FileStatus{"E02.mkv": {Bytes: 10, DoneBytes: 3, State: downloads.StatePaused}}, "-p", 2000, "ready 2/2"},
		{"specials hidden", func(s *library.Show) {
			it.exec(`UPDATE shows SET include_specials = FALSE WHERE id = $1`, s.ID)
			it.season(s, 1, 2, "2099-01-01")
			ids := it.season(s, 0, 1, "2020-01-03")
			it.assign(library.ItemTypeEpisode, ids[1], hash+"-s", "SP01.mkv")
		}, nil, "", 0, ""},
		{"special file only", func(s *library.Show) {
			it.season(s, 1, 2, "2099-01-01")
			ids := it.season(s, 0, 1, "2020-01-03")
			it.assign(library.ItemTypeEpisode, ids[1], hash+"-o", "SP01.mkv")
		}, nil, "", 1000, ""},
		// HDRezka files have no info hash and are never downloading.
		{"hdrezka only", func(s *library.Show) {
			ids := it.season(s, 1, 2, "2020-01-03")
			it.stream(library.ItemTypeEpisode, s.ID, ids[1], 1)
			it.stream(library.ItemTypeEpisode, s.ID, ids[2], 2)
		}, nil, "", 2 * streamSize, "ready 2/2"},
		{"mixed sources", func(s *library.Show) {
			ids := it.season(s, 1, 4, "2020-01-03")
			it.stream(library.ItemTypeEpisode, s.ID, ids[1], 1)
			it.assign(library.ItemTypeEpisode, ids[2], hash+"-m", "E02.mkv")
			it.stream(library.ItemTypeEpisode, s.ID, ids[3], 3)
		}, nil, "", 2*streamSize + 1000, "needs_torrent 3/4"},
		{"mixed sources downloading", func(s *library.Show) {
			ids := it.season(s, 1, 2, "2020-01-03")
			it.stream(library.ItemTypeEpisode, s.ID, ids[1], 1)
			it.assign(library.ItemTypeEpisode, ids[2], hash+"-md", "E02.mkv")
		}, map[string]downloads.FileStatus{"E02.mkv": fetching}, "-md", streamSize + 1000, "downloading 2/2"},
		// Plays cover episodes like files (B5): the frontier starts after
		// the last episode with either.
		{"watched, no files", func(s *library.Show) {
			it.season(s, 1, 4, "2020-01-03")
			it.watched(s.TMDBID, 1, 1, 0)
			it.watched(s.TMDBID, 1, 2, 0)
		}, nil, "", 0, "needs_torrent 0/4"},
		{"watched after the last file", func(s *library.Show) {
			ids := it.season(s, 1, 4, "2020-01-03")
			it.assign(library.ItemTypeEpisode, ids[1], hash+"-w", "E01.mkv")
			it.watched(s.TMDBID, 1, 3, 0)
		}, nil, "", 1000, "needs_torrent 1/4"},
		{"watched up to the end", func(s *library.Show) {
			ids := it.season(s, 1, 3, "2020-01-03")
			it.assign(library.ItemTypeEpisode, ids[1], hash+"-e", "E01.mkv")
			it.watched(s.TMDBID, 1, 2, 0)
			it.watched(s.TMDBID, 1, 3, 0)
		}, nil, "", 1000, "ready 1/3"},
		// Trakt numbers E2 as S3E7: TMDB's episode id finds it; a play
		// with E3's number and another id isn't E3's.
		{"watched by TMDB id", func(s *library.Show) {
			ids := it.season(s, 1, 3, "2020-01-03")
			it.assign(library.ItemTypeEpisode, ids[1], hash+"-i", "E01.mkv")
			it.exec(`UPDATE episodes SET tmdb_episode_id = $2 + episode_number WHERE id = ANY($1)`, []int64{ids[2], ids[3]}, s.TMDBID%100000*10)
			it.watched(s.TMDBID, 3, 7, s.TMDBID%100000*10+2)
			it.watched(s.TMDBID, 1, 3, 1)
		}, nil, "", 1000, "needs_torrent 1/3"},
	}

	routes := map[string]any{}
	byTitle := map[string]variant{}
	for i, v := range variants {
		show := it.show(it.id(10+i), fmt.Sprintf("Variant %s %s", it.tag, v.name))
		v.build(show)
		byTitle[show.Title] = v
		routes[fmt.Sprintf("/tv/%d", show.TMDBID)] = tmdb.ShowDetails{ID: show.TMDBID, Name: show.Title}
		for path, f := range v.active {
			it.torrents.loaded[hash+v.activeHash] = fakeTorrent{active: true}
			it.downloads.files[downloads.FileKey{Hash: hash + v.activeHash, Path: path}] = f
		}
	}
	// And a movie that plays from HDRezka.
	movie := it.movie(it.id(30), fmt.Sprintf("Variant %s hdrezka movie", it.tag), "")
	it.stream(library.ItemTypeMovie, movie.ID, movie.ID, 0)
	routes[fmt.Sprintf("/movie/%d", movie.TMDBID)] = tmdb.MovieDetails{ID: movie.TMDBID, Title: movie.Title}
	srv := fakeTMDBServer(t, routes)
	t.Cleanup(func() {
		for i := range variants {
			_, _ = it.db.Exec(`DELETE FROM tmdb_cache WHERE key LIKE $1`, fmt.Sprintf("%%/tv/%d", it.id(10+i)))
		}
		_, _ = it.db.Exec(`DELETE FROM tmdb_cache WHERE key LIKE $1`, fmt.Sprintf("%%/movie/%d", movie.TMDBID))
	})
	it.svc = New(Deps{
		Store:     library.NewCatalogRepository(it.db),
		TMDB:      tmdb.NewCached(tmdb.NewClient("k", tmdb.WithBaseURL(srv.URL), tmdb.WithRetries(1, 0)), library.NewTMDBCacheRepository(it.db)),
		Torrents:  it.torrents,
		Downloads: func() DownloadsSnapshot { return it.downloads },
	})

	page, err := it.svc.Library(ctx, LibraryParams{MediaType: "tv", Query: "Variant " + it.tag, Sort: "title"})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != len(variants) {
		t.Fatalf("library = %d items, want %d", page.Total, len(variants))
	}
	seen := map[Stage]bool{}
	for _, item := range page.Items {
		title, err := it.svc.ShowTitle(ctx, item.TMDBID)
		if err != nil {
			t.Fatal(err)
		}
		lib := title.Library
		if lib.AvailabilityStage != item.AvailabilityStage || *lib.EpisodesTotal != *item.EpisodesTotal || *lib.EpisodesReady != *item.EpisodesReady {
			t.Errorf("%s: title %s %d/%d, library %s %d/%d", item.Title, lib.AvailabilityStage, *lib.EpisodesReady, *lib.EpisodesTotal,
				item.AvailabilityStage, *item.EpisodesReady, *item.EpisodesTotal)
		}
		// The fields the library card reads from the title page's state
		if !reflect.DeepEqual(item.Missing, lib.Missing) || !reflect.DeepEqual(item.WebDAVPath, lib.WebDAVPath) ||
			!reflect.DeepEqual(item.WebDAVURL, lib.WebDAVURL) || !reflect.DeepEqual(item.ReviewJobID, lib.ReviewJobID) ||
			(item.Download == nil) != (lib.Download == nil) || (item.Download != nil && item.Download.Pct != lib.Download.Pct) {
			t.Errorf("%s: library missing %+v path %v download %+v review %v; title missing %+v path %v download %+v review %v",
				item.Title, item.Missing, item.WebDAVPath, item.Download, item.ReviewJobID, lib.Missing, lib.WebDAVPath, lib.Download, lib.ReviewJobID)
		}
		v := byTitle[item.Title]
		if item.SizeBytes != v.size {
			t.Errorf("%s: size %d, want %d", item.Title, item.SizeBytes, v.size)
		}
		if got := fmt.Sprintf("%s %d/%d", item.AvailabilityStage, *item.EpisodesReady, *item.EpisodesTotal); v.want != "" && got != v.want {
			t.Errorf("%s: %s, want %s", item.Title, got, v.want)
		}
		seen[item.AvailabilityStage] = true
	}
	if !seen[StageNeedsTorrent] || !seen[StageReady] || !seen[StageDownloading] {
		t.Errorf("stages covered = %v, want all three", seen)
	}

	// The HDRezka movie is ready on both, with no torrent.
	movies, err := it.svc.Library(ctx, LibraryParams{MediaType: "movie", Query: "Variant " + it.tag})
	if err != nil || movies.Total != 1 {
		t.Fatalf("movies = %+v, %v", movies, err)
	}
	movieTitle, err := it.svc.MovieTitle(ctx, movie.TMDBID)
	if err != nil {
		t.Fatal(err)
	}
	lib := movieTitle.Library
	if item := movies.Items[0]; item.AvailabilityStage != StageReady || lib.AvailabilityStage != StageReady || item.SizeBytes != streamSize ||
		!reflect.DeepEqual(item.WebDAVPath, lib.WebDAVPath) || item.Download != nil || item.Resolution == nil || *item.Resolution != "1080p" {
		t.Errorf("HDRezka movie: library %s %d %v, title %s %v", item.AvailabilityStage, item.SizeBytes, item.WebDAVPath, lib.AvailabilityStage, lib.WebDAVPath)
	}
	if lib.Torrent != nil || lib.Download != nil || lib.WebDAVPath == nil || !strings.HasSuffix(*lib.WebDAVPath, ").mp4") {
		t.Errorf("HDRezka movie library = %+v", lib)
	}
	if s := lib.Stream; s == nil || s.Provider != "hdrezka" || s.Title != it.tag || s.URL != "https://rezka.ag/series/646-show.html" ||
		s.Translator != "LostFilm" || s.Quality != "1080p" || s.SizeBytes != streamSize || s.AddedAt.IsZero() {
		t.Errorf("HDRezka movie stream = %+v", lib.Stream)
	}
}
