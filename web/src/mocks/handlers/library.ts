import { http, HttpResponse } from 'msw';
import type { Paginated, LibraryItem } from '@/lib/api/contract';
import { LIBRARY } from '../fixtures/library';
import { latency } from './util';

export const libraryHandlers = [
  http.get('*/api/library', async ({ request }) => {
    await latency();
    const url = new URL(request.url);
    const type = url.searchParams.get('type');
    const q = url.searchParams.get('q')?.toLowerCase();
    const items = LIBRARY.filter((item) => (!type || item.media_type === type) && (!q || item.title.toLowerCase().includes(q)));
    const body: Paginated<LibraryItem> = { items, total: items.length, page: 1, page_size: 50 };
    return HttpResponse.json(body);
  }),
];
