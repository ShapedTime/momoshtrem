package library

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

// traktSeed writes what a Trakt sync would import. 0 stands for a NULL id.
type traktSeed struct {
	t  *testing.T
	db *DB
}

func (s traktSeed) exec(query string, args ...any) {
	s.t.Helper()
	if _, err := s.db.Exec(query, args...); err != nil {
		s.t.Fatalf("%s: %v", query, err)
	}
}

func (s traktSeed) watched(show, season, episode, episodeTMDB, plays int, at time.Time) {
	s.t.Helper()
	s.exec(`INSERT INTO trakt_watched_episodes (show_tmdb_id, season, episode, episode_tmdb_id, plays, last_watched_at)
		VALUES ($1, $2, $3, NULLIF($4, 0), $5, $6)`, show, season, episode, episodeTMDB, plays, at)
}

func (s traktSeed) watchedMovie(tmdbID, plays int, at time.Time) {
	s.t.Helper()
	s.exec(`INSERT INTO trakt_watched_movies (tmdb_id, plays, last_watched_at) VALUES ($1, $2, $3)`, tmdbID, plays, at)
}

func (s traktSeed) rating(mediaType string, traktID, tmdbID, rating int) {
	s.t.Helper()
	s.exec(`INSERT INTO trakt_ratings (media_type, trakt_id, tmdb_id, rating, rated_at) VALUES ($1, $2, $3, $4, NOW())`,
		mediaType, traktID, tmdbID, rating)
}

func (s traktSeed) hidden(section string, traktID, tmdbID int) {
	s.t.Helper()
	s.exec(`INSERT INTO trakt_hidden (section, trakt_id, tmdb_id) VALUES ($1, $2, $3)`, section, traktID, tmdbID)
}

// progressRow is a trakt_show_progress row; zero next/last fields are NULL.
type progressRow struct {
	traktID, tmdbID             int
	title                       string
	aired, completed            int
	lastWatched                 time.Time
	lastSeason, lastEpisode     int
	nextSeason, nextEpisode     int
	nextTMDB                    int
	nextTitle                   string
	hasNext, hasLast, noWatchAt bool
}

func (s traktSeed) progress(p progressRow) {
	s.t.Helper()
	var last any = p.lastWatched
	if p.noWatchAt {
		last = nil
	}
	var lastSeason, lastEpisode, nextSeason, nextEpisode any
	if p.hasLast {
		lastSeason, lastEpisode = p.lastSeason, p.lastEpisode
	}
	if p.hasNext {
		nextSeason, nextEpisode = p.nextSeason, p.nextEpisode
	}
	s.exec(`INSERT INTO trakt_show_progress (show_trakt_id, show_tmdb_id, title, aired, completed, last_watched_at,
			last_season, last_episode, next_season, next_episode, next_episode_tmdb_id, next_title)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NULLIF($11, 0), NULLIF($12, ''))`,
		p.traktID, p.tmdbID, p.title, p.aired, p.completed, last, lastSeason, lastEpisode, nextSeason, nextEpisode,
		p.nextTMDB, p.nextTitle)
}

func (s traktSeed) calendar(showTraktID, showTMDB, season, episode, episodeTMDB int, title string, firstAired time.Time) {
	s.t.Helper()
	s.exec(`INSERT INTO trakt_calendar (show_trakt_id, season, episode, show_tmdb_id, episode_tmdb_id, show_title, episode_title,
			first_aired, still_path)
		VALUES ($1, $2, $3, $4, NULLIF($5, 0), 'Calendar show', $6, $7, '/still.jpg')`,
		showTraktID, season, episode, showTMDB, episodeTMDB, title, firstAired)
}

func (s traktSeed) watchlist(mediaType string, traktID, tmdbID int, title string, listed time.Time) {
	s.t.Helper()
	s.exec(`INSERT INTO trakt_watchlist (media_type, trakt_id, tmdb_id, title, year, poster_path, listed_at)
		VALUES ($1, $2, $3, $4, 2020, '/poster.jpg', $5)`, mediaType, traktID, tmdbID, title, listed)
}

func (s traktSeed) play(id int64, mediaType string, tmdbID int, title string, season, episode int, episodeTitle string, at time.Time) {
	s.t.Helper()
	var se, ep any
	if mediaType == "episode" {
		se, ep = season, episode
	}
	s.exec(`INSERT INTO trakt_history (id, media_type, tmdb_id, trakt_id, title, year, season, episode, episode_title, watched_at)
		VALUES ($1, $2, $3, $3::int + 0, $4, 2020, $5, $6, NULLIF($7, ''), $8)`, id, mediaType, tmdbID, title, se, ep, episodeTitle, at)
}

