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

Link library items to torrent files.

**Context**:
- A movie/episode can have one active torrent assignment
- Assignment includes: which torrent, which file within torrent
- Optional quality info for display (resolution, source)

**Table**:
```sql
torrent_assignments (
    item_type,      -- 'movie' or 'episode'
    item_id,        -- references movies.id or episodes.id
    info_hash,      -- torrent identifier
    magnet_uri,     -- for adding torrent
    file_path,      -- path within torrent
    file_size,      -- for VFS file size
    resolution,     -- optional: '1080p', '4K'
    source,         -- optional: 'BluRay', 'WEB-DL'
    is_active       -- only one active per item
)
```

**Tasks**:
1. Create AssignmentRepository
2. CreateAssignment(itemType, itemID, assignment)
3. GetActiveAssignment(itemType, itemID)
4. GetAssignmentsByHash(infoHash) → which library items use this torrent
5. DeactivateAssignment(id)

---

## Step 2.4: Assignment API

Endpoints to assign torrents to library items.

**Context**:
- User finds torrent, gets magnet link
- User tells momoshtrem: "this movie is in this torrent, at this file path"
- System stores assignment, VFS now shows the file

**Endpoints**:
```
POST   /api/movies/:id/assign
DELETE /api/movies/:id/assign

POST   /api/episodes/:id/assign
DELETE /api/episodes/:id/assign
```

**Request format**:
```json
{
    "magnet_uri": "magnet:?xt=urn:btih:...",
    "file_path": "Movie.Name.2024.1080p.BluRay.mkv",
    "resolution": "1080p",
    "source": "BluRay"
}
```

**Tasks**:
1. Implement assignment handlers
2. On assign: validate magnet, add torrent, wait for metadata
3. Verify file_path exists in torrent
4. Get file size from torrent metadata
5. Store assignment
6. On delete: deactivate assignment, optionally remove torrent if no other assignments

**Design considerations**:
- Auto-detect file if torrent has single video file?
- List available files in torrent for user to choose?

---

## Step 2.5: Torrent Management API

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

## Step 2.6: TorrentFile for VFS

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

## Step 2.7: Update LibraryVFS

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

## Step 2.8: Idle Mode (Optional but Recommended)

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

## Step 2.9: Integration Testing

Verify end-to-end streaming.

**Test flow**:
1. Start momoshtrem
2. Add movie: `POST /api/movies {"tmdb_id": 550}`
3. Assign torrent: `POST /api/movies/1/assign {...}`
4. Mount WebDAV in Infuse
5. Navigate to movie
6. Play → should stream (downloading pieces on demand)

**What to verify**:
- File appears with correct name and size
- Playback starts (may buffer initially)
- Seeking works (jumps to different position)
- Idle mode pauses after timeout (check logs/stats)

---

## Suggested Additions After Stage 2

```
momoshtrem/
├── internal/
│   ├── torrent/
│   │   ├── client.go      # anacrolix client setup
│   │   ├── service.go     # torrent lifecycle
│   │   └── activity.go    # idle mode
│   ├── library/
│   │   └── assignment_repo.go
│   ├── api/
│   │   └── torrent.go     # assignment & torrent handlers
│   └── vfs/
│       └── torrent_file.go
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
