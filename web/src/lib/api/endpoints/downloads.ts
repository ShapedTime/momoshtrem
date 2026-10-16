import { queryOptions } from '@tanstack/react-query';
import { ApiError, apiJson, isApiError, toQuery } from '../client';
import type {
  Download,
  DownloadCreate,
  DownloadEstimate,
  DownloadList,
  DownloadPatch,
  DownloadState,
  DownloadTarget,
  StorageFullDetails,
} from '../contract';

/**
 * Downloads (PLAN §6 "Downloads (#77)"): Keep a movie, show, season or episode
 * on disk, and the smart picks. Every /api/downloads* path goes through the
 * session-checked proxy to the backend. A Keep changes the title's `keep` and
 * stage and the torrents' `wanted` and `holds`, so invalidate `titleKeys.all`,
 * `libraryKeys.all` and `torrentKeys.all` with `downloadKeys.all`.
 */
export const downloadKeys = {
  all: ['downloads'] as const,
  list: ['downloads', 'list'] as const,
  estimate: (target: DownloadTarget) =>
    ['downloads', 'estimate', target.scope, target.id, target.scope === 'season' ? target.season_number : null] as const,
};

const downloadPath = (id: number) => `/api/downloads/${id}`;

export const downloadsApi = {
  list: () => apiJson<DownloadList>('/api/downloads'),
  /** What a Keep would need against the space left, before it's made (the Keep sheet). 400 for a bad target, 404 when not in the library. */
  estimate: (target: DownloadTarget) => apiJson<DownloadEstimate>(`/api/downloads/estimate${toQuery(target)}`),
  /**
   * A Keep: 201 a new download, 200 one already kept (unchanged) or a smart pick turned kept.
   * 409 `storage_full` (`isStorageFullError`), 422 `not_downloadable` (HDRezka) or `validation_error`
   * naming the field, 404 when the item isn't in the library.
   */
  create: (request: DownloadCreate) => apiJson<Download>('/api/downloads', { method: 'POST', json: request }),
  /** Change a kept download's duration; a smart pick is 422 (Keep it first). */
  update: (id: number, patch: DownloadPatch) => apiJson<Download>(downloadPath(id), { method: 'PATCH', json: patch }),
  /** Stop keeping: its data stays as ordinary cache and plays until it's evicted. A smart pick isn't picked again. */
  remove: (id: number) => apiJson<void>(downloadPath(id), { method: 'DELETE' }),
};

/** States that won't move on their own: nothing to poll for. */
const SETTLED: ReadonlySet<DownloadState> = new Set(['done', 'no_file', 'unavailable']);

/** Polls every 5 s while a download is still moving, never in a hidden tab. */
export const downloadsQuery = () =>
  queryOptions({
    queryKey: downloadKeys.list,
    queryFn: downloadsApi.list,
    refetchInterval: (query) => (query.state.data?.items.some((download) => !SETTLED.has(download.state)) ? 5000 : false),
  });

/** The Keep sheet's estimate; downloads move, so it's fresh for 10 s. */
export const downloadEstimateQuery = (target: DownloadTarget) =>
  queryOptions({ queryKey: downloadKeys.estimate(target), queryFn: () => downloadsApi.estimate(target), staleTime: 10_000 });

/** POST /downloads' 409: the Keep doesn't fit in the kept cap. `details` says what it needs and what's left. */
export function isStorageFullError(error: unknown): error is ApiError & { details: StorageFullDetails } {
  return (
    isApiError(error) &&
    error.status === 409 &&
    error.code === 'storage_full' &&
    typeof error.details.needed_bytes === 'number' &&
    typeof error.details.available_bytes === 'number'
  );
}

/** POST /downloads' 422 for something that plays from HDRezka: it can't be kept. */
export const isNotDownloadableError = (error: unknown): error is ApiError =>
  isApiError(error) && error.status === 422 && error.code === 'not_downloadable';
