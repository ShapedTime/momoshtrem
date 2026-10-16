import { queryOptions } from '@tanstack/react-query';
import { apiJson } from '../client';
import type { AiSettings, AiSettingsPatch, AiUsage } from '../contract';

/** AI settings and usage (PLAN §6 "Recommendations (R1–R3)"; #9). The quality profile lives in search.ts. */
export const settingsKeys = {
  ai: ['settings', 'ai'] as const,
  aiUsage: ['settings', 'ai', 'usage'] as const,
};

export const settingsApi = {
  ai: () => apiJson<AiSettings>('/api/settings/ai'),
  /** Partial: send only what changed. 422 names the field (`ApiError.field`). Answers every setting. */
  putAi: (patch: AiSettingsPatch) => apiJson<AiSettings>('/api/settings/ai', { method: 'PUT', json: patch }),
  aiUsage: () => apiJson<AiUsage>('/api/settings/ai/usage'),
};

export const aiSettingsQuery = () => queryOptions({ queryKey: settingsKeys.ai, queryFn: settingsApi.ai });

/** Polls every 3 s while a recommendation run is active ("Run now"), never in a hidden tab. */
export const aiUsageQuery = () =>
  queryOptions({
    queryKey: settingsKeys.aiUsage,
    queryFn: settingsApi.aiUsage,
    refetchInterval: (query) => (query.state.data?.recs.active_run ? 3000 : false),
  });
