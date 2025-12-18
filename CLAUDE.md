# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Apple TV Torrent Streaming Stack - A self-hosted media streaming service combining:
- **distribyted** (Go): Torrent client exposing content via WebDAV for streaming
- **Jackett**: Torrent indexer aggregator
- **PrettyTVCatalog** (Next.js): Frontend for browsing TMDB, searching torrents, managing library

## Commands

```bash
# Start all services
docker-compose up -d

# Build and start with rebuild
docker-compose up -d --build

# View logs
docker-compose logs -f [service]  # jackett, distribyted, prettytvcatalog

# Stop services
docker-compose down
```

### PrettyTVCatalog (when implemented)
```bash
cd PrettyTVCatalog
npm install
npm run dev          # Development server on :3000
npm run build        # Production build
npm run lint         # ESLint
```

## Architecture

```
Browser/AppleTV ─► PrettyTVCatalog (:3000) ─► distribyted (:4444 API, :36911 WebDAV)
                        │
            ┌───────────┴───────────┐
            ▼                       ▼
         TMDB API              Jackett (:9117)
```

**Flow**: Browse TMDB → Search torrents via Jackett → Add magnet to distribyted → Stream via WebDAV

## Key APIs

### distribyted REST API (port 4444)
```
POST /api/routes/{route}/torrent    # Add magnet: {"magnet": "magnet:?xt=..."}
DELETE /api/routes/{route}/torrent/{hash}
GET /api/routes                     # List routes and torrents
GET /api/status                     # Status info
```

### Jackett Torznab API (port 9117)
```
GET /api/v2.0/indexers/all/results/torznab/?apikey={key}&t=search&q={query}
```
Returns XML - parse for title, size, seeders, magnet URI.

## Documentation References

| Topic | Location |
|-------|----------|
| Frontend style guide | `docs/prettytvcatalog/STYLE_GUIDE.md` |
| ALWAYS follow code quality standards | `docs/prettytvcatalog/CODE_QUALITY.md` |
| Stage 1 implementation tasks | `docs/prettytvcatalog/STAGE_1_TASKS.md` |
| distribyted architecture | `docs/distribyted-architecture.md` |
| Deployment guide | `proxmox-installation.md` |

## Environment Variables

```env
APP_PASSWORD=...           # Frontend auth
TMDB_API_KEY=...          # themoviedb.org API key
JACKETT_API_KEY=...       # From Jackett dashboard
DISTRIBYTED_URL=http://distribyted:4444
DISTRIBYTED_ROUTE=media   # Single route for all content
```

## Key Source Locations

- `distribyted/http/api.go` - REST API endpoints
- `distribyted/torrent/service.go` - Torrent management
- `distribyted/webdav/` - WebDAV server
- `distribyted/config/model.go` - Configuration structure
