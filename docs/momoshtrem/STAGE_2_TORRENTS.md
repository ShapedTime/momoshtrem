# Stage 2: Torrent Integration

**Goal**: Connect torrents to library items. Basic streaming works.

**Prerequisite**: Stage 1 complete (library management, WebDAV structure)

**End state**: Assign magnet to movie → file appears in VFS → streaming works in Infuse

---

## Step 2.1: Torrent Client Setup

Integrate anacrolix/torrent.

**Context**:
- anacrolix/torrent is the same library distribyted uses
- Handles BitTorrent protocol, piece management, peer connections
- We configure it, it handles the heavy lifting

**Key configuration**:
- Storage backend (filecache for pieces)
- Piece completion tracking (BoltDB)
- Network settings (DHT, ports, etc.)

**Tasks**:
1. Add anacrolix/torrent dependency
2. Create torrent client initialization
3. Configure storage (piece cache location, max size)
4. Configure piece completion database
5. Configure network (DHT enabled, TCP/UTP)

**Reference**: See `distribyted/torrent/client.go` and `distribyted/cmd/distribyted/main.go` (lines 101-137)

**Configuration options to expose**:
```yaml
torrent:
  metadata_folder: ./data/torrents
  global_cache_size: 4096  # MB
  add_timeout: 60          # seconds
  read_timeout: 120
```

---

## Step 2.2: Torrent Service

Manage torrent lifecycle.

**Context**:
- Service layer between API/VFS and raw torrent client
- Handles add, remove, get operations
- Tracks which torrents are loaded

**Tasks**:
1. Create TorrentService struct wrapping client
2. AddTorrent(infoHash, magnetURI) → waits for metadata, returns torrent
3. GetTorrent(infoHash) → returns existing or nil
4. GetOrAddTorrent(infoHash, magnetURI) → get if exists, add if not
5. RemoveTorrent(infoHash)
6. ListTorrents() → active torrents with status

**Design considerations**:
- Thread safety (multiple file opens can request same torrent)
- Timeout handling (what if metadata never arrives?)
- Memory management (keep references to prevent GC?)

**Reference**: See `distribyted/torrent/service.go`

---

## Step 2.3: Assignment Repository

**Note**: Already implemented in Stage 1.

The `AssignmentRepository` is already in place with:
- `Create()` - creates assignment, auto-deactivates previous
- `GetActiveForItem()` - gets active assignment for movie/episode
- `GetByInfoHash()` - finds all items using a torrent
- `DeactivateForItem()` - removes assignment
- `ListDistinctTorrents()` - lists unique torrent hashes

---

## Step 2.4: Identification Engine

