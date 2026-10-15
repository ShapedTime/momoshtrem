import { queryOptions, type QueryClient } from '@tanstack/react-query';
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

/** Polls every 1.5 s while the job is working, initial run or re-check (paused when the tab is hidden). */
export const assignmentJobQuery = (jobId: number) =>
  queryOptions({
    queryKey: assignmentKeys.job(jobId),
    queryFn: () => assignmentApi.job(jobId),
    refetchInterval: (query) => (query.state.data && isJobWorking(query.state.data.job.status) ? JOB_POLL_MS : false),
  });

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
 * After a PATCH: the answer is the new detail, so put it in the cache. The
 * title and library counts (`needs_review`) moved too.
 */
export function setJobDetail(queryClient: QueryClient, detail: AssignmentJobDetail): Promise<unknown> {
  queryClient.setQueryData(assignmentKeys.job(detail.job.id), detail);
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
