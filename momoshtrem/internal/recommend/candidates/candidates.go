// Package candidates is R1's candidate pool (PLAN §7 R1, #153): titles TMDB
// suggests for your taste profile, without anything you watched, dropped
// or turned down, pre-ranked by how well they overlap the profile. It's
// deterministic: no LLM. The picks agent reads it through get_candidates
// (internal/recommend/tools), and a run inlines it in its first turn.
//
// Entry point: Builder.Build(ctx, profile). Pool.Filter is get_candidates.
package candidates

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/recommend/profile"
	"github.com/shapedtime/momoshtrem/internal/tmdb"
)

// Pool sizes.
const (
	// PoolSize is how many candidates a pool keeps.
	PoolSize = 150
	// described is how many of the best TMDB describes (runtime, seasons,
	// keywords) before the final ranking.
	described = 200
	// seeds is how many loved titles lend their recommendations and
	// similar titles.
	seeds         = 10
	detailWorkers = 8
	maxWhy        = 3
)

// Candidate is a title the agent may pick, compactly.
type Candidate struct {
	MediaType string   `json:"media_type"`
	TMDBID    int      `json:"tmdb_id"`
	Title     string   `json:"title"`
	Date      string   `json:"date,omitempty"` // release or first air date, YYYY-MM-DD
	Genres    []string `json:"genres,omitempty"`
	Language  string   `json:"lang,omitempty"`
	Anime     bool     `json:"anime,omitempty"`
	Runtime   int      `json:"runtime,omitempty"` // minutes: a movie's, a show's episode
	Seasons   int      `json:"seasons,omitempty"` // shows, Specials left out
	Ended     bool     `json:"ended,omitempty"`   // shows that won't return
	Vote      float64  `json:"vote,omitempty"`    // TMDB's average, with enough votes
	// Why are its sources: "rec:Severance" and "similar:Dark" (a loved
	// title's recommendations and similar titles), "kw" (your keywords),
	// "new", "gems", "lang", "trending", "upcoming", "on_air".
	Why         []string `json:"why"`
	InLibrary   bool     `json:"in_library,omitempty"`
	OnWatchlist bool     `json:"watchlist,omitempty"`

	year       int
	genreIDs   []int
	keywordIDs []int
	score      float64
}

// Ref is the candidate's title.
func (c *Candidate) Ref() library.TitleRef {
	return library.TitleRef{MediaType: c.MediaType, TMDBID: c.TMDBID}
}

// Year is its release or first air year, 0 when unknown.
func (c *Candidate) Year() int { return c.year }

// Score is its pre-rank: how well it overlaps the profile.
func (c *Candidate) Score() float64 { return c.score }

// Stats say what the pool came from: the step detail "148 candidates ·
// 23 already seen".
type Stats struct {
	Considered int `json:"considered"` // distinct titles the sources gave
	Excluded   int `json:"excluded"`   // watched, started, hidden, or turned down
	Kept       int `json:"kept"`
}

// Pool is the ranked candidates, best first.
type Pool struct {
	BuiltAt time.Time   `json:"built_at"`
	Stats   Stats       `json:"stats"`
	Items   []Candidate `json:"items"`
}

// Lists are TMDB's lists and details (tmdb.Cached).
type Lists interface {
	MovieDetails(ctx context.Context, id int) (*tmdb.MovieDetails, error)
	ShowDetails(ctx context.Context, id int) (*tmdb.ShowDetails, error)
	Discover(ctx context.Context, mediaType string, q tmdb.DiscoverQuery) (*tmdb.ResultsPage, error)
	Trending(ctx context.Context, mediaType, window string) (*tmdb.ResultsPage, error)
	UpcomingMovies(ctx context.Context, page int) (*tmdb.ResultsPage, error)
	OnTheAir(ctx context.Context, page int) (*tmdb.ResultsPage, error)
	GenreList(ctx context.Context, mediaType string) ([]tmdb.Genre, error)
}

