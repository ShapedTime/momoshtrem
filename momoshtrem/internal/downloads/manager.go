// Package downloads keeps kept and smart downloads on disk (#77 P4 #202,
// PLAN §6 "Downloads (#77)"): a Keep or a smart pick on a movie, a show, a
// season or an episode fetches its torrent files in the background and
// pins them in the piece cache while it lasts.
//
// The Service is the download manager and the API's service. A pass,
// every 30 s and whenever something changes (Wake), resolves what each
// active download covers from the active assignments, releases the ones
// that ended (expiry, until watched), admits files against the kept and
// smart caps, pins each torrent's admitted pieces, and gives at most two
// torrents with bytes left a slot: loaded (asynchronously), their admitted
// files at Normal priority, kept awake with MarkActive. The rest wait:
// paused (the user's sticky pause, or background downloads off), for the
// window, for the stream to end (pause while streaming: other torrents
// wait until a minute after the last session), stalled (no progress for
// 10 minutes: the slot rotates, and the torrent is retried with backoff),
// or queued. A session poll every 5 s notices streams starting and ending
// and records local watches (90% of a file).
//
// Each pass builds a Snapshot, which the read paths (P6 #204) read without
// any lock. The planner (planner.go, plan_states.go) is pure; the Service
// does the reading and the applying, through torrent.Service's public API
// (ServiceClient) and the piece cache's.
package downloads

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/anacrolix/torrent/metainfo"

	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/sessions"
	"github.com/shapedtime/momoshtrem/internal/storagecap"
	"github.com/shapedtime/momoshtrem/internal/torrent/piececache"
)

// The loop's intervals.
const (
	// Interval is how often a pass runs without a change.
	Interval = 30 * time.Second
	// PollInterval is how often the sessions are read.
	PollInterval = 5 * time.Second
	// WatchedAt is how much of a file a local session must reach for its
	// items to count as watched here, in percent.
	WatchedAt = 90
	// watchConfirm is how long a session must stay past WatchedAt before
	// it counts: a player reading the index at a file's end isn't a watch.
	watchConfirm = 20 * time.Second
	// loadRetry is the wait after a failed load before the next.
	loadRetry = time.Minute
	// demoteSince is how far back a restart demotes the files of
	// downloads released as watched (the cache's demotion is in memory).
	demoteSince = 30 * 24 * time.Hour
	// pinOwnerPrefix names the cache's pin owners: one per torrent.
	pinOwnerPrefix = "downloads:"
)

// Store is the download store (*library.DownloadsRepository).
type Store interface {
	ResolveDownloads(ctx context.Context) ([]library.ResolvedDownload, error)
	ResolveReleasedWatched(ctx context.Context, since time.Time) ([]library.ResolvedDownload, error)
	ResolveTarget(ctx context.Context, target library.DownloadTarget) (*library.ResolvedDownload, error)
	ReleaseDownloads(ctx context.Context, releases []library.DownloadRelease, at time.Time) ([]int64, error)
	GetDownload(ctx context.Context, id int64) (library.DownloadRecord, error)
	ActiveDownloadFor(ctx context.Context, rec library.DownloadRecord) (*library.DownloadRecord, error)
	CreateDownload(ctx context.Context, rec library.DownloadRecord) (library.DownloadRecord, error)
	KeepSmartDownload(ctx context.Context, id int64, untilWatched bool, expiresAt *time.Time, at time.Time) (library.DownloadRecord, error)
	UpdateDownloadDuration(ctx context.Context, id int64, patch library.DownloadDurationPatch) (library.DownloadRecord, error)
	UpsertSmartDownload(ctx context.Context, episodeID int64, source library.SmartSource, at, expiresAt time.Time) (int64, bool, error)
	SmartPickBlocked(ctx context.Context, episodeID int64) (string, error)
	UpsertLocalWatch(ctx context.Context, item library.ItemRef, at time.Time) error
	FileItems(ctx context.Context, infoHash, filePath string) ([]library.ItemRef, error)
	EpisodeIDByNumber(ctx context.Context, showID int64, season, episode int) (int64, error)
}

var _ Store = (*library.DownloadsRepository)(nil)

// Limits are the storage settings and caps (*storagecap.Governor).
type Limits interface {
	Limits(ctx context.Context) (library.StorageSettings, storagecap.Limits, error)
}

var _ Limits = (*storagecap.Governor)(nil)

