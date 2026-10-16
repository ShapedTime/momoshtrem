import { queryOptions } from '@tanstack/react-query';
import { apiJson, toQuery } from '../client';
import type { AddTitleResult, EpisodeGroup, MediaType, SeasonDetail, ShowSettingsPatch, ShowTitle, Title } from '../contract';

/** Title pages (PLAN §6 "Titles and library"; #4). */
export const titleKeys = {
  all: ['titles'] as const,
  detail: (mediaType: MediaType, tmdbId: number) => ['titles', mediaType, tmdbId] as const,
  /** Under the show's detail key, so invalidating a title refetches its seasons too. */
  season: (tmdbId: number, seasonNumber: number) => ['titles', 'tv', tmdbId, 'season', seasonNumber] as const,
  episodeGroups: (showId: number) => ['shows', showId, 'episode-groups'] as const,
};

export interface RemoveOptions {
  /** Also delete the downloaded data of torrents no other title uses (#96). Unset: the backend's default (false). */
  deleteData?: boolean;
}

/** `DELETE /movies/{id}` or `/shows/{id}`, with `?delete_data=` when chosen. */
export function removeTitleUrl(mediaType: MediaType, libraryId: number, options: RemoveOptions = {}): string {
  return `/api/${mediaType === 'movie' ? 'movies' : 'shows'}/${libraryId}${toQuery({ delete_data: options.deleteData })}`;
}

export const titlesApi = {
  get: (mediaType: MediaType, tmdbId: number) => apiJson<Title>(`/api/titles/${mediaType}/${tmdbId}`),
  season: (tmdbId: number, seasonNumber: number) => apiJson<SeasonDetail>(`/api/titles/tv/${tmdbId}/seasons/${seasonNumber}`),
  addMovie: (tmdbId: number) => apiJson<AddTitleResult>('/api/movies', { method: 'POST', json: { tmdb_id: tmdbId } }),
  addShow: (tmdbId: number) => apiJson<AddTitleResult>('/api/shows', { method: 'POST', json: { tmdb_id: tmdbId } }),
  /** Removes the title, its subtitles and the torrents nothing else uses; deleteData also deletes their data (B6). */
  removeMovie: (movieId: number, options?: RemoveOptions) => apiJson<void>(removeTitleUrl('movie', movieId, options), { method: 'DELETE' }),
  removeShow: (showId: number, options?: RemoveOptions) => apiJson<void>(removeTitleUrl('tv', showId, options), { method: 'DELETE' }),
  /** The answer is informational: refetch the title afterwards. */
  refreshMovie: (movieId: number) => apiJson<void>(`/api/movies/${movieId}/refresh`, { method: 'POST' }),
  refreshShow: (showId: number) => apiJson<void>(`/api/shows/${showId}/refresh`, { method: 'POST' }),
  unassignMovie: (movieId: number) => apiJson<void>(`/api/movies/${movieId}/assign`, { method: 'DELETE' }),
  unassignEpisode: (episodeId: number) => apiJson<void>(`/api/episodes/${episodeId}/assign`, { method: 'DELETE' }),
  patchShow: (showId: number, patch: ShowSettingsPatch) =>
    apiJson<ShowTitle>(`/api/shows/${showId}`, { method: 'PATCH', json: patch }),
  episodeGroups: (showId: number) => apiJson<{ items: EpisodeGroup[] }>(`/api/shows/${showId}/episode-groups`),
};

export const titleQuery = (mediaType: MediaType, tmdbId: number) =>
  queryOptions({ queryKey: titleKeys.detail(mediaType, tmdbId), queryFn: () => titlesApi.get(mediaType, tmdbId) });

export const seasonQuery = (tmdbId: number, seasonNumber: number) =>
  queryOptions({ queryKey: titleKeys.season(tmdbId, seasonNumber), queryFn: () => titlesApi.season(tmdbId, seasonNumber) });

export const episodeGroupsQuery = (showId: number) =>
  queryOptions({ queryKey: titleKeys.episodeGroups(showId), queryFn: () => titlesApi.episodeGroups(showId), staleTime: 60 * 60_000 });
