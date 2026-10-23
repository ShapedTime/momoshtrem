package runs

import (
	"context"
	"encoding/json"
	"time"

	"github.com/shapedtime/momoshtrem/internal/catalog"
	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/recommend"
)

// ReadStore reads runs (library.RecRunRepository).
type ReadStore interface {
	ActiveRecRun(ctx context.Context, memberID int64) (*library.RecRun, error)
	GetRecRun(ctx context.Context, id int64) (*library.RecRun, error)
	LatestSucceededRecRun(ctx context.Context, memberID int64) (*library.RecRun, error)
	LatestFinishedRecRun(ctx context.Context, memberID int64) (*library.RecRun, error)
	ListRecRuns(ctx context.Context, memberID int64, limit int) ([]library.RecRun, error)
	RecRunShelves(ctx context.Context, runID int64) ([]library.RecShelfRow, error)
	RecRunHeroPicks(ctx context.Context, runID int64) ([]library.RecHeroPickRow, error)
	RecommendationReason(ctx context.Context, memberID int64, ref library.TitleRef) (*string, error)
	RecRunCostMeans(ctx context.Context, n int) (map[string]float64, error)
}

var _ ReadStore = (*library.RecRunRepository)(nil)

// LibraryStates is where titles are in the library, as a card shows it
// (catalog.Service).
type LibraryStates interface {
	LibraryStates(ctx context.Context, ids library.TMDBIDs) (map[catalog.TitleRef]*catalog.SimilarLibrary, error)
}

var _ LibraryStates = (*catalog.Service)(nil)

// ReaderDeps are what a Reader reads.
type ReaderDeps struct {
	Store    ReadStore
	Settings Settings
	// Titles answers a member's feedback and watchlist (their
	// TasteRepository's TitleStates, one query).
	Titles  func(memberID int64) Titles
	Library LibraryStates
	// Configured says whether ANTHROPIC_API_KEY is set.
	Configured bool
	Now        func() time.Time // nil: time.Now; its location is server-local
}

// Reader serves the picks from the database: /home, the runs, the title
// pages' reason and the usage page's part, each a member's (the run
// estimates are everyone's). It never calls TMDB.
type Reader struct {
	d ReaderDeps
}

// NewReader creates a Reader.
func NewReader(d ReaderDeps) *Reader {
	if d.Now == nil {
		d.Now = time.Now
	}
	return &Reader{d: d}
}

// EstimateRuns is how many succeeded runs a model's estimate averages.
const EstimateRuns = 5

// enabled reports whether picks are on: configured and rec_enabled.
func (r *Reader) enabled(ctx context.Context) (bool, error) {
	if !r.d.Configured {
		return false, nil
	}
	settings, err := r.d.Settings.AISettings(ctx)
	if err != nil {
		return false, err
	}
	return settings.RecEnabled, nil
}