func (s catalogSeed) setTMDBEpisode(episodeID int64, tmdbID int) {
	s.t.Helper()
	if _, err := s.db.Exec(`UPDATE episodes SET tmdb_episode_id = NULLIF($2, 0) WHERE id = $1`, episodeID, tmdbID); err != nil {
		s.t.Fatal(err)
	}
}

// The matching rule: TMDB's episode id when both sides have one, else the
// season and number. The same in every read that matches.
func TestTraktEpisodeMatching(t *testing.T) {
	db := openSchemaDB(t)
	ctx := context.Background()
	repo := NewCatalogRepository(db)
	seed := catalogSeed{t: t, db: db, meta: NewMetadataRepository(db)}
	tr := traktSeed{t: t, db: db}
	at := time.Date(2026, 10, 1, 20, 0, 0, 0, time.UTC)

	show := seed.show("Matching", true)
	s1 := seed.episodes(show, 1, aired, aired, aired, aired, aired)
	s2 := seed.episodes(show, 2, aired)
	for n, id := range map[int]int{1: 1001, 2: 1002, 4: 1004, 5: 1005} { // E3 has no TMDB id
		seed.setTMDBEpisode(s1[n], id)
	}
	seed.setTMDBEpisode(s2[1], 2001)
	tr.watched(show.TMDBID, 1, 1, 1001, 1, at) // the same id
	tr.watched(show.TMDBID, 1, 2, 9999, 1, at) // E2's number, another id: not E2
	tr.watched(show.TMDBID, 1, 3, 1003, 1, at) // the library's E3 has no id: by number
	tr.watched(show.TMDBID, 1, 4, 0, 1, at)    // Trakt has no id: by number
	tr.watched(show.TMDBID, 1, 6, 2001, 1, at) // Trakt's S1E6 is TMDB's S2E1: by id
	want := map[string]bool{"1x1": true, "1x2": false, "1x3": true, "1x4": true, "1x5": false, "2x1": true}

	seasons, err := repo.ShowEpisodes(ctx, show.ID)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, s := range seasons {
		for _, ep := range s.Episodes {
			got[fmt.Sprintf("%dx%d", s.SeasonNumber, ep.EpisodeNumber)] = ep.Watched
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ShowEpisodes watched = %v, want %v", got, want)
	}

	got = map[string]bool{}
	for _, n := range []int{1, 2} {
		view, err := repo.SeasonByTMDBID(ctx, show.TMDBID, n)
		if err != nil {
			t.Fatal(err)
		}
		for _, ep := range view.Season.Episodes {
			got[fmt.Sprintf("%dx%d", n, ep.EpisodeNumber)] = ep.Watched
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("SeasonByTMDBID watched = %v, want %v", got, want)
	}

	listing, err := repo.ListLibrary(ctx, LibraryQuery{Today: today})
	if err != nil || len(listing.Items) != 1 {
		t.Fatalf("library = %+v, %v", listing, err)
	}
	if n := listing.Items[0].EpisodesWatched; n == nil || *n != 4 {
		t.Errorf("episodes_watched = %v, want 4", n)
	}

	// Trakt's episodes to the library's, the other way round.
	keys := []TraktEpisodeKey{
		{ShowTMDBID: show.TMDBID, Season: 1, Episode: 6, EpisodeTMDBID: 2001},
		{ShowTMDBID: show.TMDBID, Season: 1, Episode: 2, EpisodeTMDBID: 9999},
		{ShowTMDBID: show.TMDBID, Season: 1, Episode: 3, EpisodeTMDBID: 1003},
		{ShowTMDBID: show.TMDBID, Season: 1, Episode: 5},
	}
	found, err := repo.EpisodesByKeys(ctx, keys)
	if err != nil {
		t.Fatal(err)
	}
	var places []string
	for _, k := range found {
		if k.Episode == nil {
			places = append(places, "none")
			continue
		}
		places = append(places, fmt.Sprintf("%dx%d", k.SeasonNumber, k.Episode.EpisodeNumber))
	}
	if want := []string{"2x1", "none", "1x3", "1x5"}; !slices.Equal(places, want) {
		t.Errorf("EpisodesByKeys = %v, want %v", places, want)
	}
}

// /library's watch state: episodes_watched, last_watched_at, the frontier
// after the last episode with a file or a play, status=watching and its
// count, sort=last_watched.
func TestListLibrary_WatchState(t *testing.T) {
	db := openSchemaDB(t)
	ctx := context.Background()
	repo := NewCatalogRepository(db)
	seed := catalogSeed{t: t, db: db, meta: NewMetadataRepository(db)}
	tr := traktSeed{t: t, db: db}
	day := func(n int) time.Time { return time.Date(2026, 10, n, 20, 0, 0, 0, time.UTC) }

	// Alpha: E1–E2 watched, no files: the frontier starts at E3.
	alpha := seed.show("Alpha", true)
	seed.episodes(alpha, 1, aired, aired, aired, aired)
	tr.watched(alpha.TMDBID, 1, 1, 0, 1, day(1))
	tr.watched(alpha.TMDBID, 1, 2, 0, 2, day(9))
	// Bravo: all watched, no files: nothing missing, but no file either.
	bravo := seed.show("Bravo", true)
	seed.episodes(bravo, 1, aired, aired)
	tr.watched(bravo.TMDBID, 1, 1, 0, 1, day(2))
	tr.watched(bravo.TMDBID, 1, 2, 0, 1, day(3))
	// Charlie: E1 has a file, E2 is watched, E3 is missing; dropped on
	// Trakt, so it isn't watching.
	charlie := seed.show("Charlie", true)
	c1 := seed.episodes(charlie, 1, aired, aired, aired)
	seed.file(ItemTypeEpisode, c1[1], "hash-charlie", "E01.mkv", 10)
	tr.watched(charlie.TMDBID, 1, 2, 0, 1, day(4))
	tr.hidden("dropped", 7001, charlie.TMDBID)
	// Delta: both have files, E1 watched: ready and watching. A special's
	// play counts for last watched, not for episodes_watched.
	delta := seed.show("Delta", true)
	d1 := seed.episodes(delta, 1, aired, aired)
	seed.episodes(delta, 0, aired)
	seed.file(ItemTypeEpisode, d1[1], "hash-delta", "E01.mkv", 10)
	seed.file(ItemTypeEpisode, d1[2], "hash-delta", "E02.mkv", 10)
	tr.watched(delta.TMDBID, 1, 1, 0, 1, day(5))
	tr.watched(delta.TMDBID, 0, 1, 0, 1, day(6))
	// Echo was watched last of all; Foxtrot never.
	echo := seed.movie("Echo")
	tr.watchedMovie(echo.TMDBID, 3, day(12))
	seed.movie("Foxtrot")

	listing, err := repo.ListLibrary(ctx, LibraryQuery{Today: today, Sort: LibrarySortLastWatched})
	if err != nil {
		t.Fatal(err)
	}
	var order []string
	byTitle := map[string]LibraryItem{}
	for _, item := range listing.Items {
		order = append(order, item.Title)
		byTitle[item.Title] = item
	}
	// Newest play first; never watched last, newest added first.
	if want := []string{"Echo", "Alpha", "Delta", "Charlie", "Bravo", "Foxtrot"}; !slices.Equal(order, want) {
		t.Errorf("sort=last_watched = %v, want %v", order, want)
	}
	if c := listing.Counts.Watching; c != 2 {
		t.Errorf("counts.watching = %d, want 2 (Alpha, Delta)", c)
	}

	type watch struct {
		watched  int
		last     time.Time
		watching bool
		stage    string
		missing  string
	}
	got := func(item LibraryItem) watch {
		w := watch{watching: item.Watching, stage: item.Stage}
		if item.EpisodesWatched != nil {
			w.watched = *item.EpisodesWatched
		}
		if item.LastWatchedAt != nil {
			w.last = item.LastWatchedAt.UTC()
		}
		if m := item.Missing; m != nil {
			w.missing = fmt.Sprintf("S%dE%d–%d", m.SeasonNumber, m.From, m.To)
		}
		return w
	}
	for title, want := range map[string]watch{
		"Alpha":   {2, day(9), true, "needs_torrent", "S1E3–4"},
		"Bravo":   {2, day(3), false, "needs_torrent", ""},
		"Charlie": {1, day(4), false, "needs_torrent", "S1E3–3"},
		"Delta":   {1, day(6), true, "ready", ""},
		"Echo":    {0, day(12), false, "needs_torrent", ""},
		"Foxtrot": {0, time.Time{}, false, "needs_torrent", ""},
	} {
		if g := got(byTitle[title]); g != want {
			t.Errorf("%s = %+v, want %+v", title, g, want)
		}
	}
	if byTitle["Echo"].EpisodesWatched != nil {
		t.Errorf("a movie's episodes_watched = %v, want null", *byTitle["Echo"].EpisodesWatched)
	}

	watching, err := repo.ListLibrary(ctx, LibraryQuery{Today: today, Status: LibraryStatusWatching, Sort: LibrarySortTitle})
	if err != nil {
		t.Fatal(err)
	}
	var titles []string
	for _, item := range watching.Items {
		titles = append(titles, item.Title)
	}
	if !slices.Equal(titles, []string{"Alpha", "Delta"}) || watching.Total != 2 || watching.Counts.All != 6 {
		t.Errorf("status=watching = %v (total %d, all %d)", titles, watching.Total, watching.Counts.All)
	}
}

func TestWatchState(t *testing.T) {
	db := openSchemaDB(t)
	ctx := context.Background()
	repo := NewCatalogRepository(db)
	seed := catalogSeed{t: t, db: db, meta: NewMetadataRepository(db)}
	tr := traktSeed{t: t, db: db}
	at := time.Date(2026, 10, 10, 21, 0, 0, 0, time.UTC)
	soon := time.Date(2026, 10, 20, 1, 30, 0, 0, time.UTC)

	// Movies: plays and a rating, a rating alone, nothing.
	tr.watchedMovie(603, 2, at)
	tr.rating("movie", 1, 603, 9)
	tr.rating("movie", 2, 604, 7)
	ws, err := repo.WatchState(ctx, "movie", 603)
	if err != nil || ws == nil || ws.Plays != 2 || !ws.LastWatchedAt.Equal(at) || *ws.Rating != 9 || ws.Progress != nil || ws.Next != nil {
		t.Errorf("movie 603 = %+v, %v", ws, err)
	}
	if ws, err := repo.WatchState(ctx, "movie", 604); err != nil || ws == nil || ws.Plays != 0 || ws.LastWatchedAt != nil || *ws.Rating != 7 {
		t.Errorf("movie 604 = %+v, %v", ws, err)
	}
	if ws, err := repo.WatchState(ctx, "movie", 605); err != nil || ws != nil {
		t.Errorf("movie 605 = %+v, %v; want nil", ws, err)
	}

	// A library show: Trakt's next episode, named as the library names it
	// (absolute number, name, air date), found by TMDB id.
	golf := seed.show("Golf", true)
	g1 := seed.episodes(golf, 1, aired, aired, "2026-10-21")
	if _, err := db.Exec(`UPDATE episodes SET absolute_number = 100 + episode_number WHERE id = $1`, g1[3]); err != nil {
		t.Fatal(err)
	}
	seed.setTMDBEpisode(g1[3], 5003)
	tr.watched(golf.TMDBID, 1, 1, 0, 2, at.Add(-time.Hour))
	tr.watched(golf.TMDBID, 1, 2, 0, 1, at)
	tr.rating("show", 3, golf.TMDBID, 8)
	tr.progress(progressRow{traktID: 11, tmdbID: golf.TMDBID, title: "Golf", aired: 3, completed: 2, lastWatched: at,
		hasLast: true, lastSeason: 1, lastEpisode: 2, hasNext: true, nextSeason: 9, nextEpisode: 9, nextTMDB: 5003, nextTitle: "Trakt's name"})
	ws, err = repo.WatchState(ctx, "tv", golf.TMDBID)
	if err != nil || ws == nil {
		t.Fatalf("golf = %+v, %v", ws, err)
	}
	if ws.Plays != 3 || !ws.LastWatchedAt.Equal(at) || *ws.Rating != 8 || *ws.Progress != (TraktProgress{2, 3}) {
		t.Errorf("golf = %+v progress %+v", ws, ws.Progress)
	}
	if n := ws.Next; n == nil || n.Season != 1 || n.Episode != 3 || *n.AbsoluteNumber != 103 || n.Name != "Episode 3" || n.AirDate != "2026-10-21" {
		t.Errorf("golf next = %+v", ws.Next)
	}

	// Caught up: the first regular calendar entry after the last watched
	// episode, as Trakt names it (not in the library).
	tr.progress(progressRow{traktID: 12, tmdbID: 7012, title: "Hotel", aired: 10, completed: 10, lastWatched: at,
		hasLast: true, lastSeason: 1, lastEpisode: 10})
	tr.calendar(12, 7012, 1, 9, 0, "Before", at)
	tr.calendar(12, 7012, 0, 3, 0, "A special", at)
	tr.calendar(12, 7012, 2, 2, 0, "Later", soon.Add(7*24*time.Hour))
	tr.calendar(12, 7012, 2, 1, 0, "Premiere", soon)
	ws, err = repo.WatchState(ctx, "tv", 7012)
	if err != nil || ws == nil || ws.Plays != 0 || ws.Next == nil {
		t.Fatalf("hotel = %+v, %v", ws, err)
	}
	if n := *ws.Next; n.Season != 2 || n.Episode != 1 || n.Name != "Premiere" || n.AirDate != "2026-10-20" || n.AbsoluteNumber != nil {
		t.Errorf("hotel next = %+v", n)
	}

	// A special is never next: the calendar names the next regular one.
	tr.progress(progressRow{traktID: 13, tmdbID: 7013, title: "India", aired: 4, completed: 4, lastWatched: at,
		hasLast: true, lastSeason: 1, lastEpisode: 4, hasNext: true, nextSeason: 0, nextEpisode: 1})
	tr.calendar(13, 7013, 1, 5, 0, "Regular", soon)
	if ws, err := repo.WatchState(ctx, "tv", 7013); err != nil || ws.Next == nil || ws.Next.Season != 1 || ws.Next.Episode != 5 {
		t.Errorf("india = %+v, %v", ws, err)
	}

	// Caught up with nothing on the calendar: progress, no next.
	tr.progress(progressRow{traktID: 14, tmdbID: 7014, title: "Juliett", aired: 4, completed: 4, lastWatched: at,
		hasLast: true, lastSeason: 1, lastEpisode: 4})
	if ws, err := repo.WatchState(ctx, "tv", 7014); err != nil || ws == nil || ws.Next != nil || ws.Progress == nil {
		t.Errorf("juliett = %+v, %v", ws, err)
	}
	if ws, err := repo.WatchState(ctx, "tv", 7999); err != nil || ws != nil {
		t.Errorf("unknown show = %+v, %v; want nil", ws, err)
	}
}

func TestUpNextCandidates(t *testing.T) {
	db := openSchemaDB(t)
	ctx := context.Background()
	repo := NewCatalogRepository(db)
	seed := catalogSeed{t: t, db: db, meta: NewMetadataRepository(db)}
	tr := traktSeed{t: t, db: db}
	at := time.Date(2026, 10, 10, 21, 0, 0, 0, time.UTC)
	soon := time.Date(2026, 10, 20, 1, 30, 0, 0, time.UTC)

	tr.progress(progressRow{traktID: 1, tmdbID: 8001, title: "Next", aired: 5, completed: 2, lastWatched: at,
		hasLast: true, lastSeason: 1, lastEpisode: 2, hasNext: true, nextSeason: 1, nextEpisode: 3, nextTMDB: 9003, nextTitle: "Three"})
	tr.calendar(1, 8001, 1, 3, 9003, "Three", soon) // its airing
	tr.progress(progressRow{traktID: 2, tmdbID: 8002, title: "Dropped", aired: 5, completed: 2, lastWatched: at,
		hasNext: true, nextSeason: 1, nextEpisode: 3})
	tr.hidden("dropped", 2, 8002)
	tr.progress(progressRow{traktID: 3, tmdbID: 8003, title: "Done", aired: 5, completed: 5, lastWatched: at})

	unstarted := seed.show("Unstarted", true)
	seed.episodes(unstarted, 1, aired, aired, upcoming)
	seed.episodes(unstarted, 0, aired)
	started := seed.show("Started", true)
	tr.watched(started.TMDBID, 1, 1, 0, 1, at)
	tr.watchlist("tv", 21, unstarted.TMDBID, "Unstarted", at)
	tr.watchlist("tv", 22, started.TMDBID, "Started", at)
	tr.watchlist("tv", 23, 8099, "Not in the library", at)
	tr.watchlist("movie", 24, 603, "A movie", at)

	candidates, err := repo.UpNextCandidates(ctx, today)
	if err != nil {
		t.Fatal(err)
	}
	byShow := map[int]UpNextCandidate{}
	for _, c := range candidates {
		byShow[c.ShowTMDBID] = c
	}
	if len(candidates) != 4 {
		t.Fatalf("candidates = %+v, want the three progress rows and one watchlist start", candidates)
	}
	next := byShow[8001]
	if next.Source != UpNextFromProgress || next.Title != "Next" || next.Completed != 2 || next.Aired != 5 || next.Hidden ||
		!next.LastWatchedAt.Equal(at) || next.Next == nil || next.Next.TraktEpisodeKey != (TraktEpisodeKey{8001, 1, 3, 9003}) ||
		next.Next.Title != "Three" || !next.Next.FirstAired.Equal(soon) || next.Next.FromCalendar {
		t.Errorf("next = %+v (episode %+v)", next, next.Next)
	}
	if c := byShow[8002]; !c.Hidden || c.Next == nil {
		t.Errorf("dropped = %+v", c)
	}
	if c := byShow[8003]; c.Next != nil {
		t.Errorf("done = %+v, want no next episode", c.Next)
	}
	start := byShow[unstarted.TMDBID]
	if start.Source != UpNextFromWatchlist || start.Aired != 2 || start.Completed != 0 || start.LastWatchedAt != nil ||
		!start.ListedAt.Equal(at) || start.Next == nil || start.Next.TraktEpisodeKey != (TraktEpisodeKey{unstarted.TMDBID, 1, 1, 0}) {
		t.Errorf("watchlist start = %+v (episode %+v)", start, start.Next)
	}
}

func TestNewEpisodeCandidates(t *testing.T) {
	db := openSchemaDB(t)
	ctx := context.Background()
	repo := NewCatalogRepository(db)
	seed := catalogSeed{t: t, db: db, meta: NewMetadataRepository(db)}
	tr := traktSeed{t: t, db: db}
	from := time.Date(2026, 10, 7, 0, 0, 0, 0, time.UTC)
	now := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)

	// E1 aired before the window, E2 is watched, E3 and E4 aired in it,
	// E5 hasn't aired; specials never count.
	kilo := seed.show("Kilo", true)
	seed.episodes(kilo, 1, "2026-10-06", "2026-10-08", "2026-10-09", "2026-10-14", "2026-10-15")
	seed.episodes(kilo, 0, "2026-10-10")
	tr.watched(kilo.TMDBID, 1, 2, 0, 1, now)
	// A hidden show's episodes never count.
	lima := seed.show("Lima", true)
	seed.episodes(lima, 1, "2026-10-10")
	tr.hidden("progress_watched", 31, lima.TMDBID)
	// The calendar: one in the window, one after now, a special, a
	// watched one and one of a hidden show.
	tr.calendar(41, 8101, 1, 1, 0, "Aired", time.Date(2026, 10, 13, 2, 0, 0, 0, time.UTC))
	tr.calendar(41, 8101, 1, 2, 0, "Tonight", time.Date(2026, 10, 14, 20, 0, 0, 0, time.UTC))
	tr.calendar(41, 8101, 0, 1, 0, "Special", time.Date(2026, 10, 12, 2, 0, 0, 0, time.UTC))
	tr.calendar(42, 8102, 3, 4, 0, "Seen", time.Date(2026, 10, 12, 2, 0, 0, 0, time.UTC))
	tr.watched(8102, 3, 4, 0, 1, now)
	tr.calendar(43, lima.TMDBID, 1, 2, 0, "Hidden", time.Date(2026, 10, 12, 2, 0, 0, 0, time.UTC))

	candidates, err := repo.NewEpisodeCandidates(ctx, from, now, "2026-10-07", "2026-10-14")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, c := range candidates {
		got = append(got, fmt.Sprintf("%d S%dE%d calendar=%v %s%s", c.ShowTMDBID, c.Season, c.Episode, c.FromCalendar, c.AirDate, c.EpisodeTitle))
	}
	slices.Sort(got)
	want := []string{
		fmt.Sprintf("%d S1E3 calendar=false 2026-10-09", kilo.TMDBID),
		fmt.Sprintf("%d S1E4 calendar=false 2026-10-14", kilo.TMDBID),
		"8101 S1E1 calendar=true Aired",
	}
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("candidates = %q, want %q", got, want)
	}
}

