// Package activity reports the torrents the library knows and what the
// client is doing with them, and pauses and resumes them (PLAN §6
// "Torrents and activity (B6)").
//
// Torrents stream on demand: pieces are fetched only around readers and
// kept in an LRU cache, and a torrent loads on its first WebDAV read. So a
// torrent the library knows but the client hasn't loaded is idle, with no
// progress, and nothing has an ETA.
package activity

import (
	"cmp"
	"context"
	"fmt"
	"log/slog"
	"maps"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/sessions"
	"github.com/shapedtime/momoshtrem/internal/storagecap"
	"github.com/shapedtime/momoshtrem/internal/torrent"
	"github.com/shapedtime/momoshtrem/internal/torrent/piececache"
)

// Torrents is the torrent client. torrent.Service implements it.
type Torrents interface {
	ListTorrents() ([]torrent.TorrentStatus, error)
	GetStatus(infoHash string) (*torrent.TorrentStatus, error)
	// Detail is a loaded torrent's piece map, files and trackers.
	Detail(infoHash string) (*torrent.Detail, error)
	Pause(infoHash string) error
	Resume(infoHash string) error
	Pauses() map[string]time.Time
	RestorePauses(pauses map[string]time.Time)
}

var _ Torrents = (torrent.Service)(nil)

// Sessions are the clients streaming over WebDAV, the most recently read
// first: a loaded torrent with one is streaming. *sessions.Registry
// implements it.
type Sessions interface {
	Sessions(ctx context.Context) []sessions.Session
}

var _ Sessions = (*sessions.Registry)(nil)

// Library is what the library knows about torrents.
// *library.AssignmentJobRepository implements it.
type Library interface {
	KnownTorrents(ctx context.Context, infoHash string) ([]library.KnownTorrent, error)
	// The detail's: a magnet, the active assignments and the newest job's
	// file list.
	TorrentMagnet(ctx context.Context, infoHash string) (string, error)
	ActiveMappings(ctx context.Context, infoHash string) ([]library.FileMapping, error)
	JobFiles(ctx context.Context, infoHash string) ([]library.JobFile, error)
}

var _ Library = (*library.AssignmentJobRepository)(nil)

// Metainfo reads a torrent's stored metainfo without loading it.
// *torrent.MetainfoStore implements it.
type Metainfo interface {
	Summary(infoHash string) (*torrent.StoredInfo, error)
}

var _ Metainfo = (*torrent.MetainfoStore)(nil)

// PauseStore keeps the pauses across restarts. *library.SettingsRepository
// implements it.
type PauseStore interface {
	TorrentPauses(ctx context.Context) (map[string]time.Time, error)
	PauseTorrents(ctx context.Context, pauses map[string]time.Time) error
	ResumeTorrents(ctx context.Context, hashes []string) error
}

var _ PauseStore = (*library.SettingsRepository)(nil)

// Speeds are the total speeds and their recent history.
// *torrent.SpeedSampler implements it.
type Speeds interface {
	Total() torrent.Rate
	History() []torrent.Rate
	Interval() time.Duration
}

var _ Speeds = (*torrent.SpeedSampler)(nil)

// Cache is the piece cache. *piececache.Cache implements it.
type Cache interface {
	Info() piececache.CacheInfo
}

var _ Cache = (*piececache.Cache)(nil)

// Deps are a Service's dependencies. Sessions, Metainfo, Speeds and Cache
// are optional: without them nothing streams, an unloaded torrent's detail
// has only the job's file list, speeds are 0 and the disk is null.
type Deps struct {
	Torrents Torrents
	Sessions Sessions
	Library  Library
	Pauses   PauseStore
	Metainfo Metainfo
	Speeds   Speeds
	Cache    Cache
	// DiskPath is the folder whose file system the summary reports: the
	// torrent metadata folder, which holds the cache.
	DiskPath string
	// OnPauseChange runs after every pause and resume: the download
	// manager's Wake (#77), so a paused torrent's downloads stop at once.
	// Optional.
	OnPauseChange func()
	// Now and Statfs are swapped in tests.
	Now    func() time.Time
	Statfs func(path string) (total, free int64, err error)
}

