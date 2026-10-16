import { delay, http, HttpResponse } from 'msw';
import type { LibraryItem, LibraryPage } from '@/lib/api/contract';
import { parseLibrarySearch, queryLibrary } from '@/lib/library/query';
import { libraryItems } from '../fixtures/library';
import { scenario } from '../scenario';
import { mockLibrary } from './titles';
import { apiError, latency } from './util';

/** The page's library, as the `library-*` scenarios shape it. */
function items(): LibraryItem[] {
  switch (scenario()) {
    case 'library-empty':
    case 'first-time':
      return [];
    case 'library-reviewed':
      return libraryItems(mockLibrary()).map((item) => ({ ...item, needs_review: 0, review_job_id: null }));
    default:
      return libraryItems(mockLibrary());
  }
}

/** GET /library over the page's library: filters, sorts, pages, counts, summary and the ids lookup (lib/library/query.ts). */
export const libraryHandlers = [
  http.get('*/api/library', async ({ request }) => {
    if (scenario() === 'library-loading') await delay('infinite');
    await latency();
    if (scenario() === 'library-error') return apiError(500, 'internal_error', 'The library query failed.');
    const parsed = parseLibrarySearch(new URL(request.url).searchParams);
    if (!parsed.ok) return apiError(400, 'bad_request', parsed.message);
    const page: LibraryPage = queryLibrary(items(), parsed.params);
    return HttpResponse.json(page);
  }),
];
