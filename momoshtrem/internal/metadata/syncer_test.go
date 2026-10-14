package metadata

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/tmdb"
)

func TestSyncer_CreateShow_StoresSpecialsAndNumbers(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	id := uniqueTMDBID()
	h.cleanupShow(t, id)
	h.tmdb.addShow(tmdb.ShowDetails{
		ID: id, Name: "Doctor Who", FirstAirDate: "2005-03-26", Status: "Ended",
		OriginalLanguage: "en", OriginCountry: []string{"GB"}, Genres: []tmdb.Genre{{ID: 18, Name: "Drama"}},
		ExternalIDs: tmdb.ExternalIDs{IMDbID: "tt0436992", TVDBID: 78804}, PosterPath: "/dw.jpg",
	}, map[int][]string{
		0: {"The Christmas Invasion", "The Runaway Bride"},
		1: {"Rose", "The End of the World", "The Unquiet Dead"},
		2: {"New Earth", "Tooth and Claw"},
	})

	res, err := h.syncer.CreateShow(ctx, CreateShowInput{TMDBID: id})
	if err != nil {
		t.Fatalf("CreateShow: %v", err)
	}
	if res.Existing || res.SeasonsAdded != 3 || res.EpisodesAdded != 7 || !res.Complete() {
		t.Errorf("result = existing %v, %d seasons, %d episodes, errors %v", res.Existing, res.SeasonsAdded, res.EpisodesAdded, res.SeasonErrors)
	}

	show := res.Show
	if show.Title != "Doctor Who" || show.Year != 2005 || show.IsAnime || show.EpisodeOrdering != library.OrderingAired ||
		!show.IncludeSpecials || show.Status != "Ended" || show.TVDBID != 78804 || !reflect.DeepEqual(show.Genres, []string{"Drama"}) {
		t.Errorf("show = %+v", show)
	}
	if got := seasonNumbers(show); !reflect.DeepEqual(got, []int{0, 1, 2}) {
		t.Fatalf("seasons = %v, want specials stored as season 0", got)
	}
	if show.Seasons[0].Name != "Specials" || show.Seasons[0].PosterPath != "/s0.jpg" {
		t.Errorf("season 0 = %+v", show.Seasons[0])
	}

	special := episodeOf(t, show, 0, 1)
	rose := episodeOf(t, show, 1, 1)
	if special.Name != "The Christmas Invasion" || special.AirDate != "2020-01-01" || special.AbsoluteNumber != nil {
		t.Errorf("special = %+v (specials get no absolute number)", special)
	}
	if rose.TMDBEpisodeID != episodeTMDBID(id, 1, 1) || rose.Runtime != 24 || rose.StillPath == "" || rose.Overview == "" || rose.EpisodeType != "standard" {
		t.Errorf("rose = %+v", rose)
	}

	wantAbsolute := map[[2]int]int{{1, 1}: 1, {1, 2}: 2, {1, 3}: 3, {2, 1}: 4, {2, 2}: 5}
	stored, err := h.shows.GetWithSeasonsAndEpisodesContext(ctx, show.ID)
	if err != nil {
		t.Fatal(err)
	}
	for key, want := range wantAbsolute {
		if got := absoluteOf(episodeOf(t, show, key[0], key[1])); got != want {
			t.Errorf("returned S%dE%d absolute = %d, want %d", key[0], key[1], got, want)
		}
		if got := absoluteOf(episodeOf(t, stored, key[0], key[1])); got != want {
			t.Errorf("stored S%dE%d absolute = %d, want %d", key[0], key[1], got, want)
		}
	}
	if stored.MetadataUpdatedAt == nil {
		t.Error("show not stamped as synced")
	}

	again, err := h.syncer.CreateShow(ctx, CreateShowInput{TMDBID: id})
	if err != nil || !again.Existing || again.Show.ID != show.ID {
		t.Fatalf("second CreateShow = (%+v, %v), want the existing show", again, err)
	}
	if n := h.tmdb.count("show"); n != 1 {
		t.Errorf("show details fetched %d times, want 1", n)
	}
}

