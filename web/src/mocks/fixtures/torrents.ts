import type {
  PieceRun,
  StreamingSession,
  TorrentDetail,
  TorrentDetailFile,
  TorrentListItem,
  TorrentsSummary,
  TorrentState,
} from '@/lib/api/contract';
import { legacyTorrentStatus, type TorrentStatus } from '@/types/torrent';
import { iso } from './catalog';
import { episodeIdFor, SHOWS } from './title-seeds';

const SEVERANCE_HASH = 'b2c3d4e5f60718293a4b5c6d7e8f9012345678a1';
const SEVERANCE_RELEASE = 'Severance.S02.2160p.ATVP.WEB-DL.DV.H.265';
const SEVERANCE_EPISODES = ['Hello, Ms. Cobel', 'Goodbye, Mrs. Selvig', 'Who Is Alive?', 'Woe’s Hollow', 'Trojan’s Horse', 'Attila',
  'Chikhai Bardo', 'Sweet Vitriol', 'The After Hours', 'Cold Harbor'];
const SEVERANCE_EPISODE_BYTES = 4_830_000_000;
const FRIEREN_BATCH_HASH = 'b7a3f1e9c2d4058a6e1f3c5b7d9a2e4f6081c3d5';
const FRIEREN_BATCH = '[SubsPlease] Sousou no Frieren (29-38) (1080p) [Batch]';

/**
 * Infuse on the Apple TV playing Severance S02E05, 18:42 of 52:00. The torrent is
 * complete, so the rest of the file is buffered ahead.
 */
export const STREAMING_SESSION: StreamingSession = {
  id: '7',
  source: 'torrent',
  info_hash: SEVERANCE_HASH,
  item: { media_type: 'tv', tmdb_id: 95396, library_id: 4, title: 'Severance', season_number: 2, episode_number: 5, episode_name: 'Trojan’s Horse' },
  file_name: 'Severance - S02E05 - Trojan’s Horse.mkv',
  file_path: `${SEVERANCE_RELEASE}/Severance.S02E05.Trojans.Horse.2160p.ATVP.WEB-DL.DV.H.265.mkv`,
  client: 'Infuse',
  device: 'Apple TV',
  offset_bytes: 1_736_942_308,
  size_bytes: SEVERANCE_EPISODE_BYTES,
  position_seconds: 1122,
  duration_seconds: 3120,
  rate_bps: 9_100_000,
  buffered_ahead_bytes: SEVERANCE_EPISODE_BYTES - 1_736_942_308,
  buffered_ahead_seconds: 3120 - 1122,
  started_at: '2026-10-14T20:41:30Z',
  last_read_at: '2026-10-14T21:00:12Z',
};

/** Infuse on an iPhone playing Doctor Who S01E01 from HDRezka: no torrent, so no buffer; it read a minute earlier. */
export const HDREZKA_SESSION: StreamingSession = {
  id: '6',
  source: 'hdrezka',
  info_hash: null,
  item: { media_type: 'tv', tmdb_id: 57243, library_id: 3, title: 'Doctor Who', season_number: 1, episode_number: 1, episode_name: 'Rose' },
  file_name: 'Doctor Who - S01E01 - Rose.mp4',
  file_path: null,
  client: 'Infuse',
  device: 'iPhone',
  offset_bytes: 412_000_000,
  size_bytes: 1_236_000_000,
  position_seconds: 900,
  duration_seconds: 2700,
  rate_bps: 2_400_000,
  buffered_ahead_bytes: null,
  buffered_ahead_seconds: null,
  started_at: '2026-10-14T20:44:02Z',
  last_read_at: '2026-10-14T20:59:10Z',
};

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
    info_hash: SEVERANCE_HASH,
    name: SEVERANCE_RELEASE,
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
    streaming: [STREAMING_SESSION],
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
    info_hash: FRIEREN_BATCH_HASH,
    name: FRIEREN_BATCH,
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

/**
 * GET /torrents/summary over `items` (the mock's current torrents). Its streaming is every
 * torrent's sessions plus the HDRezka one, the most recently read first: "Streaming now"
 * reads streaming[0].
 */
export function torrentsSummary(items: TorrentListItem[]): TorrentsSummary {
  const counts: Record<TorrentState, number> = { streaming: 0, downloading: 0, paused: 0, idle: 0 };
  for (const t of items) counts[t.state]++;
  const streaming = [...items.flatMap((t) => t.streaming), HDREZKA_SESSION].sort((a, b) => b.last_read_at.localeCompare(a.last_read_at));
  return {
    download_bps: items.reduce((sum, t) => sum + t.download_bps, 0),
    upload_bps: items.reduce((sum, t) => sum + t.upload_bps, 0),
    peers: items.reduce((sum, t) => sum + (t.peers ?? 0), 0),
    history: { interval_seconds: 2, download_bps: HISTORY_DOWN, upload_bps: HISTORY_UP },
    counts,
    disk: { total_bytes: 2_000_000_000_000, free_bytes: 812_000_000_000, torrents_bytes: 3_100_000_000, cache_limit_bytes: 4_294_967_296 },
    streaming,
  };
}

