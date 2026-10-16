import { delay, http, HttpResponse } from 'msw';
import type { DiscoverItem, DiscoverPage, DiscoverRelaxed } from '@/lib/api/contract';
import { type DiscoverFilters, withoutFilter } from '@/lib/discover/filters';
import { relaxKeys, RUNTIME_RANGES } from '@/lib/discover/tmdb-query';
import { parseDiscoverSearch } from '@/lib/discover/url';
import { DISCOVER_FIXTURES, type DiscoverFixture } from '../fixtures/discover';
import { scenario } from '../scenario';
import { apiError, latency } from './util';

/** Like TMDB's page size; `type=all` pages through one merged list here (the real route merges two pages). */
export const MOCK_DISCOVER_PAGE_SIZE = 20;

function inRuntime(entry: DiscoverFixture, filters: DiscoverFilters): boolean {
  if (!filters.runtime) return true;
  const { gte = 0, lte = Infinity } = RUNTIME_RANGES[entry.media_type][filters.runtime];
  return entry.runtime >= gte && entry.runtime <= lte;
}

/** Approximate, deterministic filtering: genres match by slug (TMDB's merged TV genres aren't modelled). */
export function matchesDiscover(entry: DiscoverFixture, filters: DiscoverFilters): boolean {
  if (filters.type !== 'all' && entry.media_type !== filters.type) return false;
  if (!filters.genres.every((slug) => entry.genres.includes(slug))) return false;
  if (filters.anime && !(entry.genres.includes('animation') && entry.lang === 'ja')) return false;
  if (filters.from !== null && entry.year < filters.from) return false;
  if (filters.to !== null && entry.year > filters.to) return false;
  if (filters.rating !== null && entry.rating < filters.rating) return false;
  if (!inRuntime(entry, filters)) return false;
  if (filters.lang && entry.lang !== filters.lang) return false;
  if (filters.country && entry.country !== filters.country) return false;
  if (filters.status && entry.status !== filters.status) return false;
  return true;
}

const SORTS: Record<DiscoverFilters['sort'], (a: DiscoverFixture, b: DiscoverFixture) => number> = {
  popular: (a, b) => b.popularity - a.popularity,
  rating: (a, b) => b.rating - a.rating,
  newest: (a, b) => b.year - a.year,
  title: (a, b) => a.title.localeCompare(b.title, 'en'),
};

export function queryDiscover(filters: DiscoverFilters): DiscoverFixture[] {
  return DISCOVER_FIXTURES.filter((entry) => matchesDiscover(entry, filters)).sort(SORTS[filters.sort]);
}

const toItem = ({ media_type, tmdb_id, title, year }: DiscoverFixture): DiscoverItem => ({ media_type, tmdb_id, title, year, poster_path: null });

/** GET /api/discover over the fixture catalogue, with `relax=1` (scenarios: `discover-error`, `discover-loading`). */
export const discoverHandlers = [
  http.get('*/api/discover', async ({ request }) => {
    if (scenario() === 'discover-loading') await delay('infinite');
    await latency();
    if (scenario() === 'discover-error') return apiError(502, 'tmdb_unavailable', 'TMDB didn’t answer. Try again in a moment.');
    const search = new URL(request.url).searchParams;
    const filters = parseDiscoverSearch(search);
    if (search.get('relax') === '1') {
      const body: DiscoverRelaxed = { relaxed: relaxKeys(filters).map((filter) => ({ filter, would_be: queryDiscover(withoutFilter(filters, filter)).length })) };
      return HttpResponse.json(body);
    }
    const page = Number(search.get('page') ?? '1');
    if (!Number.isInteger(page) || page < 1 || page > 500) return apiError(400, 'bad_request', 'page must be a whole number from 1 to 500.');
    const all = queryDiscover(filters);
    const start = (page - 1) * MOCK_DISCOVER_PAGE_SIZE;
    const body: DiscoverPage = {
      items: all.slice(start, start + MOCK_DISCOVER_PAGE_SIZE).map(toItem),
      total: all.length,
      page,
      total_pages: Math.ceil(all.length / MOCK_DISCOVER_PAGE_SIZE),
    };
    return HttpResponse.json(body);
  }),
];
