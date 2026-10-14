package library

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestLikePattern(t *testing.T) {
	tests := map[string]string{
		"":            "",
		"  ":          "",
		"dune":        "%dune%",
		"50%":         `%50\%%`,
		"a_b":         `%a\_b%`,
		`back\slash`:  `%back\\slash%`,
		" frieren  ":  "%frieren%",
		"100%_\\done": `%100\%\_\\done%`,
	}
	for in, want := range tests {
		if got := likePattern(in); got != want {
			t.Errorf("likePattern(%q) = %q, want %q", in, got, want)
		}
	}
}

// catalogSeed builds library rows for the catalog tests and removes them
// afterwards.
type catalogSeed struct {
	t    *testing.T
	db   *DB
	meta *MetadataRepository
}

func (s catalogSeed) movie(title string) *Movie {
	return seedMovie(s.t, s.db, title, 2024)
}

func (s catalogSeed) show(title string, includeSpecials bool) *Show {
	show := seedShow(s.t, s.db, title, 2020)
	if !includeSpecials {
		if _, err := s.db.Exec(`UPDATE shows SET include_specials = FALSE WHERE id = $1`, show.ID); err != nil {
			s.t.Fatal(err)
		}
		show.IncludeSpecials = false
	}
	return show
}

// episodes adds a season whose episodes air on the given dates ("" for
// unknown) and returns their IDs by episode number.
func (s catalogSeed) episodes(show *Show, season int, airDates ...string) map[int]int64 {
	s.t.Helper()
	ctx := context.Background()
	row := &Season{ShowID: show.ID, SeasonNumber: season}
	if _, err := s.meta.UpsertSeason(ctx, row); err != nil {
		s.t.Fatal(err)
	}
	eps := make([]EpisodeMetadata, len(airDates))
	for i, d := range airDates {
		eps[i] = EpisodeMetadata{EpisodeNumber: i + 1, Name: fmt.Sprintf("Episode %d", i+1), AirDate: d}
	}
	if _, err := s.meta.UpsertEpisodes(ctx, row.ID, eps, false); err != nil {
		s.t.Fatal(err)
	}
	ids := map[int]int64{}
	rows, err := s.db.Query(`SELECT episode_number, id FROM episodes WHERE season_id = $1`, row.ID)
	if err != nil {
		s.t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var n int
		var id int64
		if err := rows.Scan(&n, &id); err != nil {
			s.t.Fatal(err)
		}
		ids[n] = id
	}
	return ids
}

// file assigns a torrent file to a movie or episode.
func (s catalogSeed) file(itemType ItemType, itemID int64, hash, path string, size int64) {
	s.t.Helper()
	var id int64
	err := s.db.QueryRow(`
		INSERT INTO torrent_assignments (item_type, item_id, info_hash, magnet_uri, file_path, file_size, resolution, source)
		VALUES ($1, $2, $3, 'magnet:?xt=urn:btih:' || $3 || '&dn=Release.Name', $4, $5, '1080p', 'WEB-DL')
		RETURNING id
	`, string(itemType), itemID, hash, path, size).Scan(&id)
	if err != nil {
		s.t.Fatalf("seed file: %v", err)
	}
	s.t.Cleanup(func() { _, _ = s.db.Exec(`DELETE FROM torrent_assignments WHERE id = $1`, id) })
}

const (
	aired    = "2020-01-01"
	upcoming = "2099-01-01"
	today    = "2026-10-14"
)