// EpisodesByKeys: the show, the episode, its file and the files it shares
// one with, the review.
func TestEpisodesByKeys(t *testing.T) {
	db := openSchemaDB(t)
	ctx := context.Background()
	repo := NewCatalogRepository(db)
	seed := catalogSeed{t: t, db: db, meta: NewMetadataRepository(db)}

	show := seed.show("Mike", true)
	eps := seed.episodes(show, 1, aired, aired, aired, aired)
	for _, n := range []int{1, 2} { // a two-episode file a job mapped
		if _, err := db.Exec(`
			INSERT INTO torrent_assignments (item_type, item_id, info_hash, magnet_uri, file_path, file_size, match_source, confidence)
			VALUES ('episode', $1, 'hash-mike', 'magnet:?xt=urn:btih:hash-mike', 'Mike/E01E02.mkv', 100, 'rules', 'high')`, eps[n]); err != nil {
			t.Fatal(err)
		}
	}
	job := seedJob(t, db, newShowJob(show, strings.Repeat("m", 40)), JobNeedsReview, []ProposalDraft{
		draft("Pack/E03.mkv", GroupNeedsReview, DecisionPending, eps[3]),
	})

	found, err := repo.EpisodesByKeys(ctx, []TraktEpisodeKey{
		{ShowTMDBID: show.TMDBID, Season: 1, Episode: 1},
		{ShowTMDBID: show.TMDBID, Season: 1, Episode: 3},
		{ShowTMDBID: show.TMDBID, Season: 1, Episode: 9},
		{ShowTMDBID: 8899, Season: 1, Episode: 1},
	})
	if err != nil || len(found) != 4 {
		t.Fatalf("found = %+v, %v", found, err)
	}
	e1, e3, e9, other := found[0], found[1], found[2], found[3]
	if e1.Show == nil || e1.Show.ID != show.ID || e1.Show.Title != "Mike" || e1.SeasonNumber != 1 || e1.Episode == nil ||
		e1.Episode.Assignment == nil || e1.Episode.Assignment.FilePath != "Mike/E01E02.mkv" || e1.ReviewJobID != nil {
		t.Errorf("E1 = %+v", e1)
	}
	if len(e1.FileMates) != 1 || e1.FileMates[0].EpisodeNumber != 2 || !e1.FileMates[0].Assignment.Groupable() {
		t.Errorf("E1's file mates = %+v", e1.FileMates)
	}
	if e3.Episode == nil || e3.Episode.Assignment != nil || e3.ReviewJobID == nil || *e3.ReviewJobID != job.ID || e3.FileMates != nil {
		t.Errorf("E3 = %+v", e3)
	}
	if e9.Show == nil || e9.Episode != nil {
		t.Errorf("E9 = %+v, want the show without an episode", e9)
	}
	if other.Show != nil || other.Episode != nil || other.Key.ShowTMDBID != 8899 {
		t.Errorf("not in the library = %+v", other)
	}
	if found, err := repo.EpisodesByKeys(ctx, nil); err != nil || found != nil {
		t.Errorf("no keys = %+v, %v", found, err)
	}
}

