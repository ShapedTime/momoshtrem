package downloads

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/shapedtime/momoshtrem/internal/library"
)

// Smart downloads (#77 P5 #203, PLAN §6 "Downloads (#77)" › "Smart
// picks"): the next smart.episodes_ahead episodes, with torrent files, of
// the shows being watched, fetched before they're played. Two signals say
// what's being watched:
//   - Trakt's Up next shows watched in the last UpNextWithin: K episodes
//     from the one it names next, that one included (up_next);
//   - the episodes streaming now: K episodes after each (streaming). This
//     works without Trakt.
//
// K counts the episodes the re-pick rules allow (NextEpisodesWithFiles).
// The SmartPicker makes the picks through Service.UpsertSmart and releases
// those that stop being candidates as expired, never deleted (that's a
// dismissal, never picked again). The planner releases watched picks
// (watched, or a later episode streaming) and expired ones, and admits
// picks against the smart cap. While smart.enabled is off the picker does
// nothing at all.
//
// Order: picks are admitted by creation, then episode (no score column).
// A pass makes its picks in the order of its signals: the episodes
// streaming now first, then Up next by last watched, newest first, each
// in episode order. A pick made earlier keeps its room, except that a
// stream's pick that doesn't fit takes it from Up next's picks of other
// shows (victimsFor); the stale ones leave on their own (an Up next show
// after UpNextWithin without a play, a streaming pick within its
// SmartLifetime).

// The smart picks' rules.
const (
	// SmartInterval is how often the smart picks are reconsidered without
	// a nudge (a Trakt sync, a storage change, an episode starting to
	// stream).
	SmartInterval = 5 * time.Minute
	// UpNextWithin is how recently an Up next show must have been watched
	// for its next episodes to be picked.
	UpNextWithin = 21 * 24 * time.Hour
	// SmartKeepRecent spares a pick all on disk of a show watched within
	// it when a stream's pick needs room.
	SmartKeepRecent = 7 * 24 * time.Hour
)

// UpNextShow is a library show Trakt's Up next lists, with the library
// episode it names next.
type UpNextShow struct {
	ShowID        int64
	SeasonNumber  int
	EpisodeNumber int
	// LastWatchedAt is the show's last Trakt play; nil for a watchlist
	// start, which isn't being watched.
	LastWatchedAt *time.Time
}

// UpNext lists Up next's library shows (me.Service.UpNextEpisodes, which
// main adapts): empty without Trakt.
type UpNext interface {
	UpNextShows(ctx context.Context) ([]UpNextShow, error)
}

// SmartStore is what the smart picks read and release
// (*library.DownloadsRepository).
type SmartStore interface {
	NextEpisodesWithFiles(ctx context.Context, showID int64, season, episode, k int) ([]library.NextEpisodeFile, error)
	ActiveSmartPicks(ctx context.Context) ([]library.SmartPick, error)
	ReleaseSmartPicks(ctx context.Context, releases []library.DownloadRelease, at time.Time) ([]int64, error)
}

var _ SmartStore = (*library.DownloadsRepository)(nil)

// ---- The rules ----------------------------------------------------------------

// smartAnchor is where a show's next episodes start: from, that one
// included.
type smartAnchor struct {
	show   int64
	from   episodeNumber
	source library.SmartSource
}

