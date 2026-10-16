import type { IndexerReport, Release, SearchTorrentsResponse } from '@/lib/api/contract';
import { bestRelease, packBanner, searchViewKind } from '@/lib/search/view';

const release = (id: string) => ({ id }) as Release;
const ok: IndexerReport = { name: 'Nyaa', status: 'ok', count: 1, ms: 200 };

function response(patch: Partial<SearchTorrentsResponse> = {}): SearchTorrentsResponse {
  return {
    items: [],
    total: 0,
    page: 1,
    page_size: 25,
    total_unfiltered: 0,
    hidden: 0,
    best_id: null,
    facets: { resolution: [], source: [], codec: [], hdr: [], audio: [], pack: [], indexer: [], group: [] },
    queries: [],
    indexers: [ok],
    complete: true,
    missing: null,
    cached_at: '2026-10-14T00:00:00Z',
    warnings: [],
    ...patch,
  };
}

describe('searchViewKind', () => {
  it('searching before the first answer, and while nothing has arrived', () => {
    expect(searchViewKind(undefined, 0, false)).toBe('searching');
    expect(searchViewKind(response({ complete: false }), 0, false)).toBe('searching');
  });

  it('results as soon as anything arrived, even while the search runs', () => {
    expect(searchViewKind(response({ complete: false }), 3, false)).toBe('results');
    expect(searchViewKind(response(), 3, false)).toBe('results');
  });

  it('error without an answer; failed when every indexer failed; filtered-empty; none', () => {
    expect(searchViewKind(undefined, 0, true)).toBe('error');
    expect(searchViewKind(response({ indexers: [{ name: 'TPB', status: 'timeout', count: 0, ms: 8000 }] }), 0, false)).toBe('failed');
    expect(searchViewKind(response({ total_unfiltered: 27 }), 0, false)).toBe('filtered-empty');
    expect(searchViewKind(response(), 0, false)).toBe('none');
  });
});

describe('bestRelease', () => {
  it('is page 1’s lead when it’s the best match and the sort is best', () => {
    const first = response({ items: [release('a'), release('b')], best_id: 'a' });
    expect(bestRelease(first, 'best')?.id).toBe('a');
    expect(bestRelease(first, 'seeders')).toBeNull();
    expect(bestRelease(response({ items: [release('b'), release('a')], best_id: 'a' }), 'best')).toBeNull();
    expect(bestRelease(response({ items: [release('a')], best_id: null }), 'best')).toBeNull();
  });
});

describe('packBanner', () => {
  const missing = { season_number: 1, from: 29, to: 38, absolute_from: 29, absolute_to: 38, count: 10, best_covers: true };

  it('shows in episode scope for a run of more than one', () => {
    expect(packBanner(response({ missing }), 'episode')).toEqual(missing);
    expect(packBanner(response({ missing }), 'season')).toBeNull();
    expect(packBanner(response({ missing: { ...missing, to: 29, count: 1 } }), 'episode')).toBeNull();
    expect(packBanner(response(), 'episode')).toBeNull();
  });
});
