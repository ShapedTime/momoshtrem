package profile

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/tmdb"
)

// Store reads the history (library.TasteRepository).
type Store interface {
	WatchedMovies(ctx context.Context) ([]library.WatchedMovie, error)
	WatchedShows(ctx context.Context) ([]library.WatchedShow, error)
	TasteRatings(ctx context.Context) ([]library.TasteRating, error)
	HiddenShows(ctx context.Context) ([]library.HiddenShow, error)
	TasteFeedback(ctx context.Context) ([]library.TasteFeedback, error)
	LibraryTitles(ctx context.Context) ([]library.LibraryTitle, error)
}

// ProfileStore keeps built profiles (library.TasteProfileRepository).
type ProfileStore interface {
	Insert(ctx context.Context, counts, profile json.RawMessage, pruneBefore time.Time) (*library.StoredTasteProfile, error)
	Latest(ctx context.Context) (*library.StoredTasteProfile, error)
}

// Settings reads Trakt's settings: use_ratings
// (library.SettingsRepository).
type Settings interface {
	TraktSettings(ctx context.Context) (library.TraktSettings, error)
}

// Details describes titles (tmdb.Cached).
type Details interface {
	MovieDetails(ctx context.Context, id int) (*tmdb.MovieDetails, error)
	ShowDetails(ctx context.Context, id int) (*tmdb.ShowDetails, error)
}

// Connection says whether Trakt is connected (trakt.Auth).
type Connection interface {
	Connected() bool
}

// Deps are what a Builder reads.
type Deps struct {
	Store    Store
	Profiles ProfileStore
	Settings Settings
	// TMDB describes the heaviest titles; nil uses the library's
	// metadata alone.
	TMDB  Details
	Trakt Connection // nil: never connected
	Now   func() time.Time
}

// keepUnreferenced is how long a profile no run references is kept.
const keepUnreferenced = 24 * time.Hour

// detailWorkers is how many TMDB details are fetched at once.
const detailWorkers = 4

// Builder builds taste profiles.
type Builder struct {
	d   Deps
	log *slog.Logger
}

// New creates a Builder.
func New(d Deps) *Builder {
	if d.Now == nil {
		d.Now = time.Now
	}
	return &Builder{d: d, log: slog.With("component", "taste-profile")}
}

// History reads everything the profile weighs. Six queries.
func (b *Builder) History(ctx context.Context) (*History, error) {
	var h History
	var err error
	if h.Movies, err = b.d.Store.WatchedMovies(ctx); err != nil {
		return nil, err
	}
	if h.Shows, err = b.d.Store.WatchedShows(ctx); err != nil {
		return nil, err
	}
	if h.Ratings, err = b.d.Store.TasteRatings(ctx); err != nil {
		return nil, err
	}
	if h.Hidden, err = b.d.Store.HiddenShows(ctx); err != nil {
		return nil, err
	}
	if h.Feedback, err = b.d.Store.TasteFeedback(ctx); err != nil {
		return nil, err
	}
	if h.Library, err = b.d.Store.LibraryTitles(ctx); err != nil {
		return nil, err
	}
	return &h, nil
}

func (b *Builder) useRatings(ctx context.Context) (bool, error) {
	settings, err := b.d.Settings.TraktSettings(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to read the Trakt settings: %w", err)
	}
	return settings.UseRatings, nil
}

func (b *Builder) connected() bool { return b.d.Trakt != nil && b.d.Trakt.Connected() }

// CheckHistory says whether a run can go ahead: Trakt connected and at
// least MinLoved loved titles (refresh's 409 no_history). It reads the
// database only, never TMDB.
func (b *Builder) CheckHistory(ctx context.Context) (Readiness, error) {
	useRatings, err := b.useRatings(ctx)
	if err != nil {
		return Readiness{}, err
	}
	h, err := b.History(ctx)
	if err != nil {
		return Readiness{}, err
	}
	return Readiness{Connected: b.connected(), Counts: NewPlan(h, useRatings, b.d.Now()).Counts()}, nil
}

