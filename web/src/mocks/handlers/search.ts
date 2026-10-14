import { http, HttpResponse } from 'msw';
import type { QualityProfile } from '@/lib/api/contract';
import { QUALITY_PROFILE, searchTorrents } from '../fixtures/search';
import { accepted, latency } from './util';

let profile: QualityProfile = { ...QUALITY_PROFILE };

export const searchHandlers = [
  http.get('*/api/search/torrents', async ({ request }) => {
    await latency();
    const url = new URL(request.url);
    return HttpResponse.json(searchTorrents(Number(url.searchParams.get('page') ?? 1), Number(url.searchParams.get('page_size') ?? 25)));
  }),
  http.post('*/api/search/torrents/inspect', async () => accepted({ job_id: 201 })),
  http.get('*/api/settings/quality-profile', () => HttpResponse.json(profile)),
  http.put('*/api/settings/quality-profile', async ({ request }) => {
    profile = (await request.json()) as QualityProfile;
    return HttpResponse.json(profile);
  }),
];
