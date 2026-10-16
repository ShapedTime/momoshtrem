/** @jest-environment node */
import { server } from '@/mocks/node';
import { resetMockState } from '@/mocks/handlers';
import { setMockScenario } from '@/mocks/scenario';
import { titleSearchApi } from '@/lib/api/endpoints';

let mswFetch: typeof fetch;
beforeAll(() => {
  server.listen({ onUnhandledRequest: 'error' });
  mswFetch = global.fetch;
  global.fetch = ((input: RequestInfo | URL, init?: RequestInit) =>
    mswFetch(typeof input === 'string' ? new URL(input, 'http://localhost') : input, init)) as typeof fetch;
});
afterEach(resetMockState);
afterAll(() => {
  global.fetch = mswFetch;
  server.close();
});

const titles = (items: Array<{ title: string; year: number | null }>) => items.map((item) => `${item.title} (${item.year})`);

describe('mock GET /api/title-search', () => {
  it('finds the six Dunes, best first', async () => {
    const result = await titleSearchApi.search('dune');
    expect(titles(result.items)).toEqual([
      'Dune (2021)',
      'Dune (1984)',
      'Dune: Part Two (2024)',
      'Dune: Prophecy (2024)',
      'Jodorowsky’s Dune (2013)',
      'Frank Herbert’s Dune (2000)',
    ]);
    expect(result.counts).toEqual({ all: 6, movie: 4, tv: 2 });
  });

  it('finds Frieren by its romanised title', async () => {
    const { items } = await titleSearchApi.search('sousou no frieren');
    expect(items).toEqual([expect.objectContaining({ tmdb_id: 209867, match_reason: 'Original title match' })]);
  });

  it('finds a person’s titles, with why', async () => {
    const { items, counts } = await titleSearchApi.search('vince gilligan');
    expect(items.map((item) => [item.title, item.match_reason])).toEqual([
      ['Breaking Bad', 'Created by Vince Gilligan'],
      ['Better Call Saul', 'Created by Vince Gilligan'],
      ['El Camino: A Breaking Bad Movie', 'Written by Vince Gilligan'],
    ]);
    expect(counts).toEqual({ all: 3, movie: 1, tv: 2 });
  });

  it('finds nothing for nonsense', async () => {
    expect(await titleSearchApi.search('zzxq')).toEqual({ query: 'zzxq', items: [], counts: { all: 0, movie: 0, tv: 0 } });
  });

  it('answers the error scenario like the route', async () => {
    setMockScenario('title-search-error');
    await expect(titleSearchApi.search('dune')).rejects.toMatchObject({ status: 502, code: 'tmdb_unavailable' });
  });
});
