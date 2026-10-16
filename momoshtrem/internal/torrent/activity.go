package torrent

import (
	"context"
	"errors"
	"log/slog"
	"maps"
	"sync"
	"time"

	"github.com/anacrolix/torrent"
)

// TorrentState represents the current network state of a torrent.
type TorrentState string

const (
	StateActive TorrentState = "active"
	StateIdle   TorrentState = "idle"
	// StatePaused is a torrent the user paused: its network stays off until
	// it's resumed. Reads don't wake it and the idle check leaves it alone.
	StatePaused TorrentState = "paused"
)

// ErrPaused is WaitForActivation's answer for a paused torrent: it won't
// connect to peers until it's resumed.
var ErrPaused = errors.New("torrent is paused")

// networked is what the manager switches on a torrent. *torrent.Torrent
// implements it.
type networked interface {
	AllowDataDownload()
	DisallowDataDownload()
	AllowDataUpload()
	DisallowDataUpload()
	Stats() torrent.TorrentStats
}

var _ networked = (*torrent.Torrent)(nil)

// ActivityManager tracks torrent file access and manages idle state.
// When no files are being read, it pauses network activity to save bandwidth.
//
// It also holds the user's pauses (PLAN §6 "Torrents and activity (B6)"):
// sticky, for torrents loaded or not. A paused torrent's network stays off
// whatever reads it, and one that isn't loaded is paused when it registers.
// The pauses live in memory; the caller persists them and restores them at
// startup (RestorePauses).
type ActivityManager struct {
	mu         sync.RWMutex
	torrents   map[string]networked    // hash -> torrent
	lastAccess map[string]time.Time    // hash -> last access time
	state      map[string]TorrentState // hash -> current state
	paused     map[string]time.Time    // hash -> when the user paused it, loaded or not

	idleTimeout   time.Duration
	checkInterval time.Duration
	startPaused   bool
	now           func() time.Time

	stopChan chan struct{}
	stopped  bool
	log      *slog.Logger
}

// NewActivityManager creates a new activity manager.
// idleTimeout: duration of inactivity before pausing a torrent
// startPaused: whether new torrents should start with network disabled
func NewActivityManager(idleTimeout time.Duration, startPaused bool) *ActivityManager {
	return &ActivityManager{
		torrents:      make(map[string]networked),
		lastAccess:    make(map[string]time.Time),
		state:         make(map[string]TorrentState),
		paused:        make(map[string]time.Time),
		idleTimeout:   idleTimeout,
		checkInterval: 30 * time.Second,
		startPaused:   startPaused,
		now:           time.Now,
		stopChan:      make(chan struct{}),
		log:           slog.With("component", "activity-manager"),
	}
}

// newPauseManager is the manager the torrent service uses when idle mode is
// off: torrents start active and never idle (Start isn't called), and only
// the user's pauses turn their network off.
func newPauseManager() *ActivityManager {
	return NewActivityManager(0, false)
}

// Register adds a torrent to be managed. A paused torrent's network is
// disabled at once, and so is every torrent's when startPaused is set.
func (am *ActivityManager) Register(hash string, t *torrent.Torrent) {
	am.register(hash, t)
}

func (am *ActivityManager) register(hash string, t networked) {
	am.mu.Lock()
	defer am.mu.Unlock()

	am.torrents[hash] = t
	am.lastAccess[hash] = am.now()

	_, paused := am.paused[hash]
	switch {
	case paused:
		am.setPaused(hash, t)
	case am.startPaused:
		am.setIdle(hash, t)
	default:
		am.state[hash] = StateActive
	}

	am.log.Info("registered torrent",
		"hash", hash,
		"state", string(am.state[hash]),
		"start_paused", am.startPaused,
	)
}

// Unregister removes a torrent from management. Its pause, if any, stays:
// it applies again when the torrent loads.
func (am *ActivityManager) Unregister(hash string) {
	am.mu.Lock()
	defer am.mu.Unlock()

	delete(am.torrents, hash)
	delete(am.lastAccess, hash)
	delete(am.state, hash)

	am.log.Info("unregistered torrent", "hash", hash)
}

