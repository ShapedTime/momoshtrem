import type { AiSettings, AiUsage } from '@/lib/api/contract';

export const AI_SETTINGS: AiSettings = {
  rec_enabled: true,
  rec_model: 'claude-opus-5-5',
  rec_schedule_cron: '0 4 * * *',
  monthly_budget_usd: 10,
  classifier_enabled: true,
  classifier_model: 'claude-haiku-5-5',
  auto_apply_high_confidence: true,
};

export const AI_USAGE: AiUsage = {
  month: '2026-10',
  total_usd: 3.12,
  budget_usd: 10,
  by_feature: [
    { feature: 'recs', cost_usd: 2.61, requests: 14 },
    { feature: 'ask', cost_usd: 0.43, requests: 9 },
    { feature: 'classifier', cost_usd: 0.08, requests: 31 },
  ],
};
