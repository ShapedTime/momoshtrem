package catalog

import (
	"context"
	"encoding/json"
	"slices"
	"testing"
	"time"

	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/member"
	"github.com/shapedtime/momoshtrem/internal/tmdb"
)

// watchFrieren marks Frieren's E28 (a file being fetched) and E29–E30 (no
// files) watched, and gives it Trakt's watch state with next.
func watchFrieren(f *fixture, next *library.TraktNext) {
	for i := range f.store.seasons[3][1].Episodes {
		ep := &f.store.seasons[3][1].Episodes[i]
		ep.Watched = ep.EpisodeNumber >= 28 && ep.EpisodeNumber <= 30
	}
	at := time.Date(2026, 10, 12, 21, 30, 0, 0, time.UTC)
	rating := 9
	f.store.watch["tv:209867"] = &library.TraktWatchState{Plays: 31, LastWatchedAt: &at, Rating: &rating,
		Progress: &library.TraktProgress{Completed: 30, Aired: 38}, Next: next}
}

func TestShowTitle_WatchState(t *testing.T) {
	f := newFixture()
	abs := 31
	watchFrieren(f, &library.TraktNext{Season: 1, Episode: 31, AbsoluteNumber: &abs, Name: "Episode 31", AirDate: "2024-04-26"})

	title, err := f.svc.ShowTitle(context.Background(), frierenTMDB)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(title.WatchState)
	want := `{"watched_at":"2026-10-12T21:30:00Z","plays":31,"rating":9,"progress":{"completed":30,"aired":38},` +
		`"next_episode":{"season":1,"episode":31,"absolute_number":31,"name":"Episode 31","air_date":"2024-04-26"}}`
	if string(raw) != want {
		t.Errorf("watch_state = %s\nwant          %s", raw, want)
	}
	// E29–E30 are watched: the frontier starts after them; E28's file
	// still counts as ready.
	lib := title.Library
	if m := lib.Missing; m == nil || m.From != 31 || m.To != 38 || m.Count != 8 || *lib.EpisodesReady != 28 {
		t.Errorf("missing = %+v, ready %d", lib.Missing, *lib.EpisodesReady)
	}
	if s := title.Seasons[0]; s.Ready != 28 || s.Missing != 8 || s.Upcoming != 0 {
		t.Errorf("season 1 = %+v", s)
	}
	if f.store.calls["watch"] != 1 {
		t.Errorf("watch state reads = %d, want 1", f.store.calls["watch"])
	}

	// Without a next episode the key stays, null.
	f = newFixture()
	watchFrieren(f, nil)
	title, _ = f.svc.ShowTitle(context.Background(), frierenTMDB)
	if raw, _ := json.Marshal(title.WatchState); string(raw) !=
		`{"watched_at":"2026-10-12T21:30:00Z","plays":31,"rating":9,"progress":{"completed":30,"aired":38},"next_episode":null}` {
		t.Errorf("watch_state without next = %s", raw)
	}
}

// default_season: the next episode's season when the page lists it, else
// the frontier's.
func TestShowTitle_DefaultSeasonPrefersNext(t *testing.T) {
	f := newFixture()
	s2 := library.Season{ID: 32, ShowID: 3, SeasonNumber: 2, Episodes: []library.Episode{{ID: 32001, EpisodeNumber: 1, AirDate: "2026-01-16"}}}
	f.store.seasons[3] = append(f.store.seasons[3], s2)

	for _, tt := range []struct {
		name string
		next *library.TraktNext
		want int
	}{
		{"the next episode's season", &library.TraktNext{Season: 2, Episode: 1}, 2},
		{"a season the page doesn't list", &library.TraktNext{Season: 5, Episode: 1}, 1},
		{"no next episode: the frontier's", nil, 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f.store.watch["tv:209867"] = &library.TraktWatchState{Plays: 1, Next: tt.next}
			title, err := f.svc.ShowTitle(context.Background(), frierenTMDB)
			if err != nil || title.DefaultSeason == nil || *title.DefaultSeason != tt.want {
				t.Errorf("default season = %v, %v; want %d", deref2(title.DefaultSeason), err, tt.want)
			}
		})
	}

	// A show that isn't in the library opens on it too.
	f.tmdb.shows[66573] = &tmdb.ShowDetails{ID: 66573, Name: "The Good Place", Seasons: []tmdb.SeasonSummary{
		{SeasonNumber: 1, EpisodeCount: 13}, {SeasonNumber: 2, EpisodeCount: 13}, {SeasonNumber: 3, EpisodeCount: 12},
	}}
	f.store.watch["tv:66573"] = &library.TraktWatchState{Plays: 14, Next: &library.TraktNext{Season: 2, Episode: 2, Name: "Dance Dance Resolution"}}
	title, err := f.svc.ShowTitle(context.Background(), 66573)
	if err != nil || *title.DefaultSeason != 2 || title.WatchState.NextEpisode.Name == nil || title.Library.InLibrary {
		t.Errorf("not in the library: default %v, watch %+v, %v", deref2(title.DefaultSeason), title.WatchState, err)
	}
}

func TestMovieTitle_WatchState(t *testing.T) {
	f := newFixture()
	at := time.Date(2026, 3, 1, 20, 0, 0, 0, time.UTC)
	rating := 8
	f.store.watch["movie:693134"] = &library.TraktWatchState{Plays: 2, LastWatchedAt: &at, Rating: &rating}
	title, err := f.svc.MovieTitle(context.Background(), duneTMDB)
	if err != nil {
		t.Fatal(err)
	}
	// A movie's has no progress or next episode.
	if raw, _ := json.Marshal(title.WatchState); string(raw) != `{"watched_at":"2026-03-01T20:00:00Z","plays":2,"rating":8}` {
		t.Errorf("watch_state = %s", raw)
	}
}

// The watch state's viewer (its rating's) is the context's member; none
// without one.
func TestWatchState_Viewer(t *testing.T) {
	f := newFixture()
	if _, err := f.svc.MovieTitle(member.WithID(context.Background(), 2), duneTMDB); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.ShowTitle(member.WithID(context.Background(), 3), frierenTMDB); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.MovieTitle(context.Background(), duneTMDB); err != nil {
		t.Fatal(err)
	}
	if want := []int64{2, 3, 0}; !slices.Equal(f.store.viewers, want) {
		t.Errorf("viewers = %v, want %v", f.store.viewers, want)
	}
}

// A watched episode is watched whatever else is true; its file still
// shows. Ranges count it out of missing.
func TestSeason_Watched(t *testing.T) {
	f := newFixture()
	watchFrieren(f, nil)
	season, err := f.svc.Season(context.Background(), frierenTMDB, 1)
	if err != nil {
		t.Fatal(err)
	}
	e28, e29, e31 := season.Episodes[27], season.Episodes[28], season.Episodes[30]
	if e28.State != EpisodeWatched || e28.Pct == nil || *e28.Pct != 42 || e28.WebDAVPath == nil || e28.Source == nil {
		t.Errorf("E28 = %+v, want watched with its file being fetched", e28)
	}
	if e29.State != EpisodeWatched || e29.WebDAVPath != nil || e31.State != EpisodeMissing {
		t.Errorf("E29 = %s, E31 = %s", e29.State, e31.State)
	}
	missing := map[string]int{}
	for _, r := range season.Ranges {
		missing[r.Label] = r.Missing
	}
	if missing["21–30"] != 0 || missing["31–38"] != 8 {
		t.Errorf("ranges' missing = %v", missing)
	}
	if f.store.calls["watch"] != 0 {
		t.Errorf("the season read the watch state: %d", f.store.calls["watch"])
	}
}
