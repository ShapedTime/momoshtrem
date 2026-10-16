import { http, HttpResponse } from 'msw';
import type { LibraryMovie, LibraryShow, RecentlyAiredResponse } from '@/types/momoshtrem';
import type { TrendingResults } from '@/types/tmdb';
import { CATALOG, iso } from '../fixtures/catalog';

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

export const legacyHandlers = [
  http.get('*/api/tmdb/trending', () => HttpResponse.json(trending)),
  http.get('*/api/library/recently-aired', () => HttpResponse.json(recentlyAired)),
  http.get('*/api/library/movies', () => HttpResponse.json<{ movies: LibraryMovie[] }>({ movies: [] })),
  http.get('*/api/library/shows', () => HttpResponse.json<{ shows: LibraryShow[] }>({ shows: [] })),
];