// MarkActive signals that a torrent's files are being accessed.
// This wakes up idle torrents and resets the idle timer. It does nothing
// while the torrent is paused.
func (am *ActivityManager) MarkActive(hash string) {
	am.mu.Lock()
	defer am.mu.Unlock()

	t, registered := am.torrents[hash]
	if _, paused := am.paused[hash]; paused || !registered {
		return
	}
	am.lastAccess[hash] = am.now()

	// Wake up if idle
	if am.state[hash] == StateIdle {
		am.setActive(hash, t)
	}
}

// WaitForActivation blocks until the torrent has connected peers or timeout expires.
// Returns immediately if: torrent not registered, already has peers, or start_paused is false.
// This fixes the race condition where MarkActive enables network but the torrent needs
// time to connect to peers before data is available.
// A paused torrent returns ErrPaused at once: it won't connect.
func (am *ActivityManager) WaitForActivation(hash string, timeout time.Duration) error {
	if _, paused := am.PausedAt(hash); paused {
		return ErrPaused
	}
	am.MarkActive(hash) // Enable network first

	if !am.startPaused {
		return nil
	}

	am.mu.RLock()
	t, exists := am.torrents[hash]
	am.mu.RUnlock()

	if !exists || t.Stats().ActivePeers > 0 {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			am.log.Warn("timeout waiting for torrent activation", "hash", hash)
			return ctx.Err()
		case <-ticker.C:
			if t.Stats().ActivePeers > 0 {
				return nil
			}
		}
	}
}

// Pause pauses the torrent, loaded or not: a loaded torrent's network goes
// off now, one that isn't loaded is paused when it registers. at is when it
// was paused; pausing a paused torrent keeps the first time.
func (am *ActivityManager) Pause(hash string, at time.Time) {
	am.mu.Lock()
	defer am.mu.Unlock()

	if _, paused := am.paused[hash]; paused {
		return
	}
	am.paused[hash] = at
	if t, ok := am.torrents[hash]; ok {
		am.setPaused(hash, t)
	}
}

// Resume clears the torrent's pause. A loaded torrent's network comes back
// on and it counts as just accessed, so it stays on until the idle timeout;
// one that isn't loaded stays unloaded. Resuming a torrent that isn't
// paused changes nothing.
func (am *ActivityManager) Resume(hash string) {
	am.mu.Lock()
	defer am.mu.Unlock()

	if _, paused := am.paused[hash]; !paused {
		return
	}
	delete(am.paused, hash)
	if t, ok := am.torrents[hash]; ok {
		am.lastAccess[hash] = am.now()
		am.setActive(hash, t)
	}
}

// RestorePauses applies pauses kept from before a restart, as Pause does.
func (am *ActivityManager) RestorePauses(pauses map[string]time.Time) {
	for hash, at := range pauses {
		am.Pause(hash, at)
	}
}

// PausedAt returns when the torrent was paused; ok is false when it isn't.
func (am *ActivityManager) PausedAt(hash string) (at time.Time, ok bool) {
	am.mu.RLock()
	defer am.mu.RUnlock()
	at, ok = am.paused[hash]
	return at, ok
}

// Pauses returns every paused torrent, loaded or not, with when it was
// paused. The map is a copy.
func (am *ActivityManager) Pauses() map[string]time.Time {
	am.mu.RLock()
	defer am.mu.RUnlock()
	return maps.Clone(am.paused)
}

// LastAccess returns when a registered torrent was last read, resumed or
// registered; ok is false when it isn't registered.
func (am *ActivityManager) LastAccess(hash string) (at time.Time, ok bool) {
	am.mu.RLock()
	defer am.mu.RUnlock()
	at, ok = am.lastAccess[hash]
	return at, ok
}

// setActive enables network activity for a torrent.
func (am *ActivityManager) setActive(hash string, t networked) {
	t.AllowDataDownload()
	t.AllowDataUpload()
	am.state[hash] = StateActive
	am.log.Info("torrent activated - network enabled", "hash", hash)
}

