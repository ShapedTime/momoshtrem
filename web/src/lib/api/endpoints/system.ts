import { queryOptions } from '@tanstack/react-query';
import { apiJson } from '../client';
import type { SystemStatus } from '../contract';

/** System status (PLAN §6 "System"). */
export const systemKeys = {
  status: ['status'] as const,
};

export const systemApi = {
  status: (refresh = false) => apiJson<SystemStatus>(`/api/status${refresh ? '?refresh=1' : ''}`),
};

/** Re-checked every 60 s while the tab is visible; the backend caches probes for 30 s. */
export const statusQuery = () =>
  queryOptions({ queryKey: systemKeys.status, queryFn: () => systemApi.status(), refetchInterval: 60_000 });
