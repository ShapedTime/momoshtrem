import { queryOptions } from '@tanstack/react-query';
import { apiJson } from '../client';
import type { EpisodeGroup, MediaType, ShowSettingsPatch, ShowTitle, Title } from '../contract';

/** Title pages (PLAN §6 "Titles and library"; #4). */
export const titleKeys = {
  all: ['titles'] as const,
  detail: (mediaType: MediaType, tmdbId: number) => ['titles', mediaType, tmdbId] as const,
  episodeGroups: (showId: number) => ['shows', showId, 'episode-groups'] as const,
};

export const titlesApi = {
  get: (mediaType: MediaType, tmdbId: number) => apiJson<Title>(`/api/titles/${mediaType}/${tmdbId}`),
  patchShow: (showId: number, patch: ShowSettingsPatch) =>
    apiJson<ShowTitle>(`/api/shows/${showId}`, { method: 'PATCH', json: patch }),
  episodeGroups: (showId: number) => apiJson<{ items: EpisodeGroup[] }>(`/api/shows/${showId}/episode-groups`),
};

export const titleQuery = (mediaType: MediaType, tmdbId: number) =>
  queryOptions({ queryKey: titleKeys.detail(mediaType, tmdbId), queryFn: () => titlesApi.get(mediaType, tmdbId) });

export const episodeGroupsQuery = (showId: number) =>
  queryOptions({ queryKey: titleKeys.episodeGroups(showId), queryFn: () => titlesApi.episodeGroups(showId), staleTime: 60 * 60_000 });