// smartAnchors are the signals of the shows being watched, in the order
// their picks are made: each episode streaming now (by show, then
// episode), counting from the one after it; then each Up next show
// watched within UpNextWithin, the most recently watched first, counting
// from the episode it names. Specials anchor nothing.
func smartAnchors(upNext []UpNextShow, streaming map[int64][]episodeNumber, now time.Time) []smartAnchor {
	var out []smartAnchor
	for _, show := range slices.Sorted(maps.Keys(streaming)) {
		eps := slices.SortedFunc(slices.Values(streaming[show]), compareEpisodes)
		for _, ep := range slices.Compact(eps) {
			if ep.season > 0 {
				out = append(out, smartAnchor{show, episodeNumber{ep.season, ep.episode + 1}, library.SmartStreaming})
			}
		}
	}
	var recent []UpNextShow
	for _, u := range upNext {
		if u.LastWatchedAt != nil && now.Sub(*u.LastWatchedAt) <= UpNextWithin && u.SeasonNumber > 0 {
			recent = append(recent, u)
		}
	}
	slices.SortStableFunc(recent, func(a, b UpNextShow) int {
		return cmp.Or(b.LastWatchedAt.Compare(*a.LastWatchedAt), cmp.Compare(a.ShowID, b.ShowID))
	})
	for _, u := range recent {
		out = append(out, smartAnchor{u.ShowID, episodeNumber{u.SeasonNumber, u.EpisodeNumber}, library.SmartUpNext})
	}
	return out
}

func compareEpisodes(a, b episodeNumber) int {
	return cmp.Or(cmp.Compare(a.season, b.season), cmp.Compare(a.episode, b.episode))
}

// streamsAfter says an episode of the show after n streams now.
func streamsAfter(streaming []episodeNumber, n episodeNumber) bool {
	return slices.ContainsFunc(streaming, func(s episodeNumber) bool { return s.after(n) })
}

// smartInput is what a smart pass decides from.
type smartInput struct {
	// k is smart.episodes_ahead.
	k       int
	anchors []smartAnchor
	// windows are each anchor's episodes (NextEpisodesWithFiles): the
	// first k from it that the re-pick rules allow and that have a torrent
	// file.
	windows [][]library.NextEpisodeFile
	// picks are the active smart picks; streaming the episodes streaming
	// now, per show.
	picks     []library.SmartPick
	streaming map[int64][]episodeNumber
}

// smartCreate is a pick to make: the episode, its show, why, and its
// file.
type smartCreate struct {
	episodeID int64
	show      int64
	source    library.SmartSource
	file      FileKey
	size      int64
}

// smartPlan is what a smart pass does: the picks to make, in order, and
// those to release.
type smartPlan struct {
	creates  []smartCreate
	releases []library.DownloadRelease
}

// planSmart decides a pass. The candidates are the windows' episodes, the
// first anchor that names one giving its source, but those before an
// episode of their show streaming now (the planner would release them as
// watched). An active pick:
//   - watched before it was made (a play that synced late) is released
//     watched: the planner only counts plays after a download's creation;
//   - watched since, or before an episode of its show streaming now, is
//     the planner's to release (watched);
//   - a candidate, or streaming now, stays;
//   - else it's released expired when its show streams now (a window
//     says where the show is), when Up next picked it (the show left Up
//     next's UpNextWithin, or Trakt is off), or when it has no torrent
//     file any more;
//   - else it's a streaming pick whose stream ended (or paused): it stays
//     until it's watched or its SmartLifetime ends, the first k+1 of its
//     show's by episode (the one that streamed and the k after it; k may
//     have been lowered).
//
// The picks to make are the candidates without an active pick, in order.
func planSmart(in smartInput) smartPlan {
	candidate := map[int64]bool{}
	var order []smartCreate
	streams := map[int64]bool{}
	for i, a := range in.anchors {
		if a.source == library.SmartStreaming {
			streams[a.show] = true
		}
		window := in.windows[i]
		for _, ep := range window[:min(len(window), in.k)] {
			if candidate[ep.EpisodeID] || streamsAfter(in.streaming[a.show], episodeNumber{ep.SeasonNumber, ep.EpisodeNumber}) {
				continue
			}
			candidate[ep.EpisodeID] = true
			order = append(order, smartCreate{episodeID: ep.EpisodeID, show: a.show, source: a.source,
				file: FileKey{Hash: ep.InfoHash, Path: ep.FilePath}, size: ep.FileSize})
		}
	}

	var plan smartPlan
	release := func(p library.SmartPick, reason library.ReleaseReason) {
		plan.releases = append(plan.releases, library.DownloadRelease{ID: p.ID, Reason: reason})
	}
	picked := map[int64]bool{}
	held := map[int64][]library.SmartPick{}
	for _, p := range in.picks {
		picked[p.EpisodeID] = true
		n := episodeNumber{p.SeasonNumber, p.EpisodeNumber}
		streaming := in.streaming[p.ShowID]
		switch {
		case p.WatchedAt != nil && !p.WatchedAt.After(p.CreatedAt):
			release(p, library.ReleaseWatched)
		case p.WatchedAt != nil, streamsAfter(streaming, n):
		case candidate[p.EpisodeID], slices.Contains(streaming, n):
		case streams[p.ShowID], p.Source == library.SmartUpNext, !p.HasFile:
			release(p, library.ReleaseExpired)
		default:
			held[p.ShowID] = append(held[p.ShowID], p)
		}
	}
	for _, show := range slices.Sorted(maps.Keys(held)) {
		ps := held[show]
		slices.SortFunc(ps, func(a, b library.SmartPick) int {
			return compareEpisodes(episodeNumber{a.SeasonNumber, a.EpisodeNumber}, episodeNumber{b.SeasonNumber, b.EpisodeNumber})
		})
		for _, p := range ps[min(len(ps), in.k+1):] {
			release(p, library.ReleaseExpired)
		}
	}
	for _, c := range order {
		if !picked[c.episodeID] {
			plan.creates = append(plan.creates, c)
		}
	}
	return plan
}

