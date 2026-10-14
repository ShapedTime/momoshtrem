/** @jest-environment node */
import { ApiError, apiFetch, apiJson, parseErrorBody, resetRedirectGuard, toQuery } from '@/lib/api/client';

const assign = jest.fn();

function stubWindow(pathname: string, search = '') {
  (globalThis as unknown as { window: unknown }).window = { location: { pathname, search, hash: '', assign } };
}

function respond(status: number, body?: unknown) {
  global.fetch = jest.fn().mockResolvedValue(
    new Response(body === undefined ? null : JSON.stringify(body), { status, headers: { 'content-type': 'application/json' } })
  );
}

afterEach(() => {
  assign.mockReset();
  resetRedirectGuard();
  delete (globalThis as { window?: unknown }).window;
});

describe('apiFetch', () => {
  it('sends a 401 to /login, remembering where you were, and throws', async () => {
    stubWindow('/library', '?type=tv');
    respond(401, { error: { code: 'unauthorized', message: 'Your session has ended.' } });
    await expect(apiFetch('/api/library')).rejects.toMatchObject({ status: 401, code: 'unauthorized' });
    expect(assign).toHaveBeenCalledWith('/login?redirect=%2Flibrary%3Ftype%3Dtv');
  });

  it('redirects only once for a burst of 401s', async () => {
    stubWindow('/library');
    respond(401, { error: { code: 'unauthorized', message: 'Your session has ended.' } });
    await Promise.allSettled([apiFetch('/api/a'), apiFetch('/api/b'), apiFetch('/api/c')]);
    expect(assign).toHaveBeenCalledTimes(1);
  });

  it('never redirects from the login page itself', async () => {
    stubWindow('/login');
    respond(401, { error: { code: 'unauthorized', message: 'Your session has ended.' } });
    await expect(apiFetch('/api/x')).rejects.toBeInstanceOf(ApiError);
    expect(assign).not.toHaveBeenCalled();
  });

  it('treats an upstream 401 (a rejected API key) as an ordinary error, not a lost session', async () => {
    stubWindow('/search');
    respond(401, { error: 'TMDB API request failed: Request failed with status code 401' });
    const response = await apiFetch('/api/tmdb/search?q=x');
    expect(response.status).toBe(401);
    expect(assign).not.toHaveBeenCalled();
  });

  it('passes other statuses through untouched', async () => {
    respond(502, { error: { code: 'backend_auth', message: 'token' } });
    const response = await apiFetch('/api/status');
    expect(response.status).toBe(502);
  });
});

describe('apiJson', () => {
  it('parses JSON and sends JSON bodies', async () => {
    respond(200, { ok: true });
    await expect(apiJson('/api/x', { method: 'POST', json: { a: 1 } })).resolves.toEqual({ ok: true });
    const [, init] = (global.fetch as jest.Mock).mock.calls[0];
    expect(init.body).toBe('{"a":1}');
    expect(init.headers['Content-Type']).toBe('application/json');
  });

  it('throws an ApiError with the contract code and message', async () => {
    respond(404, { error: { code: 'not_found', message: 'No such title' } });
    await expect(apiJson('/api/titles/tv/1')).rejects.toMatchObject({ status: 404, code: 'not_found', message: 'No such title' });
  });

  it('reads the legacy {error: "message"} shape too', async () => {
    respond(500, { error: 'Failed to fetch movies' });
    await expect(apiJson('/api/library/movies')).rejects.toMatchObject({ status: 500, code: 'internal_error', message: 'Failed to fetch movies' });
  });

  it('derives the code from the status like the backend when the body has none', async () => {
    respond(418);
    await expect(apiJson('/api/x')).rejects.toMatchObject({ status: 418, code: 'bad_request' });
  });

  it('resolves 204 to undefined', async () => {
    respond(204);
    await expect(apiJson('/api/x', { method: 'DELETE' })).resolves.toBeUndefined();
  });
});

describe('helpers', () => {
  it('parseErrorBody ignores junk', () => {
    expect(parseErrorBody(null)).toEqual({});
    expect(parseErrorBody({ error: 42 })).toEqual({});
  });

  it('toQuery skips empty values and repeats arrays', () => {
    expect(toQuery({ q: 'frieren', page: 2, type: undefined, empty: '', resolution: ['2160p', '1080p'] })).toBe(
      '?q=frieren&page=2&resolution=2160p&resolution=1080p'
    );
    expect(toQuery({})).toBe('');
  });
});
