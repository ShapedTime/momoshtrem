import { http, HttpResponse } from 'msw';
import type { AiSettings, QualityProfile } from '@/lib/api/contract';
import { AI_SETTINGS, AI_USAGE, QUALITY_PROFILE } from '../fixtures/settings';

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
    profile = (await request.json()) as QualityProfile;
    return HttpResponse.json(profile);
  }),
];
