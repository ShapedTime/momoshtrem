/** @jest-environment node */
import { server } from '@/mocks/node';
import { resetMockState } from '@/mocks/handlers';
import { isApiError } from '@/lib/api/client';
import { searchApi } from '@/lib/api/endpoints';
import { setMockScenario } from '@/mocks/scenario';

/** The quality profile mock (#94): it validates like the backend, and torrent search ranks with what it saved. */

let mswFetch: typeof fetch;
beforeAll(() => {
  server.listen({ onUnhandledRequest: 'error' });
  mswFetch = global.fetch;
  global.fetch = ((input: RequestInfo | URL, init?: RequestInit) =>
    mswFetch(typeof input === 'string' ? new URL(input, 'http://localhost') : input, init)) as typeof fetch;
});
afterEach(() => resetMockState());
afterAll(() => {
  global.fetch = mswFetch;
  server.close();
});

describe('quality profile mock', () => {
  it('saves trimmed, deduped lists, and torrent search follows the saved profile', async () => {
    const profile = await searchApi.qualityProfile();
    const before = await searchApi.torrents({ media_type: 'movie', tmdb_id: 1054867 });
    expect(before.items.some((r) => r.attrs.group === 'FLUX')).toBe(true);

    const saved = await searchApi.putQualityProfile({ ...profile, avoid_terms: [...profile.avoid_terms, ' FLUX ', 'cam'] });
    expect(saved.avoid_terms).toEqual([...profile.avoid_terms, 'FLUX']);

    const after = await searchApi.torrents({ media_type: 'movie', tmdb_id: 1054867 });
    expect(after.hidden).toBeGreaterThan(before.hidden);
    expect(after.items.some((r) => r.attrs.group === 'FLUX')).toBe(false);
  });

  it('answers 422 naming the field, as the backend does', async () => {
    const profile = await searchApi.qualityProfile();
    const error = await searchApi.putQualityProfile({ ...profile, preferred_resolution: '2160p', max_resolution: '1080p' }).catch((e: unknown) => e);
    expect(isApiError(error) && { status: error.status, code: error.code, field: error.field, message: error.message }).toEqual({
      status: 422,
      code: 'validation_error',
      field: 'max_resolution',
      message: 'max_resolution must be at least preferred_resolution',
    });

    setMockScenario('quality-invalid');
    const rejected = await searchApi.putQualityProfile(profile).catch((e: unknown) => e);
    expect(isApiError(rejected) && rejected.field).toBe('avoid_terms');
  });
});
