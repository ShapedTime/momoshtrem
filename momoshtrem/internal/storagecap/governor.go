package storagecap

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/shapedtime/momoshtrem/internal/library"
)

// Interval is how often the Governor checks the disk.
const Interval = time.Minute

// stepPause is the wait between two trim steps, so streaming reads get the
// cache's lock in between.
const stepPause = 100 * time.Millisecond

// CacheInfo is what the Governor reads of the piece cache.
type CacheInfo struct {
	// Capacity is the cap now; negative is unlimited.
	Capacity int64
	// Filled is every byte in the cache, Pinned the part kept and smart
	// downloads hold (#201; 0 until then).
	Filled int64
	Pinned int64
}

// Cache is the piece cache the Governor caps: missinggo's filecache, or
// #201's piececache. main adapts it with CacheFuncs, so swapping the type
// is a one-line change there.
type Cache interface {
	Info() CacheInfo
	// SetCapacity sets the cap without trimming.
	SetCapacity(bytes int64)
	// TrimToCapacity evicts until the fill is under the cap.
	TrimToCapacity()
}

// CacheFuncs adapts a cache's methods to Cache.
type CacheFuncs struct {
	InfoFunc           func() CacheInfo
	SetCapacityFunc    func(bytes int64)
	TrimToCapacityFunc func()
}

func (c CacheFuncs) Info() CacheInfo         { return c.InfoFunc() }
func (c CacheFuncs) SetCapacity(bytes int64) { c.SetCapacityFunc(bytes) }
func (c CacheFuncs) TrimToCapacity()         { c.TrimToCapacityFunc() }

var _ Cache = CacheFuncs{}

// Settings stores the "storage" document (*library.SettingsRepository).
type Settings interface {
	StorageSettings(ctx context.Context) (library.StorageSettings, error)
	UpdateStorageSettings(ctx context.Context, patch library.StorageSettingsPatch) (library.StorageSettings, error)
}

var _ Settings = (*library.SettingsRepository)(nil)

// DownloadUsage is what kept and smart downloads hold.
type DownloadUsage struct {
	// KeptBytes and SmartBytes are on disk now.
	KeptBytes  int64
	SmartBytes int64
	// CommittedKeptBytes is the wanted size of every kept download, on
	// disk or not: a PUT can't cap kept downloads below it.
	CommittedKeptBytes int64
}

// Downloads reports the downloads' usage (#77 P4). Until it lands,
// NoDownloads: nothing is kept.
type Downloads interface {
	DownloadUsage(ctx context.Context) (DownloadUsage, error)
}

// NoDownloads is Downloads before there are any.
type NoDownloads struct{}

func (NoDownloads) DownloadUsage(context.Context) (DownloadUsage, error) { return DownloadUsage{}, nil }

// Clock is the Governor's time; tests drive a fake one.
type Clock interface {
	// After fires once d has passed.
	After(d time.Duration) <-chan time.Time
}

type realClock struct{}

func (realClock) After(d time.Duration) <-chan time.Time { return time.After(d) }

// Deps are what a Governor needs.
type Deps struct {
	Settings Settings
	// DiskPath is the folder whose file system holds the cache: the
	// torrent metadata folder.
	DiskPath string
	// ConfigBytes is config.yaml's global_cache_size, nil when the file
	// doesn't set it (config.TorrentConfig.GlobalCacheBytes).
	ConfigBytes *int64
	// Downloads is nil until #77 P4: nothing is kept.
	Downloads Downloads
	// Statfs and Clock are swapped in tests; nil is the real ones.
	Statfs func(path string) (total, free int64, err error)
	Clock  Clock
	// StepPause is the wait between trim steps; 0 is 100 ms.
	StepPause time.Duration
}

// Governor caps the piece cache at the effective size, and serves the
// storage settings.
type Governor struct {
	d   Deps
	log *slog.Logger

	wake chan struct{} // a PUT changed the settings: apply now

	mu        sync.Mutex
	cache     Cache // set by Start
	downloads Downloads
	onChange  func()
	known     *library.StorageSettings // the last settings read, for a pass that can't read them
	last      *Limits                  // what the last pass applied, for its log lines
	cancel    context.CancelFunc
	done      chan struct{}
}

// New creates a Governor; StartupLimit sizes the cache before it exists,
// and Start runs the Governor over it.
func New(d Deps) *Governor {
	if d.Statfs == nil {
		d.Statfs = Statfs
	}
	if d.Clock == nil {
		d.Clock = realClock{}
	}
	if d.StepPause <= 0 {
		d.StepPause = stepPause
	}
	downloads := d.Downloads
	if downloads == nil {
		downloads = NoDownloads{}
	}
	return &Governor{
		d: d, log: slog.With("component", "storagecap"),
		wake: make(chan struct{}, 1), downloads: downloads,
	}
}