// Build reads the history, has TMDB describe the heaviest titles, and
// builds the profile. It doesn't store it (Save does). A title TMDB can't
// describe falls back to the library's metadata, or is left out of the
// features.
func (b *Builder) Build(ctx context.Context) (*Profile, error) {
	useRatings, err := b.useRatings(ctx)
	if err != nil {
		return nil, err
	}
	h, err := b.History(ctx)
	if err != nil {
		return nil, err
	}
	plan := NewPlan(h, useRatings, b.d.Now())
	facts, err := b.describe(ctx, plan.WantDetails())
	if err != nil {
		return nil, err
	}
	return plan.Build(facts), nil
}

// describe fetches TMDB's details of refs, a few at a time. Only a
// cancelled ctx is an error: a title TMDB can't describe is skipped.
func (b *Builder) describe(ctx context.Context, refs []library.TitleRef) (map[library.TitleRef]Facts, error) {
	facts := map[library.TitleRef]Facts{}
	if b.d.TMDB == nil || len(refs) == 0 {
		return facts, nil
	}
	var (
		mu     sync.Mutex
		wg     sync.WaitGroup
		failed int
		noKey  bool
	)
	work := make(chan library.TitleRef)
	for i := 0; i < detailWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ref := range work {
				f, err := b.details(ctx, ref)
				mu.Lock()
				switch {
				case err == nil:
					facts[ref] = f
				case errors.Is(err, tmdb.ErrNoAPIKey):
					noKey = true
				default:
					failed++
				}
				mu.Unlock()
			}
		}()
	}
	for _, ref := range refs {
		mu.Lock()
		stop := noKey
		mu.Unlock()
		if stop || ctx.Err() != nil {
			break
		}
		work <- ref
	}
	close(work)
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if failed > 0 {
		b.log.Warn("TMDB couldn't describe some titles; the profile leaves them out", "titles", failed)
	}
	return facts, nil
}

func (b *Builder) details(ctx context.Context, ref library.TitleRef) (Facts, error) {
	if ref.MediaType == library.TitleMovie {
		d, err := b.d.TMDB.MovieDetails(ctx, ref.TMDBID)
		if err != nil {
			return Facts{}, err
		}
		return MovieFacts(d), nil
	}
	d, err := b.d.TMDB.ShowDetails(ctx, ref.TMDBID)
	if err != nil {
		return Facts{}, err
	}
	return ShowFacts(d), nil
}

// Stored is a profile as taste_profiles keeps it.
type Stored struct {
	ID        int64
	CreatedAt time.Time
	Profile   *Profile
}

// Save stores p as a taste_profiles row for a run to reference
// (rec_runs.profile_id). Profiles no run references are pruned after a
// day.
func (b *Builder) Save(ctx context.Context, p *Profile) (*Stored, error) {
	counts, err := json.Marshal(p.Counts)
	if err != nil {
		return nil, fmt.Errorf("failed to encode the profile's counts: %w", err)
	}
	doc, err := json.Marshal(p)
	if err != nil {
		return nil, fmt.Errorf("failed to encode the profile: %w", err)
	}
	row, err := b.d.Profiles.Insert(ctx, counts, doc, b.d.Now().Add(-keepUnreferenced))
	if err != nil {
		return nil, err
	}
	return &Stored{ID: row.ID, CreatedAt: row.CreatedAt, Profile: p}, nil
}

// Current is the newest stored profile while it's younger than maxAge;
// else it builds and saves a new one (Ask, between nightly runs).
func (b *Builder) Current(ctx context.Context, maxAge time.Duration) (*Stored, error) {
	row, err := b.d.Profiles.Latest(ctx)
	if err != nil {
		return nil, err
	}
	if row != nil && b.d.Now().Sub(row.CreatedAt) < maxAge {
		var p Profile
		if err := json.Unmarshal(row.Profile, &p); err == nil {
			return &Stored{ID: row.ID, CreatedAt: row.CreatedAt, Profile: &p}, nil
		}
		b.log.Warn("Stored taste profile doesn't decode; building a new one", "id", row.ID)
	}
	p, err := b.Build(ctx)
	if err != nil {
		return nil, err
	}
	return b.Save(ctx, p)
}
