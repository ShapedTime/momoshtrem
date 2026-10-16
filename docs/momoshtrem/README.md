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
| Metadata | Stored in Postgres, TMDB responses cached in `tmdb_cache` | Pages load without a TMDB call per item; see [Metadata](#metadata) |

## Configuration

Settings are layered, and each layer overrides the one before:

1. **Built-in defaults** (`internal/config/config.go`, `DefaultConfig`).
2. **`config.yaml`**, if it exists (path from `-config`, `/config/config.yaml` in Docker). It's optional. See `momoshtrem/config.yaml.example`.
3. **Environment variables**, which always apply, with or without a config file. Values are trimmed, and an empty variable counts as unset.

The service refuses to start without a database URL, with a port outside 1–65535, an invalid boolean, or a CORS origin or `HDREZKA_URL` that isn't `http(s)://host[:port]`.

| Env var | Config key | Notes |
|---|---|---|
| `DATABASE_URL` | `database.url` | **Required.** Postgres connection string. Migrations in `internal/library/migrations/` run at startup (see below). |
| `BACKEND_API_TOKEN` | `server.api_token` | Shared secret for the REST API (see below). |
| `CORS_ALLOWED_ORIGINS` | `server.cors_allowed_origins` | Comma-separated browser origins. Empty sends no CORS headers. |
| `WEBDAV_PUBLIC_URL` | `server.webdav_public_url` | The WebDAV address Infuse uses, shown by `/api/status`. Defaults to `http://localhost:<webdav_port>/`. |
| `WEBDAV_AUTH_ENABLED`, `WEBDAV_USERNAME`, `WEBDAV_PASSWORD` | `server.webdav_auth.*` | Basic auth for WebDAV. |
| `TMDB_API_KEY` | `tmdb.api_key` | Library metadata and air-date sync. Without it, adding or refreshing titles returns `503`. |
| `TMDB_LANGUAGE` | `tmdb.language` | Language of names and overviews, e.g. `de-DE`. Default `en-US`. |
| `TMDB_RPS` | `tmdb.rps` | TMDB requests per second, a whole number of at least 1. Default `20` (TMDB allows about 50). |
| `JACKETT_URL`, `JACKETT_API_KEY` | `jackett.*` | Torrent search (`/search/torrents`, Inspect and `/status` indexer health). Only momoshtrem talks to Jackett. |
| `HDREZKA_ENABLED` | `hdrezka.enabled` | HDRezka as a second source of files ([epic #63](https://github.com/ShapedTime/momoshtrem/issues/63)). Default `false`. Run [`hdrezka-probe`](#checking-hdrezka) before turning it on. |
| `HDREZKA_URL` | `hdrezka.url` | The HDRezka mirror, `http(s)://host[:port]` with no path. Default `https://rezka.ag`. |
| `OPENSUBTITLES_API_KEY`, `OPENSUBTITLES_USERNAME`, `OPENSUBTITLES_PASSWORD` | `opensubtitles.*` | Subtitle download. |
| `ANTHROPIC_API_KEY` | `ai.anthropic_api_key` | AI classifier and recommendations; disabled when unset. |
| `TRAKT_CLIENT_ID`, `TRAKT_CLIENT_SECRET` | `trakt.*` | Read-only Trakt integration. The secret is optional: new Trakt apps have none. |
| `SECRET_KEY` | `security.secret_key` | Encrypts Trakt tokens at rest. |
| `METRICS_ENABLED`, `METRICS_PORT` | `metrics.*` | Prometheus endpoint. |
| `TORRENT_LISTEN_PORT` | `torrent.listen_port` | BitTorrent port, TCP and uTP. Default `42069`; `0` picks a free port, so two instances can run side by side. |

Optional integrations that aren't configured are disabled, and `GET /api/status` reports them as `configured: false`.

### REST API authentication

When `BACKEND_API_TOKEN` is set, every `/api/*` request must send the same value in the `X-Momoshtrem-Token` header, or it gets a `401`. The web app sends it automatically when its own `BACKEND_API_TOKEN` matches, so set the variable on **both** services and restart both. Generate a value with `openssl rand -hex 32`.

When the token is unset the API is open and the service logs a warning at startup. Only do this for local development.

CORS is off unless `CORS_ALLOWED_ORIGINS` lists origins, each exactly as the browser sends it (`https://host[:port]`, no path; `*` is rejected). The web app calls the API server-to-server, so it doesn't need CORS.

### Errors and status

Errors use one shape: `{"error": {"code": "not_found", "message": "Movie not found"}}`.

`GET /api/status` reports the version, uptime, database health and size, the WebDAV URL, whether Jackett and TMDB are reachable, and this month's AI spend (`ai.mtd_cost_usd`, every AI feature). Results are cached for 30 s (5 s while the database is down), and `?refresh=1` forces a new check at most once every 5 s. It returns `503` only when the database is down.

The Docker image stamps the version from the `VERSION` build arg (`MOMOSHTREM_VERSION` in compose).

### Migrations

At startup every embedded migration that isn't recorded in `schema_migrations` runs, in version order, each in its own transaction. Numbers are reserved per task, so a lower number can land after a higher one and still runs. A Postgres advisory lock serialises concurrent starts (the server and `backfill-metadata`, say). Versions recorded in the database but missing from the binary are logged as a warning: the binary is older than the database.

## Metadata

Adding a movie or show stores its TMDB metadata: IMDb and TVDB IDs, artwork, runtime, genres and status, plus every season, specials (season 0) included, with episode names, air dates, stills and runtimes. Refresh (`POST /api/movies/{id}/refresh`, `POST /api/shows/{id}/refresh`) fetches them again. Titles and years are fixed when the item is added, because they name the WebDAV folders. When a refresh renames an episode that has a file, the WebDAV tree is rebuilt.

- **Specials.** Season 0 is stored when the show's `include_specials` is on, the default. Turning it off keeps the rows and hides them.
- **Episode ordering.** Each show has an ordering: `aired`, `absolute` or `episode_group`. Anime defaults to `absolute`. Anime means the Animation genre with Japanese as the original language or Japan as an origin country, or TMDB's `anime` keyword. Absolute numbers come from the chosen TMDB episode group. For `absolute`, if no group is chosen, they come from the absolute-type group with the most episodes. Groups often lag airing shows, so episodes that aired after the group's last one carry on its numbering. Without a group, numbers count up across the regular seasons. Specials never get one.
- **TMDB cache.** TMDB responses are kept in `tmdb_cache`. Details stay fresh for 7 days and episode groups for 7 days. Seasons stay fresh for 1 day while a show airs and for 30 days once it has ended. Concurrent requests for the same item share one TMDB call. When TMDB fails, an expired entry is served instead. The server deletes entries that expired more than 30 days ago when it starts. Refresh bypasses the cache. The client retries 5xx responses and timeouts with backoff, honours `Retry-After` on `429`, and never retries `401` or `404`.

### Backfilling an existing library

Movies and shows added before this existed have only a title and year. `backfill-metadata` fills in the rest. It adds specials and newly aired episodes, sets absolute numbers and, on a show's first sync, anime's default ordering. It never renames anything WebDAV shows: titles, years and existing episode names stay as they are, so Infuse paths don't move. It can run while the server is up: it only needs the database.

```bash
# Docker: a one-off container next to the running server
docker compose run --rm momoshtrem backfill-metadata --dry-run   # list what it would do
docker compose run --rm momoshtrem backfill-metadata

# Locally
cd momoshtrem && go build -o momoshtrem ./cmd/momoshtrem
DATABASE_URL=postgres://... TMDB_API_KEY=... ./momoshtrem backfill-metadata
```

| Flag | Default | |
|---|---|---|
| `--dry-run` | off | List the movies and shows it would process. No TMDB calls and no metadata writes, but like every start it first applies pending migrations. |
| `--force` | off | Also reprocess items that already have metadata, least recently synced first, so repeated `--force --limit=N` runs cover the whole library. |
| `--only=movies\|shows` | both | Limit the run to one kind. |
| `--limit=N` | `0` (all) | Process at most N movies and N shows. |
| `--rps=N` | `10` | TMDB requests per second, kept below the server's so both fit TMDB's limit. |

It is resumable. An item is marked done (`metadata_updated_at`) only after it fully synced. A rerun, or a run after Ctrl-C, picks up the rest. Failures are logged and counted, and the exit code is `1` when any item failed, so run it again.

## Checking HDRezka

`internal/hdrezka` is a small client for HDRezka: search, title pages (translations, seasons, episodes) and the CDN links of a movie or an episode. Its protocol follows [n0madic/go-hdrezka](https://github.com/n0madic/go-hdrezka) (MIT). It sends a User-Agent that doesn't start with `Mozilla`, because the site's Anubis proxy challenges browser user agents only. It makes at most 4 site requests a second.

`hdrezka-probe` checks, from the host it runs on, that a title resolves to a seekable MP4. It searches, ranks the hits, reads the title page and its translations, fetches the streams of one translation and measures each quality. Then it makes two Range requests on the chosen quality: bytes `0-1023` must come back `206` with the measured total and `ftyp` at byte 4, and 1 KB from the middle must come back `206` at that offset. It also reports whether `moov` comes before `mdat` (faststart). It exits `1` when anything fails. It needs no database and ignores `HDREZKA_ENABLED`: it's what tells you whether to turn HDRezka on.

```bash
# Docker: no Postgres needed
docker compose run --rm --no-deps momoshtrem hdrezka-probe interstellar
docker compose run --rm --no-deps momoshtrem hdrezka-probe -season 1 -episode 1 "breaking bad"

# Locally
cd momoshtrem && go run ./cmd/momoshtrem hdrezka-probe -translator 238 interstellar
```

| Flag | Default | |
|---|---|---|
| `-translator id` | the page default | The translation to check. When the default is premium, the first one that isn't. `238` is usually the original audio with subtitles. |
| `-quality label` | the best that isn't premium | E.g. `1080p`. Premium labels (`1080p Ultra`, `2K`, `4K`) are a short teaser without an account. |
| `-season N`, `-episode N` | `1` | For a series. |
| `-show-urls` | off | Print the CDN links in full. They're signed for this IP and expire, so the default prints only the host and file name. |

The links are signed and may be bound to the IP that asked for them, so run the probe on the host that will stream. A challenge page (`ErrChallenge`) or a `403` there means HDRezka won't work from it.

With `HDREZKA_ENABLED=true` the API serves `/api/hdrezka/*` and the assign routes (PLAN §6 "HDRezka (#63)"), and WebDAV streams the assigned files: momoshtrem resolves a stream's signed link (cached for at most 4 hours), then proxies the bytes from the CDN with Range requests, resolving the link again when the CDN refuses it. Opening a file does no I/O, so directory listings never touch the CDN. Signed links never appear in logs or errors. `/api/status` reports `hdrezka.reachable`, and the anti-bot page as `HDRezka asks for a browser check`.

An assign also downloads the stream's subtitles: WebVTT tracks, such as the English and Russian ones of "Оригинал (+субтитры)". They're converted to SRT and stored in `{subtitles.download_path}/hdrezka/{movie|episode}/{id}/{lang}.srt`. WebDAV lists them next to the video, while it plays from HDRezka. Assigning another translator replaces them.

## Episode mapping

Assigning a torrent (`POST /api/shows/{id}/assign-torrent`, or `/movies/{id}/…`) starts a mapping job and answers `202 {job_id}`; nothing blocks on the torrent. `{info_hash}` instead of `{magnet_uri}` re-runs the mapping of a torrent the library already knows, with the magnet it stored. A worker (`internal/service/assignment_worker.go`) takes it from there:

1. It fetches the file list, refreshing the show's metadata when it's stale.
2. The rules classifier (`internal/identify`) maps every file to episodes, with a confidence and a reason.
3. Claude Haiku looks at the files the rules aren't sure of (`internal/identify/ai`; only with `ANTHROPIC_API_KEY`, and never over a `high` rules answer).
4. A job where everything that maps is `high`, with no conflict and no unmatched video, applies on its own. Any other job waits for the review screen (`/mapping/{id}` in the web app): accept, change or ignore files, re-check with AI with a hint, then Apply.

Apply writes the assignments in one transaction and rebuilds the WebDAV tree. A file that holds several episodes is one entry, `Show - S01E01-E02 - Name.ext`. Assignments made before #5 (`match_source` NULL) keep their per-episode names, so their Infuse paths don't move. `?wait=true` keeps the old synchronous call until H2. The state machine, endpoints and rules are in [PLAN §6 "Assignment"](../redesign/PLAN.md).

- **AI settings** are the `ai` key of `app_settings`, with defaults: the classifier on, `claude-haiku-5-5`, auto-apply at high confidence, no monthly budget. #9 serves the endpoints to change them.
- **Accuracy** is measured on the corpus in `internal/identify/testdata/`: `go test ./internal/identify` checks the gates (≥ 95% exact, zero wrong `high`). `go run ./cmd/classifier-eval` runs the golden torrents through the rules and the AI and reports accuracy, cost and latency. Its AI modes cost cents; `-modes rules` and `-dry-run` are free.

## Torrent activity

What the torrents are doing, for the Activity screen (`internal/activity`; the contract is in [PLAN §6 "Torrents and activity (B6)"](../redesign/PLAN.md)).

- **`GET /api/torrents`** lists every torrent the library knows (active assignments and open mapping jobs), loaded or not, plus loaded torrents nothing references. Torrents stream on demand and load on their first WebDAV read, so one that isn't loaded is `idle` with null progress, and there's no ETA.
- **`GET /api/torrents/summary`** has the total speeds, the last minute of them, how many torrents are in each state, and the disk under `metadata_folder` with the piece cache's fill.
- **Speeds** come from a sampler that reads each loaded torrent's byte counters every 2 s and smooths them.
- **Streaming sessions** (`internal/sessions`) say who plays what: the WebDAV server puts each request's method, User-Agent and address in its context, and every video a GET opens reports its reads. One client on one file is one session, however many Range GETs it makes, until 30 s after its last read. Sniffs (under 64 KB, closed within 2 s) and PROPFIND or HEAD opens don't count. Each session has the client and device from the User-Agent ("Infuse", "Apple TV"), the item, a smoothed rate, a position from the runtime and, for a torrent, how much is buffered ahead. They're on `/api/torrents` items and the summary, they make a torrent `streaming`, and `/api/status` counts them (`webdav.active_readers`).
- **`GET /api/torrents/{hash}`** is a torrent's detail: its piece map (run-length), files with what each plays as or why it's ignored, trackers and totals. It never loads the torrent: an unloaded one's files come from its stored metainfo, else from the newest job.
- **`POST /api/torrents/{hash}/pause`** and **`/resume`**, and **`/pause-all`** and **`/resume-all`**. A pause is sticky: it's kept in `app_settings` (key `torrents`) across restarts, reads don't wake the torrent, and a torrent that isn't loaded is paused when it loads. It goes with the torrent: deleting or releasing a torrent clears it, and startup prunes the pauses of torrents the library no longer knows.

## Removing torrents and titles

The deletes clean up after themselves (`internal/service/removal.go`; the contract is in [PLAN §6 "Torrents and activity (B6)"](../redesign/PLAN.md)).

- **`DELETE /api/torrents/{hash}?delete_data=true`.** It cancels the torrent's open jobs and deactivates its assignments, keeping the rows as history. It deletes its torrent subtitles and drops it from the client. With `delete_data=true` it also deletes its pieces from the cache.
- **`DELETE /api/movies/{id}` and `/shows/{id}`** take `delete_data` too. They delete every subtitle of the title, rows and files, and release the torrents nothing else uses.
- **Unassigning a file** deletes the subtitles that came with it and releases its torrent when it's unused, never its data.

Each torrent's metainfo is kept in `{metadata_folder}/metainfo/<hash>.torrent` when its metadata arrives. A torrent added again loads without waiting for peers, and one that isn't loaded can still have its data deleted. The torrent service serialises add, wait, drop and remove per hash, so a WebDAV read and a mapping job never fight over one torrent.

## Tests

`go vet ./... && go test ./...` needs no services: TMDB and HDRezka tests use an `httptest` server and fixtures in `internal/tmdb/testdata/` and `internal/hdrezka/testdata/` (recorded from the live site, with the signed CDN links and the client IP replaced), and Anthropic sits behind a fake. The Postgres tests (`internal/library`, `internal/metadata`, `internal/catalog`, `internal/service`, `internal/subtitle`, `internal/activity` and `internal/api`) skip unless `MOMOSHTREM_TEST_DATABASE_URL` points at a throwaway database (they write to it):

```bash
docker run -d --rm --name momo-test-pg -e POSTGRES_PASSWORD=pw -e POSTGRES_DB=momo -p 55432:5432 postgres:16-alpine
MOMOSHTREM_TEST_DATABASE_URL='postgres://postgres:pw@localhost:55432/momo?sslmode=disable' go test ./internal/...
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