// Store reads what excludes and marks titles (library.TasteRepository).
type Store interface {
	WatchedMovies(ctx context.Context) ([]library.WatchedMovie, error)
	WatchedShows(ctx context.Context) ([]library.WatchedShow, error)
	HiddenShows(ctx context.Context) ([]library.HiddenShow, error)
	TasteFeedback(ctx context.Context) ([]library.TasteFeedback, error)
	LibraryTitles(ctx context.Context) ([]library.LibraryTitle, error)
	WatchlistRefs(ctx context.Context) ([]library.TitleRef, error)
}

// Deps are what a Builder reads.
type Deps struct {
	TMDB  Lists
	Store Store
	Now   func() time.Time // time.Now when nil; its location is server-local
}

// Builder builds candidate pools.
type Builder struct {
	d   Deps
	log *slog.Logger
}

// New creates a Builder.
func New(d Deps) *Builder {
	if d.Now == nil {
		d.Now = time.Now
	}
	return &Builder{d: d, log: slog.With("component", "candidates")}
}

// ErrNoSources means every TMDB source failed (TMDB down, or no key).
var ErrNoSources = errors.New("TMDB answered none of the candidate sources")

// Known reads what the database knows about titles in bulk. Five queries.
func (b *Builder) Known(ctx context.Context) (*Known, error) {
	movies, err := b.d.Store.WatchedMovies(ctx)
	if err != nil {
		return nil, err
	}
	shows, err := b.d.Store.WatchedShows(ctx)
	if err != nil {
		return nil, err
	}
	hidden, err := b.d.Store.HiddenShows(ctx)
	if err != nil {
		return nil, err
	}
	feedback, err := b.d.Store.TasteFeedback(ctx)
	if err != nil {
		return nil, err
	}
	titles, err := b.d.Store.LibraryTitles(ctx)
	if err != nil {
		return nil, err
	}
	watchlist, err := b.d.Store.WatchlistRefs(ctx)
	if err != nil {
		return nil, err
	}
	return NewKnown(movies, shows, hidden, feedback, titles, watchlist), nil
}

// Build gathers the candidates for p: TMDB's recommendations and similar
// titles for the top loved titles, discover by your keywords and genres
// in a few windows, trending, upcoming and on the air. It drops what's
// excluded, ranks the rest by overlap, has TMDB describe the best, and
// keeps PoolSize. A source TMDB fails is skipped; all of them failing is
// ErrNoSources.
func (b *Builder) Build(ctx context.Context, p *profile.Profile) (*Pool, error) {
	known, err := b.Known(ctx)
	if err != nil {
		return nil, err
	}
	now := b.d.Now()
	genreNames := b.genreNames(ctx)
	sources := b.sources(p, now, genreNames)

	raw, failed := b.fetch(ctx, p, sources)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if failed == len(sources)+min(seeds, len(p.Loved)) && failed > 0 {
		return nil, ErrNoSources
	}
	if failed > 0 {
		b.log.Warn("Some candidate sources failed; the pool goes without them", "failed", failed)
	}

	today := now.Format(time.DateOnly)
	merged := merge(raw, today, genreNames)
	pool := &Pool{BuiltAt: now, Stats: Stats{Considered: len(merged)}}
	var kept []*Candidate
	for _, c := range merged {
		ref := c.Ref()
		if known.Exclusion(ref) != "" {
			pool.Stats.Excluded++
			continue
		}
		c.InLibrary, c.OnWatchlist = known.InLibrary(ref), known.OnWatchlist(ref)
		kept = append(kept, c)
	}

	s := newScorer(p)
	rank(kept, s)
	if len(kept) > described {
		kept = kept[:described]
	}
	if err := b.describe(ctx, kept); err != nil {
		return nil, err
	}
	rank(kept, s)
	if len(kept) > PoolSize {
		kept = kept[:PoolSize]
	}
	pool.Items = make([]Candidate, len(kept))
	for i, c := range kept {
		pool.Items[i] = *c
		if len(c.Why) > maxWhy {
			pool.Items[i].Why = c.Why[:maxWhy:maxWhy]
		}
	}
	pool.Stats.Kept = len(pool.Items)
	return pool, nil
}