// Cache is the piece cache (*piececache.Cache).
type Cache interface {
	Pin(owner string, hexes []string)
	Unpin(owner string)
	HavePieces(pieces []piececache.PieceRef) []bool
	Demote(hexes []string)
	Info() piececache.CacheInfo
}

var _ Cache = (*piececache.Cache)(nil)

// Metainfo reads stored .torrent files (*torrent.MetainfoStore): nil, nil
// when there's none.
type Metainfo interface {
	Load(hash string) (*metainfo.MetaInfo, error)
}

// Sessions are the streaming sessions (*sessions.Registry).
type Sessions interface {
	Sessions(ctx context.Context) []sessions.Session
}

// Clock is the Service's time; tests drive a fake one.
type Clock interface {
	Now() time.Time
	After(d time.Duration) <-chan time.Time
}

type realClock struct{}

func (realClock) Now() time.Time                         { return time.Now() }
func (realClock) After(d time.Duration) <-chan time.Time { return time.After(d) }

// Deps are what a Service needs. Metainfo and Sessions are optional:
// without them piece maps come from loaded torrents only, and nothing
// streams.
type Deps struct {
	Store    Store
	Limits   Limits
	Cache    Cache
	Metainfo Metainfo
	Client   Client
	Sessions Sessions
	// Clock is nil for real time; Location nil for time.Local, the
	// server's TZ (download windows, air dates).
	Clock    Clock
	Location *time.Location
	// OnRestored runs once, after the first pass that pinned: the cache's
	// Ready, then the governor's Wake (startup holds trims until the pins
	// are back).
	OnRestored func()
}

// Service is the download manager and the API's service.
type Service struct {
	d     Deps
	clock Clock
	loc   *time.Location
	log   *slog.Logger
	wake  chan struct{}
	snap  atomic.Pointer[Snapshot]

	// passMu serialises passes, Keeps and storage PUTs (LockCommits); the
	// fields below it are a pass's.
	passMu   sync.Mutex
	maps     map[string]*PieceMap
	misses   map[string]bool // piece maps asked for in vain this pass
	refs     map[string]*pieceRefs
	pinned   map[string]string   // per torrent, the pinned pieces' key
	normal   map[string][]string // per torrent, the files set to Normal
	stalls   map[string]stall
	demoted  map[FileKey]bool
	streams  streamState
	marks    map[string]*watchMark
	holdKey  string
	restored bool

	loadMu     sync.Mutex
	loading    map[string]bool
	loadFailed map[string]time.Time

	runMu  sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
}

// New creates a Service; RestorePins, then Start, run it.
func New(d Deps) *Service {
	s := &Service{
		d: d, clock: d.Clock, loc: d.Location, log: slog.With("component", "downloads"),
		wake: make(chan struct{}, 1),
		maps: map[string]*PieceMap{}, misses: map[string]bool{}, refs: map[string]*pieceRefs{}, pinned: map[string]string{},
		normal: map[string][]string{}, stalls: map[string]stall{}, demoted: map[FileKey]bool{},
		marks: map[string]*watchMark{}, loading: map[string]bool{}, loadFailed: map[string]time.Time{},
	}
	if s.clock == nil {
		s.clock = realClock{}
	}
	if s.loc == nil {
		s.loc = time.Local
	}
	return s
}

// Snapshot is the latest pass's view; nil before the first.
func (s *Service) Snapshot() *Snapshot { return s.snap.Load() }

// Wake runs a pass soon: something changed (a Keep, a removal, an Apply,
// a pause, the storage settings).
func (s *Service) Wake() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// RestorePins runs the first pass at startup, before anything loads a
// torrent: it pins every download's pieces (and loads nothing), so the
// cache's deferred trim spares them, then demotes what was watched. Run it
// after the stored pauses are restored; OnRestored follows a success.
func (s *Service) RestorePins(ctx context.Context) error {
	s.passMu.Lock()
	defer s.passMu.Unlock()
	p, err := s.plan(ctx)
	if err != nil {
		return err
	}
	s.apply(ctx, p, false)
	s.snap.Store(p.snapshot)
	s.log.Info("Download pins restored", "downloads", len(p.stage.covs), "torrents", len(p.refs))

	released, err := s.d.Store.ResolveReleasedWatched(ctx, p.now.Add(-demoteSince))
	if err != nil {
		s.log.Warn("Couldn't read the downloads released as watched; their data isn't evicted first", "error", err)
		return nil
	}
	var covs []*coverage
	in := fileInput{pieces: s.pieceMap}
	for i := range released {
		covs = append(covs, cover(&released[i], in))
	}
	s.demote(covs, true, p.refs)
	return nil
}

