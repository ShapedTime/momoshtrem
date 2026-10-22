package library

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"
)

// Two household members' Trakt rows (PLAN §6 "Household members"): what's
// watched is the household's, a union that never repeats a title; /me and
// the taste are each member's; a member's connection, sync and purge touch
// their rows only.

// twoMembers adds Sam (member 2) beside the seeded member 1, and a seeder
// for each.
func twoMembers(t *testing.T, db *DB) (a, b traktSeed) {
	t.Helper()
	sam, err := NewMemberRepository(db).Create(context.Background(), "Sam", "")
	if err != nil || sam.ID != 2 {
		t.Fatalf("Create(Sam) = %+v, %v", sam, err)
	}
	return traktSeed{t: t, db: db, member: 1}, traktSeed{t: t, db: db, member: sam.ID}
}

func householdDay(n int) time.Time { return time.Date(2026, 10, n, 20, 0, 0, 0, time.UTC) }

// /library: a movie both members watched is one item, at the later play;
// watching is a show some member with a play of it hasn't dropped or
// hidden.
func TestHousehold_ListLibrary(t *testing.T) {
	db := openSchemaDB(t)
	ctx := context.Background()
	repo := NewCatalogRepository(db)
	seed := catalogSeed{t: t, db: db, meta: NewMetadataRepository(db)}
	a, b := twoMembers(t, db)
	day := householdDay

	echo := seed.movie("Echo")
	a.watchedMovie(echo.TMDBID, 2, day(3))
	b.watchedMovie(echo.TMDBID, 1, day(5))
	seed.movie("Foxtrot")
	// Alpha: A dropped it, B plays on: watching.
	alpha := seed.show("Alpha", true)
	seed.episodes(alpha, 1, aired, aired, aired)
	a.watched(alpha.TMDBID, 1, 1, 0, 1, day(1))
	a.hidden("dropped", 101, alpha.TMDBID)
	b.watched(alpha.TMDBID, 1, 2, 0, 1, day(7))
	// Bravo: A dropped it, and only A played it: not watching.
	bravo := seed.show("Bravo", true)
	seed.episodes(bravo, 1, aired, aired)
	a.watched(bravo.TMDBID, 1, 1, 0, 1, day(2))
	a.hidden("dropped", 102, bravo.TMDBID)
	// Charlie: both played it, and both dropped or hid it: not watching.
	charlie := seed.show("Charlie", true)
	seed.episodes(charlie, 1, aired, aired)
	a.watched(charlie.TMDBID, 1, 1, 0, 1, day(2))
	b.watched(charlie.TMDBID, 1, 1, 0, 1, day(4))
	a.hidden("dropped", 103, charlie.TMDBID)
	b.hidden("progress_watched", 103, charlie.TMDBID)
	// Delta: A plays it; B hid it, but never played it: watching.
	delta := seed.show("Delta", true)
	seed.episodes(delta, 1, aired, aired)
	a.watched(delta.TMDBID, 1, 1, 0, 1, day(6))
	b.hidden("dropped", 104, delta.TMDBID)
	// Both played the same episode: it counts once.
	b.watched(delta.TMDBID, 1, 1, 0, 3, day(8))

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
	if want := []string{"Delta", "Alpha", "Echo", "Charlie", "Bravo", "Foxtrot"}; !slices.Equal(order, want) {
		t.Errorf("sort=last_watched = %v, want %v (each title once)", order, want)
	}
	if listing.Total != 6 || listing.Counts.All != 6 || listing.Counts.Movies != 2 || listing.Counts.Shows != 4 || listing.Summary.Total != 6 {
		t.Errorf("total %d, counts %+v, summary %+v; want 6 titles", listing.Total, listing.Counts, listing.Summary)
	}
	if last := byTitle["Echo"].LastWatchedAt; last == nil || !last.Equal(day(5)) {
		t.Errorf("Echo's last_watched_at = %v, want the later play", last)
	}
	if last := byTitle["Alpha"].LastWatchedAt; last == nil || !last.Equal(day(7)) {
		t.Errorf("Alpha's last_watched_at = %v, want B's play", last)
	}
	for title, want := range map[string]int{"Alpha": 2, "Bravo": 1, "Charlie": 1, "Delta": 1} {
		if n := byTitle[title].EpisodesWatched; n == nil || *n != want {
			t.Errorf("%s's episodes_watched = %v, want %d", title, n, want)
		}
	}
	if listing.Counts.Watching != 2 {
		t.Errorf("counts.watching = %d, want 2 (Alpha, Delta)", listing.Counts.Watching)
	}

	watching, err := repo.ListLibrary(ctx, LibraryQuery{Today: today, Status: LibraryStatusWatching, Sort: LibrarySortTitle})
	if err != nil {
		t.Fatal(err)
	}
	var titles []string
	for _, item := range watching.Items {
		titles = append(titles, item.Title)
	}
	if !slices.Equal(titles, []string{"Alpha", "Delta"}) || watching.Total != 2 {
		t.Errorf("status=watching = %v (total %d), want Alpha and Delta", titles, watching.Total)
	}
}