func TestCatalogRepository_LibraryItems(t *testing.T) {
	db, counter := openTracedDB(t)
	ctx := context.Background()
	repo := NewCatalogRepository(db)
	seed := catalogSeed{t: t, db: db, meta: NewMetadataRepository(db)}
	tok := fmt.Sprintf("Zq%d", time.Now().UnixNano())

	noFile := seed.movie("Movie NoFile " + tok)
	withFile := seed.movie("Movie WithFile " + tok)
	seed.file(ItemTypeMovie, withFile.ID, "hash-movie", "Movie/movie.mkv", 5000)

	// Ready: a gap before the last assigned episode doesn't count, nor do
	// specials or upcoming episodes. E1 and E2 share a two-episode file.
	ready := seed.show("Show Ready "+tok, true)
	r1 := seed.episodes(ready, 1, aired, aired, aired, aired, upcoming)
	seed.episodes(ready, 0, aired)
	seed.file(ItemTypeEpisode, r1[1], "hash-ready", "S01E01E02.mkv", 700)
	seed.file(ItemTypeEpisode, r1[2], "hash-ready", "S01E01E02.mkv", 700)
	seed.file(ItemTypeEpisode, r1[4], "hash-ready", "S01E04.mkv", 300)

	// Frontier: S01E03 aired after the last assigned episode.
	frontier := seed.show("Show Frontier "+tok, true)
	f1 := seed.episodes(frontier, 1, aired, aired, aired)
	seed.file(ItemTypeEpisode, f1[1], "hash-frontier", "S01E01.mkv", 100)

	// Nothing assigned.
	seed.episodes(seed.show("Show Nothing "+tok, true), 1, aired)

	// Only a special is assigned. With specials hidden, nothing counts.
	specialsOn := seed.show("Show SpecialOn "+tok, true)
	seed.episodes(specialsOn, 1, upcoming)
	seed.file(ItemTypeEpisode, seed.episodes(specialsOn, 0, aired)[1], "hash-special", "special.mkv", 50)
	specialsOff := seed.show("Show SpecialOff "+tok, false)
	seed.episodes(specialsOff, 1, upcoming)
	seed.file(ItemTypeEpisode, seed.episodes(specialsOff, 0, aired)[1], "hash-special", "special2.mkv", 50)

	// Downloading: its only file is being fetched.
	downloading := seed.show("Show Downloading "+tok, true)
	seed.file(ItemTypeEpisode, seed.episodes(downloading, 1, aired)[1], "hash-dl", "dl.mkv", 900)

	query := LibraryQuery{
		Search:      tok,
		Today:       today,
		Sort:        LibrarySortTitle,
		Downloading: []FileRef{{InfoHash: "hash-dl", Path: "dl.mkv"}, {InfoHash: "hash-movie", Path: "other.mkv"}},
	}

	before := counter.n.Load()
	items, total, err := repo.LibraryItems(ctx, query)
	if err != nil {
		t.Fatal(err)
	}
	if n := counter.n.Load() - before; n != 1 {
		t.Errorf("queries = %d, want 1", n)
	}
	if total != 8 || len(items) != 8 {
		t.Fatalf("total = %d, items = %d, want 8", total, len(items))
	}

	type want struct {
		stage        string
		total, ready int // -1: null (movies)
		size         int64
		mediaType    string
	}
	wants := map[string]want{
		"Movie NoFile":     {"needs_torrent", -1, -1, 0, "movie"},
		"Movie WithFile":   {"ready", -1, -1, 5000, "movie"},
		"Show Ready":       {"ready", 4, 3, 1000, "tv"},
		"Show Frontier":    {"needs_torrent", 3, 1, 100, "tv"},
		"Show Nothing":     {"needs_torrent", 1, 0, 0, "tv"},
		"Show SpecialOn":   {"ready", 0, 0, 50, "tv"},
		"Show SpecialOff":  {"needs_torrent", 0, 0, 0, "tv"},
		"Show Downloading": {"downloading", 1, 1, 900, "tv"},
	}
	for _, item := range items {
		name := item.Title[:len(item.Title)-len(tok)-1]
		w, ok := wants[name]
		if !ok {
			t.Errorf("unexpected item %q", item.Title)
			continue
		}
		if item.Stage != w.stage || item.SizeBytes != w.size || item.MediaType != w.mediaType {
			t.Errorf("%s: stage %s size %d type %s, want %s %d %s", name, item.Stage, item.SizeBytes, item.MediaType, w.stage, w.size, w.mediaType)
		}
		if w.total < 0 {
			if item.EpisodesTotal != nil || item.EpisodesReady != nil {
				t.Errorf("%s: movie episode counts = %v/%v, want nil", name, item.EpisodesTotal, item.EpisodesReady)
			}
		} else if item.EpisodesTotal == nil || *item.EpisodesTotal != w.total || *item.EpisodesReady != w.ready {
			t.Errorf("%s: episodes %v/%v, want %d/%d", name, item.EpisodesReady, item.EpisodesTotal, w.ready, w.total)
		}
		if item.ID == 0 || item.TMDBID == 0 || item.CreatedAt.IsZero() {
			t.Errorf("%s: missing identity %+v", name, item)
		}
	}
	if items[0].Title != "Movie NoFile "+tok || items[7].Title != "Show SpecialOn "+tok {
		t.Errorf("title order = %q … %q", items[0].Title, items[7].Title)
	}

	t.Run("filters", func(t *testing.T) {
		count := func(q LibraryQuery) int {
			t.Helper()
			items, total, err := repo.LibraryItems(ctx, q)
			if err != nil {
				t.Fatal(err)
			}
			if len(items) != total {
				t.Errorf("items %d != total %d", len(items), total)
			}
			return total
		}
		q := query
		q.Status = LibraryStatusReady
		if n := count(q); n != 3 {
			t.Errorf("ready = %d, want 3", n)
		}
		q.Status = LibraryStatusIncomplete
		if n := count(q); n != 5 {
			t.Errorf("incomplete = %d, want 5", n)
		}
		q = query
		q.MediaType = "movie"
		if n := count(q); n != 2 {
			t.Errorf("movies = %d, want 2", n)
		}
		q.MediaType = "tv"
		if n := count(q); n != 6 {
			t.Errorf("shows = %d, want 6", n)
		}
		q = query
		q.Search = "Frontier " + tok
		if n := count(q); n != 1 {
			t.Errorf("search = %d, want 1", n)
		}
		q.Search = "%" + tok
		if n := count(q); n != 0 {
			t.Errorf("search with a literal %% = %d, want 0", n)
		}
	})

	t.Run("downloading movie", func(t *testing.T) {
		q := query
		q.MediaType = "movie"
		q.Downloading = []FileRef{{InfoHash: "hash-movie", Path: "Movie/movie.mkv"}}
		items, _, err := repo.LibraryItems(ctx, q)
		if err != nil {
			t.Fatal(err)
		}
		stages := map[string]string{}
		for _, item := range items {
			stages[item.Title] = item.Stage
		}
		if stages["Movie WithFile "+tok] != "downloading" || stages["Movie NoFile "+tok] != "needs_torrent" {
			t.Errorf("stages = %v", stages)
		}
		q.Status = LibraryStatusIncomplete
		if _, total, err := repo.LibraryItems(ctx, q); err != nil || total != 2 {
			t.Errorf("incomplete movies = %d, %v; want 2 (downloading counts)", total, err)
		}
	})

	t.Run("paging", func(t *testing.T) {
		q := query
		q.Limit, q.Offset = 3, 3
		items, total, err := repo.LibraryItems(ctx, q)
		if err != nil || total != 8 || len(items) != 3 || items[0].Title != "Show Frontier "+tok {
			t.Fatalf("page 2 = %d items, total %d, err %v", len(items), total, err)
		}

		q.Offset = 30
		before := counter.n.Load()
		items, total, err = repo.LibraryItems(ctx, q)
		if err != nil || total != 8 || len(items) != 0 {
			t.Fatalf("past the end = %d items, total %d, err %v", len(items), total, err)
		}
		if n := counter.n.Load() - before; n != 2 {
			t.Errorf("queries past the end = %d, want 2", n)
		}
	})

	t.Run("sorts", func(t *testing.T) {
		for _, sort := range []string{LibrarySortAdded, LibrarySortLastWatched, LibrarySortSize} {
			q := query
			q.Sort = sort
			items, _, err := repo.LibraryItems(ctx, q)
			if err != nil || len(items) != 8 {
				t.Fatalf("%s: %d items, %v", sort, len(items), err)
			}
			if sort == LibrarySortSize && items[0].Title != "Movie WithFile "+tok {
				t.Errorf("largest first = %q", items[0].Title)
			}
			if sort == LibrarySortAdded && items[0].Title != "Show Downloading "+tok {
				t.Errorf("newest first = %q", items[0].Title)
			}
		}
		q := query
		q.Sort = "rating; DROP TABLE movies"
		if _, _, err := repo.LibraryItems(ctx, q); err == nil {
			t.Error("unknown sort accepted")
		}
	})

	t.Run("restricted to TMDB ids", func(t *testing.T) {
		before := counter.n.Load()
		items, _, err := repo.LibraryItems(ctx, LibraryQuery{
			Today:    today,
			OnlyTMDB: &TMDBIDs{Movies: []int{withFile.TMDBID, frontier.TMDBID}, Shows: []int{frontier.TMDBID}},
		})
		if err != nil {
			t.Fatal(err)
		}
		if n := counter.n.Load() - before; n != 1 {
			t.Errorf("queries = %d, want 1", n)
		}
		if len(items) != 2 {
			t.Fatalf("items = %+v, want the movie and the show", items)
		}
		_ = noFile
	})
}