// Service lists the torrents, sums them up, and pauses and resumes them.
type Service struct {
	torrents Torrents
	sessions Sessions
	library  Library
	pauses   PauseStore
	metainfo Metainfo
	speeds   Speeds
	cache    Cache
	diskPath string
	onPause  func()
	now      func() time.Time
	statfs   func(path string) (total, free int64, err error)
	log      *slog.Logger

	// writes serialises pauses and resumes, so the stored pauses and the
	// client's always end up the same.
	writes sync.Mutex

	// stored caches what each torrent's stored metainfo says: an info dict
	// never changes under its hash.
	storedMu sync.Mutex
	stored   map[string]*torrent.StoredInfo
}

// maxStored bounds the stored-metainfo cache; past it, it starts over.
const maxStored = 1024

// New creates a Service.
func New(d Deps) *Service {
	s := &Service{
		torrents: d.Torrents,
		sessions: d.Sessions,
		library:  d.Library,
		pauses:   d.Pauses,
		metainfo: d.Metainfo,
		speeds:   d.Speeds,
		cache:    d.Cache,
		diskPath: d.DiskPath,
		onPause:  d.OnPauseChange,
		now:      d.Now,
		statfs:   d.Statfs,
		log:      slog.With("component", "activity"),
		stored:   map[string]*torrent.StoredInfo{},
	}
	if s.now == nil {
		s.now = time.Now
	}
	if s.statfs == nil {
		s.statfs = storagecap.Statfs
	}
	return s
}

// RestorePauses applies the stored pauses to the client. Run it at
// startup, before anything loads a torrent. It first prunes the pauses of
// torrents the library doesn't know (no active torrent assignment, no open
// non-dry-run job): a job discard, or any other release, leaves the
// torrent unused, and it shouldn't come back paused when it's assigned
// again. Nothing is loaded yet, so only the library counts.
func (s *Service) RestorePauses(ctx context.Context) error {
	pauses, err := s.pauses.TorrentPauses(ctx)
	if err != nil {
		return err
	}
	if len(pauses) == 0 {
		return nil
	}
	known, err := s.library.KnownTorrents(ctx, "")
	if err != nil {
		return err
	}
	inLibrary := make(map[string]bool, len(known))
	for _, k := range known {
		inLibrary[k.InfoHash] = true
	}
	var stale []string
	for hash := range pauses {
		if !inLibrary[torrent.NormalizeInfoHash(hash)] {
			stale = append(stale, hash)
		}
	}
	if len(stale) > 0 {
		slices.Sort(stale)
		if err := s.pauses.ResumeTorrents(ctx, stale); err != nil {
			return err
		}
		for _, hash := range stale {
			delete(pauses, hash)
		}
		s.log.Info("Pruned the pauses of torrents the library doesn't know", "count", len(stale))
	}
	s.torrents.RestorePauses(pauses)
	return nil
}

// List returns every torrent the library knows, loaded or not, and every
// loaded torrent nothing references (an Inspect dry run, a WebDAV listing):
// streaming, then downloading, paused and idle; then the most recently
// active first (not loaded last), then the newest.
func (s *Service) List(ctx context.Context) ([]Item, error) {
	return s.list(ctx, s.streams(ctx))
}

func (s *Service) list(ctx context.Context, live streams) ([]Item, error) {
	known, err := s.library.KnownTorrents(ctx, "")
	if err != nil {
		return nil, err
	}
	statuses, err := s.torrents.ListTorrents()
	if err != nil {
		return nil, fmt.Errorf("failed to list the loaded torrents: %w", err)
	}
	pauses := s.torrents.Pauses()

	loaded := make(map[string]*torrent.TorrentStatus, len(statuses))
	for i := range statuses {
		loaded[statuses[i].InfoHash] = &statuses[i]
	}
	items := make([]Item, 0, len(known)+len(statuses))
	for i := range known {
		k := &known[i]
		items = append(items, s.item(k.InfoHash, k, loaded[k.InfoHash], pauses, live))
		delete(loaded, k.InfoHash)
	}
	for hash, st := range loaded {
		items = append(items, s.item(hash, nil, st, pauses, live))
	}
	sortItems(items)
	return items, nil
}