// smartRoom is what the latest pass leaves new picks.
type smartRoom struct {
	// known is false before the first pass: the planner alone decides.
	known bool
	// left is what the smart cap has left after the admitted smart files.
	left int64
	// has says a download has the file admitted already: it costs nothing.
	has func(FileKey) bool
	// kept says a Keep holds the episode: it isn't picked.
	kept func(episodeID int64) bool
}

// roomOf is the room the snapshot leaves.
func roomOf(s *Snapshot) smartRoom {
	if s == nil {
		return smartRoom{has: func(FileKey) bool { return false }, kept: func(int64) bool { return false }}
	}
	return smartRoom{
		known: true,
		left:  s.smartLeft,
		has:   func(key FileKey) bool { _, ok := s.files[key]; return ok },
		kept: func(id int64) bool {
			k, ok := s.episodes[id]
			return ok && k.Reason == library.DownloadKept
		},
	}
}

// cost is what c takes of the room: nothing for a file a download has, or
// an earlier pick of the pass (taken), or before the first pass.
func (r smartRoom) cost(c smartCreate, taken map[FileKey]bool) int64 {
	if !r.known || taken[c.file] || r.has(c.file) {
		return 0
	}
	return c.size
}

// smartEvict makes room for c, a stream's pick short of need bytes, and
// answers the room after; false when it released nothing.
type smartEvict func(c smartCreate, need int64) (smartRoom, bool, error)

// fitSmart makes the picks that can fit, in order, through pick (false:
// not made, as the re-pick rules refuse it, so it takes no room). An
// episode a Keep holds isn't picked; a file a download has, or an earlier
// pick of the pass, costs nothing. A stream's pick that doesn't fit what
// the smart cap has left first has evict make room; one that still
// doesn't is skipped (it would only read storage_full; Smart never takes
// kept room), and a later, smaller one may still fit. skipped counts
// those.
func fitSmart(creates []smartCreate, room smartRoom, pick func(smartCreate) (bool, error), evict smartEvict) (skipped int, err error) {
	taken := map[FileKey]bool{}
	for _, c := range creates {
		if room.kept(c.episodeID) {
			continue
		}
		cost := room.cost(c, taken)
		if cost > room.left && c.source == library.SmartStreaming && evict != nil {
			after, ok, err := evict(c, cost-room.left)
			if err != nil {
				return skipped, err
			}
			if ok {
				room = after
				cost = room.cost(c, taken)
			}
		}
		if cost > room.left {
			skipped++
			continue
		}
		made, err := pick(c)
		if err != nil {
			return skipped, err
		}
		if made {
			room.left -= cost
			taken[c.file] = true
		}
	}
	return skipped, nil
}

