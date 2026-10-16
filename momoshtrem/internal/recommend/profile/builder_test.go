package profile

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/tmdb"
)

// fakeStore answers the history from a History.
type fakeStore struct{ h *History }

func (f fakeStore) WatchedMovies(context.Context) ([]library.WatchedMovie, error) {
	return f.h.Movies, nil
}
func (f fakeStore) WatchedShows(context.Context) ([]library.WatchedShow, error) {
	return f.h.Shows, nil
}
func (f fakeStore) TasteRatings(context.Context) ([]library.TasteRating, error) {
	return f.h.Ratings, nil
}
func (f fakeStore) HiddenShows(context.Context) ([]library.HiddenShow, error) { return f.h.Hidden, nil }
func (f fakeStore) TasteFeedback(context.Context) ([]library.TasteFeedback, error) {
	return f.h.Feedback, nil
}
func (f fakeStore) LibraryTitles(context.Context) ([]library.LibraryTitle, error) {
	return f.h.Library, nil
}

type fakeSettings struct{ useRatings bool }

func (f fakeSettings) TraktSettings(context.Context) (library.TraktSettings, error) {
	return library.TraktSettings{UseRatings: f.useRatings}, nil
}

type fakeConn bool

func (c fakeConn) Connected() bool { return bool(c) }

// fakeTMDB describes movies 1 and 2 and show 11; anything else is 404.
type fakeTMDB struct {
	mu    sync.Mutex
	calls int
	err   error
}

func (f *fakeTMDB) MovieDetails(_ context.Context, id int) (*tmdb.MovieDetails, error) {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	switch id {
	case 1:
		return &tmdb.MovieDetails{ID: 1, Title: "Dune: Part Two", ReleaseDate: "2024-02-27", Runtime: 166, OriginalLanguage: "en",
			Genres: []tmdb.Genre{{ID: 878, Name: "Science Fiction"}}, Keywords: tmdb.Keywords{Keywords: []tmdb.Keyword{{ID: 4, Name: "desert"}}},
			Credits: tmdb.Credits{Crew: []tmdb.CrewMember{{ID: 137427, Name: "Denis Villeneuve", Job: "Director"}, {ID: 5, Name: "Writer", Job: "Screenplay"}},
				Cast: []tmdb.CastMember{{ID: 1190668, Name: "Timothée Chalamet"}}}}, nil
	case 24:
		return &tmdb.MovieDetails{ID: 24, Title: "Morbius", ReleaseDate: "2022-03-30"}, nil
	}
	return nil, tmdb.ErrNotFound
}

func (f *fakeTMDB) ShowDetails(_ context.Context, id int) (*tmdb.ShowDetails, error) {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	if id == 11 {
		return &tmdb.ShowDetails{ID: 11, Name: "Frieren", FirstAirDate: "2023-09-29", OriginalLanguage: "ja", OriginCountry: []string{"JP"},
			Genres: []tmdb.Genre{{ID: 16, Name: "Animation"}}, EpisodeRunTime: []int{24},
			CreatedBy: []tmdb.Creator{{ID: 7, Name: "Kanehito Yamada"}}}, nil
	}
	return nil, tmdb.ErrNotFound
}

// fakeProfiles keeps stored profiles in memory.
type fakeProfiles struct {
	rows        []library.StoredTasteProfile
	pruneBefore time.Time
}

func (f *fakeProfiles) Insert(_ context.Context, counts, profile json.RawMessage, pruneBefore time.Time) (*library.StoredTasteProfile, error) {
	row := library.StoredTasteProfile{ID: int64(len(f.rows) + 1), CreatedAt: now, Counts: counts, Profile: profile}
	f.rows = append(f.rows, row)
	f.pruneBefore = pruneBefore
	return &row, nil
}

func (f *fakeProfiles) Latest(context.Context) (*library.StoredTasteProfile, error) {
	if len(f.rows) == 0 {
		return nil, nil
	}
	row := f.rows[len(f.rows)-1]
	return &row, nil
}

func newBuilder(h *History, details Details, profiles *fakeProfiles, connected bool) *Builder {
	return New(Deps{Store: fakeStore{h}, Profiles: profiles, Settings: fakeSettings{true}, TMDB: details, Trakt: fakeConn(connected),
		Now: func() time.Time { return now }})
}