// Start runs the loop: a pass every Interval and on Wake, the sessions
// every PollInterval, until Close.
func (s *Service) Start() {
	s.runMu.Lock()
	defer s.runMu.Unlock()
	if s.cancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel, s.done = cancel, make(chan struct{})
	go func() {
		defer close(s.done)
		s.loop(ctx)
	}()
}

// Close stops the loop. Loads in flight finish on their own.
func (s *Service) Close() {
	s.runMu.Lock()
	cancel, done := s.cancel, s.done
	s.runMu.Unlock()
	if cancel == nil {
		return
	}
	cancel()
	<-done
}

func (s *Service) loop(ctx context.Context) {
	s.runPass(ctx)
	tick, poll := s.clock.After(Interval), s.clock.After(PollInterval)
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.wake:
			s.runPass(ctx)
		case <-tick:
			s.runPass(ctx)
			tick = s.clock.After(Interval)
		case <-poll:
			if s.poll(ctx) {
				s.runPass(ctx)
			}
			poll = s.clock.After(PollInterval)
		}
	}
}

func (s *Service) runPass(ctx context.Context) {
	if err := s.Refresh(ctx); err != nil && ctx.Err() == nil {
		s.log.Warn("Downloads pass failed; the next one tries again", "error", err)
	}
}

// Refresh runs a pass now and waits for it.
func (s *Service) Refresh(ctx context.Context) error {
	s.passMu.Lock()
	defer s.passMu.Unlock()
	return s.passLocked(ctx, true)
}

// poll reads the sessions; it reports whether a pass should follow (a
// stream started or ended, or a local watch was recorded).
func (s *Service) poll(ctx context.Context) bool {
	s.passMu.Lock()
	defer s.passMu.Unlock()
	now := s.clock.Now()
	recorded := s.observeSessions(ctx, now)
	return recorded || s.streams.key(now) != s.holdKey
}

// ---- A pass ---------------------------------------------------------------------

// passPlan is everything a pass decided.
type passPlan struct {
	now      time.Time
	settings library.StorageSettings
	limits   storagecap.Limits
	live     map[string]int64 // loaded torrents' speeds
	stage    fileStage
	wants    map[string]*torrentWant
	facts    map[string]*torrentFacts
	refs     map[string]*pieceRefs
	have     map[string][]bool
	torrents map[string]*torrentPlan
	snapshot *Snapshot
}

// pieceRefs are a torrent's admitted pieces, kept while its admitted files
// stay the same.
type pieceRefs struct {
	key   string // the admitted paths
	index []int  // the pieces, in order
	refs  []piececache.PieceRef
	hexes []string
	spans map[string]FileSpan
}

func (s *Service) passLocked(ctx context.Context, loads bool) error {
	p, err := s.plan(ctx)
	if err != nil {
		return err
	}
	s.apply(ctx, p, loads)
	s.snap.Store(p.snapshot)
	return nil
}