// SetDownloads installs the downloads' usage (#77 P4).
func (g *Governor) SetDownloads(d Downloads) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if d == nil {
		d = NoDownloads{}
	}
	g.downloads = d
}

// SetOnChange installs a function called after each successful PUT, once
// the Governor has been woken: the downloads plan again (#77 P4).
func (g *Governor) SetOnChange(fn func()) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.onChange = fn
}

// StartupLimit is the limit to open the cache with (torrent.InitStorage):
// the setting, config.yaml or auto. The floor needs the cache's fill, so
// Start's first pass applies it. Without the settings it goes on with the
// defaults (config.yaml, else auto); without the disk, auto is 4 GiB.
func (g *Governor) StartupLimit(ctx context.Context) Limits {
	settings, err := g.d.Settings.StorageSettings(ctx)
	if err != nil {
		g.log.Warn("Couldn't read the storage settings; sizing the cache from config.yaml or the disk", "error", err)
	}
	disk, err := g.disk()
	if err != nil {
		g.log.Warn("Couldn't read the disk; auto is 4 GiB", "path", g.d.DiskPath, "error", err)
	}
	l := Resolve(Inputs{Settings: settings, ConfigBytes: g.d.ConfigBytes, Disk: disk})
	l.EffectiveBytes, l.Floored = l.LimitBytes, false // no floor before the fill is known
	return l
}

// Start applies the effective size to cache once, then every minute and
// after each PUT, until Close.
func (g *Governor) Start(ctx context.Context, cache Cache) {
	g.mu.Lock()
	if g.cancel != nil {
		g.mu.Unlock()
		return
	}
	g.cache = cache
	runCtx, cancel := context.WithCancel(context.Background())
	g.cancel, g.done = cancel, make(chan struct{})
	g.mu.Unlock()

	g.pass(ctx)
	go func() {
		defer close(g.done)
		g.loop(runCtx)
	}()
}

// Close stops the loop, interrupting a trim between two steps.
func (g *Governor) Close() {
	g.mu.Lock()
	cancel, done := g.cancel, g.done
	g.mu.Unlock()
	if cancel == nil {
		return
	}
	cancel()
	<-done
}

// Wake applies the settings now.
func (g *Governor) Wake() {
	select {
	case g.wake <- struct{}{}:
	default:
	}
}

func (g *Governor) loop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-g.wake:
		case <-g.d.Clock.After(Interval):
		}
		g.pass(ctx)
	}
}

// pass reads the settings, the disk and the cache, and applies the
// effective size. Without the settings it uses the last ones read (the
// floor still matters); without the disk it changes nothing.
func (g *Governor) pass(ctx context.Context) {
	cache := g.currentCache()
	if cache == nil {
		return
	}
	settings, err := g.d.Settings.StorageSettings(ctx)
	g.mu.Lock()
	switch {
	case err == nil:
		g.known = &settings
	case ctx.Err() != nil:
		g.mu.Unlock()
		return
	case g.known != nil:
		settings = *g.known
	}
	g.mu.Unlock()
	if err != nil {
		g.log.Warn("Couldn't read the storage settings; applying the last ones read", "error", err)
	}
	disk, err := g.disk()
	if err != nil {
		g.log.Warn("Couldn't read the disk; the cache keeps its cap", "path", g.d.DiskPath, "error", err)
		return
	}
	info := cache.Info()
	l := Resolve(Inputs{Settings: settings, ConfigBytes: g.d.ConfigBytes, Disk: disk, FilledBytes: info.Filled})
	g.logChange(l, disk)
	g.apply(ctx, cache, l.EffectiveBytes)
}

// apply moves the cache's cap to target. While the fill is under target
// it's one SetCapacity. Otherwise it steps down from the fill at most
// TrimStepBytes at a time, trimming after each step, so no one trim holds
// the cache's lock for long: also when the cap is already at target but
// the cache holds more (opened below its fill, which filecache would trim
// in one go on its next write). Once a step can't evict its share (the
// rest is pinned, #201), it goes straight to target.
func (g *Governor) apply(ctx context.Context, cache Cache, target int64) {
	info := cache.Info()
	if info.Filled <= target {
		if info.Capacity != target {
			cache.SetCapacity(target)
		}
		return
	}
	for c := info.Filled; c > target; {
		c = max(target, c-TrimStepBytes)
		cache.SetCapacity(c)
		cache.TrimToCapacity()
		if c == target {
			return
		}
		if cache.Info().Filled > c { // nothing more to evict
			cache.SetCapacity(target)
			cache.TrimToCapacity()
			return
		}
		select {
		case <-ctx.Done(): // the next start's first pass goes on from here
			return
		case <-time.After(g.d.StepPause):
		}
	}
}