func TestBuilder_Build(t *testing.T) {
	h := &History{
		Movies:   []library.WatchedMovie{{TMDBID: 1, Title: "Dune: Part Two", Year: 2024, Plays: 1, LastWatchedAt: daysAgo(1)}},
		Shows:    []library.WatchedShow{{TMDBID: 11, Title: "Frieren", Year: 2023, Aired: 28, Completed: 28, HasProgress: true, LastWatchedAt: daysAgoPtr(1)}},
		Feedback: []library.TasteFeedback{{TitleRef: movie(24), Signal: "dislike", UpdatedAt: daysAgo(2)}, {TitleRef: movie(25), Signal: "dislike", UpdatedAt: daysAgo(3)}},
	}
	details := &fakeTMDB{}
	b := newBuilder(h, details, &fakeProfiles{}, true)
	p, err := b.Build(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// Dune, Frieren and the two nameless aversions; 25 is a 404
	if details.calls != 4 {
		t.Errorf("TMDB calls = %d, want 4", details.calls)
	}
	if len(p.Creators) != 2 || p.Mix.AnimeShare != 0.5 || p.Runtime.EpisodeMedian != 24 || p.Runtime.MovieMedian != 166 {
		t.Errorf("profile = %+v", p)
	}
	if len(p.Aversions) != 1 || p.Aversions[0].Title != "Morbius" {
		t.Errorf("aversions = %+v", p.Aversions)
	}
	if p.Counts.Loved != 2 || !p.BuiltAt.Equal(now) {
		t.Errorf("counts = %+v, built at %v", p.Counts, p.BuiltAt)
	}
}

// Without a TMDB key the profile is the library's: no request after the
// first refusal, and no error.
func TestBuilder_NoTMDBKey(t *testing.T) {
	h := fixture()
	details := &fakeTMDB{err: tmdb.ErrNoAPIKey}
	p, err := newBuilder(h, details, &fakeProfiles{}, true).Build(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if details.calls > detailWorkers {
		t.Errorf("TMDB calls = %d after it said there's no key", details.calls)
	}
	// Frieren's genres come from the library
	if len(p.Genres) != 2 || p.Mix.AnimeShare != 1 {
		t.Errorf("genres = %+v, mix %+v", p.Genres, p.Mix)
	}
}

func TestBuilder_Cancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := newBuilder(fixture(), &fakeTMDB{}, &fakeProfiles{}, true).Build(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want canceled", err)
	}
}

// CheckHistory never asks TMDB.
func TestBuilder_CheckHistory(t *testing.T) {
	details := &fakeTMDB{}
	for _, connected := range []bool{true, false} {
		r, err := newBuilder(fixture(), details, &fakeProfiles{}, connected).CheckHistory(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if r.Connected != connected || r.Counts.Loved != 9 || r.Enough() != connected {
			t.Errorf("connected %v: %+v, enough %v", connected, r, r.Enough())
		}
	}
	if details.calls != 0 {
		t.Errorf("CheckHistory called TMDB %d times", details.calls)
	}
	// Never connected: not enough
	b := New(Deps{Store: fakeStore{fixture()}, Settings: fakeSettings{true}})
	if r, err := b.CheckHistory(context.Background()); err != nil || r.Enough() {
		t.Errorf("without Trakt: %+v, %v", r, err)
	}
}

func TestBuilder_SaveAndCurrent(t *testing.T) {
	profiles := &fakeProfiles{}
	details := &fakeTMDB{}
	b := newBuilder(fixture(), details, profiles, true)
	ctx := context.Background()

	stored, err := b.Current(ctx, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if stored.ID != 1 || len(profiles.rows) != 1 || !profiles.pruneBefore.Equal(now.Add(-24*time.Hour)) {
		t.Fatalf("first current = %+v, rows %d, prune before %v", stored, len(profiles.rows), profiles.pruneBefore)
	}
	var counts Counts
	if err := json.Unmarshal(profiles.rows[0].Counts, &counts); err != nil || counts != stored.Profile.Counts {
		t.Errorf("stored counts = %s (%v)", profiles.rows[0].Counts, err)
	}
	calls := details.calls

	// Fresh: the stored one, decoded, no rebuild
	again, err := b.Current(ctx, time.Hour)
	if err != nil || again.ID != 1 || details.calls != calls || again.Profile.Counts != stored.Profile.Counts {
		t.Errorf("second current = %+v, %v (TMDB calls %d → %d)", again, err, calls, details.calls)
	}
	// Stale: rebuilt and saved
	profiles.rows[0].CreatedAt = now.Add(-2 * time.Hour)
	if third, err := b.Current(ctx, time.Hour); err != nil || third.ID != 2 {
		t.Errorf("stale current = %+v, %v", third, err)
	}
}
