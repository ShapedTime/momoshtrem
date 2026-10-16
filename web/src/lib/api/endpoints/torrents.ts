import { queryOptions } from '@tanstack/react-query';
import { apiJson } from '../client';
import type { TorrentListItem, TorrentsPauseCount, TorrentsSummary } from '../contract';
import type { TorrentListResponse } from '@/types/torrent';

/**
 * Activity (PLAN §6 "Torrents and activity (B6)").
 *
 * Until #102 retires app/api/torrents/**, the legacy web routes answer these
 * paths on the real stack: GET /api/torrents keeps the legacy `{torrents}`
 * shape, and the [hash] routes catch summary, pause, resume and the *-all
 * routes. Mock mode (MSW) answers the new shapes, except GET /api/torrents.
 */
export const torrentKeys = {
  all: ['torrents'] as const,
  list: ['torrents', 'list'] as const,
  summary: ['torrents', 'summary'] as const,
  legacy: ['torrents', 'legacy'] as const,
};

const torrentPath = (infoHash: string) => `/api/torrents/${encodeURIComponent(infoHash)}`;

export const torrentsApi = {
  list: () => apiJson<{ items: TorrentListItem[] }>('/api/torrents'),
  summary: () => apiJson<TorrentsSummary>('/api/torrents/summary'),
  /** Sticky: the network stays off and reads don't wake it, across restarts. Idempotent. */
  pause: (infoHash: string) => apiJson<TorrentListItem>(`${torrentPath(infoHash)}/pause`, { method: 'POST' }),
  /** A loaded torrent comes back on until the idle timeout; one that isn't loaded stays unloaded. */
  resume: (infoHash: string) => apiJson<TorrentListItem>(`${torrentPath(infoHash)}/resume`, { method: 'POST' }),
  /** Pauses the streaming and downloading torrents. */
  pauseAll: () => apiJson<TorrentsPauseCount>('/api/torrents/pause-all', { method: 'POST' }),
  /** Resumes every paused torrent. */
  resumeAll: () => apiJson<TorrentsPauseCount>('/api/torrents/resume-all', { method: 'POST' }),
  /** Forgets the torrent (its jobs, assignments and torrent subtitles) and drops it; deleteData also deletes its pieces. */
  remove: (infoHash: string, { deleteData = false }: { deleteData?: boolean } = {}) =>
    apiJson<void>(`${torrentPath(infoHash)}${deleteData ? '?delete_data=true' : ''}`, { method: 'DELETE' }),
  /** The current web route's shape ({torrents}); replaced by `list` in #102. */
  legacyList: () => apiJson<TorrentListResponse>('/api/torrents'),
};

/** Every torrent the library knows, loaded or not; polls every 5 s while the tab is visible. */
export const torrentsListQuery = () =>
  queryOptions({ queryKey: torrentKeys.list, queryFn: torrentsApi.list, refetchInterval: 5000 });

/** Live torrent stats; polls every 5 s while the tab is visible. */
export const legacyTorrentsQuery = (refetchInterval: number | false = 5000) =>
  queryOptions({ queryKey: torrentKeys.legacy, queryFn: torrentsApi.legacyList, refetchInterval });

/** Speeds sample every 2 s on the server; 5 s keeps the strip and the sparkline moving. */
export const torrentsSummaryQuery = () =>
  queryOptions({ queryKey: torrentKeys.summary, queryFn: torrentsApi.summary, refetchInterval: 5000 });
