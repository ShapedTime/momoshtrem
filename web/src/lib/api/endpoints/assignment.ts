import { QueryObserver, queryOptions, replaceEqualDeep, type QueryClient, type QueryObserverResult } from '@tanstack/react-query';
import { apiJson } from '../client';
import type {
  AssignMovieTorrentRequest,
  AssignShowTorrentRequest,
  AssignmentJob,
  AssignmentJobDetail,
  JobAccepted,
  JobListFilter,
  ManualAssignment,
  ProposalsPatch,
  ReclassifyRequest,
  ShowEpisodesSeason,
  TorrentFileEntry,
} from '../contract';
import { JOB_OPEN_STATUSES, JOB_WORKING_STATUSES, isJobOpen, isJobWorking } from '../contract/assignment';
import { libraryKeys } from './library';
import { subtitleKeys } from './subtitles';
import { titleKeys } from './titles';
import { torrentKeys } from './torrents';

/** Assignment jobs and mapping review (PLAN §6 "Assignment (B2/B3)"; #5). */
export const assignmentKeys = {
  all: ['assignment-jobs'] as const,
  lists: ['assignment-jobs', 'list'] as const,
  jobs: (filter: JobListFilter = 'open') => ['assignment-jobs', 'list', filterParam(filter)] as const,
  job: (jobId: number) => ['assignment-jobs', jobId] as const,
  allEpisodes: ['show-episodes'] as const,
  episodes: (showId: number) => ['show-episodes', showId] as const,
  torrentFiles: (infoHash: string) => ['torrents', infoHash, 'files'] as const,
};

// The status groups live with the contract (pure code shares them); re-exported for existing imports
export { JOB_WORKING_STATUSES, JOB_OPEN_STATUSES, isJobWorking, isJobOpen };

const JOB_POLL_MS = 1500;
const JOBS_POLL_MS = 3000;

function filterParam(filter: JobListFilter): string {
  return filter === 'open' ? 'open' : [...filter].sort().join(',');
}

export const assignmentApi = {
  /** 202 for a new job; 200 with the open job for the same show and torrent. */
  assignShow: (showId: number, body: AssignShowTorrentRequest) =>
    apiJson<JobAccepted>(`/api/shows/${showId}/assign-torrent`, { method: 'POST', json: body }),
  assignMovie: (movieId: number, body: AssignMovieTorrentRequest) =>
    apiJson<JobAccepted>(`/api/movies/${movieId}/assign-torrent`, { method: 'POST', json: body }),
  job: (jobId: number) => apiJson<AssignmentJobDetail>(`/api/assignment-jobs/${jobId}`),
  jobs: (filter: JobListFilter = 'open') =>
    apiJson<{ items: AssignmentJob[] }>(`/api/assignment-jobs?status=${encodeURIComponent(filterParam(filter))}`),
  /** Several rows at once (Accept all), all or nothing. */
  patchProposals: (jobId: number, patch: ProposalsPatch) =>
    apiJson<AssignmentJobDetail>(`/api/assignment-jobs/${jobId}/proposals`, { method: 'PATCH', json: patch }),
  /** 202: the job goes to `resolving`; poll it. 503 `unavailable` without AI. */
  reclassify: (jobId: number, body: ReclassifyRequest) =>
    apiJson<JobAccepted>(`/api/assignment-jobs/${jobId}/reclassify`, { method: 'POST', json: body }),
  /** 409 with an ApplyConflictCode when something still blocks it. */
  apply: (jobId: number) => apiJson<AssignmentJobDetail>(`/api/assignment-jobs/${jobId}/apply`, { method: 'POST' }),
  discard: (jobId: number) => apiJson<AssignmentJobDetail>(`/api/assignment-jobs/${jobId}/discard`, { method: 'POST' }),
  showEpisodes: (showId: number) => apiJson<{ items: ShowEpisodesSeason[] }>(`/api/shows/${showId}/episodes`),
  setEpisodeAssignment: (episodeId: number, body: ManualAssignment) =>
    apiJson<void>(`/api/episodes/${episodeId}/assignment`, { method: 'PUT', json: body }),
  setMovieAssignment: (movieId: number, body: ManualAssignment) =>
    apiJson<void>(`/api/movies/${movieId}/assignment`, { method: 'PUT', json: body }),
  torrentFiles: (infoHash: string) => apiJson<{ items: TorrentFileEntry[] }>(`/api/torrents/${infoHash}/files`),
};

