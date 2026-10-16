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

	"github.com/anacrolix/missinggo/v2/filecache"

	"github.com/shapedtime/momoshtrem/internal/library"
	"github.com/shapedtime/momoshtrem/internal/streaming"
	"github.com/shapedtime/momoshtrem/internal/torrent"
)

// Torrents is the torrent client. torrent.Service implements it.
type Torrents interface {
	ListTorrents() ([]torrent.TorrentStatus, error)
	GetStatus(infoHash string) (*torrent.TorrentStatus, error)
	Pause(infoHash string) error
	Resume(infoHash string) error
	Pauses() map[string]time.Time
	RestorePauses(pauses map[string]time.Time)
}

var _ Torrents = (torrent.Service)(nil)

// Readers are the open readers of each torrent's files: a torrent with one
// is streaming. *streaming.ReaderTracker implements it; #99's streaming
// sessions replace it.
type Readers interface {
	GetReaders(infoHash string) []streaming.ReaderSnapshot
}

var _ Readers = (*streaming.ReaderTracker)(nil)

// Library is what the library knows about torrents.
// *library.AssignmentJobRepository implements it.
type Library interface {
	KnownTorrents(ctx context.Context, infoHash string) ([]library.KnownTorrent, error)
}

var _ Library = (*library.AssignmentJobRepository)(nil)

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

// Cache is the piece cache. *filecache.Cache implements it.
type Cache interface {
	Info() filecache.CacheInfo
}

var _ Cache = (*filecache.Cache)(nil)

// Deps are a Service's dependencies. Readers, Speeds and Cache are
// optional: without them nothing streams, speeds are 0 and the disk is
// null.
type Deps struct {
	Torrents Torrents
	Readers  Readers
	Library  Library
	Pauses   PauseStore
	Speeds   Speeds
	Cache    Cache
	// DiskPath is the folder whose file system the summary reports: the
	// torrent metadata folder, which holds the cache.
	DiskPath string
	// Now and Statfs are swapped in tests.
	Now    func() time.Time
	Statfs func(path string) (total, free int64, err error)
}

// Service lists the torrents, sums them up, and pauses and resumes them.
type Service struct {
	torrents Torrents
	readers  Readers
	library  Library
	pauses   PauseStore
	speeds   Speeds
	cache    Cache
	diskPath string
	now      func() time.Time
	statfs   func(path string) (total, free int64, err error)
	log      *slog.Logger

	// writes serialises pauses and resumes, so the stored pauses and the
	// client's always end up the same.
	writes sync.Mutex
}

// New creates a Service.
func New(d Deps) *Service {
	s := &Service{
		torrents: d.Torrents,
		readers:  d.Readers,
		library:  d.Library,
		pauses:   d.Pauses,
		speeds:   d.Speeds,
		cache:    d.Cache,
		diskPath: d.DiskPath,
		now:      d.Now,
		statfs:   d.Statfs,
		log:      slog.With("component", "activity"),
	}
	if s.now == nil {
		s.now = time.Now
	}
	if s.statfs == nil {
		s.statfs = statfs
	}
	return s
}

// RestorePauses applies the stored pauses to the client. Run it at
// startup, before anything loads a torrent.
func (s *Service) RestorePauses(ctx context.Context) error {
	pauses, err := s.pauses.TorrentPauses(ctx)
	if err != nil {
		return err
	}
	s.torrents.RestorePauses(pauses)
	return nil
}

// List returns every torrent the library knows, loaded or not, and every
// loaded torrent nothing references (an Inspect dry run, a WebDAV listing):
// streaming, then downloading, paused and idle; then the most recently
// active first (not loaded last), then the newest.
func (s *Service) List(ctx context.Context) ([]Item, error) {
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
		items = append(items, s.item(k.InfoHash, k, loaded[k.InfoHash], pauses))
		delete(loaded, k.InfoHash)
	}
	for hash, st := range loaded {
		items = append(items, s.item(hash, nil, st, pauses))
	}
	sortItems(items)
	return items, nil
}

// Summary returns the totals: speeds and their last minute, connected
// peers, how many torrents are in each state, and the disk.
func (s *Service) Summary(ctx context.Context) (*Summary, error) {
	items, err := s.List(ctx)
	if err != nil {
		return nil, err
	}
	sum := &Summary{
		History:   History{DownloadBps: []int64{}, UploadBps: []int64{}},
		Disk:      s.disk(),
		Streaming: []StreamingSession{},
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

	known, st, err := s.lookup(ctx, hash)
	if err != nil {
		return nil, err
	}
	item := s.item(hash, known, st, s.torrents.Pauses())
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
	return len(targets), nil
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
	return len(hashes), nil
}

// item merges what the library (k, nil for a torrent nothing references)
// and the client (st, nil while not loaded) know of a torrent.
func (s *Service) item(hash string, k *library.KnownTorrent, st *torrent.TorrentStatus, pauses map[string]time.Time) Item {
	_, paused := pauses[hash]
	it := Item{
		InfoHash:  hash,
		Loaded:    st != nil,
		Titles:    []Title{},
		Streaming: []StreamingSession{},
	}
	facts := Facts{Paused: paused, Loaded: st != nil}

	if st != nil {
		facts.Active = !st.IsPaused
		facts.Complete = st.Complete
		if s.readers != nil {
			facts.Readers = len(s.readers.GetReaders(hash))
		}
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
	it.Name = name(hash, k, st)
	it.State = StateOf(facts)
	return it
}

// name is the info dict's name, else the newest job's, else the magnet's
// dn, else the hash.
func name(hash string, k *library.KnownTorrent, st *torrent.TorrentStatus) string {
	if st != nil && st.Name != "" {
		return st.Name
	}
	if k == nil {
		return hash
	}
	if k.JobName != "" && !strings.EqualFold(k.JobName, hash) {
		return k.JobName
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
