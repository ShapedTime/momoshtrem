import { infiniteQueryOptions, keepPreviousData, queryOptions } from '@tanstack/react-query';
import { apiJson, toQuery } from '../client';
import type { LibraryPage, LibraryParams, LibraryRef } from '../contract';

/** Library (PLAN §6; #7, #97). */
export const libraryKeys = {
  all: ['library'] as const,
  list: (params: LibraryParams) => ['library', params] as const,
  pages: (params: Omit<LibraryParams, 'page'>) => ['library', 'pages', params] as const,
  lookup: (ids: string) => ['library', 'lookup', ids] as const,
};

/** At most this many ids per `/library?ids=` call (the backend's cap). */
export const MAX_LIBRARY_IDS = 200;

/** `ids` for these titles: `movie:603,tv:1399`, each once, sorted so equal sets share a cache entry. */
export function libraryIds(refs: LibraryRef[]): string {
  return [...new Set(refs.map((ref) => `${ref.media_type}:${ref.tmdb_id}`))].sort().join(',');
}

export const libraryApi = {
  list: (params: LibraryParams = {}) => apiJson<LibraryPage>(`/api/library${toQuery(params)}`),
};

export const libraryQuery = (params: LibraryParams = {}) =>
  queryOptions({ queryKey: libraryKeys.list(params), queryFn: () => libraryApi.list(params), placeholderData: keepPreviousData });

/** The library a page at a time, for "Load more". */
export const libraryPagesQuery = (params: Omit<LibraryParams, 'page'>) =>
  infiniteQueryOptions({
    queryKey: libraryKeys.pages(params),
    queryFn: ({ pageParam }) => libraryApi.list({ ...params, page: pageParam }),
    initialPageParam: 1,
    getNextPageParam: (last) => (last.page * last.page_size < last.total ? last.page + 1 : undefined),
    placeholderData: keepPreviousData,
  });

/** Which of these titles are in the library (`ids` from `libraryIds`); empty ids ask nothing. */
export const libraryLookupQuery = (ids: string) =>
  queryOptions({
    queryKey: libraryKeys.lookup(ids),
    queryFn: () => libraryApi.list({ ids }),
    enabled: ids !== '',
    staleTime: 30_000,
  });
