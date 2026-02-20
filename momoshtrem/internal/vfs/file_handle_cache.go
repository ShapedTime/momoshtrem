package vfs

import (
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/shapedtime/momoshtrem/internal/common"
	"github.com/shapedtime/momoshtrem/internal/metrics"
)

// fileHandleKey uniquely identifies a torrent file for caching.
type fileHandleKey struct {
	infoHash string
	filePath string
}

// fileHandleCacheEntry holds a shared TorrentFile with reference counting.
type fileHandleCacheEntry struct {
	mu         sync.Mutex
	tf         *TorrentFile
	refCount   int
	graceTimer *time.Timer
}

// fileHandleCache caches open TorrentFile handles keyed by {infoHash, filePath}.
// When all references are released, a grace period keeps the handle alive
// so that rapid re-opens (e.g. Infuse reconnects) reuse the same reader
// instead of creating competing ones.
type fileHandleCache struct {
	mu          sync.Mutex
	entries     map[fileHandleKey]*fileHandleCacheEntry
	gracePeriod time.Duration
	metrics     *metrics.Metrics
}

func newFileHandleCache(gracePeriod time.Duration) *fileHandleCache {
	return &fileHandleCache{
		entries:     make(map[fileHandleKey]*fileHandleCacheEntry),
		gracePeriod: gracePeriod,
	}
}

// getOrCreate returns a CachedFileHandle for the key if an entry exists.
// Returns nil if no entry is cached; the caller must create one and call put().
func (c *fileHandleCache) getOrCreate(key fileHandleKey) *CachedFileHandle {
	c.mu.Lock()
	entry, ok := c.entries[key]
	if !ok {
		c.mu.Unlock()
		if c.metrics != nil {
			c.metrics.StreamingCacheOps.WithLabelValues("miss").Inc()
		}
		return nil
	}

	entry.mu.Lock()
	c.mu.Unlock()

	// Cancel grace timer if running
	if entry.graceTimer != nil {
		entry.graceTimer.Stop()
		entry.graceTimer = nil
		if c.metrics != nil {
			c.metrics.StreamingCacheOps.WithLabelValues("grace_cancel").Inc()
		}
		slog.Debug("file handle cache: grace period cancelled",
			"info_hash", key.infoHash,
			"file_path", key.filePath,
		)
	}

	entry.refCount++
	tf := entry.tf
	entry.mu.Unlock()

	if c.metrics != nil {
		c.metrics.StreamingCacheOps.WithLabelValues("hit").Inc()
	}
	slog.Debug("file handle cache hit",
		"info_hash", key.infoHash,
		"file_path", key.filePath,
		"ref_count", entry.refCount,
	)

	return &CachedFileHandle{
		tf:    tf,
		key:   key,
		cache: c,
	}
}

// put inserts a new TorrentFile into the cache with refCount=1.
// If another goroutine raced and already inserted an entry for the same key,
// the duplicate tf is closed and the existing entry is returned.
func (c *fileHandleCache) put(key fileHandleKey, tf *TorrentFile) *CachedFileHandle {
	c.mu.Lock()

	if existing, ok := c.entries[key]; ok {
		// Race: another goroutine already inserted. Close our duplicate.
		existing.mu.Lock()
		c.mu.Unlock()

		// Cancel grace timer if running
		if existing.graceTimer != nil {
			existing.graceTimer.Stop()
			existing.graceTimer = nil
		}
		existing.refCount++
		existingTF := existing.tf
		existing.mu.Unlock()

		// Close the duplicate TorrentFile we just created
		go tf.Close()

		slog.Debug("file handle cache: race resolved, reusing existing",
			"info_hash", key.infoHash,
			"file_path", key.filePath,
		)

		return &CachedFileHandle{
			tf:    existingTF,
			key:   key,
			cache: c,
		}
	}

	entry := &fileHandleCacheEntry{
		tf:       tf,
		refCount: 1,
	}
	c.entries[key] = entry
	c.mu.Unlock()

	slog.Debug("file handle cache miss",
		"info_hash", key.infoHash,
		"file_path", key.filePath,
	)

	return &CachedFileHandle{
		tf:    tf,
		key:   key,
		cache: c,
	}
}