// streams are the streaming sessions now: all of them, and the torrents'
// by hash, each the most recently read first.
type streams struct {
	all    []StreamingSession
	byHash map[string][]StreamingSession
}

func (s *Service) streams(ctx context.Context) streams {
	live := streams{all: []StreamingSession{}, byHash: map[string][]StreamingSession{}}
	if s.sessions == nil {
		return live
	}
	live.all = s.sessions.Sessions(ctx)
	for _, ss := range live.all {
		if ss.InfoHash != nil {
			hash := torrent.NormalizeInfoHash(*ss.InfoHash)
			live.byHash[hash] = append(live.byHash[hash], ss)
		}
	}
	return live
}

// Summary returns the totals: speeds and their last minute, connected
// peers, how many torrents are in each state, the disk, and every
// streaming session (HDRezka's too).
func (s *Service) Summary(ctx context.Context) (*Summary, error) {
	live := s.streams(ctx)
	items, err := s.list(ctx, live)
	if err != nil {
		return nil, err
	}
	sum := &Summary{
		History:   History{DownloadBps: []int64{}, UploadBps: []int64{}},
		Disk:      s.disk(),
		Streaming: live.all,
	}
	for _, it := range items {
		if it.Peers != nil {
			sum.Peers += *it.Peers
		}
		switch it.State {
		case StateStreaming:
			sum.Counts.Streaming++
		case StateDownloading:
			sum.Counts.Downloading++
		case StatePaused:
			sum.Counts.Paused++
		default:
			sum.Counts.Idle++
		}
	}
	if s.speeds != nil {
		total := s.speeds.Total()
		sum.DownloadBps, sum.UploadBps = total.Down, total.Up
		sum.History.IntervalSeconds = int(s.speeds.Interval() / time.Second)
		for _, r := range s.speeds.History() {
			sum.History.DownloadBps = append(sum.History.DownloadBps, r.Down)
			sum.History.UploadBps = append(sum.History.UploadBps, r.Up)
		}
	} else {
		sum.History.IntervalSeconds = int(torrent.SpeedInterval / time.Second)
	}
	return sum, nil
}

// Pause pauses a torrent, loaded or not, and keeps it paused across
// restarts: its network goes off and reads don't wake it; one that isn't
// loaded is paused when it loads. Idempotent. torrent.ErrInvalidInfoHash
// for a malformed hash, torrent.ErrTorrentNotFound when nothing knows it.
func (s *Service) Pause(ctx context.Context, infoHash string) (*Item, error) {
	return s.setPaused(ctx, infoHash, true)
}

// Resume clears a torrent's pause: a loaded torrent's network comes back on
// until the idle timeout, one that isn't loaded stays unloaded. Idempotent;
// errors as Pause.
func (s *Service) Resume(ctx context.Context, infoHash string) (*Item, error) {
	return s.setPaused(ctx, infoHash, false)
}

func (s *Service) setPaused(ctx context.Context, infoHash string, pause bool) (*Item, error) {
	hash := torrent.NormalizeInfoHash(infoHash)
	if hash == "" {
		return nil, fmt.Errorf("%w: %q", torrent.ErrInvalidInfoHash, infoHash)
	}
	s.writes.Lock()
	defer s.writes.Unlock()

	if _, _, err := s.lookup(ctx, hash); err != nil {
		return nil, err
	}
	if pause {
		if err := s.pauses.PauseTorrents(ctx, map[string]time.Time{hash: s.now()}); err != nil {
			return nil, err
		}
		if err := s.torrents.Pause(hash); err != nil {
			return nil, err
		}
	} else {
		if err := s.pauses.ResumeTorrents(ctx, []string{hash}); err != nil {
			return nil, err
		}
		if err := s.torrents.Resume(hash); err != nil {
			return nil, err
		}
	}
	if pause {
		s.log.Info("Torrent paused", "info_hash", hash)
	} else {
		s.log.Info("Torrent resumed", "info_hash", hash)
	}
	s.pauseChanged()

	known, st, err := s.lookup(ctx, hash)
	if err != nil {
		return nil, err
	}
	item := s.item(hash, known, st, s.torrents.Pauses(), s.streams(ctx))
	return &item, nil
}

