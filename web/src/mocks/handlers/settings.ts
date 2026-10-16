import { http, HttpResponse } from 'msw';
import type { AiSettings, ApiErrorBody, QualityProfile } from '@/lib/api/contract';
import { type QualityFieldError, validateQualityProfile } from '@/lib/settings/quality';
import { AI_SETTINGS, AI_USAGE, QUALITY_PROFILE } from '../fixtures/settings';
import { scenario } from '../scenario';
import { isRecord, jsonBody, latency } from './util';

let ai: AiSettings = { ...AI_SETTINGS };
let profile: QualityProfile = { ...QUALITY_PROFILE };

/** This page's AI settings (mapping jobs read `classifier_enabled` and `auto_apply_high_confidence`). */
export const mockAiSettings = () => ai;

/** Tests: back to the fixture's AI settings and quality profile. */
export function resetMockSettings(): void {
  ai = { ...AI_SETTINGS };
  profile = { ...QUALITY_PROFILE };
}

export const settingsHandlers = [
  http.get('*/api/settings/ai', () => HttpResponse.json(ai)),
  http.put('*/api/settings/ai', async ({ request }) => {
    ai = (await request.json()) as AiSettings;
    return HttpResponse.json(ai);
  }),
  http.get('*/api/settings/ai/usage', () => HttpResponse.json(AI_USAGE)),
  http.get('*/api/settings/quality-profile', () => HttpResponse.json(profile)),
  http.put('*/api/settings/quality-profile', async ({ request }) => {
    await latency();
    const body = await jsonBody(request);
    if (!isRecord(body)) return HttpResponse.json<ApiErrorBody>({ error: { code: 'bad_request', message: 'Send the whole profile as JSON.' } }, { status: 400 });
    const next = { ...QUALITY_PROFILE, ...body } as QualityProfile;
    const invalid: QualityFieldError | null =
      scenario() === 'quality-invalid' ? { field: 'avoid_terms', message: 'Each term needs 1 to 30 characters.' } : validateQualityProfile(next);
    if (invalid) return HttpResponse.json<ApiErrorBody>({ error: { code: 'validation_error', ...invalid } }, { status: 422 });
    profile = next;
    return HttpResponse.json(profile);
  }),
];
