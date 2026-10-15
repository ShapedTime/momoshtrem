# momoshtrem - Code Quality Guide

## Core Principles

1. **Small, focused interfaces** - Define where used, not in separate files
2. **Explicit dependencies** - Constructor injection, no globals
3. **Graceful degradation** - Optional features warn, don't crash
4. **Readable error chains** - Wrap errors with context

---

## Package Organization

```
internal/
├── api/           # REST API (Gin handlers, routing, middleware)
├── config/        # Configuration loading (defaults + YAML + env)
├── library/       # Database layer (Postgres repositories, models, migrations)
├── status/        # /api/status report and dependency probes
├── torrent/       # Torrent client wrapper & service
├── vfs/           # Virtual filesystem for WebDAV
├── webdav/        # WebDAV server wrapper
├── streaming/     # Piece prioritization, format detection
├── identify/      # Episode mapping from file names: pure rules classifier (ai/: Haiku resolver)
├── service/       # Assignment jobs: worker, review, re-check, Apply
├── subtitle/      # Subtitle storage & management
├── opensubtitles/ # OpenSubtitles API client
└── common/        # Shared utilities
```

**Naming:**
- Package names are domain nouns: `library`, `torrent`, `vfs`
- Type names are PascalCase: `MovieRepository`, `TorrentService`
- Receivers are 1-2 chars: `(r *MovieRepository)`, `(s *service)`

---

## Interface Patterns

**Small and focused:**
```go
type Service interface {
    AddTorrent(magnetURI string) (*TorrentInfo, error)
    GetTorrent(infoHash string) (*TorrentInfo, error)
    RemoveTorrent(infoHash string, deleteData bool) error
    // ... focused on one domain
}
```

**Compile-time verification:**
```go
var _ Service = (*service)(nil)  // Catches missing methods at compile time
```

**Compose standard library interfaces:**
```go
type File interface {
    io.Reader
    io.ReaderAt
    io.Closer
    Name() string
    Size() int64
}
```

---

## Error Handling

**Sentinel errors for known conditions:**
```go
var (
    ErrTorrentNotFound = errors.New("torrent not found")
    ErrMetadataTimeout = errors.New("timeout waiting for torrent metadata")
    ErrInvalidMagnet   = errors.New("invalid magnet URI")
)
```

**Always wrap with context:**
```go
if err != nil {
    return fmt.Errorf("failed to open database: %w", err)
}
```

**Handle sql.ErrNoRows explicitly:**
```go
if err == sql.ErrNoRows {
    return nil, nil  // Not found is not an error
}
```

---

## Repository Pattern

```go
type MovieRepository struct {
    db *DB
}

func NewMovieRepository(db *DB) *MovieRepository {
    return &MovieRepository{db: db}
}

func (r *MovieRepository) Create(movie *Movie) error { ... }
func (r *MovieRepository) GetByID(id int64) (*Movie, error) { ... }
func (r *MovieRepository) List() ([]*Movie, error) { ... }
func (r *MovieRepository) Delete(id int64) error { ... }
```

**Verify affected rows:**
```go
result, err := r.db.Exec(`DELETE FROM movies WHERE id = $1`, id)
affected, _ := result.RowsAffected()
if affected == 0 {
    return fmt.Errorf("movie not found")
}
```

---

## Concurrency

**RWMutex for read-heavy maps:**
```go
func (s *service) GetTorrent(hash string) (*TorrentInfo, error) {
    s.mu.RLock()
    t, exists := s.torrents[hash]
    s.mu.RUnlock()
    // ...
}

func (s *service) RemoveTorrent(hash string) error {
    s.mu.Lock()
    defer s.mu.Unlock()
    delete(s.torrents, hash)
    // ...
}
```

**Background goroutines with stop channel:**
```go
func (am *ActivityManager) Start() {
    go am.idleCheckLoop()
}

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
```

---

## Configuration

**Hierarchy: Defaults → File (optional) → Environment (always)**
```go
func Load(path string) (*Config, error) {
    cfg := DefaultConfig()
    // The file is optional: a missing file is not an error...
    if data, err := os.ReadFile(path); err == nil {
        yaml.Unmarshal(data, cfg)  // File overrides defaults
    }
    // ...and env overrides apply whether or not it exists.
    if err := applyEnv(cfg); err != nil {
        return nil, err            // e.g. METRICS_PORT=abc
    }
    return cfg, cfg.Validate()     // e.g. DATABASE_URL missing
}
```

- Every new setting gets a YAML key (snake_case) **and** an env var, both read in `applyEnv` with the `envString`/`envBool`/`envPort`/`envList` helpers. Add a case to `config_test.go`.
- An empty env var means "unset". Invalid values are errors, never silently ignored.
- Optional integrations (Jackett, Anthropic, Trakt, OpenSubtitles) are disabled when their keys are empty, and `/api/status` reports `configured: false`.
- Cross-cutting API settings (token, CORS) go in `api.Options` and are passed to `NewServer`. Gin middleware added after routes are registered doesn't apply to them, so don't add setters for these.

---

## API Errors

Every error response has the same shape:

```json
{"error": {"code": "not_found", "message": "Movie not found"}}
```

```go
errorResponse(c, http.StatusNotFound, "Movie not found")            // code derived from status
errorCodeResponse(c, http.StatusConflict, "already_assigned", "...") // explicit code
```

`errorResponse` derives the code from the status: 400 `bad_request`, 401 `unauthorized`, 403 `forbidden`, 404 `not_found`, 409 `conflict`, 422 `validation_error`, 429 `rate_limited`, 503 `unavailable`, 504 `timeout`, other 5xx `internal_error`. Use `errorCodeResponse` when a client needs to tell two errors with the same status apart. Messages are shown to users, so never put secrets in them, including URLs that carry API keys (strip `*url.Error`).

---

## Logging

**Use slog with component context:**
```go
log := slog.With("component", "torrent-service")

slog.Info("Torrent storage initialized", "metadata_folder", cfg.Torrent.MetadataFolder)
slog.Warn("TMDB API key not configured")
slog.Error("Failed to load config", "error", err)
```

---

## Testing

**Table-driven tests:**
```go
func TestByteToPiece(t *testing.T) {
    tests := []struct {
        name   string
        offset int64
        want   int
    }{
        {"start of first piece", 0, 0},
        {"second piece", 1024, 1},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got := p.byteToPiece(tt.offset)
            if got != tt.want {
                t.Errorf("got %d, want %d", got, tt.want)
            }
        })
    }
}
```