// genreNames maps each media type's genre ids to their names; a list TMDB
// fails is empty.
func (b *Builder) genreNames(ctx context.Context) map[string]map[int]string {
	names := map[string]map[int]string{}
	for _, mt := range []string{tmdb.MediaMovie, tmdb.MediaTV} {
		names[mt] = map[int]string{}
		genres, err := b.d.TMDB.GenreList(ctx, mt)
		if err != nil {
			b.log.Warn("TMDB genre list unavailable", "media_type", mt, "error", err)
			continue
		}
		for _, g := range genres {
			names[mt][g.ID] = g.Name
		}
	}
	return names
}

// found is one source's titles. order sorts the sources so a pool doesn't
// depend on which answered first: the seeds' first, in rank.
type found struct {
	order  int
	source string
	items  []tmdb.ListItem
}

// fetch runs the sources and the seeds' details, a few at a time. failed
// counts the ones TMDB didn't answer.
func (b *Builder) fetch(ctx context.Context, p *profile.Profile, sources []source) (raw []found, failed int) {
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, detailWorkers)
	run := func(f func() ([]found, error)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			got, err := f()
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				failed++
				return
			}
			raw = append(raw, got...)
		}()
	}
	for i, t := range p.Loved {
		if i == seeds {
			break
		}
		run(func() ([]found, error) { return b.seed(ctx, i, t) })
	}
	for i, src := range sources {
		run(func() ([]found, error) {
			page, err := src.fetch(ctx, b.d.TMDB)
			if err != nil {
				return nil, err
			}
			return []found{{order: 2*seeds + i, source: src.name, items: page.Results}}, nil
		})
	}
	wg.Wait()
	slices.SortStableFunc(raw, func(a, b found) int { return cmp.Compare(a.order, b.order) })
	return raw, failed
}

// seed is the rank-th loved title's recommendations and similar titles.
func (b *Builder) seed(ctx context.Context, rank int, t profile.Title) ([]found, error) {
	var recs, similar []tmdb.ListItem
	if t.MediaType == library.TitleMovie {
		d, err := b.d.TMDB.MovieDetails(ctx, t.TMDBID)
		if err != nil {
			return nil, err
		}
		recs, similar = d.Recommendations.Results, d.Similar.Results
	} else {
		d, err := b.d.TMDB.ShowDetails(ctx, t.TMDBID)
		if err != nil {
			return nil, err
		}
		recs, similar = d.Recommendations.Results, d.Similar.Results
	}
	// TMDB's details lists don't always say what they list: the seed's
	// own media type, unless an item says otherwise
	mark := func(items []tmdb.ListItem) []tmdb.ListItem {
		out := make([]tmdb.ListItem, len(items))
		for i, item := range items {
			if item.MediaType == "" {
				item.MediaType = t.MediaType
			}
			out[i] = item
		}
		return out
	}
	name := clipName(t.Title)
	return []found{{2 * rank, "rec:" + name, mark(recs)}, {2*rank + 1, "similar:" + name, mark(similar)}}, nil
}

func clipName(s string) string {
	if r := []rune(s); len(r) > 30 {
		return string(r[:29]) + "…"
	}
	return s
}

// merge turns the sources' titles into candidates, one per title, in the
// order first seen. Upcoming titles already out count as new.
func merge(raw []found, today string, genreNames map[string]map[int]string) []*Candidate {
	byRef := map[library.TitleRef]*Candidate{}
	var out []*Candidate
	for _, f := range raw {
		for _, item := range f.items {
			mt := item.MediaType
			if mt != tmdb.MediaMovie && mt != tmdb.MediaTV || item.ID <= 0 || item.DisplayTitle() == "" {
				continue
			}
			ref := library.TitleRef{MediaType: mt, TMDBID: item.ID}
			c := byRef[ref]
			if c == nil {
				c = fromItem(mt, item, genreNames[mt])
				byRef[ref] = c
				out = append(out, c)
			}
			src := f.source
			if src == "upcoming" && c.Date != "" && c.Date <= today {
				src = "new"
			}
			c.addWhy(src)
		}
	}
	return out
}

