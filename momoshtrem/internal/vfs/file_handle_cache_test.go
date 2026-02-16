package vfs

import (
	"sync"
	"testing"
	"time"
)

// mockTorrentFile creates a minimal TorrentFile for testing cache behavior.
// Only the close tracking matters — read/seek are not tested here.
func mockTorrentFile(closed *bool) *TorrentFile {
	return &TorrentFile{
		name:        "test.mkv",
		hash:        "abc123",
		readTimeout: 30 * time.Second,
	}
}

func TestCacheHitReturnsSameFile(t *testing.T) {
	cache := newFileHandleCache(5 * time.Second)
	key := fileHandleKey{infoHash: "abc123", filePath: "movie.mkv"}

	tf := &TorrentFile{
		name:        "test.mkv",
		hash:        "abc123",
		readTimeout: 30 * time.Second,
	}

	// Put creates with refCount=1
	h1 := cache.put(key, tf)
	if h1 == nil {
		t.Fatal("put returned nil")
	}

	// getOrCreate should return same underlying TorrentFile
	h2 := cache.getOrCreate(key)
	if h2 == nil {
		t.Fatal("getOrCreate returned nil on cache hit")
	}
	if h2.tf != h1.tf {
		t.Error("cache hit returned different TorrentFile")
	}

	h1.Close()
	h2.Close()
}

func TestCacheMissReturnsNil(t *testing.T) {
	cache := newFileHandleCache(5 * time.Second)
	key := fileHandleKey{infoHash: "abc123", filePath: "movie.mkv"}

	h := cache.getOrCreate(key)
	if h != nil {
		t.Error("expected nil on cache miss")
	}
}

func TestGracePeriodKeepsEntry(t *testing.T) {
	cache := newFileHandleCache(200 * time.Millisecond)
	key := fileHandleKey{infoHash: "abc123", filePath: "movie.mkv"}

	tf := &TorrentFile{
		name:        "test.mkv",
		hash:        "abc123",
		readTimeout: 30 * time.Second,
	}

	h1 := cache.put(key, tf)
	h1.Close() // refCount drops to 0, starts grace timer

	// Within grace period, entry should still be retrievable
	time.Sleep(50 * time.Millisecond)
	h2 := cache.getOrCreate(key)
	if h2 == nil {
		t.Fatal("entry should still be cached during grace period")
	}
	if h2.tf != tf {
		t.Error("grace period returned different TorrentFile")
	}
	h2.Close()
}

func TestGracePeriodEvicts(t *testing.T) {
	cache := newFileHandleCache(50 * time.Millisecond)
	key := fileHandleKey{infoHash: "abc123", filePath: "movie.mkv"}

	tf := &TorrentFile{
		name:        "test.mkv",
		hash:        "abc123",
		readTimeout: 30 * time.Second,
	}

	h1 := cache.put(key, tf)
	h1.Close() // starts grace timer

	// Wait for grace period to expire
	time.Sleep(150 * time.Millisecond)

	h2 := cache.getOrCreate(key)
	if h2 != nil {
		t.Error("entry should have been evicted after grace period")
		h2.Close()
	}
}

func TestInvalidateRemovesEntries(t *testing.T) {
	cache := newFileHandleCache(5 * time.Second)
	hash := "abc123"

	key1 := fileHandleKey{infoHash: hash, filePath: "movie.mkv"}
	key2 := fileHandleKey{infoHash: hash, filePath: "subs.srt"}
	key3 := fileHandleKey{infoHash: "other", filePath: "movie.mkv"}

	tf1 := &TorrentFile{name: "m.mkv", hash: hash, readTimeout: 30 * time.Second}
	tf2 := &TorrentFile{name: "s.srt", hash: hash, readTimeout: 30 * time.Second}
	tf3 := &TorrentFile{name: "o.mkv", hash: "other", readTimeout: 30 * time.Second}

	cache.put(key1, tf1)
	cache.put(key2, tf2)
	h3 := cache.put(key3, tf3)

	cache.invalidate(hash)

	// Entries for hash should be gone
	if h := cache.getOrCreate(key1); h != nil {
		t.Error("key1 should be invalidated")
		h.Close()
	}
	if h := cache.getOrCreate(key2); h != nil {
		t.Error("key2 should be invalidated")
		h.Close()
	}

	// Entry for different hash should remain
	if h := cache.getOrCreate(key3); h == nil {
		t.Error("key3 should still be cached (different hash)")
	} else {
		h.Close()
	}

	h3.Close()
}

func TestPutRaceResolvesToSingleEntry(t *testing.T) {
	cache := newFileHandleCache(5 * time.Second)
	key := fileHandleKey{infoHash: "abc123", filePath: "movie.mkv"}

	tf1 := &TorrentFile{name: "test.mkv", hash: "abc123", readTimeout: 30 * time.Second}
	tf2 := &TorrentFile{name: "test.mkv", hash: "abc123", readTimeout: 30 * time.Second}

	// First put succeeds
	h1 := cache.put(key, tf1)

	// Second put should detect existing and return it
	h2 := cache.put(key, tf2)

	if h1.tf != h2.tf {
		t.Error("race put should return the same underlying TorrentFile")
	}

	// Both should reference the first TorrentFile
	if h2.tf != tf1 {
		t.Error("race put should keep the first TorrentFile")
	}

	h1.Close()
	h2.Close()
}

func TestDoubleCloseIsSafe(t *testing.T) {
	cache := newFileHandleCache(5 * time.Second)
	key := fileHandleKey{infoHash: "abc123", filePath: "movie.mkv"}

	tf := &TorrentFile{name: "test.mkv", hash: "abc123", readTimeout: 30 * time.Second}
	h := cache.put(key, tf)

	// Close twice should not panic (sync.Once protects)
	h.Close()
	h.Close()
}

func TestConcurrentAccess(t *testing.T) {
	cache := newFileHandleCache(100 * time.Millisecond)
	key := fileHandleKey{infoHash: "abc123", filePath: "movie.mkv"}

	tf := &TorrentFile{name: "test.mkv", hash: "abc123", readTimeout: 30 * time.Second}
	initial := cache.put(key, tf)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			h := cache.getOrCreate(key)
			if h != nil {
				time.Sleep(10 * time.Millisecond)
				h.Close()
			}
		}()
	}

	wg.Wait()
	initial.Close()
}