func TestSyncer_CreateShow_AnimeDefaultsToAbsolute(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	id := uniqueTMDBID()
	h.cleanupShow(t, id)
	h.tmdb.addShow(tmdb.ShowDetails{
		ID: id, Name: "Frieren", FirstAirDate: "2023-09-29", Status: "Returning Series",
		OriginalLanguage: "ja", OriginCountry: []string{"JP"}, Genres: []tmdb.Genre{{ID: 16, Name: "Animation"}},
	}, map[int][]string{1: {"E1", "E2", "E3"}, 2: {"E4", "E5"}})

	// The absolute group puts S2E1 before S1E3, so its numbers differ from
	// the cumulative ones.
	gid := fmt.Sprintf("abs-%d", id)
	h.tmdb.groupLists[id] = []tmdb.EpisodeGroupSummary{
		{ID: "arcs", Type: tmdb.GroupStoryArc, EpisodeCount: 5},
		{ID: gid, Type: tmdb.GroupAbsolute, EpisodeCount: 5},
	}
	h.tmdb.groups[gid] = &tmdb.EpisodeGroup{ID: gid, Type: tmdb.GroupAbsolute, Groups: []tmdb.EpisodeGroupGroup{{
		Order: 0,
		Episodes: []tmdb.GroupEpisode{
			{ID: episodeTMDBID(id, 1, 1), SeasonNumber: 1, EpisodeNumber: 1, Order: 0},
			{ID: episodeTMDBID(id, 1, 2), SeasonNumber: 1, EpisodeNumber: 2, Order: 1},
			{ID: episodeTMDBID(id, 2, 1), SeasonNumber: 2, EpisodeNumber: 1, Order: 2},
			{ID: episodeTMDBID(id, 1, 3), SeasonNumber: 1, EpisodeNumber: 3, Order: 3},
			{ID: episodeTMDBID(id, 2, 2), SeasonNumber: 2, EpisodeNumber: 2, Order: 4},
		},
	}}}

	res, err := h.syncer.CreateShow(ctx, CreateShowInput{TMDBID: id})
	if err != nil {
		t.Fatalf("CreateShow: %v", err)
	}
	show := res.Show
	if !show.IsAnime || show.EpisodeOrdering != library.OrderingAbsolute || show.EpisodeGroupID != "" {
		t.Errorf("anime show = anime %v, ordering %q, group %q", show.IsAnime, show.EpisodeOrdering, show.EpisodeGroupID)
	}
	want := map[[2]int]int{{1, 1}: 1, {1, 2}: 2, {2, 1}: 3, {1, 3}: 4, {2, 2}: 5}
	for key, n := range want {
		if got := absoluteOf(episodeOf(t, show, key[0], key[1])); got != n {
			t.Errorf("S%dE%d absolute = %d, want %d", key[0], key[1], got, n)
		}
	}
}

func TestSyncer_CreateShow_SeasonFailureIsNotStamped(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	id := uniqueTMDBID()
	h.cleanupShow(t, id)
	h.tmdb.addShow(tmdb.ShowDetails{ID: id, Name: "Flaky", FirstAirDate: "2020-01-01"},
		map[int][]string{1: {"A"}, 2: {"B"}})
	h.tmdb.seasonErrs[[2]int{id, 2}] = &tmdb.StatusError{StatusCode: 503}

	res, err := h.syncer.CreateShow(ctx, CreateShowInput{TMDBID: id})
	if err != nil {
		t.Fatalf("CreateShow: %v", err)
	}
	if len(res.SeasonErrors) != 1 || res.SeasonErrors[0].SeasonNumber != 2 || !errors.Is(res.SeasonErrors[0].Err, library.ErrTMDBFetch) {
		t.Errorf("season errors = %v", res.SeasonErrors)
	}
	if got := seasonNumbers(res.Show); !reflect.DeepEqual(got, []int{1}) {
		t.Errorf("seasons = %v, want the working one", got)
	}
	if stored, _ := h.shows.GetByID(res.Show.ID); stored.MetadataUpdatedAt != nil {
		t.Error("a partly synced show was stamped; the backfill would skip it")
	}
}