func TestTraktWatchlistAndHistory(t *testing.T) {
	db := openSchemaDB(t)
	ctx := context.Background()
	repo := NewCatalogRepository(db)
	tr := traktSeed{t: t, db: db}
	day := func(n int) time.Time { return time.Date(2026, 10, n, 20, 0, 0, 0, time.UTC) }

	tr.watchlist("movie", 1, 603, "The Matrix", day(1))
	tr.watchlist("tv", 2, 1399, "Game of Thrones", day(3))
	tr.watchlist("movie", 3, 604, "Reloaded", day(2))
	all, err := repo.TraktWatchlist(ctx, "")
	if err != nil || len(all) != 3 || all[0].TMDBID != 1399 || all[1].TMDBID != 604 || all[2].TMDBID != 603 ||
		all[0].Year != 2020 || all[0].PosterPath != "/poster.jpg" || !all[0].ListedAt.Equal(day(3)) {
		t.Errorf("watchlist = %+v, %v", all, err)
	}
	if movies, err := repo.TraktWatchlist(ctx, "movie"); err != nil || len(movies) != 2 || movies[0].MediaType != "movie" {
		t.Errorf("movies = %+v, %v", movies, err)
	}

	tr.play(1, "movie", 603, "The Matrix", 0, 0, "", day(1))
	tr.play(2, "episode", 1399, "Game of Thrones", 1, 1, "Winter Is Coming", day(2))
	tr.play(3, "episode", 1399, "Game of Thrones", 1, 2, "", day(2)) // same time: the higher id first
	tr.play(4, "movie", 604, "Reloaded", 0, 0, "", day(4))
	tr.play(5, "episode", 1399, "Game of Thrones", 1, 3, "Lord Snow", day(3))
	var ids []int64
	for offset := 0; offset < 6; offset += 2 {
		plays, total, err := repo.TraktHistory(ctx, 2, offset)
		if err != nil || total != 5 {
			t.Fatalf("history at %d = %+v, total %d, %v", offset, plays, total, err)
		}
		for _, p := range plays {
			ids = append(ids, p.ID)
		}
	}
	if want := []int64{4, 5, 3, 2, 1}; !slices.Equal(ids, want) {
		t.Errorf("history order = %v, want %v", ids, want)
	}
	plays, total, err := repo.TraktHistory(ctx, 2, 10)
	if err != nil || total != 5 || len(plays) != 0 {
		t.Errorf("past the end = %+v, total %d, %v", plays, total, err)
	}
	plays, _, _ = repo.TraktHistory(ctx, 5, 0)
	if m, e := plays[0], plays[3]; m.MediaType != "movie" || m.Season != nil || m.Episode != nil || m.EpisodeTitle != "" ||
		e.MediaType != "episode" || *e.Season != 1 || *e.Episode != 1 || e.EpisodeTitle != "Winter Is Coming" || e.Year != 2020 {
		t.Errorf("plays = %+v, %+v", m, e)
	}
}

