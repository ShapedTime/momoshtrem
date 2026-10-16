import { delay, http, HttpResponse } from 'msw';
import type { MediaType, NewEpisodeItem, TraktSettings, TraktSettingsPatch, UpNextItem, WatchlistItem } from '@/lib/api/contract';
import {
  connectTrakt,
  DEFAULT_HISTORY_PAGE_SIZE,
  disconnectTrakt,
  historyPage,
  newEpisodeItems,
  readTraktIntegration,
  resetMockTrakt,
  startTraktSync,
  traktImported,
  traktSettings,
  updateTraktSettings,
  upNextItems,
  watchlistItems,
} from '../fixtures/trakt';
import { scenario } from '../scenario';
import { mockLibrary } from './titles';
import { apiError, isRecord, jsonBody, latency, noContent } from './util';

/** Tests: back to the starting Trakt account (fixtures/trakt.ts keeps it, so /status can read it). */
export { resetMockTrakt };

const SETTINGS: ReadonlyArray<keyof TraktSettings> = ['use_ratings', 'show_watchlist', 'include_paused'];
const MAX_HISTORY_PAGE_SIZE = 200;

/** `me-error` (and `for-you-no-hero`): every /me read fails. */
const meError = () =>
  scenario() === 'me-error' || scenario() === 'for-you-no-hero' ? apiError(500, 'internal_error', 'Couldn’t read what Trakt imported.') : null;

/** A whole number from min to max, the default when absent, or null when it's anything else. */
function intParam(raw: string | null, fallback: number, min: number, max: number): number | null {
  if (raw === null || raw === '') return fallback;
  const n = Number(raw);
  return Number.isInteger(n) && n >= min && n <= max ? n : null;
}

export const traktHandlers = [
  http.get('*/api/integrations/trakt', async () => {
    if (scenario() === 'trakt-loading') await delay('infinite');
    await latency();
    return HttpResponse.json(readTraktIntegration(mockLibrary()));
  }),
  http.post('*/api/integrations/trakt/connect', async () => {
    await latency();
    const result = connectTrakt();
    return result.ok ? HttpResponse.json(result.value) : apiError(result.status, result.code, result.message);
  }),
  http.post('*/api/integrations/trakt/sync', async () => {
    await latency();
    const result = startTraktSync(mockLibrary());
    return result.ok ? HttpResponse.json(result.value, { status: 202 }) : apiError(result.status, result.code, result.message);
  }),
  http.put('*/api/integrations/trakt/settings', async ({ request }) => {
    await latency();
    const body = await jsonBody(request);
    if (!isRecord(body) || SETTINGS.some((field) => body[field] !== undefined && typeof body[field] !== 'boolean')) {
      return apiError(400, 'bad_request', 'use_ratings, show_watchlist and include_paused must be true or false.');
    }
    const patch: TraktSettingsPatch = {};
    for (const field of SETTINGS) {
      const value = body[field];
      if (typeof value === 'boolean') patch[field] = value;
    }
    return HttpResponse.json(updateTraktSettings(patch));
  }),
  http.delete('*/api/integrations/trakt', async () => {
    await latency();
    disconnectTrakt();
    return noContent();
  }),

  http.get('*/api/me/up-next', async () => {
    await latency();
    const failed = meError();
    if (failed) return failed;
    const items: UpNextItem[] = traktImported('progress') && scenario() !== 'up-next-empty' ? upNextItems(mockLibrary(), traktSettings()) : [];
    return HttpResponse.json({ items });
  }),
  http.get('*/api/me/new-episodes', async ({ request }) => {
    await latency();
    const failed = meError();
    if (failed) return failed;
    const days = intParam(new URL(request.url).searchParams.get('days'), 7, 1, 30);
    if (days === null) return apiError(400, 'bad_request', 'days must be a whole number from 1 to 30');
    // Without Trakt, library shows still count (nothing is watched)
    const items: NewEpisodeItem[] = newEpisodeItems(mockLibrary(), days, traktImported('history'));
    return HttpResponse.json({ items });
  }),
  http.get('*/api/me/watchlist', async ({ request }) => {
    await latency();
    const failed = meError();
    if (failed) return failed;
    const type = new URL(request.url).searchParams.get('type') || undefined;
    if (type !== undefined && type !== 'movie' && type !== 'tv') return apiError(400, 'bad_request', 'type must be movie or tv');
    const items: WatchlistItem[] = traktImported('watchlist') ? watchlistItems(mockLibrary(), type as MediaType | undefined) : [];
    return HttpResponse.json({ items });
  }),
  http.get('*/api/me/history', async ({ request }) => {
    await latency();
    const failed = meError();
    if (failed) return failed;
    const search = new URL(request.url).searchParams;
    const page = intParam(search.get('page'), 1, 1, Number.MAX_SAFE_INTEGER);
    const pageSize = intParam(search.get('page_size'), DEFAULT_HISTORY_PAGE_SIZE, 1, MAX_HISTORY_PAGE_SIZE);
    if (page === null || pageSize === null) return apiError(400, 'bad_request', 'page and page_size must be positive whole numbers (page_size at most 200)');
    return HttpResponse.json(historyPage(page, pageSize, traktImported('history')));
  }),
];
