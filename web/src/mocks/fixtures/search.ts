import type { QualityProfile, Release, SearchTorrentsResponse } from '@/lib/api/contract';
import { iso } from './catalog';

function release(i: number, raw: string, partial: Partial<Release> = {}): Release {
  return {
    id: `rel-${i}`,
    raw_title: raw,
    display_title: raw.replace(/[._]/g, ' '),
    info_hash: `${i}`.padStart(40, 'a'),
    magnet_uri: `magnet:?xt=urn:btih:${`${i}`.padStart(40, 'a')}`,
    size_bytes: 1_400_000_000 + i * 120_000_000,
    seeders: Math.max(0, 320 - i * 37),
    leechers: 12,
    published_at: iso(i),
    indexer: i % 2 ? 'Nyaa' : 'TorrentLeech',
    attrs: {
      resolution: i % 3 ? '1080p' : '2160p',
      source: 'WEB-DL',
      codec: i % 2 ? 'x265' : 'x264',
      hdr: i % 3 ? [] : ['DV'],
      audio: ['AAC 2.0'],
      group: 'SubsPlease',
      languages: ['ja', 'en'],
      edition: null,
      repack: false,
      pack: { type: 'episode', seasons: [1], episodes: [29] },
    },
    score: 96 - i * 6,
    score_reasons: ['matches your quality profile', `${Math.max(0, 320 - i * 37)} seeders`, 'exact episode'],
    flags: i > 7 ? ['no_seeders'] : [],
    ...partial,
  };
}

const RELEASES: Release[] = [
  release(0, 'Frieren.Beyond.Journeys.End.S01E29.2160p.WEB-DL.DV.H.265-SubsPlease'),
  release(1, '[SubsPlease] Sousou no Frieren - 29 (1080p) [A1B2C3D4].mkv'),
  release(2, 'Frieren.S02E01.1080p.WEB.H264-VARYG', { attrs: { ...release(2, '').attrs, pack: { type: 'episode', seasons: [2], episodes: [1] } } }),
  release(3, 'Frieren.Beyond.Journeys.End.S01.Part.2.1080p.BluRay.x265-Judas', { attrs: { ...release(3, '').attrs, pack: { type: 'season', seasons: [1], episodes: [] } } }),
  release(4, '[Erai-raws] Sousou no Frieren - 29 [720p][Multiple Subtitle].mkv'),
  release(5, 'Frieren.Complete.Series.1080p.WEB-DL', { attrs: { ...release(5, '').attrs, pack: { type: 'complete', seasons: [1, 2], episodes: [] } } }),
  release(6, 'Frieren.S01E29.CAM.x264', { flags: ['cam'] }),
  release(7, 'Frieren.S01E29.480p.WEB.x264'),
  release(8, 'Frieren.S01E29.1080p.HDTV.x264-OLD'),
];

export function searchTorrents(page = 1, pageSize = 25): SearchTorrentsResponse {
  const start = (page - 1) * pageSize;
  return {
    items: RELEASES.slice(start, start + pageSize),
    facets: {
      resolution: { '2160p': 3, '1080p': 5, '720p': 1 },
      source: { 'WEB-DL': 6, BluRay: 1, HDTV: 1, CAM: 1 },
      codec: { x264: 5, x265: 4 },
      hdr: { DV: 3 },
      pack: { episode: 6, season: 1, complete: 1 },
      indexer: { Nyaa: 4, TorrentLeech: 5 },
    },
    total: RELEASES.length,
    page,
    page_size: pageSize,
    queries: ['Frieren 29', 'Sousou no Frieren 29', 'Frieren S02E01 (scene numbering)'],
    indexers: [
      { name: 'Nyaa', status: 'ok', count: 4, ms: 210 },
      { name: 'TorrentLeech', status: 'ok', count: 5, ms: 340 },
      { name: 'The Pirate Bay', status: 'timeout', count: 0, ms: 8000 },
    ],
    cached_at: iso(0),
    warnings: ['The Pirate Bay timed out'],
  };
}

export const QUALITY_PROFILE: QualityProfile = {
  preferred_resolution: '2160p',
  min_resolution: '1080p',
  max_resolution: '2160p',
  prefer_codecs: ['x265'],
  prefer_hdr: true,
  max_gb_per_hour: 8,
  avoid_terms: ['CAM', 'TS'],
  preferred_groups: ['SubsPlease'],
};
