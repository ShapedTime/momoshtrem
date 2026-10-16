import type { AiFeature, AiSettings, AiSettingsPatch, QualityProfile } from '@/lib/api/contract';
import { type MockScenario, scenario } from '../scenario';

/** A fresh install's AI settings (PLAN §6): picks on, Opus 5.5 nightly, a $15 budget every feature shares. */
export const AI_SETTINGS: AiSettings = {
  rec_enabled: true,
  rec_model: 'claude-opus-5-5',
  rec_schedule: 'nightly',
  monthly_budget_usd: 15,
  classifier_enabled: true,
  classifier_model: 'claude-haiku-5-5',
  auto_apply_high_confidence: true,
};

/**
 * The page's AI settings, kept here (not in handlers/settings.ts) so the
 * recommendations, mapping and status mocks read them without msw. A new
 * scenario starts over: `recs-off` with picks off, `ask-budget` with the
 * design's $5 budget (all spent).
 */
let aiSettings: { from: MockScenario | null; value: AiSettings } | undefined;

function startingAiSettings(from: MockScenario | null): AiSettings {
  if (from === 'recs-off') return { ...AI_SETTINGS, rec_enabled: false };
  if (from === 'ask-budget') return { ...AI_SETTINGS, monthly_budget_usd: 5 };
  return { ...AI_SETTINGS };
}

export function mockAiSettings(): AiSettings {
  const from = scenario();
  if (!aiSettings || aiSettings.from !== from) aiSettings = { from, value: startingAiSettings(from) };
  return aiSettings.value;
}

/** PUT /settings/ai, once validated: fields left out keep their value. */
export function updateMockAiSettings(patch: AiSettingsPatch): AiSettings {
  const current = mockAiSettings();
  Object.assign(current, patch);
  return current;
}

/** Tests: back to a fresh install's AI settings (rebuilt for the scenario on the next read). */
export function resetMockAiSettings(): void {
  aiSettings = undefined;
}

/** ANTHROPIC_API_KEY is set: everywhere but `ai-off`. */
export const aiConfigured = (): boolean => scenario() !== 'ai-off';

/**
 * This month's spend before the page did anything (the design's Settings ›
 * AI & picks: $3.12 in all). Runs and asks on the page add to it
 * (fixtures/recommendations.ts).
 */
export const AI_SPEND: Record<AiFeature, { cost_usd: number; requests: number }> = {
  recs: { cost_usd: 2.18, requests: 41 },
  ask: { cost_usd: 0.53, requests: 23 },
  classifier: { cost_usd: 0.41, requests: 96 },
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
