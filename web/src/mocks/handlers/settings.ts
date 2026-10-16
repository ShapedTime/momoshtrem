import { http, HttpResponse } from 'msw';
import type { AiSettings, AiSettingsPatch, ApiErrorBody, ClassifierModel, QualityProfile, RecModel, RecSchedule } from '@/lib/api/contract';
import { type QualityFieldError, validateQualityProfile } from '@/lib/settings/quality';
import { aiUsage } from '../fixtures/recommendations';
import { mockAiSettings, QUALITY_PROFILE, resetMockAiSettings, updateMockAiSettings } from '../fixtures/settings';
import { scenario } from '../scenario';
import { apiError, isRecord, jsonBody, latency } from './util';

let profile: QualityProfile = { ...QUALITY_PROFILE };

const tidy = (list: string[]) =>
  list.map((item) => item.trim()).filter((item, i, all) => all.findIndex((other) => other.toLowerCase() === item.toLowerCase()) === i);

/** This page's AI settings (mapping jobs read `classifier_enabled` and `auto_apply_high_confidence`; picks read the rest). */
export { mockAiSettings };

/** This page's quality profile as Settings › Quality saved it (torrent search ranks with it). */
export const getMockQualityProfile = () => profile;

/** Tests: back to the fixture's AI settings and quality profile. */
export function resetMockSettings(): void {
  resetMockAiSettings();
  profile = { ...QUALITY_PROFILE };
}

const REC_MODELS: readonly RecModel[] = ['claude-opus-5-5', 'claude-sonnet-5-5'];
const CLASSIFIER_MODELS: readonly ClassifierModel[] = ['claude-haiku-5-5', 'claude-sonnet-5-5', 'claude-opus-5-5'];
const SCHEDULES: readonly RecSchedule[] = ['nightly', 'weekly', 'manual'];

type Check = (value: unknown) => string | null;
const oneOf =
  (field: string, allowed: readonly string[]): Check =>
  (value) =>
    typeof value === 'string' && allowed.includes(value) ? null : `${field} must be ${allowed.slice(0, -1).join(', ')} or ${allowed.at(-1)}`;
const boolean =
  (field: string): Check =>
  (value) =>
    typeof value === 'boolean' ? null : `${field} must be true or false`;

/** PUT /settings/ai's checks, as the backend's: each known field that's sent; unknown fields are ignored. */
const CHECKS: Record<keyof AiSettings, Check> = {
  rec_enabled: boolean('rec_enabled'),
  rec_model: oneOf('rec_model', REC_MODELS),
  rec_schedule: oneOf('rec_schedule', SCHEDULES),
  monthly_budget_usd: (value) =>
    value === null || (typeof value === 'number' && Number.isFinite(value) && value >= 0) ? null : 'monthly_budget_usd must be 0 or more, or null for no cap',
  classifier_enabled: boolean('classifier_enabled'),
  classifier_model: oneOf('classifier_model', CLASSIFIER_MODELS),
  auto_apply_high_confidence: boolean('auto_apply_high_confidence'),
};

export const settingsHandlers = [
  http.get('*/api/settings/ai', async () => {
    await latency();
    return HttpResponse.json(mockAiSettings());
  }),
  http.put('*/api/settings/ai', async ({ request }) => {
    await latency();
    const body = await jsonBody(request);
    if (!isRecord(body)) return apiError(400, 'bad_request', 'Send the settings to change as a JSON object.');
    if (scenario() === 'ai-budget-invalid' && 'monthly_budget_usd' in body) {
      return apiError(422, 'validation_error', CHECKS.monthly_budget_usd(-1) ?? 'monthly_budget_usd is invalid', { field: 'monthly_budget_usd' });
    }
    const patch: Record<string, unknown> = {};
    for (const [field, check] of Object.entries(CHECKS)) {
      if (!(field in body)) continue;
      const message = check(body[field]);
      if (message) return apiError(422, 'validation_error', message, { field });
      patch[field] = body[field];
    }
    return HttpResponse.json(updateMockAiSettings(patch as AiSettingsPatch));
  }),
  http.get('*/api/settings/ai/usage', async () => {
    await latency();
    return HttpResponse.json(aiUsage());
  }),
  http.get('*/api/settings/quality-profile', () => HttpResponse.json(profile)),
  http.put('*/api/settings/quality-profile', async ({ request }) => {
    await latency();
    const body = await jsonBody(request);
    if (!isRecord(body)) return HttpResponse.json<ApiErrorBody>({ error: { code: 'bad_request', message: 'Send the whole profile as JSON.' } }, { status: 400 });
    const sent = { ...QUALITY_PROFILE, ...body } as QualityProfile;
    // Like the backend: lists trimmed and deduped (the first spelling stays) before the checks
    const next = { ...sent, avoid_terms: tidy(sent.avoid_terms), preferred_groups: tidy(sent.preferred_groups) };
    const invalid: QualityFieldError | null =
      scenario() === 'quality-invalid' ? { field: 'avoid_terms', message: 'avoid_terms must be 1–30 characters each' } : validateQualityProfile(next);
    if (invalid) return HttpResponse.json<ApiErrorBody>({ error: { code: 'validation_error', ...invalid } }, { status: 422 });
    profile = next;
    return HttpResponse.json(profile);
  }),
];
