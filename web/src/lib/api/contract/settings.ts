import type { Timestamp } from './common';
import type { RunProgress, RunSummary } from './recommendations';

/** AI settings and usage (PLAN §6 "Recommendations (R1–R3)"; #9). The quality profile lives in search.ts. */

export type RecModel = 'claude-opus-5-5' | 'claude-sonnet-5-5';
export type ClassifierModel = 'claude-haiku-5-5' | 'claude-sonnet-5-5' | 'claude-opus-5-5';
/** At 03:00 server-local time; weekly runs on Sunday. */
export type RecSchedule = 'nightly' | 'weekly' | 'manual';
export type AiFeature = 'recs' | 'ask' | 'classifier';

/** GET/PUT /settings/ai: app_settings key `ai`. Readable and writable without an API key. */
export interface AiSettings {
  /** The master switch for picks: shelves, hero picks, runs and title reasons. Ask doesn't depend on it. */
  rec_enabled: boolean;
  /** Picks and Ask; a thread keeps the model it started on. */
  rec_model: RecModel;
  rec_schedule: RecSchedule;
  /** Shared by every AI feature, per server-local calendar month; null is no cap. 15 on a fresh install. */
  monthly_budget_usd: number | null;
  classifier_enabled: boolean;
  classifier_model: ClassifierModel;
  auto_apply_high_confidence: boolean;
}

/** PUT /settings/ai is partial: fields left out keep their value, unknown ones are ignored, a bad one is 422 naming it. */
export type AiSettingsPatch = Partial<AiSettings>;

/** GET /settings/ai/usage: this server-local month. */
export interface AiUsage {
  /** ANTHROPIC_API_KEY is set. */
  configured: boolean;
  /** "2026-10" */
  month: string;
  /** The 1st of next month, 00:00 server-local. */
  resets_at: Timestamp;
  total_usd: number;
  budget_usd: number | null;
  /** total_usd ≥ budget_usd: runs are skipped, Ask answers 429 and mapping runs on rules. */
  budget_reached: boolean;
  /** Always all three, in this order; `requests` counts API calls. */
  by_feature: Array<{ feature: AiFeature; cost_usd: number; requests: number }>;
  recs: {
    /** The newest finished run: succeeded, failed or skipped. */
    last_run: RunSummary | null;
    active_run: RunProgress | null;
    /** The schedule's next run; null when manual, off or not configured. */
    next_run_at: Timestamp | null;
  };
  estimates: {
    /** The mean cost of the last 5 succeeded runs on each model; null before any. */
    run_usd: Record<RecModel, number | null>;
    /** The mean cost of the last 20 done Ask turns; null before any. */
    ask_usd: number | null;
  };
  /** Ask turns started today (server-local), cancelled and failed ones included. */
  asks_today: number;
  ask_daily_limit: number;
}