// plan reads everything a pass needs and plans it, changing nothing but
// the session state (and local watches) it reads.
func (s *Service) plan(ctx context.Context) (*passPlan, error) {
	now := s.clock.Now()
	clear(s.misses)
	settings, limits, err := s.d.Limits.Limits(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to read the storage limits: %w", err)
	}
	rows, err := s.d.Store.ResolveDownloads(ctx)
	if err != nil {
		return nil, err
	}
	live, err := s.d.Client.Loaded()
	if err != nil {
		s.log.Warn("Couldn't list the loaded torrents", "error", err)
		live = map[string]int64{}
	}
	s.observeSessions(ctx, now)
	p := &passPlan{now: now, settings: settings, limits: limits, live: live,
		facts: map[string]*torrentFacts{}, refs: map[string]*pieceRefs{}, have: map[string][]bool{}}
	p.stage = planFiles(fileInput{
		now: now, loc: s.loc, smart: settings.Smart.Enabled,
		keptMax: limits.KeptMaxBytes, smartMax: limits.SmartMaxBytes,
		downloads: rows, pieces: s.pieceMap, streaming: s.streams.episodes,
	})
	p.wants = torrentWants(p.stage)
	for hash, w := range p.wants {
		p.facts[hash] = s.readTorrent(p, w)
	}

	inside, next := windowNow(settings.Background.Windows, now, s.loc)
	info := s.d.Cache.Info()
	h := holds{
		disabled:   !settings.Background.Enabled,
		diskFull:   limits.Floored && info.Pinned+min(limits.ReserveBytes, limits.EffectiveBytes) >= limits.EffectiveBytes,
		outside:    !inside,
		nextWindow: next,
		stream:     settings.Background.PauseWhileStreaming && s.streams.hold(now),
	}
	p.torrents = planTorrents(p.wants, stateInput{now: now, holds: h, facts: p.facts})
	views := make([]downloadView, 0, len(p.stage.covs))
	for _, cov := range p.stage.covs {
		views = append(views, viewDownload(cov, p.stage.adm, p.torrents, p.facts))
	}
	usage := Usage{CommittedKeptBytes: p.stage.adm.committedKept,
		KeptMaxBytes: limits.KeptMaxBytes, SmartMaxBytes: limits.SmartMaxBytes}
	for key := range p.stage.adm.kept {
		usage.KeptBytes += p.doneOf(key)
	}
	for key := range p.stage.adm.smart {
		usage.SmartBytes += p.doneOf(key)
	}
	reason, resumes := pausedReason(h, s.streams.resumes(now))
	p.snapshot = buildSnapshot(snapshotInput{at: now, stage: p.stage, torrents: p.torrents, facts: p.facts,
		views: views, usage: usage, reason: reason, resumes: resumes})
	return p, nil
}

func (p *passPlan) doneOf(key FileKey) int64 {
	if f := p.facts[key.Hash]; f != nil {
		return min(f.done[key.Path], p.stage.adm.sizes[key])
	}
	return 0
}

// readTorrent reads what the cache and the client say of a torrent with
// admitted files: its pieces on disk (one HavePieces), its pause, speed
// and progress.
func (s *Service) readTorrent(p *passPlan, w *torrentWant) *torrentFacts {
	f := &torrentFacts{done: map[string]int64{}, complete: map[string]bool{}, speed: p.live[w.hash],
		streamed: s.streams.streamed(w.hash, p.now), stall: s.stalls[w.hash]}
	f.holder = !f.stall.slotSince.IsZero()
	_, f.paused = s.d.Client.PausedAt(w.hash)
	if refs := s.pieceRefsFor(w); refs != nil {
		have := s.d.Cache.HavePieces(refs.refs)
		p.refs[w.hash], p.have[w.hash] = refs, have
		pm := s.maps[w.hash]
		at := make(map[int]bool, len(refs.index))
		for k, i := range refs.index {
			at[i] = have[k]
		}
		for path, span := range refs.spans {
			whole := true
			for i := span.Begin; i < span.End; i++ {
				if at[i] {
					f.done[path] += pm.Overlap(span, i)
				} else {
					whole = false
				}
			}
			f.complete[path] = whole
			f.measure += f.done[path]
		}
	}
	if _, loaded := p.live[w.hash]; loaded {
		f.measure += s.d.Client.Progress(w.hash, slices.Sorted(maps.Keys(w.files)))
	}
	return f
}

// pieceRefsFor is the torrent's admitted pieces; nil while its piece map
// is unknown.
func (s *Service) pieceRefsFor(w *torrentWant) *pieceRefs {
	paths := slices.Sorted(maps.Keys(w.files))
	pm := s.pieceMap(w.hash, paths[0])
	if !pm.V1() {
		return nil
	}
	key := strings.Join(paths, "\x00")
	if r := s.refs[w.hash]; r != nil && r.key == key {
		return r
	}
	r := &pieceRefs{key: key, spans: map[string]FileSpan{}}
	seen := map[int]bool{}
	for _, path := range paths {
		span, ok := pm.File(path)
		if !ok {
			continue
		}
		r.spans[path] = span
		for i := span.Begin; i < span.End; i++ {
			seen[i] = true
		}
	}
	r.index = slices.Sorted(maps.Keys(seen))
	for _, i := range r.index {
		ref := pm.Ref(i)
		r.refs = append(r.refs, ref)
		r.hexes = append(r.hexes, ref.Hex)
	}
	s.refs[w.hash] = r
	return r
}

