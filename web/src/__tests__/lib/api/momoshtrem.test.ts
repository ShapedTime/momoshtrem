import { MomoshtremClient, BACKEND_AUTH_CODE } from '@/lib/api/momoshtrem';
import { APIError } from '@/lib/errors';
import { parseMomoshtremError } from '@/types/momoshtrem';
import { TORRENTS } from '@/mocks/fixtures/torrents';

interface FakeResponseInit {
  status: number;
  body?: unknown;
  statusText?: string;
}

function fakeResponse({ status, body, statusText = '' }: FakeResponseInit): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    statusText,
    json: async () => {
      if (body === undefined) throw new SyntaxError('Unexpected end of JSON input');
      return body;
    },
  } as Response;
}

describe('MomoshtremClient', () => {
  const originalEnv = process.env;
  let fetchMock: jest.Mock;

  beforeEach(() => {
    process.env = { ...originalEnv, MOMOSHTREM_URL: 'http://backend:4444' };
    fetchMock = jest.fn();
    global.fetch = fetchMock as unknown as typeof fetch;
  });

  afterEach(() => {
    process.env = originalEnv;
  });

  function sentHeaders(): Record<string, string> {
    const init = fetchMock.mock.calls[0][1] as RequestInit;
    return init.headers as Record<string, string>;
  }

  describe('token header', () => {
    it('sends X-Momoshtrem-Token when BACKEND_API_TOKEN is set', async () => {
      process.env.BACKEND_API_TOKEN = 'shared-secret';
      fetchMock.mockResolvedValue(fakeResponse({ status: 200, body: [] }));

      await new MomoshtremClient().getMovies();

      expect(fetchMock.mock.calls[0][0]).toBe('http://backend:4444/api/movies');
      expect(sentHeaders()['X-Momoshtrem-Token']).toBe('shared-secret');
      expect(sentHeaders()['Content-Type']).toBe('application/json');
    });

    it('omits the header when BACKEND_API_TOKEN is unset', async () => {
      delete process.env.BACKEND_API_TOKEN;
      fetchMock.mockResolvedValue(fakeResponse({ status: 200, body: [] }));

      await new MomoshtremClient().getMovies();

      expect(sentHeaders()).not.toHaveProperty('X-Momoshtrem-Token');
    });
  });

  describe('error handling', () => {
    async function errorFor(init: FakeResponseInit): Promise<APIError> {
      fetchMock.mockResolvedValue(fakeResponse(init));
      try {
        await new MomoshtremClient().getMovies();
      } catch (error) {
        return error as APIError;
      }
      throw new Error('expected getMovies to throw');
    }

    it.each([401, 403])(
      'maps a backend %i to a 502 backend_auth error, never a 401',
      async (status) => {
        const error = await errorFor({
          status,
          body: { error: { code: 'unauthorized', message: 'invalid X-Momoshtrem-Token header' } },
        });

        expect(error).toBeInstanceOf(APIError);
        expect(error.statusCode).toBe(502);
        expect(error.code).toBe(BACKEND_AUTH_CODE);
        expect(error.message).toMatch(/BACKEND_API_TOKEN/);
      }
    );

    it('reads the code and message from the structured error shape', async () => {
      const error = await errorFor({
        status: 404,
        body: { error: { code: 'not_found', message: 'Movie not found' } },
      });

      expect(error.statusCode).toBe(404);
      expect(error.message).toBe('Movie not found');
      expect(error.code).toBe('not_found');
    });

    it('still reads the legacy string error shape', async () => {
      const error = await errorFor({ status: 400, body: { error: 'Invalid movie ID' } });

      expect(error.statusCode).toBe(400);
      expect(error.message).toBe('Invalid movie ID');
    });

    it('falls back to the operation and status text without a JSON body', async () => {
      const error = await errorFor({ status: 500, statusText: 'Internal Server Error' });

      expect(error.statusCode).toBe(500);
      expect(error.message).toBe('Failed to get movies: Internal Server Error');
    });
  });

  describe('legacy torrents (until #102)', () => {
    it("adapts the backend's items to the legacy shape the /torrents page and the badge read", async () => {
      fetchMock.mockResolvedValue(fakeResponse({ status: 200, body: { items: TORRENTS } }));

      const torrents = await new MomoshtremClient().getTorrents();

      expect(fetchMock.mock.calls[0][0]).toBe('http://backend:4444/api/torrents');
      expect(torrents).toHaveLength(TORRENTS.length);
      const byName = new Map(torrents.map((t) => [t.name, t]));
      // Fetching: its stats pass through.
      expect(byName.get('Frieren.S02.1080p.WEB-DL.x264-SubsPlease')).toEqual({
        info_hash: 'a1b2c3d4e5f60718293a4b5c6d7e8f9012345678',
        name: 'Frieren.S02.1080p.WEB-DL.x264-SubsPlease',
        total_size: 12_600_000_000,
        downloaded: 8_064_000_000,
        progress: 0.64,
        seeders: 31,
        leechers: 17,
        download_speed: 6_400_000,
        upload_speed: 420_000,
        is_paused: false,
      });
      // Not loaded: zeros where it had nulls, and its network is off.
      expect(byName.get('Dune.Part.Two.2024.2160p.WEB-DL.DDP5.1.Atmos.DV.HDR.H.265-FLUX')).toMatchObject({
        total_size: 18_400_000_000, downloaded: 0, progress: 0, seeders: 0, leechers: 0, is_paused: true,
      });
      // The badge's count (not paused, not complete) stays the downloading torrent.
      expect(torrents.filter((t) => !t.is_paused && t.progress < 1).map((t) => t.name)).toEqual(['Frieren.S02.1080p.WEB-DL.x264-SubsPlease']);
    });

    it('answers an empty list for an empty body', async () => {
      fetchMock.mockResolvedValue(fakeResponse({ status: 200, body: { items: [] } }));
      await expect(new MomoshtremClient().getTorrents()).resolves.toEqual([]);
    });
  });
});

describe('parseMomoshtremError', () => {
  it.each([
    [{ error: { code: 'conflict', message: 'Already exists' } }, { code: 'conflict', message: 'Already exists' }],
    [{ error: { message: 'No code' } }, { code: undefined, message: 'No code' }],
    [{ error: 'Legacy message' }, { message: 'Legacy message' }],
  ])('parses %j', (body, expected) => {
    expect(parseMomoshtremError(body)).toEqual(expected);
  });

  it.each([null, undefined, 'text', 42, {}, { error: '' }, { error: { code: 'x' } }, { error: null }])(
    'returns null for %j',
    (body) => {
      expect(parseMomoshtremError(body)).toBeNull();
    }
  );
});
