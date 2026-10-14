# momoshtrem

A library-first media streaming service inspired by distribyted.

## Core Concept

Unlike distribyted (which builds VFS from torrents), momoshtrem builds VFS from a library database. Torrents are mapped TO library items, not the other way around.

```
Traditional:  Torrent → VFS → WebDAV
momoshtrem:   Library (Postgres) → VFS → Torrent files → WebDAV
```

## Design Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Language | Go | Reuse anacrolix/torrent, familiar patterns |
| Database | Postgres | Relational queries, JSONB, concurrent access from background jobs. (Stages 1–3 were built on SQLite; `cmd/migrate-to-pg` moved the data.) |
| VFS approach | Library-first | Structure driven by user's library, not torrent contents |
| Metadata | On-demand from TMDB | Keep DB minimal, fetch when needed |

## Configuration

Settings are layered, and each layer overrides the one before:

1. **Built-in defaults** (`internal/config/config.go`, `DefaultConfig`).
2. **`config.yaml`**, if it exists (path from `-config`, `/config/config.yaml` in Docker). It's optional. See `momoshtrem/config.yaml.example`.
3. **Environment variables**, which always apply, with or without a config file. Values are trimmed, and an empty variable counts as unset.

The service refuses to start without a database URL, with a port outside 1–65535, an invalid boolean, or a CORS origin that isn't `http(s)://host[:port]`.

| Env var | Config key | Notes |
|---|---|---|
| `DATABASE_URL` | `database.url` | **Required.** Postgres connection string. Migrations in `internal/library/migrations/` run at startup. |
| `BACKEND_API_TOKEN` | `server.api_token` | Shared secret for the REST API (see below). |
| `CORS_ALLOWED_ORIGINS` | `server.cors_allowed_origins` | Comma-separated browser origins. Empty sends no CORS headers. |
| `WEBDAV_PUBLIC_URL` | `server.webdav_public_url` | The WebDAV address Infuse uses, shown by `/api/status`. Defaults to `http://localhost:<webdav_port>/`. |
| `WEBDAV_AUTH_ENABLED`, `WEBDAV_USERNAME`, `WEBDAV_PASSWORD` | `server.webdav_auth.*` | Basic auth for WebDAV. |
| `TMDB_API_KEY` | `tmdb.api_key` | Library metadata and air-date sync. |
| `JACKETT_URL`, `JACKETT_API_KEY` | `jackett.*` | Torrent search (moving from web to the backend in B4). |
| `OPENSUBTITLES_API_KEY`, `OPENSUBTITLES_USERNAME`, `OPENSUBTITLES_PASSWORD` | `opensubtitles.*` | Subtitle download. |
| `ANTHROPIC_API_KEY` | `ai.anthropic_api_key` | AI classifier and recommendations; disabled when unset. |
| `TRAKT_CLIENT_ID`, `TRAKT_CLIENT_SECRET` | `trakt.*` | Read-only Trakt integration. |
| `SECRET_KEY` | `security.secret_key` | Encrypts Trakt tokens at rest. |
| `METRICS_ENABLED`, `METRICS_PORT` | `metrics.*` | Prometheus endpoint. |

Optional integrations that aren't configured are disabled, and `GET /api/status` reports them as `configured: false`.

### REST API authentication

When `BACKEND_API_TOKEN` is set, every `/api/*` request must send the same value in the `X-Momoshtrem-Token` header, or it gets a `401`. The web app sends it automatically when its own `BACKEND_API_TOKEN` matches, so set the variable on **both** services and restart both. Generate a value with `openssl rand -hex 32`.

When the token is unset the API is open and the service logs a warning at startup. Only do this for local development.

CORS is off unless `CORS_ALLOWED_ORIGINS` lists origins, each exactly as the browser sends it (`https://host[:port]`, no path; `*` is rejected). The web app calls the API server-to-server, so it doesn't need CORS.

### Errors and status

Errors use one shape: `{"error": {"code": "not_found", "message": "Movie not found"}}`.

`GET /api/status` reports the version, uptime, database health and size, the WebDAV URL, and whether Jackett and TMDB are reachable. Results are cached for 30 s (5 s while the database is down), and `?refresh=1` forces a new check at most once every 5 s. It returns `503` only when the database is down.

The Docker image stamps the version from the `VERSION` build arg (`MOMOSHTREM_VERSION` in compose).

## Tests

`go vet ./... && go test ./...` needs no services. The Postgres tests in `internal/library` skip unless `MOMOSHTREM_TEST_DATABASE_URL` points at a throwaway database (they write to it):

```bash
docker run -d --rm --name momo-test-pg -e POSTGRES_PASSWORD=pw -e POSTGRES_DB=momo -p 55432:5432 postgres:16-alpine
MOMOSHTREM_TEST_DATABASE_URL='postgres://postgres:pw@localhost:55432/momo?sslmode=disable' go test ./internal/library/...
docker stop momo-test-pg
```

## Implementation Stages

### Stage 1: Foundation
Library management + WebDAV structure. No actual streaming yet.

→ [STAGE_1_FOUNDATION.md](./STAGE_1_FOUNDATION.md)

### Stage 2: Torrent Integration
Connect torrents to library items. Basic streaming works.

→ [STAGE_2_TORRENTS.md](./STAGE_2_TORRENTS.md)

### Stage 3: Streaming Optimization
Piece prioritization for smooth playback. Seek-friendly buffering.

→ [STAGE_3_STREAMING.md](./STAGE_3_STREAMING.md)

### Future Stages (deferred)
- Stage 4: Subtitles (OpenSubtitles API)
- Stage 5: Skip Intro (EDL files)
- Stage 6: Trakt integration

## Reference Architecture

```
┌─────────────────────────────────────────────────────────┐
│                     REST API (:4444)                     │
│  Library CRUD, Torrent assignments, Status              │
└────────────────────────────┬────────────────────────────┘
                             │
┌────────────────────────────┴────────────────────────────┐
│                   Library (Postgres)                     │
│  movies, shows, seasons, episodes, torrent_assignments  │
└────────────────────────────┬────────────────────────────┘
                             │
┌────────────────────────────┴────────────────────────────┐
│                     LibraryVFS                           │
│  /Movies/Title (Year)/Title (Year).mkv                  │
│  /TV Shows/Show (Year)/Season 01/S01E01 - Name.mkv      │
└────────────────────────────┬────────────────────────────┘
                             │
┌────────────────────────────┴────────────────────────────┐
│                   Torrent Service                        │
│  anacrolix/torrent client, activity tracking            │
└────────────────────────────┬────────────────────────────┘
                             │
┌────────────────────────────┴────────────────────────────┐
│                  WebDAV Server (:36911)                  │
│  Infuse / VLC / other WebDAV clients                    │
└─────────────────────────────────────────────────────────┘
```

## Key Files in distribyted (Reference)

Study these before implementation:
- `distribyted/fs/torrent.go` - VFS with lazy-loading
- `distribyted/torrent/service.go` - Torrent lifecycle
- `distribyted/webdav/fs.go` - WebDAV adapter
- `distribyted/torrent/activity.go` - Idle mode

## Getting Started

Start with Stage 1. Each stage builds on the previous and produces a working (if limited) system.
