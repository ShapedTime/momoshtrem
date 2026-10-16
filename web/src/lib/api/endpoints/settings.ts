import { queryOptions } from '@tanstack/react-query';
import { ApiError, apiJson, isApiError } from '../client';
import type { AiSettings, AiSettingsPatch, AiUsage, LimitBelowCommittedDetails, StorageSettings, StorageSettingsPatch } from '../contract';

/**
 * AI settings and usage (PLAN §6 "Recommendations (R1–R3)"; #9), and storage
 * (PLAN §6 "Storage (#77)"). The quality profile lives in search.ts.
 */
export const settingsKeys = {
  ai: ['settings', 'ai'] as const,
  aiUsage: ['settings', 'ai', 'usage'] as const,
  storage: ['settings', 'storage'] as const,
};

export const settingsApi = {
  ai: () => apiJson<AiSettings>('/api/settings/ai'),
  /** Partial: send only what changed. 422 names the field (`ApiError.field`). Answers every setting. */
  putAi: (patch: AiSettingsPatch) => apiJson<AiSettings>('/api/settings/ai', { method: 'PUT', json: patch }),
  aiUsage: () => apiJson<AiUsage>('/api/settings/ai/usage'),
  storage: () => apiJson<StorageSettings>('/api/settings/storage'),
  /**
   * Partial: send only what changed (nested objects merge, `windows` is replaced whole). 422 names
   * the field (`ApiError.field`: `limit_bytes`, `kept.max_bytes`, `background.windows`, …); 409
   * `limit_below_committed` (`isLimitBelowCommittedError`). Answers every setting.
   */
  putStorage: (patch: StorageSettingsPatch) => apiJson<StorageSettings>('/api/settings/storage', { method: 'PUT', json: patch }),
};

export const aiSettingsQuery = () => queryOptions({ queryKey: settingsKeys.ai, queryFn: settingsApi.ai });

/** Polls every 3 s while a recommendation run is active ("Run now"), never in a hidden tab. */
export const aiUsageQuery = () =>
  queryOptions({
    queryKey: settingsKeys.aiUsage,
    queryFn: settingsApi.aiUsage,
    refetchInterval: (query) => (query.state.data?.recs.active_run ? 3000 : false),
  });

/** The disk and usage move with downloads; Settings › Storage refetches on focus. */
export const storageSettingsQuery = () => queryOptions({ queryKey: settingsKeys.storage, queryFn: settingsApi.storage });

/** PUT /settings/storage's 409: the new kept cap would be below what kept downloads already need. */
export function isLimitBelowCommittedError(error: unknown): error is ApiError & { details: LimitBelowCommittedDetails } {
  return (
    isApiError(error) && error.status === 409 && error.code === 'limit_below_committed' && typeof error.details.committed_kept_bytes === 'number'
  );
}