// setIdle disables network activity for a torrent.
func (am *ActivityManager) setIdle(hash string, t networked) {
	t.DisallowDataDownload()
	t.DisallowDataUpload()
	am.state[hash] = StateIdle
	am.log.Info("torrent idle - network disabled", "hash", hash)
}

// setPaused disables network activity for a paused torrent.
func (am *ActivityManager) setPaused(hash string, t networked) {
	t.DisallowDataDownload()
	t.DisallowDataUpload()
	am.state[hash] = StatePaused
	am.log.Info("torrent paused - network disabled", "hash", hash)
}

// Start begins the background idle check goroutine.
func (am *ActivityManager) Start() {
	am.log.Info("activity manager started",
		"idle_timeout_seconds", am.idleTimeout.Seconds(),
		"check_interval_seconds", am.checkInterval.Seconds(),
		"start_paused", am.startPaused,
	)

	go am.idleCheckLoop()
}

// Stop halts the background idle check goroutine.
func (am *ActivityManager) Stop() {
	am.mu.Lock()
	if am.stopped {
		am.mu.Unlock()
		return
	}
	am.stopped = true
	am.mu.Unlock()

	close(am.stopChan)
	am.log.Info("activity manager stopped")
}

// idleCheckLoop periodically checks for idle torrents.
func (am *ActivityManager) idleCheckLoop() {
	ticker := time.NewTicker(am.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-am.stopChan:
			return
		case <-ticker.C:
			am.checkIdleTorrents()
		}
	}
}

// checkIdleTorrents finds active torrents that should become idle.
// Uses two-phase locking to minimize contention with MarkActive().
// Paused torrents aren't active, so they're never candidates.
func (am *ActivityManager) checkIdleTorrents() {
	// Phase 1: Collect candidates using RLock (allows concurrent MarkActive)
	am.mu.RLock()
	now := am.now()
	var candidates []string
	for hash := range am.torrents {
		if am.state[hash] == StateActive {
			if now.Sub(am.lastAccess[hash]) >= am.idleTimeout {
				candidates = append(candidates, hash)
			}
		}
	}
	am.mu.RUnlock()

	if len(candidates) == 0 {
		return
	}

	// Phase 2: Apply changes using Lock (brief, targeted)
	am.mu.Lock()
	defer am.mu.Unlock()

	for _, hash := range candidates {
		// Re-check conditions - state may have changed since Phase 1
		if am.state[hash] != StateActive {
			continue
		}
		// Re-check time - might have been marked active since collection
		if am.now().Sub(am.lastAccess[hash]) < am.idleTimeout {
			continue
		}
		if t, ok := am.torrents[hash]; ok {
			am.setIdle(hash, t)
		}
	}
}

// GetState returns the current state of a torrent.
func (am *ActivityManager) GetState(hash string) TorrentState {
	am.mu.RLock()
	defer am.mu.RUnlock()
	return am.state[hash]
}

// IsPaused returns whether a registered torrent's network is off: idle, or
// paused by the user. Use PausedAt for the user's pause alone.
func (am *ActivityManager) IsPaused(hash string) bool {
	state := am.GetState(hash)
	return state == StateIdle || state == StatePaused
}

// GetStats returns activity statistics for monitoring/debugging.
// idle_torrents counts every registered torrent whose network is off,
// paused ones too; paused_torrents counts the user's pauses, loaded or not.
func (am *ActivityManager) GetStats() map[string]interface{} {
	am.mu.RLock()
	defer am.mu.RUnlock()

	active := 0
	idle := 0
	for _, state := range am.state {
		if state == StateActive {
			active++
		} else {
			idle++
		}
	}

	return map[string]interface{}{
		"idle_timeout_seconds": am.idleTimeout.Seconds(),
		"active_torrents":      active,
		"idle_torrents":        idle,
		"paused_torrents":      len(am.paused),
		"total_torrents":       len(am.torrents),
		"start_paused":         am.startPaused,
	}
}
