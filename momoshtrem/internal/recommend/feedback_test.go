package recommend

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/recommend/candidates"
	"github.com/shapedtime/momoshtrem/internal/recommend/profile"
	"github.com/shapedtime/momoshtrem/internal/tmdb"
)

type fakeFeedbackStore struct {
	calls []string
	err   error
}

func (f *fakeFeedbackStore) Set(_ context.Context, ref library.TitleRef, signal, source string) error {
	f.calls = append(f.calls, fmt.Sprintf("set %s %d %s %s", ref.MediaType, ref.TMDBID, signal, source))
	return f.err
}

func (f *fakeFeedbackStore) Clear(_ context.Context, ref library.TitleRef) error {
	f.calls = append(f.calls, fmt.Sprintf("clear %s %d", ref.MediaType, ref.TMDBID))
	return f.err
}

func TestFeedbackService(t *testing.T) {
	store := &fakeFeedbackStore{}
	s := NewFeedbackService(store)
	ctx := context.Background()
	if err := s.SetFeedback(ctx, Feedback{MediaType: "tv", TMDBID: 95396, Signal: SignalNotInterested, Source: SourceHero}); err != nil {
		t.Fatal(err)
	}
	if err := s.ClearFeedback(ctx, "tv", 95396); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(store.calls, "; "); got != "set tv 95396 not_interested hero; clear tv 95396" {
		t.Errorf("calls = %s", got)
	}
	store.err = errors.New("db down")
	if err := s.SetFeedback(ctx, Feedback{MediaType: "movie", TMDBID: 1, Signal: SignalLike, Source: SourceTitle}); err == nil {
		t.Error("a store error was swallowed")
	}
}

// emptyStore is a database with nothing in it, for both builders.
type emptyStore struct{}

func (emptyStore) WatchedMovies(context.Context) ([]library.WatchedMovie, error)  { return nil, nil }
func (emptyStore) WatchedShows(context.Context) ([]library.WatchedShow, error)    { return nil, nil }
func (emptyStore) TasteRatings(context.Context) ([]library.TasteRating, error)    { return nil, nil }
func (emptyStore) HiddenShows(context.Context) ([]library.HiddenShow, error)      { return nil, nil }
func (emptyStore) TasteFeedback(context.Context) ([]library.TasteFeedback, error) { return nil, nil }
func (emptyStore) LibraryTitles(context.Context) ([]library.LibraryTitle, error)  { return nil, nil }
func (emptyStore) WatchlistRefs(context.Context) ([]library.TitleRef, error)      { return nil, nil }
func (emptyStore) TraktSettings(context.Context) (library.TraktSettings, error) {
	return library.DefaultTraktSettings(), nil
}

// noTMDB is TMDB without a key.
type noTMDB struct{}

func (noTMDB) MovieDetails(context.Context, int) (*tmdb.MovieDetails, error) {
	return nil, tmdb.ErrNoAPIKey
}
func (noTMDB) ShowDetails(context.Context, int) (*tmdb.ShowDetails, error) {
	return nil, tmdb.ErrNoAPIKey
}
func (noTMDB) Discover(context.Context, string, tmdb.DiscoverQuery) (*tmdb.ResultsPage, error) {
	return nil, tmdb.ErrNoAPIKey
}
func (noTMDB) Trending(context.Context, string, string) (*tmdb.ResultsPage, error) {
	return nil, tmdb.ErrNoAPIKey
}
func (noTMDB) UpcomingMovies(context.Context, int) (*tmdb.ResultsPage, error) {
	return nil, tmdb.ErrNoAPIKey
}
func (noTMDB) OnTheAir(context.Context, int) (*tmdb.ResultsPage, error) { return nil, tmdb.ErrNoAPIKey }
func (noTMDB) GenreList(context.Context, string) ([]tmdb.Genre, error)  { return nil, tmdb.ErrNoAPIKey }

// Without history or TMDB the report still answers: an empty profile,
// not enough history, and why there's no pool.
func TestDebug_NoHistory(t *testing.T) {
	now := func() time.Time { return time.Date(2026, 10, 16, 3, 0, 0, 0, time.UTC) }
	d := &Debug{
		Profiles: profile.New(profile.Deps{Store: emptyStore{}, Settings: emptyStore{}, TMDB: noTMDB{}, Now: now}),
		Pools:    candidates.New(candidates.Deps{TMDB: noTMDB{}, Store: emptyStore{}, Now: now}),
	}
	report, err := d.DebugProfile(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if report.TraktConnected || report.EnoughHistory || report.Profile == nil || report.Pool != nil || report.PoolError == nil ||
		report.ProfileBytes == 0 {
		t.Errorf("report = %+v", report)
	}
}
