# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Task Tracking: GitHub Issues

All work is tracked in GitHub issues on `ShapedTime/momoshtrem`. **Every agent starts from an issue and reports back to it.**

1. **Start from an issue.** `gh issue list --label v2 --state open`, then `gh issue view <n> --comments`. Read the body *and* every comment: they hold decisions and progress from earlier agents.
2. **No issue? Create one** for anything beyond a trivial fix (`gh issue create`) with *Goal*, a *Scope* checklist and *Done when*. Work you discover that's out of scope gets its own new issue (linked), not a bigger current one.
3. **Pick unblocked work.** Respect "Depends on". Skip issues labelled `in-progress` or `blocked`.
4. **Claim it.** `gh issue edit <n> --add-label in-progress` and comment your plan and branch name.
5. **Branch** `<issue#>-<short-slug>` from `main`. Issues labelled `epic` are large: split them into sub-issues ("Part of #n") or one PR per coherent slice.
6. **Report progress on the issue.** Tick checklist items in the body as they land. Comment on decisions, API contract changes and blockers. When blocked, add `blocked` and say what's needed.
7. **Open a PR.** Conventional-commit title. Body has `Closes #n` (or `Part of #n`), what changed, verification output, and screenshots at 390 px and 1440 px for UI work.
8. **Finish.** Remove `in-progress` and leave a summary comment: what shipped, what didn't, and links to follow-up issues.

Labels: `v2` (redesign and features), `ops` (deployment, CI/CD, backups and server setup), `streaming` (the streaming path and its tooling, streamlab), `epic` (large, may be split), `in-progress` (claimed), `blocked` (waiting on an issue or the user).

## Project Overview

Self-hosted media streaming stack for Apple TV with library management:
- **momoshtrem** (Go): Library-first torrent streaming service with WebDAV
- **web** (Next.js, package `momoshtrem-web`): the momoshtrem UI for browsing TMDB, managing the library and searching torrents. Lives in `web/`.
- **Jackett**: Torrent indexer aggregator, queried by momoshtrem for torrent search
- **FlareSolverr**: passes Cloudflare challenges for Jackett's indexers behind one (1337x, EZTV, RuTracker); internal only, set once in Jackett (README)
- **Postgres**: Library database

## Redesign (v2)

v2 is a redesign plus new features (Trakt, AI recommendations, episode mapping, torrent search). Read these before any v2 work:
- `docs/redesign/PLAN.md`: decisions (§2), data model (§5), **API contract (§6)**, task details (§7), "Acceptance for every F task" (end of §7), verification (§9). If the contract is wrong, fix it in PLAN.md in the same PR.
- `docs/redesign/design-cinematic/`: the chosen **Cinematic dark** design.
  - `HANDOFF.md`: layout grid, component inventory, interactions, and data the screens need beyond PLAN §6.
  - `css/tokens.css`, `tokens/theme.css`: tokens and the Tailwind v4 `@theme` mapping.
  - `screens/<screen>/<state>-<width>.png`: every screen and state.
  - `prototype.html`: the clickable prototype. Open it in a browser and use the hash routes from HANDOFF §7.
  - `assets/logo/`: logo files.

## Ops track

Server setup, CI/CD and backups are a separate track from v2: issues labelled `ops` (`gh issue list --label ops --state open`), epic #32. Read `docs/ops/PLAN.md` before any ops work: decisions (§2), state inventory (§5), rollback model (§6) and the deploy contract (§8). The VM is configured by the Ansible playbook in `ops/ansible/` (runbook: `docs/ops/HOST.md`). CI checks PRs and publishes images to GHCR from `.github/workflows/ci.yml` (runbook: `docs/ops/CI.md`). Deploys run from the Mac with `ops/deploy/deploy.sh` over SSH, never from GitHub (runbook: `docs/ops/DEPLOY.md`). The repo is public, so secrets and the public hostname never go in it.

## Streaming track