func fromItem(mediaType string, item tmdb.ListItem, genreNames map[int]string) *Candidate {
	c := &Candidate{MediaType: mediaType, TMDBID: item.ID, Title: item.DisplayTitle(), Language: item.OriginalLanguage,
		genreIDs: item.GenreIDs, year: item.Year()}
	c.Date = item.ReleaseDate
	if mediaType == tmdb.MediaTV {
		c.Date = item.FirstAirDate
	}
	for _, id := range item.GenreIDs {
		if name := genreNames[id]; name != "" {
			c.Genres = append(c.Genres, name)
		}
		if id == tmdb.GenreAnimation && (item.OriginalLanguage == "ja" || contains(item.OriginCountry, "JP")) {
			c.Anime = true
		}
	}
	if item.VoteCount >= minVotes {
		c.Vote = round1(item.VoteAverage)
	}
	return c
}

func (c *Candidate) addWhy(src string) {
	for _, w := range c.Why {
		if w == src {
			return
		}
	}
	c.Why = append(c.Why, src)
}

// describe has TMDB describe each candidate (runtime, seasons, keywords,
// whether a show ended), a few at a time. A title TMDB can't describe
// keeps what the lists said; only a cancelled ctx is an error.
func (b *Builder) describe(ctx context.Context, cands []*Candidate) error {
	var wg sync.WaitGroup
	sem := make(chan struct{}, detailWorkers)
	for _, c := range cands {
		if ctx.Err() != nil {
			break
		}
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			b.describeOne(ctx, c)
		}()
	}
	wg.Wait()
	return ctx.Err()
}

func (b *Builder) describeOne(ctx context.Context, c *Candidate) {
	if c.MediaType == tmdb.MediaMovie {
		d, err := b.d.TMDB.MovieDetails(ctx, c.TMDBID)
		if err != nil {
			return
		}
		c.Runtime = d.Runtime
		c.keywordIDs = keywordIDs(d.Keywords.Keywords)
		if !c.Anime {
			c.Anime = isAnime(d.Genres, d.OriginalLanguage, d.OriginCountry, d.Keywords.Keywords)
		}
		return
	}
	d, err := b.d.TMDB.ShowDetails(ctx, c.TMDBID)
	if err != nil {
		return
	}
	c.Runtime = episodeRuntime(d)
	for _, s := range d.Seasons {
		if s.SeasonNumber > 0 {
			c.Seasons++
		}
	}
	c.Ended = d.Status == "Ended" || d.Status == "Canceled" || d.Status == "Cancelled"
	c.keywordIDs = keywordIDs(d.Keywords.Keywords)
	if !c.Anime {
		c.Anime = isAnime(d.Genres, d.OriginalLanguage, d.OriginCountry, d.Keywords.Keywords)
	}
}

// episodeRuntime is a show's usual episode runtime: TMDB's list, else its
// last episode's.
func episodeRuntime(d *tmdb.ShowDetails) int {
	if len(d.EpisodeRunTime) > 0 {
		return d.EpisodeRunTime[0]
	}
	return 0
}

func keywordIDs(ks []tmdb.Keyword) []int {
	ids := make([]int, len(ks))
	for i, k := range ks {
		ids[i] = k.ID
	}
	return ids
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if strings.EqualFold(x, s) {
			return true
		}
	}
	return false
}

// String is the pool's summary, for logs.
func (p *Pool) String() string {
	return fmt.Sprintf("%d candidates (%d considered, %d excluded)", p.Stats.Kept, p.Stats.Considered, p.Stats.Excluded)
}
