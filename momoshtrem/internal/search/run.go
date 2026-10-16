package search

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/service"
	"github.com/shapedtime/momoshtrem/internal/torznab"
)

// requestLimit is the results each Torznab request asks for, capped at the
// indexer's maximum.
const requestLimit = 100

// Fan-out concurrency: the client's 2 requests per indexer, and enough
// overall that indexers timing out can't hold every slot.
const (
	perIndexer = 2
	maxTotal   = 20
)

// entry is one plan's search: its progress and the releases in scope so
// far. Everything below mu changes while the search runs.
type entry struct {
	key       string
	plan      *QueryPlan
	tc        *service.TitleContext // the context the search ran with
	startedAt time.Time
	ctx       context.Context // the search's own: detached, with its deadline
	cancel    context.CancelFunc
	done      chan struct{} // closed when the search has finished

	mu       sync.Mutex
	complete bool
	err      error // ErrJackettUnavailable: the search couldn't start
	queries  []SentQuery
	order    []string // indexer ids, in first-request order
	indexers map[string]*indexerState
	items    []*item
	byHash   map[string]*item   // dedupe by info hash
	byName   map[string][]*item // dedupe by normalised title, then size
	warnings []string
	// showSeasons is how many regular seasons the run's catalog has.
	showSeasons int
}

func newEntry(plan *QueryPlan, tc *service.TitleContext, now time.Time, timeout time.Duration) *entry {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	e := &entry{
		key: plan.Key, plan: plan, tc: tc, startedAt: now, ctx: ctx, cancel: cancel,
		done:     make(chan struct{}),
		indexers: map[string]*indexerState{},
		byHash:   map[string]*item{},
		byName:   map[string][]*item{},
	}
	for _, q := range plan.TextQueries() {
		e.queries = append(e.queries, sentQuery(q))
	}
	return e
}

func sentQuery(q Query) SentQuery {
	sq := SentQuery{Q: q.Text}
	if q.Label != "" {
		label := q.Label
		sq.Label = &label
	}
	return sq
}

// state reports whether the search is still running and whether it failed.
func (e *entry) state() (running, failed bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return !e.complete, e.err != nil
}

// indexerState is one indexer over every request of a search, across the
// exact and fallback passes.
type indexerState struct {
	name   string
	status torznab.Status
	err    string
	count  int
	doneAt time.Time // when it last stopped being pending
	// Before the current pass.
	baseCount  int
	baseStatus torznab.Status
	baseErr    string
}

// startPass marks the indexer pending for a new fan-out, keeping what the
// earlier passes found.
func (st *indexerState) startPass() {
	st.baseCount, st.baseStatus, st.baseErr = st.count, st.status, st.err
	st.status = torznab.StatusPending
}

// update applies a fan-out report: Count and Status are over the pass's
// requests to this indexer. A failure in an earlier pass stays the first.
func (st *indexerState) update(p torznab.Progress, now time.Time) {
	st.count = st.baseCount + p.Count
	if p.Status == torznab.StatusPending {
		st.status = torznab.StatusPending
		return
	}
	st.doneAt = now
	if st.baseStatus == torznab.StatusTimeout || st.baseStatus == torznab.StatusError {
		st.status, st.err = st.baseStatus, st.baseErr
		return
	}
	st.status, st.err = p.Status, p.Error
}

// run searches for e's plan: the indexer list, the exact queries on every
// indexer and, when they found nothing in scope, the fallback.
func (s *Service) run(e *entry) {
	start := time.Now()
	defer e.cancel()
	defer close(e.done)
	defer func() {
		if r := recover(); r != nil {
			s.log.Error("Torrent search panicked", "key", e.key, "panic", r)
			e.finish(fmt.Errorf("the search failed unexpectedly: %v", r))
		}
	}()

	list, err := s.jackett.Indexers(e.ctx)
	if err != nil {
		s.log.Warn("Torrent search can't list Jackett's indexers", "error", s.jackett.SafeError(err))
		e.finish(fmt.Errorf("%w: %s", ErrJackettUnavailable, s.jackett.SafeError(err)))
		return
	}
	in := newIngester(e.tc, e.plan, s.now())
	e.mu.Lock()
	e.showSeasons = in.showSeasons
	e.mu.Unlock()

	s.pass(e, in, e.plan.Queries, list)
	if e.inScope() == 0 && len(e.plan.Fallback) > 0 && e.ctx.Err() == nil {
		e.mu.Lock()
		for _, q := range e.plan.Fallback {
			e.queries = append(e.queries, sentQuery(q))
		}
		e.mu.Unlock()
		s.pass(e, in, e.plan.Fallback, list)
	}
	if errors.Is(e.ctx.Err(), context.DeadlineExceeded) {
		e.warn(fmt.Sprintf("The search stopped after %s; indexers still searching were left out.", roundSeconds(s.runTimeout)))
	}
	e.finish(nil)

	ok, failed := e.indexerCounts()
	s.log.Info("Torrent search finished", "tmdb_id", e.tc.TMDBID, "media_type", e.tc.MediaType,
		"scope", e.plan.Scope, "releases", e.inScope(), "indexers_ok", ok, "indexers_failed", failed,
		"ms", time.Since(start).Milliseconds())
}

