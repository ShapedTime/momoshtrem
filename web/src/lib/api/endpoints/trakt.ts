import { queryOptions } from '@tanstack/react-query';
import { apiJson, toQuery } from '../client';
import type { HistoryItem, MediaType, Paginated, TraktConnect, TraktIntegration, UpNextItem, WatchlistItem } from '../contract';

/** Trakt and personal lists (PLAN §6 "Trakt"; #8). */
export const traktKeys = {
  integration: ['integrations', 'trakt'] as const,
  upNext: ['me', 'up-next'] as const,
  watchlist: (type?: MediaType) => ['me', 'watchlist', type ?? 'all'] as const,
  history: (page: number) => ['me', 'history', page] as const,
};

export const traktApi = {
  integration: () => apiJson<TraktIntegration>('/api/integrations/trakt'),
  connect: () => apiJson<TraktConnect>('/api/integrations/trakt/connect', { method: 'POST' }),
  sync: () => apiJson<void>('/api/integrations/trakt/sync', { method: 'POST' }),
  disconnect: () => apiJson<void>('/api/integrations/trakt', { method: 'DELETE' }),
  upNext: () => apiJson<{ items: UpNextItem[] }>('/api/me/up-next'),
  watchlist: (type?: MediaType) => apiJson<{ items: WatchlistItem[] }>(`/api/me/watchlist${toQuery({ type })}`),
  history: (page = 1) => apiJson<Paginated<HistoryItem>>(`/api/me/history${toQuery({ page })}`),
};

/** Polls while a device-code connection or a sync is in progress. */
export const traktIntegrationQuery = (pollWhileConnecting = false) =>
  queryOptions({
    queryKey: traktKeys.integration,
    queryFn: traktApi.integration,
    refetchInterval: (query) =>
      pollWhileConnecting || query.state.data?.sync_status === 'syncing' ? 3000 : false,
  });

export const upNextQuery = () => queryOptions({ queryKey: traktKeys.upNext, queryFn: traktApi.upNext });
export const watchlistQuery = (type?: MediaType) =>
  queryOptions({ queryKey: traktKeys.watchlist(type), queryFn: () => traktApi.watchlist(type) });
export const historyQuery = (page = 1) =>
  queryOptions({ queryKey: traktKeys.history(page), queryFn: () => traktApi.history(page) });