// pieceMap is the torrent's piece map: from its stored metainfo, else from
// the client when it's loaded (found through path, one of its files); nil
// while neither has it, asked once per pass. It's kept once known: an
// info dict never changes under its hash.
func (s *Service) pieceMap(hash, path string) *PieceMap {
	if pm, ok := s.maps[hash]; ok {
		return pm
	}
	if s.misses[hash] {
		return nil
	}
	defer func() {
		if _, ok := s.maps[hash]; !ok {
			s.misses[hash] = true
		}
	}()
	var info *metainfo.Info
	if s.d.Metainfo != nil {
		mi, err := s.d.Metainfo.Load(hash)
		if err != nil {
			s.log.Warn("Couldn't read a torrent's stored metainfo", "info_hash", hash, "error", err)
		}
		if mi != nil {
			if i, err := mi.UnmarshalInfo(); err == nil {
				info = &i
			}
		}
	}
	if info == nil && path != "" {
		info = s.d.Client.Info(hash, path)
	}
	if info == nil {
		return nil
	}
	pm := NewPieceMap(info)
	s.maps[hash] = pm
	return pm
}

// ---- Applying a pass ----------------------------------------------------------

// apply carries a plan out: releases, pins, demotions, priorities, the F11
// re-sync, MarkActive and (with loads) loading the slots' torrents.
func (s *Service) apply(ctx context.Context, p *passPlan, loads bool) {
	if len(p.stage.releases) > 0 {
		ids, err := s.d.Store.ReleaseDownloads(ctx, p.stage.releases, p.now)
		if err != nil {
			s.log.Warn("Couldn't release downloads; the next pass tries again", "error", err)
		}
		for _, rel := range p.stage.releases {
			if slices.Contains(ids, rel.ID) {
				s.log.Info("Download released", "download_id", rel.ID, "reason", rel.Reason)
			}
		}
	}

	// Pins, one owner per torrent.
	for hash, r := range p.refs {
		if s.pinned[hash] == r.key {
			continue
		}
		s.d.Cache.Pin(pinOwnerPrefix+hash, r.hexes)
		s.pinned[hash] = r.key
	}
	for hash := range s.pinned {
		if _, ok := p.refs[hash]; !ok {
			s.d.Cache.Unpin(pinOwnerPrefix + hash)
			delete(s.pinned, hash)
		}
	}
	if !s.restored {
		s.restored = true
		if s.d.OnRestored != nil {
			s.d.OnRestored()
		}
	}

	// Watched files go first when the cache trims.
	s.demote(p.stage.released, true, p.refs)
	s.demote(p.stage.covs, false, p.refs)

	// The torrents: priorities, the F11 re-sync, wakes and loads.
	for hash, t := range p.torrents {
		paths := slices.Sorted(maps.Keys(t.want.files))
		if _, loaded := p.live[hash]; !loaded {
			if loads && t.state == StateDownloading {
				s.load(hash, t.want.magnet, p.now)
			}
			continue
		}
		var normal, none []string
		if t.state == StateDownloading {
			normal = paths
		} else {
			none = paths
		}
		for _, path := range s.normal[hash] {
			if !slices.Contains(normal, path) && !slices.Contains(none, path) {
				none = append(none, path)
			}
		}
		if err := s.d.Client.SetPriorities(hash, normal, none); err != nil {
			s.log.Warn("Couldn't set a torrent's file priorities", "info_hash", hash, "error", err)
		}
		s.normal[hash] = normal
		s.resync(p, hash, paths)
		if t.state == StateDownloading {
			s.d.Client.MarkActive(hash)
		}
	}
	for hash, paths := range s.normal {
		if _, wanted := p.torrents[hash]; wanted {
			continue
		}
		if _, loaded := p.live[hash]; loaded {
			if err := s.d.Client.SetPriorities(hash, nil, paths); err != nil {
				s.log.Warn("Couldn't reset a torrent's file priorities", "info_hash", hash, "error", err)
			}
		}
		delete(s.normal, hash)
	}

	stalls := make(map[string]stall, len(p.torrents))
	for hash, t := range p.torrents {
		stalls[hash] = t.stall
		if t.stall.attempts > s.stalls[hash].attempts {
			s.log.Info("Download stalled; its slot goes to the next torrent", "info_hash", hash,
				"attempt", t.stall.attempts, "retry_at", t.stall.retryAt)
		}
	}
	s.stalls = stalls
	for hash := range s.refs {
		if _, ok := p.wants[hash]; !ok {
			delete(s.refs, hash)
		}
	}
	s.holdKey = s.streams.key(p.now)
}