// pass runs queries on every indexer and ingests results as they arrive.
func (s *Service) pass(e *entry, in *ingester, queries []Query, list []torznab.Indexer) {
	var reqs []torznab.Request
	var queryOf []int
	for qi, q := range queries {
		tq := torznabQuery(q, e.tc)
		for _, ix := range list {
			reqs = append(reqs, torznab.Request{IndexerID: ix.ID, Query: tq})
			queryOf = append(queryOf, qi)
		}
	}
	if len(reqs) == 0 {
		return
	}
	total := min(max(perIndexer*len(list), 6), maxTotal)
	s.jackett.Fanout(e.ctx, reqs, torznab.FanoutOptions{
		PerIndexer: perIndexer,
		Total:      total,
		OnProgress: func(p torznab.Progress) {
			var kept []*item
			if p.RequestIndex >= 0 {
				q := queries[queryOf[p.RequestIndex]]
				for _, r := range p.Results {
					if it, ok := in.ingest(r, q); ok {
						kept = append(kept, it)
						s.index.put(it.id, indexedOf(r, it), s.now())
					}
				}
			}
			for id, magnet := range e.progress(p, kept, s.now()) {
				s.index.setMagnet(id, magnet)
			}
		},
	})
}

// torznabQuery is a plan query as a Torznab request, asking for the
// title's categories only.
func torznabQuery(q Query, tc *service.TitleContext) torznab.Query {
	p := q.Params
	tq := torznab.Query{Q: p.Q, IMDbID: p.IMDbID, TVDBID: p.TVDBID, TMDBID: p.TMDBID, Limit: requestLimit}
	switch p.Type {
	case TypeTVSearch:
		tq.Mode = torznab.ModeTV
	case TypeMovie:
		tq.Mode = torznab.ModeMovie
	default:
		tq.Mode = torznab.ModeSearch
	}
	if p.Season != nil {
		tq.Season = strconv.Itoa(*p.Season)
	}
	if p.Ep != nil {
		tq.Episode = strconv.Itoa(*p.Ep)
	}
	switch {
	case tc.MediaType == library.JobTV:
		tq.Categories = []int{torznab.CategoryTV, torznab.CategoryAnime}
	case tc.IsAnime: // anime films are often filed under anime
		tq.Categories = []int{torznab.CategoryMovies, torznab.CategoryAnime}
	default:
		tq.Categories = []int{torznab.CategoryMovies}
	}
	return tq
}

// progress applies a fan-out report and the releases it brought. It
// returns the magnets of the releases that absorbed a duplicate, by id.
func (e *entry) progress(p torznab.Progress, kept []*item, now time.Time) (adopted map[string]string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	st := e.indexers[p.IndexerID]
	if st == nil {
		st = &indexerState{name: p.IndexerName}
		e.indexers[p.IndexerID] = st
		e.order = append(e.order, p.IndexerID)
	}
	if p.RequestIndex < 0 {
		st.startPass()
		return nil
	}
	st.update(p, now)
	for _, it := range kept {
		if id, magnet := e.add(it); id != "" {
			if adopted == nil {
				adopted = map[string]string{}
			}
			adopted[id] = magnet
		}
	}
	return adopted
}

// sizeTolerance: copies of a release on two indexers may differ this much
// in size, since indexers round sizes ("1.4 GiB").
const sizeTolerance = 0.05

