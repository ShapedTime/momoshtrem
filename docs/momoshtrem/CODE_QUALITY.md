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
├── api/           # REST API (Gin handlers, routing)
├── config/        # Configuration loading (YAML + env)
├── library/       # Database layer (repositories, models)
├── torrent/       # Torrent client wrapper & service
├── vfs/           # Virtual filesystem for WebDAV
├── webdav/        # WebDAV server wrapper
├── streaming/     # Piece prioritization, format detection
├── identify/      # Episode identification from filenames
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
result, err := r.db.Exec(`DELETE FROM movies WHERE id = ?`, id)
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

**Hierarchy: Defaults → File → Environment**
```go
func Load(path string) (*Config, error) {
    cfg := DefaultConfig()
    yaml.Unmarshal(data, cfg)  // File overrides defaults
    if env := os.Getenv("TMDB_API_KEY"); env != "" {
        cfg.TMDB.APIKey = env  // Env overrides file
    }
    return cfg, nil
}
```

---

## Logging

**Use slog with component context:**
```go
log := slog.With("component", "torrent-service")

slog.Info("Database initialized", "path", cfg.Database.Path)
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
