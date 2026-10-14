import { queryOptions } from '@tanstack/react-query';
import { apiJson } from '../client';
import type { AiSettings, AiUsage } from '../contract';

/** AI settings and usage (PLAN §6; #8/#9). The quality profile lives in search.ts. */
export const settingsKeys = {
  ai: ['settings', 'ai'] as const,
  aiUsage: ['settings', 'ai', 'usage'] as const,
};

export const settingsApi = {
  ai: () => apiJson<AiSettings>('/api/settings/ai'),
  putAi: (settings: AiSettings) => apiJson<AiSettings>('/api/settings/ai', { method: 'PUT', json: settings }),
  aiUsage: () => apiJson<AiUsage>('/api/settings/ai/usage'),
};

export const aiSettingsQuery = () => queryOptions({ queryKey: settingsKeys.ai, queryFn: settingsApi.ai });
export const aiUsageQuery = () => queryOptions({ queryKey: settingsKeys.aiUsage, queryFn: settingsApi.aiUsage });
