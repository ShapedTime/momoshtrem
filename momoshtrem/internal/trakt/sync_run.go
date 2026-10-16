package trakt

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/shapedtime/momoshtrem/internal/library"
)

const (
	// The calendar covers 7 days back and 33 days in all (Trakt's most).
	calendarBack = 7
	calendarDays = 33
	// historyOverlap goes back before the last known play: the overlap is
	// upserted onto the same rows by event id.
	historyOverlap = time.Hour
	detailWorkers  = 4
)

// fullSteps are an initial import's and a reconcile's steps, in order.
var fullSteps = []SyncStep{StepHistory, StepRatings, StepWatchlist, StepProgress, StepDetails}

var allRatingTypes = []string{"movies", "shows", "seasons", "episodes"}

// plan is what a sync does.
type plan struct {
	kind SyncKind
	seq  int
	// A failed import picked up again: the step it failed at, its
	// last_activities snapshot and what it had imported.
	from     int
	snapshot json.RawMessage
	progress SyncProgress
	// prev is the stored state: an incremental sync compares with it.
	prev library.TraktSyncState
}

// run is one sync.
type run struct {
	s          *Syncer
	ctx        context.Context
	plan       plan
	gen        time.Time       // the connection it writes for
	step       SyncStep        // the running step
	activities json.RawMessage // this sync's last_activities snapshot
	result     SyncRun         // how it finished, when it did
}

// execute runs a sync: every request takes the connection's token, every
// write names its generation.
func (s *Syncer) execute(ctx context.Context, p plan) (*run, error) {
	r := &run{s: s, ctx: ctx, plan: p, step: fullSteps[p.from]}
	tok, err := s.auth.Token(ctx)
	if err != nil {
		return r, err
	}
	r.gen = tok.Generation
	if p.kind == SyncIncremental {
		return r, r.incremental()
	}
	return r, r.full()
}

// full is the initial import and the reconcile: every section replaced, in
// transactions, which also catches deleted and backdated plays. A failed
// import is picked up at the step that failed, with its snapshot.
func (r *run) full() error {
	from := r.plan.from
	r.activities = r.plan.snapshot
	if r.activities == nil {
		from = 0
		var err error
		if r.activities, _, err = r.lastActivities(); err != nil {
			return err
		}
	}
	for _, step := range fullSteps[from:] {
		r.setStep(step)
		var err error
		switch step {
		case StepHistory:
			err = r.history()
		case StepRatings:
			err = r.ratings(allRatingTypes...)
		case StepWatchlist:
			err = r.watchlist()
		case StepProgress:
			err = each(r.hidden, r.progress, r.calendar)
		case StepDetails:
			err = r.details()
		}
		if err != nil {
			return err
		}
	}
	return r.done(true)
}

// incremental compares last_activities with the stored snapshot and
// fetches only what changed; nothing changed is one request. History comes
// as deltas from just before the last known play, so a play added with an
// older date waits for the reconcile.
func (r *run) incremental() error {
	raw, cur, err := r.lastActivities()
	if err != nil {
		return err
	}
	r.activities = raw
	var prev lastActivities
	if err := json.Unmarshal(r.plan.prev.Activities, &prev); err != nil {
		r.s.log.Warn("Unreadable stored last_activities; fetching every section", "error", err)
	}

	changed := func(a, b time.Time) bool { return !a.Equal(b) }
	movies := changed(prev.Movies.WatchedAt, cur.Movies.WatchedAt)
	episodes := changed(prev.Episodes.WatchedAt, cur.Episodes.WatchedAt)
	var ratings []string
	for _, c := range []struct {
		typ      string
		was, now time.Time
	}{
		{"movies", prev.Movies.RatedAt, cur.Movies.RatedAt},
		{"shows", prev.Shows.RatedAt, cur.Shows.RatedAt},
		{"seasons", prev.Seasons.RatedAt, cur.Seasons.RatedAt},
		{"episodes", prev.Episodes.RatedAt, cur.Episodes.RatedAt},
	} {
		if changed(c.was, c.now) {
			ratings = append(ratings, c.typ)
		}
	}
	watchlist := changed(prev.Watchlist.UpdatedAt, cur.Watchlist.UpdatedAt) ||
		changed(prev.Movies.WatchlistedAt, cur.Movies.WatchlistedAt) || changed(prev.Shows.WatchlistedAt, cur.Shows.WatchlistedAt)
	hidden := changed(prev.Shows.HiddenAt, cur.Shows.HiddenAt) || changed(prev.Shows.DroppedAt, cur.Shows.DroppedAt) ||
		changed(prev.Seasons.HiddenAt, cur.Seasons.HiddenAt)
	calendar := episodes || r.s.clock.Now().Sub(r.plan.prev.CalendarFetchedAt) > calendarMaxAge

	if movies || episodes {
		lastMovie, lastEpisode, err := r.s.store.LatestPlays(r.ctx)
		if err != nil {
			return err
		}
		if movies {
			if err := r.historySince("movies", earliest(prev.Movies.WatchedAt, lastMovie)); err != nil {
				return err
			}
		}
		if episodes {
			if err := r.historySince("episodes", earliest(prev.Episodes.WatchedAt, lastEpisode)); err != nil {
				return err
			}
		}
	}
	if len(ratings) > 0 {
		r.setStep(StepRatings)
		if err := r.ratings(ratings...); err != nil {
			return err
		}
	}
	if watchlist {
		r.setStep(StepWatchlist)
		if err := r.watchlist(); err != nil {
			return err
		}
	}
	if hidden || episodes || calendar {
		r.setStep(StepProgress)
		var steps []func() error
		if hidden {
			steps = append(steps, r.hidden)
		}
		if hidden || episodes {
			steps = append(steps, r.progress)
		}
		if calendar {
			steps = append(steps, r.calendar)
		}
		if err := each(steps...); err != nil {
			return err
		}
	}
	if movies || episodes || len(ratings) > 0 || watchlist || hidden || calendar {
		r.setStep(StepDetails)
		if err := r.details(); err != nil {
			return err
		}
	}
	return r.done(false)
}