// lookup returns what the library and the client know of a torrent:
// torrent.ErrTorrentNotFound when neither does.
func (s *Service) lookup(ctx context.Context, hash string) (*library.KnownTorrent, *torrent.TorrentStatus, error) {
	known, err := s.library.KnownTorrents(ctx, hash)
	if err != nil {
		return nil, nil, err
	}
	st, err := s.torrents.GetStatus(hash)
	if err != nil {
		st = nil // not loaded
	}
	if len(known) == 0 && st == nil {
		return nil, nil, torrent.ErrTorrentNotFound
	}
	if len(known) == 0 {
		return nil, st, nil
	}
	return &known[0], st, nil
}

// PauseAll pauses every streaming and downloading torrent and returns how
// many it paused.
func (s *Service) PauseAll(ctx context.Context) (int, error) {
	s.writes.Lock()
	defer s.writes.Unlock()

	items, err := s.List(ctx)
	if err != nil {
		return 0, err
	}
	now := s.now()
	targets := map[string]time.Time{}
	for _, it := range items {
		if it.State == StateStreaming || it.State == StateDownloading {
			targets[it.InfoHash] = now
		}
	}
	if err := s.pauses.PauseTorrents(ctx, targets); err != nil {
		return 0, err
	}
	for hash := range targets {
		if err := s.torrents.Pause(hash); err != nil {
			return 0, err
		}
	}
	s.log.Info("Torrents paused", "count", len(targets))
	s.pauseChanged()
	return len(targets), nil
}

// pauseChanged tells the download manager (OnPauseChange).
func (s *Service) pauseChanged() {
	if s.onPause != nil {
		s.onPause()
	}
}

// ResumeAll resumes every paused torrent and returns how many it resumed.
func (s *Service) ResumeAll(ctx context.Context) (int, error) {
	s.writes.Lock()
	defer s.writes.Unlock()

	hashes := slices.Sorted(maps.Keys(s.torrents.Pauses()))
	if err := s.pauses.ResumeTorrents(ctx, hashes); err != nil {
		return 0, err
	}
	for _, hash := range hashes {
		if err := s.torrents.Resume(hash); err != nil {
			return 0, err
		}
	}
	s.log.Info("Torrents resumed", "count", len(hashes))
	s.pauseChanged()
	return len(hashes), nil
}

// item merges what the library (k, nil for a torrent nothing references),
// the client (st, nil while not loaded) and the streaming sessions know of
// a torrent. A torrent that isn't loaded falls back to its stored metainfo
// for a name and size no job gives.
func (s *Service) item(hash string, k *library.KnownTorrent, st *torrent.TorrentStatus, pauses map[string]time.Time, live streams) Item {
	_, paused := pauses[hash]
	it := Item{
		InfoHash:  hash,
		Loaded:    st != nil,
		Titles:    []Title{},
		Streaming: live.byHash[hash],
	}
	if it.Streaming == nil {
		it.Streaming = []StreamingSession{}
	}
	facts := Facts{Paused: paused, Loaded: st != nil, Sessions: len(it.Streaming)}

	if st != nil {
		facts.Active = !st.IsPaused
		facts.Complete = st.Complete
		it.Complete = &st.Complete
		it.Progress = &st.Progress
		it.BytesCompleted = &st.Downloaded
		it.SizeBytes = &st.TotalSize
		it.DownloadBps, it.UploadBps = st.DownloadSpeed, st.UploadSpeed
		it.Peers, it.Seeders = &st.Peers, &st.Seeders
		it.AddedAt = st.LoadedAt
		if !st.LastActive.IsZero() {
			it.LastActiveAt = &st.LastActive
		}
	}
	if k != nil {
		if !k.AddedAt.IsZero() {
			it.AddedAt = k.AddedAt
		}
		if it.SizeBytes == nil {
			it.SizeBytes = k.SizeBytes
		}
		if k.OpenJob != nil {
			it.OpenJob = &OpenJob{ID: k.OpenJob.ID, Status: string(k.OpenJob.Status)}
		}
		for _, t := range k.Titles {
			it.Titles = append(it.Titles, title(t))
		}
	}
	var stored *torrent.StoredInfo
	if st == nil && (it.SizeBytes == nil || jobName(hash, k) == "") {
		stored = s.storedInfo(hash)
	}
	if it.SizeBytes == nil && stored != nil {
		it.SizeBytes = &stored.TotalSize
	}
	it.Name = name(hash, k, st, stored)
	it.State = StateOf(facts)
	return it
}

