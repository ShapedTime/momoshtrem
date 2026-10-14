package metadata

import (
	"context"
	"reflect"
	"testing"

	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/tmdb"
)

// seedLegacyShow inserts a show the way the pre-B1 code did: title and year
// only, regular seasons only, episodes with a name and nothing else.
func seedLegacyShow(t *testing.T, h *harness, tmdbID int, title string, year int, seasons map[int][]string) *library.Show {
	t.Helper()
	show := &library.Show{TMDBID: tmdbID, Title: title, Year: year}
	if err := h.shows.Create(show); err != nil {
		t.Fatal(err)
	}
	h.cleanupShow(t, tmdbID)
	for n, names := range seasons {
		var seasonID int64
		if err := h.db.QueryRow(`INSERT INTO seasons (show_id, season_number) VALUES ($1, $2) RETURNING id`, show.ID, n).Scan(&seasonID); err != nil {
			t.Fatal(err)
		}
		for i, name := range names {
			if _, err := h.db.Exec(`INSERT INTO episodes (season_id, episode_number, name) VALUES ($1, $2, $3)`, seasonID, i+1, name); err != nil {
				t.Fatal(err)
			}
		}
	}
	return show
}

func seedLegacyMovie(t *testing.T, h *harness, tmdbID int, title string, year int) *library.Movie {
	t.Helper()
	movie := &library.Movie{TMDBID: tmdbID, Title: title, Year: year}
	if err := h.movies.Create(movie); err != nil {
		t.Fatal(err)
	}
	h.cleanupMovie(t, tmdbID)
	return movie
}

// eventsFor keeps the events about the given items, keyed by TMDB ID: the
// shared test database may hold other packages' rows, which the fake TMDB
// doesn't know. Not by library ID: a movie and a show can share one (both
// are 1 on a new database).
func eventsFor(events []BackfillEvent, tmdbIDs ...int) map[int]BackfillEvent {
	want := map[int]bool{}
	for _, id := range tmdbIDs {
		want[id] = true
	}
	out := map[int]BackfillEvent{}
	for _, ev := range events {
		if want[ev.Item.TMDBID] {
			out[ev.Item.TMDBID] = ev
		}
	}
	return out
}