// done stores the snapshot, last_sync_at (and last_reconcile_at) and the
// run, once every fetch succeeded.
func (r *run) done(reconcile bool) error {
	now := r.s.clock.Now()
	r.s.update(r.plan.seq, func(run *SyncRun) { r.result = *run })
	r.result.Status, r.result.Step, r.result.Error, r.result.FinishedAt = SyncIdle, nil, nil, ptr(now)
	raw, err := json.Marshal(r.result)
	if err != nil {
		return fmt.Errorf("failed to encode the Trakt sync run: %w", err)
	}
	if err := r.s.store.FinishSync(r.ctx, r.gen, library.TraktSyncDone{
		Activities: r.activities, At: now, Reconcile: reconcile, Run: raw,
	}); err != nil {
		return err
	}
	// Still syncing as far as Status says: the hook runs before the import
	// reports idle.
	r.s.mu.Lock()
	onImported := r.s.onImported
	r.s.mu.Unlock()
	if r.plan.kind == SyncInitial && onImported != nil {
		onImported(r.ctx)
	}
	return nil
}

func (r *run) setStep(step SyncStep) {
	r.step = step
	r.s.update(r.plan.seq, func(run *SyncRun) { run.Step = ptr(step) })
}

func (r *run) count(fn func(*SyncProgress)) {
	r.s.update(r.plan.seq, func(run *SyncRun) { fn(&run.Progress) })
}

// lastActivities snapshots GET /sync/last_activities.
func (r *run) lastActivities() (json.RawMessage, lastActivities, error) {
	var raw json.RawMessage
	if _, err := r.s.auth.Get(r.ctx, "/sync/last_activities", nil, &raw); err != nil {
		return nil, lastActivities{}, err
	}
	var la lastActivities
	if err := json.Unmarshal(raw, &la); err != nil {
		return nil, lastActivities{}, fmt.Errorf("failed to decode Trakt's last_activities: %w", err)
	}
	return raw, la, nil
}

// history replaces every play.
func (r *run) history() error {
	r.count(func(p *SyncProgress) { p.Plays, p.PlaysTotal = 0, 0 })
	var plays []library.TraktPlayRow
	for _, typ := range []string{"movies", "episodes"} {
		got, err := r.fetchHistory(typ, nil)
		if err != nil {
			return err
		}
		plays = append(plays, got...)
	}
	return r.s.store.ReplaceHistory(r.ctx, r.gen, plays)
}

// historySince upserts the plays of typ (movies|episodes) from an hour
// before since; zero since is all of them.
func (r *run) historySince(typ string, since time.Time) error {
	query := url.Values{"end_at": {isoTime(r.s.clock.Now())}}
	if !since.IsZero() {
		query.Set("start_at", isoTime(since.Add(-historyOverlap)))
	}
	plays, err := r.fetchHistory(typ, query)
	if err != nil {
		return err
	}
	return r.s.store.UpsertHistory(r.ctx, r.gen, plays)
}

