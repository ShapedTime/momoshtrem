import { http, HttpResponse } from 'msw';
import type { JackettSearchResponse, TorrentResult, VideoQuality } from '@/types/jackett';
import type { LibraryMovie, LibraryShow, RecentlyAiredResponse } from '@/types/momoshtrem';
import type { GenreList, TrendingResults } from '@/types/tmdb';
import { CATALOG, iso } from '../fixtures/catalog';
import { latency } from './util';

/** Just enough of the legacy web routes for the old pages to render in mock mode. */
const trending: TrendingResults = {
  movies: CATALOG.filter((e) => e.media_type === 'movie').map((e) => ({
    id: e.tmdb_id,
    mediaType: 'movie',
    title: e.title,
    releaseDate: `${e.year}-01-01`,
    posterPath: null,
    backdropPath: null,
    voteAverage: 8.1,
    overview: e.overview,
  })),
  tvShows: CATALOG.filter((e) => e.media_type === 'tv').map((e) => ({
    id: e.tmdb_id,
    mediaType: 'tv',
    name: e.title,
    firstAirDate: `${e.year}-01-01`,
    posterPath: null,
    backdropPath: null,
    voteAverage: 8.6,
    overview: e.overview,
  })),
};

const recentlyAired: RecentlyAiredResponse = { episodes: [], last_sync_time: iso(0), sync_status: 'ok' };

/** 40 hex characters from a string (FNV-1a with five seeds): a stable fake info hash. */
function fakeHash(text: string): string {
  return [0x811c9dc5, 0x01000193, 0x2f8f7f, 0xa0662a, 0x3c4a5c]
    .map((seed) => {
      let hash = seed >>> 0;
      for (let i = 0; i < text.length; i++) hash = Math.imul(hash ^ text.charCodeAt(i), 0x01000193) >>> 0;
      return hash.toString(16).padStart(8, '0');
    })
    .join('');
}

/** Three releases named after any query, best first, as the legacy Jackett route answers. */
function jackettResults(query: string): TorrentResult[] {
  const base = query.trim().replace(/[:'’]/g, '').replace(/\s+/g, '.');
  const releases: Array<[string, number, number, VideoQuality, string]> = [
    [`${base}.2160p.WEB-DL.DDP5.1.Atmos.DV.HDR.H.265-FLUX`, 18_400_000_000, 312, '4K', 'TorrentLeech'],
    [`${base}.1080p.BluRay.x264-SPARKS`, 9_800_000_000, 148, '1080p', 'IPTorrents'],
    [`${base}.720p.WEB.H264-GRP`, 2_100_000_000, 41, '720p', '1337x'],
  ];
  return releases.map(([title, size, seeders, quality, indexer], i) => ({
    guid: `mock-${fakeHash(title)}`,
    title,
    size,
    seeders,
    leechers: Math.round(seeders / 6),
    magnetUri: `magnet:?xt=urn:btih:${fakeHash(title)}&dn=${encodeURIComponent(title)}`,
    indexer,
    publishDate: iso(3 + i * 10),
    quality,
  }));
}

export const legacyHandlers = [
  http.get('*/api/tmdb/trending', () => HttpResponse.json(trending)),
  http.get('*/api/tmdb/genres', () => HttpResponse.json<GenreList>({ genres: [{ id: 16, name: 'Animation' }, { id: 18, name: 'Drama' }] })),
  http.get('*/api/library/recently-aired', () => HttpResponse.json(recentlyAired)),
  http.get('*/api/library/movies', () => HttpResponse.json<{ movies: LibraryMovie[] }>({ movies: [] })),
  http.get('*/api/library/shows', () => HttpResponse.json<{ shows: LibraryShow[] }>({ shows: [] })),
  // Torrent search (the legacy modal, until #6); adding a result creates a mapping job (handlers/assignment.ts)
  http.get('*/api/jackett/search', async ({ request }) => {
    await latency();
    const query = new URL(request.url).searchParams.get('q')?.trim();
    if (!query) return HttpResponse.json({ error: 'Search query is required' }, { status: 400 });
    return HttpResponse.json<JackettSearchResponse>({ results: jackettResults(query), query });
  }),
];