func TestBackfill(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	showTMDB := uniqueTMDBID()
	movieTMDB := uniqueTMDBID()
	legacyShow := seedLegacyShow(t, h, showTMDB, "Sousou no Frieren", 2023, map[int][]string{
		1: {"The Journey's End", "It Didn't Have to Be Magic..."},
	})
	legacyMovie := seedLegacyMovie(t, h, movieTMDB, "Spirited Away", 2001)

	// An assigned episode: its name is part of a WebDAV path.
	before, err := h.shows.GetWithSeasonsAndEpisodesContext(ctx, legacyShow.ID)
	if err != nil {
		t.Fatal(err)
	}
	h.assign(t, episodeOf(t, before, 1, 1).ID)

	// TMDB has since changed the title, year and episode names, and lists
	// specials and a newer episode.
	h.tmdb.addShow(tmdb.ShowDetails{
		ID: showTMDB, Name: "Frieren: Beyond Journey's End", FirstAirDate: "2024-01-01", Status: "Ended",
		OriginalName: "葬送のフリーレン", OriginalLanguage: "ja", OriginCountry: []string{"JP"},
		Genres: []tmdb.Genre{{ID: 16, Name: "Animation"}}, ExternalIDs: tmdb.ExternalIDs{TVDBID: 424536},
		PosterPath: "/frieren.jpg",
	}, map[int][]string{
		0: {"Mini Anime 1"},
		1: {"The Journey's End (TMDB)", "It Didn't Have to Be Magic (TMDB)", "Killing Magic"},
	})
	h.tmdb.movies[movieTMDB] = &tmdb.MovieDetails{
		ID: movieTMDB, Title: "Spirited Away (2001)", ReleaseDate: "2002-07-20", Runtime: 125,
		PosterPath: "/sa.jpg", Genres: []tmdb.Genre{{ID: 16, Name: "Animation"}}, IMDbID: "tt0245429",
	}

	t.Run("dry run lists without fetching or writing", func(t *testing.T) {
		var events []BackfillEvent
		if _, err := h.syncer.Backfill(ctx, BackfillOptions{DryRun: true}, func(ev BackfillEvent) { events = append(events, ev) }); err != nil {
			t.Fatal(err)
		}
		mine := eventsFor(events, showTMDB, movieTMDB)
		if len(mine) != 2 || !mine[showTMDB].DryRun {
			t.Errorf("dry-run events = %+v", mine)
		}
		if h.tmdb.count("show")+h.tmdb.count("movie")+h.tmdb.count("season") != 0 {
			t.Error("dry run called TMDB")
		}
		if m, _ := h.movies.GetByID(legacyMovie.ID); m.MetadataUpdatedAt != nil {
			t.Error("dry run wrote metadata")
		}
	})

	t.Run("only shows", func(t *testing.T) {
		var events []BackfillEvent
		if _, err := h.syncer.Backfill(ctx, BackfillOptions{DryRun: true, Only: library.MediaShow}, func(ev BackfillEvent) { events = append(events, ev) }); err != nil {
			t.Fatal(err)
		}
		for _, ev := range events {
			if ev.Kind != library.MediaShow {
				t.Fatalf("--only shows listed a %s", ev.Kind)
			}
		}
	})

	var events []BackfillEvent
	summary, err := h.syncer.Backfill(ctx, BackfillOptions{}, func(ev BackfillEvent) { events = append(events, ev) })
	if err != nil {
		t.Fatalf("Backfill: %v", err)
	}
	mine := eventsFor(events, showTMDB, movieTMDB)
	if len(mine) != 2 {
		t.Fatalf("events for seeded items = %+v", mine)
	}
	for id, ev := range mine {
		if ev.Err != nil {
			t.Errorf("item %d failed: %v", id, ev.Err)
		}
	}
	if ev := mine[showTMDB]; ev.EpisodesAdded != 2 {
		t.Errorf("episodes added = %d, want 2 (a special and a new episode)", ev.EpisodesAdded)
	}
	if summary.Movies < 1 || summary.Shows < 1 {
		t.Errorf("summary = %+v", summary)
	}

	t.Run("movie keeps its title and year", func(t *testing.T) {
		m, _ := h.movies.GetByID(legacyMovie.ID)
		if m.Title != "Spirited Away" || m.Year != 2001 {
			t.Errorf("movie renamed to %q (%d)", m.Title, m.Year)
		}
		if m.Runtime != 125 || m.PosterPath != "/sa.jpg" || m.IMDbID != "tt0245429" || m.MetadataUpdatedAt == nil {
			t.Errorf("movie metadata = %+v", m)
		}
	})

	t.Run("show keeps its title, year and episode names", func(t *testing.T) {
		show, err := h.shows.GetWithSeasonsAndEpisodesContext(ctx, legacyShow.ID)
		if err != nil {
			t.Fatal(err)
		}
		if show.Title != "Sousou no Frieren" || show.Year != 2023 {
			t.Errorf("show renamed to %q (%d)", show.Title, show.Year)
		}
		if got := episodeOf(t, show, 1, 1); got.Name != "The Journey's End" || got.Assignment == nil {
			t.Errorf("assigned episode = %+v, want its old name", got)
		}
		if got := episodeOf(t, show, 1, 2).Name; got != "It Didn't Have to Be Magic..." {
			t.Errorf("episode 2 renamed to %q", got)
		}
		if got := episodeOf(t, show, 1, 3).Name; got != "Killing Magic" {
			t.Errorf("new episode name = %q, want TMDB's", got)
		}
		if got := episodeOf(t, show, 1, 1); got.AirDate == "" || got.TMDBEpisodeID == 0 || got.StillPath == "" {
			t.Errorf("episode metadata not filled in: %+v", got)
		}

		if !reflect.DeepEqual(seasonNumbers(show), []int{0, 1}) || show.Seasons[0].Name != "Specials" {
			t.Errorf("seasons = %v, want specials added", seasonNumbers(show))
		}
		if !show.IsAnime || show.EpisodeOrdering != library.OrderingAbsolute || show.TVDBID != 424536 ||
			show.OriginalName != "葬送のフリーレン" || show.Status != "Ended" || show.MetadataUpdatedAt == nil {
			t.Errorf("show metadata = %+v", show)
		}
		got := []int{absoluteOf(episodeOf(t, show, 1, 1)), absoluteOf(episodeOf(t, show, 1, 2)), absoluteOf(episodeOf(t, show, 1, 3))}
		if !reflect.DeepEqual(got, []int{1, 2, 3}) || episodeOf(t, show, 0, 1).AbsoluteNumber != nil {
			t.Errorf("absolute numbers = %v", got)
		}
		if n := h.tree.n.Load(); n != 0 {
			t.Errorf("backfill rebuilt the tree %d times; nothing it writes is in a path", n)
		}
	})

	t.Run("a rerun resumes: done items are skipped", func(t *testing.T) {
		var rerun []BackfillEvent
		if _, err := h.syncer.Backfill(ctx, BackfillOptions{}, func(ev BackfillEvent) { rerun = append(rerun, ev) }); err != nil {
			t.Fatal(err)
		}
		if mine := eventsFor(rerun, showTMDB, movieTMDB); len(mine) != 0 {
			t.Errorf("rerun reprocessed %v", mine)
		}
	})

	t.Run("force reprocesses but still renames nothing", func(t *testing.T) {
		h.tmdb.renameEpisode(showTMDB, 1, 1, "Yet another name")
		var forced []BackfillEvent
		if _, err := h.syncer.Backfill(ctx, BackfillOptions{Force: true}, func(ev BackfillEvent) { forced = append(forced, ev) }); err != nil {
			t.Fatal(err)
		}
		if mine := eventsFor(forced, showTMDB, movieTMDB); len(mine) != 2 {
			t.Errorf("forced run processed %d seeded items, want 2", len(mine))
		}
		show, _ := h.shows.GetWithSeasonsAndEpisodesContext(ctx, legacyShow.ID)
		if got := episodeOf(t, show, 1, 1).Name; got != "The Journey's End" {
			t.Errorf("forced backfill renamed the episode to %q", got)
		}
	})

	t.Run("a failing item is counted and the run goes on", func(t *testing.T) {
		broken := seedLegacyMovie(t, h, uniqueTMDBID(), "Unknown to TMDB", 1990)
		var failed []BackfillEvent
		summary, err := h.syncer.Backfill(ctx, BackfillOptions{Only: library.MediaMovie}, func(ev BackfillEvent) { failed = append(failed, ev) })
		if err != nil {
			t.Fatal(err)
		}
		if ev, ok := eventsFor(failed, broken.TMDBID)[broken.TMDBID]; !ok || ev.Err == nil {
			t.Errorf("broken movie event = %+v", ev)
		}
		if summary.Failed < 1 {
			t.Errorf("summary = %+v, want a failure", summary)
		}
		if m, _ := h.movies.GetByID(broken.ID); m.MetadataUpdatedAt != nil || m.Title != "Unknown to TMDB" {
			t.Errorf("failed movie was written: %+v", m)
		}
	})
}
