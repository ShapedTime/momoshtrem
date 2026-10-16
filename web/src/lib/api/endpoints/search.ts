import { infiniteQueryOptions, keepPreviousData, queryOptions } from '@tanstack/react-query';
import { apiJson, isApiError, toQuery } from '../client';
import type {
  InspectRequest,
  JobAccepted,
  QualityProfile,
  ResolveRequest,
  ResolveResponse,
  SearchTorrentsParams,
  SearchTorrentsResponse,
} from '../contract';

/** Torrent search and quality profile (PLAN §6 "Search"; #6). */
export const searchKeys = {
  torrents: (params: SearchTorrentsParams) => ['search', 'torrents', params] as const,
  /** The sheet's pages (Load more appends): page 1's params, without `page`. */
  torrentPages: (params: SearchTorrentsParams) => ['search', 'torrents', 'pages', params] as const,
  qualityProfile: ['settings', 'quality-profile'] as const,
};

export const searchApi = {
  torrents: (params: SearchTorrentsParams) => apiJson<SearchTorrentsResponse>(`/api/search/torrents${toQuery(params)}`),
  /** A release without a magnet: the server fetches its .torrent (the Jackett key never leaves it). */
  resolve: (body: ResolveRequest) => apiJson<ResolveResponse>('/api/search/torrents/resolve', { method: 'POST', json: body }),
  inspect: (body: InspectRequest) => apiJson<JobAccepted>('/api/search/torrents/inspect', { method: 'POST', json: body }),
  qualityProfile: () => apiJson<QualityProfile>('/api/settings/quality-profile'),
  putQualityProfile: (profile: QualityProfile) =>
    apiJson<QualityProfile>('/api/settings/quality-profile', { method: 'PUT', json: profile }),
};

/** The backend caches result sets for 10 minutes; paging and filters keep the previous page on screen. */
export const searchTorrentsQuery = (params: SearchTorrentsParams) =>
  queryOptions({
    queryKey: searchKeys.torrents(params),
    queryFn: () => searchApi.torrents(params),
    staleTime: 5 * 60_000,
    placeholderData: keepPreviousData,
  });

/** Polled every second while the search runs (`complete: false`); pauses in a hidden tab. */
export const SEARCH_POLL_MS = 1000;

/**
 * The search sheet's results, page by page. While the search runs, a refetch
 * (the poll) rereads every loaded page without `refresh`. A failed request is
 * retried once only when it never reached the backend: the sheet has Retry.
 */
export const searchTorrentPagesQuery = (params: SearchTorrentsParams) =>
  infiniteQueryOptions({
    queryKey: searchKeys.torrentPages(params),
    queryFn: ({ pageParam }) => searchApi.torrents({ ...params, page: pageParam }),
    initialPageParam: 1,
    getNextPageParam: (last: SearchTorrentsResponse) => (last.page * last.page_size < last.total ? last.page + 1 : undefined),
    staleTime: 5 * 60_000,
    retry: (failures: number, error: Error) => !isApiError(error) && failures < 1,
    refetchInterval: (query) => (query.state.data?.pages[0]?.complete === false ? SEARCH_POLL_MS : false),
  });

export const qualityProfileQuery = () =>
  queryOptions({ queryKey: searchKeys.qualityProfile, queryFn: searchApi.qualityProfile });