// Home is the member's GET /recommendations/home: their newest succeeded
// run's shelves and the day's hero picks, with library state, their
// watchlist and feedback joined now, and their active run. Queries: the
// active run, the newest succeeded one, its shelves and items, its hero
// picks, then one each for feedback with the watchlist, and library
// state.
func (r *Reader) Home(ctx context.Context, memberID int64) (*recommend.Home, error) {
	home := &recommend.Home{Configured: r.d.Configured, HeroPicks: []recommend.HeroPick{}, Shelves: []recommend.Shelf{}}
	on, err := r.enabled(ctx)
	if err != nil || !on {
		return home, err
	}
	home.Enabled = true

	active, err := r.d.Store.ActiveRecRun(ctx, memberID)
	if err != nil {
		return nil, err
	}
	if active != nil {
		home.ActiveRun = ProgressOf(active)
	}
	run, err := r.d.Store.LatestSucceededRecRun(ctx, memberID)
	if err != nil || run == nil {
		return home, err
	}
	home.Run = SummaryOf(run)
	shelves, err := r.d.Store.RecRunShelves(ctx, run.ID)
	if err != nil {
		return nil, err
	}
	picks, err := r.d.Store.RecRunHeroPicks(ctx, run.ID)
	if err != nil {
		return nil, err
	}

	var refs []library.TitleRef
	seen := map[library.TitleRef]bool{}
	add := func(ref library.TitleRef) {
		if !seen[ref] {
			seen[ref] = true
			refs = append(refs, ref)
		}
	}
	for _, s := range shelves {
		for _, item := range s.Items {
			add(item.TitleRef)
		}
	}
	for _, p := range picks {
		add(p.TitleRef)
	}
	states, err := r.states(ctx, memberID, refs)
	if err != nil {
		return nil, err
	}
	libs, err := r.library(ctx, refs)
	if err != nil {
		return nil, err
	}

	for _, s := range shelves {
		shelf := recommend.Shelf{Slug: s.Slug, Kind: s.Kind, Title: s.Title, Subtitle: s.Subtitle, Items: []recommend.Item{}}
		for _, row := range s.Items {
			st := states[row.TitleRef]
			if negative(st.Feedback) {
				continue
			}
			shelf.Items = append(shelf.Items, recommend.Item{
				MediaType: row.MediaType, TMDBID: row.TMDBID, ItemMeta: decodeMeta(row.Meta), Reason: row.Reason,
				Library:     libs[catalog.TitleRef{MediaType: row.MediaType, TMDBID: row.TMDBID}],
				OnWatchlist: st.OnWatchlist, Feedback: liked(st.Feedback),
			})
		}
		if len(shelf.Items) > 0 {
			home.Shelves = append(home.Shelves, shelf)
		}
	}
	now := r.d.Now()
	finished := now
	if run.FinishedAt != nil {
		finished = *run.FinishedAt
	}
	for _, p := range DayPicks(picks, func(ref library.TitleRef) bool { return negative(states[ref].Feedback) }, finished, now) {
		home.HeroPicks = append(home.HeroPicks, recommend.HeroPick{
			MediaType: p.MediaType, TMDBID: p.TMDBID, Kicker: p.Kicker, Reason: p.Reason, Feedback: liked(states[p.TitleRef].Feedback),
		})
	}
	return home, nil
}

func (r *Reader) states(ctx context.Context, memberID int64, refs []library.TitleRef) (map[library.TitleRef]library.TitleState, error) {
	out := map[library.TitleRef]library.TitleState{}
	if len(refs) == 0 {
		return out, nil
	}
	list, err := r.d.Titles(memberID).TitleStates(ctx, refs)
	if err != nil {
		return nil, err
	}
	for _, s := range list {
		out[s.TitleRef] = s
	}
	return out, nil
}

func (r *Reader) library(ctx context.Context, refs []library.TitleRef) (map[catalog.TitleRef]*catalog.SimilarLibrary, error) {
	if len(refs) == 0 || r.d.Library == nil {
		return map[catalog.TitleRef]*catalog.SimilarLibrary{}, nil
	}
	var ids library.TMDBIDs
	for _, ref := range refs {
		if ref.MediaType == library.TitleMovie {
			ids.Movies = append(ids.Movies, ref.TMDBID)
		} else {
			ids.Shows = append(ids.Shows, ref.TMDBID)
		}
	}
	return r.d.Library.LibraryStates(ctx, ids)
}

// DayPicks are the hero picks for now's server-local day (PLAN §6 "Hero
// picks"): on the n-th day since the run finished (0 on its own day) they
// start at position 3n mod count and wrap, skipping titles with negative
// feedback, up to three.
func DayPicks(picks []library.RecHeroPickRow, skip func(library.TitleRef) bool, finished, now time.Time) []library.RecHeroPickRow {
	count := len(picks)
	if count == 0 {
		return nil
	}
	start := (recommend.HeroPicksPerDay * daysBetween(finished, now)) % count
	var out []library.RecHeroPickRow
	for i := 0; i < count && len(out) < recommend.HeroPicksPerDay; i++ {
		p := picks[(start+i)%count]
		if skip(p.TitleRef) {
			continue
		}
		out = append(out, p)
	}
	return out
}

// daysBetween counts the calendar days from a to b in b's location (the
// server-local zone), 0 when b is before a. Calendar days, not 24-hour
// spans: a DST change doesn't move the rotation.
func daysBetween(a, b time.Time) int {
	a = a.In(b.Location())
	ya, ma, da := a.Date()
	yb, mb, db := b.Date()
	days := int(time.Date(yb, mb, db, 0, 0, 0, 0, time.UTC).Sub(time.Date(ya, ma, da, 0, 0, 0, 0, time.UTC)).Hours() / 24)
	return max(days, 0)
}