func TestSyncer_FetchesAtMostFourSeasonsAtOnce(t *testing.T) {
	h := newHarness(t)
	id := uniqueTMDBID()
	h.cleanupShow(t, id)
	seasons := map[int][]string{}
	for n := 1; n <= 10; n++ {
		seasons[n] = []string{"x"}
	}
	h.tmdb.addShow(tmdb.ShowDetails{ID: id, Name: "Long", FirstAirDate: "1999-01-01"}, seasons)
	h.tmdb.seasonDelay = 30 * time.Millisecond

	if _, err := h.syncer.CreateShow(context.Background(), CreateShowInput{TMDBID: id}); err != nil {
		t.Fatal(err)
	}
	if h.tmdb.maxInFlight > seasonParallelism || h.tmdb.maxInFlight < 2 {
		t.Errorf("max concurrent season fetches = %d, want 2..%d", h.tmdb.maxInFlight, seasonParallelism)
	}
}

func TestSyncer_CreateShow_Errors(t *testing.T) {
	h := newHarness(t)
	_, err := h.syncer.CreateShow(context.Background(), CreateShowInput{TMDBID: uniqueTMDBID()})
	if !errors.Is(err, tmdb.ErrNotFound) || !errors.Is(err, library.ErrTMDBFetch) {
		t.Errorf("unknown TMDB ID: error = %v, want ErrNotFound wrapped in ErrTMDBFetch", err)
	}
}

