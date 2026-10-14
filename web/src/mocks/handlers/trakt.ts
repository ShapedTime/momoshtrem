import { http, HttpResponse } from 'msw';
import type { TraktConnect } from '@/lib/api/contract';
import { history, TRAKT, UP_NEXT, WATCHLIST } from '../fixtures/trakt';
import { latency } from './util';

export const traktHandlers = [
  http.get('*/api/integrations/trakt', () => HttpResponse.json(TRAKT)),
  http.post('*/api/integrations/trakt/connect', () => {
    const body: TraktConnect = {
      user_code: 'A1B2C3D4',
      verification_url: 'https://trakt.tv/activate',
      expires_at: new Date(Date.now() + 10 * 60_000).toISOString(),
      interval: 5,
    };
    return HttpResponse.json(body);
  }),
  http.post('*/api/integrations/trakt/sync', () => new HttpResponse(null, { status: 202 })),
  http.delete('*/api/integrations/trakt', () => new HttpResponse(null, { status: 204 })),
  http.get('*/api/me/up-next', async () => {
    await latency();
    return HttpResponse.json({ items: UP_NEXT });
  }),
  http.get('*/api/me/watchlist', async ({ request }) => {
    const type = new URL(request.url).searchParams.get('type');
    return HttpResponse.json({ items: WATCHLIST.filter((item) => !type || item.media_type === type) });
  }),
  http.get('*/api/me/history', ({ request }) => HttpResponse.json(history(Number(new URL(request.url).searchParams.get('page') ?? 1)))),
];
