import { queryOptions } from '@tanstack/react-query';
import { apiJson } from '../client';
import type {
  AssignMovieTorrentRequest,
  AssignShowTorrentRequest,
  AssignmentJobDetail,
  JobAccepted,
  JobStatus,
  ManualAssignment,
  ProposalPatch,
  ReclassifyRequest,
  TorrentFileEntry,
} from '../contract';

/** Assignment jobs and mapping review (PLAN §6 "Assignment"; #5). */
export const assignmentKeys = {
  job: (jobId: number) => ['assignment-jobs', jobId] as const,
  torrentFiles: (infoHash: string) => ['torrents', infoHash, 'files'] as const,
};

const ACTIVE: JobStatus[] = ['queued', 'fetching_metadata', 'classifying'];

export const assignmentApi = {
  assignShow: (showId: number, body: AssignShowTorrentRequest) =>
    apiJson<JobAccepted>(`/api/shows/${showId}/assign-torrent`, { method: 'POST', json: body }),
  assignMovie: (movieId: number, body: AssignMovieTorrentRequest) =>
    apiJson<JobAccepted>(`/api/movies/${movieId}/assign-torrent`, { method: 'POST', json: body }),
  job: (jobId: number) => apiJson<AssignmentJobDetail>(`/api/assignment-jobs/${jobId}`),
  patchProposal: (jobId: number, proposalId: number, patch: ProposalPatch) =>
    apiJson<AssignmentJobDetail>(`/api/assignment-jobs/${jobId}/proposals/${proposalId}`, { method: 'PATCH', json: patch }),
  reclassify: (jobId: number, body: ReclassifyRequest) =>
    apiJson<JobAccepted>(`/api/assignment-jobs/${jobId}/reclassify`, { method: 'POST', json: body }),
  apply: (jobId: number) => apiJson<AssignmentJobDetail>(`/api/assignment-jobs/${jobId}/apply`, { method: 'POST' }),
  setEpisodeAssignment: (episodeId: number, body: ManualAssignment) =>
    apiJson<void>(`/api/episodes/${episodeId}/assignment`, { method: 'PUT', json: body }),
  setMovieAssignment: (movieId: number, body: ManualAssignment) =>
    apiJson<void>(`/api/movies/${movieId}/assignment`, { method: 'PUT', json: body }),
  torrentFiles: (infoHash: string) => apiJson<{ items: TorrentFileEntry[] }>(`/api/torrents/${infoHash}/files`),
};

/** Polls every 1.5 s while the job is still working (paused when the tab is hidden). */
export const assignmentJobQuery = (jobId: number) =>
  queryOptions({
    queryKey: assignmentKeys.job(jobId),
    queryFn: () => assignmentApi.job(jobId),
    refetchInterval: (query) => (query.state.data && ACTIVE.includes(query.state.data.job.status) ? 1500 : false),
  });

export const torrentFilesQuery = (infoHash: string) =>
  queryOptions({ queryKey: assignmentKeys.torrentFiles(infoHash), queryFn: () => assignmentApi.torrentFiles(infoHash) });