The streaming path (WebDAV → VFS → anacrolix, and HDRezka remote streams: `internal/{webdav,vfs,streaming,torrent,sessions,hdrezka}`) is fragile, and it was tuned by hand against Infuse. It has its own track: issues labelled `streaming`, epic #171. Read `docs/streamlab/PLAN.md` before any streaming work:
- decisions (§2)
- suspected live bugs F1–F12 (§3)
- production hooks (§4)
- the lab (§5)
- verdict rules (§6)
- streamtrace's no-behaviour-change rules (§7)
- read-only production (§8)

Until streamlab's gate exists, keep changes to this path minimal, and test them on the device.

## Commands

```bash
# Start all services (compose.yml). .env must set the data paths and the WebDAV
# values; cp .env.example .env covers them locally. Without MOMOSHTREM_TAG the
# momoshtrem and web images are built as :dev
docker compose up -d

# Rebuild and start
docker compose up -d --build

# Frontend
cd web && npm run dev
cd web && NEXT_PUBLIC_API_MOCKING=enabled npm run dev   # mock mode: MSW answers every PLAN §6 endpoint
cd web && npm run lint && npx tsc --noEmit && npm test
cd web && npx playwright test   # E2E (own dev server on :3100, or E2E_PORT=<port> to run beside another; first run: npx playwright install chromium)

# Go service (momoshtrem)
cd momoshtrem && go build -o momoshtrem ./cmd/momoshtrem
cd momoshtrem && go vet ./... && go test ./...
docker compose run --rm momoshtrem backfill-metadata [--dry-run]   # fill TMDB metadata for titles added before #4 (never renames WebDAV paths)
docker compose run --rm --no-deps momoshtrem hdrezka-probe [-translator 238] [-season 1 -episode 1] <query>   # check HDRezka resolves to a seekable MP4 from this host (no database; exit 1 on failure)
docker compose run --rm momoshtrem recommend-run [-dry-run]   # one picks run in the foreground (costs cents: prints the shelves and the cost); -dry-run builds its first request and counts its tokens (free)
# Episode-mapping eval on the corpus's golden torrents: accuracy, cost and latency per mode.
# The AI modes (auto, ai_always) call Claude and cost cents; -modes rules and -dry-run (request sizes only) are free
cd momoshtrem && go run ./cmd/classifier-eval -env-file ../.env [-modes rules,auto,ai_always] [-cases id,…] [-dry-run] [-v]

# Deploy (from the Mac): the newest green main commit, or a given one, to a VM over SSH
ops/deploy/deploy.sh -n momoshtrem-v2            # dry run: print the plan, change nothing
ops/deploy/deploy.sh momoshtrem-v2 [<commit>]    # deploy; a rollback is an older commit
```

## Architecture

```
                    ┌─────────────────────────────────────────┐
                    │        web: momoshtrem UI (:3000)       │
                    │   Browse TMDB, manage library, search   │
                    └──────────────────┬──────────────────────┘
                                       │
                        ┌──────────────┴──────────────┐
                        ▼                             ▼
                    TMDB API                 momoshtrem (:4444) ───► Jackett (:9117) ───► FlareSolverr (:8191)
                                                      │              (torrent search)     (Cloudflare challenges)
                                                      ▼
                                             Postgres (library)
                                                      │
AppleTV/Infuse ─────────────────────────────► WebDAV (:36911)
                                                      │
                                                      ▼
                                               OpenSubtitles API
```

The v2 target architecture is in PLAN.md §4.

**For You** (`/`): Trakt (read-only, synced every 30 min) drives Up next, New episodes and the watchlist, each card with what it needs next (Play in Infuse, Find, Add…); without Trakt it falls back to trending. With `ANTHROPIC_API_KEY`, Noir's picks join them: a Claude agent run (nightly at 03:00 server-local by default, or weekly or manual) reads the Trakt taste profile, TMDB and the web and writes AI shelves and Tonight's pick, each card taking feedback; **Ask momoshtrem** (`/ask`) answers a request over SSE with picks and refine chips, in saved threads. Every AI feature shares one monthly budget (Settings › AI & picks, $15 by default).

