import { http, HttpResponse } from 'msw';
import type { JackettSearchResponse, TorrentResult, VideoQuality } from '@/types/jackett';
import type { AddTorrentResponse, LibraryMovie, LibraryShow, RecentlyAiredResponse } from '@/types/momoshtrem';
import type { GenreList, TrendingResults } from '@/types/tmdb';
import { CATALOG, findTitle, iso } from '../fixtures/catalog';
import { addToLibrary, assignMovieTorrent, libraryKey } from '../fixtures/titles';
import { mockLibrary } from './titles';
import { isRecord, jsonBody, latency } from './util';

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

const isAddTorrent = (body: unknown): body is { magnet_uri: string; media_type: 'movie' | 'tv'; tmdb_id: number } =>
  isRecord(body) &&
  typeof body.magnet_uri === 'string' &&
  (body.media_type === 'movie' || body.media_type === 'tv') &&
  typeof body.tmdb_id === 'number';

export const legacyHandlers = [
  http.get('*/api/tmdb/trending', () => HttpResponse.json(trending)),
  http.get('*/api/tmdb/genres', () => HttpResponse.json<GenreList>({ genres: [{ id: 16, name: 'Animation' }, { id: 18, name: 'Drama' }] })),
  http.get('*/api/library/recently-aired', () => HttpResponse.json(recentlyAired)),
  http.get('*/api/library/movies', () => HttpResponse.json<{ movies: LibraryMovie[] }>({ movies: [] })),
  http.get('*/api/library/shows', () => HttpResponse.json<{ shows: LibraryShow[] }>({ shows: [] })),
  // Torrent search and add (the legacy modal, until #6)
  http.get('*/api/jackett/search', async ({ request }) => {
    await latency();
    const query = new URL(request.url).searchParams.get('q')?.trim();
    if (!query) return HttpResponse.json({ error: 'Search query is required' }, { status: 400 });
    return HttpResponse.json<JackettSearchResponse>({ results: jackettResults(query), query });
  }),
  http.post('*/api/library/add-torrent', async ({ request }) => {
    await latency();
    const body = await jsonBody(request);
    if (!isAddTorrent(body)) return HttpResponse.json({ error: 'Send {magnet_uri, media_type, tmdb_id}.' }, { status: 400 });
    const entry = findTitle(body.media_type, body.tmdb_id);
    if (!entry) return HttpResponse.json({ error: 'TMDB has no such title.' }, { status: 400 });
    const library = mockLibrary();
    const existing = library.get(libraryKey(entry.media_type, entry.tmdb_id));
    const row = existing ?? addToLibrary(library, entry);
    const base = { success: true, added_to_library: !existing, library_id: row.library_id };
    if (body.media_type === 'tv') {
      // Episode matching arrives with the mapping review (#5); the mock matches nothing
      const answer: AddTorrentResponse = { ...base, media_type: 'tv', summary: { total_files: 0, matched: 0, unmatched: 0, skipped: 0 }, matched: [], unmatched: [] };
      return HttpResponse.json(answer, { status: 201 });
    }
    const torrent = assignMovieTorrent(row, body.magnet_uri);
    const answer: AddTorrentResponse = {
      ...base,
      media_type: 'movie',
      assignment: {
        id: row.library_id,
        info_hash: torrent.info_hash,
        file_path: torrent.file_name,
        file_size: torrent.size_bytes,
        resolution: torrent.resolution ?? undefined,
        source: torrent.source ?? undefined,
      },
    };
    return HttpResponse.json(answer, { status: 201 });
  }),
];