const MiB = 1 << 20;
const dotted = (name: string) => name.replace(/[’'?,.]/g, '').replace(/\s+/g, '.');
const trackers = (...urls: string[]) =>
  urls.map((url) => ({ url, status: 'unknown' as const, error: null, seeders: null, peers: null, last_announce_at: null }));
const ignored = (reason: string, label: string) => ({ reason, label });

/** The files of the torrents that hold more than one; the others hold one video, named after the torrent. */
function filesOf(t: TorrentListItem): TorrentDetailFile[] {
  const progress = t.loaded ? (t.progress ?? 0) : null;
  if (t.info_hash === SEVERANCE_HASH) {
    return SEVERANCE_EPISODES.map((name, i) => ({
      path: `${SEVERANCE_RELEASE}/Severance.S02E${String(i + 1).padStart(2, '0')}.${dotted(name)}.2160p.ATVP.WEB-DL.DV.H.265.mkv`,
      size_bytes: SEVERANCE_EPISODE_BYTES,
      progress,
      plays_as: [{ media_type: 'tv', item_id: episodeIdFor(SHOWS[95396].key, 2, i + 1), season_number: 2, episode_number: i + 1, episode_name: name,
        label: `S02E${String(i + 1).padStart(2, '0')}` }],
      ignored: null,
    }));
  }
  if (t.info_hash === FRIEREN_BATCH_HASH) {
    // Not loaded and not applied yet (job 101 is in review): the job's list, nothing plays yet.
    const file = (label: string | number) => `${FRIEREN_BATCH}/[SubsPlease] Sousou no Frieren - ${label} (1080p).mkv`;
    return [
      ...[29, 30, 31, 32, 33, 34, 35, 36, 37, 38].map((n) => ({ path: file(n), size_bytes: 1_320_000_000, progress, plays_as: [], ignored: null })),
      { path: file('NCOP2'), size_bytes: 142 * MiB, progress, plays_as: [], ignored: ignored('opening', 'Ignored (NCOP)') },
      { path: file('NCED2'), size_bytes: 139 * MiB, progress, plays_as: [], ignored: ignored('ending', 'Ignored (NCED)') },
      { path: `${FRIEREN_BATCH}/Fonts/OpenSans-Semibold.ttf`, size_bytes: 220_000, progress, plays_as: [], ignored: ignored('font', 'Ignored (font)') },
    ];
  }
  const title = t.titles[0];
  return [{
    path: `${t.name}/${t.name}.mkv`,
    size_bytes: t.size_bytes ?? 0,
    progress,
    plays_as: title?.media_type === 'movie'
      ? [{ media_type: 'movie', item_id: title.library_id, season_number: null, episode_number: null, episode_name: null, label: 'Movie' }]
      : [],
    ignored: null,
  }];
}

/** A loaded torrent's piece map: what's on disk, then (fetching) the streaming window, then nothing wanted. */
function piecesOf(t: TorrentListItem, total: number): PieceRun[] {
  const have = Math.round(total * (t.progress ?? 0));
  const runs: PieceRun[] = [{ state: 'have', count: have }];
  if (t.state === 'downloading' && have < total) {
    const partial = Math.min(2, total - have);
    const priority = Math.min(24, total - have - partial);
    runs.push({ state: 'partial', count: partial }, { state: 'priority', count: priority });
  }
  const rest = total - runs.reduce((sum, r) => sum + r.count, 0);
  if (rest > 0) runs.push({ state: 'missing', count: rest });
  return runs.filter((r) => r.count > 0);
}

/**
 * GET /torrents/{hash} for a mock torrent, from its current state. A loaded torrent
 * has its piece map, progress and totals; one that isn't has `pieces: null` and null
 * progress, with piece facts from its stored metainfo (the Frieren batch has none
 * stored: just the job's file list and the magnet's tracker).
 */
export function torrentDetail(t: TorrentListItem): TorrentDetail {
  const pieceLength = t.info_hash === FRIEREN_BATCH_HASH ? null : 16 * MiB;
  const total = pieceLength && t.size_bytes ? Math.ceil(t.size_bytes / pieceLength) : null;
  const pieces = t.loaded && total ? piecesOf(t, total) : null;
  const downloaded = t.loaded ? (t.bytes_completed ?? 0) : null;
  const uploaded = t.loaded ? Math.round((t.bytes_completed ?? 0) * 0.12) : null;
  return {
    ...t,
    magnet_uri: `magnet:?xt=urn:btih:${t.info_hash}&dn=${encodeURIComponent(t.name)}`,
    piece_length: pieceLength,
    pieces_total: total,
    pieces_have: pieces ? pieces.filter((r) => r.state === 'have').reduce((sum, r) => sum + r.count, 0) : null,
    pieces,
    files: filesOf(t),
    trackers: t.info_hash === FRIEREN_BATCH_HASH
      ? trackers('http://nyaa.tracker.wf:7777/announce')
      : trackers('udp://tracker.opentrackr.org:1337/announce', 'udp://open.stealth.si:80/announce'),
    downloaded_bytes: downloaded,
    uploaded_bytes: uploaded,
    ratio: downloaded ? (uploaded ?? 0) / downloaded : null,
    swarm: t.loaded ? { connected_peers: t.peers ?? 0, connected_seeders: t.seeders ?? 0, total_seeders: null } : null,
  };
}

export const TORRENTS_SUMMARY: TorrentsSummary = torrentsSummary(TORRENTS);

/** The legacy web route's shape (TorrentStatus), as lib/api/momoshtrem.ts adapts the items, until #102 moves to TORRENTS. */
export const LEGACY_TORRENTS: TorrentStatus[] = TORRENTS.map(legacyTorrentStatus);