// storedInfo is the torrent's stored metainfo, read once and cached: nil
// when there's none (or no store).
func (s *Service) storedInfo(hash string) *torrent.StoredInfo {
	if s.metainfo == nil {
		return nil
	}
	s.storedMu.Lock()
	si, ok := s.stored[hash]
	s.storedMu.Unlock()
	if ok {
		return si
	}
	si, err := s.metainfo.Summary(hash)
	if err != nil {
		s.log.Warn("Can't read a torrent's stored metainfo", "info_hash", hash, "error", err)
		return nil
	}
	if si == nil {
		return nil // not cached: it's stored when its metadata arrives
	}
	s.storedMu.Lock()
	if len(s.stored) >= maxStored {
		clear(s.stored)
	}
	s.stored[hash] = si
	s.storedMu.Unlock()
	return si
}

// jobName is the newest job's torrent name, "" when there's none (or it's
// only the hash).
func jobName(hash string, k *library.KnownTorrent) string {
	if k == nil || strings.EqualFold(k.JobName, hash) {
		return ""
	}
	return k.JobName
}

// name is the info dict's name, else the newest job's, else the stored
// metainfo's (a torrent that isn't loaded), else the magnet's dn, else the
// hash.
func name(hash string, k *library.KnownTorrent, st *torrent.TorrentStatus, stored *torrent.StoredInfo) string {
	if st != nil && st.Name != "" {
		return st.Name
	}
	if n := jobName(hash, k); n != "" {
		return n
	}
	if stored != nil && stored.Name != "" {
		return stored.Name
	}
	if k == nil {
		return hash
	}
	if u, err := url.Parse(k.MagnetURI); err == nil {
		if dn := strings.TrimSpace(u.Query().Get("dn")); dn != "" {
			return dn
		}
	}
	return hash
}

func title(t library.KnownTitle) Title {
	out := Title{
		MediaType: string(t.MediaType),
		TMDBID:    t.TMDBID,
		LibraryID: t.LibraryID,
		Title:     t.Title,
		Year:      t.Year,
	}
	if t.PosterPath != "" {
		out.PosterPath = &t.PosterPath
	}
	if t.MediaType == library.JobTV {
		out.Scope = Scope(t.Episodes, t.SeasonSizes)
	}
	return out
}

// sortItems orders the list: by state, then the most recently active
// (never active last), then the newest, then by hash.
func sortItems(items []Item) {
	slices.SortFunc(items, func(a, b Item) int {
		if c := cmp.Compare(stateRank(a.State), stateRank(b.State)); c != 0 {
			return c
		}
		switch {
		case a.LastActiveAt != nil && b.LastActiveAt == nil:
			return -1
		case a.LastActiveAt == nil && b.LastActiveAt != nil:
			return 1
		case a.LastActiveAt != nil:
			if c := b.LastActiveAt.Compare(*a.LastActiveAt); c != 0 {
				return c
			}
		}
		if c := b.AddedAt.Compare(a.AddedAt); c != 0 {
			return c
		}
		return cmp.Compare(a.InfoHash, b.InfoHash)
	})
}

// disk reads the metadata folder's file system and the cache; nil when
// either can't be read.
func (s *Service) disk() *Disk {
	if s.cache == nil || s.diskPath == "" {
		return nil
	}
	total, free, err := s.statfs(s.diskPath)
	if err != nil {
		s.log.Warn("Can't read the disk usage", "path", s.diskPath, "error", err)
		return nil
	}
	info := s.cache.Info()
	return &Disk{TotalBytes: total, FreeBytes: free, TorrentsBytes: info.Filled, CacheLimitBytes: info.Capacity}
}
