package torrent

import (
	"sync"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// TorrentState represents the current network state of a torrent
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
	log      zerolog.Logger
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
		checkInterval: 30 * time.Second, // Check every 30 seconds
		startPaused:   startPaused,
		stopChan:      make(chan struct{}),
		log:           log.Logger.With().Str("component", "activity-manager").Logger(),
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

	am.log.Info().
		Str("hash", hash).
		Str("state", string(am.state[hash])).
		Bool("start_paused", am.startPaused).
		Msg("registered torrent")
}

// Unregister removes a torrent from management.
func (am *ActivityManager) Unregister(hash string) {
	am.mu.Lock()
	defer am.mu.Unlock()

	delete(am.torrents, hash)
	delete(am.lastAccess, hash)
	delete(am.state, hash)

	am.log.Info().Str("hash", hash).Msg("unregistered torrent")
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

// setActive enables network activity for a torrent.
func (am *ActivityManager) setActive(hash string, t *torrent.Torrent) {
	t.AllowDataDownload()
	t.AllowDataUpload()
	am.state[hash] = StateActive
	am.log.Info().Str("hash", hash).Msg("torrent activated - network enabled")
}

// setIdle disables network activity for a torrent.
func (am *ActivityManager) setIdle(hash string, t *torrent.Torrent) {
	t.DisallowDataDownload()
	t.DisallowDataUpload()
	am.state[hash] = StateIdle
	am.log.Info().Str("hash", hash).Msg("torrent idle - network disabled")
}

// Start begins the background idle check goroutine.
func (am *ActivityManager) Start() {
	am.log.Info().
		Dur("idle_timeout", am.idleTimeout).
		Dur("check_interval", am.checkInterval).
		Bool("start_paused", am.startPaused).
		Msg("activity manager started")

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
	am.log.Info().Msg("activity manager stopped")
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
func (am *ActivityManager) checkIdleTorrents() {
	am.mu.Lock()
	defer am.mu.Unlock()

	now := time.Now()

	for hash, t := range am.torrents {
		if am.state[hash] != StateActive {
			continue
		}

		lastAccess := am.lastAccess[hash]
		if now.Sub(lastAccess) >= am.idleTimeout {
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
		"idleTimeout":     am.idleTimeout.Seconds(),
		"activeTorrents":  active,
		"idleTorrents":    idle,
		"totalTorrents":   len(am.torrents),
		"startPaused":     am.startPaused,
	}
}
