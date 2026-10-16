import { delay, http, HttpResponse } from 'msw';
import { cleanQuery, MAX_QUERY_LENGTH } from '@/lib/title-search/text';
import { mockTitleSearch } from '../fixtures/title-search';
import { scenario } from '../scenario';
import { apiError, latency } from './util';

/** GET /api/title-search over the fixtures (scenarios: `title-search-error`, `title-search-loading`). */
export const titleSearchHandlers = [
  http.get('*/api/title-search', async ({ request }) => {
    if (scenario() === 'title-search-loading') await delay('infinite');
    await latency();
    if (scenario() === 'title-search-error') return apiError(502, 'tmdb_unavailable', 'TMDB didn’t answer. Try again in a moment.');
    const query = cleanQuery(new URL(request.url).searchParams.get('q'));
    if (!query) return apiError(400, 'bad_request', `q must be 1 to ${MAX_QUERY_LENGTH} characters.`);
    return HttpResponse.json(mockTitleSearch(query));
  }),
];
