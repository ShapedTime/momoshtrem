/** @jest-environment node */
import { server } from '@/mocks/node';
import { resetMockState } from '@/mocks/handlers';
import { isApiError } from '@/lib/api/client';
import { downloadsApi, torrentsApi } from '@/lib/api/endpoints';
import { setMockScenario } from '@/mocks/scenario';

/**
 * `downloads-absent`: a backend before #77's P4 and P6, so Activity degrades
 * cleanly (#207): /downloads answers 404, and the torrents' `wanted`, `holds`
 * and pinned bytes are missing, which the endpoints read as nothing.
 */

let mswFetch: typeof fetch;
beforeAll(() => {
  server.listen({ onUnhandledRequest: 'error' });
  mswFetch = global.fetch;
  global.fetch = ((input: RequestInfo | URL, init?: RequestInit) =>
    mswFetch(typeof input === 'string' ? new URL(input, 'http://localhost') : input, init)) as typeof fetch;
});
beforeEach(() => setMockScenario('downloads-absent'));
afterEach(() => resetMockState());
afterAll(() => {
  global.fetch = mswFetch;
  server.close();
});

const failure = (promise: Promise<unknown>) => promise.then(() => undefined, (error: unknown) => error);

describe('a backend without downloads', () => {
  it('has no /downloads routes', async () => {
    for (const call of [downloadsApi.list(), downloadsApi.remove(5), downloadsApi.estimate({ scope: 'movie', id: 2 })]) {
      const error = await failure(call);
      expect(isApiError(error) && error.status).toBe(404);
    }
  });

  it('sends torrents without wanted or holds, read as nothing wanting them', async () => {
    const { items } = await torrentsApi.list();
    expect(items.length).toBeGreaterThan(0);
    for (const item of items) expect(item).toMatchObject({ wanted: null, holds: [] });
    // Nothing wants Interstellar now, so it's just a torrent downloading on its own
    expect(items.find((t) => t.name.startsWith('Interstellar'))).toMatchObject({ state: 'downloading', download_bps: 2_300_000 });
    const detail = await torrentsApi.detail(items[0].info_hash);
    expect(detail).toMatchObject({ wanted: null, holds: [] });
  });

  it('reports no pinned bytes on the disk', async () => {
    const { disk } = await torrentsApi.summary();
    expect(disk).toMatchObject({ kept_bytes: 0, smart_bytes: 0 });
  });
});
