# Stage 1: Foundation

**Goal**: Library management with WebDAV structure. No streaming yet - just the skeleton.

**End state**: Add movies/shows via API → see folder structure in Infuse (empty until torrents assigned)

---

## Step 1.1: Project Setup

Initialize Go module and establish project structure.

**Context**:
- Use standard Go project layout (`cmd/`, `internal/`, `pkg/`)
- Consider using Gin for REST API (fast, simple)
- Configuration via YAML (viper or similar)

**Tasks**:
1. Initialize Go module: `momoshtrem`
2. Create directory structure
3. Set up configuration loading (ports, database path, etc.)
4. Create main.go with basic startup

**Decision points**:
- HTTP framework choice (Gin, Echo, Chi, stdlib)
- Configuration library (viper, env, yaml direct)
- Logging approach (zerolog, zap, slog)

---

## Step 1.2: Database Layer

SQLite schema and repository pattern.

**Context**:
- Schema is intentionally minimal - only what's needed for VFS paths
- TMDB metadata (posters, descriptions) fetched on-demand, not stored
- Use migrations for schema versioning

**Schema** (core tables only):

```sql
movies (id, tmdb_id, title, year, created_at)
shows (id, tmdb_id, title, year, created_at)
seasons (id, show_id, season_number)
episodes (id, season_id, episode_number, name)
torrent_assignments (id, item_type, item_id, info_hash, magnet_uri, file_path, file_size, resolution, source, is_active)
```

**Tasks**:
1. Choose SQLite driver (modernc.org/sqlite is pure Go, mattn/go-sqlite3 requires CGO)
2. Create migration system (golang-migrate or simple embedded SQL)
3. Define domain models (keep minimal)
4. Implement repository layer with basic CRUD

**Decision points**:
- Pure Go SQLite vs CGO-based (portability vs maturity)
- ORM vs raw SQL (sqlx is a good middle ground)
- Transaction handling patterns

---

## Step 1.3: Library Repositories

CRUD operations for movies, shows, seasons, episodes.

**Context**:
- Repository pattern isolates database access
- For shows: adding a show should create seasons/episodes structure (from TMDB)
- Consider how to handle "partial" shows (user only wants specific seasons)

**Tasks**:
1. MovieRepository: Create, Get, List, Delete
2. ShowRepository: Create (with seasons/episodes), Get, List, Delete
3. SeasonRepository: Basic CRUD
4. EpisodeRepository: Basic CRUD
5. AssignmentRepository: Create, GetForItem, Delete (Stage 2 uses this heavily)

**Design considerations**:
- Should adding a show auto-fetch all seasons/episodes from TMDB?
- How to handle show updates (new episodes aired)?
- Soft delete vs hard delete?

---

## Step 1.4: TMDB Client

Fetch metadata on-demand.

**Context**:
- Only fetch what's needed (title, year, episode names for VFS)
- Don't store in database - cache in memory if needed
- TMDB API: https://developer.themoviedb.org/docs

**Tasks**:
1. Create TMDB client (API key from config)
2. GetMovie(tmdbID) → title, year
3. GetShow(tmdbID) → title, year, seasons with episode counts
4. GetSeasonEpisodes(tmdbID, seasonNumber) → episode numbers and names

**Decision points**:
- Cache TMDB responses? (in-memory TTL cache)
- Rate limiting approach
- Error handling when TMDB unavailable

---

## Step 1.5: REST API

Library management endpoints.

**Context**:
- Simple REST API for managing library
- WebDAV will be separate (Step 1.7)
- Consider API versioning from start

**Endpoints**:
```
POST   /api/movies          # Add movie by TMDB ID
GET    /api/movies          # List all movies
GET    /api/movies/:id      # Get movie details
DELETE /api/movies/:id      # Remove movie

POST   /api/shows           # Add show by TMDB ID
GET    /api/shows           # List all shows
GET    /api/shows/:id       # Get show with seasons/episodes
DELETE /api/shows/:id       # Remove show

GET    /api/status          # Health check
```

**Tasks**:
1. Set up router with chosen framework
2. Implement movie handlers
3. Implement show handlers
4. Add input validation
5. Add error responses (consistent format)

**Design considerations**:
- Request/response DTOs vs domain models
- How much detail to return in list vs single item
- Pagination for large libraries?

---

## Step 1.6: LibraryVFS

Build virtual filesystem from database.

**Context**:
- This is the key abstraction - VFS built from library, not torrents
- Items without torrent assignments are hidden
- Directory tree rebuilt on demand (with caching)

**VFS Structure**:
```
/Movies/
  └── Title (Year)/
      └── Title (Year).ext    ← only if torrent assigned

/TV Shows/
  └── Show Name (Year)/
      └── Season 01/
          └── S01E01 - Episode Name.ext
```

**Interface**:
```go
type Filesystem interface {
    Open(path string) (File, error)
    ReadDir(path string) (map[string]File, error)
}
```

**Tasks**:
1. Define Filesystem and File interfaces
2. Implement tree builder (queries DB, creates virtual structure)
3. Implement path resolution (virtual path → entry)
4. Handle root directory listing
5. Handle nested directory navigation

**Key design decisions**:
- Tree rebuild strategy (on every request? cached with TTL? event-driven?)
- How to handle path sanitization (special characters in titles)
- Case sensitivity

**For Stage 1**: Open() can return placeholder or error since no torrents yet. Focus on ReadDir() returning correct structure.

---

## Step 1.7: WebDAV Server

Expose VFS via WebDAV.

**Context**:
- Use `golang.org/x/net/webdav` package
- Adapt LibraryVFS to webdav.FileSystem interface
- Basic auth is sufficient for now

**Tasks**:
1. Create WebDAV adapter wrapping LibraryVFS
2. Implement webdav.FileSystem methods (OpenFile, Stat, etc.)
3. Set up HTTP server on configured port
4. Add basic authentication

**Reference**: See `distribyted/webdav/fs.go` for adapter pattern.

**Verification**:
- Mount WebDAV in Finder/Explorer
- Browse folder structure
- Folders should appear but files won't work (no torrents yet)

---

## Step 1.8: Integration & Testing

Wire everything together.

**Tasks**:
1. Wire up dependencies in main.go
2. Start API server and WebDAV server
3. Manual testing flow:
   - Add movie via API: `POST /api/movies {"tmdb_id": 550}`
   - Mount WebDAV in Infuse
   - Verify folder structure appears

**What works after Stage 1**:
- ✅ Add/remove movies and shows to library
- ✅ Browse library structure via WebDAV
- ❌ Files don't play (no torrent backend)
- ❌ No torrent assignment yet

---

## Suggested Project Structure After Stage 1

```
momoshtrem/
├── cmd/momoshtrem/main.go
├── internal/
│   ├── config/
│   │   └── config.go
│   ├── api/
│   │   ├── router.go
│   │   └── library.go
│   ├── library/
│   │   ├── db.go
│   │   ├── models.go
│   │   ├── movie_repo.go
│   │   └── show_repo.go
│   ├── tmdb/
│   │   └── client.go
│   ├── vfs/
│   │   ├── interface.go
│   │   └── library_fs.go
│   └── webdav/
│       └── server.go
├── migrations/
│   └── 001_core_schema.sql
├── config.yaml
└── go.mod
```

---

## Next: Stage 2

Once Stage 1 is complete and you can browse the library structure, proceed to [Stage 2: Torrent Integration](./STAGE_2_TORRENTS.md).