> **Superseded (#5).** The first-match regex identifier described in the original Stage 2 plan (`patterns.go`, `identifier.go`, `matcher.go`) has been replaced by the rules classifier in `momoshtrem/internal/identify/`. Read the package doc and `Classify` there, and `testdata/corpus/` for what it must get right.

The classifier maps every file of a torrent to the show's episodes, with a confidence and a short reason:
- **Tokenizer** (`internal/identify/token/`): episode markers, quality tags, groups, years and dates, so resolutions, years and codecs are never episode numbers.
- **Readings**, weighed together: aired `S01E01`/`1x01` (with ranges and multi-episode files), absolute numbers (anime), air dates (daily shows), season remaps for release seasons TMDB doesn't have, specials (season 0) and episode names.
- **Torrent context**: folder seasons, ranges in names, and the show's title, which is never read as a marker.
- **Adapter**: `internal/service/assignment_catalog.go` builds the classifier's catalog from a library show.

**Key files**:
- `internal/identify/classify.go` - `Classify(Input) Result`
- `internal/identify/match.go` - Readings, weights and confidence
- `internal/identify/resolver.go` - `Resolver` interface for the AI resolver, and `MergeAI`
- `internal/identify/movie.go` - `RankMovieFiles`

---

## Step 2.5: Assignment API (Auto-Detection)

> **Superseded (#5).** Assigning no longer waits for the torrent: it starts a mapping job and answers `202 {job_id}`. The job classifies the files, waits for review when it's unsure, and applies (momoshtrem README, "Episode mapping"; PLAN §6 "Assignment"). The responses below are what the legacy `?wait=true` still answers, plus `job_id`, until H2.

**Note**: API handlers already implemented in Stage 1. Needs torrent service implementation.

The assignment API uses auto-detection instead of requiring manual file paths.

**Endpoints**:
```
POST   /api/movies/:id/assign-torrent    # Auto-detect movie file
DELETE /api/movies/:id/assign            # Unassign movie

POST   /api/shows/:id/assign-torrent     # Auto-detect episodes
DELETE /api/episodes/:id/assign          # Unassign episode
```

**Request format** (just magnet URI):
```json
{
    "magnet_uri": "magnet:?xt=urn:btih:..."
}
```

**Movie response**:
```json
{
    "success": true,
    "assignment": {
        "id": 1,
        "info_hash": "abc123...",
        "file_path": "Movie.2024.1080p.BluRay.mkv",
        "file_size": 5000000000,
        "resolution": "1080p",
        "source": "BluRay"
    }
}
```

**Show response**:
```json
{
    "success": true,
    "summary": {
        "total_files": 15,
        "matched": 12,
        "unmatched": 3,
        "skipped": 0
    },
    "matched": [
        {
            "episode_id": 101,
            "season": 1,
            "episode": 1,
            "file_path": "Show.S01E01.mkv",
            "file_size": 1000000000,
            "resolution": "1080p",
            "confidence": "high"
        }
    ],
    "unmatched": [
        {
            "file_path": "Show.S01E13.mkv",
            "reason": "no_library_episode",
            "season": 1,
            "episode": 13
        }
    ]
}
```

**Flow**:
1. PrettyTVCatalog sends magnet URI
2. momoshtrem adds torrent, waits for metadata
3. Identification engine parses filenames
4. Matcher maps files to library episodes
5. Assignments created for matched episodes
6. Unmatched files logged and returned in response

**Remaining tasks for Stage 2**:
1. Implement `torrent.Service` using anacrolix/torrent
2. Wire up service in main.go
3. Currently returns 503 "Torrent service not available"

---

## Step 2.6: Torrent Management API

> **Superseded (#96).** `DELETE /api/torrents/:hash?delete_data=true|false` cancels the torrent's jobs, deactivates its assignments (keeping them as history), deletes its torrent subtitles and drops it, with its data when asked. Unassigning a file and deleting a title release torrents nothing else uses. Since #98, `GET /api/torrents` answers `{items}`: every torrent the library knows, loaded or not, with its titles and speeds, beside `/torrents/summary` and sticky pauses. See PLAN §6 "Torrents and activity (B6)".

Direct torrent operations.

**Endpoints**:
```
GET    /api/torrents          # List active torrents
GET    /api/torrents/:hash    # Get torrent status (peers, progress, etc.)
DELETE /api/torrents/:hash    # Remove torrent
```

**Tasks**:
1. List handler: return all loaded torrents with basic info
2. Status handler: return detailed stats (connected peers, seeders, piece completion)
3. Delete handler: remove torrent if no active assignments, or force remove

---

## Step 2.7: TorrentFile for VFS

Bridge between VFS and torrent streaming.

**Context**:
- When VFS.Open() is called, return a TorrentFile
- TorrentFile wraps torrent.File with reading capabilities
- Supports Read(), ReadAt(), Seek() for streaming

**Tasks**:
1. Create TorrentFile struct
2. Implement Open(torrentService) → initializes reader
3. Implement io.Reader (sequential read)
4. Implement io.ReaderAt (random access for seeking)
5. Implement io.Closer

**Reference**: See `distribyted/fs/torrent.go` (torrentFile struct, ~lines 345-414)

**Key considerations**:
- Lazy initialization (don't load torrent until file is opened)
- Timeout on reads (don't hang forever if pieces unavailable)
- Thread safety for concurrent reads

---

## Step 2.8: Update LibraryVFS

Connect to torrent backend.

**Context**:
- Stage 1 VFS returns structure only
- Now Open() returns TorrentFile when assignment exists
- ReadDir() still shows items, but now they're backed by real files

**Tasks**:
1. Inject TorrentService into LibraryVFS
2. Update tree builder to fetch assignments
3. In Open(): look up assignment → create TorrentFile
4. Update file size from assignment (not placeholder)

**Flow**:
```
VFS.Open("/Movies/Fight Club (1999)/Fight Club (1999).mkv")
  → lookup in tree → found TorrentFile entry
  → get assignment: {hash: "abc123", file_path: "Fight.Club.mkv"}
  → torrentService.GetOrAddTorrent("abc123", magnet)
  → create reader for "Fight.Club.mkv"
  → return TorrentFile wrapper
```

---

## Step 2.9: Idle Mode (Optional but Recommended)

Pause torrents when not streaming.

**Context**:
- Torrents consume bandwidth even when not being watched
- Idle mode: pause downloads after N seconds of no file access
- Resume when file is accessed again

**Reference**: See `distribyted/torrent/activity.go`

**Tasks**:
1. Create ActivityManager
2. Track last access time per torrent
3. MarkActive(hash) → called on file read
4. Background goroutine checks every 30s
5. If idle > timeout: pause torrent (DisallowDataDownload/Upload)
6. On next access: resume (AllowDataDownload/Upload)

**Configuration**:
```yaml
torrent:
  idle_enabled: true
  idle_timeout: 300  # 5 minutes
  start_paused: true # don't download until accessed
```

---

## Step 2.10: Integration Testing

Verify end-to-end streaming.

**Test flow**:
1. Start momoshtrem
2. Add movie: `POST /api/movies {"tmdb_id": 550}`
3. Assign torrent: `POST /api/movies/1/assign-torrent {"magnet_uri": "..."}`
4. Mount WebDAV in Infuse
5. Navigate to movie
6. Play → should stream (downloading pieces on demand)

**Show test flow**:
1. Add show: `POST /api/shows {"tmdb_id": 1396}` (Breaking Bad)
2. Assign torrent: `POST /api/shows/1/assign-torrent {"magnet_uri": "..."}` → `202 {job_id}`
3. Follow the job: `GET /api/assignment-jobs/{job_id}`, or review it at `/mapping/{job_id}` in the web app
4. Mount WebDAV, navigate to show
5. Play episode → should stream

**What to verify**:
- File appears with correct name and size
- Playback starts (may buffer initially)
- Seeking works (jumps to different position)
- Idle mode pauses after timeout (check logs/stats)

---

## Project Structure After Stage 2

```
momoshtrem/
├── internal/
│   ├── identify/           # Rules classifier (#5; replaced the Stage 1 regex identifier)
│   │   ├── token/          # Tokenizer
│   │   ├── classify.go     # Classify(Input) Result
│   │   └── ...             # candidates, context, remap, match, dedupe, movie, resolver
│   ├── torrent/
│   │   ├── service.go      # Interface (Stage 1), implementation (Stage 2)
│   │   ├── client.go       # anacrolix client setup (Stage 2)
│   │   └── activity.go     # idle mode (Stage 2)
│   ├── library/
│   │   └── assignment_repo.go  # Already implemented in Stage 1
│   ├── api/
│   │   ├── library.go      # Updated with assign-torrent endpoints
│   │   └── router.go       # Updated routes
│   └── vfs/
│       └── torrent_file.go # Stage 2
└── ...
```

---

## What Works After Stage 2

- ✅ Add movies/shows to library
- ✅ Assign torrents to library items
- ✅ Browse and play via WebDAV
- ✅ On-demand piece downloading
- ✅ Idle mode saves bandwidth
- ❌ Playback may buffer (no optimization yet)
- ❌ Seeking may be slow (no piece prioritization)

---

## Next: Stage 3

Streaming works but may not be smooth. Proceed to [Stage 3: Streaming Optimization](./STAGE_3_STREAMING.md) to add piece prioritization.
