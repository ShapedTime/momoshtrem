import { http, HttpResponse } from 'msw';
import type { AiSettings } from '@/lib/api/contract';
import { AI_SETTINGS, AI_USAGE } from '../fixtures/settings';

let ai: AiSettings = { ...AI_SETTINGS };

export const settingsHandlers = [
  http.get('*/api/settings/ai', () => HttpResponse.json(ai)),
  http.put('*/api/settings/ai', async ({ request }) => {
    ai = (await request.json()) as AiSettings;
    return HttpResponse.json(ai);
  }),
  http.get('*/api/settings/ai/usage', () => HttpResponse.json(AI_USAGE)),
];