func TestSyncer_RefreshShow(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	id := uniqueTMDBID()
	h.cleanupShow(t, id)
	h.tmdb.addShow(tmdb.ShowDetails{ID: id, Name: "Severance", FirstAirDate: "2022-02-18", Status: "Returning Series"},
		map[int][]string{1: {"Good News About Hell", "Half Loop"}})

	created, err := h.syncer.CreateShow(ctx, CreateShowInput{TMDBID: id})
	if err != nil {
		t.Fatal(err)
	}
	h.assign(t, episodeOf(t, created.Show, 1, 1).ID)

	t.Run("renaming an assigned episode rebuilds the tree", func(t *testing.T) {
		h.tmdb.mu.Lock()
		h.tmdb.shows[id].Name = "Severance (renamed on TMDB)"
		h.tmdb.mu.Unlock()
		h.tmdb.renameEpisode(id, 1, 1, "Good News About Hell (Director's Cut)")
		h.tmdb.addShow(tmdb.ShowDetails{ID: id, Name: "Severance (renamed on TMDB)", FirstAirDate: "2022-02-18", Status: "Returning Series"},
			map[int][]string{1: {"Good News About Hell (Director's Cut)", "Half Loop", "In Perpetuity"}})

		res, err := h.syncer.RefreshShow(ctx, created.Show.ID)
		if err != nil {
			t.Fatalf("RefreshShow: %v", err)
		}
		if res.EpisodesAdded != 1 || res.SeasonsAdded != 0 {
			t.Errorf("added %d seasons, %d episodes; want 0, 1", res.SeasonsAdded, res.EpisodesAdded)
		}
		if got := episodeOf(t, res.Show, 1, 1); got.Name != "Good News About Hell (Director's Cut)" || got.Assignment == nil {
			t.Errorf("episode 1 = %+v", got)
		}
		if res.Show.Title != "Severance" || res.Show.Year != 2022 {
			t.Errorf("title/year changed to %q (%d)", res.Show.Title, res.Show.Year)
		}
		if n := h.tree.n.Load(); n != 1 {
			t.Errorf("tree rebuilds = %d, want 1", n)
		}
	})

	t.Run("no change, no rebuild", func(t *testing.T) {
		if _, err := h.syncer.RefreshShow(ctx, created.Show.ID); err != nil {
			t.Fatal(err)
		}
		if n := h.tree.n.Load(); n != 1 {
			t.Errorf("tree rebuilds = %d, want still 1", n)
		}
	})

	t.Run("renaming an unassigned episode doesn't rebuild", func(t *testing.T) {
		h.tmdb.renameEpisode(id, 1, 2, "Half Loop (renamed)")
		res, err := h.syncer.RefreshShow(ctx, created.Show.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got := episodeOf(t, res.Show, 1, 2); got.Name != "Half Loop (renamed)" {
			t.Errorf("episode 2 name = %q", got.Name)
		}
		if n := h.tree.n.Load(); n != 1 {
			t.Errorf("tree rebuilds = %d, want still 1", n)
		}
	})

	t.Run("missing show", func(t *testing.T) {
		if _, err := h.syncer.RefreshShow(ctx, -1); !errors.Is(err, library.ErrShowNotFound) {
			t.Errorf("error = %v, want ErrShowNotFound", err)
		}
	})
}

func TestSyncer_ApplySettings(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	id := uniqueTMDBID()
	h.cleanupShow(t, id)
	h.tmdb.addShow(tmdb.ShowDetails{ID: id, Name: "Settings", FirstAirDate: "2010-01-01", Status: "Ended"},
		map[int][]string{0: {"Special"}, 1: {"A", "B", "C"}})
	dvd := fmt.Sprintf("dvd-%d", id)
	h.tmdb.groupLists[id] = []tmdb.EpisodeGroupSummary{{ID: dvd, Type: tmdb.GroupDVD, EpisodeCount: 3}}
	h.tmdb.groups[dvd] = &tmdb.EpisodeGroup{ID: dvd, Type: tmdb.GroupDVD, Groups: []tmdb.EpisodeGroupGroup{{
		Order: 0,
		Episodes: []tmdb.GroupEpisode{
			{ID: episodeTMDBID(id, 1, 3), SeasonNumber: 1, EpisodeNumber: 3, Order: 0},
			{ID: episodeTMDBID(id, 1, 1), SeasonNumber: 1, EpisodeNumber: 1, Order: 1},
			{ID: episodeTMDBID(id, 1, 2), SeasonNumber: 1, EpisodeNumber: 2, Order: 2},
		},
	}}}

	created, err := h.syncer.CreateShow(ctx, CreateShowInput{TMDBID: id})
	if err != nil {
		t.Fatal(err)
	}
	showID := created.Show.ID

	ptr := func(s string) *string { return &s }
	ordering := func(o library.EpisodeOrdering) *library.EpisodeOrdering { return &o }
	mode := func(m library.ClassifierMode) *library.ClassifierMode { return &m }
	boolPtr := func(b bool) *bool { return &b }

	invalid := []struct {
		name  string
		patch SettingsPatch
	}{
		{"unknown ordering", SettingsPatch{EpisodeOrdering: ordering("dvd")}},
		{"unknown classifier mode", SettingsPatch{ClassifierMode: mode("sometimes")}},
		{"episode_group without a group", SettingsPatch{EpisodeOrdering: ordering(library.OrderingEpisodeGroup)}},
		{"group of another show", SettingsPatch{EpisodeOrdering: ordering(library.OrderingEpisodeGroup), EpisodeGroupID: ptr("someone-elses")}},
	}
	for _, tt := range invalid {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := h.syncer.ApplySettings(ctx, showID, tt.patch); !errors.Is(err, library.ErrInvalidShowSettings) {
				t.Errorf("error = %v, want ErrInvalidShowSettings", err)
			}
		})
	}
	if stored, _ := h.shows.GetByID(showID); stored.Settings() != created.Show.Settings() {
		t.Fatalf("invalid patches changed the settings: %+v", stored.Settings())
	}

	t.Run("episode group renumbers", func(t *testing.T) {
		show, err := h.syncer.ApplySettings(ctx, showID, SettingsPatch{
			EpisodeOrdering: ordering(library.OrderingEpisodeGroup), EpisodeGroupID: ptr(dvd), ClassifierMode: mode(library.ClassifierAIAlways),
		})
		if err != nil {
			t.Fatalf("ApplySettings: %v", err)
		}
		if show.EpisodeOrdering != library.OrderingEpisodeGroup || show.EpisodeGroupID != dvd || show.ClassifierMode != library.ClassifierAIAlways {
			t.Errorf("settings = %+v", show.Settings())
		}
		stored, _ := h.shows.GetWithSeasonsAndEpisodesContext(ctx, showID)
		for _, s := range []*library.Show{show, stored} {
			got := []int{absoluteOf(episodeOf(t, s, 1, 1)), absoluteOf(episodeOf(t, s, 1, 2)), absoluteOf(episodeOf(t, s, 1, 3))}
			if !reflect.DeepEqual(got, []int{2, 3, 1}) {
				t.Errorf("numbers = %v, want the DVD order [2 3 1]", got)
			}
		}
	})

	t.Run("specials off keeps the rows", func(t *testing.T) {
		show, err := h.syncer.ApplySettings(ctx, showID, SettingsPatch{IncludeSpecials: boolPtr(false)})
		if err != nil {
			t.Fatal(err)
		}
		if show.IncludeSpecials || !reflect.DeepEqual(seasonNumbers(show), []int{0, 1}) {
			t.Errorf("specials %v, seasons %v; want off with season 0 kept", show.IncludeSpecials, seasonNumbers(show))
		}
	})

	t.Run("specials on fetches season 0", func(t *testing.T) {
		if _, err := h.db.Exec(`DELETE FROM seasons WHERE show_id = $1 AND season_number = 0`, showID); err != nil {
			t.Fatal(err)
		}
		show, err := h.syncer.ApplySettings(ctx, showID, SettingsPatch{IncludeSpecials: boolPtr(true)})
		if err != nil {
			t.Fatal(err)
		}
		if !show.IncludeSpecials || !reflect.DeepEqual(seasonNumbers(show), []int{0, 1}) {
			t.Errorf("specials %v, seasons %v; want season 0 back", show.IncludeSpecials, seasonNumbers(show))
		}
		if episodeOf(t, show, 0, 1).AbsoluteNumber != nil {
			t.Error("a special got an absolute number")
		}
	})

	t.Run("back to aired numbers cumulatively", func(t *testing.T) {
		show, err := h.syncer.ApplySettings(ctx, showID, SettingsPatch{EpisodeOrdering: ordering(library.OrderingAired)})
		if err != nil {
			t.Fatal(err)
		}
		got := []int{absoluteOf(episodeOf(t, show, 1, 1)), absoluteOf(episodeOf(t, show, 1, 2)), absoluteOf(episodeOf(t, show, 1, 3))}
		if !reflect.DeepEqual(got, []int{1, 2, 3}) {
			t.Errorf("numbers = %v, want [1 2 3]", got)
		}
	})

	t.Run("missing show", func(t *testing.T) {
		if _, err := h.syncer.ApplySettings(ctx, -1, SettingsPatch{}); !errors.Is(err, library.ErrShowNotFound) {
			t.Errorf("error = %v, want ErrShowNotFound", err)
		}
	})
}

