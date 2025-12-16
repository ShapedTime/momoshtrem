# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Code Quality Standards

ALWAYS follow best practices: SOLID principles, DRY, clean code. Maintain high code quality throughout all components.

## Project Overview

AppleTV torrent streaming service with three components:
- **PrettyTVCatalog** - Next.js 16 media catalog UI (port 3000)
- **stremio-torrent-stream** - Torrent search via Jackett and public trackers (ports 58827/58828)
- **distribyted** - Go torrent client exposing files via WebDAV for AppleTV playback (port 4444)

**User flow**: Browse media in PrettyTVCatalog → Search for torrents → Add to distribyted → Stream via WebDAV to AppleTV

## Commands

### PrettyTVCatalog (Next.js)
```bash
cd PrettyTVCatalog
npm install && npm run dev     # Development (localhost:3000)
npm run build && npm run start # Production
npm run lint                   # ESLint
```

### stremio-torrent-stream (Node.js/pnpm)
```bash
cd stremio-torrent-stream
pnpm i && pnpm dev    # Development with watch
pnpm build && pnpm start  # Production
```

### distribyted (Go)
```bash
cd distribyted
make build    # Build binary
make run      # Run with example config
make test     # Run tests
go test -v -run TestName ./package/  # Single test
```
System deps: Linux (`fuse`, `libfuse-dev`), macOS (`macfuse`)

### Docker (Full Stack)
```bash
docker compose up --build
```

## Architecture

```
┌──────────────────┐     ┌──────────────────────┐     ┌─────────────┐
│  PrettyTVCatalog │────▶│ stremio-torrent-     │────▶│  distribyted │
│  (Catalog UI)    │     │ stream (Search API)  │     │  (WebDAV)    │
│  :3000           │     │  :58827              │     │  :4444       │
└──────────────────┘     └──────────────────────┘     └──────┬───────┘
                                                             │ WebDAV
                                                      ┌──────▼───────┐
                                                      │   AppleTV    │
                                                      └──────────────┘
```

### Key APIs

**stremio-torrent-stream**:
- `GET /torrents/:query` - Search all sources
- `POST /torrents/:query` - Search with options (categories, sources, credentials)
- `GET /torrent/:torrentUri` - Get torrent info/files
- `GET /stream/:torrentUri/:filePath` - Stream file
- Returns: `{ name, tracker, size, seeds, peers, magnet }`

**distribyted**:
- `POST /api/routes/:route/torrent` - Add torrent: `{ "magnet": "magnet:?..." }`
- `DELETE /api/routes/:route/torrent/:hash` - Remove torrent
- `GET /api/routes` - List routes with stats
- WebDAV endpoint at configured port

### PrettyTVCatalog Structure

```
src/
├── app/                    # Next.js App Router
│   ├── api/               # API routes (library, torrents, settings)
│   ├── movies/, shows/    # Browse + detail pages
│   └── library/, settings/
├── components/
│   ├── media/             # MediaCard, MediaHero, EpisodeCard
│   ├── torrent/           # TorrentSearchModal, TorrentButton
│   └── ui/                # SearchBar, FilterDropdown, Pagination
├── lib/
│   ├── tmdb.ts            # TMDB API client
│   ├── stremio-client.ts  # stremio-torrent-stream client
│   ├── distribyted-client.ts  # distribyted API client
│   └── db/                # SQLite for library/torrents
└── types/                 # TypeScript types
```

### Torrent Search Sources

stremio-torrent-stream supports: `jackett` (aggregate indexer at :9117), `yts` (movies), `eztv` (TV), `itorrent`, `ncore`, `insane` (private trackers)

## Configuration

**PrettyTVCatalog** (`.env.local`):
```
TMDB_API_KEY=your_key
STREMIO_API_URL=http://localhost:58827
DISTRIBYTED_API_URL=http://localhost:4444
DISTRIBYTED_DEFAULT_ROUTE=default
```

**distribyted**: Config at `./distribyted-data/config/config.yaml`

**Jackett**: API key from http://localhost:9117

## Data Storage

- **PrettyTVCatalog**: SQLite (`data/catalog.db`) for library and torrent state
- **distribyted**: Badger DB for dynamic torrents, BoltDB for piece completion
- **stremio-torrent-stream**: In-memory WebTorrent client