**Flow**: Browse TMDB → Add to library → Search torrents (the search sheet, `?sheet=torrent`; momoshtrem queries Jackett and ranks releases by the quality profile) → Assign torrent (a mapping job: rules, then AI for unsure files; review at `/mapping/{id}` unless all high) → Stream via WebDAV (Infuse)

**Key design**: momoshtrem is library-first. The VFS structure is driven by the database (movies, shows, episodes), not by torrent contents. Migrations are embedded from `momoshtrem/internal/library/migrations/`. The root `momoshtrem/migrations/` folder is a legacy SQLite schema.

## Key APIs

### momoshtrem REST API (port 4444)
```
GET  /api/titles/{movie|tv}/{tmdb_id}       # Title page: TMDB details, library state and availability stage
GET  /api/titles/tv/{tmdb_id}/seasons/{n}   # One season's episodes and ranges
GET  /api/library             # Library list with filter counts and a summary (filters, sort, search); ?ids=movie:603,tv:1399 looks up TMDB titles (≤200)
POST /api/movies              # Add movie by TMDB ID
POST /api/shows               # Add show by TMDB ID
DELETE /api/{movies|shows}/{id}?delete_data=   # Remove a title: its subtitles go, torrents nothing else uses are released (with their data when asked)
POST /api/{shows|movies}/{id}/assign-torrent   # Start a mapping job: 202 {job_id}; {info_hash} alone re-runs a known torrent (legacy ?wait=true applies it and answers the old shape)
GET  /api/assignment-jobs/{id}                 # The job and one proposal per file; poll while it works
GET  /api/assignment-jobs?status=open          # Open jobs, newest first (the Activity banner)
PATCH /api/assignment-jobs/{id}/proposals      # Accept, ignore or re-target files, in one batch
POST /api/assignment-jobs/{id}/reclassify      # Re-check with AI, with a hint (503 without AI)
POST /api/assignment-jobs/{id}/apply           # Write the assignments and rebuild the VFS (409 while anything blocks)
POST /api/assignment-jobs/{id}/discard         # Cancel; drops the torrent when nothing else uses it
PUT  /api/{episodes|movies}/{id}/assignment    # Manual override: one file of a torrent, outside any job
GET  /api/torrents/{hash}/files                # A torrent's files and what each one is mapped to (loads the torrent)
GET  /api/shows/{id}/episodes                  # Every season's episodes, Specials included (the episode picker)
GET  /api/torrents                         # Every torrent the library knows, loaded or not: state (streaming|downloading|paused|idle), speeds, titles, sessions
GET  /api/torrents/summary                 # Speeds with 60 s of history, counts per state, disk, streaming sessions (Activity's strip, the nav badge)
GET  /api/torrents/{hash}                  # Activity's detail: piece runs, files and what they play as, trackers, sessions; never loads the torrent
POST /api/torrents/{hash}/pause|resume     # Sticky pause, kept across restarts (app_settings `torrents`); also /torrents/pause-all|resume-all
DELETE /api/torrents/{hash}?delete_data=   # Drop it: open jobs cancelled, assignments deactivated (history kept), pieces deleted when asked
GET  /api/search/torrents                  # Torrent search across Jackett's indexers: scored releases, facets, per-indexer progress (poll while complete=false)
POST /api/search/torrents/resolve          # A release without a magnet: the server fetches its .torrent and answers the magnet
POST /api/search/torrents/inspect          # Inspect: a dry-run mapping job for a release or magnet (202 {job_id})
GET|PUT /api/settings/quality-profile      # The quality profile search ranks by
GET  /api/movies/{id}/subtitles/search   # OpenSubtitles, ranked by release (also /episodes/{id})
GET  /api/integrations/trakt               # Trakt (read-only): state off|pending|connected|reauth, the live device code, sync progress, counts, settings
POST /api/integrations/trakt/connect|sync  # Device-code connect (the backend polls Trakt); sync 202 (joins a running one)
PUT  /api/integrations/trakt/settings      # use_ratings, show_watchlist, include_paused (kept across a disconnect)
DELETE /api/integrations/trakt             # Disconnect: revoke the token, purge everything imported
GET  /api/me/up-next|new-episodes?days=|watchlist?type=|history   # From what Trakt imported, with the library's files; empty without Trakt, but new episodes of library shows
GET  /api/recommendations/home             # For You's picks: the newest succeeded run's shelves and the day's hero picks, the active run (poll 3 s while set); always 200 (configured/enabled false without a key or with picks off)
POST /api/recommendations/refresh          # A manual picks run: 202 {run_id}, joining an active one; 409 recs_disabled|no_history, 429 budget_exceeded, 503 without a key
GET  /api/recommendations/runs[/{id}]      # Runs, newest first (?limit= 1–50); one run's steps, shelves done and usage (poll 2 s while queued or running)
POST /api/recommendations/feedback         # like|dislike|not_interested|seen on a title, an upsert (the negative ones hide it from every AI surface); DELETE /feedback/{media_type}/{tmdb_id} clears it
POST /api/recommendations/ask              # Ask momoshtrem: JSON errors first (thread_full, ask_busy, ask_limit, budget_exceeded…), then SSE start, status, text, item, error|done; a disconnect cancels the turn
GET  /api/recommendations/threads[/{id}]   # Ask's threads (≤ 50, newest first); one thread with its turns' saved views
GET  /api/recommendations/debug/profile    # Dev only, with RECS_DEBUG: R1's taste profile and candidate pool, built now, stored nowhere
GET|PUT /api/settings/ai                   # rec_enabled, rec_model, rec_schedule (nightly|weekly|manual), monthly_budget_usd ($15; null: no cap), the classifier's settings; PUT is partial, a 422 names the field
GET  /api/settings/ai/usage                # The month's AI spend by feature (recs, ask, classifier) against the budget, the last, active and next run, estimates, asks today
```
Full list: `momoshtrem/internal/api/router.go`. v2 endpoints: PLAN.md §6 (mapping jobs: "Assignment"; torrent search: "Search"; torrents: "Torrents and activity (B6)"; Trakt, `/me` and watch state: "Trakt (B5)" and "Watch state rules"; picks, Ask and the AI settings: "Recommendations (R1–R3)").