func TestSyncer_Movies(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	id := uniqueTMDBID()
	h.cleanupMovie(t, id)
	h.tmdb.movies[id] = &tmdb.MovieDetails{
		ID: id, Title: "Interstellar", ReleaseDate: "2014-11-05", Runtime: 169, PosterPath: "/p1.jpg",
		Genres: []tmdb.Genre{{ID: 878, Name: "Science Fiction"}}, ExternalIDs: tmdb.ExternalIDs{IMDbID: "tt0816692"},
	}

	res, err := h.syncer.CreateMovie(ctx, id)
	if err != nil {
		t.Fatalf("CreateMovie: %v", err)
	}
	m := res.Movie
	if res.Existing || m.Title != "Interstellar" || m.Year != 2014 || m.Runtime != 169 || m.IMDbID != "tt0816692" ||
		!reflect.DeepEqual(m.Genres, []string{"Science Fiction"}) || m.MetadataUpdatedAt == nil {
		t.Errorf("movie = %+v", m)
	}

	if again, err := h.syncer.CreateMovie(ctx, id); err != nil || !again.Existing || again.Movie.ID != m.ID {
		t.Errorf("second CreateMovie = (%+v, %v)", again, err)
	}

	h.tmdb.movies[id].Title = "Interstellar (Remastered)"
	h.tmdb.movies[id].PosterPath = "/p2.jpg"
	refreshed, err := h.syncer.RefreshMovie(ctx, m.ID)
	if err != nil {
		t.Fatalf("RefreshMovie: %v", err)
	}
	if refreshed.Title != "Interstellar" || refreshed.PosterPath != "/p2.jpg" {
		t.Errorf("refreshed = %q with poster %q; want the old title and the new poster", refreshed.Title, refreshed.PosterPath)
	}

	if _, err := h.syncer.RefreshMovie(ctx, -1); !errors.Is(err, library.ErrMovieNotFound) {
		t.Errorf("missing movie: error = %v", err)
	}
	if _, err := h.syncer.CreateMovie(ctx, uniqueTMDBID()); !errors.Is(err, tmdb.ErrNotFound) {
		t.Errorf("unknown TMDB ID: error = %v", err)
	}
}