func (r *run) fetchHistory(typ string, query url.Values) ([]library.TraktPlayRow, error) {
	var plays []library.TraktPlayRow
	g := totalTap{Getter: r.s.auth, total: func(n int) { r.count(func(p *SyncProgress) { p.PlaysTotal += n }) }}
	err := Paginate(r.ctx, g, "/sync/history/"+typ, query, func(items []historyItem) error {
		for _, it := range items {
			if p, ok := it.play(); ok {
				plays = append(plays, p)
			}
		}
		r.count(func(p *SyncProgress) { p.Plays += len(items) })
		return nil
	})
	return plays, err
}

// ratings replaces the ratings of types (movies, shows, seasons,
// episodes).
func (r *run) ratings(types ...string) error {
	var rows []library.TraktRatingRow
	mediaTypes := make([]string, len(types))
	for i, typ := range types {
		mediaType := typ[:len(typ)-1] // movies → movie
		mediaTypes[i] = mediaType
		err := Paginate(r.ctx, r.s.auth, "/sync/ratings/"+typ, nil, func(items []ratingItem) error {
			for _, it := range items {
				if row, ok := it.rating(mediaType); ok {
					rows = append(rows, row)
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	if err := r.s.store.ReplaceRatings(r.ctx, r.gen, mediaTypes, rows); err != nil {
		return err
	}
	r.count(func(p *SyncProgress) { p.Ratings = len(rows) })
	return nil
}

// watchlist replaces the watchlisted movies and shows. Trakt answers 10
// without page and limit, so it's always paginated.
func (r *run) watchlist() error {
	var items []library.TraktWatchlistRow
	for _, typ := range []string{"movies", "shows"} {
		err := Paginate(r.ctx, r.s.auth, "/sync/watchlist/"+typ+"/added/desc", nil, func(page []watchlistItem) error {
			for _, it := range page {
				if item, ok := it.item(); ok {
					items = append(items, item)
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	if err := r.s.store.ReplaceWatchlist(r.ctx, r.gen, items); err != nil {
		return err
	}
	r.count(func(p *SyncProgress) { p.Watchlist = len(items) })
	return nil
}

// hidden replaces the dropped shows and those hidden from progress.
func (r *run) hidden() error {
	var shows []library.TraktHiddenRow
	for _, h := range []struct {
		section string
		query   url.Values
	}{
		{"dropped", nil},
		{"progress_watched", url.Values{"type": {"show"}}},
	} {
		err := Paginate(r.ctx, r.s.auth, "/users/hidden/"+h.section, h.query, func(items []hiddenItem) error {
			for _, it := range items {
				if it.Show != nil && it.Show.IDs.TMDB != 0 {
					shows = append(shows, library.TraktHiddenRow{Section: h.section, TraktID: it.Show.IDs.Trakt, TMDBID: it.Show.IDs.TMDB})
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	return r.s.store.ReplaceHidden(r.ctx, r.gen, shows)
}

// progress replaces every show's progress, from the bulk endpoint (never
// one request per show).
func (r *run) progress() error {
	var shows []library.TraktShowProgressRow
	err := Paginate(r.ctx, r.s.auth, "/sync/progress/watched", nil, func(items []progressItem) error {
		for _, it := range items {
			if row, ok := it.progress(); ok {
				shows = append(shows, row)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	if err := r.s.store.ReplaceShowProgress(r.ctx, r.gen, shows); err != nil {
		return err
	}
	r.count(func(p *SyncProgress) { p.Shows = len(shows) })
	return nil
}

// calendar replaces the calendar: from 7 days ago (UTC), 33 days. It
// isn't paginated.
func (r *run) calendar() error {
	now := r.s.clock.Now().UTC()
	path := fmt.Sprintf("/calendars/my/shows/%s/%d", now.AddDate(0, 0, -calendarBack).Format(time.DateOnly), calendarDays)
	var items []calendarItem
	if _, err := r.s.auth.Get(r.ctx, path, nil, &items); err != nil {
		return err
	}
	entries := make([]library.TraktCalendarRow, 0, len(items))
	for _, it := range items {
		if e, ok := it.entry(); ok {
			entries = append(entries, e)
		}
	}
	return r.s.store.ReplaceCalendar(r.ctx, r.gen, entries, now)
}

// details fills the artwork of rows outside the library that miss it, from
// TMDB (the cache first), 4 at a time. A title TMDB can't answer for stays
// without; only a cancelled sync or a failed write fails the step.
func (r *run) details() error {
	if r.s.art == nil {
		return nil
	}
	wants, err := r.s.store.ArtworkWants(r.ctx)
	if err != nil {
		return err
	}
	stills := map[int]map[int][]int{} // show → season → episodes
	for _, e := range wants.Episodes {
		if stills[e.ShowTMDBID] == nil {
			stills[e.ShowTMDBID] = map[int][]int{}
		}
		stills[e.ShowTMDBID][e.Season] = append(stills[e.ShowTMDBID][e.Season], e.Episode)
	}
	shows := slices.Clone(wants.Shows)
	for id := range stills {
		if !slices.Contains(shows, id) {
			shows = append(shows, id)
		}
	}
	if len(wants.Movies)+len(shows) == 0 {
		return nil
	}

	found := library.TraktArtwork{
		Movies: map[int]library.TraktImages{}, Shows: map[int]library.TraktImages{},
		Stills: map[library.TraktStillKey]string{},
	}
	var mu sync.Mutex
	failed, firstErr := 0, error(nil)
	miss := func(err error) {
		mu.Lock()
		defer mu.Unlock()
		if failed++; firstErr == nil {
			firstErr = err
		}
	}
	var g errgroup.Group
	g.SetLimit(detailWorkers)
	for _, id := range wants.Movies {
		g.Go(func() error {
			m, err := r.s.art.MovieDetails(r.ctx, id)
			if err != nil {
				miss(err)
				return nil
			}
			mu.Lock()
			found.Movies[id] = library.TraktImages{Poster: m.PosterPath, Backdrop: m.BackdropPath}
			mu.Unlock()
			return nil
		})
	}
	for _, id := range shows {
		g.Go(func() error {
			d, err := r.s.art.ShowDetails(r.ctx, id)
			if err != nil {
				miss(err)
				return nil
			}
			mu.Lock()
			found.Shows[id] = library.TraktImages{Poster: d.PosterPath, Backdrop: d.BackdropPath}
			mu.Unlock()
			for number, episodes := range stills[id] {
				season, err := r.s.art.Season(r.ctx, id, number, d.Status)
				if err != nil {
					miss(err)
					continue
				}
				mu.Lock()
				for _, e := range season.Episodes {
					if slices.Contains(episodes, e.EpisodeNumber) {
						found.Stills[library.TraktStillKey{ShowTMDBID: id, Season: number, Episode: e.EpisodeNumber}] = e.StillPath
					}
				}
				mu.Unlock()
			}
			return nil
		})
	}
	_ = g.Wait()
	if err := r.ctx.Err(); err != nil {
		return err
	}
	if failed > 0 {
		r.s.log.Warn("Some Trakt artwork isn't on TMDB or TMDB didn't answer", "lookups", failed, "error", firstErr)
	}
	return r.s.store.SetArtwork(r.ctx, r.gen, found)
}

// totalTap reports a list's X-Pagination-Item-Count, read on its first
// page: Trakt's count.
type totalTap struct {
	Getter
	total func(int)
}

func (t totalTap) Get(ctx context.Context, path string, query url.Values, v any) (http.Header, error) {
	header, err := t.Getter.Get(ctx, path, query, v)
	if err == nil && query.Get("page") == "1" {
		if n, err := strconv.Atoi(header.Get("X-Pagination-Item-Count")); err == nil {
			t.total(n)
		}
	}
	return header, err
}

// syncErrorMessage is a failed sync's error, for the user; the log has
// the details.
func syncErrorMessage(step SyncStep, err error) string {
	what := map[SyncStep]string{
		StepHistory: "your history", StepRatings: "your ratings", StepWatchlist: "your watchlist",
		StepProgress: "your show progress", StepDetails: "artwork",
	}[step]
	var se *StatusError
	var ue *url.Error
	switch {
	case errors.As(err, &se):
		return fmt.Sprintf("Trakt didn’t answer while importing %s (%d).", what, se.StatusCode)
	case errors.As(err, &ue):
		return fmt.Sprintf("Couldn’t reach Trakt while importing %s.", what)
	default:
		return fmt.Sprintf("The sync stopped while importing %s.", what)
	}
}

func stepIndex(step SyncStep) int {
	return max(slices.Index(fullSteps, step), 0)
}

// earliest is the earlier of two times, ignoring a zero one.
func earliest(a, b time.Time) time.Time {
	if a.IsZero() || (!b.IsZero() && b.Before(a)) {
		return b
	}
	return a
}

// isoTime is a full ISO-8601 UTC time, as Trakt's start_at and end_at
// take.
func isoTime(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05.000Z")
}

func each(fns ...func() error) error {
	for _, fn := range fns {
		if err := fn(); err != nil {
			return err
		}
	}
	return nil
}
