package torrent

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/anacrolix/torrent"
)

// TorrentState represents the current network state of a torrent.
type TorrentState string

const (
	StateActive TorrentState = "active"
	StateIdle   TorrentState = "idle"
)

// ActivityManager tracks torrent file access and manages idle state.
// When no files are being read, it pauses network activity to save bandwidth.
type ActivityManager struct {
	mu         sync.RWMutex
	torrents   map[string]*torrent.Torrent // hash -> torrent
	lastAccess map[string]time.Time        // hash -> last access time
	state      map[string]TorrentState     // hash -> current state

	idleTimeout   time.Duration
	checkInterval time.Duration
	startPaused   bool

	stopChan chan struct{}
	stopped  bool
	log      *slog.Logger
}

// NewActivityManager creates a new activity manager.
// idleTimeout: duration of inactivity before pausing a torrent
// startPaused: whether new torrents should start with network disabled
func NewActivityManager(idleTimeout time.Duration, startPaused bool) *ActivityManager {
	return &ActivityManager{
		torrents:      make(map[string]*torrent.Torrent),
		lastAccess:    make(map[string]time.Time),
		state:         make(map[string]TorrentState),
		idleTimeout:   idleTimeout,
		checkInterval: 30 * time.Second,
		startPaused:   startPaused,
		stopChan:      make(chan struct{}),
		log:           slog.With("component", "activity-manager"),
	}
}

// Register adds a torrent to be managed.
// If startPaused is true, the torrent's network activity is disabled immediately.
func (am *ActivityManager) Register(hash string, t *torrent.Torrent) {
	am.mu.Lock()
	defer am.mu.Unlock()

	am.torrents[hash] = t
	am.lastAccess[hash] = time.Now()

	if am.startPaused {
		am.setIdle(hash, t)
	} else {
		am.state[hash] = StateActive
	}

	am.log.Info("registered torrent",
		"hash", hash,
		"state", string(am.state[hash]),
		"start_paused", am.startPaused,
	)
}

// Unregister removes a torrent from management.
func (am *ActivityManager) Unregister(hash string) {
	am.mu.Lock()
	defer am.mu.Unlock()

	delete(am.torrents, hash)
	delete(am.lastAccess, hash)
	delete(am.state, hash)

	am.log.Info("unregistered torrent", "hash", hash)
}

// MarkActive signals that a torrent's files are being accessed.
// This wakes up idle torrents and resets the idle timer.
func (am *ActivityManager) MarkActive(hash string) {
	am.mu.Lock()
	defer am.mu.Unlock()

	am.lastAccess[hash] = time.Now()

	// Wake up if idle
	if am.state[hash] == StateIdle {
		if t, ok := am.torrents[hash]; ok {
			am.setActive(hash, t)
		}
	}
}

// WaitForActivation blocks until the torrent has connected peers or timeout expires.
// Returns immediately if: torrent not registered, already has peers, or start_paused is false.
// This fixes the race condition where MarkActive enables network but the torrent needs
// time to connect to peers before data is available.
func (am *ActivityManager) WaitForActivation(hash string, timeout time.Duration) error {
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

// setActive enables network activity for a torrent.
func (am *ActivityManager) setActive(hash string, t *torrent.Torrent) {
	t.AllowDataDownload()
	t.AllowDataUpload()
	am.state[hash] = StateActive
	am.log.Info("torrent activated - network enabled", "hash", hash)
}

// setIdle disables network activity for a torrent.
func (am *ActivityManager) setIdle(hash string, t *torrent.Torrent) {
	t.DisallowDataDownload()
	t.DisallowDataUpload()
	am.state[hash] = StateIdle
	am.log.Info("torrent idle - network disabled", "hash", hash)
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
func (am *ActivityManager) checkIdleTorrents() {
	// Phase 1: Collect candidates using RLock (allows concurrent MarkActive)
	am.mu.RLock()
	now := time.Now()
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
		if time.Since(am.lastAccess[hash]) < am.idleTimeout {
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

// IsPaused returns whether a torrent is currently idle/paused.
func (am *ActivityManager) IsPaused(hash string) bool {
	return am.GetState(hash) == StateIdle
}

// GetStats returns activity statistics for monitoring/debugging.
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
		"total_torrents":       len(am.torrents),
		"start_paused":         am.startPaused,
	}
}