// The title pages: every member's plays (summed, the latest), the viewer's
// rating, and the progress and next episode of the member who played the
// show last, from that member's calendar.
func TestHousehold_WatchState(t *testing.T) {
	db := openSchemaDB(t)
	ctx := context.Background()
	repo := NewCatalogRepository(db)
	seed := catalogSeed{t: t, db: db, meta: NewMetadataRepository(db)}
	a, b := twoMembers(t, db)
	day := householdDay

	// A movie both watched and rated.
	a.watchedMovie(603, 2, day(3))
	b.watchedMovie(603, 1, day(5))
	a.rating("movie", 481, 603, 9)
	b.rating("movie", 481, 603, 4)
	for _, tt := range []struct {
		viewer int64
		rating *int
	}{{1, intPtr(9)}, {2, intPtr(4)}, {0, nil}} {
		ws, err := repo.WatchState(ctx, tt.viewer, "movie", 603)
		if err != nil || ws == nil || ws.Plays != 3 || !ws.LastWatchedAt.Equal(day(5)) || !equalIntPtr(ws.Rating, tt.rating) {
			t.Errorf("viewer %d: movie 603 = %+v, %v; want 3 plays, the later play and rating %v", tt.viewer, ws, err, deref2(tt.rating))
		}
	}
	// A movie only B rated: nothing for A.
	b.rating("movie", 482, 604, 7)
	if ws, err := repo.WatchState(ctx, 1, "movie", 604); err != nil || ws != nil {
		t.Errorf("A's movie 604 = %+v, %v; want nil", ws, err)
	}
	if ws, err := repo.WatchState(ctx, 2, "movie", 604); err != nil || ws == nil || ws.Plays != 0 || *ws.Rating != 7 {
		t.Errorf("B's movie 604 = %+v, %v", ws, err)
	}

	// A show: A watched E1–E2 and rated it, B watched E1 twice, later. B's
	// progress and next episode win.
	golf := seed.show("Golf", true)
	seed.episodes(golf, 1, aired, aired, aired, aired, aired, aired)
	a.watched(golf.TMDBID, 1, 1, 0, 1, day(2))
	a.watched(golf.TMDBID, 1, 2, 0, 1, day(2))
	b.watched(golf.TMDBID, 1, 1, 0, 2, day(6))
	a.rating("show", 11, golf.TMDBID, 8)
	a.progress(progressRow{traktID: 11, tmdbID: golf.TMDBID, title: "Golf", aired: 6, completed: 2, lastWatched: day(2),
		hasLast: true, lastSeason: 1, lastEpisode: 2, hasNext: true, nextSeason: 1, nextEpisode: 3})
	b.progress(progressRow{traktID: 11, tmdbID: golf.TMDBID, title: "Golf", aired: 6, completed: 1, lastWatched: day(6),
		hasLast: true, lastSeason: 1, lastEpisode: 1, hasNext: true, nextSeason: 1, nextEpisode: 2})
	for _, tt := range []struct {
		viewer int64
		rating *int
	}{{1, intPtr(8)}, {2, nil}} {
		ws, err := repo.WatchState(ctx, tt.viewer, "tv", golf.TMDBID)
		if err != nil || ws == nil {
			t.Fatalf("viewer %d: golf = %+v, %v", tt.viewer, ws, err)
		}
		if ws.Plays != 4 || !ws.LastWatchedAt.Equal(day(6)) || !equalIntPtr(ws.Rating, tt.rating) ||
			ws.Progress == nil || *ws.Progress != (TraktProgress{Completed: 1, Aired: 6}) || ws.Next == nil || ws.Next.Episode != 2 {
			t.Errorf("viewer %d: golf = %+v (progress %+v, next %+v)", tt.viewer, ws, ws.Progress, ws.Next)
		}
	}

	// Played last at the same time: the lower member's.
	a.progress(progressRow{traktID: 12, tmdbID: 7012, title: "Hotel", aired: 9, completed: 2, lastWatched: day(4),
		hasNext: true, nextSeason: 1, nextEpisode: 3})
	b.progress(progressRow{traktID: 12, tmdbID: 7012, title: "Hotel", aired: 9, completed: 4, lastWatched: day(4),
		hasNext: true, nextSeason: 1, nextEpisode: 5})
	if ws, err := repo.WatchState(ctx, 2, "tv", 7012); err != nil || ws == nil || ws.Next == nil || ws.Next.Episode != 3 || ws.Progress.Completed != 2 {
		t.Errorf("hotel = %+v, %v; want A's (the lower id)", ws, err)
	}

	// Caught up: B played last and has nothing on their calendar, so no
	// next episode, even though A's calendar has one.
	a.progress(progressRow{traktID: 13, tmdbID: 7013, title: "India", aired: 4, completed: 4, lastWatched: day(1),
		hasLast: true, lastSeason: 1, lastEpisode: 4})
	a.calendar(13, 7013, 1, 5, 0, "A's calendar", day(20))
	b.progress(progressRow{traktID: 13, tmdbID: 7013, title: "India", aired: 4, completed: 4, lastWatched: day(9),
		hasLast: true, lastSeason: 1, lastEpisode: 4})
	if ws, err := repo.WatchState(ctx, 1, "tv", 7013); err != nil || ws == nil || ws.Next != nil || ws.Progress == nil {
		t.Errorf("india = %+v, %v; want B's progress without a next episode", ws, err)
	}
	// B's next episode isn't named from A's calendar either.
	b.progress(progressRow{traktID: 14, tmdbID: 7014, title: "Juliett", aired: 4, completed: 3, lastWatched: day(9),
		hasNext: true, nextSeason: 1, nextEpisode: 4})
	a.calendar(14, 7014, 1, 4, 0, "A's name", day(20))
	if ws, err := repo.WatchState(ctx, 1, "tv", 7014); err != nil || ws == nil || ws.Next == nil || ws.Next.Name != "" || ws.Next.AirDate != "" {
		t.Errorf("juliett = %+v, %v; want B's next episode, unnamed", ws, err)
	}
	// A show only B rated: nothing for A.
	b.rating("show", 15, 7015, 6)
	if ws, err := repo.WatchState(ctx, 1, "tv", 7015); err != nil || ws != nil {
		t.Errorf("A's show 7015 = %+v, %v; want nil", ws, err)
	}
}