// decRef decrements the reference count for a key. When it reaches 0,
// a grace timer is started. If the timer fires without a new reference,
// the entry is evicted and the TorrentFile is closed.
func (c *fileHandleCache) decRef(key fileHandleKey) {
	c.mu.Lock()
	entry, ok := c.entries[key]
	if !ok {
		c.mu.Unlock()
		return
	}
	entry.mu.Lock()
	c.mu.Unlock()

	entry.refCount--
	if entry.refCount > 0 {
		entry.mu.Unlock()
		return
	}

	// refCount == 0: start grace timer
	if c.metrics != nil {
		c.metrics.StreamingCacheOps.WithLabelValues("grace_start").Inc()
	}
	slog.Debug("file handle cache: grace period started",
		"info_hash", key.infoHash,
		"file_path", key.filePath,
		"grace_seconds", c.gracePeriod.Seconds(),
	)

	entry.graceTimer = time.AfterFunc(c.gracePeriod, func() {
		c.evict(key)
	})
	entry.mu.Unlock()
}

// evict removes an entry and closes its TorrentFile.
// Only evicts if refCount is still 0 (a new reference may have arrived).
func (c *fileHandleCache) evict(key fileHandleKey) {
	c.mu.Lock()
	entry, ok := c.entries[key]
	if !ok {
		c.mu.Unlock()
		return
	}

	entry.mu.Lock()
	if entry.refCount > 0 {
		// A new reference arrived during the grace period; don't evict.
		entry.mu.Unlock()
		c.mu.Unlock()
		return
	}

	// Safe to evict
	entry.graceTimer = nil
	tf := entry.tf
	delete(c.entries, key)
	entry.mu.Unlock()
	c.mu.Unlock()

	if c.metrics != nil {
		c.metrics.StreamingCacheOps.WithLabelValues("evict").Inc()
	}
	slog.Debug("file handle cache: evicted",
		"info_hash", key.infoHash,
		"file_path", key.filePath,
	)

	tf.Close()
}

// invalidate removes all entries for a given infoHash, closing their TorrentFiles.
func (c *fileHandleCache) invalidate(infoHash string) {
	c.mu.Lock()

	var toClose []*fileHandleCacheEntry
	var keys []fileHandleKey
	for key, entry := range c.entries {
		if key.infoHash == infoHash {
			keys = append(keys, key)
			toClose = append(toClose, entry)
			delete(c.entries, key)
		}
	}
	c.mu.Unlock()

	for i, entry := range toClose {
		entry.mu.Lock()
		if entry.graceTimer != nil {
			entry.graceTimer.Stop()
			entry.graceTimer = nil
		}
		tf := entry.tf
		entry.mu.Unlock()

		slog.Debug("file handle cache: invalidated",
			"info_hash", keys[i].infoHash,
			"file_path", keys[i].filePath,
		)
		tf.Close()
	}
}

// CachedFileHandle wraps a shared TorrentFile with reference counting.
// Each WebDAV open returns a CachedFileHandle; Close() decrements the
// ref count instead of immediately closing the underlying TorrentFile.
type CachedFileHandle struct {
	tf    *TorrentFile
	key   fileHandleKey
	cache *fileHandleCache

	once sync.Once // Ensures decRef is called exactly once
}

// Ensure CachedFileHandle implements File
var _ File = (*CachedFileHandle)(nil)

func (h *CachedFileHandle) Name() string { return h.tf.Name() }
func (h *CachedFileHandle) IsDir() bool  { return false }
func (h *CachedFileHandle) Size() int64  { return h.tf.Size() }

func (h *CachedFileHandle) Stat() (os.FileInfo, error) {
	return common.NewFileInfo(h.tf.Name(), h.tf.Size(), false, time.Now()), nil
}

func (h *CachedFileHandle) Read(p []byte) (int, error) {
	return h.tf.Read(p)
}

func (h *CachedFileHandle) ReadAt(p []byte, off int64) (int, error) {
	return h.tf.ReadAt(p, off)
}

func (h *CachedFileHandle) Close() error {
	h.once.Do(func() {
		h.cache.decRef(h.key)
	})
	return nil
}
