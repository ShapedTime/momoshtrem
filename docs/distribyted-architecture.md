# Distribyted Architecture Deep Dive

This document explains how distribyted works, from adding a magnet URI to streaming via WebDAV.

## Overview

Distribyted is a Go torrent client that exposes torrent contents as files via WebDAV (and optionally FUSE). It enables on-demand streaming - pieces are only downloaded when accessed.

## What is FUSE?

FUSE (Filesystem in Userspace) is a software interface that lets non-privileged users create their own file systems without editing kernel code.

**How it works:**
- Normally, filesystems run in kernel space (privileged)
- FUSE provides a bridge: your program runs in userspace but appears as a real mounted filesystem
- The kernel forwards filesystem operations (read, write, open, etc.) to your FUSE program

**Platform implementations:**
- **Linux**: Native FUSE support (packages: `fuse`, `libfuse-dev`)
- **macOS**: Requires [macFUSE](https://osxfuse.github.io/) (third-party)
- **Windows**: Requires WinFsp or similar

In distribyted's context, FUSE makes torrent contents appear as regular files. However, for AppleTV streaming, WebDAV is used since tvOS doesn't support FUSE.

---

## Complete Flow: Magnet → WebDAV → Streaming

### Phase 1: Adding a Magnet

```
POST /api/routes/:route/torrent  { "magnet": "magnet:?xt=..." }
```

**1. HTTP Handler** (`http/api.go:46-63`)
```go
s.AddMagnet(route, json.Magnet)
```

**2. Service Layer** (`torrent/service.go:94-122`)
```go
func (s *Service) AddMagnet(r, m string) error {
    s.addMagnet(r, m)           // Add to torrent client
    return s.db.AddMagnet(r, m) // Persist to Badger DB
}
```

**3. Torrent Client** (anacrolix/torrent library)
- Parses magnet URI, extracts info hash
- Connects to DHT network and trackers
- Fetches torrent metadata (file list, piece hashes, sizes)
- **Does NOT download pieces yet** - just metadata

**4. Filesystem Registration** (`torrent/service.go:137-173`)
```go
// Wait for metadata with timeout
select {
case <-time.After(timeout):
    // timeout handling
case <-t.GotInfo():
    // metadata received
}

// Register torrent in virtual filesystem
tfs.AddTorrent(t)
```

**5. Persistence** (`torrent/loader/db.go`)
- Badger DB stores: `/route/{infohash}/{routeName}` → magnet URI
- Survives restarts

---

### Phase 2: Virtual Filesystem Layer

The torrent is now exposed as files in a virtual filesystem:

```
┌─────────────────────────────────────────────────────┐
│  ContainerFS (aggregates all routes)                │
│  └── /movies/                                       │
│      └── TorrentFS (per-route)                      │
│          └── Ubuntu.22.04.iso (torrentFile wrapper) │
│          └── Movie.mkv                              │
└─────────────────────────────────────────────────────┘
```

**TorrentFS** (`fs/torrent.go`) wraps each torrent file:

```go
type torrentFile struct {
    readerFunc func() torrent.Reader  // Lazy - created on first read
    reader     reader
    len        int64
    timeout    int
}
```

**Key insight**: No reader exists until someone actually reads the file!

---

### Phase 3: WebDAV Server

**Server startup** (`webdav/http.go`):
```go
srv := &webdav.Handler{
    FileSystem: newFS(containerFS),  // Wraps our virtual FS
    LockSystem: webdav.NewMemLS(),
}
http.ListenAndServe(":4444", srv)
```

**WebDAV adapter** (`webdav/fs.go`) translates WebDAV operations:
- `PROPFIND /movies/` → list directory
- `GET /movies/Movie.mkv` → open file, read bytes

---

### Phase 4: Streaming (The Magic)

When AppleTV requests `GET /movies/Movie.mkv`:

**1. WebDAV opens file**
```go
func (wd *WebDAV) OpenFile(ctx, name, flag, perm) {
    f := wd.lookupFile(path)  // Gets torrentFile wrapper
    return newFile(name, f, ...)
}
```

**2. First read triggers lazy initialization** (`fs/torrent.go:144-197`)
```go
func (d *torrentFile) load() {
    if d.reader == nil {
        d.reader = d.readerFunc()  // Creates anacrolix Reader NOW
    }
}
```

**3. Read with timeout**
```go
func (d *torrentFile) ReadAt(p []byte, off int64) (n int, err error) {
    d.load()
    ctx, cancel := context.WithTimeout(context.Background(), timeout)
    defer cancel()
    return d.reader.ReadContext(ctx, p)  // Blocks until pieces arrive
}
```

**4. anacrolix/torrent handles piece fetching**
- Determines which pieces cover bytes `off` to `off+len(p)`
- **Prioritizes those pieces** over others
- Connects to peers, requests pieces
- Verifies SHA1 hashes against metadata
- Returns data when pieces complete

**5. Piece caching**
- Completed pieces stored in `metadata_folder/cache/`
- BoltDB tracks which pieces are complete
- Future reads of same bytes → instant (no network)

---

## Architecture Diagram

```
┌────────────┐   HTTP POST    ┌─────────────┐
│  Catalog   │───magnet──────▶│   HTTP API  │
│    UI      │                └──────┬──────┘
└────────────┘                       │
                                     ▼
                              ┌─────────────┐
                              │   Service   │──▶ Badger DB (persist)
                              └──────┬──────┘
                                     │
                                     ▼
                              ┌─────────────┐
                              │  anacrolix  │──▶ DHT + Trackers
                              │   torrent   │    (metadata only)
                              └──────┬──────┘
                                     │
                                     ▼
                              ┌─────────────┐
                              │  TorrentFS  │ (virtual file tree)
                              └──────┬──────┘
                                     │
                                     ▼
┌────────────┐   WebDAV GET   ┌─────────────┐
│  AppleTV   │◀──streaming────│   WebDAV    │
│  (VLC/     │   video data   │   Server    │
│  Infuse)   │                └──────┬──────┘
└────────────┘                       │
                                     ▼
                              ┌─────────────┐
                              │ torrentFile │──▶ reader.ReadAt(offset)
                              └──────┬──────┘
                                     │
                                     ▼
                              ┌─────────────┐
                              │  anacrolix  │──▶ Peers (piece fetch)
                              │   Reader    │
                              └──────┬──────┘
                                     │
                                     ▼
                              ┌─────────────┐
                              │ FileCached  │ (disk cache)
                              │  + BoltDB   │
                              └─────────────┘
```

---

## Why This Works for Streaming

1. **On-demand only**: Pieces fetched when read, not upfront
2. **Seek support**: AppleTV can skip ahead; distribyted fetches those pieces
3. **Prioritization**: anacrolix prioritizes pieces being actively read
4. **Caching**: Once a piece is downloaded, it's cached to disk
5. **Timeouts**: Reads fail gracefully if pieces take too long (configurable)

The key insight is that **no actual video data exists locally until AppleTV requests it**. The torrent client acts as a "lazy loader" that materializes bytes on-demand, making the torrent appear as a regular file to WebDAV clients.

---

## Storage Architecture

### Three-Tier Storage Design

```
┌─────────────────────────────────────────┐
│         WebDAV/HTTP/FUSE Access         │
│  (WebDAV Handler, HTTP FS, FUSE Mount)  │
└────────────────┬────────────────────────┘
                 │
┌────────────────▼────────────────────────┐
│    Container FS (Multi-route)            │
│  - Aggregates route filesystems         │
│  - Archive factory support              │
│  - Path-based lookup                    │
└────────────────┬────────────────────────┘
                 │
┌────────────────▼────────────────────────┐
│    Torrent FS (Per-route)                │
│  - Torrent collection per route         │
│  - File extraction from torrent         │
│  - Lazy torrent.Reader wrapping         │
└────────────────┬────────────────────────┘
                 │
┌────────────────▼────────────────────────┐
│   anacrolix/torrent Client               │
│  - Piece fetching & prioritization      │
│  - Peer management                      │
│  - DHT support                          │
└────────────────┬────────────────────────┘
                 │
┌────────────────▼────────────────────────┐
│   Storage Layers (Bottom-Up)             │
│  1. FileCached Storage (primary)        │
│     - `metadata_folder/cache`           │
│  2. Piece Completion (BoltDB)           │
│     - `metadata_folder/piece-completion`│
│  3. Item Store (Badger DB)              │
│     - `metadata_folder/items` (DHT)     │
│  4. Magnet DB (Badger DB)               │
│     - `metadata_folder/magnetdb`        │
└─────────────────────────────────────────┘
```

### Data Persistence Layers

| Layer | Technology | Purpose | Location |
|-------|-----------|---------|----------|
| Piece Cache | FileCached (anacrolix) | Downloaded pieces | `metadata_folder/cache` |
| Piece Completion | BoltDB | Track complete pieces | `metadata_folder/piece-completion` |
| DHT Items | Badger DB | DHT bootstrap | `metadata_folder/items` |
| Magnet DB | Badger DB | Persistent torrents | `metadata_folder/magnetdb` |
| Torrent Metadata | In-memory (anacrolix) | Runtime torrent state | N/A |

---

## Caching Mechanism

### Cache Creation (`cmd/distribyted/main.go:101-107`)

```go
cf := filepath.Join(conf.Torrent.MetadataFolder, "cache")
fc, err := filecache.NewCache(cf)
st := storage.NewResourcePieces(fc.AsResourceProvider())
```

### Capacity Setting (`cmd/distribyted/main.go:195-196`)

```go
log.Info().Msg(fmt.Sprintf("setting cache size to %d MB", conf.Torrent.GlobalCacheSize))
fc.SetCapacity(conf.Torrent.GlobalCacheSize * 1024 * 1024)
```

### Automatic Eviction

The `anacrolix/missinggo/v2/filecache` library handles cache eviction automatically:

| Aspect | Details |
|--------|---------|
| **Policy** | **LRU (Least Recently Used)** |
| **Trigger** | When cache exceeds `global_cache_size` |
| **Method** | `TrimToCapacity()` called automatically |
| **Tracks** | Access time per cached item |
| **Evicts** | Oldest accessed pieces first |

### Configuration

In `config.yaml` (`config/model.go:27`):
```yaml
torrent:
  global_cache_size: 2048  # MB (default: 2GB)
  metadata_folder: ./distribyted-data
```

### Cache Lifecycle

```
┌─────────────────────────────────────────────────────────┐
│                    Cache Lifecycle                       │
├─────────────────────────────────────────────────────────┤
│  1. WebDAV read request for offset X                    │
│  2. Torrent client fetches pieces covering X            │
│  3. Pieces written to cache: metadata_folder/cache/     │
│  4. Access time updated for those pieces (LRU tracking) │
│  5. If cache > global_cache_size:                       │
│     └─> TrimToCapacity() evicts LRU pieces              │
│  6. Future reads of same bytes → served from cache      │
└─────────────────────────────────────────────────────────┘
```

### What Gets Cached

| Storage | Location | Purpose |
|---------|----------|---------|
| **Piece data** | `metadata_folder/cache/` | Actual torrent content (video bytes) |
| **Piece completion** | `metadata_folder/piece-completion/` (BoltDB) | Tracks which pieces are complete |
| **Magnets** | `metadata_folder/magnetdb/` (Badger) | Persistent torrent list |

### Key Insight

The cache is **piece-level**, not file-level. A 50GB movie has thousands of pieces (~16KB-4MB each). When you seek to a new position:
- New pieces fetched and cached
- If cache is full, oldest-accessed pieces evicted
- Pieces you've recently watched stay cached

This means if you rewatch a scene, it streams instantly from disk. But if you watch a 50GB movie with a 2GB cache, only the last ~2GB of content stays cached.

---

## Archive Transparency

Distribyted transparently extracts archives on-the-fly (`fs/archive.go`):

```go
var SupportedFactories = map[string]FsFactory{
    ".zip": func(f File) (Filesystem, error) {
        return NewArchive(f, f.Size(), &Zip{}), nil
    },
    ".rar": func(f File) (Filesystem, error) {
        return NewArchive(f, f.Size(), &Rar{}), nil
    },
    ".7z": func(f File) (Filesystem, error) {
        return NewArchive(f, f.Size(), &SevenZip{}), nil
    },
}
```

- Uses `io.TeeReader` to buffer archive data while reading
- Lazy-loads archive metadata on first access
- Implements efficient streaming decompression via `DiskTeeReader`

---

## Configuration Reference

### Critical Settings (`config.yaml`)

```yaml
torrent:
  global_cache_size: 2048    # MB - Total piece cache
  metadata_folder: path      # Storage location
  add_timeout: 60            # Seconds to wait for metadata
  read_timeout: 120          # Read operation timeout (seconds)
  continue_when_add_timeout: false  # Skip timeout errors

webdav:
  port: 36911
  user: username
  pass: password

routes:
  - name: movies
    torrents:
      - magnet_uri: "magnet:?xt=..."
```

---

## Complete Data Flow Summary

```
1. ADD PHASE
   POST /api/routes/movies/torrent { magnet: "..." }
   └─> Service.AddMagnet()
       ├─> Torrent.Client.AddMagnet()  [starts piece fetching engine]
       ├─> Torrent.Service.addTorrent()  [waits for metadata]
       ├─> Stats.Add()  [tracks torrent]
       └─> TorrentFS.AddTorrent()  [registers in filesystem]
       └─> DB.AddMagnet()  [persists to Badger]

2. STREAM PHASE
   WebDAV GET /movies/torrent_name/file.mkv
   └─> WebDAV.OpenFile()
       └─> ContainerFS.Open()
           └─> TorrentFS.Open()
               └─> torrentFile.load()
                   └─> torrent.File.NewReader()  [anacrolix reader]

   WebDAV READ operation
   └─> webDAVFile.Read() at offset
       └─> torrentFile.ReadAt(offset)
           └─> readAtWrapper.ReadAt()
               └─> torrent.Reader.ReadContext()
                   └─> anacrolix pieces fetch on-demand
                       └─> Storage.GetPiece()
                           └─> FileCached.Get()
                               └─> BoltDB.GetPieceCompletion()

3. ARCHIVE TRANSPARENCY
   IF file is .zip/.rar/.7z
   └─> Storage factory creates Archive FS
       └─> Archive.loadOnce()
           └─> DiskTeeReader buffers to temp
               └─> Stream decompression
                   └─> Individual archive file reading
```

---

## Key Source Files

| File | Purpose |
|------|---------|
| `cmd/distribyted/main.go` | Application entry, cache setup |
| `http/api.go` | REST API handlers |
| `torrent/service.go` | Torrent lifecycle management |
| `torrent/loader/db.go` | Badger DB persistence |
| `fs/torrent.go` | Virtual filesystem for torrents |
| `fs/container.go` | Multi-route filesystem aggregation |
| `fs/archive.go` | Archive extraction support |
| `webdav/http.go` | WebDAV server setup |
| `webdav/fs.go` | WebDAV filesystem adapter |
| `config/model.go` | Configuration structures |