// Downloads: watched is any member's latest play. The movie lookup is one
// value with two members' rows (it used to fail with "more than one row").
func TestHousehold_Downloads(t *testing.T) {
	f := newDownloadsFixture(t)
	ctx := context.Background()
	a, b := twoMembers(t, f.db)
	a.watchedMovie(f.movie.TMDBID, 1, householdDay(2))
	b.watchedMovie(f.movie.TMDBID, 1, householdDay(4))
	// S1E1 (TMDB 9011) is watched by B only, as Trakt numbers it S9E9.
	b.watched(f.show.TMDBID, 9, 9, 9011, 1, householdDay(3))
	a.watched(f.show.TMDBID, 9, 9, 9011, 1, householdDay(1))

	if _, err := f.repo.CreateDownload(ctx, DownloadRecord{Reason: DownloadKept, Scope: DownloadScopeMovie, MovieID: f.movie.ID, UntilWatched: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.repo.CreateDownload(ctx, DownloadRecord{Reason: DownloadKept, Scope: DownloadScopeSeason, SeasonID: f.seasons[1],
		UntilWatched: true}); err != nil {
		t.Fatal(err)
	}
	resolved, err := f.repo.ResolveDownloads(ctx)
	if err != nil || len(resolved) != 2 {
		t.Fatalf("ResolveDownloads = %+v, %v", resolved, err)
	}
	movie, season := resolved[0].Items, resolved[1].Items
	if len(movie) != 1 || movie[0].WatchedAt == nil || !movie[0].WatchedAt.Equal(householdDay(4)) {
		t.Errorf("the movie's items = %+v, want one watched at B's play", movie)
	}
	if len(season) != 2 || season[0].WatchedAt == nil || !season[0].WatchedAt.Equal(householdDay(3)) || season[1].WatchedAt != nil {
		t.Errorf("the season's items = %+v, want E1 watched at B's play", season)
	}

	// A pick of an episode only B watched is blocked as watched.
	b.watched(f.show.TMDBID, 2, 1, 0, 1, householdDay(5))
	if reason, err := f.repo.SmartPickBlocked(ctx, f.episodes[[2]int{2, 1}]); err != nil || reason != "watched" {
		t.Errorf("SmartPickBlocked(S2E1) = %q, %v; want watched", reason, err)
	}
	if reason, err := f.repo.SmartPickBlocked(ctx, f.episodes[[2]int{2, 2}]); err != nil || reason != "" {
		t.Errorf("SmartPickBlocked(S2E2) = %q, %v; want none", reason, err)
	}
}

// /me and the taste are the member's own: the other member's plays,
// progress, calendar, hidden shows, watchlist, history and ratings never
// show.
func TestHousehold_PersonalReads(t *testing.T) {
	db := openSchemaDB(t)
	ctx := context.Background()
	repo := NewCatalogRepository(db)
	seed := catalogSeed{t: t, db: db, meta: NewMetadataRepository(db)}
	a, b := twoMembers(t, db)
	day := householdDay

	// Up next: A's progress on 8001, which B hid and has on their
	// calendar; B's on 8002. A's watchlist start, which only B played.
	a.progress(progressRow{traktID: 1, tmdbID: 8001, title: "A's show", aired: 5, completed: 2, lastWatched: day(2),
		hasNext: true, nextSeason: 1, nextEpisode: 3})
	b.hidden("dropped", 1, 8001)
	b.calendar(1, 8001, 1, 3, 0, "B's airing", day(20))
	b.progress(progressRow{traktID: 2, tmdbID: 8002, title: "B's show", aired: 5, completed: 1, lastWatched: day(3),
		hasNext: true, nextSeason: 1, nextEpisode: 2})
	unstarted := seed.show("Unstarted", true)
	seed.episodes(unstarted, 1, aired, aired)
	a.watchlist("tv", 21, unstarted.TMDBID, "Unstarted", day(1))
	b.watched(unstarted.TMDBID, 1, 1, 0, 1, day(4))

	upNext := func(member int64) []string {
		t.Helper()
		candidates, err := repo.UpNextCandidates(ctx, member, today)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, c := range candidates {
			s := fmt.Sprintf("%s %d hidden=%v", c.Source, c.ShowTMDBID, c.Hidden)
			if c.Next != nil && c.Next.FirstAired != nil {
				s += " airs"
			}
			out = append(out, s)
		}
		slices.Sort(out)
		return out
	}
	if got, want := upNext(1), []string{"progress 8001 hidden=false", fmt.Sprintf("watchlist %d hidden=false", unstarted.TMDBID)}; !slices.Equal(got, want) {
		t.Errorf("A's Up next = %q, want %q", got, want)
	}
	if got, want := upNext(2), []string{"progress 8002 hidden=false"}; !slices.Equal(got, want) {
		t.Errorf("B's Up next = %q, want %q", got, want)
	}

	// New episodes: Kilo's E1 A watched, E2 B watched; Lima B hid; A's
	// calendar has 8101, which A started.
	from := time.Date(2026, 10, 7, 0, 0, 0, 0, time.UTC)
	now := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	kilo := seed.show("Kilo", true)
	seed.episodes(kilo, 1, "2026-10-08", "2026-10-09")
	a.watched(kilo.TMDBID, 1, 1, 0, 1, now)
	b.watched(kilo.TMDBID, 1, 2, 0, 1, now)
	lima := seed.show("Lima", true)
	seed.episodes(lima, 1, "2026-10-10")
	b.hidden("progress_watched", 31, lima.TMDBID)
	a.progress(progressRow{traktID: 41, tmdbID: 8101, title: "Calendar show", aired: 1, lastWatched: now})
	a.calendar(41, 8101, 1, 1, 0, "Aired", time.Date(2026, 10, 13, 2, 0, 0, 0, time.UTC))
	newEpisodes := func(member int64) []string {
		t.Helper()
		candidates, err := repo.NewEpisodeCandidates(ctx, member, from, now, "2026-10-07", "2026-10-14")
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, c := range candidates {
			out = append(out, fmt.Sprintf("%d S%dE%d", c.ShowTMDBID, c.Season, c.Episode))
		}
		slices.Sort(out)
		return out
	}
	wantA := []string{fmt.Sprintf("%d S1E2", kilo.TMDBID), fmt.Sprintf("%d S1E1", lima.TMDBID), "8101 S1E1"}
	slices.Sort(wantA)
	if got := newEpisodes(1); !slices.Equal(got, wantA) {
		t.Errorf("A's new episodes = %q, want %q", got, wantA)
	}
	if got, want := newEpisodes(2), []string{fmt.Sprintf("%d S1E1", kilo.TMDBID)}; !slices.Equal(got, want) {
		t.Errorf("B's new episodes = %q, want %q", got, want)
	}

	// The watchlist and the history, with the same Trakt ids for both.
	a.watchlist("movie", 51, 603, "The Matrix", day(1))
	b.watchlist("movie", 51, 604, "Reloaded", day(2))
	a.play(1, "movie", 603, "The Matrix", 0, 0, "", day(1))
	b.play(1, "movie", 604, "Reloaded", 0, 0, "", day(2))
	b.play(2, "episode", 8002, "B's show", 1, 1, "", day(3))
	if w, err := repo.TraktWatchlist(ctx, 1, "movie"); err != nil || len(w) != 1 || w[0].TMDBID != 603 {
		t.Errorf("A's watchlist = %+v, %v", w, err)
	}
	if w, err := repo.TraktWatchlist(ctx, 2, ""); err != nil || len(w) != 1 || w[0].TMDBID != 604 {
		t.Errorf("B's watchlist = %+v, %v", w, err)
	}
	if plays, total, err := repo.TraktHistory(ctx, 1, 50, 0); err != nil || total != 1 || len(plays) != 1 || plays[0].TMDBID != 603 {
		t.Errorf("A's history = %+v, %d, %v", plays, total, err)
	}
	if plays, total, err := repo.TraktHistory(ctx, 2, 50, 0); err != nil || total != 2 || len(plays) != 2 || plays[0].ID != 2 {
		t.Errorf("B's history = %+v, %d, %v", plays, total, err)
	}

	// The taste: each member's own.
	a.watchedMovie(603, 1, day(1))
	b.watchedMovie(604, 2, day(2))
	a.rating("movie", 51, 603, 9)
	b.rating("movie", 52, 604, 3)
	tasteA, tasteB := NewTasteRepository(db).ForMember(1), NewTasteRepository(db).ForMember(2)
	if movies, err := tasteA.WatchedMovies(ctx); err != nil || len(movies) != 1 || movies[0].TMDBID != 603 || movies[0].Title != "The Matrix" {
		t.Errorf("A's watched movies = %+v, %v", movies, err)
	}
	if movies, err := tasteB.WatchedMovies(ctx); err != nil || len(movies) != 1 || movies[0].TMDBID != 604 || movies[0].Plays != 2 {
		t.Errorf("B's watched movies = %+v, %v", movies, err)
	}
	shows := func(r *TasteRepository) []int {
		t.Helper()
		list, err := r.WatchedShows(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var ids []int
		for _, s := range list {
			ids = append(ids, s.TMDBID)
		}
		slices.Sort(ids)
		return ids
	}
	if got, want := shows(tasteA), []int{kilo.TMDBID, 8001, 8101}; !slices.Equal(got, sorted(want)) {
		t.Errorf("A's watched shows = %v, want %v", got, sorted(want))
	}
	if got, want := shows(tasteB), []int{unstarted.TMDBID, kilo.TMDBID, 8002}; !slices.Equal(got, sorted(want)) {
		t.Errorf("B's watched shows = %v, want %v", got, sorted(want))
	}
	if r, err := tasteA.TasteRatings(ctx); err != nil || len(r) != 1 || r[0].TMDBID != 603 || r[0].Rating != 9 {
		t.Errorf("A's ratings = %+v, %v", r, err)
	}
	if h, err := tasteA.HiddenShows(ctx); err != nil || len(h) != 0 {
		t.Errorf("A's hidden shows = %+v, %v; want none", h, err)
	}
	if h, err := tasteB.HiddenShows(ctx); err != nil || len(h) != 2 {
		t.Errorf("B's hidden shows = %+v, %v; want 8001 and Lima", h, err)
	}
	if w, err := tasteB.WatchlistRefs(ctx); err != nil || len(w) != 1 || w[0] != (TitleRef{TitleMovie, 604}) {
		t.Errorf("B's watchlist = %+v, %v", w, err)
	}
	refs := []TitleRef{{TitleMovie, 603}, {TitleTV, 8001}}
	if s, err := tasteB.TitleStates(ctx, refs); err != nil || len(s) != 2 || s[0].Plays != 0 || s[0].Rating != nil || s[0].OnWatchlist ||
		s[1].Started || !s[1].Dropped {
		t.Errorf("B's title states = %+v, %v; want A's movie unseen and A's show dropped", s, err)
	}
	if s, err := tasteA.TitleStates(ctx, refs); err != nil || len(s) != 2 || s[0].Plays != 1 || *s[0].Rating != 9 || !s[0].OnWatchlist ||
		!s[1].Started || s[1].Dropped || s[1].Completed != 2 {
		t.Errorf("A's title states = %+v, %v", s, err)
	}
	if _, err := NewTasteRepository(db).WatchedMovies(ctx); !errors.Is(err, errUnbound) {
		t.Errorf("an unbound taste read = %v, want errUnbound", err)
	}
}

// A member's connection, sync and purge touch their rows only, and an
// account connects to one member.
func TestHousehold_TraktConnections(t *testing.T) {
	db := openSchemaDB(t)
	ctx := context.Background()
	twoMembers(t, db)
	authA, authB := NewTraktAuthRepository(db).ForMember(1), NewTraktAuthRepository(db).ForMember(2)
	syncA, syncB := NewTraktSyncStore(db).ForMember(1), NewTraktSyncStore(db).ForMember(2)

	genA, err := authA.Connect(ctx, traktAuthRow("teymur"))
	if err != nil {
		t.Fatal(err)
	}
	genB, err := authB.Connect(ctx, traktAuthRow("sam"))
	if err != nil {
		t.Fatal(err)
	}
	seedTraktData(t, db, 1)
	seedTraktData(t, db, 2)
	if _, err := db.Exec(`INSERT INTO trakt_ratings (member_id, media_type, trakt_id, tmdb_id, rating, rated_at) VALUES (2, 'movie', 482, 604, 5, NOW())`); err != nil {
		t.Fatal(err)
	}
	seeded := func(member int64) map[string]int {
		want := map[string]int{"trakt_watchlist": 2, "trakt_ratings": 1}
		if member == 2 {
			want["trakt_ratings"] = 2
		}
		for _, table := range traktTables {
			if _, ok := want[table]; !ok {
				want[table] = 1
			}
		}
		return want
	}
	assertRows := func(what string, member int64, want map[string]int) {
		t.Helper()
		for table, n := range rowCounts(t, db, member) {
			if n != want[table] {
				t.Errorf("%s: member %d's %s has %d rows, want %d", what, member, table, n, want[table])
			}
		}
	}

	// Counts are the member's.
	if c, err := authB.Counts(ctx); err != nil || c != (TraktCounts{History: 1, Ratings: 2, Watchlist: 2}) {
		t.Errorf("B's counts = %+v, %v", c, err)
	}
	if c, err := authA.Counts(ctx); err != nil || c.Ratings != 1 {
		t.Errorf("A's counts = %+v, %v", c, err)
	}

	// A's account for B: refused, and B's connection and rows stay.
	if _, err := authB.Connect(ctx, traktAuthRow("teymur")); !errors.Is(err, ErrTraktAccountInUse) {
		t.Errorf("connecting A's account for B = %v, want ErrTraktAccountInUse", err)
	}
	if got, err := authB.Get(ctx); err != nil || got == nil || got.UserSlug != "sam" || !got.ConnectedAt.Equal(genB) {
		t.Errorf("B's connection after the refusal = %+v, %v", got, err)
	}
	assertRows("after the refusal", 2, seeded(2))

	// The generation is the member's: A's doesn't write B's rows.
	plays := []TraktPlayRow{moviePlay(1, 603, "2026-10-10T20:00:00Z"), moviePlay(2, 603, "2026-10-11T20:00:00Z")}
	if err := syncB.ReplaceHistory(ctx, genA, plays); !errors.Is(err, ErrTraktConnectionReplaced) {
		t.Errorf("B's write with A's generation = %v, want ErrTraktConnectionReplaced", err)
	}
	if err := authB.UpdateTokens(ctx, genA, TraktTokens{AccessTokenEnc: []byte{1}, RefreshTokenEnc: []byte{2}}); !errors.Is(err, ErrTraktConnectionReplaced) {
		t.Errorf("B's tokens with A's generation = %v, want ErrTraktConnectionReplaced", err)
	}

	// Each sync rebuilds its member's watched tables only.
	if err := syncA.ReplaceHistory(ctx, genA, plays); err != nil {
		t.Fatal(err)
	}
	if err := syncB.ReplaceHistory(ctx, genB, plays[:1]); err != nil {
		t.Fatal(err)
	}
	if got, want := queryStrings(t, db, `SELECT member_id, tmdb_id, plays FROM trakt_watched_movies ORDER BY member_id`),
		[]string{"1 603 2", "2 603 1"}; !slices.Equal(got, want) {
		t.Errorf("watched movies = %q, want %q", got, want)
	}
	if got := queryStrings(t, db, `SELECT member_id, count(*) FROM trakt_watched_episodes GROUP BY member_id ORDER BY 1`); len(got) != 0 {
		t.Errorf("watched episodes after both replaces = %q, want none", got)
	}
	if m, _, err := syncB.LatestPlays(ctx); err != nil || !m.Equal(at("2026-10-10T20:00:00Z")) {
		t.Errorf("B's latest movie play = %v, %v", m, err)
	}
	if err := syncA.FinishSync(ctx, genA, TraktSyncDone{At: at("2026-10-17T12:00:00Z"), Run: []byte(`{}`)}); err != nil {
		t.Fatal(err)
	}
	if st, err := syncB.State(ctx); err != nil || !st.LastSyncAt.Equal(at("2026-10-16T08:00:00Z")) || st.LastRun != nil {
		t.Errorf("B's sync state after A's sync = %+v, %v", st, err)
	}
	// Artwork: B's watchlist movie misses it, A's has it.
	if _, err := db.Exec(`UPDATE trakt_watchlist SET poster_path = '/p.jpg', backdrop_path = '/b.jpg' WHERE member_id = 1`); err != nil {
		t.Fatal(err)
	}
	if w, err := syncA.ArtworkWants(ctx); err != nil || len(w.Movies) != 0 {
		t.Errorf("A's artwork wants = %+v, %v; want no movies", w, err)
	}
	if w, err := syncB.ArtworkWants(ctx); err != nil || !slices.Equal(w.Movies, []int{603}) {
		t.Errorf("B's artwork wants = %+v, %v; want movie 603", w, err)
	}
	if err := syncB.SetArtwork(ctx, genB, TraktArtwork{Movies: map[int]TraktImages{603: {Poster: "/b-poster.jpg"}}}); err != nil {
		t.Fatal(err)
	}
	if got := queryStrings(t, db, `SELECT member_id, poster_path FROM trakt_watchlist WHERE tmdb_id = 603 ORDER BY 1`); !slices.Equal(got, []string{"1 /p.jpg", "2 /b-poster.jpg"}) {
		t.Errorf("posters = %q", got)
	}

	// A connecting another account purges A's rows, not B's; then A's old
	// account is free for B.
	if _, err := authA.Connect(ctx, traktAuthRow("teymur-2")); err != nil {
		t.Fatal(err)
	}
	empty := map[string]int{"trakt_auth": 1}
	assertRows("after A's new account", 1, empty)
	if n := rowCounts(t, db, 2)["trakt_history"]; n != 1 {
		t.Errorf("B's history after A's new account = %d rows, want 1", n)
	}
	if _, err := authB.Connect(ctx, traktAuthRow("teymur")); err != nil {
		t.Errorf("B connecting the freed account = %v", err)
	}

	// A purge is the member's.
	seedTraktData(t, db, 1)
	if err := authB.Purge(ctx); err != nil {
		t.Fatal(err)
	}
	assertRows("after B's purge", 2, map[string]int{})
	assertRows("after B's purge", 1, seeded(1))

	// Unbound, nothing reads or writes.
	if _, err := NewTraktAuthRepository(db).Get(ctx); !errors.Is(err, errUnbound) {
		t.Errorf("an unbound Get = %v, want errUnbound", err)
	}
	if err := NewTraktAuthRepository(db).Purge(ctx); !errors.Is(err, errUnbound) {
		t.Errorf("an unbound Purge = %v, want errUnbound", err)
	}
	if _, err := NewTraktSyncStore(db).State(ctx); !errors.Is(err, errUnbound) {
		t.Errorf("an unbound State = %v, want errUnbound", err)
	}
	if err := NewTraktSyncStore(db).SaveRun(ctx, genA, []byte(`{}`)); !errors.Is(err, errUnbound) {
		t.Errorf("an unbound write = %v, want errUnbound", err)
	}
}

func equalIntPtr(a, b *int) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func deref2(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}

func sorted(ids []int) []int {
	out := slices.Clone(ids)
	slices.Sort(out)
	return out
}
