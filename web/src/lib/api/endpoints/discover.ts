import { infiniteQueryOptions, keepPreviousData, queryOptions } from '@tanstack/react-query';
import type { DiscoverFilters } from '@/lib/discover/filters';
import { discoverApiQuery } from '@/lib/discover/url';
import { apiJson } from '../client';
import type { DiscoverPage, DiscoverRelaxed } from '../contract';

/** Discover (#100): the web route GET /api/discover (web-side, not PLAN §6). */
export const discoverKeys = {
  all: ['discover'] as const,
  pages: (query: string) => ['discover', 'pages', query] as const,
  count: (query: string) => ['discover', 'count', query] as const,
  relax: (query: string) => ['discover', 'relax', query] as const,
};

const withQuery = (query: string, extra: string) => `/api/discover?${query ? `${query}&` : ''}${extra}`;

export const discoverApi = {
  /** `query` from `discoverApiQuery`. */
  page: (query: string, page: number) => apiJson<DiscoverPage>(withQuery(query, `page=${page}`)),
  relax: (query: string) => apiJson<DiscoverRelaxed>(withQuery(query, 'relax=1')),
};

/** Results a page at a time, for infinite scroll. */
export const discoverPagesQuery = (filters: DiscoverFilters) => {
  const query = discoverApiQuery(filters);
  return infiniteQueryOptions({
    queryKey: discoverKeys.pages(query),
    queryFn: ({ pageParam }) => discoverApi.page(query, pageParam),
    initialPageParam: 1,
    getNextPageParam: (last) => (last.items.length > 0 && last.page < last.total_pages ? last.page + 1 : undefined),
    staleTime: 5 * 60_000,
  });
};

/** How many titles these filters find (page 1), for the phone sheet's "Show 42 shows". */
export const discoverCountQuery = (filters: DiscoverFilters) => {
  const query = discoverApiQuery(filters);
  return queryOptions({
    queryKey: discoverKeys.count(query),
    queryFn: () => discoverApi.page(query, 1),
    select: (page: DiscoverPage) => page.total,
    placeholderData: keepPreviousData,
    staleTime: 5 * 60_000,
  });
};

/** The total without each active filter, for the empty state's fix chips. */
export const discoverRelaxQuery = (filters: DiscoverFilters) => {
  const query = discoverApiQuery(filters);
  return queryOptions({
    queryKey: discoverKeys.relax(query),
    queryFn: () => discoverApi.relax(query),
    staleTime: 5 * 60_000,
  });
};