// logChange logs a new source or limit, and the floor taking or letting go.
func (g *Governor) logChange(l Limits, disk Disk) {
	g.mu.Lock()
	last := g.last
	g.last = &l
	g.mu.Unlock()
	if last != nil && last.Source == l.Source && last.LimitBytes == l.LimitBytes && last.Floored == l.Floored {
		return
	}
	attrs := []any{"source", l.Source, "limit_bytes", l.LimitBytes, "effective_bytes", l.EffectiveBytes,
		"disk_total_bytes", disk.TotalBytes, "disk_free_bytes", disk.FreeBytes}
	if l.Floored {
		g.log.Warn("Piece cache held below its limit to keep the disk's free space", attrs...)
		return
	}
	g.log.Info("Piece cache limit", attrs...)
}

func (g *Governor) currentCache() Cache {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.cache
}

func (g *Governor) currentDownloads() Downloads {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.downloads
}

func (g *Governor) cacheInfo() CacheInfo {
	if cache := g.currentCache(); cache != nil {
		return cache.Info()
	}
	return CacheInfo{}
}

func (g *Governor) disk() (Disk, error) {
	total, free, err := g.d.Statfs(g.d.DiskPath)
	if err != nil {
		return Disk{}, err
	}
	return Disk{TotalBytes: total, FreeBytes: free}, nil
}

// Limits are the limits now, with the settings they come from: for the
// downloads (#77 P4), which never call back into Downloads from here.
func (g *Governor) Limits(ctx context.Context) (library.StorageSettings, Limits, error) {
	settings, err := g.d.Settings.StorageSettings(ctx)
	if err != nil {
		return settings, Limits{}, err
	}
	in, err := g.inputs(settings)
	if err != nil {
		return settings, Limits{}, err
	}
	return settings, Resolve(in), nil
}

func (g *Governor) inputs(settings library.StorageSettings) (Inputs, error) {
	disk, err := g.disk()
	if err != nil {
		return Inputs{}, fmt.Errorf("failed to read the disk of %s: %w", g.d.DiskPath, err)
	}
	return Inputs{Settings: settings, ConfigBytes: g.d.ConfigBytes, Disk: disk, FilledBytes: g.cacheInfo().Filled}, nil
}

// Storage is GET /settings/storage.
func (g *Governor) Storage(ctx context.Context) (*Report, error) {
	settings, err := g.d.Settings.StorageSettings(ctx)
	if err != nil {
		return nil, err
	}
	in, err := g.inputs(settings)
	if err != nil {
		return nil, err
	}
	usage, err := g.currentDownloads().DownloadUsage(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to read the downloads' usage: %w", err)
	}
	return newReport(in, Resolve(in), g.cacheInfo(), usage), nil
}

// UpdateStorage is PUT /settings/storage: it checks the patch against the
// disk and the downloads, stores it, wakes the Governor and answers the
// new report. A bad value is a *FieldError and a cap below what kept
// downloads hold a *CommittedError; neither changes anything.
func (g *Governor) UpdateStorage(ctx context.Context, patch library.StorageSettingsPatch) (*Report, error) {
	current, err := g.d.Settings.StorageSettings(ctx)
	if err != nil {
		return nil, err
	}
	in, err := g.inputs(current)
	if err != nil {
		return nil, err
	}
	before := Resolve(in)
	in.Settings = patch.Apply(current)
	after := Resolve(in)
	if err := checkLimits(patch, after); err != nil {
		return nil, err
	}
	usage, err := g.currentDownloads().DownloadUsage(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to read the downloads' usage: %w", err)
	}
	// Only a change that lowers the kept cap below what's committed is
	// refused: one that raises it, while still short, helps.
	if after.KeptMaxBytes < usage.CommittedKeptBytes && after.KeptMaxBytes < before.KeptMaxBytes {
		return nil, &CommittedError{CommittedKeptBytes: usage.CommittedKeptBytes, KeptMaxBytes: after.KeptMaxBytes}
	}
	saved, err := g.d.Settings.UpdateStorageSettings(ctx, patch)
	if err != nil {
		return nil, err
	}
	g.Wake()
	g.mu.Lock()
	onChange := g.onChange
	g.mu.Unlock()
	if onChange != nil {
		onChange()
	}
	in.Settings = saved
	return newReport(in, Resolve(in), g.cacheInfo(), usage), nil
}
