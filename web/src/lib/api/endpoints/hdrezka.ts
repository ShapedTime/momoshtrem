import { queryOptions } from '@tanstack/react-query';
import { apiJson, isApiError, toQuery } from '../client';
import type {
  AssignMovieStreamRequest,
  AssignShowStreamRequest,
  AssignShowStreamResult,
  HDRezkaSearchParams,
  HDRezkaSearchResult,
  HDRezkaStreams,
  HDRezkaStreamsParams,
  HDRezkaTitle,
  HDRezkaTitleParams,
} from '../contract';

/** HDRezka as a second source (PLAN §6 "HDRezka (#63)"). */
export const hdrezkaKeys = {
  all: ['hdrezka'] as const,
  search: ({ media_type, tmdb_id, q }: HDRezkaSearchParams) => ['hdrezka', 'search', media_type, tmdb_id, q ?? null] as const,
  title: ({ url, translator_id, show_id, movie_id }: HDRezkaTitleParams) =>
    ['hdrezka', 'titles', url, translator_id ?? null, show_id ?? null, movie_id ?? null] as const,
  streams: ({ url, translator_id, camrip, ads, director, season, episode }: HDRezkaStreamsParams) =>
    ['hdrezka', 'streams', url, translator_id, camrip ?? null, ads ?? null, director ?? null, season ?? null, episode ?? null] as const,
};

export const hdrezkaApi = {
  search: (params: HDRezkaSearchParams) => apiJson<HDRezkaSearchResult>(`/api/hdrezka/search${toQuery(params)}`),
  title: (params: HDRezkaTitleParams) => apiJson<HDRezkaTitle>(`/api/hdrezka/titles${toQuery(params)}`),
  streams: (params: HDRezkaStreamsParams) => apiJson<HDRezkaStreams>(`/api/hdrezka/streams${toQuery(params)}`),
  /** 204; the movie then plays the stream (its torrent, if any, is unassigned). */
  assignMovie: (movieId: number, body: AssignMovieStreamRequest) =>
    apiJson<void>(`/api/movies/${movieId}/assign-hdrezka`, { method: 'POST', json: body }),
  /** 1–30 episodes a request; some may fail while the rest are assigned. */
  assignShow: (showId: number, body: AssignShowStreamRequest) =>
    apiJson<AssignShowStreamResult>(`/api/shows/${showId}/assign-hdrezka`, { method: 'POST', json: body }),
};

/**
 * HDRezka's own failures (the anti-bot page, a timeout) don't pass in a
 * second: the sheet shows them with Retry instead. Network errors retry once.
 */
const retryOnce = (failureCount: number, error: Error) => !isApiError(error) && failureCount < 1;

/** Hits change rarely; keep them for 5 minutes. */
export const hdrezkaSearchQuery = (params: HDRezkaSearchParams) =>
  queryOptions({ queryKey: hdrezkaKeys.search(params), queryFn: () => hdrezkaApi.search(params), staleTime: 5 * 60_000, retry: retryOnce });

export const hdrezkaTitleQuery = (params: HDRezkaTitleParams) =>
  queryOptions({ queryKey: hdrezkaKeys.title(params), queryFn: () => hdrezkaApi.title(params), staleTime: 5 * 60_000, retry: retryOnce });

/** The backend measures sizes from the CDN; they don't change while the sheet is open. */
export const hdrezkaStreamsQuery = (params: HDRezkaStreamsParams) =>
  queryOptions({ queryKey: hdrezkaKeys.streams(params), queryFn: () => hdrezkaApi.streams(params), staleTime: 5 * 60_000, retry: retryOnce });
