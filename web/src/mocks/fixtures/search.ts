import type { Release, ReleaseAttributes, ReleasePack, SearchTorrentsResponse } from '@/lib/api/contract';
import { iso } from './catalog';

function release(i: number, raw: string, attrs: Partial<ReleaseAttributes> = {}, partial: Partial<Release> = {}): Release {
  const seeders = Math.max(0, 320 - i * 37);
  return {
    id: `rel-${i}`,
    raw_title: raw,
    display_title: raw.replace(/[._]/g, ' '),
    info_hash: `${i}`.padStart(40, 'a'),
    magnet_uri: `magnet:?xt=urn:btih:${`${i}`.padStart(40, 'a')}`,
    size_bytes: 1_400_000_000 + i * 120_000_000,
    seeders,
    leechers: 12,
    published_at: iso(i),
    indexer: i % 2 ? 'Nyaa' : 'TorrentLeech',
    attrs: {
      resolution: '1080p',
      source: 'WEB-DL',
      codec: 'x264',
      bit_depth: null,
      hdr: [],
      audio: ['AAC'],
      channels: '2.0',
      languages: ['ja'],
      subs: 'multi',
      group: 'SubsPlease',
      edition: null,
      repack: false,
      pack: { type: 'episode', label: 'S01E29', seasons: [1], episodes: { from: 29, to: 29 } },
      ...attrs,
    },
    score: 96 - i * 6,
    score_reasons: ['matches your quality profile', `${seeders} seeders`, 'exact episode'],
    notes: [],
    warnings: [],
    flags: [],
    ...partial,
  };
}

// Absolute numbering: no season in the name
const absolute29: ReleasePack = { type: 'episode', label: '29', seasons: [], episodes: { from: 29, to: 29 } };

const RELEASES: Release[] = [
  release(0, 'Frieren.Beyond.Journeys.End.S01E29.2160p.WEB-DL.DV.H.265-SubsPlease', { resolution: '2160p', codec: 'x265', bit_depth: 10, hdr: ['DV'] }),
  release(1, '[SubsPlease] Sousou no Frieren - 29 (1080p) [A1B2C3D4].mkv', { codec: 'unknown', pack: absolute29 }),
  release(
    2,
    'Frieren.S02E01.1080p.WEB.H264-VARYG',
    { source: 'WEB', group: 'VARYG', pack: { type: 'episode', label: 'S02E01', seasons: [2], episodes: { from: 1, to: 1 } } },
    { notes: ['Scene "Season 2" = TMDB S01E29–E38'] },
  ),
  release(3, 'Frieren.Beyond.Journeys.End.S01.Part.2.1080p.BluRay.x265-Judas', {
    source: 'BluRay',
    codec: 'x265',
    group: 'Judas',
    pack: { type: 'season', label: 'Season 1', seasons: [1], episodes: null },
  }),
  release(4, '[Erai-raws] Sousou no Frieren - 29 [720p][Multiple Subtitle].mkv', { resolution: '720p', codec: 'unknown', group: 'Erai-raws', pack: absolute29 }),
  release(
    5,
    'Frieren.Complete.Series.1080p.WEB-DL',
    { group: null, pack: { type: 'complete', label: 'Complete series', seasons: [1, 2], episodes: null } },
    { notes: ['Covers both seasons, 38 episodes'] },
  ),
  release(6, 'Frieren.S01E29.CAM.x264', { resolution: 'unknown', source: 'CAM', group: null }, { flags: ['cam'], warnings: ['CAM recording · filmed in a cinema'] }),
  release(7, 'Frieren.S01E29.480p.WEB.x264', { resolution: '480p', source: 'WEB', group: null }),
  release(8, 'Frieren.S01E29.1080p.HDTV.x264-OLD', { source: 'HDTV', group: 'OLD' }, { seeders: 0, flags: ['no_seeders'], warnings: ['No seeders · it may never start'] }),
];

/** Every release, unfiltered: the context-aware mock is #92's. */
export function searchTorrents(page = 1, pageSize = 25): SearchTorrentsResponse {
  const start = (page - 1) * pageSize;
  return {
    items: RELEASES.slice(start, start + pageSize),
    total: RELEASES.length,
    page,
    page_size: pageSize,
    total_unfiltered: RELEASES.length,
    hidden: 0,
    best_id: 'rel-0',
    facets: {
      resolution: [
        { value: '2160p', count: 1 },
        { value: '1080p', count: 5 },
        { value: '720p', count: 1 },
        { value: '480p', count: 1 },
        { value: 'unknown', count: 1 },
      ],
      source: [
        { value: 'BluRay', count: 1 },
        { value: 'WEB-DL', count: 4 },
        { value: 'WEB', count: 2 },
        { value: 'HDTV', count: 1 },
        { value: 'CAM', count: 1 },
      ],
      codec: [
        { value: 'x265', count: 2 },
        { value: 'x264', count: 5 },
        { value: 'unknown', count: 2 },
      ],
      hdr: [
        { value: 'DV', count: 1 },
        { value: 'SDR', count: 8 },
      ],
      audio: [{ value: 'AAC', count: 9 }],
      pack: [
        { value: 'episode', count: 7 },
        { value: 'season', count: 1 },
        { value: 'complete', count: 1 },
      ],
      indexer: [
        { value: 'TorrentLeech', count: 5 },
        { value: 'Nyaa', count: 4 },
      ],
      group: [
        { value: 'SubsPlease', count: 2 },
        { value: 'Erai-raws', count: 1 },
        { value: 'Judas', count: 1 },
        { value: 'OLD', count: 1 },
        { value: 'VARYG', count: 1 },
      ],
    },
    queries: [
      { q: 'Frieren 29', label: null },
      { q: 'Sousou no Frieren 29', label: 'original title' },
      { q: 'Frieren S02E01', label: 'scene numbering' },
    ],
    indexers: [
      { name: 'Nyaa', status: 'ok', count: 4, ms: 210 },
      { name: 'TorrentLeech', status: 'ok', count: 5, ms: 340 },
      { name: 'The Pirate Bay', status: 'timeout', count: 0, ms: 8000, error: 'No answer in 8 s' },
    ],
    complete: true,
    missing: null,
    cached_at: iso(0),
    warnings: [],
  };
}
