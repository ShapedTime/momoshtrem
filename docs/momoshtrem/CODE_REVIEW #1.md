# momoshtrem Code Review Findings

Code quality review of the momoshtrem Go service covering SOLID principles, DRY, KISS, concurrency safety, and error handling.

**Scope:** `internal/` packages — API, VFS, torrent, library, streaming

---

## Critical

### 1. Goroutine Leak in Torrent File Reader

**File:** `internal/vfs/torrent_file.go` — `readContext()`

When a read times out, the function returns `ctx.Err()` to the caller, but the goroutine spawned for `f.reader.Read(p)` continues to run indefinitely. The underlying torrent reader has no cancellation mechanism, so the goroutine blocks forever on slow or stalled torrents.

The channel is buffered (size 1), which prevents the goroutine from blocking on its write after timeout — but the goroutine still holds a reference to the reader and the shared buffer `p`, which the caller may have already reused or freed.

Under sustained load with slow peers, goroutines accumulate without bound, leading to memory exhaustion and eventual OOM.

```go
// torrent_file.go:210-228
func (f *TorrentFile) readContext(ctx context.Context, p []byte) (int, error) {
    done := make(chan result, 1)
    go func() {
        n, err := f.reader.Read(p)  // blocks forever if torrent stalls
        done <- result{n, err}       // goroutine never exits
    }()
    select {
    case r := <-done:
        return r.n, r.err
    case <-ctx.Done():
        return 0, ctx.Err()  // caller returns, goroutine orphaned
    }
}
```