/** An RFC 3339 instant to the nanosecond: Go trims trailing zeros, and Date.parse stops at milliseconds. */
function instant(timestamp: string): [number, number] {
  const match = /^(.*?)(?:\.(\d+))?(Z|[+-]\d\d:\d\d)$/i.exec(timestamp);
  if (!match) return [Date.parse(timestamp), 0];
  return [Date.parse(`${match[1]}${match[3]}`), Number((match[2] ?? '').padEnd(9, '0').slice(0, 9))];
}

/** `next` was written before `current` (by `job.updated_at`); unreadable timestamps never count as older. */
export function isOlderDetail(next: AssignmentJobDetail, current: AssignmentJobDetail): boolean {
  if (next.job.id !== current.job.id) return false;
  const [nextMs, nextNs] = instant(next.job.updated_at);
  const [currentMs, currentNs] = instant(current.job.updated_at);
  if (Number.isNaN(nextMs) || Number.isNaN(currentMs)) return false;
  return nextMs < currentMs || (nextMs === currentMs && nextNs < currentNs);
}

const isDetail = (value: unknown): value is AssignmentJobDetail =>
  typeof value === 'object' && value !== null && 'job' in value && typeof (value as AssignmentJobDetail).job?.updated_at === 'string';

/**
 * A poll answered after a newer PATCH answer is stale (it was read before the
 * change): keep the newer detail. Otherwise the usual structural sharing.
 */
export function keepNewerDetail(previous: unknown, next: unknown): unknown {
  if (isDetail(previous) && isDetail(next) && isOlderDetail(next, previous)) return previous;
  return replaceEqualDeep(previous, next);
}

/** A write to this job (PATCH, re-check, apply, discard) is on its way or waiting its turn. */
const writing = (client: QueryClient, jobId: number) =>
  client.isMutating({ predicate: (mutation) => mutation.options.scope?.id === jobMutationScope(jobId).id }) > 0;

/**
 * Polls every 1.5 s while the job is working, initial run or re-check (paused when the tab is hidden).
 * A read that lands while a write is on its way may predate it, and would undo the decision just shown:
 * keep the cached detail then (the write's answer replaces it).
 */
export const assignmentJobQuery = (jobId: number) =>
  queryOptions({
    queryKey: assignmentKeys.job(jobId),
    queryFn: async ({ client }) => {
      const detail = await assignmentApi.job(jobId);
      const cached = client.getQueryData<AssignmentJobDetail>(assignmentKeys.job(jobId));
      return cached && writing(client, jobId) ? cached : detail;
    },
    refetchInterval: (query) => (query.state.data && isJobWorking(query.state.data.job.status) ? JOB_POLL_MS : false),
    structuralSharing: keepNewerDetail,
  });

/**
 * Resolves with the job once it stops working, polling it as the review page
 * does (`refetchInterval`, paused in hidden tabs). Rejects when reading it
 * fails, after `timeoutMs` (a TimeoutError), or when `signal` aborts (an AbortError).
 */
