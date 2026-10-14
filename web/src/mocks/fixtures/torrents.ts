import type { TorrentListItem, TorrentsSummary } from '@/lib/api/contract';
import type { TorrentStatus } from '@/types/torrent';
import { iso } from './catalog';

export const TORRENTS: TorrentListItem[] = [
  {
    info_hash: 'a1b2c3d4e5f60718293a4b5c6d7e8f9012345678',
    name: 'Frieren.S02.1080p.WEB-DL.x264-SubsPlease',
    state: 'downloading',
    progress: 0.64,
    size_bytes: 12_600_000_000,
    download_speed: 6_400_000,
    upload_speed: 420_000,
    peers: 48,
    seeders: 312,
    titles: [{ media_type: 'tv', tmdb_id: 209867, title: 'Frieren: Beyond Journey’s End', scope: 'S01E29–E38' }],
    added_at: iso(0),
  },
  {
    info_hash: 'b2c3d4e5f60718293a4b5c6d7e8f9012345678a1',
    name: 'Severance.S02.2160p.ATVP.WEB-DL.DV.H.265',
    state: 'streaming',
    progress: 1,
    size_bytes: 48_300_000_000,
    download_speed: 0,
    upload_speed: 1_200_000,
    peers: 12,
    seeders: 96,
    titles: [{ media_type: 'tv', tmdb_id: 95396, title: 'Severance', scope: 'Season 2' }],
    added_at: iso(4),
  },
  {
    info_hash: 'c3d4e5f60718293a4b5c6d7e8f9012345678a1b2',
    name: 'Past.Lives.2023.1080p.BluRay.x265',
    state: 'idle',
    progress: 1,
    size_bytes: 6_100_000_000,
    download_speed: 0,
    upload_speed: 0,
    peers: 0,
    seeders: 40,
    titles: [{ media_type: 'movie', tmdb_id: 666277, title: 'Past Lives', scope: null }],
    added_at: iso(20),
  },
];

export const TORRENTS_SUMMARY: TorrentsSummary = {
  down_bps: 6_400_000,
  up_bps: 1_620_000,
  active: 2,
  idle: 1,
  paused: 0,
  disk: { used: 67_000_000_000, free: 812_000_000_000 },
};

/** The legacy web route's shape (TorrentStatus), until Activity (#7) moves to TORRENTS. */
export const LEGACY_TORRENTS: TorrentStatus[] = TORRENTS.map((t) => ({
  info_hash: t.info_hash,
  name: t.name,
  total_size: t.size_bytes,
  downloaded: Math.round(t.size_bytes * t.progress),
  progress: t.progress,
  seeders: t.seeders,
  leechers: Math.max(0, t.peers - t.seeders),
  download_speed: t.download_speed,
  upload_speed: t.upload_speed,
  is_paused: false,
}));
