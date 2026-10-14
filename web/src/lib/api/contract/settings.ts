/** AI settings and usage (R2; #8/#9). Quality profile lives in search.ts. */

/** GET/PUT /settings/ai */
export interface AiSettings {
  rec_enabled: boolean;
  rec_model: string;
  rec_schedule_cron: string;
  monthly_budget_usd: number;
  classifier_enabled: boolean;
  classifier_model: string;
  auto_apply_high_confidence: boolean;
}

/** GET /settings/ai/usage: month to date */
export interface AiUsage {
  month: string;
  total_usd: number;
  budget_usd: number;
  by_feature: { feature: 'classifier' | 'recs' | 'ask'; cost_usd: number; requests: number }[];
}
