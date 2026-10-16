import { keepPreviousData, queryOptions } from '@tanstack/react-query';
import { apiJson, toQuery } from '../client';
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

export const qualityProfileQuery = () =>
  queryOptions({ queryKey: searchKeys.qualityProfile, queryFn: searchApi.qualityProfile });
