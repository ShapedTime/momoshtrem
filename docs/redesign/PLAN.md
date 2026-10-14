# momoshtrem v2: redesign and feature plan

> Status: draft for orchestration · 2026-10-11
> Companion files in this folder: `design-prompt-cinematic.md`, `design-prompt-playful.md` (one Claude Design session each)

## 0. How to use this plan

- The work is split into **phases** and **tasks**. Each task has an ID (`G1`, `B2`, `F3`, …), dependencies, owned paths and acceptance criteria. To brief an agent, give it the task's section plus the template in §11.
- Tasks in the same wave with no dependency between them can run in parallel, each on its own branch or worktree `v2/<id>-<slug>`. Owned paths are chosen so that parallel tasks rarely edit the same files.
- **Contract first.** §6 is the API contract. Backend tasks implement it. Frontend tasks build against it using MSW mocks, so the frontend never waits for the backend.
- **Additive backend changes until Phase 4.** Existing endpoints keep working, so the current UI stays usable while v2 is built.
- **Shared-file etiquette.** Each backend track registers its routes from its own file (`internal/api/<domain>_routes.go` → `register<Domain>Routes(r)`); `router.go` gets only a one-line call. Migration numbers are reserved per task in §5.
- Every agent reads `CLAUDE.md`, the relevant `docs/*/CODE_QUALITY.md`, and §2–§6 of this plan before starting.

## 1. Goals and non-goals

**Goals**
1. **One product, momoshtrem.** The frontend is renamed and gets a new identity with a small cat logo, plus a modern responsive design that works as well on a phone as on a laptop.
2. **Personal discovery.** Trakt-powered watch state and "Up next", AI-curated recommendation shelves, and an "Ask momoshtrem" box.
3. **Reliable episode mapping.** Handle specials, anime/absolute numbering and date-based shows, using an AI resolver (Haiku) plus a human review and override step.
4. **Torrent search you control.** Structured release info, filters, sorting, pagination and a smart "best match", with no layout breakage on small screens.
5. **A login that password managers understand** (username + password).

**Non-goals (this round):** multi-user accounts; writing to Trakt (Infuse already scrobbles); an in-browser player; automatically grabbing new episodes (see backlog, §10).

## 2. Decisions

| # | Decision | Notes |
|---|---|---|
| D1 | Product name **momoshtrem** everywhere | The Go service keeps its name and directory. Frontend: `PrettyTVCatalog/` → `web/`, package `momoshtrem-web`, compose service `web` (container `momoshtrem-web`), cookie `momoshtrem_session`, docs `docs/prettytvcatalog/` → `docs/web/`. |
| D2 | Two Claude Design explorations, **A: Cinematic dark** and **B: Playful cat-forward**. The user then picks one or merges them. | Prompts are in this folder. Phase 3 starts after the pick. |
| D3 | Trakt is **read-only** | Device-code OAuth. Import history, ratings, watchlist and progress. No writes. |
| D4 | AI recommendations are **nightly shelves plus an on-demand "Ask momoshtrem"** | Shelves are precomputed into Postgres so pages load instantly. |
| D5 | AI, Trakt and Jackett logic live in the **Go backend** | One home for secrets, DB access, scheduling and cost tracking. Next.js stays a thin BFF: auth, proxying, streaming pass-through, TMDB browsing. |
| D6 | Models: classifier `claude-haiku-5-5`; recommendation agent `claude-opus-5-5` by default, configurable (e.g. `claude-sonnet-5-5`) | Uses the Go SDK `github.com/anthropics/anthropic-sdk-go`. Haiku can't use the `_20260209` web search/fetch tools, so the agent runs on Opus or Sonnet. **Load the `claude-api` skill before writing any Claude API code.** |
| D7 | Single user | `APP_USERNAME` and `APP_PASSWORD` env vars; no users table. |
| D8 | Specials are stored as season 0 | Shown as "Specials" in the UI. On WebDAV: a `Season 00` folder with `S00Exx` file names (the Infuse/Plex convention; verify on device). |
| D9 | The database is **Postgres** (already true) | Docs and `config.yaml.example` said SQLite until G2 fixed them. Migrations live in `momoshtrem/internal/library/migrations/` (embedded). |

## 3. Current state: findings that drive this plan

Based on a code audit and a live audit on 2026-10-11. Screenshots of the current UI are in `docs/screenshots/`.