// Runs lists the member's newest runs first.
func (r *Reader) Runs(ctx context.Context, memberID int64, limit int) ([]recommend.RunSummary, error) {
	rows, err := r.d.Store.ListRecRuns(ctx, memberID, limit)
	if err != nil {
		return nil, err
	}
	out := make([]recommend.RunSummary, 0, len(rows))
	for i := range rows {
		out = append(out, *SummaryOf(&rows[i]))
	}
	return out, nil
}

// Run is one of the member's runs with its progress and usage;
// recommend.ErrNotFound for an unknown one or another member's.
func (r *Reader) Run(ctx context.Context, memberID, id int64) (*recommend.RunDetail, error) {
	run, err := r.d.Store.GetRecRun(ctx, id)
	if err != nil {
		return nil, err
	}
	if run == nil || run.MemberID != memberID {
		return nil, recommend.ErrNotFound
	}
	return &recommend.RunDetail{
		RunProgress: *ProgressOf(run),
		Usage: recommend.Usage{
			APICalls: run.APICalls, InputTokens: run.InputTokens, OutputTokens: run.OutputTokens,
			CacheReadTokens: run.CacheReadTokens, WebSearches: run.WebSearches, CostUSD: run.CostUSD,
		},
	}, nil
}

// RecommendationReason is a title page's recommendation_reason for the
// member: their newest succeeded run's reason for it; nil without one,
// while picks are off, and after their negative feedback.
func (r *Reader) RecommendationReason(ctx context.Context, memberID int64, mediaType string, tmdbID int) (*string, error) {
	on, err := r.enabled(ctx)
	if err != nil || !on {
		return nil, err
	}
	return r.d.Store.RecommendationReason(ctx, memberID, library.TitleRef{MediaType: mediaType, TMDBID: tmdbID})
}

// LastFinished is the member's newest finished run (succeeded, failed or
// skipped), nil before their first.
func (r *Reader) LastFinished(ctx context.Context, memberID int64) (*recommend.RunSummary, error) {
	run, err := r.d.Store.LatestFinishedRecRun(ctx, memberID)
	if err != nil || run == nil {
		return nil, err
	}
	return SummaryOf(run), nil
}

// Active is the member's queued or running run, nil when there's none.
func (r *Reader) Active(ctx context.Context, memberID int64) (*recommend.RunProgress, error) {
	run, err := r.d.Store.ActiveRecRun(ctx, memberID)
	if err != nil || run == nil {
		return nil, err
	}
	return ProgressOf(run), nil
}

// RunEstimates is, per rec model, the mean cost of its newest 5
// succeeded runs, anyone's; nil for a model without any.
func (r *Reader) RunEstimates(ctx context.Context) (map[string]*float64, error) {
	means, err := r.d.Store.RecRunCostMeans(ctx, EstimateRuns)
	if err != nil {
		return nil, err
	}
	out := make(map[string]*float64, len(recommend.RecModels))
	for _, model := range recommend.RecModels {
		if mean, ok := means[model]; ok {
			out[model] = &mean
		} else {
			out[model] = nil
		}
	}
	return out, nil
}

// SummaryOf is a run as lists show it.
func SummaryOf(run *library.RecRun) *recommend.RunSummary {
	s := &recommend.RunSummary{
		ID: run.ID, Kind: run.Kind, Status: run.Status, Model: run.Model,
		StartedAt: run.StartedAt, FinishedAt: run.FinishedAt, CostUSD: run.CostUSD,
	}
	if run.Status == recommend.StatusSkipped {
		s.SkipReason = run.SkipReason
	}
	if run.Status == recommend.StatusFailed {
		s.Error = run.Error
	}
	return s
}

// ProgressOf is a run with its progress.
func ProgressOf(run *library.RecRun) *recommend.RunProgress {
	p := DecodeProgress(run.Progress)
	return &recommend.RunProgress{
		RunSummary: *SummaryOf(run), Steps: p.Steps, ShelvesDone: p.ShelvesDone, ShelvesTarget: p.ShelvesTarget,
	}
}

func decodeMeta(raw json.RawMessage) recommend.ItemMeta {
	var m recommend.ItemMeta
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &m)
	}
	return m
}

// negative reports feedback that hides a title from every AI surface.
func negative(signal string) bool {
	return signal == recommend.SignalDislike || signal == recommend.SignalNotInterested || signal == recommend.SignalSeen
}

// liked is an item's feedback as served: "like" or null.
func liked(signal string) *string {
	if signal != recommend.SignalLike {
		return nil
	}
	s := recommend.SignalLike
	return &s
}