// smartVictim is an Up next pick that may give its room to a stream's.
type smartVictim struct {
	id   int64
	show int64
	// bytes are its admitted files, as the last pass planned them; done
	// says they're all on disk.
	bytes int64
	done  bool
	// watched is its show's last Trakt play; nil when unknown.
	watched *time.Time
}

// victimsFor chooses the Up next picks of shows other than show to
// release, as expired, so a stream's pick short of need bytes fits: the
// least recently watched shows' first (unknown first; a show's newest
// picks first). It never chooses a pick all on disk of a show watched
// within SmartKeepRecent. Nil when they can't free that much.
func victimsFor(need int64, show int64, pool []smartVictim, now time.Time) []smartVictim {
	var eligible []smartVictim
	for _, v := range pool {
		recent := v.watched != nil && now.Sub(*v.watched) < SmartKeepRecent
		if v.show != show && v.bytes > 0 && !(v.done && recent) {
			eligible = append(eligible, v)
		}
	}
	slices.SortStableFunc(eligible, func(a, b smartVictim) int {
		switch {
		case a.watched == nil && b.watched != nil:
			return -1
		case a.watched != nil && b.watched == nil:
			return 1
		case a.watched != nil:
			if c := a.watched.Compare(*b.watched); c != 0 {
				return c
			}
		}
		return cmp.Or(cmp.Compare(a.show, b.show), cmp.Compare(b.id, a.id))
	})
	var out []smartVictim
	var freed int64
	for _, v := range eligible {
		if freed >= need {
			break
		}
		out = append(out, v)
		freed += v.bytes
	}
	if freed < need {
		return nil
	}
	return out
}

// victimPool is the Up next picks that may give their room to a stream's:
// the active ones not released this pass, as the snapshot plans them,
// with their shows' last plays.
func victimPool(picks []library.SmartPick, released map[int64]bool, upNext []UpNextShow, s *Snapshot) []smartVictim {
	watched := map[int64]*time.Time{}
	for _, u := range upNext {
		watched[u.ShowID] = u.LastWatchedAt
	}
	var pool []smartVictim
	for _, p := range picks {
		if p.Source != library.SmartUpNext || released[p.ID] {
			continue
		}
		if d, ok := s.Download(p.ID); ok {
			pool = append(pool, smartVictim{id: p.ID, show: p.ShowID, bytes: d.WantedBytes, done: d.State == StateDone,
				watched: watched[p.ShowID]})
		}
	}
	return pool
}

// demoteWatchedSince demotes the files of the downloads released as
// watched since t, but those the last pass still pins.
func (s *Service) demoteWatchedSince(ctx context.Context, t time.Time) {
	s.passMu.Lock()
	defer s.passMu.Unlock()
	s.demoteReleasedWatched(ctx, t, s.refs)
}

// StorageChanged is the storage governor's OnChange: a storage PUT wakes
// the manager, and nudges the smart picks (smart.enabled, episodes_ahead
// and the caps).
func (s *Service) StorageChanged() {
	s.Wake()
	if fn := s.onStorage.Load(); fn != nil {
		(*fn)()
	}
}

// ---- The picker ---------------------------------------------------------------

// SmartDeps are what a SmartPicker needs besides the Service.
type SmartDeps struct {
	Store SmartStore
	// UpNext is nil without Up next: the streaming signal alone picks.
	UpNext UpNext
}

