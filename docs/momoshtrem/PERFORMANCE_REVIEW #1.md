# Momoshtrem Performance Review

## Overview

Performance analysis of the momoshtrem torrent streaming service. Issues are categorized as **Critical** (direct streaming impact) or **High Priority** (significant overhead).

**Overall assessment**: The streaming architecture is well-designed with proper piece prioritization and format detection. The main bottlenecks are in the read path allocations and database query patterns.

---

## Critical Issues

### 1. Per-Read Memory Allocations

**Location**: `internal/vfs/torrent_file.go:readContext()`

Every streaming read operation allocates:
- A new byte buffer (typically 64KB)
- A new channel for result communication
- A new goroutine for timeout handling

At typical streaming rates (~150 reads/second for 10 Mbps), this creates ~10MB of allocations per second plus goroutine overhead. This pressure causes GC pauses during video playback.

The extra buffer also requires a memory copy operation after each read completes, adding latency.

---

### 2. Lock Contention in Streaming Reader

**Location**: `internal/streaming/reader.go:Read()`

The PriorityReader holds a mutex for the entire duration of read operations. Since torrent reads can block waiting for pieces to download, this lock is held for extended periods.

Effects:
- Format detection goroutine waits to acquire lock
- Seek operations blocked during slow reads
- Multiple readers of the same file serialize unnecessarily

---

### 3. N+1 Database Query Pattern

**Location**: `internal/library/show_repo.go` and `internal/vfs/library_fs.go:rebuildTree()`

The VFS tree rebuild executes queries in nested loops:
1. Query all shows with assigned episodes
2. For each show: query its seasons with assigned episodes
3. For each season: query its episodes with assignments

For a library with 100 shows averaging 7 seasons each, this executes 800+ database queries.

The tree rebuilds every 30 seconds (default TTL), and a write lock is held during the entire operation, blocking all VFS reads.

---

## High Priority Issues

### 4. Full Write Lock During Tree Rebuild

**Location**: `internal/vfs/library_fs.go:rebuildTree()`

The VFS tree rebuild acquires a write lock before starting and holds it through all database queries. All file operations (opens, reads, directory listings) block during this time.

The rebuild could be performed without a lock, with only a brief lock needed to swap the new tree pointer.

---

### 5. Activity Manager Locking During Idle Checks

**Location**: `internal/torrent/activity.go:checkIdleTorrents()`

The idle check (runs every 30 seconds) acquires a full lock and iterates all torrents. During this iteration, `MarkActive()` calls are blocked, which can delay streaming read operations that need to wake an idle torrent.

---

### 6. Unbounded Piece Priority Updates

**Location**: `internal/streaming/prioritizer.go`

Each seek operation triggers individual `SetPriority()` calls for every piece in the readahead range. With 16KB pieces and 16MB readahead, this means ~1,000 individual calls per seek.

Rapid seeking (common during video scrubbing) amplifies this overhead.

---

### 7. Polling for Peer Connection

**Location**: `internal/torrent/activity.go:WaitForActivation()`

When `start_paused` is enabled, the first read waits for peer connections using a polling loop with a 50ms ticker. This creates 20 wakeups per second while waiting.

---

### 8. Missing Database Indexes

**Location**: `internal/library/migrations/`

The `torrent_assignments` table lacks composite indexes for common query patterns:
- `(item_type, item_id, is_active)` - used in most assignment lookups
- `(info_hash, is_active)` - used in GetActiveByInfoHash

---

## Well-Implemented Patterns

The following patterns are already well-optimized:

- **Async format detection**: MP4/MKV detection runs in background without blocking playback
- **Goroutine leak protection**: Bounded to at most one leaked goroutine per file via `pendingRead` drain
- **Piece prioritization**: Header/footer bytes get high priority for fast playback start
- **Activity tracking**: Idle torrents pause network activity to save bandwidth
- **Incremental tree updates**: Methods exist for adding/removing items without full rebuild
- **SQLite WAL mode**: Enables concurrent reads

---

## Quick Wins

1. **Add missing indexes** - Minimal effort, immediate query improvement
2. **Increase tree TTL** - Configuration change to reduce rebuild frequency
3. **Buffer pool** - Add `sync.Pool` for read buffers to reduce allocations

---

## Key Files

| Area | File |
|------|------|
| Read path | `internal/vfs/torrent_file.go` |
| Streaming | `internal/streaming/reader.go` |
| Database queries | `internal/library/show_repo.go` |
| VFS tree | `internal/vfs/library_fs.go` |
| Activity management | `internal/torrent/activity.go` |