### Jackett Torznab API (port 9117)
Only momoshtrem calls it (`internal/torznab/`): the indexer list, then one search per indexer:
```
GET /api/v2.0/indexers/all/results/torznab/api?apikey={key}&t=indexers&configured=true
GET /api/v2.0/indexers/{id}/results/torznab/api?apikey={key}&t=search&q={query}
```

## Code Quality

**ALWAYS follow these standards:**
- `docs/web/CODE_QUALITY.md` - Architecture, TypeScript, error handling
- `docs/web/STYLE_GUIDE.md` - The cinematic design system in code: tokens, layout, components, states, copy (gallery: `/dev/ui`)
- `docs/momoshtrem/CODE_QUALITY.md` - Go patterns, interfaces, concurrency

**Key principles:**
- API routes are thin orchestration; business logic lives in `lib/api/` (web) or `internal/<domain>` (Go)
- TypeScript strict mode, avoid `any`, use type guards
- Custom error classes: `APIError`, `NotFoundError`, `ValidationError`
- Conventional commits: `feat:`, `fix:`, `refactor:`, `chore:`, `docs:`

## Key Source Locations

**momoshtrem (Go):**
- `cmd/momoshtrem/main.go` - Entry point
- `internal/api/router.go` - REST API endpoints
- `internal/library/` - Models, repositories, embedded migrations
- `internal/catalog/` - Title pages, seasons and `/library`: availability stages, episode states, ranges, "more like this"
- `internal/me/` - `/me` (B5): Up next (`upnext.go`, pure rules), new episodes, watchlist, history; `library/trakt_reads.go` reads the Trakt tables, with the one episode matching rule (`sameEpisode`) and `EpisodesByKeys`
- `internal/metadata/` - TMDB sync into the library (specials, absolute numbering, anime detection) and `backfill-metadata`
- `internal/tmdb/` - TMDB client with retries, rate limit and the `tmdb_cache` cache
- `internal/artwork/` - Backdrop dominant colour for the hero glow
- `internal/identify/` - Pure rules classifier: `Classify` maps a torrent's files to episodes with a confidence and a reason, `RankMovieFiles`, `MergeAI`; `token/` tokenizer; `corpus/` loads the corpus in `testdata/` (gates: ≥95% exact, zero wrong `high`)
- `internal/identify/ai/` - The Haiku resolver: asks Claude about the files the rules can't place (prompt, scope, cost)
- `internal/release/` - Release-name parser on `identify/token`: `Parse` gives PLAN §6 `Release.attrs` (its JSON), the title, year, display title and pack coverage (absolute spans flagged). Search and subtitle ranking use it
- `internal/torznab/` - Jackett over Torznab: the indexer list and caps (`Indexers`), one search per indexer (`Search`), `Fanout` with per-indexer progress, health for `/status`, `.torrent` `Download` (SSRF guard). Links and errors never carry the API key (`SafeError`)
- `internal/search/` - Torrent search (`GET /search/torrents`): `plan.go` builds the query plan; `Service` caches searches per plan (10 min, detached single-flight runs, `refresh`), ingests results (`ingest.go`: title match, dedupe, coverage and scope through `identify.Numbering`), and reads them per request (`read.go`: quality profile, filters, disjunctive facets, sort, `missing`); `Resolve` turns a release id into a magnet (also Inspect's resolver)
- `internal/service/assignment_*.go` - Mapping jobs: create and worker (`_jobs`, `_worker`), the contract's rules (`_rules`), PATCH (`_review`), re-check (`_recheck`), Apply and its VFS writer (`_apply`), overrides and torrent files (`_override`), legacy `?wait=true` (`_legacy`), library show → classifier catalog (`_catalog`)
- `internal/library/assignment_job*.go`, `ai_store.go`, `settings_ai.go` - Jobs and proposals (claimed with `FOR UPDATE SKIP LOCKED`), `ai_usage` and `ai_cache`, AI settings in `app_settings`
- `cmd/classifier-eval/` - The classifier on the golden corpus, rules and AI modes (Commands above)
- `internal/subtitle/` - Subtitles: OpenSubtitles search ranked by release match, downloads, torrent subtitles
- `internal/hdrezka/` - HDRezka client: search, title pages and translations, CDN streams (MP4 qualities, subtitles), sizes, ranking against TMDB (`cmd/momoshtrem/probe.go`: `hdrezka-probe`); `resolver.go` caches signed links, `streamer.go` opens Range bodies on the CDN
- `internal/service/hdrezka*.go` - HDRezka search, titles, streams and synchronous assigns (`_assign`); `hdrezka_source.go` streams stored streams for the VFS
- `internal/vfs/library_fs.go` - Virtual filesystem; `remote_file.go` streams non-torrent (HDRezka) files
- `internal/torrent/service.go` - Torrent service interface; `activity.go` the ActivityManager (idle unloading, sticky `StatePaused`), `hashlock.go` one add/wait/drop per hash, `metainfo_store.go` stored `.torrent`s (`{metadata_folder}/metainfo/`), `speed.go` the speed sampler, `detail.go` piece runs, files and trackers, `infohash.go` hex or base32 hashes
- `internal/activity/` - Torrents and activity (PLAN §6 B6): the DB's known torrents merged with live state (`rules.go`: state order, titles, scope), the summary (speeds, counts, disk), pause and resume, the detail that never loads a torrent
- `internal/sessions/` - Streaming sessions for every source: reads grouped by client, User-Agent and file, sniff reads skipped, client and device from the User-Agent (`useragent.go`), rate, position and buffer ahead; `request.go` carries the WebDAV request into `LibraryFS.Open`
- `internal/streaming/prioritizer.go` - Piece prioritization (torrents stream on demand: pieces only around readers, kept in an LRU file cache)
- `internal/secret/` - AES-256-GCM box for secrets at rest under `SECRET_KEY` (64 hex chars, or ≥ 32 chars hashed)
- `internal/trakt/` - Trakt, read-only (PLAN §6 B5): `client.go` (auth and API hosts, rate limits, 429/5xx retries, `Paginate`), `auth.go` the connection (device-code connect, sealed rotating tokens, `Token`/`Invalidate`/`Get`, reauth, disconnect, `Hooks` for the sync); `trakttest/` a fake Trakt; `library/trakt_auth_repo.go` stores and purges
- `internal/trakt/sync*.go` - The Trakt sync (#132): `sync.go` the `Syncer` (one sync at a time, one queued; the 30-minute schedule, nightly reconcile, `Nudge`, the `Hooks`), `sync_run.go` the steps (initial import = reconcile, incremental from `last_activities`, `details` artwork through `tmdb.Cached`), `sync_items.go` Trakt's answers as rows; `library/trakt_sync_store.go` writes them, each write guarded by the connection's `connected_at`
- `internal/aicore/` - The Claude core every AI feature shares (#154): our own streaming tool loop (`Client.Run`: strict custom tools one at a time, server tools, `pause_turn`, refusal fallbacks, caps on calls, tokens, cost, tool calls and web searches, `AllowFinal`), prices per model plus web searches, `Ledger` (`ai_usage` rows, the monthly budget check), `MonthBounds`/`DayBounds` in `TZ`, `CountTokens`/`RequestBody`; `fake/` a scripted Anthropic SSE server that records requests, for tests
- `internal/aisettings/` - `/settings/ai` and its usage: partial PUTs on the `ai` document, spend by feature; the picks and Ask add their parts with `SetRecs`/`SetAsk`, and `SetOnChange` reschedules runs
- `internal/recommend/` - Picks and Ask (PLAN §6 "Recommendations (R1–R3)"): `types.go` the wire shapes, `feedback.go` (`FeedbackService`), `debug.go` (`RECS_DEBUG`'s route), `match.go` (`SameTitle`, `SameYear`: a picked id must be the title the model names, for shelves, hero picks and `show_picks`); `profile/` the taste profile (R1: weights, loved, dropped, aversions, `CheckHistory`), `candidates/` the candidate pool (≤ 150, the `Exclusion` rule: watched, rated, hidden, negative feedback), `tools/` the agent's strict tools (`get_taste_profile`, `get_candidates`, `tmdb_search`, `tmdb_details`, `check_seen`; goldens in `testdata/`), `agent/` the `Tool` type
- `internal/recommend/runs/` + `schedule/` - Picks runs (R2, #157): `runs` the prompt, `submit_shelf`/`submit_hero_picks` with their validation (`validate.go`), progress steps, `Runner.Execute` (web search 5 a call, 20 a run; one nudge turn), and the reads (`/home`, runs, the title pages' reason, the day's hero picks); `schedule` the `Scheduler` (03:00 slots, catch-up, one worker, refresh, `FirstSync` from Trakt's initial import, `NextRunAt`); `cmd/momoshtrem/recommend_run.go` is `recommend-run`
- `internal/recommend/ask/` - Ask momoshtrem (R3, #158): turns over SSE (`turn.go`), the step events (`steps.go`), `show_picks` (`picks.go`), thread titles and views, the limits (30 a day, 20 a thread, one at a time), append-only `ask_messages` replay, `Close` on shutdown
- `internal/library/taste_reads.go`, `taste_profile_repo.go`, `rec_feedback_repo.go`, `rec_runs_repo.go`, `ask_repo.go` - Migration 006's stores: what the profile reads (`TasteRepository`, `TitleStates`), stored profiles, feedback, runs with their shelves and hero picks, Ask's threads and turns
- `cmd/momoshtrem/wire_{ai,recs,ask}.go` - Wiring the AI core, the picks and Ask, with or without a key

**web (Next.js, `web/`):**
- `src/components/ui/`, `src/components/brand/` - Design-system primitives, logo and Noir (gallery: `/dev/ui`)
- `src/components/shell/` - App shell: top bar, tab bar, account menu, ⌘K palette, URL-driven sheets
- `src/components/title/` - Movie and show pages: hero, primary panel and action dock, subtitles, episodes (`show/`: season tabs, rows, long-season tools, show settings and episode sheets)
- `src/lib/title/` - Pure title helpers: stage copy (`primaryActionFor`), formats, facts, episodes (`parseJump`, ranges, next-up)
- `src/components/mapping/` + `src/app/(protected)/mapping/[id]/` - Mapping review (`/mapping/{job_id}`): progress steps, grouped rows, the episode picker (desktop pane, phone sheet), sticky Apply foot, Applied state
- `src/lib/mapping/` - Pure mapping-review rules: counts, conflicts, ranges, short file names, foot state (`review.ts`) and picker search and tags (`picker.ts`)
- `src/components/source/` - The source sheet's shared head (Torrents · HDRezka tabs) and the Replace menu; `src/lib/source/context.ts` reads and builds its URL params (`movie=<tmdb>`, or `show=<tmdb>` with `season` / `se`)
- `src/components/search/` - Torrent search (`?sheet=torrent`): `TorrentSearchSheet`, release cards, chips and the Filters, Sort and Inspect sheets, states; `src/hooks/useReleaseSearch.ts` (polling, resolve, Inspect) and `useAssignTorrent.ts` (Add → mapping job)
- `src/lib/search/` - Pure search helpers: default query and scope, params, copy, filters and facets, size scale, session drafts, Inspect summaries
- `src/components/library/` + `src/lib/library/` - Library (`/library`): grid and list, filter chips with counts, sort, the "…" menu (`actions.ts`), Remove with Undo; `LibraryPill` and `src/hooks/useLibraryLookup.ts` (`/library?ids=`) mark library titles on TMDB cards; `query.ts` is the `/library` filter the mocks and Jest share
- `src/lib/undo/removals.ts` + `src/hooks/useUndoableRemoval.ts` - Undo as a delayed commit: the item hides at once and the DELETE goes when the 10 s toast closes (or on `pagehide`)
- `src/components/discover/` + `src/lib/discover/` - Discover (`/discover`; `/movies` and `/tv-shows` redirect): filters in the URL (nuqs, `url.ts`), the genre table, mood presets (`moods.ts`), TMDB params (`tmdb-query.ts`); web route `src/app/api/discover/`
- `src/components/title-search/` + `src/lib/title-search/` - Search (`/search`) and the ⌘K palette's data: ranking (`rank.ts`), library rows, palette actions, recent searches; web route `src/app/api/title-search/` (TMDB movie, tv and person searches)
- `src/components/ask/` + `src/lib/ask/` - Ask momoshtrem (`/ask`, `/ask/{thread}`, #156): the screen lives in `app/(protected)/ask/layout.tsx` so a stream survives `start` moving the URL; `lib/ask/turns.ts` folds the SSE events into turns (pure reducer), `copy.ts` has each failure's card, `route.ts` the thread in the URL; `hooks/useAskConversation.ts` sends, stops and retries `ask_busy` after Stop; `useAskLibrary.ts` the cards' library state (`/library?ids=`, keeping its last answer) and per-title adds. `AskEntry` + `hooks/useAskAvailable.ts` (`/home` `configured`) gate every entry point
- `src/components/recommendations/` + `src/lib/recommendations/` - What For You and Ask share: `RecPosterCard` (`cards.ts`), feedback (`useRecFeedback`, `FeedbackRow`, `FeedbackSheet`; `feedback.ts` the rules, `feedback-store.ts` per title, so every card of a title agrees; Undo restores the previous signal), `useAddRecTitle`, the run and Ask step labels (`steps.ts`), `askHref(q)` (`ask-href.ts`)
- `src/components/activity/` + `src/lib/activity/` - Activity (`/activity`, `/activity/{hash}`; `/torrents` redirects): summary strip and sparkline, sections by state, torrent cards, the detail with health in plain words (`health.ts`), files and the Advanced piece map; offline keeps the last known data
- `src/components/for-you/` + `src/lib/for-you/` - For You (`/`): its states (`state.ts`: first time, Trakt off, first sync, personal), the hero (`hero.ts`: Noir's Tonight's pick from `/home`'s `hero_picks`, else its own deterministic picks), the Up next, New episodes and watchlist rails, Noir's shelves in the design's order (`shelves.ts`, `Picks.tsx`: the Ask band, the refreshing banner, the failed-refresh note and the picks footer from `picks.ts`); `components/ui/EpisodeCard.tsx` with one action per availability (`episode-card.ts`)
- `src/components/settings/` - Settings: Trakt (`TraktSection.tsx`, `trakt/`: off, device code, waiting, connected; `lib/settings/trakt.ts`; `hooks/useTraktIntegration.ts` polls while pending or syncing, `useTraktActions.ts`), AI & picks (`AiSection.tsx`, `ai/`: `PicksCard` with the schedule, the model cards and Run now, `BudgetCard` with the spend bar, `MappingCard`; `lib/settings/ai.ts`: partial PUTs, budget input, projection, schedule help), Quality profile, Playback, System status (`status-model.ts`, Jackett's indexers)
- `src/lib/api/contract/`, `src/lib/api/endpoints/` - PLAN §6 types and typed TanStack Query calls (browser side)
- `src/lib/api/backend-proxy.ts` + `src/app/api/[...path]/` - Session-checked proxy to the backend
- `src/lib/api/` - Server-side clients (momoshtrem, tmdb)
- `src/mocks/` - MSW handlers and fixtures for every §6 endpoint (mock mode, Jest, Playwright)
- `src/lib/status.ts` - The one status vocabulary
- `e2e/` - Playwright specs (390 phone + 1440 desktop)

## Environment Variables

```env
TMDB_API_KEY=...              # themoviedb.org
JACKETT_API_KEY=...           # From Jackett dashboard; momoshtrem uses it for torrent search (JACKETT_URL is set in compose)
APP_USERNAME=...              # Sign-in username (defaults to admin)
APP_PASSWORD=...              # Sign-in password
OPENSUBTITLES_API_KEY=...     # Optional: subtitles
HDREZKA_ENABLED=...           # Optional: HDRezka as a second source (default false)
HDREZKA_URL=...               # Optional: HDRezka mirror (default https://rezka.ag)
DATABASE_URL=...              # Postgres (momoshtrem)
TORRENT_LISTEN_PORT=...       # Optional: the torrent client's port (blank keeps 42069; 0 picks a free one)
TRAKT_CLIENT_ID=...           # Optional: Trakt, read-only (an API app, redirect URI urn:ietf:wg:oauth:2.0:oob); TRAKT_CLIENT_SECRET is optional, new apps get none
SECRET_KEY=...                # Encrypts Trakt's tokens at rest (openssl rand -hex 32); Trakt stays off without it, and changing it means connecting again
ANTHROPIC_API_KEY=...         # Optional: the AI features (picks, Ask, the episode-mapping AI); without it they're off and /recommendations/home says configured: false
TZ=...                        # Optional: momoshtrem's zone, an IANA name (compose passes UTC when unset): the 03:00 picks run, the hero's day, Ask's daily limit and the budget month
RECS_DEBUG=...                # Dev only: true serves GET /api/recommendations/debug/profile; keep it off in production
```

v2 adds more (backend token, CORS, the WebDAV public URL); see PLAN.md §4.

Keep in mind that you're not working in the actual server but rather in a local development environment.