// SmartPicker makes and ends the smart picks: a pass every SmartInterval,
// on Nudge (after a Trakt sync), after a storage change (the Service's
// StorageChanged), and when an episode starts streaming (the manager's
// session reads tell it). One pass runs at a time.
type SmartPicker struct {
	svc   *Service
	d     SmartDeps
	log   *slog.Logger
	nudge chan struct{}

	mu sync.Mutex
	// streaming are the episodes streaming now, as the manager's last
	// session read saw them.
	streaming map[int64][]episodeNumber

	passMu sync.Mutex

	runMu  sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
}

// NewSmartPicker creates the smart picks over the Service, and has its
// session reads and storage changes tell it; Start runs it.
func NewSmartPicker(svc *Service, d SmartDeps) *SmartPicker {
	p := &SmartPicker{svc: svc, d: d, log: slog.With("component", "downloads", "part", "smart"), nudge: make(chan struct{}, 1)}
	observe, nudge := p.observe, p.Nudge
	svc.onStreams.Store(&observe)
	svc.onStorage.Store(&nudge)
	return p
}

// observe keeps the episodes streaming now (eps, which the caller never
// changes again) and nudges a pass when one starts. It runs within the
// manager's pass, so it never waits.
func (p *SmartPicker) observe(eps map[int64][]episodeNumber) {
	p.mu.Lock()
	started := false
	for show, list := range eps {
		for _, ep := range list {
			if ep.season > 0 && !slices.Contains(p.streaming[show], ep) {
				started = true
			}
		}
	}
	p.streaming = eps
	p.mu.Unlock()
	if started {
		p.Nudge()
	}
}

func (p *SmartPicker) streamingNow() map[int64][]episodeNumber {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.streaming
}

// Nudge runs a pass soon: Up next or the storage settings changed.
func (p *SmartPicker) Nudge() {
	select {
	case p.nudge <- struct{}{}:
	default:
	}
}

// Start runs the loop: a pass now, then every SmartInterval and on Nudge,
// until Close.
func (p *SmartPicker) Start() {
	p.runMu.Lock()
	defer p.runMu.Unlock()
	if p.cancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	p.cancel, p.done = cancel, make(chan struct{})
	go func() {
		defer close(p.done)
		p.loop(ctx)
	}()
}

// Close stops the loop, waiting for a pass in flight.
func (p *SmartPicker) Close() {
	p.runMu.Lock()
	cancel, done := p.cancel, p.done
	p.runMu.Unlock()
	if cancel == nil {
		return
	}
	cancel()
	<-done
}

func (p *SmartPicker) loop(ctx context.Context) {
	p.runPass(ctx)
	tick := p.svc.clock.After(SmartInterval)
	for {
		select {
		case <-ctx.Done():
			return
		case <-p.nudge:
			p.runPass(ctx)
		case <-tick:
			p.runPass(ctx)
			tick = p.svc.clock.After(SmartInterval)
		}
	}
}

func (p *SmartPicker) runPass(ctx context.Context) {
	if err := p.Run(ctx); err != nil && ctx.Err() == nil {
		p.log.Warn("Smart picks pass failed; the next one tries again", "error", err)
	}
}

