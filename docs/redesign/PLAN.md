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
                          │ HTTPS (Caddy, Let's Encrypt over TLS-ALPN)
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
| `JACKETT_URL`, `JACKETT_API_KEY` | **moved** from web to momoshtrem | Search runs in the backend (B4). The web's Jackett route and env were removed in #95. |
| `ANTHROPIC_API_KEY` | momoshtrem | Classifier and recommendations. The features are disabled when unset. |
| `TRAKT_CLIENT_ID`, `TRAKT_CLIENT_SECRET` | momoshtrem | Trakt API app, created at app.trakt.tv/settings/apps/api/new (redirect URI `urn:ietf:wg:oauth:2.0:oob`). The secret is optional: Trakt no longer issues one to new apps, and the device flow, refresh and revoke work without it. |
| `SECRET_KEY` | momoshtrem | AES-GCM key for encrypting Trakt tokens at rest |
| `CORS_ALLOWED_ORIGINS` | momoshtrem | Optional comma-separated browser origins. Empty (the default) sends no CORS headers, since web calls the backend server-to-server. |
| `WEBDAV_PUBLIC_URL` | momoshtrem | The WebDAV address Infuse uses (e.g. `https://momoshtrem.duckdns.org:36911/`), reported by `/status`. Defaults to `http://localhost:<webdav_port>/`. |
| `RECS_DEBUG` | momoshtrem | `true` serves the dev-only `GET /recommendations/debug/profile` (R1's profile and pool, built now, stored nowhere). Off by default; YAML `ai.recs_debug`. |
| `TZ` | momoshtrem | Server-local time for the AI features (#9): the 03:00 picks run, the hero's day, Ask's daily limit and the budget month. An IANA name; compose passes `${TZ:-UTC}` (Go falls back to UTC for a name it doesn't know; the image has `tzdata`). The startup log prints the zone. |

momoshtrem reads these settings and the other secrets and service URLs (`DATABASE_URL`, `TMDB_API_KEY`, OpenSubtitles, WebDAV auth, metrics) from env, with or without `config.yaml` (defaults → file → env). `config.yaml` covers the rest (ports, torrent, streaming…). `DATABASE_URL` is required.

## 5. Data model changes (Postgres)

Migration numbers are reserved per task so parallel branches don't collide.

**`002_app_settings.sql` (G2)**
- `app_settings(key text pk, value jsonb, updated_at)`. Holds the quality profile, AI settings and classifier defaults.
  - Key `ai`: one document shaped like `GET/PUT /settings/ai` (`{rec_enabled, rec_model, rec_schedule, monthly_budget_usd, classifier_enabled, classifier_model, auto_apply_high_confidence}`; §6 "Recommendations (R1–R3)"). A missing field takes its default; the assignment jobs (B2) read `classifier_enabled` (true), `classifier_model` (`claude-haiku-5-5`; `claude-sonnet-5-5` and `claude-opus-5-5` are priced too) and `auto_apply_high_confidence` (true), and every AI feature `monthly_budget_usd` (15 when the document doesn't have it; an explicit `null` is no cap; once every AI feature's spend this month reaches it, AI calls are skipped). `library.SettingsRepository.AISettings` reads the classifier's share; #9 serves the endpoints (#154), and `PUT` writes with one jsonb `||` statement.
  - Key `quality_profile`: one document shaped like `GET/PUT /settings/quality-profile` (§6 "Search"; #6). A missing field takes its default.

**`003_metadata.sql` (B1)**
- `movies`: add `imdb_id`, `poster_path`, `backdrop_path`, `runtime`, `genres jsonb`, `metadata_updated_at`.
- `shows`: add the following columns:
  - IDs and names: `imdb_id`, `tvdb_id`, `original_name`.
  - Origin: `original_language`, `origin_country text[]`, `is_anime bool`.
  - Artwork and status: `poster_path`, `backdrop_path`, `status`, `genres jsonb`.
  - Episode ordering: `episode_ordering` (CHECK `aired`|`absolute`|`episode_group`, default `aired`) and `episode_group_id`. Creating a show sets `absolute` when `is_anime` (the Animation genre plus a `ja` language or `JP` origin, or TMDB keyword 210024); everything else gets `aired`.
  - Matching options: `include_specials bool default true` and `classifier_mode` (CHECK `auto`|`rules_only`|`ai_always`).
  - `metadata_updated_at`.
- `seasons`: allow `season_number = 0`; add `name` and `poster_path`.
- `episodes`: add `tmdb_episode_id` (indexed), `absolute_number`, `episode_type`, `runtime`, `still_path`, `overview`. `air_date` stays TEXT (ISO dates compare correctly), NULL instead of `''`.
- `torrent_assignments`: a partial index on `(item_type, item_id) WHERE is_active`.
- `tmdb_cache(key text pk, payload jsonb, fetched_at, expires_at)`, indexed on `expires_at`. It also holds the hero colours (`color:w300{path}`, no expiry).

**`004_assignment_jobs.sql` (B2)**
- `torrent_assignments`: add `confidence` (`high|medium|low`; NULL for manual and legacy rows) and `match_source` (`rules|ai|manual`; NULL for legacy rows). The VFS groups a multi-episode file into one entry only for rows that have a `match_source` (§6 "Assignment").
- `assignment_jobs`:
  - The item: `media_type` (`movie|tv`) and `tmdb_id`, plus nullable `show_id` and `movie_id` FKs with `ON DELETE CASCADE`. Both are NULL for a dry run on a title that isn't in the library (#6 Inspect).
  - The torrent: `info_hash`, `magnet_uri`, `torrent_name` (the magnet's `dn`, else the hash, until the file list arrives), `indexer`, `release_group`, plus `size_bytes`, `file_count` and `video_count`, written when the file list arrives (`torrent.*` comes from them before proposals exist).
  - `file_path`: movies, the file the assign request picked up front.
  - `status` (CHECK `queued|fetching_metadata|classifying|resolving|needs_review|ready|applied|failed|cancelled`; flow in §6), `dry_run bool`.
  - `hint jsonb` (`{season, episode, text}`), `suggested_hints jsonb`, `ai_files int`, `error` (for the user), `error_code` (for code; §6 "Failure"), `ai_error`.
  - `created_at`, `updated_at`, `applied_at`.
  - Indexes: the worker's claim on working statuses; `show_id`, `movie_id`. A partial unique index on `(media_type, tmdb_id, info_hash)` for open, non-dry-run jobs, so one torrent has one open job per title.
- `assignment_proposals`, one row per file (subtitles too):
  - `id`, `job_id` (FK, cascade), `file_path`, `file_size`.
  - `kind` (`episode|movie|extra|subtitle`), `review_group` (`matched|needs_review|specials|extras|unmatched`), `extra_type` (`opening|ending|sample|trailer|featurette|font|info|image|superseded|other`, NULL unless an extra).
  - The targets: `episode_ids bigint[]` (in order; re-validated at apply).
  - What the classifier or AI last suggested, which Undo restores: `suggested_episode_ids bigint[]`, `suggested_confidence`, `suggested_source`, `suggested_reason`.
  - `confidence` (`high|medium|low`, NULL), `source` (`rules|ai|manual`), `reason` (≤ 120 characters).
  - `decision` (`pending|accepted|ignored`), `user_decided bool`, `updated bool`, `hinted bool`.
  - `superseded_by` (FK to the proposal that replaces it, its v2) and `linked_proposal_id` (a subtitle's video), both `ON DELETE SET NULL` and indexed (deleting a big job's proposals otherwise scans the table once per row).
- `ai_usage(id, feature classifier|recs|ask, model, input_tokens, output_tokens, cache_read_tokens, web_searches, cost_usd numeric, ref_id, created_at)`. Shared by every AI feature, one row per API call. `input_tokens` counts the prompt tokens not read from the cache (cache writes included); `cost_usd` prices each kind at its rate. The classifier's `ref_id` is the job.
- `ai_cache(key text pk, feature, payload jsonb, created_at, expires_at)`, indexed on `expires_at`. Shared by AI features; the classifier keys it by a hash of `(info_hash, show_id, metadata_updated_at, hint, scope)` plus the model and its prompt version, for 30 days.

**`005_trakt.sql` (B5)**
Every table is Trakt's: a disconnect purges them all in one transaction, and the settings (`app_settings` key `trakt`) stay. Movies and shows without a TMDB id are skipped on import, so every row maps to TMDB.
- `trakt_auth`, the one connection (`id smallint` CHECK `id = 1`):
  - `username`, `user_slug`;
  - `access_token_enc`, `refresh_token_enc` (`bytea`, AES-GCM with `SECRET_KEY`), `token_created_at` (Trakt's `created_at`), `expires_at` (`created_at + expires_in`);
  - `needs_reauth`, `reauth_reason`;
  - `connected_at`, the generation guard: every sync write checks it, so nothing lands after a disconnect or reconnect replaced it; `updated_at`.
- `trakt_sync_state(key pk, value jsonb, updated_at)`: the `last_activities` snapshot, `last_sync_at`, `last_reconcile_at` and the calendar's fetch time.
- `trakt_history`, every play: `id bigint` pk (Trakt's history event id, so an incremental sync's overlap upserts onto the same rows), `media_type` (`movie|episode`), `tmdb_id` and `trakt_id` (the movie's or the show's), `title`, `year`, `season`, `episode`, `episode_tmdb_id`, `episode_title`, `watched_at`. A CHECK: episode rows have a season and an episode, movie rows neither. Indexed on `(watched_at DESC, id DESC)` (`/me/history`) and `(media_type, tmdb_id)`.
- Derived from `trakt_history` in the transaction that writes it: `trakt_watched_movies(tmdb_id pk, plays, last_watched_at)` and `trakt_watched_episodes(show_tmdb_id, season, episode, episode_tmdb_id, plays, last_watched_at)`, pk `(show_tmdb_id, season, episode)`, with a partial index on `episode_tmdb_id`.
- `trakt_ratings(media_type movie|show|season|episode, trakt_id, tmdb_id, show_tmdb_id, season, episode, rating 1–10, rated_at)`, pk `(media_type, trakt_id)`, indexed on `(media_type, tmdb_id)`. `tmdb_id` is the rated item's own (NULL when a season or an episode has none).
- `trakt_watchlist(media_type movie|tv, trakt_id, tmdb_id, title, year, poster_path, backdrop_path, rank, listed_at)`, pk `(media_type, trakt_id)`, indexed on `(media_type, tmdb_id)` and `(listed_at DESC)`.
- `trakt_show_progress`, one row per show from the bulk `/sync/progress/watched`: `show_trakt_id` pk, `show_tmdb_id` (indexed), `title`, `year`, `poster_path`, `backdrop_path`, `aired` and `completed` (regular episodes), `last_watched_at`, `reset_at`, `last_season`, `last_episode`, Trakt's next episode (`next_season`, `next_episode`, `next_episode_tmdb_id`, `next_title`, `next_still_path`; NULL when caught up), `updated_at`.
- `trakt_hidden(section dropped|progress_watched, trakt_id, tmdb_id)`, pk `(section, trakt_id)`, indexed on `tmdb_id`.
- `trakt_calendar(show_trakt_id, season, episode, show_tmdb_id, episode_tmdb_id, show_title, year, episode_title, first_aired, still_path, poster_path, backdrop_path)`, pk `(show_trakt_id, season, episode)`, indexed on `first_aired` and `show_tmdb_id`: `/calendars/my/shows` from 7 days ago, 33 days long.
- `app_settings` key `trakt`: `{use_ratings, show_watchlist, include_paused}`, defaults `true`, `true`, `false`, read over the defaults (`library.SettingsRepository.TraktSettings`). `UpdateTraktSettings` is a partial update in one jsonb statement, so two toggles never lose each other's change.
- Rollback: nothing else reads these tables, so an older binary ignores them.

**`006_recommendations.sql` (R1, #153)**
New tables only, so an older binary ignores them. Wire shapes: §6 "Recommendations (R1–R3)".
- `taste_profiles(id, created_at, counts jsonb, profile jsonb)`: R1's profile, one row per build.
  - `counts` (`{loved, completed, dropped, rated, plays}`) drives `no_history` (fewer than 5 loved or completed titles).
  - `profile` stays within about 16 KB (≤ 4k tokens). Rows no run references are pruned once they're a day old (each new build prunes; the day lets a run reference the one it just built).
- `rec_runs`:
  - `id`, `kind` (CHECK `scheduled|manual|first_sync`), `status` (CHECK `queued|running|succeeded|failed|skipped`), `model`, `profile_id` (FK `taste_profiles`, `ON DELETE SET NULL`);
  - `progress jsonb`: `{steps: [{step, state, detail}], shelves_done, shelves_target}`, written as the run moves;
  - `skip_reason` (CHECK `budget_exceeded|no_history`), `error` (for the user);
  - usage, summed from its `ai_usage` rows as they land: `api_calls`, `input_tokens`, `output_tokens`, `cache_read_tokens`, `web_searches`, `cost_usd numeric`;
  - `created_at`, `started_at`, `finished_at`;
  - a partial unique index on `((true)) WHERE status IN ('queued', 'running')`, so at most one run is active, an index on `(status, finished_at DESC)` (the newest succeeded run), and one on `profile_id` (the prune).
  - At startup a run a crash left `queued` or `running` is `failed` ("momoshtrem restarted").
- `rec_shelves(id, run_id FK cascade, position, slug, kind, title, subtitle)`: `kind` CHECK `because|new|anime|gems|coming_soon|other`; UNIQUE `(run_id, position)` and UNIQUE `(run_id, slug)`.
- `rec_items(id, shelf_id FK cascade, position, media_type, tmdb_id, reason, meta jsonb, signals jsonb)`:
  - `meta` is a snapshot taken at write time: `title, year, year_end, poster_path, seasons, runtime_minutes, vote_average, release_date, badge`, so reads never call TMDB;
  - `signals`: why the agent picked it (profile keys, candidate source), for debugging; never served;
  - UNIQUE `(shelf_id, media_type, tmdb_id)`, and an index on `(media_type, tmdb_id)` (the title page's reason).
- `rec_hero_picks(id, run_id FK cascade, position, media_type, tmdb_id, kicker, reason, meta jsonb)`, UNIQUE `(run_id, position)` and UNIQUE `(run_id, media_type, tmdb_id)`. A run writes 3–9.
- `rec_feedback(media_type, tmdb_id, signal, source, created_at, updated_at, PK (media_type, tmdb_id))`: `signal` CHECK `like|dislike|not_interested|seen`, `source` CHECK `shelf|hero|ask|title`. It isn't Trakt data: a Trakt disconnect keeps it.
- `ask_threads(id, title, model, prompt_version, created_at, updated_at)`. `model` is the first turn's and stays for the thread's life (thinking blocks are bound to the model); `prompt_version` names the system prompt it started with.
- `ask_turns(id, thread_id FK cascade, position, message, status, view jsonb, error_code, cost_usd, created_at, finished_at)`:
  - `status` CHECK `streaming|done|error|cancelled`; UNIQUE `(thread_id, position)`;
  - an index on `created_at` (the daily limit), and a partial unique index on `((true)) WHERE status = 'streaming'` (one turn at a time: `ask_busy`);
  - `view` is what the thread page shows, written when the turn ends: `{steps, intro, items: [{media_type, tmdb_id, match_tag, reason, meta}], refinements, error, elapsed_ms}`;
  - at startup a turn a crash left `streaming` is `error` (`internal`).
- `ask_messages(id, thread_id FK cascade, turn_id FK cascade, position, role user|assistant, content json, created_at)`, UNIQUE `(thread_id, position)`, indexed on `turn_id`:
  - `content` holds the raw API content blocks, append-only: tool loops, server tools and thinking replay need them unchanged. It's `json`, not `jsonb`: jsonb reorders keys and drops whitespace, and a replay must be byte-identical, since preserved thinking and prompt caching compare the request prefix;
  - a turn's messages are written in one transaction, and only when it ends `done`. A cancelled or failed turn never replays, so no orphaned `tool_use` is ever sent.
- `ai_usage` (004): `ref_id` is the run id for `recs` and the turn id for `ask`.
- `app_settings` key `ai` gains `rec_enabled`, `rec_model` and `rec_schedule` (002 above), replacing the never-used `rec_schedule_cron`.
- Rollback: nothing else reads these tables, so an older binary ignores them; it reads `ai` with its own fields.

**`007_subtitles.sql` (B7)**
- `subtitles`: add `release_name`. Drop `UNIQUE(item_type, item_id, language_code)` (looked up through `pg_get_constraintdef`).
- Partial unique indexes instead, so a torrent subtitle never overwrites a downloaded one and unknown-language files coexist:
  - `(item_type, item_id, language_code) WHERE source = 'opensubtitles'`;
  - `(item_type, item_id, info_hash, file_path) WHERE source = 'torrent'`.
- CHECK that torrent subtitles have an `info_hash`.

**`008_stream_sources.sql` (#63)**
- `torrent_assignments`: add `kind` (`torrent|hdrezka`, default `torrent`) and `stream_ref jsonb`; `info_hash` and `magnet_uri` become nullable. One active source per item: `writeAssignments` stays the only writer and deactivates an item's other rows whatever their kind.
- `stream_ref` is an HDRezka row's stream (`library.StreamRef`): `{provider, path, post_id, title, translator_id, translator_name, camrip, ads, director, season, episode, quality}`. `path` is the title page without the host (mirrors change); `season` and `episode` are HDRezka's, absent for a movie.
- A CHECK per kind: a torrent row has a hash and a magnet and no `stream_ref`; an HDRezka row has a `stream_ref`, no hash or magnet, and NULL `match_source` and `confidence` (the VFS never groups it).
- An HDRezka row's `file_path` is made up from its stream, `hdrezka/<post_id>/t<translator_id>[-c][-a][-d]/<movie|s01e05>/<quality>.mp4`, never parsed back. It gives the VFS the `.mp4` extension and keys the row: a partial unique index `(item_type, item_id, file_path) WHERE kind = 'hdrezka'` is the upsert's arbiter, so the same stream again reactivates its row and another translator or quality is a new one. `resolution` is the quality's; `source` is NULL.
- Rollback: an older binary scans the hash and magnet into strings, so one active HDRezka row fails its VFS queries and folders vanish from WebDAV. Roll back from the pg_dump (ops PLAN §6), or as an escape hatch `UPDATE torrent_assignments SET is_active = false WHERE kind = 'hdrezka'`.

**`009_hdrezka_subtitles.sql` (#68)**
- `subtitles`: a new source, `hdrezka`, for the subtitles that came with an item's HDRezka stream. They're stored as SRT files in `{subtitles.download_path}/hdrezka/{item_type}/{item_id}/{lang}.srt`.
- A partial unique index `(item_type, item_id, language_code) WHERE source = 'hdrezka'` allows one per language. It's the upsert's arbiter, so assigning the stream again refreshes them in place.
- Like torrent subtitles, the writer drops them when the item's file changes.
- Rollback: an older binary lists them as downloaded subtitles whatever the item's file, which is harmless.

## 6. API contract (backend, under `/api`)

**Conventions**
- JSON throughout. Errors are `{error: {code, message}}` (implemented in G2 for every route: `internal/api/errors.go` derives `code` from the status: `bad_request`, `unauthorized`, `forbidden`, `not_found`, `conflict`, `validation_error`, `rate_limited`, `unavailable`, `timeout`, `internal_error`; `errorCodeResponse` sets a specific one). Paginated lists (`/library`, `/me/history`, `/search/torrents`) are `{items, total, page, page_size}`; short lists (up next, watchlist, threads, episode groups, torrent files, `/torrents`) are `{items}`.
- Long operations return `202 {job_id | run_id}` and are polled.
- Every route requires `X-Momoshtrem-Token` when `BACKEND_API_TOKEN` is set; otherwise `401 {error: {code: "unauthorized"}}` (G2). The web client maps a backend 401/403 to `502 backend_auth`, so a token mismatch never looks like an expired session.
- CORS is an allowlist (`CORS_ALLOWED_ORIGINS`), empty by default, so no CORS headers are sent. Allowed origins may use `GET, POST, PUT, PATCH, DELETE, OPTIONS` and the `Content-Type`, `Authorization`, `X-Momoshtrem-Token` headers.
- The web app exposes the same paths under its own session-checked `/api/*` routes. A catch-all `web/src/app/api/[...path]` forwards any path whose first segment is in `BACKEND_PREFIXES` (`web/src/lib/api/backend-proxy.ts`), adding the token and streaming the response (SSE included); explicit web routes take precedence. Typed browser calls and TanStack Query factories for every endpoint live in `web/src/lib/api/endpoints/`, and MSW fixtures in `web/src/mocks/` (F1).

### Titles and library (B1)
- `GET /titles/{movie|tv}/{tmdb_id}`: cached TMDB details, plus library state, watch state (B5; "Watch state rules" below) and the recommendation reason if there is one. Types: `web/src/lib/api/contract/titles.ts`.
  ```
  Title = MovieTitle | ShowTitle
  TitleBase {
    media_type, tmdb_id, title, original_title, year, overview, poster_path, backdrop_path,
    backdrop_dominant_color,                 // hex, or null while it's computed
    genres: [], vote_average, vote_count, original_language,
    cast: [{name, character, profile_path}], trailer_youtube_key,
    similar: [SimilarTitle],                 // ≤ 12: TMDB recommendations, else similar
    library: TitleLibraryState, watch_state, recommendation_reason
  }
  MovieTitle += runtime, directors: []
  ShowTitle  += status, is_anime, episode_ordering, episode_group_id, include_specials, classifier_mode,
                creators: [], networks: [], end_year, episode_run_time,
                seasons: [SeasonSummary],    // Specials (season 0) last; left out while include_specials is off
                default_season               // the season to open; null when there are none
  WatchState { watched_at, plays, rating,    // from Trakt (B5); null when Trakt knows nothing about the title
               progress: {completed, aired},                                       // shows
               next_episode: {season, episode, absolute_number, name, air_date} | null }   // shows
  SeasonSummary { season_number, name, episodes_total, ready, missing, needs_review, upcoming,
                  absolute_from, absolute_to }   // episodes_total includes upcoming; ready: episodes with a file;
                                                 // missing, needs_review and upcoming leave watched ones out;
                                                 // not in library: only episodes_total, the rest 0/null
  TitleLibraryState {
    in_library, library_id, added_at,        // library_id and added_at are null when not in the library
    availability_stage: none|needs_torrent|downloading|ready,
    webdav_path, webdav_url,                 // null until something is assigned (the VFS hides empty titles)
    download: {pct, speed_bps, eta_seconds, seeders} | null,   // shows: combined over the downloading episodes
    needs_review, review_job_id,             // mapping review (B2/#5); rules under "Availability rules"
    episodes_total, episodes_ready,          // shows: aired regular episodes (plus any with a file), and those with a file;
                                             // null for movies and while not in the library
    missing: {season_number, from, to, absolute_from, absolute_to, count} | null,   // shows: the first frontier run (below)
    torrent: {info_hash, release_name, file_name, size_bytes, resolution, source, added_at,
              state: active|idle|not_loaded|paused, seeders} | null,  // movies; seeders null unless loaded;
                                             // paused (B6): the user paused it, loaded or not; it wins
    stream: {…} | null                       // movies playing from HDRezka; see "HDRezka (#63)"
  }
  SimilarTitle { media_type, tmdb_id, title, year, poster_path,
                 library: {library_id, availability_stage, episodes_total, episodes_ready} | null }
  ```
  `download.eta_seconds` is null (torrents fetch on demand). `download.speed_bps` is the torrent's smoothed download speed (B6, #98); for a show, the speeds of the torrents its downloading episodes are in, each torrent once. A paused torrent is never downloading.
- `GET /titles/tv/{tmdb_id}/seasons/{n}`: one season at a time, so 1,000-episode shows stay light. `404 not_in_library` for a show that isn't in the library; `404 not_found` for an unknown season (or season 0 while specials are off).
  ```
  { season_number, name, episodes: [EpisodeSummary], ranges: [EpisodeRange] }
  EpisodeSummary { id, season_number, episode_number, absolute_number, name, overview, air_date, still_path,
                   runtime, episode_type: standard|mid_season|finale|null,
                   state: watched|ready|downloading|missing|upcoming|needs_review, pct,   // watched wins (B5)
                   webdav_path,              // only while the episode has a file
                   source, stream }          // the file's source; see "HDRezka (#63)"
  EpisodeRange { label, kind: arc|block, from, to, absolute_from, absolute_to, missing }   // from/to in-season
  ```
  Ranges exist only for seasons over 20 episodes. They never overlap: every episode is in exactly one range, so it counts towards one `missing`. A regular season uses the story arcs of its TMDB story-arc group (type 5) when they share no episode and cover at least 80% of the season. Each arc becomes one range per run of consecutive episodes it lists, never its min–max span: an arc interrupted by other episodes gets `"Specials"`, then `"Specials (2)"`, and so on. Otherwise the season is split where air dates are more than 45 days apart, then cut into blocks of up to 10. Episodes no arc covers (arc groups lag behind airing shows) get blocks too. Specials (season 0) always get blocks and skip the air-date split, since they air years apart. Block labels are in-season numbers, e.g. `"21–28"`, or `"31"` for a single episode. Without TMDB the season still loads, with blocks.
- **Availability rules.** The server computes `availability_stage`, so the client never infers it.
  - Movie: `none` not in the library; `needs_torrent` no active assignment; `downloading` the torrent is loaded and active and the file isn't fully cached; `ready` otherwise.
  - Show: `none` not in the library; `needs_torrent` nothing assigned, or the frontier is non-empty; `downloading` any assigned episode is downloading; `ready` otherwise.
  - The **frontier**: aired regular episodes after the last one with a file **or watched** (B5). Specials never count, nor do gaps before that episode. `missing` is its first run, within one season.
  - Episode, first match: `watched` when Trakt has a play of it (B5); `ready` or `downloading` when assigned; `needs_review` as below; `missing` when aired and unassigned; `upcoming` when the air date is in the future or unknown.
  - An HDRezka file (#63) counts as assigned like a torrent's, and is never `downloading`: it streams from the CDN, so it's always `ready`. The stage keeps the name `needs_torrent`.
  - `default_season`: `watch_state.next_episode`'s season when the title lists it, else the frontier's season, else the latest season with a file, else the first regular season.
- **Review rules** (B2, #5). They read the item's open, non-dry-run mapping jobs that are past classification (`resolving`, `needs_review` or `ready`; §6 "Assignment").
  - `needs_review` (title and `LibraryItem`) counts those jobs' files that are pending and map (have targets, or are movie candidates) or are in a conflict. `/library?status=needs_review` is `needs_review > 0`.
  - `review_job_id` is the newest of those jobs, or null. It stays set while a `ready` job waits for Apply, even at `needs_review = 0`.
  - An episode is `needs_review` when one of those files targets it and it has no file, aired or not: a file (`ready`, `downloading`) or a play (`watched`) wins. `SeasonSummary.needs_review` counts those episodes, and they don't count as `missing` or `upcoming` there, or in `EpisodeRange.missing`.
  - The frontier is about files: an aired episode under review is still part of it.
  - Cost: one more query for a title page or a season; `/library` reads it in its one query.
- **Watch state rules** (B5, #133). They read what the Trakt sync imported (§5 `005_trakt.sql`); without Trakt every `watch_state` is null and nothing is `watched`.
  - **Matching** a Trakt episode to a library episode: `episodes.tmdb_episode_id` against Trakt's `episode_tmdb_id` first; season and number only when either id is missing, so an episode TMDB numbers differently never borrows another's watched state. One rule, everywhere.
  - **`watch_state`** is null when Trakt knows nothing about the title (no play, rating or progress). A movie: `watched_at` (its last play), `plays`, `rating`; no `progress` or `next_episode`. A show: `watched_at` (its last watched episode), `plays` (the sum of its episodes' plays), `rating` (the show's), `progress` (Trakt's, regular episodes; left out until the import's `progress` step brings it) and `next_episode` (always there, maybe null).
  - **`next_episode`** is the episode Up next names for the show (§6 "Trakt (B5)"), whether or not Up next lists the show (dropped, hidden and idle shows keep theirs); null when there's none. Watchlist starts don't count: a show nobody has started has no watch state. It's in the library's numbering when the library lists it, so it drives `default_season` and the page's next-up highlight, with the frontier (`missing`) as the fallback.
  - An episode with a play is `watched`, whatever else is true; `pct`, `webdav_path`, `source` and `stream` still describe its file. `SeasonSummary.ready` counts episodes with a file (watched ones too); `missing`, `needs_review`, `upcoming` and `EpisodeRange.missing` leave watched ones out.
  - Cost: one more query for a title page; seasons and `/library` don't grow.
- `POST /movies` and `POST /shows` with `{tmdb_id}` → `201 {id, tmdb_id, title, year}` (`200` if already there; `400 bad_request` for an unknown TMDB id, as today).
- `DELETE /movies/{id}?delete_data=true|false` and `DELETE /shows/{id}?delete_data=` → `204` (B6, #96). `delete_data` defaults to `false`; any other value is `400 bad_request`. An unknown title is `404`.
  - One transaction deletes the title (its seasons, episodes and jobs cascade) and deactivates its assignments, in one query for a whole show. The rows stay, inactive.
  - Every subtitle of the title goes, from every source: the rows in one query, the files, and each item's folders (`{download_path}/{movie|episode}/{id}/` and `{download_path}/hdrezka/{movie|episode}/{id}/`).
  - Each torrent the title used (its assignments, active or not, and its jobs) that `TorrentInUse` says nothing else uses is released: dropped from the client, with its data when `delete_data=true` (see "Torrents and activity (B6)").
  - The WebDAV tree is rebuilt.
- `POST /movies/{id}/refresh` (new) and `POST /shows/{id}/refresh` re-fetch TMDB metadata. The body is informational; refetch the title.
- `DELETE /movies/{id}/assign` and `DELETE /episodes/{id}/assign` → `204`. They drop the file and keep the item; an unknown item is `404`.
  - The item's active assignment is deactivated.
  - The subtitles that came with the file go: torrent rows, and HDRezka rows with their SRT files. Downloaded subtitles stay.
  - The torrent is released when nothing else uses it, but its data is never deleted.
  - The WebDAV tree is rebuilt, which renames a grouped entry's other episodes.
- `GET /library?type=&status=ready|incomplete|needs_review|watching&sort=added|title|last_watched|size&q=&ids=&page=&page_size=` (50 by default, max 200) → `LibraryPage`. Replaces the per-item TMDB calls. Types: `web/src/lib/api/contract/library.ts`.
  ```
  LibraryPage = Paginated<LibraryItem> + {
    counts: {all, movie, tv, watching, ready, incomplete, needs_review},   // honour q and ids, never type or status
    summary: {total, size_bytes, ready}                                    // the whole library, whatever the params
  }
  LibraryItem { id, media_type, tmdb_id, title, year, poster_path, availability_stage,
                episodes_total, episodes_ready, episodes_watched,   // shows; null for movies
                needs_review, size_bytes, added_at, last_watched_at,
                runtime, resolution,                 // movies (minutes; the assigned file's); null for shows
                webdav_path, webdav_url,             // as TitleLibraryState: null until something is assigned
                review_job_id,                       // as TitleLibraryState (review rules)
                download: {pct, info_hash} | null,   // while a file is being fetched; shows: combined, one of the hashes
                missing: MissingRun | null,          // shows: the first frontier run, as TitleLibraryState.missing
                streaming }                          // a streaming session plays it (a show: any episode); an in-memory overlay (#99)
  ```
  `id` is the library id. `size_bytes` sums the assigned files. `needs_review` follows the review rules above. `episodes_watched` counts a show's regular episodes with a play, and `last_watched_at` is the movie's last play or the show's last watched episode (the watch state rules; 0 and null without Trakt). `status=ready` is `availability_stage` ready; `incomplete` is everything else (needs a torrent, or downloading); `needs_review` is `needs_review > 0`; `watching` is shows with `0 < episodes_watched < episodes_total` that aren't dropped or hidden on Trakt (`trakt_hidden`), and `counts.watching` counts them. `sort=last_watched` is `last_watched_at DESC NULLS LAST, created_at DESC, media_type, id`. Unknown `type`, `status` or `sort` values are `400 bad_request`.
  `ids=movie:603,tv:1399` looks titles up by TMDB id (Discover, Search and the palette mark library titles with it): comma-separated `media_type:tmdb_id`, at most 200, ANDed with the other filters; the answer is page 1 of all of them (`page_size` = the number of distinct ids; `page` and `page_size` are ignored). Malformed, or more than 200, is `400 bad_request`.
  Cost: one query, whatever the params, including an empty page (it still carries `counts` and `summary`).
- `PATCH /shows/{id}` with `{episode_ordering, episode_group_id, include_specials, classifier_mode}` → the updated `ShowTitle`. `422 validation_error` for `episode_group` ordering without a valid group id.
- `GET /shows/{id}/episode-groups`: `{items: [{id, name, type, group_count, episode_count}]}`, TMDB episode groups for the ordering picker.
- Errors: `502 tmdb_unavailable` when TMDB is down and nothing is cached (library titles fall back to stored metadata: no cast, crew, overview or similar titles); `503 unavailable` without a TMDB key, library titles included.

### Subtitles (B7)
Item-scoped under `/movies/{id}` and `/episodes/{id}` (the legacy `/subtitles/*` routes are gone). Types: `web/src/lib/api/contract/subtitles.ts`.
- `GET …/subtitles` → `{items: [Subtitle]}`: the downloaded subtitles, plus those that came with the item's current file: the assigned torrent's, or the HDRezka stream's (#68). None from an earlier file, and none with nothing assigned. Works without an OpenSubtitles key.
- `GET …/subtitles/search?languages=en,az` → `{items: [SubtitleCandidate], release_name}`, ranked against the assigned release. `release_name` is null when nothing is assigned; every `match` is then `unknown`. `languages` defaults to `en`.
- `POST …/subtitles` with `{file_id, language_code, release_name?}` → `201 Subtitle`. It replaces the downloaded subtitle in that language. It works with nothing assigned (`match` is then `unknown`).
- `DELETE …/subtitles/{sid}` → `204` for a downloaded or HDRezka subtitle (both are local files: the row and the file go; assigning the stream again brings HDRezka's back), or `409 conflict` for a subtitle that came with the torrent.
- Errors: `404` for an unknown item (or a subtitle that isn't the item's), `400 bad_request` for a malformed language code, `503 unavailable` for search and download without an OpenSubtitles key, `502` when OpenSubtitles fails.
- The assigned release is the magnet's `dn`, else the torrent's top folder, else the file name. `machine_translated` covers AI translation too.
```
Subtitle { id, language_code, language_name, format, source: opensubtitles|torrent|hdrezka, release_name, match, created_at }
SubtitleCandidate { file_id, language_code, language_name, release_name, file_name, downloads,
                    hearing_impaired, machine_translated, match,
                    match_score, match_reason }   // 0–100; one line, e.g. "Same group (FLUX) and source (WEB-DL)"
match: exact_release (score ≥ 90) | same_source (≥ 60) | different_timing (lower) | unknown (a release name is missing)
```
A stored subtitle's `match` compares its `release_name` with the current release; torrent and HDRezka subtitles are `exact_release`.

### Assignment (B2/B3)
Types: `web/src/lib/api/contract/assignment.ts`. Assigning a torrent creates a **job**. The worker fetches the file list and classifies every file: rules first, then the AI resolver for the files it's unsure about. The job stops for review, or applies on its own when nothing needs a look.

**State machine.** `queued → fetching_metadata → classifying → [resolving] → needs_review ⇄ ready → applied`. A working job can end in `failed`; Discard moves any open job to `cancelled`.
- *Working* means `queued…resolving`, *open* means working plus `needs_review` and `ready`, and *closed* means `applied`, `failed` or `cancelled`.
- `fetching_metadata` loads the torrent's file list. Meanwhile it refreshes the show's metadata when it was never synced or is over 24 h old, and reads TMDB's alternative titles (the classifier's aliases) and season-like episode groups (its alternative orderings). When a video then stays unmatched (perhaps an episode that just aired), it refreshes once more and reclassifies, unless a refresh was tried in this job or ran within the last hour.
- `resolving` runs the AI resolver on the unsure main videos (every main video with `classifier_mode` `ai_always`; never extras, samples or subtitles). It's skipped when nothing needs asking or AI is unavailable: no key, `classifier_enabled` off, the show's `classifier_mode` is `rules_only`, or it's a movie job. The AI never overrides a `high` rules answer: when it answers one differently (another episode that exists, or an extra, at medium or high confidence), the proposal keeps its targets (and suggestion, so Undo restores them) but drops to `medium`, so it lands in `needs_review` (pending) and auto-apply waits; its reason names the AI's pick ("AI suggests S00E03: …"). A run gets 90 s per wave of requests (up to 60 files a request, four at a time), at most 15 minutes, so a big pack can finish; when some requests fail, the answers of the others are kept and `ai_error` says the rest are unchanged (the job isn't auto-applied).
- After classification, every PATCH (during `resolving`, when the run ends) and every AI run, the status is recomputed: `needs_review` while a proposal that *maps* is `pending` or a conflict exists, otherwise `ready`. A proposal maps when it has targets, or when it's a movie candidate.
- **Auto-apply** (the `auto_apply_high_confidence` setting, on by default) sends a freshly classified job straight to `applied` when all of these hold:
  - it isn't a dry run;
  - it has no conflict;
  - something maps and isn't ignored (a torrent of openings and endings alone waits for the user);
  - nothing that maps is pending, and everything that maps is `high`;
  - no video is unmatched;
  - no proposal has `replaces` (it would replace a file the item already has, a torrent or an HDRezka stream).
- **Dry runs** (Inspect, §6 "Search") are read-only: PATCH and reclassify answer `409 dry_run`, as Apply does; Discard works. For a title not in the library, `item.id` is null and `ProposalTarget.episode_id` is a TMDB episode id.
- **Restart.** Jobs before `resolving` start again from `queued`. An interrupted `resolving` gets `ai_error` and its status recomputed (`needs_review`, or `ready` when nothing is pending), keeping the rules' proposals; it isn't auto-applied. Apply and re-check never need the torrent loaded: the file list is stored as proposals.
- **Failure.** A failed job has `error` for the user; the database also keeps why for code (`error_code`: `metadata_timeout`, `no_videos`, `no_torrents`, `title_gone`, `invalid_magnet`, `torrent_error`, `internal`; `tmdb_unavailable` for a dry run on a title outside the library whose episodes TMDB couldn't give).

**Endpoints**

| Endpoint | Answer | Errors |
|---|---|---|
| `POST /shows/{id}/assign-torrent` with `{magnet_uri, hint?: {season?, episode?, text?}, indexer?}`, or `{info_hash, hint?}` instead of `magnet_uri` (Re-run mapping, #99) | `202 {job_id}`, or `200 {job_id}` of the open job for the same show and info hash (the new hint is ignored). With `info_hash` (hex in either case, or base32) the magnet is the library's for that torrent (`TorrentMagnet`: an assignment's, active first, else a job's), then it's an ordinary assign; `magnet_uri` wins when both are sent | `400 bad_request` (neither `magnet_uri` nor `info_hash`, a malformed `info_hash`, no BitTorrent info hash, or an invalid hint: a negative season, an episode below 1, text over 500 characters), `404` (the title, or an `info_hash` the library knows no magnet for) |
| `POST /movies/{id}/assign-torrent` with `{magnet_uri, file_path?, indexer?}`, or `{info_hash}` instead of `magnet_uri` | As for shows. `file_path` picks the file up front: it's accepted, the other candidates are ignored, and the job applies once classified | As for shows |
| `GET /assignment-jobs/{id}` | `200 AssignmentJobDetail` | `404`, `502 tmdb_unavailable` (a dry run on a title outside the library, when TMDB fails and nothing is cached) |
| `GET /assignment-jobs?status=` | `{items: [AssignmentJob]}`, newest first, no dry runs. `status` is `open` (the default) or a comma-separated list of statuses | `400 bad_request` (unknown status) |
| `PATCH /assignment-jobs/{id}/proposals` with `{items: [{id, decision?, episode_ids?}]}` | `200 AssignmentJobDetail`, all or nothing | `400 bad_request` (malformed body: an item with neither field, an empty `episode_ids`), `404`, `409` with `dry_run`, `job_closed` or `job_working` (checked in that order), `422 validation_error` (well-formed but invalid) |
| `POST /assignment-jobs/{id}/reclassify` with `{scope: "unresolved"\|"all", hint?}` | `202 {job_id}`; the job goes to `resolving` | `400 bad_request` (malformed body), `409` with `dry_run`, `job_closed` or `job_working` (checked in that order), `503 unavailable` (no AI) |
| `POST /assignment-jobs/{id}/apply` | `200 AssignmentJobDetail` (`applied`) | `409` with `dry_run`, `job_closed`, `job_working`, `conflicting_proposals` or `pending_proposals`, checked in that order |
| `POST /assignment-jobs/{id}/discard` | `200 AssignmentJobDetail` (`cancelled`; again for a cancelled job) | `409 job_closed` once applied or failed |
| `GET /shows/{id}/episodes` | `{items: [{season_number, name, episodes: [{id, episode_number, absolute_number, name, air_date, has_file}]}]}` | `404` |
| `PUT /episodes/{id}/assignment`, `PUT /movies/{id}/assignment` with `{info_hash, file_path}` | `204`; a manual override (`match_source: manual`) | `400 bad_request` (no `info_hash` or `file_path`), `404` (unknown episode or movie, unknown torrent, or no such file in it), `422 validation_error` (not a video file), `503 unavailable` (the file list must be fetched and there's no torrent client), `504 timeout` (no peers sent the file list in time) |
| `GET /torrents/{hash}/files` | `{items: [{path, size, mapped_to}]}` in the torrent's order; `mapped_to` is `{episodes: [ProposalTarget]}`, `{movie_id}` or null | `404` (unknown torrent), `503 unavailable`, `504 timeout` (as for the override) |

`?wait=true` (legacy, both assign routes, until H2) waits until the job stops working, applies it, and answers `201` with the legacy response plus `job_id`. The job ends `applied`.
- Shows: every proposal with targets is applied, at any confidence, except conflicting ones. Those are left out (the legacy response reports them as unmatched) and their proposals stay `pending`.
- Movies: only the top-ranked candidate is applied.
- The wait and the apply outlive the request (bounded at 10 min), so a client that gives up early still gets the job applied.
- When the job fails: `400 bad_request` for a torrent without a video (or a film), `503 unavailable` without a torrent client, `504 timeout` when no peers sent the file list in time, `404` when the title left the library, `500` otherwise; `409 job_closed` when it was discarded.

```
AssignmentJobDetail { job: AssignmentJob, proposals: [Proposal] }   // proposals: [] until classified (queued…classifying)
AssignmentJob {
  id, status, dry_run, error,                              // error: why it failed
  item: {id, media_type, tmdb_id, title, year, poster_path},   // id: library id; null for a dry run on a title not in the library
  torrent: {info_hash, name, size_bytes, file_count, video_count, release_group, indexer},   // sizes and counts null until the file list is in
  progress: {seasons, episodes, files, videos, ai_files},  // the progress steps' details; each null until its step has run
  // video_count and videos: video files that aren't extras (Frieren's batch: 16 files · 13 videos)
  hint: {season, episode, text} | null,                    // a re-check's hint replaces text
  ai_available, ai_error,                                  // ai_error: why the latest AI run failed, whole or in part (what it couldn't answer is unchanged); cleared when a re-check starts
  suggested_hints: [string],                               // ≤ 5 chips for "Re-check with AI"
  counts: {files, to_apply, pending, conflicts, needs_review, matched, specials, ignored, unmatched},
  created_at, updated_at, applied_at                       // updated_at never goes backwards (clock time, at least the last)
}
Proposal {
  id, file_path, file_size,
  kind: episode|movie|extra|subtitle,
  group: matched|needs_review|specials|extras|unmatched,
  extra_type: opening|ending|sample|trailer|featurette|font|info|image|superseded|other | null,   // extras only
  targets: [ProposalTarget], suggested_targets: [ProposalTarget],
  confidence: high|medium|low | null, source: rules|ai|manual, reason,
  decision: pending|accepted|ignored, user_decided,
  replaces: {release_name, file_path} | null,
  updated, hinted, subtitle_count, superseded_by
}
ProposalTarget { episode_id, season, episode, absolute_number, name }
```

**Proposals**
- **One per file**, except subtitles that belong to a video: those are counted in its `subtitle_count` and follow its mapping. `file_path` is the torrent's own path; a multi-file torrent's paths start with `{torrent.name}/`, and the UI shows them without that prefix.
- **Order:** by group (needs_review, unmatched, specials, matched, extras), then first target (regular seasons before Specials), then confidence, then path.
- **Groups** are set when the job is classified (the rules, then the first AI run in `resolving`, which may move a file between groups or make it an extra) and never change after that, so rows don't jump. Each group starts with a default decision:

  | Group | What's in it | Starts |
  |---|---|---|
  | `matched` | Regular episodes, `high` | `accepted` |
  | `specials` | Season 0, `high` | `accepted` |
  | `needs_review` | Medium or low confidence, or in a conflict | `pending` |
  | `unmatched` | Videos with no episode. "Specials are off for this show" when there are no season 0 rows | `pending`, no targets |
  | `extras` | Openings, endings, samples, trailers, featurettes, superseded v1s, non-video files, and subtitles no video claims | `ignored` |
- **`targets`.** A multi-episode file has several `targets`, in order: it's one file, not a conflict. `suggested_targets` is what the classifier or AI last suggested.
- **Movie jobs.** Candidates are `kind: movie` with no targets, ranked: one plausible file is `matched`, several are `needs_review`. The user picks one by accepting it, which ignores the others. Candidates never conflict.
- **`confidence`** is null with no target and for a manual pick.
- **`reason`** is at most 120 characters and never starts with "AI:": the UI labels AI reasons from `source`.
- **Conflicts:** two or more non-ignored proposals whose targets share an episode. They block Apply. Subtitles never conflict: one that no video claims can be accepted onto an episode a video also maps to.
- **`replaces`.** An episode that already has a file from elsewhere isn't a conflict. The proposal gets a `replaces` note for its first target instead, computed when read; it never blocks Apply, but auto-apply waits.
- **`superseded_by`** points from a v1 file to its v2 in the same torrent; the v1 sits in `extras` as `superseded`.
- **`updated`/`hinted`:** the latest re-check changed this proposal (and had a hint). Both reset when the next re-check starts.

**PATCH**
- **`decision`** sets the decision. `accepted` needs targets, or a movie candidate.
- **`episode_ids`** replaces the targets. It's never empty (`400 bad_request`): to leave a file out, ignore it. The episodes must be this show's, Specials included.
  - The `suggested_targets` ids restore the suggestion, with its `confidence`, `source` and `reason`.
  - Any other ids make a manual pick: `source: manual`, `confidence: null`, `reason` "You picked this episode".
- **`user_decided`** turns true when the user changes a proposal. It turns false again once the proposal is back on its suggestion with its group's default decision, so Undo is just another PATCH.
- **Movie jobs.** Accepting a candidate ignores the job's other candidates; when one request accepts several, the first wins.
- **Extras.** An extra the user includes and gives an episode (`episode_ids` plus `accepted`) keeps its kind and group; Apply writes it as that episode's file.
- **Neither moves the group.** The answer is the whole detail, with the status, counts and conflicts recomputed. PATCH works during `resolving`; the status is then left to the end of the AI run, which recomputes it.
- **Items apply in order** (an id may come twice: the later change builds on the earlier one), all in one transaction.
- **`409 job_working`** before there are proposals (`queued…classifying`).
- **`400 bad_request`** for a malformed body: no `items`, an item without `id`, with neither field, with an unknown `decision`, or with an empty `episode_ids`.
- **`422 validation_error`** for a well-formed change that can't apply:
  - an unknown proposal id;
  - an episode outside the show;
  - `episode_ids` on a movie job;
  - accepting without targets;
  - episodes for a file that isn't a video (fonts, images, info files; a subtitle may take one).

**Re-check (reclassify)**
- **Scope.** A re-check asks about episode proposals (videos) the user hasn't decided: `scope: "unresolved"` the pending ones (unmatched included) and those in a conflict; `"all"` every one. Extras and subtitles keep the rules' reading. AI writes skip `user_decided` proposals (decided meanwhile too), so the user's decisions always survive. `progress.ai_files` is the scope's size; an empty scope settles at once without asking.
- **What changes.** Starting a re-check clears `ai_error` and every `updated`/`hinted`, and its `hint` replaces the job's text hint (season and episode stay). The status goes to `resolving`, then gets recomputed. Each changed proposal gets new `targets` and `suggested_targets`, `confidence`, `reason`, `source: ai` and `updated: true` (plus `hinted`). Its kind and group stay the same, and an accepted proposal never loses its target. A `high` rules answer is never overridden (the rules run again on the stored file list to tell which answers are theirs, so one an earlier re-check flagged stays protected): the AI's disagreement keeps its targets, drops it to `medium` and names the AI's pick in its reason, as in `resolving`. Decisions stay too, except that a disputed proposal still on its group's default `accepted` (the user hasn't decided it) goes back to `pending`, so the job can't settle `ready` with a dispute. When a later re-check's AI agrees with a flagged rules answer, the flag is lifted: the rules' `high` and reason come back, and the decision stays `pending` for the user. An earlier AI answer may be replaced.
- **When the AI run fails** (or this month's budget is spent), the job sets `ai_error` and leaves every proposal as it was; when only some of its requests fail, the others' answers are kept. Usage is recorded either way.

**Apply**
- **One transaction,** guarded by `UPDATE … WHERE status = 'ready'`:
  - one assignment row per target episode, upserted (`ON CONFLICT … DO UPDATE SET is_active = true`) with `confidence` and `match_source`;
  - the episode's other active rows are deactivated;
  - it's authoritative for the item and info hash: earlier rows from this torrent that the job no longer maps are deactivated;
  - subtitles go to their video's first episode;
  - episode ids are re-validated, and only videos become episode files;
  - an episode moving to another file (even of the same torrent) drops its torrent subtitles: they belong to the old file, and the new file's are recorded;
  - then the VFS tree is rebuilt (`InvalidateTree`).
- **Unmatched and ignored files** are left out. They never block.
- **VFS.** Rows that share `(info_hash, file_path)` and have a `match_source` become one entry per season folder and run of consecutive episodes, `Show - S01E01-E02 - FirstName.ext` (first and last episode, the first one's name); a file mapped to E07 and E09 is two entries, `S01E07` and `S01E09`, never `S01E07-E09`, and the file's torrent subtitles sit next to each; the subtitle sidecars of all its episodes follow that name, and the catalog's `webdav_path` too. Legacy rows keep their per-episode paths. Unassigning one episode of such a file rebuilds the tree, which renames the rest.

**Discard** cancels the job. It removes the torrent when no active assignment or other open job uses it.

**`GET /shows/{id}/episodes`** feeds the episode picker. It lists every season in number order, so Specials (season 0) come first. Specials are listed whenever season 0 rows exist, even with `include_specials` off. `has_file` means the episode has an active assignment. A season's `name` is TMDB's, else "Specials" or "Season N". One query.

**Manual overrides** (`PUT /episodes/{id}/assignment`, `PUT /movies/{id}/assignment`) make one file of a torrent the item's file, outside any job. They write through Apply's writer (the title's lock, the upsert, the item's other active rows deactivated) with `match_source: manual` and no `confidence`, then rebuild the VFS tree. Unlike Apply they're not authoritative for the torrent: its other rows on the title stay. A pair the item had before is reactivated. When the item's file changes its torrent subtitles go, and the subtitles a job linked to the new file are recorded. The answer is `204`; refetch the title.

**A torrent's file list** (overrides and `GET /torrents/{hash}/files`) comes from the newest job that stored it as proposals, else from the loaded torrent, else the torrent is added again with a magnet an assignment or a job has, waiting at most 25 s, under the web proxy's 30 s (`504` past that). The add carries on; a torrent added only to list its files, or whose add lands after the request gave up, is dropped again when nothing uses it. The hash may be hex in either case, or base32. `mapped_to` comes from the active assignments: `{movie_id}` for a movie's file, else its episodes in order.

### HDRezka (#63)
A movie or an episode can play from an **HDRezka** stream instead of a torrent file: original audio with subtitles, without an account, so 1080p is the top quality. It's opt-in (`HDREZKA_ENABLED`, off by default; `HDREZKA_URL`, default `https://rezka.ag`). An item has one active source, and picking one replaces the other in either direction (`008_stream_sources.sql`, §5). Episodes of one show may mix sources.
```
GET  /hdrezka/search?media_type=movie|tv&tmdb_id=[&q=]
     → {items: [{url, path, title, original_title, year, kind, category, rating, match: exact|likely|weak}], query}
GET  /hdrezka/titles?url=[&translator_id=][&show_id=|&movie_id=]
     → {url, path, post_id, kind, title, original_title, year,
        translators: [{id, name, default, premium, camrip, ads, director}], translator_id,
        seasons: [{number, episodes: [{number,
                   library: {episode_id, season, episode, name, air_date, has_file, source, stream} | null}]}]}
GET  /hdrezka/streams?url=&translator_id=[&camrip=&ads=&director=][&season=&episode=]
     → {qualities: [{label, resolution, premium, available, size_bytes | null}], subtitles: [{language_code, language_name}]}
POST /movies/{id}/assign-hdrezka {url, translator_id, camrip?, ads?, director?, quality} → 204
POST /shows/{id}/assign-hdrezka  {url, translator_id, quality, items: [{episode_id, season, episode}]}   // 1–30 items
     → 200 {assigned: [{episode_id, quality, size_bytes}], failed: [{episode_id, code, message}]}
```
- **`url`** accepts a URL on any mirror, or a bare path. The server keeps only the validated path and fetches only `{HDREZKA_URL}{path}`, so there's no SSRF.
- **Fields** (as the web reads them, `web/src/lib/api/contract/hdrezka.ts`): `title` is the site's (Russian) title; `original_title`, `year` and `rating` (Kinopoisk) are null when the site gives none; a search hit's `kind` is null when its category doesn't tell (cartoons, anime); `category` is the path's first segment (`films`, `series`, `cartoons`, `animation`). Translator names are the site's own. A quality's `label` ("1080p", "1080p Ultra") is what assign sends as `quality`; `resolution` is in lines (0 when unknown); `available` means there's a progressive MP4 and the quality isn't premium; `size_bytes` is null for a premium quality or one the CDN didn't measure within 8 s; a subtitle's `language_code` may be `""`. A search hit's and a title's `url` is the page on `HDREZKA_URL`.
- **Search** without `q` asks for the TMDB original title, then the title when that finds nothing; each query tries the quick search, then the full search. Hits are graded against TMDB's title, original title, year (±1) and kind. Without `q` it needs TMDB (`502 tmdb_unavailable`, or `503 unavailable` without a key); with `q`, a TMDB outage only costs the grading.
- **Translators and cuts.** HDRezka can list one translator id several times with different `camrip`, `ads` and `director` flags (a director's cut under the same id), and a movie's stream depends on them; `translators` lists every cut. Streams and the movie assign take the flags optionally: each one given must match, and an omitted one doesn't constrain, so with none it's the id's first entry (never "all false"); `422` when no entry matches. Series ignore the flags (their episodes don't take them). The stored `StreamRef` keeps the flags of the entry used. `/hdrezka/titles` without `translator_id` answers for the page's default (the init call's id and flags), or the first translator that isn't premium when the default is.
- **`/hdrezka/streams`** for a series without `season` and `episode` is the translator's first episode.
- **Errors:** `400 bad_request` (a malformed request: no `url`, `translator_id` or `quality`, a non-numeric parameter, an empty, over-30 or duplicated `items`), `404 not_found`, `409 premium_only`, `422 validation_error` (not an HDRezka title link, the wrong kind, an unknown translator or cut, an episode that isn't the show's or that HDRezka lacks in that voice-over, a quality not offered, HLS only), `502 hdrezka_unavailable` (the site or CDN failed, the markup changed, the CDN refused a link twice; a CDN `403` too), `502 tmdb_unavailable`, `503 hdrezka_blocked` (the anti-bot page) or `503 unavailable` (disabled), `504 timeout`. Never `401` or `403`: the web proxy reads those as `backend_auth`. A show assign's `failed[].code` uses the same codes.
- **Assigning** is synchronous, with no job: at most 30 episodes a request (the web sends chunks) within a 25 s budget, under the web proxy's 30 s. Everything that needs no network is checked first (the body, the items, that each episode is the show's), then with one request that HDRezka has each `season`/`episode` in the voice-over; then each item is resolved and measured, four at a time. `quality` is the label asked for; when it isn't offered without Premium, the best quality at or below its resolution is used (`assigned[].quality` says which; for a movie, the title's `stream.quality`). The size is measured then. It's one write through Apply's writer (`AssignmentRepository.WriteStream`, partial: the title's other items keep their files), the streams' subtitles (see "Subtitles" below) and one `InvalidateTree`; items that failed are in `failed`, and when none was assigned the answer is the first failure's error. The same stream again reactivates its row; another translator or quality replaces it.
- **Titles:** `TitleLibraryState.stream {provider, title, url, translator, quality, size_bytes, added_at} | null`, and `torrent` is null while it's set. Its `title` is HDRezka's original title, else its (Russian) title; `url` is the page on `HDREZKA_URL`. `EpisodeSummary.source torrent|hdrezka|null` and `EpisodeSummary.stream {translator, quality} | null`. Under "Availability rules" an HDRezka file is never `downloading`.
- **`/hdrezka/titles` `library`** is the library episode an HDRezka episode maps to, with its file's `source` and `stream` (the newest active row); null when it maps to none.
- **Mapping review:** a proposal whose episode plays from HDRezka gets `replaces: {release_name: "HDRezka · LostFilm · 1080p", file_path: ""}`.
- **VFS:** the file is named like any movie or episode, with `.mp4`, and never grouped with other episodes. momoshtrem resolves the signed link (they expire after about 20 h; cached at most 4 h, and never past an hour before the expiry the link carries, `…:<YYYYMMDDHH>:…`, read as Moscow time) and proxies the bytes with `Range: bytes=off-`, resolving again once on `403`, `404` or `410` and retrying a `429` or `5xx` once. Opening a file does no I/O (a PROPFIND opens every file); one body serves sequential reads, a skip of up to 256 KiB is read through, a jump opens a new body, a read that breaks reopens once where it stopped, and one that receives nothing for 30 s fails. At most 8 bodies are open at once. The CDN must agree with the size stored at assign time (a `206` with that total); otherwise reads fail and the item must be assigned again. While HDRezka is off its files still list, but reads fail.
- **Subtitles (#68).** An assign downloads each assigned item's subtitle tracks after the write, because their VTT links expire like the MP4s.
  - **Downloading:** four tracks at a time, each within 15 s and at most 2 MB, with the client's user agent. Signed links never reach logs or errors.
  - **Storing:** each track is converted to SRT (`subtitle.VTTToSRT`) and stored with source `hdrezka` (§5 `009`). There's one per language: `language_code` is the site's (its `ua` read as `uk`); without one, the code the track's name gives, else a slug of the name. `release_name` is the stream's label ("HDRezka · Оригинал (+субтитры) · 1080p") and `match` is `exact_release`.
  - **Timing:** the subtitles are recorded before the tree is rebuilt when they're in within the assign's 25 s budget. Otherwise the tree is rebuilt at once and again when they're in; they carry on in the background for up to 2 minutes. 30 episodes with three tracks each needn't fit in the budget, and the answer, which doesn't change, never waits past it.
  - **Failures:** a track that fails is logged and never fails the assign.
  - **Refreshing:** assigning the same stream again refreshes them in place. Another file (a translator, a quality or a torrent) drops them through the writer, and the next stream's replace them, its files too. Recording holds the item's stream row until it commits, so an assign that comes meanwhile waits and drops them; one that came first leaves them unrecorded.
  - **Showing:** they show only while the item plays from HDRezka (`GET …/subtitles`, and WebDAV as `{video}.{lang}.srt` next to the `.mp4`), and can be deleted. A track stored under a slug is named in WebDAV after the language its name starts with, so Infuse sees Breaking Bad's "Русский (альтернативный)" as a second Russian track, `….2.ru.srt`.
  - **Searching:** an HDRezka item has no release name to rank OpenSubtitles results against, so their matches are `unknown`.
- **`GET /status`** adds `hdrezka: {configured, reachable, latency_ms?, error?}`: one GET of the mirror's home page, only while HDRezka is on. The anti-bot page reports `error: "HDRezka asks for a browser check"`.

### Search (B4)
Torrent search across Jackett's indexers, ranked by the quality profile. Delivered by #6's slices. Types: `web/src/lib/api/contract/search.ts`.
```
GET  /search/torrents?media_type=movie|tv&tmdb_id=[&season=&episode=][&scope=][&q=][&<filter>=…][&sort=][&page=&page_size=][&refresh=1]
     → SearchTorrentsResponse
POST /search/torrents/resolve {release_id} → 200 {magnet_uri, info_hash}
POST /search/torrents/inspect {media_type, tmdb_id, release_id | magnet_uri, hint?: {season?, episode?}} → 202 {job_id}
GET  /settings/quality-profile → QualityProfile
PUT  /settings/quality-profile QualityProfile → QualityProfile
```

**`GET /search/torrents`**
- **Context:**
  - `media_type=movie|tv` and `tmdb_id` are both required; `season` and `episode` are optional.
  - `scope=episode|season|any` (TV only; ignored for movies):
    - default `episode` when an episode is given, `season` when only a season is given, otherwise `any`;
    - `scope=episode` without an episode, `scope=season` without a season, or an episode without a season, is `400 bad_request`.
  - `q`: the client sends it only when the user edited the field. It replaces the generated text queries, ID queries are skipped, and the scope still filters.
- **Filters:**
  - `resolution`, `source`, `codec`, `hdr`, `audio`, `pack`, `indexer`, `group`: each repeatable, OR within a filter and AND across filters. An unknown value matches nothing.
  - `min_seeders`.
  - `min_size_gb` and `max_size_gb`: per episode for TV (size ÷ the episodes it covers, so a pack compares with a single episode), the whole release for movies.
- **Sort and paging:**
  - `sort=best|seeders|size|newest`, default `best`: `seeders` most first, `size` smallest first, `newest` by `published_at` (unknown last);
  - `page`, default 1;
  - `page_size`, default 25, max 100;
  - `refresh=1` (Retry) re-runs the search, at most once per 10 s per plan; a second one within 10 s reads the running or cached search.
- **Query plan** (no `q`):
  - an ID query: `imdbid`, `tvdbid` or `tmdbid`, only to indexers whose caps support it, with `season`/`ep`;
  - normalised title queries: strip `: ' ! ,` and similar, with `&` as "and"; "Title Year" for movies, "Title SxxEyy" for an episode, "Title S01" for a season, "Title" for `any`; each again with the original title when it's in Latin letters (`"original title"`);
  - anime:
    - the primary is the short title (before ":") with the absolute number (`Frieren 29`);
    - the romaji title from TMDB's alternative titles takes the original title's place (`Sousou no Frieren 29`);
    - scene numbering (TMDB S01E29 is release S02E01, from cours or episode groups) is added, and a season adds the scene seasons it holds (`"scene numbering"`);
    - without scene numbering, TMDB's own SxxEyy is added, with a null label;
  - a show the library numbers absolutely that isn't anime adds "Title 17" (`"absolute number"`);
  - when the exact queries find nothing, a show-level fallback (the title, and the original title) whose results are filtered by coverage (`"whole show"`).
- **What each scope keeps:**
  - `episode`: releases that contain the episode, packs included;
  - `season`: any release that isn't a single episode and has at least one episode of the season;
  - `any`: everything that names the title.
- **Hygiene:**
  - one Torznab request per indexer per query, with 90 s per request: an indexer behind Cloudflare answers only after FlareSolverr's challenge, which Jackett solves one at a time (#106). The indexer list has 8 s;
  - requests ask for the title's categories only: TV and anime for a show, movies (plus anime for an anime film) for a movie. Adult categories (the 6000 range) are never requested, and results in them are dropped;
  - HTML entities are decoded;
  - text-query results whose title matches none of the known titles are dropped (unless `q` was given). Known titles are the title, original title, romaji and aliases, plus their short forms for anime ("Frieren"); a trailing country tag ("The Office US") and any part of a slash-separated name count. A movie's text-query results from a year more than 1 off are dropped too. ID-query results are trusted;
  - duplicates are merged by info hash, otherwise by normalised title + size: the most seeders wins, and a magnet is preferred. Sizes match within 5%, because indexers round them ("1.4 GiB"), and two different info hashes are never merged. The copy that stays takes the other's magnet when it has none.
- **Progress:**
  - a GET starts the plan's search unless it's cached or already running;
  - the search runs detached from the request, with a single flight per plan, and stops after 180 s, two passes of 90 s (a `warnings` entry says so);
  - the GET that starts it answers when the search completes or after 2 s, whichever comes first; a GET that finds it running answers at once;
  - while `complete` is false, the client polls the same URL every second, without `refresh`;
  - filters, sort and paging apply to what has arrived so far.
- **Cache:**
  - 10 minutes per plan from when the search started, keyed by (media type, TMDB id, scope, season, episode, `q`). Filters, sort and paging aren't part of it, so they never search again;
  - at most 200 plans, the oldest dropped first. A search that couldn't start (`jackett_unavailable`) isn't cached, so the next GET tries again;
  - the title's files and its `missing` run are read on every GET, so an episode added meanwhile counts at once;
  - scores and the profile's hide rules are applied when results are read, so a profile change takes effect at once.
- **Response:**
  ```
  SearchTorrentsResponse {
    items: [Release], total, page, page_size,   // total: after filters
    total_unfiltered,           // in scope, after the profile hides, before filters ("Your filters hide all 27 results")
    hidden,                     // dropped by the profile (avoid terms, above max_resolution)
    best_id,                    // string | null; sort=best puts it first on page 1
    facets: { resolution, source, codec, hdr, audio, pack, indexer, group },   // each [{value, count}]
    queries: [{q, label}],      // text queries sent, primary first (the field shows queries[0].q);
                                // label null | "original title" | "absolute number" | "scene numbering" | "whole show"
    indexers: [{name, status: pending|ok|timeout|error, count, ms, error?}],   // ms: time so far while pending
    complete,
    missing: {season_number, from, to, absolute_from, absolute_to, count, best_covers} | null,
    cached_at,                  // when the search started
    warnings: [string]          // problems that aren't about one indexer
  }
  ```
  - `indexers`: one entry per indexer over all the plan's queries. `pending` while any of its requests runs; `ok` when every one answered; otherwise its first failure, with `error` saying why (never the key or a link). `count` is what it returned.
  - `queries`: `"whole show"` labels the fallback query.
  - `missing`: the title's `library.missing` run (the frontier, under "Availability rules"), only when the context's season is the run's and its episode, if any, is in the run. It's null for movies, for titles not in the library and for one-episode runs. `best_covers` means the best match covers the whole run. It drives "10 episodes are missing · A pack gets 29–38 in one go. The best match below already does."
- **Release:**
  ```
  Release {
    id,                         // stable per indexer result across searches; indexed 1 h for resolve and inspect
    raw_title, display_title,
    info_hash | null, magnet_uri | null,
    size_bytes, seeders, leechers, published_at | null, indexer,
    attrs: { resolution, source, codec, bit_depth: 10|12|null, hdr: [], audio: [], channels: "7.1"|"5.1"|"2.0"|null,
             languages: [],         // "multi", "dual", or ISO 639-1 codes
             subs: "multi"|"hardcoded"|null, group | null, edition | null, repack,
             pack: {type, label, seasons: [n], episodes: {from, to} | null} | null },  // release numbering; null for movies
    score,                      // 0–100
    score_reasons: [string],    // positive only, most important first; the UI shows at most 4
    notes: [string],            // e.g. 'Scene "Season 2" = TMDB S01E29–E38', "Covers all 4 seasons, 53 episodes"
    warnings: [string],         // e.g. "CAM recording · filmed in a cinema", "Only 4 seeders · may be slow", "Over your 8 GB/hour limit"
    flags: [cam|no_seeders|low_seeders|size_outlier|over_size_limit|needs_torrent_fetch]
  }
  ```
  - `needs_torrent_fetch`: no magnet yet, so Add and Copy magnet call resolve first. A release with an info hash but no magnet gets one built from the hash and its name (`magnet:?xt=urn:btih:…&dn=…`), so it isn't flagged.
  - `hdr` and `audio` list only what the name says; `repack` covers PROPER too.
  - `pack` (parsed by `internal/release`):
    - `label` is short: "S01E29", "S01E01–E03", "Episode 29" and "Episodes 29–38" (absolute numbers, `seasons: []`), "Season 2", "Specials", "Seasons 1–4", "Complete", "Batch".
    - `complete` is the whole show without seasons ("Complete Series", or "Complete" with no seasons named). "S01.COMPLETE" is a `season`, and "S01-S04.COMPLETE" a `multi_season`; only "Complete Series" or "Complete Collection" makes named seasons `complete`. "Batch" alone never does (#122).
    - `unknown`: markers that don't add up, such as episodes of two seasons ("S01E05-S02E03") or a half episode ("- 28.5"), and a Batch with no numbers, which doesn't say what it holds (label "Batch").
    - Some indexers send names with "-", "+" and "." turned into spaces. Those read as the original where the spaces can mean one thing: "(01 28)" right after the title (or its season) is episodes 1–28, "S01 S04" (one space) is Seasons 1–4 while wider gaps and longer runs stay lists, and "DDP5 1", "TrueHD 7 1" and "[5 1]" are channels.
- **Allowed values** (they are both the facet values and the filter values):
  - `resolution`: 2160p, 1080p, 720p, 576p, 480p, unknown;
  - `source`: REMUX, BluRay, WEB-DL, WEBRip, WEB, HDTV, DVD, CAM (which also covers TS, HDTS, TELESYNC, TC and SCR), unknown;
  - `codec`: x265 (HEVC, H.265), x264 (AVC, H.264), AV1, other, unknown;
  - `hdr`: DV, HDR10+, HDR10, HDR, SDR (the facet's and filter's `SDR` also covers releases whose `hdr` is empty);
  - `audio`: Atmos, TrueHD, DTS-HD, DTS, DD+, DD, AAC, FLAC, Opus;
  - `pack.type`: episode, multi_episode, season, multi_season, complete, unknown;
  - `indexer`: names; `group`: release groups.
- **Facets:**
  - disjunctive: each facet is counted with every other active filter applied and its own left out, so a count is what selecting that value would give (this replaces HANDOFF §6's `would_be`);
  - the filter sheet's "Show N results" asks the same endpoint with the draft filters and `page_size=1`;
  - buckets list every value present in the unfiltered scope, plus any selected value;
  - order: allowed values in the order above; `indexer` and `group` by count (`group`: top 20);
  - `hdr` and `audio` are multi-valued: a release counts once per value. `pack` is empty for movies.
- **Profile effects:**
  - hidden (counted in `hidden`): any avoid term as a whole word, case-insensitive; a resolution above `max_resolution`;
  - warnings and score penalties: CAM, no seeders, under 5 seeders, over `max_gb_per_hour`, an implausible size;
  - `best_id`: the top score among the filtered releases flagged neither `cam` nor `no_seeders`, with a score of at least 50;
  - a pack that includes episodes that already have a file gets a note that's true: adding replaces their files, and the mapping review asks first (auto-apply waits on `replaces`, "Assignment").
- **Errors:**

  | Status | When |
  |---|---|
  | `400 bad_request` | A malformed request: no `media_type` or `tmdb_id`, a scope without its season or episode, an episode without a season, an unknown `scope` or `sort`, a malformed number |
  | `404 not_found` | Unknown TMDB title |
  | `502 tmdb_unavailable` | The title isn't in the library, there's no `q`, and TMDB is down |
  | `502 jackett_unavailable` | The indexer list can't be fetched, or Jackett rejects the key |
  | `503 unavailable` | Jackett isn't configured |
  | `200` | Every indexer failed: the answer carries the per-indexer statuses |

**`POST /search/torrents/resolve`** with `{release_id}` → `200 {magnet_uri, info_hash}`
- The client calls it before Add or Copy magnet when a release has no magnet.
- The Torznab `link` carries the Jackett API key, so it never leaves the server.
- The server fetches only the link's path and query on the configured `JACKETT_URL`, never the link's own host (the SSRF guard).
- A redirect to `magnet:` is the answer; any other redirect is refused.
- The `.torrent` file is capped at 5 MB, read with `metainfo.Load`, and turned into a magnet with `dn` and trackers.
- Errors: `400`; `404 not_found` (expired: search again); `502 torrent_unavailable`; `504 timeout` (25 s: the `.torrent` can come through FlareSolverr too, #106).

**`POST /search/torrents/inspect`** with `{media_type, tmdb_id, release_id | magnet_uri, hint?: {season?, episode?}}` → `202 {job_id}`, or `200 {job_id}` when the same title and torrent (info hash) were inspected in the last hour.
- **Reuse:** the earlier dry run is answered only when it wasn't cancelled and didn't fail, and the title's library state is the same: a title added to the library since gets a new dry run, with library ids. The new hint is then ignored.
- The job is a dry run (`dry_run: true`) with the AssignmentJob shape; `hint` becomes the job's hint (season and episode; shows only). A `release_id` is resolved as `resolve` does, and the release's indexer fills `torrent.indexer`; a `magnet_uri` is used as it is.
- For a title not in the library:
  - `item.id` is null, and `item.title`, `year` and `poster_path` come from TMDB;
  - for a show, `targets[].episode_id` is the TMDB episode id;
  - there's no metadata refresh, and the classifier runs rules only (`ai_available` is false);
  - `progress.seasons` and `episodes` count TMDB's seasons and episodes, specials included.
- A library show runs the normal pipeline, AI included.
- Dry runs are read-only: PATCH and reclassify answer `409 dry_run`, as Apply already does. Discard works.
- Dry runs are deleted after 24 h, and their torrent is dropped then unless something else uses it.
- Inspect is a nested sheet inside the search sheet, not a URL sheet.
- Errors: `400 bad_request` (neither `release_id` nor `magnet_uri`, an unknown `media_type`, no `tmdb_id`, a magnet without an info hash, a negative season or an episode below 1), `404 not_found` (an expired release or an unknown TMDB title), `502 tmdb_unavailable` (TMDB is down for a title outside the library), and resolve's `502 torrent_unavailable` and `504 timeout` for a `release_id`.

**`GET` and `PUT /settings/quality-profile`**
- `PUT` takes the whole profile and answers the saved one. Invalid values are `422 validation_error` with `error.field`, the bad field's JSON key, and a `message` that names it too: `{code: "validation_error", message: "max_gb_per_hour must be between 1 and 20", field: "max_gb_per_hour"}`. Settings shows `message` next to that field. A value of the wrong JSON type is `400 bad_request`.
- `PUT` trims terms and groups and drops case-insensitive duplicates (the first spelling stays) before checking the limits, so the answer can differ from the request.
- Stored in `app_settings` under the key `quality_profile` (§5). A missing field takes its default, in a `PUT` too.
```
QualityProfile {
  preferred_resolution: "720p"|"1080p"|"2160p",  // default "1080p"
  max_resolution: "1080p"|"2160p"|null,          // null = no limit (default); must be ≥ preferred
  max_gb_per_hour,                               // 1–20, step 0.5; default 8; soft (a warning and penalty, not hidden)
  prefer_x265, prefer_hdr, prefer_dolby_vision,  // default false
  avoid_terms: [string],                         // ≤ 30 terms of 1–30 chars; default ["CAM","HDCAM","TS","HDTS","TELESYNC"]; matching releases are hidden
  preferred_groups: [string]                     // ≤ 30; a small boost that never overrides seeders or the size limit; default []
}
```
- `max_gb_per_hour` compares the size per episode (or the movie's) with the runtime; without a runtime there's no check. Sizes are decimal (1 GB = 10⁹ bytes), as the web shows them, so a reason's "15.6 GB" is the card's.
- An avoid term matches as a whole word of the release name: any run of characters other than letters and digits separates words, so "TS" hides "Movie.2023.TS.x264" but not "Ghosts", and "WEB-DL" also matches "WEB.DL".
- There's no preview endpoint (in place of HANDOFF §6's `/settings/quality-profile/preview`): "Best match would prefer…" is a client-side function of the profile.

### Trakt (B5)
Types: `web/src/lib/api/contract/trakt.ts`. Read-only: nothing is ever written to Trakt. Served by `internal/trakt` (#131 connect, #132 sync) and `internal/me` (#133); the web mocks are a state machine (`web/src/mocks/fixtures/trakt.ts`).

- **`GET /integrations/trakt`** → `200 TraktIntegration`:
  ```
  TraktIntegration {
    configured,                        // TRAKT_CLIENT_ID and a valid SECRET_KEY (TRAKT_CLIENT_SECRET is optional)
    state: off | pending | connected | reauth,
    connected,                         // state = connected
    username,                          // while an account is linked: connected, reauth, or a code pending from reauth
    pending: {user_code, verification_url, expires_at, interval} | null,   // the live device code while pending
    connect_error: expired | denied | failed | null,   // why the last code didn't connect; the next connect clears it
    reauth_reason,                     // for the user, while reauth
    last_sync_at,                      // the last sync that finished without an error
    next_sync_at,                      // the next scheduled check; null unless connected
    sync: {
      status: idle | syncing | error,
      kind: initial | incremental | reconcile | null,   // the running run, else the last; null before the first
      step: history | ratings | watchlist | progress | details | null,   // the running step, or the one that failed; null while idle
      progress: {plays, plays_total, ratings, watchlist, shows},   // what the run imported; plays_total is Trakt's count, 0 until known
      error,                           // for the user, while error
      started_at, finished_at          // finished_at null while syncing
    },
    counts: {history, ratings, watchlist, watchlist_in_library},   // what's imported (history: plays)
    settings: {use_ratings, show_watchlist, include_paused}
  }
  ```
  - **States.** `off` → `pending` (a device code) → `connected` ↔ `reauth`. `reauth` means Trakt stopped accepting the tokens (a refresh failed, a second 401, or a `SECRET_KEY` that can't decrypt them). The imported data stays and `/me` keeps answering from it; connecting again goes through `pending`. Without `configured` the state is `off`.
  - **The initial import** after a connect is a reconcile with `kind: initial`; its steps run in order, `history`, `ratings`, `watchlist`, `progress` (shows and the calendar) and `details` (artwork), and each section of `/me` fills as its step finishes.
  - **A sync cut short by the connection** isn't a sync error: one ended by reauth (a second 401) is `idle` with its `kind`, `step` and `error` null, and `reauth_reason` says why; a disconnect puts `sync` back to never synced. Either way `last_sync_at` stays the last sync that finished. The last run is stored (`trakt_sync_state` key `last_run`), so `sync` survives a restart.
- **`POST /integrations/trakt/connect`** → `200 TraktConnect` (`{user_code, verification_url, expires_at, interval}`), from `off` or `reauth`: the device-code flow. While a code is live the same one comes back. The backend polls Trakt every `interval` seconds; the client polls `GET /integrations/trakt`. `409 conflict` while connected; `503 unavailable` without `configured`; `502 trakt_unavailable` when Trakt's device-code endpoint fails.
  - **Outcomes.** Approved: `connected`, and the initial import starts. Expired: `connect_error: expired`. Denied: `denied` (free Trakt accounts can connect only 2 apps; the UI says so). Anything else: `failed`. A code that doesn't connect goes back to `off` (`reauth` when it came from there).
- **`DELETE /integrations/trakt`** → `204`, idempotent. While `pending` it cancels the code only (back to `off`, or to `reauth` while an account is linked). Otherwise it disconnects: it stops the sync (waiting at most 10 s), purges every trakt table in one transaction, then revokes the token (best effort, 5 s), so a failed purge leaves a working connection rather than a revoked one. The settings stay.
- **`PUT /integrations/trakt/settings`** with any of `{use_ratings, show_watchlist, include_paused}` → `200 TraktSettings` (all three), in any state. Fields left out keep their value and unknown ones are ignored; a value that isn't a boolean is `400 bad_request`. Stored in `app_settings` key `trakt` (§5) and kept across a disconnect.
  - `use_ratings` (default on): recommendations (#9) weigh Trakt ratings. `show_watchlist` (on): the watchlist row on For You, and Up next's watchlist starts. `include_paused` (off): Up next keeps shows idle for over 180 days.
- **`POST /integrations/trakt/sync`** → `202 TraktIntegration`, its `sync` running: a sync now (incremental, or a reconcile when the last one is over 24 h old; a failed initial import or reconcile picks up again at the step that failed, with the `last_activities` snapshot it started with, until a restart), or the running one, joined, with exactly one more queued behind it (any trigger during a sync queues one). `409 conflict` unless connected.
- **`GET /me/up-next`** → `{items: [UpNextItem]}`, at most 40:
  ```
  UpNextItem {
    show: {tmdb_id, title, poster_path, backdrop_path, library_id},   // library_id null when not in the library
    next_episode: {season, episode, absolute_number, name, still_path, tmdb_id, air_date},   // tmdb_id: TMDB's episode id
    availability: ready | downloading | needs_review | needs_torrent | not_in_library | airs_on,
    airs_at,                           // only while airs_on: the calendar's first_aired, else the air date at 00:00 UTC; null when unknown
    progress: {completed, aired},      // Trakt's, regular episodes; {0, aired} for a watchlist start
    last_watched_at,                   // null for a watchlist start
    source: progress | watchlist,
    webdav_path, webdav_url,           // the episode's file, while ready or downloading
    download: {pct, info_hash} | null, // while downloading ("Details" opens /activity/{info_hash})
    review_job_id                      // while needs_review: the newest job that wants the episode (review rules)
  }
  ```
  - **The next episode** is Trakt progress's `next_episode`; else (caught up) the first regular calendar entry after `last_episode`, `airs_on` while it hasn't aired; else the show is left out. Specials are never next: a special progress names is skipped like a caught-up show. When the library lists the episode (the matching rule), the card has its season, number, absolute number, name and still, so it opens the episode the title page shows; else Trakt's, the calendar filling in what progress lacks (HANDOFF §6: the landscape card's still, name and `#1124`). The show's title and artwork are the library's when it's there, else Trakt's.
  - **`absolute_number`** (Up next and new episodes) follows the title page's labels: set only for a library show with `episode_ordering = absolute`, null otherwise (Doctor Who's S03E04 never reads `#30`; Trakt has none).
  - **Aired** is checked at read time: by the calendar's `first_aired` when it lists the episode, else the library's air date. Progress's next episode with neither has aired (Trakt names only aired episodes next, and the calendar lists the ones about to air).
  - **Shows**: every `trakt_show_progress` row except dropped or hidden ones (`trakt_hidden`, always left out) and those last watched over 180 days ago (left out unless `include_paused`).
  - Trakt's bulk progress also lists watchlisted shows never started (`completed: 0`, no `last_watched_at`, next S01E01). The sync skips them, so a `trakt_show_progress` row is always a show you've started, and an unstarted show is at most a watchlist start.
  - **Watchlist starts**, only with `show_watchlist`: S01E01 of a watchlisted library show with no plays, once it has aired (the library lists it, by its air date); at most 3, newest listed first. A show with Trakt progress, or dropped or hidden, is no start.
  - **Availability**, first match: the episode has a file → `ready` or `downloading`; a mapping job wants it → `needs_review` (the review rules); it hasn't aired → `airs_on`; the show is in the library → `needs_torrent` (an episode the library doesn't list yet too); else `not_in_library`. It reads the file and review facts, never `watched`.
  - **Order**: the shows that aren't `airs_on` by `last_watched_at`, newest first; then watchlist starts; then `airs_on` by `airs_at` (unknown last).
  - Reading it starts an incremental sync in the background when the last check is over 5 minutes old (#132; `api.Options.TraktNudge`).
  - No `backdrop_dominant_color` (HANDOFF §6 lists one for up-next): the hero loads the title, which has it.
  - Cost: at most 4 queries (the settings, the candidates, their episodes in the library). New episodes 3, the watchlist 2, the history 1.
- **`GET /me/new-episodes?days=7`** → `{items: [NewEpisodeItem]}`, at most 30, newest first (HANDOFF §6's "New episodes" row):
  ```
  NewEpisodeItem {
    show, episode,                     // as UpNextItem's show and next_episode
    aired_at,                          // the calendar's first_aired, else the air date at 00:00 UTC
    availability: ready | downloading | needs_review | needs_torrent | not_in_library,   // as Up next's; it has aired, so never airs_on
    webdav_path, webdav_url, download, review_job_id   // as Up next's
  }
  ```
  - Regular episodes that aired in the last `days` days (1–30, default 7; anything else is `400 bad_request`) and haven't been watched: the library shows' episodes plus the Trakt calendar's, once each. Dropped and hidden shows never show. The window runs from 00:00 UTC `days` days before today until now, by `aired_at`; a calendar entry the library lists (the matching rule) is that episode, aired when the calendar says.
  - Trakt's calendar also covers watchlisted shows, so a calendar entry counts only for a show you've started (a `trakt_show_progress` row) or one in the library.
- **`GET /me/watchlist?type=movie|tv`** → `{items: [WatchlistItem]}`, newest listed first: `{media_type, tmdb_id, title, year, poster_path, listed_at, library}`, where `library` is `SimilarTitle.library` (`{library_id, availability_stage, episodes_total, episodes_ready} | null`) from one batch lookup. Another `type` is `400 bad_request`.
- **`GET /me/history?page=&page_size=`** → `Paginated<HistoryItem>`, 50 a page by default (at most 200; otherwise `400 bad_request`), newest first (`watched_at DESC, id DESC`): `{id, media_type, tmdb_id, title, year, season, episode, episode_name, watched_at}`. `id` is Trakt's history event id and `tmdb_id` the movie's or the show's; `season`, `episode` and `episode_name` are null for movies.
- **Without Trakt** (nothing imported: `off`, or `pending` before the first import) every `/me/*` answers `200` with no items, except new episodes, where the library's shows still count. In `reauth` they answer what was imported.
- **Watch state** in `/titles`, seasons and `/library`: "Watch state rules" under "Titles and library (B1)".
- **The web app** polls `GET /integrations/trakt` every 3 s while `state` is `pending` or `sync.status` is `syncing` (never in a hidden tab), and when a sync stops it refetches `/me`, the title pages and the library (`web/src/hooks/useTraktIntegration.ts`).

### Recommendations (R1–R3)
Types: `web/src/lib/api/contract/recommendations.ts` and `settings.ts`; Go wire structs in `internal/recommend/types.go`. Served by `internal/recommend` (R1 #153, R2 #157, R3 #158) on the AI core (#154: the Claude client, our tool loop, pricing, the budget and `/settings/ai`). Routes: `internal/api/{recommendations,ask,ai_settings}_routes.go`, behind four interfaces (`Options.Recommendations`, `RecFeedback`, `Ask`, `AISettings`), each answering 503 until its service is wired (`cmd/momoshtrem/wire_{ai,recs,ask}.go`). The handlers answer every list as `[]`, never `null`. The web mocks are a state machine (`web/src/mocks/fixtures/recommendations.ts`; scenarios `recs-*`, `ask-*`, `ai-off`).

- **What needs what.** Every AI call needs `ANTHROPIC_API_KEY` (*configured*). Picks (shelves, hero picks, runs and the title page's reason) also need `rec_enabled`, the master switch (on by default): off, `/home` is empty and no run is queued. Ask depends on the key only. Feedback, threads and the AI settings read and write in any state.
- **The budget.** `monthly_budget_usd` ($15 on a fresh install; `null`: no cap) is shared by every AI feature. Once this month's spend (`ai_usage`) reaches it, runs are skipped, Ask and refresh answer `429 budget_exceeded`, and the classifier runs on rules. It's checked before each run, turn and classifier AI run, so one in flight can overshoot it by its own cost.
- **Time.** "Today", "this month", the schedule and the hero's day are server-local: the service's `TZ` (§4; UTC by default). `/status`'s `ai.mtd_cost_usd` follows the same month. Every time boundary (the budget month, the hero's day, Ask's daily limit, the schedule's slots) is computed in Go with `time.Local` and passed to SQL as a parameter, never `date_trunc(…, NOW())`: Postgres's session zone isn't `TZ`.

**Shapes**
```
RecItem {
  media_type, tmdb_id, title, year, poster_path,
  year_end,                    // shows: the last year; null while running, and for movies
  seasons,                     // shows: the season count, Specials left out; null for movies
  runtime_minutes,             // a movie's runtime, a show's episode runtime; null when unknown
  vote_average,                // TMDB's; null without votes
  release_date,                // YYYY-MM-DD: a movie's release, a show's first air date; null when unknown
  badge,                       // a short poster label ("Jan 16", "2026"), mostly on coming_soon shelves; else null
  reason,                      // ≤ 140 characters, grounded in the taste profile
  // the facts above are the run's snapshot (rec_items.meta); these are read live:
  library: {library_id, availability_stage, episodes_total, episodes_ready} | null,   // SimilarTitle.library
  on_watchlist,                // on the Trakt watchlist (what's imported)
  feedback: like | null        // a title with negative feedback is left out instead
}
AskItem = RecItem + { match_tag }   // ≤ 32 characters: "Mystery + workplace · lighter"
Shelf { slug, kind: because|new|anime|gems|coming_soon|other, title, subtitle | null,
        items: [RecItem] }     // 6–15 when written; read-time feedback can thin it, and an empty shelf is dropped
HeroPick { media_type, tmdb_id, kicker, reason, feedback: like | null }   // the hero loads the title (artwork, colour)
RunSummary { id, kind: scheduled|manual|first_sync, status: queued|running|succeeded|failed|skipped, model,
             started_at, finished_at,      // started_at null while queued and for a skipped run
             cost_usd,                     // so far, while it runs
             skip_reason: budget_exceeded|no_history | null,
             error | null }                // for the user, while failed
RunProgress = RunSummary + { steps: [RunStep], shelves_done, shelves_target }   // shelves_target 0 until it starts
RunStep { step: history|web|tmdb|write, state: active|done, detail | null }
RunDetail = RunProgress + { usage: {api_calls, input_tokens, output_tokens, cache_read_tokens, web_searches, cost_usd} }   // its ai_usage rows, summed
RecommendationsHome { configured,       // ANTHROPIC_API_KEY is set: false hides the Ask entries too
                      enabled,          // configured and rec_enabled: false with configured true is "picks off" (Ask still works)
                      run: RunSummary | null, active_run: RunProgress | null, hero_picks: [HeroPick], shelves: [Shelf] }
```
- **A run's steps** are the ones reached so far, in the order reached, each once, with at most one `active` (the newest); when the run ends, all are `done`. The web lists the rest as pending in the order history, web, tmdb, write (`web/src/lib/recommendations/steps.ts` has the labels). A step starts with its first tool: `history` (the profile, `check_seen`), `web` (web search and fetch), `tmdb` (`get_candidates`, `tmdb_search`, `tmdb_details`), `write` (submitting shelves). A run reaches `history` as it starts (it builds the profile) and `tmdb` while it builds the candidate pool, before the model's first call, so its steps usually read history, tmdb, web, write. `detail` is the step's running tally: "2,184 plays · 312 ratings", "3 searches · 1 page", "148 candidates · 23 already seen", "2 of 5 shelves". `shelves_done` counts the shelves written (one `submit_shelf` call each; resubmitting a shelf replaces it), `shelves_target` the ones asked for (5, or more when the run writes a sixth).

**Picks**
- **`GET /recommendations/home`** → `200 RecommendationsHome`, always: without a key it's `{configured: false, enabled: false, run: null, active_run: null, hero_picks: [], shelves: []}`, and with `rec_enabled` off the same with `configured: true`.
  - `run` is the newest succeeded run; `shelves` and `hero_picks` are its. Before the first one (`run: null`) both are empty and the web keeps its own trending fallback (#135): the server has no trending shelves.
  - `active_run` is the queued or running run. Poll `/home` every 3 s while it's set (the refreshing banner); the old picks stay until the new run succeeds.
  - **Feedback** is applied at read time: a title with `dislike`, `not_interested` or `seen` is left out of every shelf and the hero picks; `like` shows as `feedback: 'like'`.
  - **Hero picks:** three a server-local day, the same all day. A run writes 3–9 in order; on the n-th day since it finished (0 on its own day) `/home` takes them from position `3n mod count`, wrapping, skipping negative feedback, so a weekly run shows new picks each day. Fewer than three only when fewer are left; none means For You's own deterministic picks (#135).
  - Cost: the newest run, its shelves and items, the hero picks, then one batch each for feedback, library state and the watchlist.
- **`POST /recommendations/refresh`** → `202 {run_id}`: a `manual` run, or the active one (queued or running), joined. Checked in this order: `503 unavailable` without a key; `409 recs_disabled` while `rec_enabled` is off; the active run (`202`); `429 budget_exceeded` with `resets_at` (the 1st of next month) once the budget is spent; `409 no_history` without Trakt connected or with fewer than 5 loved titles (R1's `counts.loved`, which counts completed titles too, but never one rated 4 or less or disliked; `profile.Builder.CheckHistory`).
- **Runs are queued** by the schedule (`scheduled`: `rec_schedule`, 03:00 server-local, weekly on Sunday), by refresh (`manual`), and by the end of Trakt's initial import when there's no succeeded run yet (`first_sync`; For You's "Writing your shelves" step). One at a time. A scheduled or first-sync run that meets the budget or the history check is stored `skipped` with its `skip_reason`, never `failed`; none is queued with `rec_enabled` off or without a key. A missed slot is caught up: when the server was down at the last scheduled time and no run has been queued since (but one was before: a fresh install has missed nothing), one `scheduled` run starts shortly after startup (about a minute, so startup isn't slowed). The first-sync run is queued before the import reports `idle`, so For You finds it as `active_run`.
- **What's never picked:** watched titles (a movie with a play; a show you've started: a Trakt progress row or a play), titles you rated on Trakt (the title's own rating, whatever `use_ratings` says: you've seen it), titles with negative feedback, and shows hidden or dropped on Trakt (`trakt_hidden`). R1 leaves them out of the candidate pool and R2 drops them when it validates a run's items, both through one rule (`candidates.Exclusion` on `library.TitleState`; `TasteRepository.TitleStates` looks titles up in one query). Library titles can be picked: a card shows their state ("Ready").
- **`GET /recommendations/runs?limit=`** → `{items: [RunSummary]}`, newest first (`id DESC`); `limit` 1–50, default 10, anything else `400 bad_request`.
- **`GET /recommendations/runs/{id}`** → `RunDetail`; `404 not_found` for an unknown run. Poll every 2 s while it's queued or running.

**Feedback**
- **`POST /recommendations/feedback`** with `{media_type, tmdb_id, signal: like|dislike|not_interested|seen, source: shelf|hero|ask|title}` → `204`, an upsert: the newest signal wins (`created_at` kept, `updated_at` set). Any other body is `400 bad_request`.
- **`DELETE /recommendations/feedback/{media_type}/{tmdb_id}`** → `204`, idempotent; a bad media type or id is `400`.
- **Dev only: `GET /recommendations/debug/profile`** → `{trakt_connected, enough_history, profile_bytes, profile, pool, pool_error}`: R1's profile and candidate pool built now, stored nowhere. Registered only with `RECS_DEBUG` (§4); otherwise `404`.
- **Effects.** `dislike`, `not_interested` and `seen` drop the title from every AI surface (shelves, hero picks, Ask's thread reads, the title page's `recommendation_reason`) and from R1's candidate pools, so R2 and R3 never pick it again. `seen` also counts in the profile as a completed, neutral title. `like` is a positive signal (the profile's loved).
- **Saved at once; Undo restores the previous signal.** The card dims and a 10-second Undo toast shows (HANDOFF §4.4); Undo POSTs the previous signal, or DELETEs when there was none (the web keeps it from its cache). Thumbs up toggles (a second tap DELETEs) and never dims.

**Ask momoshtrem**
- **`GET /recommendations/threads`** → `{items: [AskThreadSummary]}`, the newest `updated_at` first, at most 50: `{id, title, created_at, updated_at, turn_count}`.
- **`GET /recommendations/threads/{id}`** → `AskThread`; `404 not_found` for an unknown one:
  ```
  AskThread = AskThreadSummary + { model, turns: [AskTurn] }   // model: the thread's, kept for its life
  AskTurn { id, message, status: streaming|done|error|cancelled,
            steps: [AskStep], intro, items: [AskItem], refinements: [string],   // the turn's view (ask_turns.view)
            error: {code, message} | null,       // while error
            elapsed_ms | null,                   // null while streaming
            created_at }
  AskStep { step: history|rerank|web|tmdb|write, state: active|done, detail | null }
  ```
  A turn's view is written when it ends: a streaming turn has only its message; a cancelled or failed one keeps what it got to. An ended turn has no `active` step: the step still active when it failed or was cancelled is dropped from its view (it never finished, and an old failed turn must never show a spinner), as a finished run has none either. Library, watchlist and feedback are joined at read time, and items with negative feedback are left out, as on `/home`.
- **`POST /recommendations/ask`** with `{thread_id?, message}`: `message` is 1–500 characters once trimmed, counted in code points (Go's runes; the web counts with `Array.from`, so an emoji is one). Without `thread_id` a thread starts, titled by this rule, in code points: the trimmed message with its first character upper-cased (Go's `unicode.ToUpper` on the first rune); if it's over 60 characters, take its first 60, cut them back to their last space when that space's index (0-based) is over 30 (otherwise keep all 60: a hard cut), trim trailing whitespace, and append `…`.
  - **JSON errors before the stream**, checked in this order: `400 bad_request` (the body); `503 unavailable` (no key); `404 not_found` (unknown thread); `409 thread_full` (20 turns); `409 ask_busy` (a turn is streaming, in any thread: one at a time); `429 ask_limit` (30 turns started this server-local day, cancelled and failed ones included) and `429 budget_exceeded`, both with `resets_at` (the next local midnight; the 1st of next month).
  - **Then `200 text/event-stream`** (`Cache-Control: no-cache, no-transform`, `X-Accel-Buffering: no`), headers flushed at once:

    | Event | Data |
    |---|---|
    | `start` | `{thread_id, turn_id, title}`, first |
    | `status` | `AskStep`: a step starts (`active`, detail null) or ends (`done`, with its detail: "2,184 plays · you loved Severance, The Bear, Ted Lasso", "14 new since Oct 1", "23 candidates · 6 already seen", "5 picks"). Each step at most once a turn, at most one `active`: a step starts with its first tool (`history`: the profile, `check_seen`; `web`: web search and fetch; `tmdb`: `get_candidates`, `tmdb_search`, `tmdb_details`), and `write` with `show_picks` or an answer's text. A custom tool's step starts when it runs, after the tools before it in its response have answered, so the step it ends has its whole detail (the first custom tool of a response starts its step as its block starts); a server tool's starts with its block. `write` is last: once it starts no other step does, and its detail ("5 picks") is the turn's when it ends. A refinement (a follow-up that is one of the chips the thread's newest done turn offered, compared case-insensitively) uses `rerank` ("23 checked") for every tool instead of `history`, `web` and `tmdb`; a typed follow-up runs the usual steps |
    | `text` | `{delta}`: the turn's intro, one paragraph: `show_picks`' `intro`, sent whole as the call runs, before its items (on Opus and Sonnet 5.5 text written before a tool call comes back as a thinking block, so the intro can't be text). An answer without picks is the final response's text, sent when that response ends; text before a tool call is never shown, nor the closing line after `show_picks` |
    | `item` | `AskItem`, as each title resolves to a TMDB id (≤ 10 a turn) |
    | `error` | `{code: refused|limit_reached|upstream|internal, message}`; it ends the stream. `refused`: Claude declined, fallbacks included; `limit_reached`: the turn hit its cap on API calls, tool calls or cost (there's no token cap, R3 "Limits"); `upstream`: the Anthropic API failed after retries |
    | `done` | `{thread_id, turn_id, elapsed_ms, refinements, usage: {input_tokens, output_tokens, web_searches, cost_usd}}`, last. `refinements`: at most 5 chips ("Lighter", "Only anime") the web sends back as follow-up messages. `usage.input_tokens` is every prompt token, cache reads and writes included |

    The usual order is `start`, `status…`, `text…`, `item…`, `done`. A `: ping` comment every 15 s keeps proxies from timing an idle stream out.
  - **A disconnect** (Stop, a closed tab) cancels generation: the turn is `cancelled` and the usage already spent is recorded. The turn frees its slot as soon as its context is cancelled, before the usage is written, so a refinement sent right after Stop doesn't meet `ask_busy`; the web still retries once (after a short pause) on an `ask_busy` that follows its own Stop.
  - The web closes the stream as soon as it reads `done` or `error`: the turn is ended (its transaction committed) before either is written, so a disconnect after them changes nothing.
  - **Shutdown** cancels every turn in flight the same way: the REST server's `RegisterOnShutdown` hook calls Ask's `Close`, since `http.Server.Shutdown` doesn't cancel request contexts and an open stream would hold shutdown for its whole timeout.
  - **Threads are append-only.** A turn's API messages (`ask_messages`) are written in one transaction when it ends `done`; a cancelled or failed turn's never are, so it's never replayed and no orphaned `tool_use` is sent. The next turn replays the stored messages unchanged, on the thread's model.
  - The web proxy passes the stream through unbuffered (`backend-proxy.ts`); `askStream` (`web/src/lib/api/endpoints/recommendations.ts`) reads it, and a pre-stream error throws an `ApiError` with `resetsAt`.

**AI settings and usage**
- **`GET /settings/ai`** → `AiSettings`. **`PUT /settings/ai`** with any of its fields → `200 AiSettings` (all of them). One jsonb `||` statement on `app_settings` key `ai` (§5): fields left out keep their value and unknown ones are ignored; a bad value is `422 validation_error` with `error.field` naming it (the first, in the order below); a body that isn't a JSON object is `400`. Both work without a key.
  ```
  AiSettings {
    rec_enabled,                   // true: the master switch for picks
    rec_model,                     // claude-opus-5-5 (default) | claude-sonnet-5-5: picks and new Ask threads
    rec_schedule,                  // nightly (default) | weekly | manual: 03:00 server-local, weekly on Sunday
    monthly_budget_usd,            // ≥ 0, or null for no cap; 15 on a fresh install
    classifier_enabled,            // true
    classifier_model,              // claude-haiku-5-5 (default) | claude-sonnet-5-5 | claude-opus-5-5
    auto_apply_high_confidence     // true
  }
  ```
- **`GET /settings/ai/usage`** → `AiUsage`:
  ```
  AiUsage {
    configured,                                   // ANTHROPIC_API_KEY is set
    month, resets_at,                             // "2026-10", and the 1st of next month 00:00, server-local
    total_usd, budget_usd | null, budget_reached, // this month, every feature; budget_reached: total ≥ budget
    by_feature: [{feature: recs|ask|classifier, cost_usd, requests}],   // always all three, in this order; requests: API calls
    recs: { last_run: RunSummary | null,          // the newest finished run: succeeded, failed or skipped
            active_run: RunProgress | null,
            next_run_at | null },                 // the schedule's next run; null when manual, off or not configured
    estimates: { run_usd: {claude-opus-5-5: n | null, claude-sonnet-5-5: n | null},   // the mean of each model's last 5 succeeded runs
                 ask_usd: n | null },                                                // the mean of the last 20 done turns
    asks_today, ask_daily_limit                   // 30
  }
  ```
- **Title pages:** `recommendation_reason` (§6 "Titles and library") is the newest succeeded run's reason for the title (a shelf item's, else a hero pick's); null without one, while picks are off, and after negative feedback.
- **Dropped from earlier drafts:** `in_library` on rec items (replaced by `library`, as on similar titles); `backdrop_dominant_color` on rec items (HANDOFF §6 lists one: the hero loads the title, which has it, as Up next does); the thread's `messages` (replaced by `turns`, the saved views).

### Torrents and activity (B6)
Types: `web/src/lib/api/contract/torrents.ts`. Served by `internal/activity` (#98, #99), `internal/sessions` (#99) and `service.RemovalService` (#96).

- **Torrents stream on demand.** `AddTorrent` sets every piece to no priority; the prioritizer raises pieces only around open readers, and pieces live in the LRU file cache (`global_cache_size`, 4 GB by default), so progress can go down. A torrent loads on its first WebDAV open (or a mapping job), and nothing reloads at startup. So:
  - a torrent the library knows but the client hasn't loaded is `idle`, with `loaded: false` and `complete`, `progress`, `bytes_completed`, `peers`, `seeders` and `last_active_at` all null;
  - there's no ETA anywhere;
  - background downloading is #77's.
- **Hashes.** Responses carry 40-character lowercase hex. Every `/torrents/{hash}` route accepts hex in either case, or base32 (32 characters). Anything else is `400 bad_request`.
- **Known torrents.** A torrent is known when the client has it loaded, or an active `kind = 'torrent'` assignment uses it, or an open job that isn't a dry run has it. `pause`, `resume` and `DELETE` answer `404` for anything else.
- **`GET /torrents`** → `200 {items: TorrentListItem[]}`: every known torrent, and every loaded one nothing references (an Inspect dry run, a WebDAV listing) with `titles: []`. One query reads the library side (active torrent assignments and open non-dry-run jobs, with their titles), merged in memory with the client's loaded torrents.
  ```
  TorrentState = streaming | downloading | paused | idle     // the first rule that matches:
    paused       the user paused it (loaded or not)
    streaming    loaded, with a streaming session on one of its files (below)
    downloading  loaded, its network on (the activity manager hasn't idled it), and complete ≠ true
    idle         anything else
  TorrentListItem {
    info_hash, name, state, loaded,
    complete: bool | null, progress: number | null, bytes_completed: number | null,   // null when not loaded
    size_bytes: number | null,
    download_bps, upload_bps,          // smoothed bytes per second; 0 when not loaded
    peers: number | null, seeders: number | null,   // connected; null when not loaded
    added_at,                          // earliest active assignment or open job; the load time for one nothing references
    last_active_at: Timestamp | null,  // the last read, resume or load since the server started (the activity manager's, in memory); null when not loaded
    titles: [TorrentTitle],            // by title, then year
    open_job: {id, status} | null,     // the newest open, non-dry-run job
    streaming: [StreamingSession]      // its files' sessions, the most recently read first
  }
  TorrentTitle { media_type, tmdb_id, library_id, title, year, poster_path, scope }
  ```
  - **Order:** by state (streaming, downloading, paused, idle), then `last_active_at` newest first (nulls last), then `added_at` newest first.
  - **`name`:** the info dict's name when loaded, else the newest job's `torrent_name` (preferring a job that has seen the file list), else the stored metainfo's name (#99), else the magnet's `dn`, else the hash.
  - **`size_bytes`:** the info dict's total when loaded, else that job's `size_bytes`, else the sum of its stored proposals, else the stored metainfo's total (#99), else null. The stored metainfo is read only for a torrent that isn't loaded and lacks a job's name or size, and cached by hash (an info dict never changes).
  - **`scope`** names a show's episodes the torrent's active assignments fill; null for movies, and for a show only an open job links. Whole seasons are `"Season 1"` or `"Seasons 1–3"` (a run of whole seasons; all of season 0 is `"Specials"`); a run within a season is `"S01E29–E38"`, one episode `"S01E05"`; anything more is the first of those and how many other episodes, `"S01E01–E05 +2"`. A season is whole when every episode it has is there. Specials go last.
- **`GET /torrents/summary`** → `200`:
  ```
  { download_bps, upload_bps,          // the sum of every loaded torrent's smoothed speed
    peers,                             // connected, over the loaded torrents
    history: {interval_seconds: 2, download_bps: number[], upload_bps: number[]},   // the last 60 s of totals, oldest first; fewer right after startup
    counts: {streaming, downloading, paused, idle},
    disk: {total_bytes, free_bytes, torrents_bytes, cache_limit_bytes} | null,      // statfs(metadata_folder); the file cache's fill and capacity; null when unreadable
    streaming: [StreamingSession] }   // every session, HDRezka's too, the most recently read first: "Streaming now" reads streaming[0]
  ```
- **Streaming sessions** (#99, `internal/sessions`): who is streaming what over WebDAV, from every source.
  - **Requests.** A handler wrapper on the WebDAV server puts each request's method, User-Agent and remote address in its context. There are no proxy settings, so the address is the socket's host (no port: Infuse reads one file over several connections). `webdavFS.OpenFile` passes the context to `LibraryFS.OpenContext`; `Open(path)` is `OpenContext` without one.
  - **Reads.** A video a **GET** opens (a torrent's `TorrentFile` or an HDRezka `RemoteFile`) is wrapped so each read reports its offset and bytes. PROPFIND and HEAD opens (and opens outside a request) are never tracked, and a handle registers nothing until its first read, so listings cost nothing.
  - **A session** is the reads with the same remote address, User-Agent and WebDAV path: one playback's Range GETs (header, index, playback) are one session. It ends 30 s after its last read; a read in flight (a torrent waiting for pieces) keeps it going. Reads after it ended start a new one.
  - **Sniffs** never show: an open that read under 64 KB in all and closed within 2 s. A session shows once one of its opens isn't a sniff (64 KB read, or open for 2 s with a read); a session of only sniffs is dropped with its last open. A sniff that waits over 2 s for its first bytes (a torrent that isn't loaded yet) counts.
  - Expiry is lazy (every read, listing and count drops the sessions that ended), with an injected clock in tests.
  ```
  StreamingSession {
    id, source: torrent | hdrezka, info_hash | null,          // null for HDRezka
    item: {media_type, tmdb_id, library_id, title,            // the movie, or the episode's show (library_id: the movie's or the show's)
           season_number | null, episode_number | null, episode_name | null} | null,   // null when the library doesn't know the file
    file_name,                                                // the name WebDAV shows
    file_path | null,                                         // the path inside the torrent, as the detail's files[].path; null for HDRezka
    client: Infuse | VLC | Safari | Kodi | Unknown | null,    // from the User-Agent; null without one
    device | null,                                            // Apple TV, iPad, iPhone, Mac, Android, Windows, Linux when the User-Agent says so
    offset_bytes, size_bytes,                                 // offset: the end of the last read (ahead of what plays by the client's buffer)
    position_seconds | null, duration_seconds | null,         // offset ÷ size × runtime; null without a runtime
    rate_bps,                                                 // EWMA, 5 s time constant
    buffered_ahead_bytes | null, buffered_ahead_seconds | null,   // complete bytes contiguous from the offset, from the piece states; null for HDRezka
    started_at, last_read_at }
  ```
  - **The item** is the VFS entry's assignment (the WebDAV tree cache keeps each video's item id, cache v6), resolved in one query and kept 5 minutes. The runtime is the movie's or the episode's; a torrent file several episodes share ("S01E01-E02") plays for their summed runtime, unknown when one of them is.
  - **`client`**: a User-Agent starting `Infuse` (`Infuse-Direct/7.7`, `Infuse-Library/…`, the CFNetwork default) is Infuse; one with `VLC` is VLC; `Kodi/` or `XBMC/` is Kodi; Safari's `Safari/` without Chrome, Edge, Firefox, Opera or Android is Safari. Infuse's own User-Agent usually names no device, so `device` is often null for it.
  - **Where they show:** each torrent's sessions on its list item (and they make a loaded torrent `streaming`), every session in the summary, `LibraryItem.streaming` (a session's item is that movie or show; an overlay on the one query) and `/status`'s `webdav.active_readers` (their count).
- **Speeds.** Every 2 s a sampler reads each loaded torrent's data bytes read and written (`BytesReadData`, `BytesWrittenData`) and smooths each torrent's rate with an EWMA (5 s time constant). A torrent that's dropped falls out; one added again starts over. It keeps the last 30 totals for `history`.
- **`POST /torrents/{hash}/pause`** and **`/resume`** → `200 TorrentListItem`, idempotent; `400` for a malformed hash, `404` for an unknown one.
  - **Pause is sticky.** It's stored, so it survives restarts. The torrent's network goes off and reads don't wake it (the activity manager ignores them, and the VFS's wait for peers returns at once). On a torrent that isn't loaded it applies when it loads.
  - **Resume** clears it. A loaded torrent's network comes back on, and it counts as just accessed, so it stays on until `idle_timeout`. One that isn't loaded stays unloaded.
  - Pausing an already paused torrent keeps its first time; resuming one that isn't paused changes nothing.
- **`POST /torrents/pause-all`** → `200 {count}`: pauses the streaming and downloading torrents only. **`POST /torrents/resume-all`** → `200 {count}`: resumes every paused torrent, loaded or not.
- **Stored pauses.** `app_settings` key `torrents`, `{paused: {"<hash>": "<RFC 3339 time>"}}`. Every write is one atomic jsonb statement (an insert on conflict, `||`, `-` or `#-`), never a read-modify-write. They're loaded at startup, before anything loads a torrent, after the pauses of torrents the library doesn't know are pruned (see "Releasing" below). Without idle mode (`idle_enabled: false`) pauses still work: torrents stay active until paused.
- **`GET /torrents/{hash}`** (#99) → `200 TorrentDetail`, Activity's detail page; `400` for a malformed hash, `404` for an unknown one (the "known" rule above). It replaces the legacy status. **It never loads the torrent**: its facts come from the client when it's loaded, else from its stored metainfo (`{metadata_folder}/metainfo/<hash>.torrent`, read without loading), else from the newest job's file list (preferring one that isn't a dry run). The legacy `GET /torrents/{hash}/debug` is gone (#102): the detail's `pieces` and `streaming` replace it.
  ```
  TorrentDetail = TorrentListItem + {
    magnet_uri | null,                         // an assignment's (active first), else a job's; else, loaded, one built from the live torrent
    piece_length | null, pieces_total | null,  // live or stored metainfo; null with only a job's list
    pieces_have | null,                        // null when not loaded
    pieces: [{state: have | partial | priority | missing, count}] | null,   // run-length in piece order (anacrolix's piece-state runs); null when not loaded
    files: [{path, size_bytes,
             progress | null,                  // 0–1; null when not loaded
             plays_as: [{media_type, item_id, season_number | null, episode_number | null, episode_name | null, label}],   // label "S01E29" | "Movie"
             ignored: {reason, label} | null}],
    trackers: [{url, status: working | error | unknown, error | null, seeders | null, peers | null, last_announce_at | null}],
    downloaded_bytes | null, uploaded_bytes | null, ratio | null,   // since it loaded (useful data in, data out); null when not loaded; ratio null before anything arrived
    swarm: {connected_peers, connected_seeders, total_seeders | null} | null   // null when not loaded; total_seeders needs a scrape: null
  }
  ```
  - **Piece states**, first match: `have` (complete), `partial` (some of it arrived), `priority` (not complete, priority High or more: the streaming windows), `missing`. Adjacent runs in the same state merge.
  - **Files** are in the torrent's order, with paths as the client names them (`{name}/…` in a multi-file torrent). `plays_as` comes from the active assignments (`ActiveMappings`): a movie (`item_id` is the movie's), or its episodes in order (`item_id` is the episode's). `ignored` is for a file with no `plays_as` that the newest job's proposal ignored: `reason` is the extra's `extra_type` (labels "Ignored (NCOP)" for `opening`, "Ignored (NCED)" for `ending`, "Ignored (not video)" for `info` and `image`, "Ignored (older version)" for `superseded`, "Ignored (extra)" for `other`, else "Ignored (sample)" and the like), `subtitle` for a subtitle no video claims, or `user` ("Ignored") for a video the user or a movie pick left out. A subtitle that follows its video isn't ignored.
  - **Trackers** are the live torrent's announce list, else the stored metainfo's, else the magnet's `tr`, each once. anacrolix keeps its announce results private, so `status` is `unknown` and the rest null; nothing scrapes.
- **Static routes and hashes.** `summary`, `pause-all` and `resume-all` sit beside `/torrents/{hash}`; they're matched first, and `/torrents/pause-all/pause` is a malformed hash (`400`).
- **The web app** (#102) has no routes of its own under `/api/torrents`: the session-checked proxy forwards every `/api/torrents*` path to the backend. Activity (`/activity`) reads the list and the summary, the detail page (`/activity/{hash}`) reads `GET /torrents/{hash}`, and the shell's Activity badge counts `counts.streaming + counts.downloading`. A torrent `DELETE` the proxy gives up on (`504` after 30 s) may still finish on the backend, so the page keeps the torrent hidden and refetches instead of bringing it back.
- **`DELETE /torrents/{hash}?delete_data=true|false`** → `204` (#96). `delete_data` defaults to `false`; any other value is `400 bad_request`.
  - It answers `404` only when nothing knows the hash: the torrent isn't loaded, no active torrent assignment uses it, and no open job has it (dry runs don't count).
  - One transaction cancels its open jobs (dry runs too), deactivates its active assignments, deletes its `source = 'torrent'` subtitles and clears its stored pause. The rows stay, inactive, so history is kept. The client's pause is cleared once it's dropped (#98).
  - The torrent is dropped from the client. It first waits for an add of the torrent in flight, at most `add_timeout`, so the answer can come late. Readers still open on its files fail, and its stored metainfo is deleted.
  - With `delete_data=true`, every piece is marked not complete, which deletes its file from the cache. The pieces come from the live info dict, else the stored metainfo. Partial pieces are left to the cache's LRU.
  - When the torrent is neither loaded nor stored, the metainfo is fetched again from its newest magnet in the background, within `add_timeout`. Its data is then deleted, unless something uses the torrent again by then. Without peers, a log line says the data is left to the LRU.
  - Then the WebDAV tree is rebuilt (`InvalidateTree`).
- **Lifecycle** (`internal/torrent`).
  - Adds, metadata waits, drops and removals of one hash are serialised in the torrent service. Concurrent adds (a job, a WebDAV read) share one attempt, so a timeout in one can't drop a torrent another is adding.
  - When metadata arrives, the metainfo is stored in `{metadata_folder}/metainfo/<hash>.torrent`. An add loads it first, with no wait for peers.
- **Releasing** a torrent (discard, title delete, unassign) drops it when `TorrentInUse` finds no active assignment and no open job using it. A release that drops it also clears its pause, stored and in the client, so it isn't paused when it's assigned again (title deletes and unassigns, in `RemovalService`). Releases elsewhere (a job discard) leave the stored pause to startup: before the pauses are restored, those of torrents the library doesn't know (no active torrent assignment, no open non-dry-run job) are pruned.
- **BitTorrent port:** `torrent.listen_port` (`TORRENT_LISTEN_PORT`), 42069 by default; 0 picks a free port, so two instances can run side by side.

### System
- `GET /status[?refresh=1]` (G2) returns:
  ```
  {
    version, uptime_seconds, checked_at,          // checked_at: RFC 3339, when the probes ran
    db: { ok, size_bytes, error? },
    webdav: { url, port, auth_enabled, username?, active_readers? },  // username only when auth is on (active_readers: the streaming sessions, #99)
    jackett: { configured, reachable, latency_ms?, error?,
               indexers?: [{name, status: ok|timeout|error|unknown, ms: number|null, since: Timestamp|null, error?}] },   // #6
    tmdb: { configured, reachable, latency_ms?, error? },
    trakt: { configured, connected },             // configured: Trakt's client id and secret and a valid SECRET_KEY; connected: state is connected (B5)
    ai: { configured, mtd_cost_usd },             // mtd_cost_usd: every AI feature's spend this month (ai_usage); null when it can't be read
    opensubtitles: { configured },
    hdrezka: { configured, reachable, latency_ms?, error? }   // #63
  }
  ```
  - Probes run concurrently with a 3 s timeout each. Results are cached for 30 s, and concurrent callers share one check. `?refresh=1` bypasses the cache at most once every 5 s.
  - It returns 200 even when a probe fails (the body says what's wrong) and 503 only when the database ping fails. The response never contains secrets.
  - **`jackett.indexers`** (#6) is the per-indexer health:
    - the probe asks `t=indexers&configured=true` instead of `t=caps`, which checks the key and lists the indexers in one request;
    - each indexer is merged with its latest search outcome (in memory since startup) and is `unknown` before any search;
    - `since` is when the current timeout or error streak began ("Timing out for 2 h");
    - `indexers` is omitted when Jackett isn't configured or isn't reachable;
    - there's no Jackett version and no "Open Jackett" link (Jackett is loopback-only).
  - The web app's own `GET /api/status` (F1) adds `web: {version}`. A backend 503 (database down) is passed through as 200 with the same body, so Settings can show which service is down. Only a backend that can't be reached becomes `503 {error: {code: "backend_unreachable"}, web, webdav: {reachable}}`, probing WebDAV directly (`WEBDAV_INTERNAL_URL`, default: the API host on port 36911) so the offline page can say whether Infuse still plays.
- `GET /torrents` (B6) returns `{items}`, `GET /torrents/summary` the totals: see "Torrents and activity (B6)".

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

**G4 · HTTPS** (infra; needs the user for router and port changes). Done in #19.
- Caddy gets a Let's Encrypt certificate for the DuckDNS name over TLS-ALPN on 443, so the stock image does it and port 80 stays closed. The first plan was DNS-01 through a custom Caddy build. The user chose TLS-ALPN in #19, which needs no custom image and no `DUCKDNS_TOKEN`. This replaces `tls internal`.
- Serve web through Caddy on 443.
- Stop publishing `3000:3000` (web is on the VM's loopback only), and default to `SECURE_COOKIES=true` and `TRUSTED_PROXY_HOPS=1`.
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
  - The AI never overrides a `high` rules result; when it disagrees with one, the proposal goes to review with the AI's pick in its reason (`identify.FlagDisagreements`).
  - AI-only mappings go to `needs_review` unless they're `high` confidence and the setting allows auto-apply.
- **Caching and cost.** Cache complete results in `ai_cache` by `(info_hash, show_id, metadata_updated_at, hint, scope)`, plus the model and prompt version; a cached answer is served even with the budget spent. Record usage in `ai_usage`. Skip AI calls once `monthly_budget_usd` is spent. If `ANTHROPIC_API_KEY` is missing, degrade quietly to rules-only. Answers are mapped to files by their number in the request (`n`), the file name checking it.
- **`cmd/classifier-eval`.** Runs at least 20 golden torrent file lists:
  - One Piece batch, Frieren (including the scene "S02" split), Doctor Who with specials, Bluey.
  - Law & Order: SVU, Trailer Park Boys.
  - A daily show, a complete-series pack, an OVA collection.

  It reports accuracy, cost and latency. Run it manually, since it costs money.
- Acceptance: at least 95% file-level accuracy on the golden set, with average cost per torrent reported in the PR.

**B4 · Torrent search service** (independent; `inspect` uses B2 jobs, so stub it until B2 merges)
- Delivered by #6's slices (#86–#91). §6 "Search" is the contract; where the bullets below differ, §6 wins.
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
- Owns `internal/trakt`, `internal/me`, migration 005 and `internal/api/trakt_routes.go`, `me_routes.go`. #8 splits it: #130 the contract (§5, §6, the web mocks), #131 connect, #132 sync, #133 `/me` and watch state.
- **Trakt API facts**, checked against developer.trakt.tv on 2026-10-16:
  - OAuth is on `https://auth.trakt.tv` (`POST /oauth/device/code`, then `POST /oauth/device/token` every `interval` seconds: 400 still pending, 404 invalid, 409 already used, 410 expired, 418 denied, 429 slow down), the API on `https://api.trakt.tv`.
  - Access tokens last about 7 days. Schedule the refresh from `created_at + expires_in`, never from a hard-coded lifetime.
  - Refresh tokens are **single-use and rotate**: store both new tokens before using either.
  - After a 401, stop using that token: refresh once, and a second 401 means `reauth`.
  - Every request needs a `User-Agent` (`momoshtrem/<version>`), plus `trakt-api-version: 2`, `trakt-api-key` and the Bearer token.
  - GETs are limited to 500 per 5 minutes. Honour `Retry-After` on 429 (Cloudflare's 429s carry none: back off with jitter).
  - `limit` is at most 250; always paginate (`page`, `limit`, the `X-Pagination-*` headers), even where a list looks short (the watchlist answers 10 items without them).
  - Show progress comes from the bulk `GET /sync/progress/watched`, never one request per show. Trakt doesn't filter dropped or hidden shows out of it: read `/users/hidden/dropped` and `/users/hidden/progress_watched` and filter them ourselves.
  - Free accounts can connect at most 2 apps, so a denied code may be that limit.
- **Device-code OAuth** (#131): tokens encrypted with `SECRET_KEY` (AES-256-GCM); a key that can't decrypt them means `reauth`, never a crash. Disconnect revokes (`POST /oauth/revoke`, best effort) and purges.
- **Sync** (#132):
  - Every 30 minutes, `GET /sync/last_activities`. When something changed, only the deltas: history (`/sync/history/{movies,episodes}?start_at=`), ratings, watchlist, hidden lists, progress and the calendar (`/calendars/my/shows`).
  - A nightly reconcile, and the initial import (the same code path, with progress), replace everything in transactions, which also catches deleted or backdated plays. History deltas miss backdated plays until then.
  - What a changed `last_activities` field fetches: `movies.watched_at` and `episodes.watched_at` that history from an hour before the earlier of the stored activity and the last stored play (upserted by event id), episodes also progress and the calendar; `*.rated_at` that rating type in full; `watchlist.updated_at` and `{movies,shows}.watchlisted_at` the watchlist; `shows.hidden_at`, `shows.dropped_at` and `seasons.hidden_at` the hidden lists and progress; a calendar over 12 hours old the calendar. Any change runs `details`. The snapshot is stored only after every fetch succeeded, so a failed incremental sync is redone from the old one.
  - Every write is one transaction that holds `trakt_auth`'s row (`FOR SHARE`) and checks `connected_at`; the purge locks that row first (`FOR UPDATE`), so a write in flight either finishes before the purge deletes it or is refused after.
- **ID mapping.** Map via `ids.tmdb` and keep the Trakt ids too; skip movies and shows without a TMDB id. Artwork for titles outside the library comes through `tmdb_cache` (the `details` step).
- **Up next, new episodes and watch state** (#133): the rules in §6 "Trakt (B5)" and "Titles and library (B1)".
- **Optional input for R1:** Trakt `/recommendations/{movies,shows}`.
- Acceptance:
  - Tests use a fake Trakt server.
  - Connect and disconnect work end to end.
  - Incremental sync fetches only deltas: an unchanged `last_activities` is one request.
  - Up next is correct for a show with specials and for a show whose next episode hasn't aired.
- **Landed in #8:** #130 (the contract, migration 005, the web mocks), #131 + #140 (connect; the client secret is optional), #132 (sync), #133 (`/me`, watch state), #147 (the legacy `/shows/recently-aired` and `/shows/sync-air-dates` are gone; the air date sync stays).
- **Live check** (2026-10-16, a real Trakt API app and account on a scratch stack):
  - A new API app gets a Client ID and no secret; the device flow, refresh and revoke work without one (#140). `verification_url` is `https://auth.trakt.tv/activate`.
  - The access token Trakt issued lasts 7 days.
  - The initial import (1,865 plays, 1 rating, 42 watchlist titles, 69 progress rows before the fix below) took about 22 s; a reconcile after it (45 progress rows) 8 s; an incremental sync with nothing changed 0.25–0.4 s (one request).
  - Trakt's bulk progress also returns watchlisted shows never started (`completed: 0`); without the filter in §6 they made 24 of 30 Up next cards.

**B6 · Torrent and activity hygiene** (independent)
- Owns `internal/torrent` and `internal/api/torrent_routes.go`.
- Compute real download and upload speeds from the delta of `Stats()` byte counts over time.
- `/torrents` lists torrents known from the DB, merged with live state (`idle` when not loaded), and links each one to its titles.
- Route pause and resume through the ActivityManager.
- Honour `delete_data`.
- Delete orphaned subtitles when items or torrents are removed.
- Acceptance: tests for the speed calculation and the merged listing. The Activity view is never empty while the Library has assigned items.
- **Landed in #96 (B6a):**
  - The torrent and title deletes and the unassign cleanup (§6 "Torrents and activity (B6)" and "Titles and library"), run by `service.RemovalService`.
  - The per-hash lifecycle and stored metainfo in `internal/torrent`.
  - `torrent.NormalizeInfoHash`.
  - The sweep of SRT files the assignment writer orphans is #82's.
- **Landed in #98 (B6b):**
  - `internal/activity`: `GET /torrents` merging the library's torrents with the client's, `GET /torrents/summary`, pause, resume, pause all and resume all (§6 "Torrents and activity (B6)"). Routes in `internal/api/torrent_routes.go`.
  - The speed sampler (`torrent.SpeedSampler`), real speeds in the torrent status and the title pages' `download.speed_bps`.
  - Sticky pauses in the activity manager (`StatePaused`, `ErrPaused`), stored in `app_settings` key `torrents`; `torrent.state: paused` on title pages.
  - `torrent.listen_port` / `TORRENT_LISTEN_PORT`.
- **Landed in #99 (B6c):**
  - `internal/sessions`: the streaming sessions (§6 "Torrents and activity (B6)"), from a WebDAV handler wrapper that puts the request in the context, `LibraryFS.OpenContext` and a read-reporting wrapper on every video a GET opens (torrent and HDRezka). They feed the `streaming` state, `streaming` on the list and the summary, `LibraryItem.streaming` and `/status`'s `active_readers`.
  - `GET /torrents/{hash}`: the detail, from `torrent.Service.Detail` (live), `MetainfoStore.Summary` (stored) or `JobFiles` (the newest job); the list's name and size fall back to the stored metainfo.
  - Re-run mapping: the assign routes take `{info_hash}` (§6 "Assignment").
  - Next: the Activity page (#102).
- **Landed in #102 (F7b):** Activity and the torrent detail (`/activity`, `/activity/{hash}`) on the endpoints above; `/torrents` redirects there. The legacy web routes under `app/api/torrents/**`, the legacy page and the backend's `GET /torrents/{hash}/debug` (with `torrent.Service.GetDebugInfo`) are gone.

**B7 · Subtitles polish** (small; independent)
- Owns `internal/subtitle`, `internal/opensubtitles` and `007_subtitles.sql`.
- Serves the item-scoped endpoints (`/movies/{id}/subtitles…`, `/episodes/{id}/subtitles…`, §6 "Subtitles").
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

Epic #9, in slices: the contract, stateful mocks and skeletons (#152); then R1 (#153), the AI core (#154), For You's picks (#155) and the Ask page (#156) in parallel; then R2 (#157), R3 (#158) and Settings › AI & picks (#159); then a live run, docs and close-out (#160). §6 "Recommendations (R1–R3)" is the contract. Each backend slice fills in its own `cmd/momoshtrem/wire_*.go` and route file; `router.go` and `main.go` already call them.

**AI core** (#154; the shared base of R2 and R3)
- Owns the Claude client, the tool loop, pricing, the budget, `/settings/ai` and its usage (`internal/api/ai_settings_routes.go`'s `AISettings`), and `cmd/momoshtrem/wire_ai.go`.
- **Where:** `internal/aicore` (the loop `Client.Run`, prices, usage, errors, the `Ledger` that records `ai_usage` and checks the budget, `MonthBounds`/`DayBounds`), `internal/aicore/fake` (a scripted Anthropic SSE server that records requests, for every package's tests), `internal/aisettings` (the `/settings/ai` service; the picks and Ask add their parts of the usage page with `SetRecs` and `SetAsk`). `recommend/agent.Tool` is `aicore.Tool`. The classifier (`identify/ai`) uses the core's prices, errors and ledger.
- **Our own streaming tool loop**, not the SDK's tool runner: in anthropic-sdk-go v1.81 the runner rewrites `params.Tools`, which drops `strict`, `cache_control` and server tools, and it runs tools in parallel. The loop keeps the tool definitions byte-stable, runs custom tools (`internal/recommend/agent.Tool`) one at a time in order, returns all of a turn's `tool_result`s in one user message, continues on `pause_turn` (server tools, at most 5 times), and stops on `end_turn`, `refusal`, `max_tokens` or a cap (calls, tokens, cost, tool calls; checked after each call, once its tools have run; `Request.AllowFinal` can let one more call past a cap end the turn, without tools: Ask, once its answer is shown, #158). Hooks (block start, text delta, tool start, tool result, each call's usage) run in order on the loop's goroutine. A cancelled context stops it at once and records the usage the cut-off call reported. It returns the messages it appended as raw blocks, for append-only replay; after a refusal fallback, the blocks before the handoff the next model can't take are dropped, as the API asks.
- **Requests.** Adaptive thinking with effort set explicitly (`medium`; Opus 5.5's default is `medium`, Sonnet 5.5's `high`, and neither can turn thinking off). Server-side refusal fallbacks: `fallbacks: "default"` with beta `server-side-fallback-2026-07-01`. Forced `tool_choice` (`any` or `tool`) is a 400 on Opus 5.5 and Sonnet 5.5, so tools are `auto`, `strict` and steered by the prompt. Prompt caching follows the render order tools → system → messages: the tools and system prompt never change between calls; the date and profile go in the user turn. `cache_control` sits on the system block, plus top-level automatic caching. Ask, whose history comes back from the database, can set `thinking.block_binding.prefix_mismatch_behavior: "drop_block"` (beta `thinking-binding-controls-2026-08-01`). Tool inputs are small, so `eager_input_streaming` stays off and `strict` keeps them schema-valid.
- **Pricing.** One table per model (input, output, cache reads and writes) plus web searches, billed per search ($10 per 1,000; web fetch is free). A fallback hop bills at the fallback model's rates (Opus 5.5 → `claude-opus-5` or `claude-opus-4-8`; Sonnet 5.5 → `claude-sonnet-5`): cost comes from `usage.iterations`, per attempt, a declined attempt included (a pre-output decline is billed only in some categories; the budget errs on the safe side). A model without a price is priced at the requested model's card, with a warning, never at $0. Every API call is one `ai_usage` row, under the requested model, its tokens summed over its attempts. Checked against the pricing page on 2026-10-16: Opus 5.5 and Sonnet 5.5 read the cache at 0.05x input ($0.20 and $0.10), the others at 0.1x.
- **Budget.** `monthly_budget_usd` ($15 by default, `null` no cap), checked before each run, turn and classifier run; the month is server-local (`TZ`), and the classifier's budget check and `/status` move to it too.

**R1 · Taste profile, candidate pool and feedback** (#153; deterministic, no LLM)
- Owns `internal/recommend/profile`, `internal/recommend/candidates`, migration 006, the agent's custom tools (each an `agent.Tool`), and feedback: `api.RecFeedback` (`SetFeedback`, `ClearFeedback`), set as `Options.RecFeedback` in `wire_recs.go` with or without a key, so it lands before the picks.
- **Taste profile.** Weighted by recency, rating (only with Trakt's `use_ratings` on, `app_settings` key `trakt`) and completion. It covers:
  - Top genres, keywords, creators and cast.
  - Decades, languages and origin countries.
  - Anime share, the movie/show ratio and runtime tolerance.
  - **Loved:** rated 8 or higher, completed, or `like`d.
  - **Dropped:** started, under 30% watched, and idle for 60 days (or dropped on Trakt).
  - **Recent:** the last 30 days.
  - **Aversions:** low ratings, `dislike` and `not_interested`. `seen` counts as a completed, neutral title.

  Stored in `taste_profiles` as compact JSON, targeting 4k tokens or fewer, with `counts` for the `no_history` check (at least 5 loved or completed titles, and Trakt connected).
- **Candidate pool.** Sources:
  - TMDB recommendations and similar titles for the top loved titles.
  - TMDB discover by top keywords and genres, within recency windows.
  - Trending, upcoming and on the air.

  Trakt's `/recommendations` is dropped as a source: its picks come from the same history the profile reads, it adds a Trakt call to every run, and it overlaps TMDB's recommendations for the loved titles while explaining nothing. Exclude anything watched (a movie with a play; a show you started, by a Trakt progress row or a play), anything with negative feedback, and shows hidden or dropped on Trakt (`trakt_hidden`); library titles stay ("already in your library", or "Ready"). Pre-rank by overlap and keep a pool of about 150 with compact metadata.
- **Tools:** `get_taste_profile`, `get_candidates(filters)`, `tmdb_search(query, type, year)`, `tmdb_details(type, id)`, `check_seen(tmdb_ids)`, all `strict`.
- **Schema budget.** Structured outputs allow at most **16 parameters with a union type** (`anyOf` or a type array) per request, counted across every strict tool; past it the request is a 400 ("Schema is too complex for compilation"). There are also at most 24 optional parameters and 20 strict tools. R1's tools, R2's `submit_shelf` and `submit_hero_picks` and R3's tools share one request, so the rule is **sentinels, not nullables**: every property is required, and "no value" is a sentinel (`"any"` or `"either"` in an enum, `[]`, `0`, `""`). R1's five tools use none (a test counts them, and allows at most 2).
- Acceptance: unit tests on fixtures, plus a dev-only `GET /recommendations/debug/profile`.
- **Landed in #153:**
  - Migration 006 (§5), tested on a database already at 009. Readers in `library/taste_reads.go` (`TasteRepository`: watched movies and shows, ratings (a show without its own takes the mean of its season and episode ratings), hidden shows, the watchlist, library titles, feedback with titles, `TitleStates`), `RecFeedbackRepository`, `TasteProfileRepository`.
  - TMDB lists in `tmdb/lists.go`: discover, trending, upcoming, on the air, search and the genre lists, with `Cached` wrappers (6–12 h; genres 7 days).
  - `internal/recommend/profile`: `Builder.CheckHistory` (no TMDB), `Build`, `Save` (a `taste_profiles` row), `Current(maxAge)`. Weights: recency halves each year (to 0.25), ratings ×0.7–2.0 (with `use_ratings`), a like ×1.5, shows by the share watched. Genres, keywords, people, languages, countries, anime share and runtimes come from TMDB's details of the 40 heaviest titles (the library's metadata without them).
  - `internal/recommend/candidates`: `Builder.Build(ctx, profile)` → `Pool` (≤ 150, best first, `Stats` for the step detail), `Pool.Filter`, `Exclusion`. Sources: the 10 most loved titles' recommendations and similar titles, discover (`kw`, `new`, `gems`, `lang`), trending, upcoming, on the air. TMDB describes the best 200 (runtime, seasons, keywords) before the final ranking.
  - `internal/recommend/tools`: `All(Deps)` (always in this order), `Fixed` (a run), `Cached` (Ask), `Encode`. The tools name titles by `media_type` and `tmdb_id` throughout: `tmdb_search(query, media_type, year)`, `tmdb_details(media_type, tmdb_id)`, `check_seen(titles: [{media_type, tmdb_id}])` (TMDB ids repeat across movies and shows). Sentinels for "no filter": `media_type: any`, `genres: []`, `0` for a runtime, year or limit, `language: ""`, `anime: only|exclude|either`, `tmdb_search`'s `year: 0`. A TMDB failure (its timeouts too) and a pool TMDB couldn't build (`ErrNoSources`) are tool errors; a database error or the run's context ending ends the run. Seeds are the 10 most loved titles but those only marked seen, labelled with their year (`rec:Dune (2021)`); a title only marked seen weighs at the recency floor. The TMDB cache moved to `tmdb:v2` (list items' `origin_country`, episodes' `runtime`).
  - `api.RecFeedback` is `recommend.FeedbackService`; `wire_recs.go` holds R1's builders for #157 and #158 (`recsWiring.profiles`, `pools`, `taste`, `tools(inputs)`).

**R2 · Recommendation runs and shelves** (#157; depends on R1 and the AI core)
- Owns the run (`internal/recommend/runs`: prompt, `submit_shelf`, `submit_hero_picks`), the scheduler (`internal/recommend/schedule`), `api.Recommendations` (`/home`, refresh, runs; set as `Options.Recommendations` in `wire_recs.go`, also without a key so `/home` answers `configured: false`), `recommendation_reason` on title pages, the `recommend-run` CLI, and the rest of `cmd/momoshtrem/wire_recs.go`.
- **Hooks are setters**, called from `wire_recs.go`: the Trakt sync's `SyncDeps` and the catalog are built in `main.go` before `wireRecs` runs, so R2 adds `traktSync.SetOnImported(fn)` (queue the `first_sync` run when the initial import finishes) and `catalogService.SetReasons(r)` (the title pages' `recommendation_reason`). The reasons interface lives in `internal/catalog`, since catalog can't import `recommend`.
- **The run.** The model from settings (`rec_model`, default `claude-opus-5-5`) on the AI core's loop, with R1's tools and the server-side `web_search_20260209` (`max_uses` 5 a call, 20 searches a run) and `web_fetch_20260209` (4 a call) for freshness: new releases, upcoming premieres, festival and awards buzz. `submit_shelf` (strict) writes one shelf per call, so `shelves_done` counts up; `submit_hero_picks` (strict) writes 3–9 hero picks. The run ends when the agent stops, or at a cap.
- **Output.** 4–6 shelves (`kind`: because, new, anime, gems, coming_soon, other; e.g. "Because you loved Severance", "New this month for you", "Anime you'll like", "Hidden gems", "Coming soon") of 6–15 items. Every item carries a one-line reason (≤ 140) grounded in the profile and must resolve to a TMDB id: the server validates each (it exists, isn't watched or `seen`, has no negative feedback, isn't a show hidden or dropped on Trakt), drops the rest, and fills `meta` from the TMDB cache; a shelf left under 6 items isn't saved (the tool result says why, and the model replaces them), and a run left under 4 shelves after its one nudge turn fails.
- **Progress** (`rec_runs.progress`) is written as the agent moves, with the steps and details in §6.
- **Guardrails.** Caps on tokens, tool calls and time per run; the budget and history checks before it (a scheduled or first-sync run is then `skipped`); usage per API call in `ai_usage` (`feature: recs`, `ref_id` the run), summed onto the run.
- **Scheduler.** `rec_schedule`: nightly at 03:00 server-local (the design's), weekly on Sunday at 03:00, or manual; `rec_enabled` off queues nothing. Single-flight through the partial unique index. Catch-up: if the server was down at the last slot and no run has been queued since (a fresh install has missed nothing), one run starts about a minute after startup. `first_sync` runs are queued from the `SetOnImported` hook when Trakt's initial import finishes and nothing has succeeded yet. Slot times come from `time.Local` (§6 "Time").
- **`recommend-run [--dry-run]`**: one run in the foreground, printing the shelves and the cost (#160's live check).
- Acceptance:
  - CI tests use a mocked API, with no network.
  - One real run is documented in the PR (#160), with its cost.
  - `/home` serves from the DB.
- **Landed in #157:**
  - `library/rec_runs_repo.go` (`RecRunRepository`): queue (`INSERT … ON CONFLICT DO NOTHING` on the one-active index, else the active run's id: refresh, slots and the first-sync trigger join it), start, progress, usage (`SyncRecRunUsage` sets the run's columns to its `ai_usage` rows' sums after every call), publish (the shelves, items and hero picks and `succeeded` in one transaction), fail, skip, fail-active at startup, and pruning to the newest 30 runs, never the newest succeeded one (its picks are `/home`'s). The reads: `/home`'s, the title reason (one query, null after negative feedback), the estimates.
  - `internal/recommend/runs`: `Runner.Execute` and the reads (`Reader`: `Home`, `Runs`, `Run`, `RecommendationReason`, `DayPicks`). Before a run starts: the request's `Validate`, `rec_enabled` (a queued run whose picks were switched off fails), the budget (`skipped: budget_exceeded`) and `CheckHistory` (`skipped: no_history`). The system prompt (`systemPrompt`, `PromptVersion`) and the 7 strict tools (R1's 5, `submit_shelf`, `submit_hero_picks`; goldens in `runs/testdata`) are byte-stable; the date, the profile and the pool's best 90 (`InlineCandidates`) go in the first user turn. Effort `medium`; `web_search` `max_uses` 5, `web_fetch` 4 (per API call, so one call can't take most of the run's searches). Caps: 30 calls, 3M tokens, $1.50, 80 tool calls, 20 web searches a run (`aicore.Caps.WebSearches`, counted from usage, since `max_uses` bounds one call only), 20 minutes (`runs.Caps`, `runs.Timeout`). The web step's tally comes from usage too (server-side filtering may search more than the blocks show).
  - Validation, as each tool call comes in (the result lists what was dropped and why, so the model resubmits the slug with replacements): each item names its `title` and `year` (0: unsure), checked against TMDB's (the title loosely, against the title or original title; the year within one): a wrong id, a remake, or "Dark" for "Dark Matter" is dropped; then `candidates.Exclusion` on `TitleStates` and the TMDB details through the cache, which fill `meta` (badges are the server's: a premiere still to come, or on a `coming_soon` shelf a show's next episode; "Jan 16", or the year past twelve months). A title sits on one shelf only; a shelf needs 6–15 that pass (the first 15 kept) or isn't saved; at most 6 shelves; 3–9 hero picks, a call replacing the last. Reasons over 140 characters and kickers over 40 are clipped. `signals` records the pool rank and sources.
  - A run that ends with fewer than 4 shelves or no hero picks gets one nudge turn on the same prefix; it fails with fewer than 4 shelves after it. Stopping at the web search cap counts as stopping short: the nudge (appended to the tool results the loop stopped on) says there are no more searches, and runs with `Caps.WebSearches: 1`, only the cap changed, never the tools or `max_uses`, so a search ends it after that call (its shelves still count). One stopped early (a cap, an API error, its own 20-minute deadline) with 4 or more publishes them; only shutdown fails it outright. A run's final write (succeeded, failed, skipped) is made past its deadline and retried (1 s, 4 s, 15 s), so it doesn't stay active (refreshing) until a restart.
  - `internal/recommend/schedule` (`Scheduler`): the 03:00 loop (re-planned on `Reschedule`, which `aisettings.Service.SetOnChange` calls after every `PUT /settings/ai`; waits at most an hour), the catch-up (dropped when the schedule turns manual or picks off), one worker, the reaping of a run left `running` that this process isn't executing once it's older than a run may be (20 minutes and a 10-minute margin, so `recommend-run` in another process is left alone; a queued one wakes the worker), `Refresh` in the contract's order, `FirstSync`, `NextRunAt`; it serves `api.Recommendations`, `aisettings.RecsReporter` and `catalog.Reasons`. `trakt.Syncer.SetOnImported` calls `FirstSync` after an initial import stored everything, before the sync reports it idle.
  - `aicore.Client.CountTokens` and `aicore.RequestBody` for `recommend-run -dry-run`: it builds the first request (profile and pool built, not stored) and counts it on the free `count_tokens`, or reports its size without a key. The real run queues a `manual` run (refused while another is active, or while picks are off) and prints the shelves, the hero picks and the usage. A demo account (16 loved titles, the full pool) measured 23,525 input tokens for the first call: $0.12 written to the cache on Opus 5.5, $0.005 a later call reading it.
- **Live check** (#160, 2026-10-16: the real API, a real Trakt account with 1,872 plays and 145 loved titles, a scratch stack on a non-UTC `TZ`):
  - **First sync.** The initial import took 18.5 s. The `first_sync` run was queued about 9 ms before the import reported idle, and started at once.
  - **Opus 5.5: 80 s, 4 API calls, $0.32.**
    - 12 s of the 80 built the profile and the pool on a cold TMDB cache.
    - The first call wrote the 24,215-token prefix to the cache ($0.12, exactly what `-dry-run` counted on `count_tokens`).
    - The next three calls read 24k, 29k and 35k from the cache, 83–98% of each prompt.
    - Output was 6.4k tokens. All five shelves came in one call: 5.2k tokens over 44 s.
    - It wrote 5 shelves of 7–11 titles (46 titles) and 7 hero picks.
  - **Sonnet 5.5 (Run now, TMDB warm): 36 s, 4 calls, $0.14.** It wrote 5 shelves and 5 hero picks. Its reasons are shorter and more generic.
  - **No web search in either run.** The "new" shelf came from the pool's on-the-air and trending titles, and neither run wrote a coming-soon shelf.
  - **Quality.**
    - The reasons cite what the profile holds: the one 10/10 rating ("the closest match to your 10/10 for The Courier"), loved titles, runtimes, book adaptations.
    - Every title is real. None was watched or rated (checked in SQL against the imported history).
    - Title pages show the reason.
    - A few reasons drift off their shelf's theme (a Knives Out nod on a What We Do in the Shadows shelf).

**R3 · Ask momoshtrem** (#158; depends on R2's agent and the AI core)
- Owns `internal/recommend/ask`, `api.Ask` (the SSE route's service and the threads), and `cmd/momoshtrem/wire_ask.go`, whose `Close` cancels the turns in flight (`main.go` registers it with `httpServer.RegisterOnShutdown`). At startup a turn a crash left `streaming` is marked `error`.
- **Agent.** The AI core's loop and R1's tools, on the thread's model; strict tools emit each `item` (match tag and reason) as a title resolves, and the final refinement chips. History comes from `ask_messages`, replayed unchanged: never edit earlier turns (an edited history also invalidates thinking blocks).
- **Streaming events** as §6, the step details from what the tools did ("2,184 plays · you loved Severance, The Bear, Ted Lasso").
- **Refinement.** Chips like "Lighter", "Shorter" or "Only anime" are follow-up messages; the turn re-ranks (`rerank`) instead of reading everything again.
- **Feedback** given on Ask cards shapes the next run and the next answers (R1).
- **Limits.** 30 turns a server-local day, 20 a thread, one at a time, and the budget; caps per turn (API calls, tool calls, cost) end it with `limit_reached`. There's no token cap: tokens count cache reads, so a long thread's replays would trip one on a cheap turn. A disconnect cancels the turn and records its usage (`feature: ask`, `ref_id` the turn).
- Acceptance:
  - An SSE contract test (`httptest`): the events in order, `: ping`, and a disconnect mid-stream that marks the turn `cancelled` with its usage recorded.
  - The web route streams without buffering (`Cache-Control: no-transform`, `X-Accel-Buffering: no`; check that Caddy doesn't buffer either).
- **Landed in #158:**
  - `internal/recommend/ask` is `api.Ask` and `aisettings.AskReporter` (`asks_today`, `ask_usd`), wired in `wire_ask.go`, which also installs it on the usage page and, at startup, ends a crash's `streaming` turn as `error` (`internal`). Threads read without a key; asking needs one. `library/ask_repo.go` stores the threads.
  - **The agent.** The thread's model (a new thread's is `rec_model`) at the core's default effort, `max_tokens` 32k, R1's five tools over a `tools.Cached` (the stored profile while it's under 12 h old, the pool kept an hour), then `show_picks`, then web search (`max_uses` 4) and fetch (2), with `drop_block`. Caps per turn: 12 calls, 24 tool calls, $1 (no token cap, see Limits). Once `show_picks` has shown a pick, a cap lets one more call end the turn (`aicore.Request.AllowFinal`; that call may not run tools), so a shown answer isn't turned into `limit_reached`. A `prompt_version` (`ask-v2` since #160; `ask-v1` is retired) pins the system prompt and every tool definition as sent: a test hashes the first request's `system` and `tools`, so a change to R1's tools, `show_picks` or the core's encoding needs a new version, and retires the old one (the old definitions aren't kept), whose threads then answer `409 thread_full`. aicore logs a warning when the API drops replayed thinking blocks (`input_transformations`, e.g. `prefix_binding_mismatch`). Each user message carries today's date (server-local) and the message in `<ask>`, or `<refine>` for a chip; a refinement that offers no chips keeps the previous turn's.
  - **`show_picks`** (strict, no union, nothing optional): `{intro, items: [{media_type, tmdb_id, title, year, match_tag, reason}], refinements}`.
    - Each pick is described from the TMDB cache (`meta`; a title not out yet gets a badge: "Jan 16", or its year past the next twelve months).
    - Its `title` and `year` must match the id's, by R2's rule (`recommend.SameTitle`, `SameYear`; a mismatch drops with `recommend.Mismatch`, the wording both agents use; added in #160).
    - Then it's checked (`candidates.Exclusion` on `TitleStates`). The name comes first, in both agents: a remembered id that happens to be watched drops as the title it really is, not as "watched", which would tell the model the title it named is watched.
    - It's sent as an `item` at once.
    - The answer lists what was shown, and what was dropped and why (watched, turned down, unknown to TMDB, an id of another title, already shown, past the turn's 10th shown), so the model may replace them. `match_tag` and `reason` are cut to 32 and 140 characters; refinements are kept to 5 distinct chips of up to 40.
  - **Ending a turn.** Its outcome is settled once: a cancelled context first marks the turn `cancelled` in the database and frees the slot (before the cut-off call's usage is written), and a turn already settled `done` never flips. The end (status, view, cost, and a done turn's messages, each message's blocks as one JSON array) is one transaction, committed before `done` or `error` is sent. A failing store never leaves a turn `streaming` (which would answer `ask_busy` to every ask): the end is retried with backoff for 10 s; a done turn whose messages can't be stored is written as `error` (`internal`); then, as a last resort, only its status is set (`error`); and if even that fails, the next ask sets it before it starts. `Close` cancels the turns in flight and waits up to 5 s for them to write their ends.
  - **Caddy** flushes a `text/event-stream` response at once (its `reverse_proxy` treats SSE as low-latency), and the Caddyfile has no `encode`, so nothing buffers between the browser and the web proxy.
- **Live check** (#160, the same account and stack as R2's; `curl -N` through the web proxy, signed in):
  - **The stream is live.** For "something like Severance but lighter" on Opus 5.5 the events came as they happened:
    - `start` at 0.03 s;
    - `history` at 12 s (the first call);
    - `tmdb` from 15 to 44 s;
    - `write` at 44 s;
    - `text` at 55 s;
    - nine `item`s from 55.8 to 58.3 s, and a tenth at 68.9 s;
    - `done` at 73 s.

    `: ping` came every 15 s, and the headers were `no-cache, no-transform` and `X-Accel-Buffering: no`.
  - **Cost.**
    - That ask: 73 s, 7 calls, $0.22.
    - The same ask after the fix below: 55 s, 5 calls, $0.24.
    - A refinement chip ("Even lighter"): 39 s, 4 calls, $0.09. Its first call read 24,835 of its 25,353 prompt tokens from the cache.
  - **Cancel.**
    - Killing curl as the web step started ended the turn `cancelled` at 13.9 s, with its $0.03 recorded (the cut-off call's prompt).
    - An ask sent right after it got a 200, not a 409.
    - The thread reloads with the cancelled turn's finished steps only, and no stored messages.
  - **Bug, fixed in #160: two of ten cards showed the wrong title.**
    - The model sent TMDB ids from memory: 82856 (The Mandalorian) for Upload, and 75299 for Russian Doll. `check_seen` passed them, so two cards showed the wrong title with another title's reason.
    - `show_picks` now takes `title` and `year` and checks them (above).
    - The prompt says ids come from tools (`ask-v2`).
    - The rerun showed ten right cards, every title looked up with `tmdb_search`.
  - **Wasted `code_execution` calls (#193).** Opus 5.5 tries to call `tmdb_search` from `code_execution`, which the 20260209 web tools bring for dynamic filtering.
    - It fails (`NameError`), and that call becomes a two-iteration call that bills its prompt twice (about $0.005 of cache reads).
    - `ask-v2`'s prompt line didn't stop it.

**Cost ballpark** (measured in #160's live run on Opus 5.5, with a 1,872-play Trakt history):
- **The classifier** costs a fraction of a cent per torrent on Haiku 5.5 (about $0.0007 in `ai_always` mode, #5).
- **A picks run: $0.32 on Opus 5.5, $0.14 on Sonnet 5.5** (the design guessed $0.36 and $0.07).
  - Rates: Opus 5.5 is $4 / $20 per million input / output tokens, Sonnet 5.5 $2 / $10.
  - About 40% of a run is the first call writing the ~24k-token prefix to the cache (tools, system prompt, profile, 90 candidates).
  - Most of the rest is output.
- **An ask: $0.22–0.24, a refinement $0.09** (the design guessed $0.02).
  - About half of an ask is writing new prompt to the cache: the tools, then the profile and the candidates the tools return.
  - A third is output.
  - A refinement reuses the thread's cache.
- **Prompt caching works.**
  - 82% of all prompt tokens were cache reads.
  - From a run's or turn's second call, 72–99% of each prompt was. The low end is the call that first reads the tools' results.
- **A month.**
  - Nightly Opus runs come to about $10.
  - Weekly Opus about $1.40.
  - Nightly Sonnet about $4.
  - On the $15 default budget, nightly Opus leaves room for about 20–25 asks.
- **Web searches** are billed per search ($0.01) on top of tokens. Neither live run used one.
- **Settings › AI & picks** shows the month's spend by feature, the budget, and estimates from real usage: the mean of each model's last 5 runs, and of the last 20 asks. There are no static figures.

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
  - **Landed in #136 (F2b, #8):** watch state on the show page (the Up next row from `watch_state.next_episode`, else the frontier; watched rows with and without a file) and the Library's Watching filter and Last watched sort.
- **F3 · Torrent search sheet** (depends on B4; B2 for Inspect)
  - Delivered by #6's slices (#92–#95; Settings › Quality: #94). §6 "Search" is the contract; where the bullets below differ, §6 wins.
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
  - **Landed in #135 (F5a, #8):** For You without AI: the hero's deterministic picks (a ready Up next show, ready watchlist titles, then trending; stable per day), Up next, New episodes and the watchlist with one action per availability (`EpisodeCard`), the trending fallback, and the first-time, Trakt-off and first-sync states. The AI shelves, Tonight's pick, feedback, the Ask band and the refreshing state are #9's; #9 hands `hero_picks` to `HomeHero`.
  - **#9's For You (#155):** the AI shelves between the rails, Tonight's pick (`hero_picks` into `HomeHero` as `source: 'recommendation'`; the deterministic picks when there are none), feedback with Undo (§6 "Feedback"), the refreshing banner while `active_run` is set, "Writing your shelves" in the first-sync steps (a `first_sync` run), the Ask band and the picks footer ("Picks written by Noir at 03:00"). Without a run (`run: null`) or with `enabled: false`, trending stays.
  - **Landed in #155 (F5b, #9):** For You with Noir's picks, built against the stateful mocks:
    - **Order.** The shelves follow the design (`lib/for-you/shelves.ts`): Up next, New episodes, the first shelf, the Ask band (desktop; the phone gets an Ask entry under the hero), the other shelves, the watchlist, then `coming_soon`.
    - **Cards.** Each card has its library pill, or on Coming soon its date (`badge`, else a future `release_date`), a two-line reason, and the feedback row.
    - **Tonight's pick.** It gets a "…" feedback button, and a dismissed pick dims in place.
    - **Feedback.** It's shared with Ask: `components/recommendations` (`useRecFeedback`, `FeedbackRow`, `FeedbackSheet`) on a per-title store (`lib/recommendations/feedback-store.ts`).
    - **Refreshing.** The banner shows the step under way and "N of M shelves done", and the first shelf after the Ask band turns to skeletons.
    - **A failed refresh.** It leaves a quiet note, read from `/runs?limit=1`, because `/home` carries only succeeded runs.
    - **First sync.** `forYouState` stays on `first_sync` while a `first_sync` run writes the first shelves, and re-reads `/home` when the import ends.
    - **Gating.** `useAskAvailable` gates the Ask entries. (Its stand-in for `/home` before #157, a 503 read as no picks, went in #160.)
  - **#9's Ask (#156):** `/ask` and `/ask/{thread_id}`: the named steps (`web/src/lib/recommendations/steps.ts`), the streamed intro and cards, refine chips, Stop, threads, and the budget and limit states (`ApiError.resetsAt`). Its entry points use `askHref(q)` (`web/src/lib/recommendations/ask-href.ts`): the top-bar pill, the palette's "Ask momoshtrem: “q”" (always present), Search's "Not quite it?" card and idle chip (#101), and Discover's empty state with a query from the active filters (#100). The entries hide while `/recommendations/home`'s `configured` is false (no key); picks being off doesn't hide them. Discover's moods stay fixed TMDB presets.
  - Both build against the stateful mocks first: `recs-*` and `ask-*` scenarios (`web/src/mocks/scenario.ts`).
  - **Landed in #156 (F5c, #9):** Ask momoshtrem, built against the stateful mocks (`ask-working` holds the design's working frame):
    - **One screen.** It lives in `app/(protected)/ask/layout.tsx` and both pages render nothing, so a stream survives `start` moving the URL to `/ask/{thread}` (`history.replaceState`). Desktop has a rail (New conversation, recent threads, the month's AI spend from `/settings/ai/usage`); the phone is a task page with Past conversations (`?sheet=ask-history`) and a composer fixed above the safe area.
    - **The stream.** `lib/ask/turns.ts` folds the SSE events into turns (a pure reducer) and `lib/ask/copy.ts` has each failure's card. `hooks/useAskConversation.ts` sends, stops (Stop or Esc), lets the stream go at `done` or `error`, and retries an `ask_busy` that follows your own Stop once, after 600 ms. `/ask?q=` asks at once.
    - **Cards.** The poster, match tag, facts, reason and #155's feedback row. Add to library is For You's (`useAddRecTitle`), whose toast offers Find a torrent: the torrent sheet on the movie, or a show's season-1 packs. The answered turns' titles are looked up in the library (`/library?ids=`), so a title added from its "…" sheet shows In library too; since #160 that lookup keeps its last answer while a new turn's ids load, and adds are tracked per title.
    - **Try again.** A turn refused before its stream was never saved, so the retry takes its place; one that failed mid-stream stays in the thread and the retry is a new turn; `refused` and `limit_reached` offer none. A 429 pauses the composer until `resets_at`, as `budget_reached` does before any ask; a clock (`hooks/useNow.ts`) lifts the pause.
    - **Entry points.** `AskEntry` and `hooks/useAskAvailable.ts`, shared with #155; the palette's rows show Noir.
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
  - **Landed in #134 (F8a, #8):** Settings › Trakt (off, device code, waiting, connected with sync status, counts and the three switches, disconnect) and the palette's Connect / Sync Trakt now. Settings › AI is #9's.
  - **#9's Settings › AI & picks (#159):** "Write picks automatically" (`rec_enabled`), the schedule (nightly, weekly, manual; the next run and the last run's time and cost from `recs`), the model cards with `estimates.run_usd`, Run now (refresh, then the run's steps), the monthly budget (the bar from `by_feature`, `resets_at`, the budget input; `null` is no cap) and the episode-mapping switches. Saves are partial PUTs; a 422 names its field. Without a key it says how to add one and still saves.
  - **Landed in #159:** Settings › AI & picks (`components/settings/AiSection.tsx`, `ai/`; `lib/settings/ai.ts`). The design's "Write picks automatically" reads "Write personal picks", since `rec_enabled` is the master switch (off: no shelves and no runs, Manual included; Ask still works). Run now posts refresh, toasts "Writing your picks" (Watch → For You), shows the run's steps (`/recommendations/runs/{id}`) and its errors in the card. The model cards fall back to "No estimate until its first run" while `estimates.run_usd` is null. The month's name and "Resets Nov 1" come from `usage.month`, not `resets_at` (midnight server-local reads as the 31st west of the server). The schedule's help takes the next run's day and time from `next_run_at` (in browser time); without one it says the slots in server time. The budget input takes dollars from 0 to 10,000, rounded to cents (empty: `null`). A server 422 on it shows in plain words. Saves send only the fields edited on the page, so a change made elsewhere stays. The classifier's help names `classifier_model` and its per-torrent cost. The nav reads "Nightly · $3.12 of $15.00" only after hydration (#148).

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
  - Remove the legacy pieces: `?wait=true` (#60), the web Jackett route and env (done in #6 / #95), the old hooks and components.
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
- **Trakt API changes.** §7 B5's facts were checked on 2026-10-16. Trakt has changed token lifetimes before, so refreshes follow each token's `expires_in`, and a token that stops working means `reauth` (shown in Settings), never lost data.
- **AI cost creep.** Mitigated by the budget cap, the usage page and the configurable schedule.
- **Until G4 ships HTTPS,** password autofill may stay unreliable even after G3.

**Open questions** (decide during D2 or Phase 1)
- The mascot's name: "Momo" is only a working name.
- Default auto-apply policy for high-confidence mappings (proposed: on).
- ~~Default recommendation model~~: decided 2026-10-16 (#9), Opus 5.5, with a $15 monthly budget shared by every AI feature.

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