// /library is one query whatever the history: the watched lookups are
// index lookups, so the work on Trakt's plays grows with the plays, not
// with plays × episodes.
func TestLibraryItemsSQL_LargeHistory(t *testing.T) {
	db := openSchemaDB(t)
	ctx := context.Background()
	exec := func(query string) {
		t.Helper()
		if _, err := db.Exec(query); err != nil {
			t.Fatalf("%s: %v", query, err)
		}
	}
	// 40 shows of 2×100 episodes and a 1,500-episode one; every episode
	// watched, some by id and some by number.
	exec(`INSERT INTO shows (tmdb_id, title, year) SELECT 300000 + g, 'Big ' || g, 2020 FROM generate_series(1, 40) g`)
	exec(`INSERT INTO shows (tmdb_id, title, year) VALUES (399999, 'Long', 1999)`)
	exec(`INSERT INTO seasons (show_id, season_number) SELECT s.id, n FROM shows s, generate_series(1, 2) n WHERE s.tmdb_id <> 399999`)
	exec(`INSERT INTO seasons (show_id, season_number) SELECT id, 1 FROM shows WHERE tmdb_id = 399999`)
	exec(`INSERT INTO episodes (season_id, episode_number, name, air_date, tmdb_episode_id)
		SELECT sn.id, e, 'E' || e, '2021-01-01',
		       CASE WHEN e % 3 = 0 THEN NULL ELSE (s.tmdb_id - 300000) * 10000 + sn.season_number * 2000 + e END
		FROM shows s JOIN seasons sn ON sn.show_id = s.id,
		     generate_series(1, CASE WHEN s.tmdb_id = 399999 THEN 1500 ELSE 100 END) e`)
	exec(`INSERT INTO trakt_watched_episodes (show_tmdb_id, season, episode, episode_tmdb_id, plays, last_watched_at)
		SELECT s.tmdb_id, sn.season_number, e.episode_number, e.tmdb_episode_id, 1, NOW() - e.id * INTERVAL '1 minute'
		FROM shows s JOIN seasons sn ON sn.show_id = s.id JOIN episodes e ON e.season_id = sn.id`)
	exec(`ANALYZE`)
	var plays int
	if err := db.QueryRow(`SELECT count(*) FROM trakt_watched_episodes`).Scan(&plays); err != nil {
		t.Fatal(err)
	}

	explain := func(q LibraryQuery) (read, ms float64) {
		t.Helper()
		query, args, err := listLibraryQuery(q)
		if err != nil {
			t.Fatal(err)
		}
		var raw []byte
		if err := db.QueryRowContext(ctx, `EXPLAIN (ANALYZE, FORMAT JSON) `+query, args...).Scan(&raw); err != nil {
			t.Fatal(err)
		}
		var plan []struct {
			Plan          planNode `json:"Plan"`
			ExecutionTime float64  `json:"Execution Time"`
		}
		if err := json.Unmarshal(raw, &plan); err != nil || len(plan) != 1 {
			t.Fatalf("plan: %v", err)
		}
		return plan[0].Plan.rowsRead("trakt_watched_episodes"), plan[0].ExecutionTime
	}

	read, ms := explain(LibraryQuery{Today: today, Sort: LibrarySortLastWatched, Status: LibraryStatusWatching})
	t.Logf("the library, %d plays: %.0f rows of trakt_watched_episodes read, %.1f ms", plays, read, ms)
	if read > 4*float64(plays) {
		t.Errorf("read %.0f rows of trakt_watched_episodes for %d plays: the watched lookups scan instead of seeking", read, plays)
	}
	// A lookup of one show (similar titles, the watchlist) reads its plays.
	read, ms = explain(LibraryQuery{Today: today, OnlyTMDB: &TMDBIDs{Shows: []int{399999}}})
	t.Logf("one 1,500-episode show: %.0f rows of trakt_watched_episodes read, %.1f ms", read, ms)
	if read > 4*1500 {
		t.Errorf("read %.0f rows of trakt_watched_episodes for one show of 1,500 plays", read)
	}
}

// planNode is a node of EXPLAIN (ANALYZE, FORMAT JSON).
type planNode struct {
	Relation      string     `json:"Relation Name"`
	ActualRows    float64    `json:"Actual Rows"`
	ActualLoops   float64    `json:"Actual Loops"`
	RemovedFilter float64    `json:"Rows Removed by Filter"`
	RemovedCheck  float64    `json:"Rows Removed by Index Recheck"`
	Plans         []planNode `json:"Plans"`
}

// rowsRead sums the rows the plan's scans of relation read: the rows they
// returned and removed, over every loop.
func (n planNode) rowsRead(relation string) float64 {
	var sum float64
	if n.Relation == relation {
		sum += (n.ActualRows + n.RemovedFilter + n.RemovedCheck) * n.ActualLoops
	}
	for _, child := range n.Plans {
		sum += child.rowsRead(relation)
	}
	return sum
}
