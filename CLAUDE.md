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

Labels: `v2` (redesign and features), `ops` (deployment, CI/CD, backups and server setup), `epic` (large, may be split), `in-progress` (claimed), `blocked` (waiting on an issue or the user).

## Project Overview

Self-hosted media streaming stack for Apple TV with library management:
- **momoshtrem** (Go): Library-first torrent streaming service with WebDAV
- **web** (Next.js, package `momoshtrem-web`): the momoshtrem UI for browsing TMDB, managing the library and searching torrents. Lives in `web/`.
- **Jackett**: Torrent indexer aggregator
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

Server setup, CI/CD and backups are a separate track from v2: issues labelled `ops` (`gh issue list --label ops --state open`), epic #32. Read `docs/ops/PLAN.md` before any ops work: decisions (§2), state inventory (§5), rollback model (§6) and the deploy agent contract (§8). The repo is public, so secrets and the public hostname never go in it.

## Commands

```bash
# Start all services (compose.yml)
docker compose up -d

# Rebuild and start
docker compose up -d --build

# Frontend
cd web && npm run dev
cd web && NEXT_PUBLIC_API_MOCKING=enabled npm run dev   # mock mode: MSW answers every PLAN §6 endpoint
cd web && npm run lint && npx tsc --noEmit && npm test
cd web && npx playwright test   # E2E (own dev server on :3100; first run: npx playwright install chromium)

# Go service (momoshtrem)
cd momoshtrem && go build -o momoshtrem ./cmd/momoshtrem
cd momoshtrem && go vet ./... && go test ./...
docker compose run --rm momoshtrem backfill-metadata [--dry-run]   # fill TMDB metadata for titles added before #4 (never renames WebDAV paths)
```

## Architecture

```
                    ┌─────────────────────────────────────────┐
                    │        web: momoshtrem UI (:3000)       │
                    │   Browse TMDB, manage library, search   │
                    └──────────────────┬──────────────────────┘
                                       │
               ┌───────────────────────┼───────────────────────┐
               ▼                       ▼                       ▼
          TMDB API              Jackett (:9117)        momoshtrem (:4444)
                                                              │
                                                              ▼
                                                     Postgres (library)
                                                              │
AppleTV/Infuse ─────────────────────────────────► WebDAV (:36911)
                                                              │
                                                              ▼
                                                       OpenSubtitles API
```

The v2 target architecture is in PLAN.md §4.

**Flow**: Browse TMDB → Add to library → Search torrents via Jackett → Assign torrent → Stream via WebDAV (Infuse)

**Key design**: momoshtrem is library-first. The VFS structure is driven by the database (movies, shows, episodes), not by torrent contents. Migrations are embedded from `momoshtrem/internal/library/migrations/`. The root `momoshtrem/migrations/` folder is a legacy SQLite schema.

## Key APIs

### momoshtrem REST API (port 4444)
```
GET  /api/titles/{movie|tv}/{tmdb_id}       # Title page: TMDB details, library state and availability stage
GET  /api/titles/tv/{tmdb_id}/seasons/{n}   # One season's episodes and ranges
GET  /api/library             # Library list with counts (filters, sort, search)
POST /api/movies              # Add movie by TMDB ID
POST /api/shows               # Add show by TMDB ID
POST /api/movies/{id}/assign-torrent   # Assign torrent to movie
POST /api/shows/{id}/assign-torrent    # Auto-detect episodes from torrent
GET  /api/torrents            # List active torrents
GET  /api/movies/{id}/subtitles/search   # OpenSubtitles, ranked by release (also /episodes/{id})
```
Full list: `momoshtrem/internal/api/router.go`. v2 endpoints: PLAN.md §6.

### Jackett Torznab API (port 9117)
```
GET /api/v2.0/indexers/all/results/torznab/?apikey={key}&t=search&q={query}
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
- `internal/metadata/` - TMDB sync into the library (specials, absolute numbering, anime detection) and `backfill-metadata`
- `internal/tmdb/` - TMDB client with retries, rate limit and the `tmdb_cache` cache
- `internal/artwork/` - Backdrop dominant colour for the hero glow
- `internal/subtitle/` - Subtitles: OpenSubtitles search ranked by release match, downloads, torrent subtitles
- `internal/vfs/library_fs.go` - Virtual filesystem
- `internal/torrent/service.go` - Torrent service interface
- `internal/streaming/prioritizer.go` - Piece prioritization

**web (Next.js, `web/`):**
- `src/components/ui/`, `src/components/brand/` - Design-system primitives, logo and Noir (gallery: `/dev/ui`)
- `src/components/shell/` - App shell: top bar, tab bar, account menu, ⌘K palette, URL-driven sheets
- `src/components/title/` - Movie and show pages: hero, primary panel and action dock, subtitles, episodes (`show/`: season tabs, rows, long-season tools, show settings and episode sheets)
- `src/lib/title/` - Pure title helpers: stage copy (`primaryActionFor`), formats, facts, episodes (`parseJump`, ranges, next-up)
- `src/lib/api/contract/`, `src/lib/api/endpoints/` - PLAN §6 types and typed TanStack Query calls (browser side)
- `src/lib/api/backend-proxy.ts` + `src/app/api/[...path]/` - Session-checked proxy to the backend
- `src/lib/api/` - Server-side clients (momoshtrem, jackett, tmdb)
- `src/mocks/` - MSW handlers and fixtures for every §6 endpoint (mock mode, Jest, Playwright)
- `src/lib/status.ts` - The one status vocabulary
- `e2e/` - Playwright specs (390 phone + 1440 desktop)

## Environment Variables

```env
TMDB_API_KEY=...              # themoviedb.org
JACKETT_API_KEY=...           # From Jackett dashboard
APP_USERNAME=...              # Sign-in username (defaults to admin)
APP_PASSWORD=...              # Sign-in password
OPENSUBTITLES_API_KEY=...     # Optional: subtitles
DATABASE_URL=...              # Postgres (momoshtrem)
```

v2 adds more (Trakt, Anthropic, backend token); see PLAN.md §4.

Keep in mind that you're not working in the actual server but rather in a local development environment.