**Additional concern:** After timeout, the goroutine still writes to `p` (the caller's buffer). If the caller reuses or discards the buffer, this is a data race.

---

### 2. Non-Atomic Multi-Step Database Writes

**File:** `internal/library/assignment_repo.go` — `Create()`

The `Create` method performs two SQL operations (deactivate existing + insert new) without a transaction. If the process crashes or the second query fails, the item's active assignment is deactivated but no new one exists, making the item disappear from the VFS with no way to recover except manually re-assigning.

```go
// assignment_repo.go:45-74
func (r *AssignmentRepository) Create(assignment *TorrentAssignment) error {
    // Step 1: Deactivate — succeeds
    _, err := r.db.Exec(`UPDATE ... SET is_active = FALSE ...`)

    // If crash/error occurs here: old assignment deactivated, new one never created

    // Step 2: Insert new
    result, err := r.db.Exec(`INSERT INTO torrent_assignments ...`)
}
```

The same pattern exists in `deleteShow()` handler (`library.go:496-534`), which deactivates episode assignments individually in a loop before deleting the show. A failure mid-loop leaves partial deactivations.

---

## High

### 3. Errors Swallowed — Clients Receive Incorrect Data

**File:** `internal/api/library.go`

Multiple handlers log assignment lookup errors but continue with a nil assignment, returning responses that show no torrent assignment even when one exists. The client has no way to know the data is incomplete.

**Occurrences:**

| Handler | Lines | Effect |
|---------|-------|--------|
| `listMovies` | 139-142 | Movie appears unassigned in list |
| `getMovie` | 204-207 | Movie detail shows no assignment |
| `deleteMovie` | 219-221 | VFS tree not updated (movie stays visible in WebDAV) |
| `getShow` | 486-488 | Episodes appear unassigned |
| `deleteShow` | 503-506 | VFS tree not cleaned up |
| `unassignEpisodeTorrent` | 728 | Error discarded with blank identifier |

```go
// library.go:139-142
assignment, err := s.assignmentRepo.GetActiveForItem(library.ItemTypeMovie, movie.ID)
if err != nil {
    slog.Error("Failed to get assignment for movie", "movie_id", movie.ID, "error", err)
    // continues — client receives { "has_assignment": false } even if assignment exists
}
```

This is distinct from the common "log and continue" pattern because the response data becomes silently wrong, not just degraded.

---

### 4. Thick Handler Violates Single Responsibility

**File:** `internal/api/library.go` — `assignShowTorrent()` (lines 539-717)

This 179-line handler performs six distinct responsibilities that belong in a service layer:

1. **Validation** — show existence, magnet URI parsing
2. **Torrent interaction** — adding torrent, waiting for metadata
3. **Episode identification** — running the identifier, matching to library
4. **Database writes** — creating assignments in a loop
5. **Subtitle processing** — detecting and storing torrent-embedded subtitles
6. **VFS mutation** — updating the in-memory directory tree

Per the project's own CODE_QUALITY.md: "API routes are thin orchestration; business logic lives in `lib/api/`." This handler is not thin orchestration — it is the business logic.

`createShow()` (lines 374-463) has the same problem at 90 lines, performing TMDB fetch, season/episode creation in nested loops, and error handling that silently skips failed seasons.

---

### 5. Retry Workaround Masks Architectural Race Condition

**File:** `internal/vfs/torrent_file.go` — `firstReadWithRetry()` (lines 161-180)

When `start_paused` is enabled, a torrent registered with the `ActivityManager` begins idle (network disabled). The first `Read()` call triggers `markActivity()`, which calls `ActivityManager.MarkActive()` to re-enable the network. But the torrent needs time to connect to peers and start downloading — the very next read (immediately after) will timeout because no data is available yet.

Rather than fixing this at the `ActivityManager` level (e.g., having `MarkActive` return only after data is flowing, or using a readiness signal), the code adds retry logic with exponential backoff:

```go
func (f *TorrentFile) firstReadWithRetry(p []byte) (int, error) {
    const maxRetries = 3
    for attempt := 0; attempt < maxRetries; attempt++ {
        n, err := f.readWithTimeout(p)
        if err == nil || err == io.EOF {
            return n, err
        }
        time.Sleep(time.Duration(attempt+1) * 100 * time.Millisecond)
    }
    return 0, lastErr
}
```

This adds up to 600ms of blocking sleep in the worst case, spawns up to 3 potentially leaked goroutines (per issue #1), and will still fail if the torrent takes longer than the combined retry window to wake up.

---

### 6. Repositories Are Concrete Types — Untestable

**File:** `internal/api/router.go` — `Server` struct

All three repositories are wired as concrete pointer types:

```go
type Server struct {
    movieRepo       *library.MovieRepository
    showRepo        *library.ShowRepository
    assignmentRepo  *library.AssignmentRepository
    // ...
}
```

This means:
- Unit testing handlers requires a real SQLite database
- Swapping storage backends requires changing every consumer
- Violates the Dependency Inversion Principle (high-level module depends on low-level implementation)

The codebase already defines interfaces correctly for `torrent.Service` and `vfs.TreeUpdater`, showing the pattern is understood. The repositories are the gap.

---

### 7. `GetStats()` Returns Untyped Map

**File:** `internal/torrent/activity.go` — `GetStats()` (lines 190-211)

Returns `map[string]interface{}`, losing all type safety:

```go
func (am *ActivityManager) GetStats() map[string]interface{} {
    return map[string]interface{}{
        "idle_timeout_seconds": am.idleTimeout.Seconds(),
        "active_torrents":      active,
        "idle_torrents":        idle,
        "total_torrents":       len(am.torrents),
        "start_paused":         am.startPaused,
    }
}
```

Callers must use type assertions or pass the map directly to JSON serialization, which is fragile (key typos, type mismatches). A typed struct should be returned instead.

---

### 8. String-Based Error Matching Instead of Typed Errors

**File:** `internal/api/subtitles.go` — `deleteSubtitle()` (line 188)

```go
if strings.Contains(err.Error(), "not found") {
    errorResponse(c, http.StatusNotFound, "Subtitle not found")
    return
}
```

This breaks if the error message wording changes and couples the handler to string internals of the subtitle service. The `torrent` package already demonstrates the correct pattern with sentinel errors (`ErrTorrentNotFound`, etc.). The subtitle package should follow the same approach.

---

### 9. No Database Query Timeouts

**File:** `internal/library/*.go` — all repository methods

All database calls use no context or timeout:

```go
row := r.db.QueryRow(`SELECT ... FROM movies WHERE id = ?`, id)
```

SQLite with WAL mode can still block on write contention. Without timeouts, a stuck database lock blocks the calling goroutine (and its HTTP request) indefinitely. The `database/sql` package supports `QueryRowContext`, `ExecContext`, etc., which accept a context with deadline.

---

### 10. `library_fs.go` Is 900 Lines With Mixed Concerns

**File:** `internal/vfs/library_fs.go`

This single file contains:

- **5 struct types** with full implementations: `LibraryFS`, `VirtualDir`, `DirFile`, `PlaceholderFile`, `SubtitleFile`, `TorrentSubtitleFile`
- **Tree cache management** — TTL, invalidation, rebuild
- **Tree mutation methods** — `AddMovieToTree`, `RemoveMovieFromTree`, `AddEpisodesToTree`, `RemoveEpisodeFromTree`, `RemoveShowFromTree`
- **File open/streaming logic** — `openTorrentFile`, `openTorrentSubtitleFile`
- **Path/name generation helpers** — `makeMediaFolderName`, `makeEpisodeFileName`, etc.
- **Subtitle integration** — `addSubtitlesToDir`

For comparison, the torrent service is well-separated with `service.go` (interface), `service_impl.go` (implementation), `activity.go` (activity tracking), and `client.go` (client setup) — all under 250 lines each.

---

## Medium (Summary)

| # | Issue | Location | Problem |
|---|-------|----------|---------|
| 11 | CORS allows all origins | `router.go:86-97` | `Access-Control-Allow-Origin: *` is hardcoded, not configurable |
| 12 | No pagination on list endpoints | `library.go:129-147`, `354-372` | `listMovies` and `listShows` return all records with no limit |
| 13 | `deleteData` parameter ignored | `service_impl.go:180` | `RemoveTorrent` accepts `deleteData bool` but never uses it |
| 14 | `Pause`/`Resume` bypass `ActivityManager` | `service_impl.go:214-245` | Directly toggle torrent state without updating `ActivityManager.state`, so the manager may override the user's pause |
| 15 | `entryToFile` returns nil for `TorrentSubtitleFile` | `library_fs.go:893-896` | `ReadDir` on a directory containing torrent subtitles returns nil entries in the map, which will cause nil pointer panics in WebDAV |

---

## What the Codebase Does Well

- Clean dependency injection via constructors — no global state
- Well-defined service interface for torrent operations (`torrent.Service`)
- Consistent error wrapping with `fmt.Errorf("...: %w", err)`
- Graceful degradation for optional services (subtitles, TMDB, air date sync)
- Proper `sync.RWMutex` usage with consistent lock/unlock patterns
- Background goroutines with clean stop channel pattern (`ActivityManager`)
- Compile-time interface verification (`var _ Service = (*service)(nil)`)
