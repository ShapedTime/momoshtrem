/** @jest-environment node */
import { ApiError, apiFetch, apiJson, parseErrorBody, resetRedirectGuard, toQuery } from '@/lib/api/client';
import { leavePage, resetPage, setPageMember } from '@/lib/members/page';

const assign = jest.fn();
const reload = jest.fn();

function stubWindow(pathname: string, search = '', hash = '') {
  (globalThis as unknown as { window: unknown }).window = {
    location: { pathname, search, hash, href: `http://localhost${pathname}${search}${hash}`, assign, reload },
  };
}

function respond(status: number, body?: unknown) {
  global.fetch = jest.fn().mockResolvedValue(
    new Response(body === undefined ? null : JSON.stringify(body), { status, headers: { 'content-type': 'application/json' } })
  );
}

afterEach(() => {
  assign.mockReset();
  reload.mockReset();
  resetRedirectGuard();
  resetPage();
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
    const response = await apiFetch('/api/title-search?q=x');
    expect(response.status).toBe(401);
    expect(assign).not.toHaveBeenCalled();
  });

  describe('household members (#239)', () => {
    const cookies: string[] = [];
    let cookie = '';
    beforeEach(() => {
      const doc = {};
      Object.defineProperty(doc, 'cookie', { get: () => cookie, set: (value: string) => cookies.push(value), configurable: true });
      Object.assign(globalThis, { document: doc });
    });
    afterEach(() => {
      cookies.length = 0;
      cookie = '';
      delete (globalThis as { document?: unknown }).document;
    });

    it('a 400 unknown_member forgets the cookie and reloads, once for a burst, then throws (a hash only scrolls on assign)', async () => {
      stubWindow('/activity', '', '#act-dl-h');
      respond(400, { error: { code: 'unknown_member', message: 'No member has that id.' } });
      const results = await Promise.allSettled([apiFetch('/api/me/up-next'), apiFetch('/api/recommendations/home'), apiFetch('/api/me/watchlist')]);
      expect(results.every((r) => r.status === 'rejected' && (r.reason as ApiError).code === 'unknown_member')).toBe(true);
      expect(cookies).toEqual(['momoshtrem_member=; Path=/; Max-Age=0; SameSite=Lax']);
      expect(reload).toHaveBeenCalledTimes(1);
      expect(assign).not.toHaveBeenCalled();
    });

    it('…and a later 401 still sends you to sign in', async () => {
      stubWindow('/library');
      respond(400, { error: { code: 'unknown_member', message: 'No member has that id.' } });
      await apiFetch('/api/me/up-next').catch(() => undefined);
      respond(401, { error: { code: 'unauthorized', message: 'Your session has ended.' } });
      await expect(apiFetch('/api/library')).rejects.toMatchObject({ code: 'unauthorized' });
      expect(assign).toHaveBeenCalledWith('/login?redirect=%2Flibrary');
    });

    it('leaves other 400s alone', async () => {
      stubWindow('/search');
      respond(400, { error: { code: 'bad_request', message: 'q is required' } });
      const response = await apiFetch('/api/search/torrents');
      expect(response.status).toBe(400);
      expect(cookies).toEqual([]);
      expect(reload).not.toHaveBeenCalled();
    });

    /** Whether the promise settled within a few ticks. */
    const settles = (promise: Promise<unknown>) =>
      Promise.race([promise.then(() => true, () => true), new Promise<boolean>((resolve) => setTimeout(() => resolve(false), 20))]);

    it('another tab switched person: nothing is sent under this page, and it reloads once', async () => {
      stubWindow('/library');
      respond(200, { ok: true });
      setPageMember(1);
      cookie = 'momoshtrem_member=2';
      expect(await settles(apiFetch('/api/recommendations/feedback', { method: 'POST' }))).toBe(false);
      expect(await settles(apiFetch('/api/me/up-next'))).toBe(false);
      expect(global.fetch).not.toHaveBeenCalled();
      expect(reload).toHaveBeenCalledTimes(1);
    });

    it('this page is switching: nothing more is sent, and no reload races its navigation', async () => {
      stubWindow('/library');
      respond(200, { ok: true });
      setPageMember(1);
      leavePage();
      cookie = 'momoshtrem_member=2';
      expect(await settles(apiFetch('/api/me/up-next'))).toBe(false);
      expect(global.fetch).not.toHaveBeenCalled();
      expect(reload).not.toHaveBeenCalled();
    });

    it('the cookie still names this page’s member (or there’s no gate): sent as usual', async () => {
      stubWindow('/library');
      respond(200, { ok: true });
      setPageMember(1);
      cookie = 'theme=dark; momoshtrem_member=1';
      expect((await apiFetch('/api/me/up-next')).status).toBe(200);
      resetPage();
      cookie = 'momoshtrem_member=2';
      expect((await apiFetch('/api/me/up-next')).status).toBe(200);
      expect(reload).not.toHaveBeenCalled();
    });
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

  it('carries a 429’s resets_at (the AI budget, Ask’s daily limit)', async () => {
    respond(429, { error: { code: 'budget_exceeded', message: 'This month’s AI budget is used up.', resets_at: '2026-11-01T00:00:00+01:00' } });
    await expect(apiJson('/api/recommendations/refresh', { method: 'POST' })).rejects.toMatchObject({
      status: 429,
      code: 'budget_exceeded',
      resetsAt: '2026-11-01T00:00:00+01:00',
    });
  });

  it('keeps the error’s other fields in details (#77’s storage_full)', async () => {
    respond(409, { error: { code: 'storage_full', message: 'Not enough space', needed_bytes: 18_400_000_000, available_bytes: 1_200_000_000 } });
    await expect(apiJson('/api/downloads', { method: 'POST', json: { scope: 'movie', id: 1 } })).rejects.toMatchObject({
      status: 409,
      code: 'storage_full',
      details: { needed_bytes: 18_400_000_000, available_bytes: 1_200_000_000 },
    });
    respond(404, { error: { code: 'not_found', message: 'No such title' } });
    await expect(apiJson('/api/titles/tv/1')).rejects.toMatchObject({ details: {} });
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
