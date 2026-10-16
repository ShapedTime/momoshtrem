import type { TorrentListItem, TorrentsSummary, TorrentState } from '@/lib/api/contract';
import { legacyTorrentStatus, type TorrentStatus } from '@/types/torrent';
import { iso } from './catalog';

/** Fields a torrent the client hasn't loaded reports as null or 0. */
const NOT_LOADED = {
  loaded: false,
  complete: null,
  progress: null,
  bytes_completed: null,
  download_bps: 0,
  upload_bps: 0,
  peers: null,
  seeders: null,
  last_active_at: null,
} as const;

/**
 * GET /torrents (PLAN §6 "Torrents and activity (B6)"), in the order the
 * backend sorts them: one torrent per state, plus idle ones that are loaded
 * and complete, loaded with nothing referencing them (an Inspect dry run),
 * known only to an open mapping job, and not loaded at all.
 */
export const TORRENTS: TorrentListItem[] = [
  {
    info_hash: 'b2c3d4e5f60718293a4b5c6d7e8f9012345678a1',
    name: 'Severance.S02.2160p.ATVP.WEB-DL.DV.H.265',
    state: 'streaming',
    loaded: true,
    complete: true,
    progress: 1,
    bytes_completed: 48_300_000_000,
    size_bytes: 48_300_000_000,
    download_bps: 0,
    upload_bps: 1_200_000,
    peers: 12,
    seeders: 9,
    added_at: iso(4),
    last_active_at: iso(0),
    titles: [{ media_type: 'tv', tmdb_id: 95396, library_id: 4, title: 'Severance', year: 2022, poster_path: null, scope: 'Season 2' }],
    open_job: null,
    streaming: [],
  },
  {
    info_hash: 'a1b2c3d4e5f60718293a4b5c6d7e8f9012345678',
    name: 'Frieren.S02.1080p.WEB-DL.x264-SubsPlease',
    state: 'downloading',
    loaded: true,
    complete: false,
    progress: 0.64,
    bytes_completed: 8_064_000_000,
    size_bytes: 12_600_000_000,
    download_bps: 6_400_000,
    upload_bps: 420_000,
    peers: 48,
    seeders: 31,
    added_at: iso(0),
    last_active_at: iso(0),
    titles: [
      { media_type: 'tv', tmdb_id: 209867, library_id: 1, title: 'Frieren: Beyond Journey’s End', year: 2023, poster_path: null, scope: 'S02E01–E04' },
    ],
    open_job: null,
    streaming: [],
  },
  {
    info_hash: '5d6e7f8091a2b3c4d5e6f708192a3b4c5d6e7f80',
    name: 'One.Piece.1086-1100.1080p.WEB.x264-GROUP',
    state: 'paused',
    loaded: true,
    complete: false,
    progress: 0.31,
    bytes_completed: 1_860_000_000,
    size_bytes: 6_000_000_000,
    download_bps: 0,
    upload_bps: 0,
    peers: 0,
    seeders: 0,
    added_at: iso(6),
    last_active_at: iso(1),
    titles: [{ media_type: 'tv', tmdb_id: 37854, library_id: 2, title: 'One Piece', year: 1999, poster_path: null, scope: 'S22E1086–E1100' }],
    open_job: null,
    streaming: [],
  },
  {
    info_hash: 'f0e1d2c3b4a5968778695a4b3c2d1e0f1a2b3c4d',
    name: 'Past.Lives.2023.1080p.BluRay.x265',
    state: 'idle',
    loaded: true,
    complete: false,
    progress: 0,
    bytes_completed: 0,
    size_bytes: 6_100_000_000,
    download_bps: 0,
    upload_bps: 0,
    peers: 0,
    seeders: 0,
    added_at: iso(0),
    last_active_at: iso(0),
    titles: [],
    open_job: null,
    streaming: [],
  },
  {
    info_hash: 'c9d4a7b3e5f60718293a4b5c6d7e8f90a1b2c3d4',
    name: 'Interstellar.2014.2160p.UHD.BluRay.REMUX.HDR.HEVC.DTS-HD.MA.5.1-FGT',
    state: 'idle',
    loaded: true,
    complete: true,
    progress: 1,
    bytes_completed: 62_400_000_000,
    size_bytes: 62_400_000_000,
    download_bps: 0,
    upload_bps: 0,
    peers: 0,
    seeders: 0,
    added_at: iso(1),
    last_active_at: iso(1),
    titles: [{ media_type: 'movie', tmdb_id: 157336, library_id: 2, title: 'Interstellar', year: 2014, poster_path: null, scope: null }],
    open_job: null,
    streaming: [],
  },
  {
    ...NOT_LOADED,
    info_hash: 'b7a3f1e9c2d4058a6e1f3c5b7d9a2e4f6081c3d5',
    name: '[SubsPlease] Sousou no Frieren (29-38) (1080p) [Batch]',
    state: 'idle',
    size_bytes: 14_200_000_000,
    added_at: iso(2),
    titles: [{ media_type: 'tv', tmdb_id: 209867, library_id: 1, title: 'Frieren: Beyond Journey’s End', year: 2023, poster_path: null, scope: null }],
    open_job: { id: 101, status: 'needs_review' },
    streaming: [],
  },
  {
    ...NOT_LOADED,
    info_hash: '4f1a2b3c4d5e6f708192a3b4c5d6e7f8091abe90',
    name: 'Dune.Part.Two.2024.2160p.WEB-DL.DDP5.1.Atmos.DV.HDR.H.265-FLUX',
    state: 'idle',
    size_bytes: 18_400_000_000,
    added_at: iso(12),
    titles: [{ media_type: 'movie', tmdb_id: 693134, library_id: 1, title: 'Dune: Part Two', year: 2024, poster_path: null, scope: null }],
    open_job: null,
    streaming: [],
  },
];

/** A minute of totals, oldest first, every 2 s: a download ramping up. */
const HISTORY_DOWN = Array.from({ length: 30 }, (_, i) => Math.round(6_400_000 * Math.min(1, (i + 1) / 20)));
const HISTORY_UP = Array.from({ length: 30 }, (_, i) => Math.round(1_620_000 * Math.min(1, (i + 4) / 24)));

/** GET /torrents/summary over `items` (the mock's current torrents). */
export function torrentsSummary(items: TorrentListItem[]): TorrentsSummary {
  const counts: Record<TorrentState, number> = { streaming: 0, downloading: 0, paused: 0, idle: 0 };
  for (const t of items) counts[t.state]++;
  return {
    download_bps: items.reduce((sum, t) => sum + t.download_bps, 0),
    upload_bps: items.reduce((sum, t) => sum + t.upload_bps, 0),
    peers: items.reduce((sum, t) => sum + (t.peers ?? 0), 0),
    history: { interval_seconds: 2, download_bps: HISTORY_DOWN, upload_bps: HISTORY_UP },
    counts,
    disk: { total_bytes: 2_000_000_000_000, free_bytes: 812_000_000_000, torrents_bytes: 3_100_000_000, cache_limit_bytes: 4_294_967_296 },
    streaming: [],
  };
}

export const TORRENTS_SUMMARY: TorrentsSummary = torrentsSummary(TORRENTS);

/** The legacy web route's shape (TorrentStatus), as lib/api/momoshtrem.ts adapts the items, until #102 moves to TORRENTS. */
export const LEGACY_TORRENTS: TorrentStatus[] = TORRENTS.map(legacyTorrentStatus);
