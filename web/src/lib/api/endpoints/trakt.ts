import { queryOptions } from '@tanstack/react-query';
import { apiJson, toQuery } from '../client';
import type {
  HistoryItem,
  MediaType,
  NewEpisodeItem,
  Paginated,
  TraktConnect,
  TraktIntegration,
  TraktSettings,
  TraktSettingsPatch,
  UpNextItem,
  WatchlistItem,
} from '../contract';

/** Trakt and personal lists (PLAN §6 "Trakt (B5)"; #8). */
export const traktKeys = {
  integration: ['integrations', 'trakt'] as const,
  /** Everything under /me: invalidate it when a sync lands. */
  me: ['me'] as const,
  upNext: ['me', 'up-next'] as const,
  newEpisodes: (days: number) => ['me', 'new-episodes', days] as const,
  watchlist: (type?: MediaType) => ['me', 'watchlist', type ?? 'all'] as const,
  history: (page: number, pageSize?: number) => ['me', 'history', page, pageSize ?? 'default'] as const,
};

/** The new-episodes window For You asks for (1–30 days). */
export const NEW_EPISODES_DAYS = 7;

/** How often the integration is polled while a code waits or a sync runs. */
export const TRAKT_POLL_MS = 3000;

export const traktApi = {
  integration: () => apiJson<TraktIntegration>('/api/integrations/trakt'),
  /** A device code; a live one comes back again. 409 while connected, 503 without a Trakt app. */
  connect: () => apiJson<TraktConnect>('/api/integrations/trakt/connect', { method: 'POST' }),
  /** Starts a sync, or joins the running one (202). 409 unless connected. */
  sync: () => apiJson<TraktIntegration>('/api/integrations/trakt/sync', { method: 'POST' }),
  /** Cancels a pending code, or disconnects and purges the imported data (settings stay). */
  disconnect: () => apiJson<void>('/api/integrations/trakt', { method: 'DELETE' }),
  updateSettings: (patch: TraktSettingsPatch) =>
    apiJson<TraktSettings>('/api/integrations/trakt/settings', { method: 'PUT', json: patch }),
  upNext: () => apiJson<{ items: UpNextItem[] }>('/api/me/up-next'),
  newEpisodes: (days = NEW_EPISODES_DAYS) => apiJson<{ items: NewEpisodeItem[] }>(`/api/me/new-episodes${toQuery({ days })}`),
  watchlist: (type?: MediaType) => apiJson<{ items: WatchlistItem[] }>(`/api/me/watchlist${toQuery({ type })}`),
  history: (page = 1, pageSize?: number) =>
    apiJson<Paginated<HistoryItem>>(`/api/me/history${toQuery({ page, page_size: pageSize })}`),
};

/** Polls while a device code waits for approval or a sync runs (never in a hidden tab). */
export const traktIntegrationQuery = () =>
  queryOptions({
    queryKey: traktKeys.integration,
    queryFn: traktApi.integration,
    refetchInterval: (query) => {
      const data = query.state.data;
      return data && (data.state === 'pending' || data.sync.status === 'syncing') ? TRAKT_POLL_MS : false;
    },
    refetchIntervalInBackground: false,
  });

export const upNextQuery = () => queryOptions({ queryKey: traktKeys.upNext, queryFn: traktApi.upNext });
export const newEpisodesQuery = (days = NEW_EPISODES_DAYS) =>
  queryOptions({ queryKey: traktKeys.newEpisodes(days), queryFn: () => traktApi.newEpisodes(days) });
export const watchlistQuery = (type?: MediaType) =>
  queryOptions({ queryKey: traktKeys.watchlist(type), queryFn: () => traktApi.watchlist(type) });
export const historyQuery = (page = 1, pageSize?: number) =>
  queryOptions({ queryKey: traktKeys.history(page, pageSize), queryFn: () => traktApi.history(page, pageSize) });