// Assignment.Create deactivates the old one, but if two are ever active the
// newest wins everywhere: title pages, /library stage and size.
func TestCatalogRepository_NewestActiveAssignmentWins(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	repo := NewCatalogRepository(db)
	seed := catalogSeed{t: t, db: db, meta: NewMetadataRepository(db)}
	tok := fmt.Sprintf("Zn%d", time.Now().UnixNano())

	file := func(itemType ItemType, itemID int64, path string, size int64, age time.Duration) {
		t.Helper()
		var id int64
		err := db.QueryRow(`
			INSERT INTO torrent_assignments (item_type, item_id, info_hash, magnet_uri, file_path, file_size, is_active, created_at)
			VALUES ($1, $2, 'hash-' || $3, 'magnet:?xt=urn:btih:x', $3, $4, TRUE, NOW() - make_interval(secs => $5))
			RETURNING id
		`, string(itemType), itemID, path, size, age.Seconds()).Scan(&id)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM torrent_assignments WHERE id = $1`, id) })
	}

	movie := seed.movie("Newest Movie " + tok)
	file(ItemTypeMovie, movie.ID, "old.mkv", 9000, time.Hour)
	file(ItemTypeMovie, movie.ID, "new.mkv", 3000, time.Minute)
	show := seed.show("Newest Show "+tok, true)
	ep := seed.episodes(show, 1, aired)[1]
	file(ItemTypeEpisode, ep, "old-ep.mkv", 800, time.Hour)
	file(ItemTypeEpisode, ep, "new-ep.mkv", 200, time.Minute)

	got, err := repo.MovieByTMDBID(ctx, movie.TMDBID)
	if err != nil || got.Assignment == nil || got.Assignment.FilePath != "new.mkv" {
		t.Errorf("movie assignment = %+v, %v; want new.mkv", got.Assignment, err)
	}
	seasons, err := repo.ShowEpisodes(ctx, show.ID)
	if err != nil || len(seasons) != 1 || seasons[0].Episodes[0].Assignment.FilePath != "new-ep.mkv" {
		t.Errorf("show episodes = %+v, %v; want new-ep.mkv", seasons, err)
	}
	view, err := repo.SeasonByTMDBID(ctx, show.TMDBID, 1)
	if err != nil || view.Season.Episodes[0].Assignment.FilePath != "new-ep.mkv" {
		t.Errorf("season view = %+v, %v; want new-ep.mkv", view, err)
	}

	// The old files being fetched doesn't make the titles downloading.
	items, _, err := repo.LibraryItems(ctx, LibraryQuery{Search: tok, Today: today, Downloading: []FileRef{
		{InfoHash: "hash-old.mkv", Path: "old.mkv"}, {InfoHash: "hash-old-ep.mkv", Path: "old-ep.mkv"},
	}})
	if err != nil || len(items) != 2 {
		t.Fatalf("library = %+v, %v", items, err)
	}
	for _, item := range items {
		want := map[string]int64{"movie": 3000, "tv": 200}[item.MediaType]
		if item.Stage != "ready" || item.SizeBytes != want {
			t.Errorf("%s: stage %s size %d, want ready %d", item.Title, item.Stage, item.SizeBytes, want)
		}
	}
}

func TestCatalogRepository_TitleQueries(t *testing.T) {
	db, counter := openTracedDB(t)
	ctx := context.Background()
	repo := NewCatalogRepository(db)
	seed := catalogSeed{t: t, db: db, meta: NewMetadataRepository(db)}

	movie := seed.movie("Catalog Movie")
	seed.file(ItemTypeMovie, movie.ID, "hash-m", "Release/movie.mkv", 1234)
	show := seed.show("Catalog Show", true)
	s1 := seed.episodes(show, 1, aired, "", upcoming)
	seed.episodes(show, 0, aired)
	seed.episodes(show, 2) // announced, no episodes yet
	seed.file(ItemTypeEpisode, s1[2], "hash-s", "S01E02.mkv", 99)

	count := func(name string, want int32, fn func()) {
		t.Helper()
		before := counter.n.Load()
		fn()
		if n := counter.n.Load() - before; n != want {
			t.Errorf("%s: queries = %d, want %d", name, n, want)
		}
	}

	count("MovieByTMDBID", 1, func() {
		got, err := repo.MovieByTMDBID(ctx, movie.TMDBID)
		if err != nil || got == nil || got.Assignment == nil || got.Assignment.FilePath != "Release/movie.mkv" ||
			got.Assignment.FileSize != 1234 || got.Assignment.Resolution != "1080p" || got.Assignment.MagnetURI == "" {
			t.Errorf("movie = %+v (%+v), err %v", got, got.Assignment, err)
		}
	})
	count("MovieByTMDBID missing", 1, func() {
		if got, err := repo.MovieByTMDBID(ctx, -5); got != nil || err != nil {
			t.Errorf("missing movie = %v, %v", got, err)
		}
	})
	count("ListWithActiveAssignment", 1, func() {
		movies, err := NewMovieRepository(db).ListWithActiveAssignment(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range movies {
			if m.ID == movie.ID && (m.Assignment == nil || m.Assignment.InfoHash != "hash-m") {
				t.Errorf("listed movie assignment = %+v", m.Assignment)
			}
		}
	})
	count("ShowByTMDBID", 1, func() {
		if got, err := repo.ShowByTMDBID(ctx, show.TMDBID); err != nil || got == nil || got.ID != show.ID {
			t.Errorf("show = %+v, %v", got, err)
		}
	})
	count("ShowEpisodes", 1, func() {
		seasons, err := repo.ShowEpisodes(ctx, show.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(seasons) != 3 || seasons[0].SeasonNumber != 0 || seasons[2].SeasonNumber != 2 || len(seasons[2].Episodes) != 0 {
			t.Fatalf("seasons = %+v", seasons)
		}
		eps := seasons[1].Episodes
		if len(eps) != 3 || eps[0].AirDate != aired || eps[1].AirDate != "" || eps[1].Assignment == nil ||
			eps[1].Assignment.InfoHash != "hash-s" || eps[0].Assignment != nil || eps[1].Name != "" {
			t.Errorf("season 1 episodes = %+v", eps)
		}
	})
	count("SeasonByTMDBID", 1, func() {
		view, err := repo.SeasonByTMDBID(ctx, show.TMDBID, 1)
		if err != nil || view == nil || view.Season == nil {
			t.Fatalf("season view = %+v, %v", view, err)
		}
		if view.ShowID != show.ID || view.ShowTitle != "Catalog Show" || !view.IncludeSpecials || len(view.Season.Episodes) != 3 ||
			view.Season.Episodes[1].Name != "Episode 2" || view.Season.Episodes[1].Assignment == nil {
			t.Errorf("season view = %+v", view)
		}
	})
	count("SeasonByTMDBID empty season", 1, func() {
		view, err := repo.SeasonByTMDBID(ctx, show.TMDBID, 2)
		if err != nil || view == nil || view.Season == nil || len(view.Season.Episodes) != 0 {
			t.Errorf("empty season = %+v, %v", view, err)
		}
	})
	count("SeasonByTMDBID unknown season", 1, func() {
		view, err := repo.SeasonByTMDBID(ctx, show.TMDBID, 9)
		if err != nil || view == nil || view.Season != nil {
			t.Errorf("unknown season = %+v, %v", view, err)
		}
	})
	count("SeasonByTMDBID unknown show", 1, func() {
		if view, err := repo.SeasonByTMDBID(ctx, -1, 1); view != nil || err != nil {
			t.Errorf("unknown show = %+v, %v", view, err)
		}
	})
}
