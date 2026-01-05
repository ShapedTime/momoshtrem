# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Self-hosted media streaming stack for Apple TV with library management:
- **momoshtrem** (Go): Library-first torrent streaming service with WebDAV
- **PrettyTVCatalog** (Next.js): Frontend for browsing TMDB, managing library, searching torrents
- **Jackett**: Torrent indexer aggregator

## Commands

```bash
# Start all services
docker-compose up -d

# Rebuild and start
docker-compose up -d --build

# Frontend development
cd PrettyTVCatalog && npm run dev

# Go service (momoshtrem)
cd momoshtrem && go build -o momoshtrem ./cmd/momoshtrem
```

## Architecture

```
                    ┌─────────────────────────────────────────┐
                    │          PrettyTVCatalog (:3000)        │
                    │   Browse TMDB, manage library, search   │
                    └──────────────────┬──────────────────────┘
                                       │
               ┌───────────────────────┼───────────────────────┐
               ▼                       ▼                       ▼
          TMDB API              Jackett (:9117)        momoshtrem (:4444)
                                                              │
                                                              ▼
                                                       SQLite Library
                                                              │
AppleTV/Infuse ─────────────────────────────────► WebDAV (:36911)
                                                              │
                                                              ▼
                                                       OpenSubtitles API
```

**Flow**: Browse TMDB → Add to library → Search torrents via Jackett → Assign torrent → Stream via WebDAV (Infuse)

**Key design**: momoshtrem is library-first. The VFS structure is driven by the SQLite database (movies, shows, episodes), not by torrent contents.

## Key APIs

### momoshtrem REST API (port 4444)
```
POST /api/movies              # Add movie by TMDB ID
POST /api/shows               # Add show by TMDB ID
POST /api/movies/{id}/assign-torrent   # Assign torrent to movie
POST /api/shows/{id}/assign-torrent    # Auto-detect episodes from torrent
GET  /api/torrents            # List active torrents
POST /api/subtitles/search    # Search OpenSubtitles
```

### Jackett Torznab API (port 9117)
```
GET /api/v2.0/indexers/all/results/torznab/?apikey={key}&t=search&q={query}
```

## Code Quality

**ALWAYS follow these standards:**
- `docs/prettytvcatalog/CODE_QUALITY.md` - Architecture, TypeScript, error handling
- `docs/prettytvcatalog/STYLE_GUIDE.md` - UI patterns, responsive design
- `docs/momoshtrem/CODE_QUALITY.md` - Go patterns, interfaces, concurrency

**Key principles:**
- API routes are thin orchestration; business logic lives in `lib/api/`
- TypeScript strict mode, avoid `any`, use type guards
- Custom error classes: `APIError`, `NotFoundError`, `ValidationError`
- Conventional commits: `feat:`, `fix:`, `refactor:`, `chore:`

## Key Source Locations

**momoshtrem (Go):**
- `cmd/momoshtrem/main.go` - Entry point
- `internal/api/router.go` - REST API endpoints
- `internal/vfs/library_fs.go` - Virtual filesystem
- `internal/torrent/service.go` - Torrent service interface
- `internal/streaming/prioritizer.go` - Piece prioritization

**PrettyTVCatalog (Next.js):**
- `src/lib/api/` - API clients (momoshtrem, jackett, tmdb)
- `src/app/api/` - Next.js API routes
- `src/types/` - TypeScript interfaces

## Environment Variables

```env
TMDB_API_KEY=...              # themoviedb.org
JACKETT_API_KEY=...           # From Jackett dashboard
APP_PASSWORD=...              # Frontend auth
OPENSUBTITLES_API_KEY=...     # Optional: subtitles
```