export function waitForJob(
  queryClient: QueryClient,
  jobId: number,
  { signal, timeoutMs }: { signal?: AbortSignal; timeoutMs?: number } = {}
): Promise<AssignmentJobDetail> {
  return new Promise((resolve, reject) => {
    const observer = new QueryObserver(queryClient, assignmentJobQuery(jobId));
    let unsubscribe = () => {};
    let timer: ReturnType<typeof setTimeout> | undefined;
    const done = (settle: () => void) => {
      unsubscribe();
      clearTimeout(timer);
      signal?.removeEventListener('abort', onAbort);
      settle();
    };
    const onAbort = () => done(() => reject(new DOMException('Stopped waiting for the job', 'AbortError')));
    if (timeoutMs !== undefined) timer = setTimeout(() => done(() => reject(new DOMException('The job is still working', 'TimeoutError'))), timeoutMs);
    const check = (result: QueryObserverResult<AssignmentJobDetail>) => {
      if (result.data && !isJobWorking(result.data.job.status)) done(() => resolve(result.data));
      else if (result.isError) done(() => reject(result.error));
    };
    if (signal?.aborted) return onAbort();
    signal?.addEventListener('abort', onAbort);
    unsubscribe = observer.subscribe(check);
    check(observer.getCurrentResult());
  });
}

/** Jobs for banners (open ones by default); polls every 3 s while one of them is working. */
export const assignmentJobsQuery = (filter: JobListFilter = 'open') =>
  queryOptions({
    queryKey: assignmentKeys.jobs(filter),
    queryFn: () => assignmentApi.jobs(filter),
    refetchInterval: (query) => (query.state.data?.items.some((job) => isJobWorking(job.status)) ? JOBS_POLL_MS : false),
  });

/** The episode picker's catalog: every season, Specials included. */
export const showEpisodesQuery = (showId: number) =>
  queryOptions({ queryKey: assignmentKeys.episodes(showId), queryFn: () => assignmentApi.showEpisodes(showId) });

export const torrentFilesQuery = (infoHash: string) =>
  queryOptions({ queryKey: assignmentKeys.torrentFiles(infoHash), queryFn: () => assignmentApi.torrentFiles(infoHash) });

/**
 * A write's answer (PATCH, apply) is the job's new detail. A poll still on
 * its way was read before the write: cancel it first (`keepNewerDetail`
 * drops one that lands anyway), then put the answer in the cache.
 */
export function putJobDetail(queryClient: QueryClient, detail: AssignmentJobDetail): void {
  const queryKey = assignmentKeys.job(detail.job.id);
  // Synchronous: the cancelled fetch reverts its state now, before the answer goes in
  void queryClient.cancelQueries({ queryKey, exact: true });
  queryClient.setQueryData(queryKey, detail);
}

/** After a PATCH: the new detail goes in the cache, and the title and library counts (`needs_review`) moved too. */
export function setJobDetail(queryClient: QueryClient, detail: AssignmentJobDetail): Promise<unknown> {
  putJobDetail(queryClient, detail);
  return Promise.all([
    queryClient.invalidateQueries({ queryKey: assignmentKeys.lists }),
    queryClient.invalidateQueries({ queryKey: titleKeys.all }),
    queryClient.invalidateQueries({ queryKey: libraryKeys.all }),
  ]);
}

/**
 * After apply, discard or a re-check: refetch the job (polling restarts while
 * it works), the jobs list, and everything an applied mapping changes: titles,
 * the library, subtitles, the picker's episodes and torrent files.
 * `refetchJob: false` only marks the job stale, for a page that is leaving
 * it (discard), so it doesn't flash the job's new state on the way out.
 */
export function invalidateAfterJob(queryClient: QueryClient, jobId: number, { refetchJob = true }: { refetchJob?: boolean } = {}): Promise<unknown> {
  return Promise.all([
    queryClient.invalidateQueries({ queryKey: assignmentKeys.job(jobId), refetchType: refetchJob ? 'active' : 'none' }),
    ...[assignmentKeys.lists, titleKeys.all, libraryKeys.all, subtitleKeys.all, assignmentKeys.allEpisodes, torrentKeys.all].map((queryKey) =>
      queryClient.invalidateQueries({ queryKey })
    ),
  ]);
}

/** Every write to one job (PATCH, apply, discard) runs in order: Apply never overtakes a PATCH still on its way. */
export const jobMutationScope = (jobId: number) => ({ id: `assignment-job-${jobId}` });
