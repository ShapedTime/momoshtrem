import type { AiSettings, AiUsage, QualityProfile } from '@/lib/api/contract';

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

/** The prototype's profile (screens/settings/quality-*), not the backend's defaults. */
export const QUALITY_PROFILE: QualityProfile = {
  preferred_resolution: '2160p',
  max_resolution: '2160p',
  max_gb_per_hour: 8,
  prefer_x265: true,
  prefer_hdr: true,
  prefer_dolby_vision: true,
  avoid_terms: ['CAM', 'TS', 'HDCAM', 'TELESYNC', 'HC', 'KORSUB'],
  preferred_groups: ['FLUX', 'NTb', 'SubsPlease', 'VARYG'],
};