// Run is a smart pass, now: while smart.enabled is on, it reads the
// signals, releases the picks that stopped being candidates and makes the
// new ones that fit. A failed read changes nothing.
func (p *SmartPicker) Run(ctx context.Context) error {
	p.passMu.Lock()
	defer p.passMu.Unlock()
	settings, _, err := p.svc.d.Limits.Limits(ctx)
	if err != nil {
		return fmt.Errorf("failed to read the storage settings: %w", err)
	}
	if !settings.Smart.Enabled {
		return nil
	}
	k := min(max(settings.Smart.EpisodesAhead, library.MinEpisodesAhead), library.MaxEpisodesAhead)
	var upNext []UpNextShow
	if p.d.UpNext != nil {
		if upNext, err = p.d.UpNext.UpNextShows(ctx); err != nil {
			return fmt.Errorf("failed to read Up next: %w", err)
		}
	}
	now := p.svc.clock.Now()
	in := smartInput{k: k, streaming: p.streamingNow()}
	in.anchors = smartAnchors(upNext, in.streaming, now)
	for _, a := range in.anchors {
		window, err := p.d.Store.NextEpisodesWithFiles(ctx, a.show, a.from.season, a.from.episode, k)
		if err != nil {
			return err
		}
		in.windows = append(in.windows, window)
	}
	if in.picks, err = p.d.Store.ActiveSmartPicks(ctx); err != nil {
		return err
	}
	plan := planSmart(in)

	released, err := p.release(ctx, plan.releases, now, "Smart pick released")
	if err != nil {
		return err
	}
	// A pass unpins what was released and frees its room; one also runs
	// when the last pass's room didn't count the picks (smart downloads
	// were off). Then the watched ones' pieces go first when the cache
	// trims.
	snap := p.svc.Snapshot()
	if len(released) > 0 || (len(plan.creates) > 0 && (snap == nil || !snap.smartOn)) {
		if err := p.svc.Refresh(ctx); err != nil {
			return err
		}
		snap = p.svc.Snapshot()
	}
	for _, rel := range plan.releases {
		if released[rel.ID] && rel.Reason == library.ReleaseWatched {
			p.svc.demoteWatchedSince(ctx, now)
			break
		}
	}
	if len(plan.creates) == 0 {
		return nil
	}

	pool := victimPool(in.picks, released, upNext, snap)
	evict := func(c smartCreate, need int64) (smartRoom, bool, error) {
		victims := victimsFor(need, c.show, pool, now)
		if victims == nil {
			return smartRoom{}, false, nil
		}
		rels := make([]library.DownloadRelease, len(victims))
		for i, v := range victims {
			rels[i] = library.DownloadRelease{ID: v.id, Reason: library.ReleaseExpired}
		}
		pool = slices.DeleteFunc(pool, func(v smartVictim) bool { return slices.Contains(victims, v) })
		freed, err := p.release(ctx, rels, now, "Smart pick released: a stream's pick needs its room")
		if err != nil || len(freed) == 0 {
			return smartRoom{}, false, err
		}
		if err := p.svc.Refresh(ctx); err != nil {
			return smartRoom{}, false, err
		}
		return roomOf(p.svc.Snapshot()), true, nil
	}
	skipped, err := fitSmart(plan.creates, roomOf(snap), func(c smartCreate) (bool, error) {
		_, _, err := p.svc.UpsertSmart(ctx, c.episodeID, c.source)
		switch {
		case errors.Is(err, ErrRepickBlocked):
			p.log.Debug("Not picked again", "episode_id", c.episodeID, "why", err)
			return false, nil
		case errors.Is(err, library.ErrEpisodeNotFound):
			return false, nil // removed meanwhile
		}
		return err == nil, err
	}, evict)
	if skipped > 0 {
		p.log.Debug("Smart picks skipped: the smart cap has no room for them", "skipped", skipped)
	}
	return err
}

// release releases smart picks, holding the manager's pass lock as
// UpsertSmart does, so a Keep that read one either turns it kept first
// (it's skipped) or finds it released. It answers the ids it released.
func (p *SmartPicker) release(ctx context.Context, rels []library.DownloadRelease, at time.Time, msg string) (map[int64]bool, error) {
	if len(rels) == 0 {
		return nil, nil
	}
	p.svc.passMu.Lock()
	ids, err := p.d.Store.ReleaseSmartPicks(ctx, rels, at)
	p.svc.passMu.Unlock()
	if err != nil {
		return nil, err
	}
	out := make(map[int64]bool, len(ids))
	for _, id := range ids {
		out[id] = true
	}
	for _, rel := range rels {
		if out[rel.ID] {
			p.log.Info(msg, "download_id", rel.ID, "reason", rel.Reason)
		}
	}
	return out, nil
}