// resync is streamlab F11's fix: admitted pieces the cache lacks that the
// client believes complete are read again from the storage, so they're
// fetched again.
func (s *Service) resync(p *passPlan, hash string, paths []string) {
	refs, have := p.refs[hash], p.have[hash]
	if refs == nil || !slices.Contains(have, false) || len(paths) == 0 {
		return
	}
	complete, err := s.d.Client.Completion(hash, paths[0])
	if err != nil {
		return
	}
	var lost []int
	for k, i := range refs.index {
		if !have[k] && i < len(complete) && complete[i] {
			lost = append(lost, i)
		}
	}
	if len(lost) == 0 {
		return
	}
	if err := s.d.Client.UpdateCompletion(hash, paths[0], lost); err != nil {
		s.log.Warn("Couldn't re-read lost pieces' completion", "info_hash", hash, "error", err)
		return
	}
	s.log.Info("Re-read the completion of pieces the cache lost", "info_hash", hash, "pieces", len(lost))
}

// demote makes the pieces of watched files the first the cache evicts,
// once per process, unless a download still wants them (wanted, a pass's
// admitted pieces). Those are the files until-watched downloads released
// one by one; with whole, every file of the downloads given (released as
// watched).
func (s *Service) demote(covs []*coverage, whole bool, wanted map[string]*pieceRefs) {
	var hexes []string
	for _, cov := range covs {
		keys := cov.released
		if whole {
			keys = nil
			for _, w := range cov.wants {
				keys = append(keys, w.key)
			}
			keys = append(keys, cov.released...)
		}
		for _, key := range keys {
			if s.demoted[key] {
				continue
			}
			if r := wanted[key.Hash]; r != nil {
				if _, pinned := r.spans[key.Path]; pinned {
					continue
				}
			}
			s.demoted[key] = true
			pm := s.pieceMap(key.Hash, key.Path)
			if !pm.V1() {
				continue
			}
			span, ok := pm.File(key.Path)
			if !ok {
				continue
			}
			for i := span.Begin; i < span.End; i++ {
				hexes = append(hexes, pm.Ref(i).Hex)
			}
		}
	}
	if len(hexes) > 0 {
		s.d.Cache.Demote(hexes)
	}
}

// load adds a slot's torrent in the background: at once from its stored
// metainfo, else within the client's add timeout. A pass follows.
func (s *Service) load(hash, magnet string, now time.Time) {
	s.loadMu.Lock()
	if s.loading[hash] || (!s.loadFailed[hash].IsZero() && now.Before(s.loadFailed[hash].Add(loadRetry))) {
		s.loadMu.Unlock()
		return
	}
	s.loading[hash] = true
	s.loadMu.Unlock()
	go func() {
		err := s.d.Client.Load(magnet)
		s.loadMu.Lock()
		delete(s.loading, hash)
		if err != nil {
			s.loadFailed[hash] = s.clock.Now()
		} else {
			delete(s.loadFailed, hash)
		}
		s.loadMu.Unlock()
		if err != nil {
			s.log.Warn("Couldn't load a torrent to download", "info_hash", hash, "error", err)
		} else {
			s.log.Info("Loaded a torrent to download", "info_hash", hash)
		}
		s.Wake()
	}()
}

// ---- Sessions: streaming and local watches ------------------------------------

// streamState is what the sessions say, kept across reads.
type streamState struct {
	// active: a session streams now; lastSeen is the last read that saw
	// one.
	active   bool
	lastSeen time.Time
	// current are the torrents streaming now, recent when each last did.
	current map[string]bool
	recent  map[string]time.Time
	// episodes are the episodes streaming now, per show.
	episodes map[int64][]episodeNumber
}

// hold says pause while streaming applies: something streams, or did
// within StreamGrace.
func (st *streamState) hold(now time.Time) bool {
	return st.active || (!st.lastSeen.IsZero() && now.Before(st.lastSeen.Add(StreamGrace)))
}

// streamed says the torrent streams, or did within StreamGrace: its own
// downloads carry on.
func (st *streamState) streamed(hash string, now time.Time) bool {
	if st.current[hash] {
		return true
	}
	at, ok := st.recent[hash]
	return ok && now.Before(at.Add(StreamGrace))
}

// resumes is when pause while streaming lets go: nil while one streams.
func (st *streamState) resumes(now time.Time) *time.Time {
	if st.active || !st.hold(now) {
		return nil
	}
	at := st.lastSeen.Add(StreamGrace).UTC()
	return &at
}

