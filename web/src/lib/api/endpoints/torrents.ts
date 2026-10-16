import { queryOptions } from '@tanstack/react-query';
import { apiJson, toQuery } from '../client';
import type { TorrentDetail, TorrentListItem, TorrentsPauseCount, TorrentsSummary } from '../contract';

/**
 * Activity (PLAN §6 "Torrents and activity (B6)"). Every /api/torrents* path
 * goes through the session-checked proxy to the backend.
 */
export const torrentKeys = {
  all: ['torrents'] as const,
  list: ['torrents', 'list'] as const,
  summary: ['torrents', 'summary'] as const,
  detail: (infoHash: string) => ['torrents', 'detail', infoHash.toLowerCase()] as const,
};

const torrentPath = (infoHash: string) => `/api/torrents/${encodeURIComponent(infoHash)}`;

/** DELETE /torrents/{hash}?delete_data=true|false, also for a keepalive request while the page unloads. */
export const removeTorrentUrl = (infoHash: string, { deleteData = false }: { deleteData?: boolean } = {}) =>
  `${torrentPath(infoHash)}${toQuery({ delete_data: deleteData })}`;

export const torrentsApi = {
  list: () => apiJson<{ items: TorrentListItem[] }>('/api/torrents'),
  summary: () => apiJson<TorrentsSummary>('/api/torrents/summary'),
  /** The list item plus pieces, files, trackers and totals; never loads the torrent. 404 for one nothing knows. */
  detail: (infoHash: string) => apiJson<TorrentDetail>(torrentPath(infoHash)),
  /** Sticky: the network stays off and reads don't wake it, across restarts. Idempotent. */
  pause: (infoHash: string) => apiJson<TorrentListItem>(`${torrentPath(infoHash)}/pause`, { method: 'POST' }),
  /** A loaded torrent comes back on until the idle timeout; one that isn't loaded stays unloaded. */
  resume: (infoHash: string) => apiJson<TorrentListItem>(`${torrentPath(infoHash)}/resume`, { method: 'POST' }),
  /** Pauses the streaming and downloading torrents. */
  pauseAll: () => apiJson<TorrentsPauseCount>('/api/torrents/pause-all', { method: 'POST' }),
  /** Resumes every paused torrent. */
  resumeAll: () => apiJson<TorrentsPauseCount>('/api/torrents/resume-all', { method: 'POST' }),
  /** Forgets the torrent (its jobs, assignments and torrent subtitles) and drops it; deleteData also deletes its pieces. */
  remove: (infoHash: string, options: { deleteData?: boolean } = {}) =>
    apiJson<void>(removeTorrentUrl(infoHash, options), { method: 'DELETE' }),
};

/** Every torrent the library knows, loaded or not; polls every 5 s while the tab is visible. */
export const torrentsListQuery = () =>
  queryOptions({ queryKey: torrentKeys.list, queryFn: torrentsApi.list, refetchInterval: 5000 });

/** One torrent's detail (Activity's detail page); polls every 5 s while the tab is visible. */
export const torrentDetailQuery = (infoHash: string) =>
  queryOptions({ queryKey: torrentKeys.detail(infoHash), queryFn: () => torrentsApi.detail(infoHash), refetchInterval: 5000 });

/** Speeds sample every 2 s on the server; 5 s keeps the strip and the sparkline moving. */
export const torrentsSummaryQuery = () =>
  queryOptions({ queryKey: torrentKeys.summary, queryFn: torrentsApi.summary, refetchInterval: 5000 });
