# momoshtrem

A library-first media streaming service inspired by distribyted.

## Core Concept

Unlike distribyted (which builds VFS from torrents), momoshtrem builds VFS from a library database. Torrents are mapped TO library items, not the other way around.

```
Traditional:  Torrent → VFS → WebDAV
momoshtrem:   Library (SQLite) → VFS → Torrent files → WebDAV
```

## Design Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Language | Go | Reuse anacrolix/torrent, familiar patterns |
| Database | SQLite | Relational queries, single file, portable |
| VFS approach | Library-first | Structure driven by user's library, not torrent contents |
| Metadata | On-demand from TMDB | Keep DB minimal, fetch when needed |

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
│                   Library (SQLite)                       │
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
