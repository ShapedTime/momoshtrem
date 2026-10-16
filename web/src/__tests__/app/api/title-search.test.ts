/** @jest-environment node */
import { NextRequest } from 'next/server';

const mockSearchMovie = jest.fn();
const mockSearchTv = jest.fn();
const mockSearchPerson = jest.fn();

jest.mock('moviedb-promise', () => ({
  MovieDb: jest.fn().mockImplementation(() => ({ searchMovie: mockSearchMovie, searchTv: mockSearchTv, searchPerson: mockSearchPerson })),
}));
jest.mock('@/lib/api/auth-guard', () => ({ requireAuth: jest.fn(async () => null) }));

const env = process.env;

/** A fresh route (and TMDB client singleton) per test, so the key check runs each time. */
async function get(query: string) {
  let route: typeof import('@/app/api/title-search/route') | undefined;
  jest.isolateModules(() => {
    route = jest.requireActual<typeof import('@/app/api/title-search/route')>('@/app/api/title-search/route');
  });
  const response = await route!.GET(new NextRequest(`http://localhost:3000/api/title-search${query}`));
  return { status: response.status, body: await response.json() };
}

const page = (results: object[]) => ({ page: 1, results, total_results: results.length, total_pages: 1 });
const nothing = () => Promise.resolve(page([]));

beforeEach(() => {
  process.env = { ...env, TMDB_API_KEY: 'test-key' };
  mockSearchMovie.mockReset().mockImplementation(nothing);
  mockSearchTv.mockReset().mockImplementation(nothing);
  mockSearchPerson.mockReset().mockImplementation(nothing);
});
afterAll(() => {
  process.env = env;
});

describe('GET /api/title-search', () => {
  it('asks TMDB for movies, shows and people at once, with a timeout', async () => {
    await get('?q=%20%20dune%20');
    for (const call of [mockSearchMovie, mockSearchTv, mockSearchPerson]) {
      expect(call).toHaveBeenCalledWith({ query: 'dune', include_adult: false }, { timeout: 10_000 });
    }
  });

  it('merges and ranks them, with reasons and counts', async () => {
    mockSearchMovie.mockResolvedValue(
      page([
        { id: 251516, title: 'Jodorowsky’s Dune', original_title: 'Jodorowsky’s Dune', release_date: '2013-07-10', vote_average: 7.9, vote_count: 900, popularity: 5, poster_path: '/j.jpg' },
        { id: 438631, title: 'Dune', original_title: 'Dune', release_date: '2021-09-15', vote_average: 7.8, vote_count: 12000, popularity: 90, poster_path: '/d.jpg' },
        { id: 0, title: 'Broken row' },
      ])
    );
    mockSearchTv.mockResolvedValue(
      page([{ id: 90228, name: 'Dune: Prophecy', original_name: 'Dune: Prophecy', first_air_date: '2024-11-17', vote_average: 7.6, vote_count: 400, popularity: 40, poster_path: null }])
    );
    mockSearchPerson.mockResolvedValue(
      page([
        {
          id: 1,
          name: 'Dune Mosley',
          known_for_department: 'Directing',
          known_for: [{ id: 5, media_type: 'movie', title: 'Sand', release_date: '1999-01-01', vote_average: 6, vote_count: 50, popularity: 1 }],
        },
      ])
    );

    const { status, body } = await get('?q=dune');
    expect(status).toBe(200);
    expect(body).toEqual({
      query: 'dune',
      items: [
        { media_type: 'movie', tmdb_id: 438631, title: 'Dune', original_title: null, year: 2021, poster_path: '/d.jpg', match: 'exact', match_reason: null },
        { media_type: 'tv', tmdb_id: 90228, title: 'Dune: Prophecy', original_title: null, year: 2024, poster_path: null, match: 'prefix', match_reason: null },
        { media_type: 'movie', tmdb_id: 251516, title: 'Jodorowsky’s Dune', original_title: null, year: 2013, poster_path: '/j.jpg', match: 'words', match_reason: null },
        { media_type: 'movie', tmdb_id: 5, title: 'Sand', original_title: null, year: 1999, poster_path: null, match: 'person', match_reason: 'Directed by Dune Mosley' },
      ],
      counts: { all: 4, movie: 3, tv: 1 },
    });
  });

  it('marks titles matched by an original or alternative title', async () => {
    mockSearchTv.mockResolvedValue(
      page([{ id: 209867, name: 'Frieren: Beyond Journey’s End', original_name: '葬送のフリーレン', first_air_date: '2023-09-29', vote_average: 8.8, vote_count: 500 }])
    );
    const { body } = await get('?q=sousou+no+frieren');
    expect(body.items).toEqual([
      expect.objectContaining({ tmdb_id: 209867, original_title: '葬送のフリーレン', match: 'original', match_reason: 'Original title match' }),
    ]);
  });

  it('answers 400 without a query or with an over-long one', async () => {
    expect(await get('')).toEqual({ status: 400, body: { error: { code: 'bad_request', message: 'q must be 1 to 100 characters.' } } });
    expect((await get('?q=%20%20')).status).toBe(400);
    expect((await get(`?q=${'x'.repeat(101)}`)).status).toBe(400);
    expect(mockSearchMovie).not.toHaveBeenCalled();
  });

  it('answers 503 tmdb_not_configured without a key', async () => {
    delete process.env.TMDB_API_KEY;
    const { status, body } = await get('?q=dune');
    expect(status).toBe(503);
    expect(body.error.code).toBe('tmdb_not_configured');
  });

  it('answers 502 tmdb_auth when TMDB rejects the key (never 401, which would sign the user out)', async () => {
    mockSearchPerson.mockRejectedValue(new Error('Request failed with status code 401'));
    const { status, body } = await get('?q=dune');
    expect(status).toBe(502);
    expect(body.error.code).toBe('tmdb_auth');
  });

  it('answers 502 tmdb_unavailable when TMDB fails or times out', async () => {
    mockSearchTv.mockRejectedValue(new Error('timeout of 10000ms exceeded'));
    const { status, body } = await get('?q=dune');
    expect(status).toBe(502);
    expect(body).toEqual({ error: { code: 'tmdb_unavailable', message: 'TMDB didn’t answer. Try again in a moment.' } });
  });
});