// key changes whenever what the sessions hold back does.
func (st *streamState) key(now time.Time) string {
	var held []string
	for hash := range st.current {
		held = append(held, hash)
	}
	for hash, at := range st.recent {
		if !st.current[hash] && now.Before(at.Add(StreamGrace)) {
			held = append(held, hash+"~")
		}
	}
	slices.Sort(held)
	return fmt.Sprint(st.hold(now), held)
}

// watchMark is a session past WatchedAt: since when, and whether it was
// recorded.
type watchMark struct {
	since    time.Time
	recorded bool
}

// observeSessions reads the sessions into the stream state and records
// local watches; it reports whether it recorded one.
func (s *Service) observeSessions(ctx context.Context, now time.Time) bool {
	var list []sessions.Session
	if s.d.Sessions != nil {
		list = s.d.Sessions.Sessions(ctx)
	}
	st := &s.streams
	st.active = len(list) > 0
	if st.active {
		st.lastSeen = now
	}
	st.current, st.episodes = map[string]bool{}, map[int64][]episodeNumber{}
	if st.recent == nil {
		st.recent = map[string]time.Time{}
	}
	for hash, at := range st.recent {
		if !now.Before(at.Add(StreamGrace)) {
			delete(st.recent, hash)
		}
	}
	seen := map[string]bool{}
	recorded := false
	for _, sess := range list {
		seen[sess.ID] = true
		if sess.InfoHash != nil {
			st.current[*sess.InfoHash] = true
			st.recent[*sess.InfoHash] = now
		}
		if it := sess.Item; it != nil && it.MediaType == "tv" && it.SeasonNumber != nil && it.EpisodeNumber != nil {
			st.episodes[it.LibraryID] = append(st.episodes[it.LibraryID], episodeNumber{*it.SeasonNumber, *it.EpisodeNumber})
		}
		if s.markWatch(ctx, sess, now) {
			recorded = true
		}
	}
	for id := range s.marks {
		if !seen[id] {
			delete(s.marks, id)
		}
	}
	return recorded
}

// markWatch follows a session past WatchedAt and records its items as
// watched here once it has stayed there watchConfirm.
func (s *Service) markWatch(ctx context.Context, sess sessions.Session, now time.Time) bool {
	if sess.SizeBytes <= 0 || sess.OffsetBytes*100 < sess.SizeBytes*WatchedAt {
		delete(s.marks, sess.ID)
		return false
	}
	m := s.marks[sess.ID]
	if m == nil {
		s.marks[sess.ID] = &watchMark{since: now}
		return false
	}
	if m.recorded || now.Sub(m.since) < watchConfirm {
		return false
	}
	items, err := s.sessionItems(ctx, sess)
	if err != nil {
		s.log.Warn("Couldn't find what a session plays to record it watched", "session", sess.ID, "error", err)
		return false
	}
	m.recorded = true
	for _, item := range items {
		if err := s.d.Store.UpsertLocalWatch(ctx, item, now); err != nil {
			s.log.Warn("Couldn't record a local watch", "item_type", item.Type, "item_id", item.ID, "error", err)
			continue
		}
		s.log.Info("Watched here", "item_type", item.Type, "item_id", item.ID, "file", sess.FileName)
	}
	return len(items) > 0
}

// sessionItems are the items a session plays: a torrent file's active
// assignments (a grouped file is several episodes), else its item.
func (s *Service) sessionItems(ctx context.Context, sess sessions.Session) ([]library.ItemRef, error) {
	if sess.InfoHash != nil && sess.FilePath != nil {
		return s.d.Store.FileItems(ctx, *sess.InfoHash, *sess.FilePath)
	}
	it := sess.Item
	switch {
	case it == nil:
		return nil, nil
	case it.MediaType == "movie":
		return []library.ItemRef{{Type: library.ItemTypeMovie, ID: it.LibraryID}}, nil
	case it.SeasonNumber == nil || it.EpisodeNumber == nil:
		return nil, nil
	}
	id, err := s.d.Store.EpisodeIDByNumber(ctx, it.LibraryID, *it.SeasonNumber, *it.EpisodeNumber)
	if err != nil || id == 0 {
		return nil, err
	}
	return []library.ItemRef{{Type: library.ItemTypeEpisode, ID: id}}, nil
}
