import { queryOptions } from '@tanstack/react-query';
import { apiJson } from '../client';
import type { TorrentListItem, TorrentsSummary } from '../contract';
import type { TorrentListResponse } from '@/types/torrent';

/** Activity (PLAN §6 "System" / B6; #7). */
export const torrentKeys = {
  all: ['torrents'] as const,
  list: ['torrents', 'list'] as const,
  summary: ['torrents', 'summary'] as const,
  legacy: ['torrents', 'legacy'] as const,
};

export const torrentsApi = {
  list: () => apiJson<{ items: TorrentListItem[] }>('/api/torrents'),
  summary: () => apiJson<TorrentsSummary>('/api/torrents/summary'),
  /** The current web route's shape ({torrents}); replaced by `list` in #7. */
  legacyList: () => apiJson<TorrentListResponse>('/api/torrents'),
};

/** Live torrent stats; polls every 5 s while the tab is visible. */
export const legacyTorrentsQuery = (refetchInterval: number | false = 5000) =>
  queryOptions({ queryKey: torrentKeys.legacy, queryFn: torrentsApi.legacyList, refetchInterval });

export const torrentsSummaryQuery = () =>
  queryOptions({ queryKey: torrentKeys.summary, queryFn: torrentsApi.summary, refetchInterval: 5000 });
