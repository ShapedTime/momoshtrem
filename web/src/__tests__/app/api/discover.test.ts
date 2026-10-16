/** @jest-environment node */
import { NextRequest } from 'next/server';

const mockDiscoverMovie = jest.fn();
const mockDiscoverTv = jest.fn();

jest.mock('moviedb-promise', () => ({
  MovieDb: jest.fn().mockImplementation(() => ({ discoverMovie: mockDiscoverMovie, discoverTv: mockDiscoverTv })),
}));
jest.mock('@/lib/api/auth-guard', () => ({ requireAuth: jest.fn(async () => null) }));

const env = process.env;

/** A fresh route (and TMDB client singleton) per test, so the key check runs each time. */
async function get(query: string) {
  let route: typeof import('@/app/api/discover/route') | undefined;
  jest.isolateModules(() => {
    route = jest.requireActual<typeof import('@/app/api/discover/route')>('@/app/api/discover/route');
  });
  const response = await route!.GET(new NextRequest(`http://localhost:3000/api/discover${query}`));
  return { status: response.status, body: await response.json() };
}

const page = (results: object[], total_results: number, total_pages: number) => ({ page: 1, results, total_results, total_pages });

beforeEach(() => {
  process.env = { ...env, TMDB_API_KEY: 'test-key' };
  mockDiscoverMovie.mockReset();
  mockDiscoverTv.mockReset();
});
afterAll(() => {
  process.env = env;
});

describe('GET /api/discover', () => {
  it('merges movies and shows by the sort key, sums the totals and caps the pages at 500', async () => {
    mockDiscoverMovie.mockResolvedValue(
      page(
        [
          { id: 1, title: 'Dune: Part Two', release_date: '2024-02-27', popularity: 90, poster_path: '/dune.jpg' },
          { id: 2, title: 'Arrival', release_date: '', popularity: 10, poster_path: null },
        ],
        20001,
        1001
      )
    );
    mockDiscoverTv.mockResolvedValue(page([{ id: 3, name: 'Severance', first_air_date: '2022-02-17', popularity: 50, poster_path: '/sev.jpg' }], 7015, 351));

    const { status, body } = await get('');
    expect(status).toBe(200);
    expect(body).toEqual({
      items: [
        { media_type: 'movie', tmdb_id: 1, title: 'Dune: Part Two', year: 2024, poster_path: '/dune.jpg' },
        { media_type: 'tv', tmdb_id: 3, title: 'Severance', year: 2022, poster_path: '/sev.jpg' },
        { media_type: 'movie', tmdb_id: 2, title: 'Arrival', year: null, poster_path: null },
      ],
      total: 27016,
      page: 1,
      total_pages: 500,
    });
    expect(mockDiscoverMovie).toHaveBeenCalledWith(expect.objectContaining({ sort_by: 'popularity.desc', page: 1, include_adult: false }), { timeout: 10_000 });
  });

  it('asks only for movies when a genre has no TV id, and passes the page', async () => {
    mockDiscoverMovie.mockResolvedValue(page([], 0, 0));
    const { status, body } = await get('?genres=thriller&page=3');
    expect(status).toBe(200);
    expect(body).toEqual({ items: [], total: 0, page: 3, total_pages: 0 });
    expect(mockDiscoverTv).not.toHaveBeenCalled();
    expect(mockDiscoverMovie).toHaveBeenCalledWith(expect.objectContaining({ with_genres: '53', page: 3 }), expect.anything());
  });

  it('answers relax=1 with the total without each active filter', async () => {
    mockDiscoverTv.mockImplementation(async (params: Record<string, unknown>) => page([], params.with_status ? 4 : 120, 1));
    const { status, body } = await get('?type=tv&genres=comedy&status=ended&relax=1');
    expect(status).toBe(200);
    expect(body).toEqual({
      relaxed: [
        { filter: 'genre:comedy', would_be: 4 },
        { filter: 'status', would_be: 120 },
      ],
    });
    expect(mockDiscoverMovie).not.toHaveBeenCalled();
  });

  it('rejects a page TMDB can’t serve', async () => {
    expect(await get('?page=501')).toEqual({ status: 400, body: { error: { code: 'bad_request', message: expect.any(String) } } });
    expect(await get('?page=abc')).toMatchObject({ status: 400 });
  });

  it('says so when TMDB_API_KEY isn’t set', async () => {
    delete process.env.TMDB_API_KEY;
    expect(await get('')).toEqual({ status: 503, body: { error: { code: 'tmdb_not_configured', message: expect.stringContaining('TMDB_API_KEY') } } });
  });

  it('tells a rejected key from TMDB being down', async () => {
    mockDiscoverTv.mockResolvedValue(page([], 0, 0));
    mockDiscoverMovie.mockRejectedValue(new Error('Request failed with status code 401'));
    expect(await get('')).toMatchObject({ status: 502, body: { error: { code: 'tmdb_auth' } } });
    mockDiscoverMovie.mockRejectedValue(new Error('getaddrinfo ENOTFOUND api.themoviedb.org'));
    expect(await get('')).toMatchObject({ status: 502, body: { error: { code: 'tmdb_unavailable' } } });
    mockDiscoverMovie.mockRejectedValue(new Error('Request failed with status code 503'));
    expect(await get('')).toMatchObject({ status: 502, body: { error: { code: 'tmdb_unavailable' } } });
  });
});