// add merges a release into the results. A duplicate has the same info
// hash or, failing that, the same normalised title and a size within
// sizeTolerance, and never another info hash; the copy with the most
// seeders stays, or the one with a magnet on a tie. After a merge it
// returns the id of the copy that stayed and its magnet (maybe the
// other's), so the release index can match.
func (e *entry) add(it *item) (adoptedID, magnet string) {
	var cur *item
	if it.infoHash != "" {
		cur = e.byHash[it.infoHash]
	}
	if cur == nil {
		cur = e.sameRelease(it)
	}
	if cur == nil {
		e.items = append(e.items, it)
		cur = it
	} else {
		*cur = merge(*cur, *it)
		if cur.magnet != "" {
			adoptedID, magnet = cur.id, cur.magnet
		}
	}
	if cur.infoHash != "" {
		e.byHash[cur.infoHash] = cur
	}
	for _, name := range []string{normName(it.title), normName(cur.title)} {
		if name != "" && !slices.Contains(e.byName[name], cur) {
			e.byName[name] = append(e.byName[name], cur)
		}
	}
	return adoptedID, magnet
}

// sameRelease is the release it duplicates by name: the same normalised
// title, the closest size within sizeTolerance, and no other info hash.
// Nil when there's none, or a size is unknown.
func (e *entry) sameRelease(it *item) *item {
	if it.size <= 0 {
		return nil
	}
	var best *item
	var bestDiff int64
	for _, c := range e.byName[normName(it.title)] {
		if c.size <= 0 || c.infoHash != "" && it.infoHash != "" && c.infoHash != it.infoHash {
			continue
		}
		diff := c.size - it.size
		if diff < 0 {
			diff = -diff
		}
		if float64(diff) > sizeTolerance*float64(max(c.size, it.size)) {
			continue
		}
		if best == nil || diff < bestDiff {
			best, bestDiff = c, diff
		}
	}
	return best
}

// merge picks between two copies of a release. The winner takes the
// other's magnet when it has none and they're the same torrent.
func merge(a, b item) item {
	win, lose := a, b
	if b.seeders > a.seeders || b.seeders == a.seeders && b.magnet != "" && a.magnet == "" {
		win, lose = b, a
	}
	if win.magnet == "" && lose.magnet != "" && (win.infoHash == "" || win.infoHash == lose.infoHash) {
		win.magnet, win.infoHash = lose.magnet, lose.infoHash
	}
	return win
}

func (e *entry) inScope() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.items)
}

func (e *entry) warn(msg string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.warnings = append(e.warnings, msg)
}

// finish marks the search complete; err fails it.
func (e *entry) finish(err error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.complete = true
	if err != nil && e.err == nil {
		e.err = err
	}
}

func (e *entry) indexerCounts() (ok, failed int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, st := range e.indexers {
		switch st.status {
		case torznab.StatusOK:
			ok++
		case torznab.StatusTimeout, torznab.StatusError:
			failed++
		}
	}
	return ok, failed
}

// snapshot is a search's state at one moment, for a read.
type snapshot struct {
	plan        *QueryPlan
	mediaType   library.JobMediaType
	startedAt   time.Time
	complete    bool
	err         error
	queries     []SentQuery
	indexers    []IndexerReport
	items       []item
	showSeasons int
	warnings    []string
}

func (e *entry) snapshot(now time.Time) snapshot {
	e.mu.Lock()
	defer e.mu.Unlock()
	snap := snapshot{
		plan:        e.plan,
		mediaType:   e.tc.MediaType,
		startedAt:   e.startedAt,
		complete:    e.complete,
		err:         e.err,
		queries:     append([]SentQuery(nil), e.queries...),
		indexers:    make([]IndexerReport, 0, len(e.order)),
		items:       make([]item, len(e.items)),
		showSeasons: e.showSeasons,
		warnings:    append([]string(nil), e.warnings...),
	}
	for _, id := range e.order {
		st := e.indexers[id]
		r := IndexerReport{Name: st.name, Status: st.status, Count: st.count, Error: st.err}
		if st.status == torznab.StatusPending {
			r.MS = now.Sub(e.startedAt).Milliseconds()
		} else {
			r.MS = st.doneAt.Sub(e.startedAt).Milliseconds()
		}
		r.MS = max(r.MS, 0)
		snap.indexers = append(snap.indexers, r)
	}
	for i, it := range e.items {
		snap.items[i] = *it
	}
	return snap
}

func roundSeconds(d time.Duration) string {
	return strconv.Itoa(int(d.Round(time.Second)/time.Second)) + " s"
}