**Frontend (PrettyTVCatalog)**
- **Stack and dead styles.** Next 16.0.10, React 19.2, Tailwind 4.1. Tailwind v4 never loads `tailwind.config.ts` (there's no `@config`), so classes used in the code such as `animate-fade-in`, `bg-bg-secondary`, `bg-bg-tertiary` and `scrollbar-thin` render nothing.
- **Data fetching.** There is no data cache: hooks are hand-written, and `setInterval` polling (1.5–5 s) never pauses. The show page goes stale after assigning a torrent. The library page makes one TMDB call per item to get posters.
- **Why password autofill fails:**
  - The form is password-only.
  - It renders only after hydration (the `useSearchParams` bail-out).
  - Inputs are disabled while submitting.
  - Success is a soft navigation.
  - The deployment is plain HTTP (`3000:3000`, `SECURE_COOKIES=false`).
- **Other login issues.** The `redirect` parameter isn't validated, there's no logout in the UI, and an expired session on `/api/*` gets a 307 to HTML instead of a 401.
- **Discovery.** Weekly trending, a single-genre discover page and search. No recommendations, no watchlist, and filter state isn't kept in the URL.
- **Torrent search:**
  - **Query.** A centred modal calling Jackett `t=search&q=&limit=50`, with no categories and no ID-based search.
  - **Filters and sorting.** Four quality chips (results with unknown quality disappear once any chip is on) and two sort toggles.
  - **No pagination.**
  - **Mobile layout.** The title `h3` lacks `min-w-0`, so long dotted release names overflow. The Add button's forced `w-full` squeezes the metadata row on mobile.
  - **Silent drops.** Failed `.torrent` → magnet conversions are silently dropped.
  - **Lost context.** Episode searches lose their episode context, and the TV results modal stacks on top of the search modal.
- **Library and torrents.** Library cards show their title and remove button only on hover, which doesn't work on touch. Torrents don't link to their titles, and nothing tells you how to play in Infuse.
- **Brand.** No logo, favicon or manifest. The name "PrettyTVCatalog" appears in about ten places, and the login footer still says "distribyted".

**Live audit (signed in, 1200–1289 px desktop and 406 px mobile)**
- **Poster grids overlap.** On Movies, TV, Library and Search, fixed-width cards (140–200 px) sit in narrower grid columns. On desktop the posters overlap; on mobile the gaps are uneven.
- **Titles appear only on hover**, in every grid and carousel, so touch users can't identify anything. Search results without a poster show only an icon.
- **Default torrent queries miss.**
  - "Frieren: Beyond Journey's End S01E01" returns 0 results, while "Frieren" returns 20; the punctuation breaks matching.
  - The show-level default returned an adult result.
  - "One Piece" returned 0.
  - Only one indexer (The Pirate Bay) is configured in Jackett, and results top out at 20.
  - `&hearts;` and other HTML entities show up literally.
- **Scene and TMDB numbering disagree.** TMDB has Frieren as one 38-episode season, but releases name the second half S02E01–E09. From episode 29 on, search and matching both fail.
- **Torrents page contradicts the Library.** It says "No Active Torrents" while the Library shows 38 movies as "Ready", because torrents load lazily. Status labels also differ between the library grid and detail pages ("In Library" vs "Ready to Stream").
- **Detail pages:**
  - "Ready to Stream" is a disabled button that looks like the main call to action, while red "Search Torrents" stays primary.
  - The torrent card sits below the fold.
  - Frieren's 38 tall episode cards make a page about 8,850 px long.
  - Episode "Remove" is a borderless 241×36 px target on mobile, larger than "Subtitles".
- **Mobile:**
  - The Movies/TV filter drawer opens on every load.
  - Hero elements overlap: the badge over the release date, and the season label over "Add to Library".
  - In torrent search, release names are cut to two lines, size and seeders wrap into cramped stacks, the indexer is hidden, and "Add" stretches to about 228 px.
  - Tap targets are as small as 28 px.
- **Torrent modal:**
  - Its height jumps after each search, so clicks land on the backdrop and close it.
  - Closing loses the query.
  - A quality-chip filter that removes everything shows "No torrents found" instead of naming the filter as the cause.
- **Subtitles:** results aren't ranked by how well they match the assigned release.
- **Home:**
  - Carousels load scrolled 48 px and have no arrows.
  - The hero changes on every load.
  - Header search plus the search page produce two search boxes with a double clear button.

**Backend (momoshtrem)**
- **Classifier** (`internal/identify`, no tests). It runs a list of regexes and takes the first match. Known failures:

  | Input | Result |
  |---|---|
  | Any special | Rejected (`special_not_supported`); season 0 is never stored |
  | `One Piece - 1071` | S10E71 (absolute numbering unsupported) |
  | `[SubsPlease] Frieren - 05 (1080p)` | Not identified |
  | `1920x1080` | S20E108 |
  | `.2019.` (a year) | S20E19 |
  | `Law.and.Order.Special.Victims.Unit.S01E01` | Treated as a special |
  | `Trailer.Park.Boys` | Skipped as a trailer |

  - A date regex exists but is never used.
  - There's no manual override.
  - Assignment blocks the HTTP request for up to 60 s.
  - `FallbackHandler` is a no-op and only ever sees unidentified files.
- **Data model.** Movies and shows store only title and year: no external IDs, TMDB episode IDs, absolute numbers, artwork or watch state.
- **Security and config.**
  - The REST API has no auth (it's internal-only) and CORS is `*`.
  - `config.Load` skips **all** env overrides when `config.yaml` is missing.
  - Jackett is called only from the frontend.
- **Torrent status.**
  - Speeds are hard-coded to 0.
  - After a restart, torrents aren't listed until a file is opened.
  - `delete_data` is ignored.
  - The API's pause/resume bypasses the ActivityManager.
  - `getShow` makes N+1 queries.
- **AI and Trakt.** There's no LLM or Trakt code yet.

## 4. Target architecture

```
                 Browser (phone / laptop; installable PWA)
                          │ HTTPS (Caddy, real cert via DuckDNS DNS-01)
                          ▼
             web: Next.js 16 BFF (:3000, internal only)
   auth (username+password, JWT cookie) · TMDB browse/discover/search
   thin /api/* proxy to the backend · SSE pass-through for Ask
                          │ X-Momoshtrem-Token (shared secret)
                          ▼
                 momoshtrem: Go (:4444)
  library · assignment jobs · classifier (rules → Haiku resolver)
  search (Jackett Torznab + release parser + scoring)
  trakt (device OAuth, sync) · recommend (agent, shelves, ask)
  scheduler (air dates, trakt sync, nightly recs) · tmdb cache · ai usage
       │              │             │              │
   Postgres        Jackett      TMDB / Trakt   Anthropic API
       │
   VFS → WebDAV (:36911, via Caddy) → Infuse on Apple TV
```

Business logic lives in Go packages (`internal/<domain>`) and HTTP handlers in `internal/api`. Next.js API routes stay thin (per CLAUDE.md).

New and moved environment variables:

| Var | Where | Purpose |
|---|---|---|
| `APP_USERNAME` | web | Login username. Optional, defaults to `admin` with a startup warning. |
| `BACKEND_API_TOKEN` | web + momoshtrem | Shared-secret header between web and backend |
| `JACKETT_URL`, `JACKETT_API_KEY` | **moves** from web to momoshtrem | Search moves to the backend (the web route is removed in H2) |
| `ANTHROPIC_API_KEY` | momoshtrem | Classifier and recommendations. The features are disabled when unset. |
| `TRAKT_CLIENT_ID`, `TRAKT_CLIENT_SECRET` | momoshtrem | Trakt app, created at trakt.tv/oauth/applications |
| `SECRET_KEY` | momoshtrem | AES-GCM key for encrypting Trakt tokens at rest |
| `CORS_ALLOWED_ORIGINS` | momoshtrem | Optional comma-separated browser origins. Empty (the default) sends no CORS headers, since web calls the backend server-to-server. |
| `WEBDAV_PUBLIC_URL` | momoshtrem | The WebDAV address Infuse uses (e.g. `https://momoshtrem.duckdns.org:36911/`), reported by `/status`. Defaults to `http://localhost:<webdav_port>/`. |

momoshtrem reads these settings and the other secrets and service URLs (`DATABASE_URL`, `TMDB_API_KEY`, OpenSubtitles, WebDAV auth, metrics) from env, with or without `config.yaml` (defaults → file → env). `config.yaml` covers the rest (ports, torrent, streaming…). `DATABASE_URL` is required.

## 5. Data model changes (Postgres)

Migration numbers are reserved per task so parallel branches don't collide.

**`002_app_settings.sql` (G2)**
- `app_settings(key text pk, value jsonb, updated_at)`. Holds the quality profile, AI settings and classifier defaults.

**`003_metadata.sql` (B1)**
- `movies`: add `imdb_id`, `poster_path`, `backdrop_path`, `runtime`, `genres jsonb`, `metadata_updated_at`.
- `shows`: add the following columns:
  - IDs and names: `imdb_id`, `tvdb_id`, `original_name`.
  - Origin: `original_language`, `origin_country text[]`, `is_anime bool`.
  - Artwork and status: `poster_path`, `backdrop_path`, `status`.
  - Episode ordering: `episode_ordering` (`aired`|`absolute`|`episode_group`) and `episode_group_id`.
  - Matching options: `include_specials bool default true` and `classifier_mode` (`auto`|`rules_only`|`ai_always`).
  - `metadata_updated_at`.
- `seasons`: allow `season_number = 0`; add `name` and `poster_path`.
- `episodes`: add `tmdb_episode_id`, `absolute_number`, `episode_type`, `runtime`, `still_path`, `overview`.
- `tmdb_cache(key text pk, payload jsonb, fetched_at, expires_at)`.

**`004_assignment_jobs.sql` (B2)**
- `torrent_assignments`: add `confidence` (`high|medium|low`) and `match_source` (`rules|ai|manual`).
- `assignment_jobs(id, item_type, item_id, info_hash, magnet_uri, torrent_name, status, dry_run bool, error, created_at, updated_at)`.
  - Status flow: `queued → fetching_metadata → classifying → needs_review | ready → applied | failed | cancelled`.
- `assignment_proposals(id, job_id, file_path, file_size, kind, season, episode, episode_id, absolute_number, confidence, source, reason, decision)`.
  - `kind`: `episode|special|movie|extra|sample|subtitle|unknown`.
  - `decision`: `pending|accepted|rejected|edited`.
- `ai_usage(id, feature classifier|recs|ask, model, input_tokens, output_tokens, cache_read_tokens, web_searches, cost_usd numeric, ref_id, created_at)`. Shared by every AI feature.

**`005_trakt.sql` (B5)**
- `trakt_auth(id=1, username, access_token_enc, refresh_token_enc, expires_at, created_at, updated_at)`.
- `trakt_history(trakt_history_id bigint pk, media_type, tmdb_id, show_tmdb_id, season, episode, watched_at)`.
- `trakt_ratings(media_type, trakt_id, tmdb_id, show_tmdb_id, season, episode, rating, rated_at, pk(media_type, trakt_id))`.
- `trakt_watchlist(media_type, tmdb_id, trakt_id, listed_at)`.
- `trakt_show_progress(show_tmdb_id pk, aired, completed, last_watched_at, next_season, next_episode, reset_at, updated_at)`.
- `trakt_sync_state(key pk, value jsonb)`. Stores the `last_activities` snapshot and cursors.

**`006_recommendations.sql` (R1/R2)**
- `taste_profiles(id, created_at, profile jsonb)`.
- `rec_runs(id, kind nightly|manual, status, model, profile_id, progress jsonb, error, started_at, finished_at)`. Usage goes into `ai_usage` with `ref_id`.
- `rec_shelves(id, run_id, position, slug, title, subtitle)`.
- `rec_items(id, shelf_id, position, media_type, tmdb_id, reason, signals jsonb)`.
- `rec_feedback(media_type, tmdb_id, signal like|dislike|not_interested|seen, created_at, pk(media_type, tmdb_id))`.
- `ask_threads(id, title, created_at, updated_at)` and `ask_messages(id, thread_id, role, content jsonb, created_at)`.
  - `content` holds the raw API content blocks, append-only. Tool loops and thinking replay need them unchanged.

## 6. API contract (backend, under `/api`)

**Conventions**
- JSON throughout. Errors are `{error: {code, message}}` (implemented in G2 for every route: `internal/api/errors.go` derives `code` from the status: `bad_request`, `unauthorized`, `forbidden`, `not_found`, `conflict`, `validation_error`, `rate_limited`, `unavailable`, `timeout`, `internal_error`; `errorCodeResponse` sets a specific one). Paginated lists (`/library`, `/me/history`, `/search/torrents`) are `{items, total, page, page_size}`; short lists (up next, watchlist, threads, episode groups, torrent files, `/torrents`) are `{items}`.
- Long operations return `202 {job_id | run_id}` and are polled.
- Every route requires `X-Momoshtrem-Token` when `BACKEND_API_TOKEN` is set; otherwise `401 {error: {code: "unauthorized"}}` (G2). The web client maps a backend 401/403 to `502 backend_auth`, so a token mismatch never looks like an expired session.
- CORS is an allowlist (`CORS_ALLOWED_ORIGINS`), empty by default, so no CORS headers are sent. Allowed origins may use `GET, POST, PUT, PATCH, DELETE, OPTIONS` and the `Content-Type`, `Authorization`, `X-Momoshtrem-Token` headers.
- The web app exposes the same paths under its own session-checked `/api/*` routes. A catch-all `web/src/app/api/[...path]` forwards any path whose first segment is in `BACKEND_PREFIXES` (`web/src/lib/api/backend-proxy.ts`), adding the token and streaming the response (SSE included); explicit web routes take precedence. Typed browser calls and TanStack Query factories for every endpoint live in `web/src/lib/api/endpoints/`, and MSW fixtures in `web/src/mocks/` (F1).

### Titles and library (B1)
- `GET /titles/{movie|tv}/{tmdb_id}`: cached TMDB details, plus library state, assignment summary, watch state (once B5 lands) and the recommendation reason if there is one.
- `GET /library?type=&status=ready|incomplete|needs_review|watching&sort=&q=&page=`: items with poster and counts (`episodes_total`, `episodes_ready`, `episodes_watched`, `needs_review`). Replaces the per-item TMDB calls.
- `PATCH /shows/{id}` with `{episode_ordering, episode_group_id, include_specials, classifier_mode}`.
- `GET /shows/{id}/episode-groups`: `{items: [{id, name, type, group_count, episode_count}]}`, TMDB episode groups for the ordering picker.

### Assignment (B2/B3)
- `POST /shows/{id}/assign-torrent` with `{magnet_uri, hint?: {season?, episode?}}` returns `202 {job_id}`. `?wait=true` keeps the legacy synchronous behaviour until H2.
- `POST /movies/{id}/assign-torrent` with `{magnet_uri, file_path?}` creates a job. If several plausible movie files are found, the job goes to `needs_review`.
- `GET /assignment-jobs/{id}` returns `{job, groups: {matched, needs_review, specials, extras, unmatched}, summary}`. Each item is `{id, file_path, file_size, kind, target: {episode_id, season, episode, name} | null, confidence, source, reason, decision}`.
- `PATCH /assignment-jobs/{id}/proposals/{pid}` with `{episode_id}` or `{kind: "ignore"}`.
- `POST /assignment-jobs/{id}/reclassify` with `{scope: "unresolved"|"all", hint?: string}`. Re-runs the AI resolver, optionally with a user hint such as "absolute numbering" or "these are the OVAs".
- `POST /assignment-jobs/{id}/apply`: writes the assignments and updates the VFS.
- Manual override: `PUT /episodes/{id}/assignment` and `PUT /movies/{id}/assignment`, both with `{info_hash, file_path}`.
- `GET /torrents/{hash}/files`: `{items: [{path, size, mapped_to}]}`, every file with its size and current mapping (`mapped_to` is an episode target, `{movie_id}` or null).

### Search (B4)

`GET /search/torrents` parameters:

| Group | Parameters |
|---|---|
| Context | `media_type=movie|tv`, `tmdb_id`, `season`, `episode`, `q` |
| Filters | `resolution`, `source`, `codec`, `hdr`, `pack`, `min_seeders`, `max_size_gb`, `indexer` |
| Sort and paging | `sort=best|seeders|size|newest`, `page`, `page_size` |

Response:
```
{
  items: [Release],
  facets: { resolution: {"2160p": 12, …}, source, codec, hdr, pack, indexer },
  total, page, page_size,
  queries: [...],   // what was sent to Jackett
  cached_at,
  warnings: [...]   // e.g. "2 indexers timed out"
}
```

Each `Release`:
```
{
  id, raw_title, display_title,
  info_hash?, magnet_uri?,
  size_bytes, seeders, leechers, published_at, indexer,
  attrs: {
    resolution, source, codec,
    hdr: [], audio: [], group, languages: [], edition, repack,
    pack: {
      type: "episode"|"multi_episode"|"season"|"multi_season"|"complete",
      seasons: [], episodes: []
    }
  },
  score, score_reasons: [],
  flags: ["cam", "no_seeders", "size_outlier", "needs_torrent_fetch"]
}
```

Related endpoints:
- `POST /search/torrents/inspect` with `{release_id | magnet_uri, media_type, tmdb_id}` returns `202 {job_id}`. The job is a dry run (`dry_run=true`) with the same shape as an assignment job, and it is never applied.
- `GET /settings/quality-profile` and `PUT /settings/quality-profile` with `{preferred_resolution, min_resolution, max_resolution, prefer_codecs[], prefer_hdr, max_gb_per_hour, avoid_terms[], preferred_groups[]}`.

### Trakt (B5)
- `GET /integrations/trakt` returns `{configured, connected, username, last_sync_at, sync_status, counts: {history, ratings, watchlist}}`.
- `POST /integrations/trakt/connect` returns `{user_code, verification_url, expires_at, interval}`. The backend polls Trakt itself; the client polls `GET /integrations/trakt`.
- `POST /integrations/trakt/sync` returns `202`.
- `DELETE /integrations/trakt` disconnects and purges the imported data.
- `GET /me/up-next` returns `{items: [{show, next_episode, availability, airs_at, progress: {completed, aired}, last_watched_at}]}`.
  - `availability` is one of `ready`, `downloading`, `needs_torrent`, `not_in_library` or `airs_on`.
- `GET /me/watchlist?type=`: `{items}` with library and availability state.
- `GET /me/history?page=`: paginated.
- Watch state (`watched`, `progress`, `rating`) is also embedded in the `/titles` and `/library` responses.

### Recommendations (R1–R3)
- `GET /recommendations/home` returns:
  ```
  {
    run: { id, finished_at } | null,
    hero_picks: [{ tmdb_id, media_type, kicker, reason }],   // "Tonight's pick", stable per day
    shelves: [{
      slug, title, subtitle,
      items: [{ media_type, tmdb_id, title, year, poster_path, reason, in_library, on_watchlist }]
    }]
  }
  ```
  It falls back to trending shelves when there's no run yet or Trakt isn't connected.
- `POST /recommendations/refresh` returns `202 {run_id}`.
- `GET /recommendations/runs/{id}`: status, progress steps, usage and cost.
- `POST /recommendations/ask` with `{thread_id?, message}` returns `text/event-stream` with these events:

  | Event | Payload |
  |---|---|
  | `status` | `{step, detail}` |
  | `item` | rec card |
  | `text` | delta |
  | `error` | |
  | `done` | `{thread_id, usage}` |

  The web route passes the stream through unbuffered.
- `GET /recommendations/threads` (`{items: [{id, title, created_at, updated_at}]}`) and `GET /recommendations/threads/{id}` (the thread plus its `messages`).
- `POST /recommendations/feedback` with `{media_type, tmdb_id, signal}`.
- `GET /settings/ai` and `PUT /settings/ai` with `{rec_enabled, rec_model, rec_schedule_cron, monthly_budget_usd, classifier_enabled, classifier_model, auto_apply_high_confidence}`.
- `GET /settings/ai/usage`: month-to-date cost by feature.

### System
- `GET /status[?refresh=1]` (G2) returns:
  ```
  {
    version, uptime_seconds, checked_at,          // checked_at: RFC 3339, when the probes ran
    db: { ok, size_bytes, error? },
    webdav: { url, port, auth_enabled, username?, active_readers? },  // username only when auth is on
    jackett: { configured, reachable, latency_ms?, error? },
    tmdb: { configured, reachable, latency_ms?, error? },
    trakt: { configured, connected },             // connected is always false until B5
    ai: { configured, mtd_cost_usd },             // mtd_cost_usd is null until B2
    opensubtitles: { configured }
  }
  ```
  - Probes run concurrently with a 3 s timeout each. Results are cached for 30 s, and concurrent callers share one check. `?refresh=1` bypasses the cache at most once every 5 s.
  - It returns 200 even when a probe fails (the body says what's wrong) and 503 only when the database ping fails. The response never contains secrets.
  - Per-indexer health (`jackett.indexers`, HANDOFF §6) is added by B4.
  - The web app's own `GET /api/status` (F1) adds `web: {version}`. A backend 503 (database down) is passed through as 200 with the same body, so Settings can show which service is down. Only a backend that can't be reached becomes `503 {error: {code: "backend_unreachable"}, web, webdav: {reachable}}`, probing WebDAV directly (`WEBDAV_INTERNAL_URL`, default: the API host on port 36911) so the offline page can say whether Infuse still plays.
- `GET /torrents` (B6) returns `{items}`. It merges torrents known from the DB but not loaded (`idle`) with live stats. It returns real `download_speed`/`upload_speed`, and each torrent links to its titles.

## 7. Phases and tasks

### Phase 0: Groundwork

**G1 · Rename to momoshtrem** (run alone; it touches almost every web file)
- First commit or stash the uncommitted `CLAUDE.md` edit. Leave `Distribyted Codebase Overview.md` alone.
- Rename the frontend:
  - `git mv PrettyTVCatalog web` and set the package name to `momoshtrem-web`.
  - Compose service `web`, container `momoshtrem-web`.
  - `Caddyfile.example` (and the local `Caddyfile`): `reverse_proxy web:3000`.
- Rename remaining brand strings:
  - Cookie `momoshtrem_session` (you'll need to log in again once).
  - User-Agent string in `src/lib/utils/torrent.ts`.
  - Metadata title and description.
  - Login footer ("distribyted" → momoshtrem).
- Docs and Next.js:
  - `git mv docs/prettytvcatalog docs/web`.
  - Update paths in CLAUDE.md, README, CONTRIBUTING and the code-quality docs.
  - Rename Next 16 `middleware.ts` → `proxy.ts`.
- Acceptance:
  - `grep -riE "prettytv|pretty tv" --exclude-dir=node_modules .` finds nothing outside historical docs. "distribyted" survives only in the architecture-history docs.
  - `npm run build` and `npm test` pass in `web/`, and `docker compose build web` succeeds.

**G2 · Backend config and security baseline**
- Fix `config.Load` so env overrides apply with or without `config.yaml`, and add tests.
- Wire the new env vars (§4). Missing optional ones disable their feature, and `/status` reports it.
- Add middleware that requires `X-Momoshtrem-Token` on `/api/*`. It's disabled when the token is unset, with a startup warning.
- Restrict CORS. Make the web client send the token header.
- Migration `002_app_settings.sql` and a small settings repository.
- Update `config.yaml.example`, `.env.example` and the docs: SQLite → Postgres.
- Acceptance:
  - `go test ./...` passes.
  - A container started without `config.yaml` honours `DATABASE_URL`.
  - When a token is configured, a request without it gets 401.

**G3 · Login that password managers understand** (a quick win that ships on the old UI)
- **Credentials.** Add `APP_USERNAME`, and compare both username and password with `timingSafeEqual`.
- **Server-rendered form.** The server page reads `searchParams` and passes them as props, removing the `useSearchParams` Suspense bail-out, so the form is in the initial HTML.
- **Markup:**
  - `<form method="post" action="/api/auth">`.
  - Username: `name="username"`, `type="text"`, `autocomplete="username"`, `autocapitalize="none"`, `spellcheck="false"`.
  - Password: `name="password"`, `autocomplete="current-password"`.
  - While submitting, disable only the button, not the inputs.
- **Submit flow.** JS-enhanced submit; on success do a **full navigation** (`window.location.assign`) to the validated redirect. Without JS, the POST returns a 303 redirect.
- **Redirect rules.** `redirect` must be a relative path starting with `/` but not `//`. An authenticated visit to `/login` redirects away.
- **Logout.** Add a temporary logout button in the header.
- **Expired sessions.** Unauthenticated `/api/*` requests get 401 JSON (not a 307), and client fetches redirect to `/login` on a 401.
- **Rate limiter.** Trust `X-Forwarded-For` only from a configured proxy; otherwise use the socket address.
- Acceptance:
  - Jest tests cover redirect validation, the 401 behaviour and username checks.
  - Manual check (best after G4): Chrome and Safari offer to save the login and then autofill it.

**G4 · HTTPS** (infra; needs the user for router and port changes)
- Build Caddy with the `caddy-dns/duckdns` module and use DNS-01 (`DUCKDNS_TOKEN` is already in `.env`) to get a real certificate for `momoshtrem.duckdns.org`. This replaces `tls internal`.
- Serve web through Caddy on 443 (or a chosen port).
- Stop publishing `3000:3000` and set `SECURE_COOKIES=true`.
- Move WebDAV onto the same certificate.
- Acceptance: a valid certificate in Chrome and Safari; port 3000 isn't reachable from outside; Infuse still connects to WebDAV.

### Phase 1: Backend tracks (start after G2; B-tasks don't touch `web/`)

**B1 · Metadata foundation**
- Owns `internal/tmdb`, `internal/library`, migration 003, `internal/api/titles_routes.go`, `library_routes.go`.
- **TMDB client:**
  - `append_to_response` (external_ids, keywords, credits, videos, recommendations, similar).
  - Season 0.
  - `/tv/{id}/episode_groups` and `/tv/episode_group/{id}`.
  - Retries with backoff, 429 handling, a small rate limiter, and a `language` parameter.
- **Persistence.** Write the new columns on create and refresh. Add a backfill command (`momoshtrem backfill-metadata`) for the existing library.
- **Absolute numbers.** Take them from the selected episode group when ordering is `episode_group` or `absolute` and a group exists. Otherwise use the cumulative index of the regular seasons in aired order.
- **Specials.** Store season 0 when `include_specials` is set (the default). Air-date sync covers season 0 too.
- **`tmdb_cache` TTLs.** Details 7 days; seasons 1 day for airing shows and 30 days for ended ones.
- **Endpoints.** `/titles`, `/library` (a single query; also fix the `getShow` N+1), `PATCH /shows/{id}` and `/shows/{id}/episode-groups`.
- Acceptance:
  - Unit tests cover absolute numbering, specials and cache TTLs.
  - `/library` runs a constant number of queries.
  - Existing endpoints are unchanged.

**B2 · Classifier v2 (rules), assignment jobs and manual overrides** (depends on B1)
- Owns `internal/identify`, `internal/service/assignment_*`, `internal/vfs` (specials) and migration 004.
- **Build the corpus first.** Write `internal/identify/testdata/corpus/*.yaml` with at least 150 real-world paths and expected results; the corpus is the spec. Cover:
  - Scene `SxxEyy`, multi-episode (`S01E01E02`, `S01E01-E03`) and `NxNN`.
  - Anime fansubs (`[Group] Title - 05 (1080p) [CRC32].mkv`, `v2`, batch folders) and absolute numbering (One Piece, Detective Conan).
  - Specials: OVA, OAD, ONA, NCOP, NCED, and "Special" inside episode titles.
  - Daily and date-based shows.
  - Season packs with `Show.S01.1080p/` folders, multi-season complete packs, and `Season 1/01 - Pilot.mkv`.
  - Show titles that contain "Special", "Trailer", "Sample" or "Extras".
  - Scene season splits that TMDB doesn't have, e.g. Frieren releases `S02E01–E09` are TMDB `S01E29–E37`.
  - Resolution and year false positives.
  - Subtitle folders (`Subs/<ep>/2_English.srt`).
- **Parser rewrite:**
  - Tokenise first.
  - Strip resolution (`\d{3,4}x\d{3,4}`, `\d{3,4}[pi]`), CRCs, codecs and years before matching.
  - Generate **multiple scored candidates** instead of taking the first hit.
  - Skip rules use token boundaries and know the show title, so words that are part of the title aren't treated as markers.
  - Record skipped files with a reason.
- **Torrent-level context:**
  - Detect pack type: season, multi-season, complete, or absolute range.
  - Use the group tag, and read the folder season (`S01` anywhere in a folder token).
  - Consistency checks: sequential numbers in a folder; numbers above the season length imply absolute numbering.
- **Matching order:**
  1. Aired S/E.
  2. Absolute number.
  3. Air date.
  4. Season-offset remap, for when the release's season doesn't exist in TMDB or exceeds its episode count. Evidence for the remap comes from:
     - TMDB episode groups;
     - cumulative episode counts;
     - air dates;
     - consistency across the pack (e.g. S02E01 → S01E29).
  5. Special by number.
  6. Special or episode by title similarity (normalised episode names found in the file name).
- **Specials.** Remove the `special_not_supported` rejection; specials map to season 0.
- **Jobs.** Assignment runs in a background worker, so the 60 s request block goes away, and proposals are persisted.
  - If everything is high confidence and there are no conflicts, the job auto-applies (controlled by the `auto_apply_high_confidence` setting). Otherwise it goes to `needs_review`.
  - Two files targeting the same episode → `needs_review`, never "last one wins".
- **Movies.** Return a ranked list of candidate files; collection packs go to `needs_review`.
- **Manual overrides** and `/torrents/{hash}/files`.
- **VFS:**
  - Specials go to `Season 00/Show - S00Exx - Name.ext`.
  - Multi-episode files get a single entry `S01E01-E02` instead of one file per episode (verify that Infuse handles this).
  - Existing paths stay stable for items that are already assigned.
- **Resolver interface:** `type Resolver interface { Resolve(ctx, ResolveInput) (ResolveOutput, error) }`, with `NoopResolver` as the default. Merge this interface early so B3 can start.
- Acceptance:
  - The rules alone exactly match ≥95% of the corpus's unambiguous cases.
  - Every failure listed in §3 passes.
  - The legacy endpoint still works with `?wait=true`.
  - Specials appear in the WebDAV tree.

**B3 · AI resolver (Haiku) and eval** (depends on B2's Resolver interface)
- Owns `internal/identify/ai/` and `cmd/classifier-eval`.
- **When it runs:** any unresolved, low-confidence or conflicting files; `classifier_mode=ai_always`; or the user's "Re-check with AI" (with an optional hint).
- **Input:**
  - The torrent name.
  - The file tree (path, size). For more than 400 files, collapse obvious sequences and send only unresolved files plus their neighbours.
  - The show: title, original title, year, `is_anime`, ordering.
  - Seasons with their episode counts.
  - Episodes, including season 0: S/E, absolute number, name, air date, type.
- **Call:**
  - `claude-haiku-5-5` with structured outputs (`output_config.format`, a JSON schema):
    ```
    {files: [{path, kind, season, episodes: [int], absolute_number, confidence, reason}]}
    ```
    Keep `reason` to 120 characters or fewer.
  - Low effort.
  - Size `max_tokens` to the file count.
  - Put the stable system prompt first so prompt caching works.
- **Validation.**
  - Every AI mapping must point at an existing episode row.
  - The AI never overrides a `high` rules result.
  - AI-only mappings go to `needs_review` unless they're `high` confidence and the setting allows auto-apply.
- **Caching and cost.** Cache results by `(info_hash, show_id, metadata_version, hint)`. Record usage in `ai_usage`. If `ANTHROPIC_API_KEY` is missing, degrade quietly to rules-only.
- **`cmd/classifier-eval`.** Runs at least 20 golden torrent file lists:
  - One Piece batch, Frieren (including the scene "S02" split), Doctor Who with specials, Bluey.
  - Law & Order: SVU, Trailer Park Boys.
  - A daily show, a complete-series pack, an OVA collection.

  It reports accuracy, cost and latency. Run it manually, since it costs money.
- Acceptance: at least 95% file-level accuracy on the golden set, with average cost per torrent reported in the PR.

**B4 · Torrent search service** (independent; `inspect` uses B2 jobs, so stub it until B2 merges)
- Owns `internal/search`, `internal/release`, `internal/api/search_routes.go` and the quality-profile settings.
- **Torznab client:**
  - Query `t=caps` at startup and cache it.
  - Search types `t=search|tvsearch|movie`.
  - Categories `cat` (2000 movies, 5000 TV, 5070 anime).
  - `imdbid`/`tvdbid`/`tmdbid` where the indexer supports them, plus `season`/`ep` and `limit`/`offset`.
- **Query plan per request.** Run in parallel:
  - an ID-based query;
  - a text query (title + year, `SxxEyy`, or season);
  - the original or alternative title for anime.

  Merge the results and dedupe by info hash, falling back to normalised title + size.
- **Query normalisation.** Strip `:` `'` `!` `,` and similar from titles, try both "Title" and "Title Year", and fall back to a show-level query with server-side coverage filtering when an exact `SxxEyy` query returns nothing.
- **Anime numbering.** Also query the absolute episode number and any scene numbering derived from B1's episode groups and offsets (e.g. TMDB S01E29 ↔ release S02E01).
- **Hygiene:**
  - Decode HTML entities in titles.
  - Always send categories, and exclude adult categories (6000-range) and anything flagged NSFW.
  - Report which indexers answered and which failed, in `warnings`.
- **Ops note for the user.** Jackett currently has only one indexer configured. Add more, including an anime-focused one, or no search code will help.
- **`.torrent` → magnet.** Port the conversion from the web app, including the SSRF guard. Resolve lazily on add when needed, and surface failures as flags instead of dropping them.
- **Release parser** (`internal/release`, sharing the tokenizer with `identify`). Extracts:
  - Resolution and source (REMUX, BluRay, WEB-DL, WEBRip, HDTV, CAM/TS).
  - Codec, HDR / DV / HDR10+, and audio.
  - Group, languages (MULTi, Dual Audio), edition, and repack/proper.
  - Season/episode coverage and pack type.

  Table-driven tests with at least 100 release names.
- **Scoring with explanations** (`score_reasons`):
  - How well the release fits the quality profile.
  - Seeders (log scale).
  - Coverage fit: for an episode query, exact episode > season pack > complete, and the reverse for a season query.
  - Size sanity (GB per hour of runtime) and freshness.
  - Penalties for CAM, zero seeders and size outliers.
- **Caching and paging.** An in-memory cache (10 minutes) keyed by query plan. Filtering, sorting, pagination and facets are computed server-side from the cached set.
- **Display title.** Turn dots and underscores into spaces where safe; keep `raw_title` as well.
- Acceptance:
  - Parser tests pass.
  - An integration test runs against recorded Torznab fixtures.
  - Page 2 is served from the cache.
  - p95 is under 4 s with Jackett warm.

**B5 · Trakt, read-only** (depends on B1)
- Owns `internal/trakt`, migration 005 and `internal/api/trakt_routes.go`, `me_routes.go`.
- **Verify first.** Check endpoint shapes, token lifetime and rate limits against the official Trakt API docs before coding. The details below come from secondary sources.
- **Device-code OAuth.**
  - Flow: `POST /oauth/device/code`, then poll `/oauth/device/token` every `interval` seconds.
  - Tokens are encrypted with `SECRET_KEY`.
  - Access tokens reportedly expire after about 24 hours (a Trakt change in March 2025). Refresh them proactively and retry once on a 401. If the refresh fails, mark the integration disconnected and show that in the UI.
- **Requests.**
  - Headers: `trakt-api-version: 2`, `trakt-api-key`, and the Bearer token.
  - Honour `Retry-After` on 429.
  - Paginate with `page`/`limit` and the `X-Pagination-*` headers.
- **Sync:**
  - Every 30 minutes, poll `/sync/last_activities`.
  - When something changed, pull only the deltas: history (`/sync/history/{movies,episodes}?start_at=`), ratings, watchlist, and watched shows and progress.
  - Run a full reconcile nightly, and show progress during the initial import.
- **ID mapping.** Map via `ids.tmdb` and keep the Trakt IDs too. Fetch unknown TMDB IDs into `tmdb_cache` lazily.
- **Up next.** For each in-progress show, find the next episode (skipping specials unless they've been watched). Its availability comes from the library, assignments and air dates.
- **Optional inputs for R1:** Trakt `/recommendations/{movies,shows}` and `/calendars/my/shows`.
- Acceptance:
  - Tests use a fake Trakt server.
  - Connect and disconnect work end to end.
  - Incremental sync fetches only deltas.
  - Up next is correct for a show with specials and for a show whose next episode hasn't aired.

**B6 · Torrent and activity hygiene** (independent)
- Owns `internal/torrent` and `internal/api/torrent_routes.go`.
- Compute real download and upload speeds from the delta of `Stats()` byte counts over time.
- `/torrents` lists torrents known from the DB, merged with live state (`idle` when not loaded), and links each one to its titles.
- Route pause and resume through the ActivityManager.
- Honour `delete_data`.
- Delete orphaned subtitles when items or torrents are removed.
- Acceptance: tests for the speed calculation and the merged listing. The Activity view is never empty while the Library has assigned items.

**B7 · Subtitles polish** (small; independent)
- Owns `internal/subtitle`, `internal/opensubtitles` and a migration amendment in 004 (coordinate with B2) or a separate `007_subtitles.sql`.
- Rank OpenSubtitles results by match to the assigned release:
  - movie hash, if available;
  - release-name similarity;
  - group/source/resolution match;
  - downloads.

  Return `match_score` and `match_reason`.
- Include `source` in the uniqueness key so a torrent-embedded subtitle doesn't overwrite a downloaded one. Give "unknown"-language files distinct keys.
- Clean up subtitles when items or torrents are deleted (shared with B6; whichever merges second rebases).
- Acceptance: the best-matching subtitle for a YIFY release is ranked first in a fixture test.

### Phase 2: Recommendations (after B1 and B5)

**R1 · Taste profile and candidate pool** (deterministic, no LLM)
- Owns `internal/recommend/profile`, `internal/recommend/candidates` and migration 006.
- **Taste profile.** Weighted by recency, rating and completion. It covers:
  - Top genres, keywords, creators and cast.
  - Decades, languages and origin countries.
  - Anime share, the movie/show ratio and runtime tolerance.
  - **Loved:** rated 8 or higher, or completed.
  - **Dropped:** started, under 30% watched, and idle for 60 days.
  - **Recent:** the last 30 days.
  - **Aversions:** low ratings and `rec_feedback`.

  Stored in `taste_profiles` as compact JSON, targeting 4k tokens or fewer.
- **Candidate pool.** Sources:
  - TMDB recommendations and similar titles for the top loved titles.
  - Trakt recommendations.
  - TMDB discover by top keywords and genres, within recency windows.
  - Trending, upcoming and on-the-air.

  Exclude anything watched, in the library, disliked or marked not-interested. Pre-rank by overlap and keep a pool of about 150 with compact metadata.
- Acceptance: unit tests on fixtures, plus a dev-only `GET /recommendations/debug/profile`.

**R2 · Nightly recommendation agent and shelves** (depends on R1)
- Owns `internal/recommend/agent`, the scheduler wiring and the `/settings/ai` routes.
- **Agent setup:**
  - The Go SDK tool runner, with the model taken from settings (default `claude-opus-5-5`).
  - Adaptive thinking, effort `medium`.
  - Server-side refusal fallbacks (`fallbacks: "default"` with beta `server-side-fallback-2026-07-01`).
  - Prompt caching on the system prompt and tools. Keep those bytes stable and put the date and profile in the user turn.
- **Tools:**
  - Server-side `web_search_20260209` (`max_uses` ≈ 8) and `web_fetch_20260209` (`max_uses` ≈ 4), for freshness: new releases, upcoming premieres, festival and awards buzz.
  - Custom tools: `get_taste_profile`, `get_candidates(filters)`, `tmdb_search(query, type, year)`, `tmdb_details(type, id)`, `check_seen(tmdb_ids)`.
  - `submit_shelves` with `strict: true`; calling it ends the run.
- **Output.** 4–6 shelves (e.g. "Because you loved Severance", "New this month for you", "Anime you'll like", "Hidden gems", "Coming soon") with 8–15 items each. Every item carries a one-line reason grounded in the profile. Every item must resolve to a TMDB ID; the server validates them and drops invalid ones.
- **Guardrails.**
  - Cap tokens and tool calls per run.
  - Enforce the monthly budget from settings: when it's exceeded, skip the run and say so in the UI.
  - Record usage and cost in `ai_usage` and keep a run history.
- **Scheduler.** Configurable cron (default 04:00 local), a manual refresh endpoint, and single-flight so two runs never overlap.
- Acceptance:
  - CI tests use a mocked API, with no network.
  - One real run is documented in the PR, with its cost.
  - The shelves endpoint serves from the DB.

**R3 · Ask momoshtrem** (depends on R2)
- Owns `internal/recommend/ask` and the SSE handler.
- **Agent.** The same tools as R2, with conversation history loaded from `ask_messages`. Append the raw content blocks unchanged; never edit earlier turns.
- **Streaming events:**
  - `status` events from tool calls ("Checking your history…", "Searching the web: …").
  - An `item` event whenever a title is resolved.
  - `text` deltas, and `done` with usage.
- **Refinement.** Chips like "lighter", "shorter" or "only anime" are simply follow-up messages.
- **Feedback.** It's written to `rec_feedback` and shapes the next run.
- **Limits.** A daily rate limit (e.g. 30 asks) and the budget guard.
- Acceptance:
  - An SSE contract test.
  - The web route streams without buffering (`Cache-Control: no-transform`, `X-Accel-Buffering: no`; check that Caddy doesn't buffer either).

**Cost ballpark** (verify with real usage):
- The classifier costs a fraction of a cent per torrent on Haiku 5.5.
- A nightly run on Opus 5.5 is probably around $0.5–1, so roughly $15–30 a month; about half that on Sonnet 5.5.
- Web searches are billed per search on top of tokens.
- The settings page shows month-to-date spend.

### Phase D: Design (involves the user; runs in parallel with Phase 1)

- **D1.** Run two Claude Design sessions, one with `design-prompt-cinematic.md` and one with `design-prompt-playful.md`. Attach `docs/screenshots/*.png` as "current UI, for context only".
- **D2.** Pick a direction, or merge (e.g. A's layout with B's mascot moments). Export the results into `docs/redesign/design/`:
  - Screens (PNG/PDF).
  - Tokens (CSS variables or JSON).
  - Logo SVGs: mark, wordmark, mono and app icon.
  - Component specs and handoff notes.
- **D3** (an agent task). Write a design adoption spec:
  - Map the designed components to an implementation list.
  - Feed any data the design needs that §6 lacks back into §6.
  - Rewrite `docs/web/STYLE_GUIDE.md` for the new system.

### Phase 3: Frontend rebuild (after D3; use mocks until backend tasks land)

**F1 · Foundation and shell** (serial; blocks F2–F8)
- Owns `web/src/app/layout.tsx`, `globals.css`, `components/ui`, `components/layout`, `lib/api`, `mocks/` and `public/`.
- **Tokens and assets.**
  - Tokens go in Tailwind v4's CSS-first `@theme`; delete the dead `tailwind.config.ts`.
  - Fonts via `next/font`.
  - An icon set (e.g. `lucide-react`) replaces the inline SVGs.
- **Primitives, rebuilt from the design:**
  - Controls: Button (no forced full width), IconButton, Input, Select, Chip/FilterChip, SegmentedControl, Tabs.
  - Display: Badge/StatusPill, Progress, Skeleton, EmptyState/ErrorState with the cat.
  - Overlays: Sheet (a bottom sheet on mobile, a dialog on desktop; stack-safe, with unique IDs), Toast, Tooltip.
  - Media: Poster, Backdrop and Still images with placeholders.
  - CommandPalette.
- **Data layer:**
  - TanStack Query for caching, invalidation after mutations, and polling that pauses when the tab is hidden.
  - A typed backend client aligned with §6.
  - A 401 response sends you to `/login`.
  - MSW handlers and fixtures for every §6 endpoint, used by a dev "mock mode" and by tests.
- **App shell:**
  - Navigation: a desktop top bar or side rail, and a mobile bottom tab bar that respects safe areas.
  - User menu (Settings, Log out) and ⌘K global search.
  - Per-route metadata titles.
  - Branding: PWA manifest and icons (iOS home screen), favicon and OG image.
- **Login page** in the new design, keeping G3's behaviour.
- Acceptance:
  - A dev-only `/dev/ui` component gallery.
  - No horizontal overflow at 320 px.
  - Lighthouse accessibility ≥ 95 on the shell.

Then, in parallel (they live in separate route folders):

- **F2 · Title detail (movie and show)** (depends on B1; B5 for watch state)
  - **Hero and info:** hero, metadata, cast and crew, trailer, "why recommended", more like this, watched and rating state.
  - **Availability state machine:** Add → Find torrent → Downloading → Ready in Infuse, with a "copy WebDAV path" action.
  - **Show-specific:**
    - Season tabs including Specials.
    - Episode states: watched, ready, missing, downloading, airs on, needs review.
    - A needs-review banner.
    - Show settings: ordering or episode group, specials, classifier mode.
    - **Long seasons** (Frieren has 38 episodes, One Piece 1,000+): a compact row mode, virtualised lists, range/arc chunks and "jump to episode".
  - **Primary action** reflects the real state. Once a title is ready, the torrent card moves above the fold and "Search torrents" becomes secondary.
  - **Subtitles** panel, ranked by release match (B7).
- **F3 · Torrent search sheet** (depends on B4; B2 for Inspect)
  - **Context-aware defaults.** Movie, season or episode, with a scope switch: Episode · Season pack · Any.
  - **Filters.** Chips with facet counts; on mobile they open in a bottom sheet. Sort and paginate ("Showing 25 of 143 · Load more").
  - **Best match.** A highlighted card with its score reasons.
  - **Release names.** They wrap (`min-w-0`, `overflow-wrap:anywhere`) and come with attribute chips.
  - **Before adding.** Inspect previews the files and mapping before Add.
  - **States.** Partial-failure warnings and an empty state. When filters cause zero results, say which ones and offer to clear them.
  - **Stability.** The sheet keeps a stable height (no jumping while results load), and the query and filters survive close and reopen within a session.
  - **Clear labels.** "Add" says what it does ("Add to library & assign").
- **F4 · Mapping review** (depends on B2 and B3)
  - **Progress.** Metadata → matching → AI.
  - **Results.** Grouped into Matched, Needs review, Specials, Extras & ignored, and Unmatched, each with confidence and reason.
  - **Editing.** An inline episode picker (searchable, grouped by season including Specials). "Re-check with AI" with a hint field, then Apply.
  - **Entry points.** Also reachable later from the show page and from Activity.
- **F5 · Home "For you" and Ask** (depends on B5, R2 and R3)
  - **Home rows.** Up next (with an availability call to action), new episodes for your shows, and AI shelves with reasons and feedback.
  - **More rows.** Watchlist, and trending when Trakt isn't connected.
  - **Ask** (page or drawer). Streaming status steps, result cards, refine chips and threads.
- **F6 · Discover and search** (no new backend dependencies; TMDB via web)
  - **Filters.** Several genres at once, year range, rating, runtime, language/origin with an anime toggle, type and sort.
  - **URL state and loading.** Filters stay in the URL (e.g. `nuqs`); results load with infinite scroll.
  - **Search.** Type tabs and the ⌘K palette.
- **F7 · Library and Activity** (depends on B1 and B6)
  - **Library browsing.** Grid and list views; filters (type, ready, incomplete, needs review, watching); sort and search.
  - **Library cards.** Ready and watched progress per show; actions visible on touch, with confirmations.
  - **Activity.** Torrents linked to their titles, with speeds, peers and progress, plus pause, resume and remove.
  - **Advanced.** A collapsed section with the piece map, readers and files.
- **F8 · Settings** (depends on B4, B5 and R2)
  - **Integrations.** Trakt, connected with the device-code flow, plus sync status.
  - **AI.** Model, schedule, budget and usage.
  - **Quality profile** and **Playback** (the WebDAV URL and an Infuse setup guide).
  - **System status.**

Acceptance for every F task:
- It matches the design at 390 px and 1440 px.
- No horizontal overflow at 320 px.
- Titles and actions are visible without hover.
- Destructive actions are visually secondary and confirmed.
- Grids use fluid columns (no fixed card widths in fixed tracks).
- Loading, empty and error states are in place.
- Keyboard and screen-reader basics work.
- A Playwright test covers the page's main flow against the MSW mocks.

### Phase 4: Hardening and release

- **H1 · End-to-end and quality**
  - A Playwright suite with mobile and desktop projects covering:
    - The login autofill attributes.
    - Ask → add → search → inspect → assign → review → ready.
    - Up next → missing episode → search on a phone.
    - Trakt connect (mocked).
  - axe accessibility checks.
  - An overflow assertion (`scrollWidth <= innerWidth`) on every route.
- **H2 · Cleanup**
  - Remove the legacy pieces: `?wait=true`, the web Jackett route and env, the old hooks and components.
  - Update docs (README, CLAUDE.md, CODE_QUALITY, STYLE_GUIDE, API reference) and refresh the screenshots.
- **H3 · Release**
  - Prepare: a `pg_dump` backup, then the migrations and the backfill command, then deploy.
  - Smoke test on Apple TV/Infuse: the specials folder, anime absolute mapping, multi-episode files and subtitles.
  - Connect Trakt, run the first nightly recommendation run, and watch costs for a week.

## 8. Dependency graph and waves

```
G1 ──► G3
G2 ─┬─► B1 ─┬─► B2 ─► B3
    │       └─► B5 ─► R1 ─► R2 ─► R3
    ├─► B4
    ├─► B6
    └─► B7
G4 (infra; any time)

D1 ─► D2 ─► D3 ─► F1 ─┬─► F2 (B1, B5, B7)
                      ├─► F3 (B4, B2)
                      ├─► F4 (B2, B3)
                      ├─► F5 (B5, R2, R3)
                      ├─► F6
                      ├─► F7 (B1, B6)
                      └─► F8 (B4, B5, R2)
everything ─► H1 ─► H2 ─► H3
```

Suggested waves, with at most about four agents at a time. The user runs D1 and D2 alongside waves 1–2.

| Wave | Tasks |
|---|---|
| 1 | G1, then G2 ∥ G3 |
| 2 | B1 ∥ B4 ∥ B6, then B7 (G4 whenever convenient) |
| 3 | B2 ∥ B5 ∥ D3, then F1 |
| 4 | B3 ∥ R1 ∥ F6 ∥ F7 |
| 5 | R2 ∥ F2 ∥ F3 |
| 6 | R3 ∥ F4 ∥ F8 |
| 7 | F5, then H1 → H2 → H3 |

F tasks start against the MSW mocks and switch to the real backend as each B or R task merges. Swapping mocks for the real backend is part of each F task's definition of done once its dependencies have merged.

## 9. Verification

- **Backend:** `cd momoshtrem && go vet ./... && go test ./...`.
  - New packages ship with table-driven tests, and handlers are tested with `httptest`.
  - TMDB, Trakt, Jackett and Anthropic sit behind interfaces with fakes; tests never touch the network.
- **Frontend:** `cd web && npm run lint && npx tsc --noEmit && npm test && npx playwright test`.
- **AI:** run `go run ./cmd/classifier-eval`, plus one documented manual recommendation run. Put the cost in the PR.
- **Every PR:**
  - A conventional-commit title.
  - What changed and how it was verified, with the command output.
  - For UI changes, screenshots at 390 px and 1440 px.

## 10. Risks, open questions and backlog

**Risks**
- **Classifier regressions on items already assigned.** Leave existing assignments alone; v2 applies only to new jobs. Add a "re-run mapping" action per torrent.
- **VFS path changes** (specials, multi-episode naming) could make Infuse rescan or lose playback state. Keep existing paths stable, and verify naming on a device before H3.
- **Trakt API details** come from secondary sources. Verify them in B5.
- **AI cost creep.** Mitigated by the budget cap, the usage page and the configurable schedule.
- **Until G4 ships HTTPS,** password autofill may stay unreliable even after G3.

**Open questions** (decide during D2 or Phase 1)
- The mascot's name: "Momo" is only a working name.
- Default auto-apply policy for high-confidence mappings (proposed: on).
- Default recommendation model: Opus 5.5 or Sonnet 5.5 (a cost-vs-quality call for the user).

**Backlog (not in this plan)**
- Auto-grab new episodes of followed shows using the quality profile; a natural next step after B4 and B5.
- Quality upgrades, e.g. replacing 720p with 1080p when it becomes available.
- An "Open in Infuse" deep link on iOS (verify Infuse's URL scheme).
- PWA push notifications for new episodes and "ready" events.
- Writing to Trakt (adding to the watchlist), and multi-user support.

## 11. Agent brief template

```
You are implementing task <ID> from docs/redesign/PLAN.md.

Read first: CLAUDE.md, docs/<web|momoshtrem>/CODE_QUALITY.md, and PLAN.md §2–§6 plus the <ID> section.
If the task calls the Claude API, load the claude-api skill before writing code.

Scope: only the paths <ID> owns. If you must touch others, keep it minimal and call it out in the PR.
Contract: implement or consume exactly the §6 shapes. If the contract is wrong, fix it in PLAN.md in the same PR and explain why.
Done when: every acceptance criterion for <ID> is met and the §9 verification passes.
Deliver: branch v2/<id>-<slug>, conventional commits, and a PR whose description includes the verification output
(and, for UI work, screenshots at 390 px and 1440 px).
```
