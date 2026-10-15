/** @jest-environment node */
import { NextRequest } from 'next/server';
import { http, HttpResponse } from 'msw';
import { setupServer } from 'msw/node';
import { proxyToBackend } from '@/lib/api/backend-proxy';

jest.mock('@/lib/auth/session', () => ({
  getSession: jest.fn(async () => ({ authenticated: true, expiresAt: Date.now() + 60_000 })),
}));

const BACKEND = 'http://backend.test';
const seen: { url: string; headers: Headers; body: string }[] = [];

const server = setupServer(
  http.all(`${BACKEND}/api/*`, async ({ request }) => {
    seen.push({ url: request.url, headers: request.headers, body: await request.text() });
    const path = new URL(request.url).pathname;
    if (path === '/api/status') return HttpResponse.json({ version: '0.9.2' }, { headers: { 'Cache-Control': 'max-age=5', 'X-Internal': 'secret' } });
    if (path === '/api/titles/tv/404') return HttpResponse.json({ error: { code: 'not_found', message: 'No such title' } }, { status: 404 });
    if (path === '/api/me/up-next') return HttpResponse.json({ error: { code: 'unauthorized', message: 'invalid token' } }, { status: 401 });
    if (path === '/api/recommendations/ask') {
      return new HttpResponse('event: text\ndata: {"delta":"hi"}\n\n', { headers: { 'Content-Type': 'text/event-stream' } });
    }
    return HttpResponse.json({ ok: true });
  })
);

beforeAll(() => {
  process.env.MOMOSHTREM_URL = BACKEND;
  process.env.BACKEND_API_TOKEN = 'shared-secret';
  server.listen({ onUnhandledRequest: 'error' });
});
afterEach(() => {
  seen.length = 0;
  server.resetHandlers();
});
afterAll(() => server.close());

function request(path: string, init?: { method?: string; body?: string; headers?: Record<string, string> }) {
  return new NextRequest(`http://localhost:3000${path}`, {
    method: init?.method ?? 'GET',
    body: init?.body,
    headers: { cookie: 'momoshtrem_session=abc', ...init?.headers },
  });
}

describe('proxyToBackend', () => {
  it('forwards path, query and the shared secret, never the cookie', async () => {
    const response = await proxyToBackend(request('/api/status?refresh=1'), ['status']);
    expect(response.status).toBe(200);
    expect(await response.json()).toEqual({ version: '0.9.2' });
    expect(seen[0].url).toBe(`${BACKEND}/api/status?refresh=1`);
    expect(seen[0].headers.get('x-momoshtrem-token')).toBe('shared-secret');
    expect(seen[0].headers.get('cookie')).toBeNull();
  });

  it('passes safe response headers only', async () => {
    const response = await proxyToBackend(request('/api/status'), ['status']);
    expect(response.headers.get('cache-control')).toBe('max-age=5');
    expect(response.headers.get('x-internal')).toBeNull();
  });

  it('forwards request bodies', async () => {
    await proxyToBackend(
      request('/api/shows/1', { method: 'PATCH', body: '{"include_specials":false}', headers: { 'content-type': 'application/json' } }),
      ['shows', '1']
    );
    expect(seen[0].body).toBe('{"include_specials":false}');
    expect(seen[0].headers.get('content-type')).toBe('application/json');
  });

  it('passes backend errors through with their contract shape', async () => {
    const response = await proxyToBackend(request('/api/titles/tv/404'), ['titles', 'tv', '404']);
    expect(response.status).toBe(404);
    expect(await response.json()).toEqual({ error: { code: 'not_found', message: 'No such title' } });
  });

  it('turns a rejected backend token into 502 backend_auth, not 401', async () => {
    const response = await proxyToBackend(request('/api/me/up-next'), ['me', 'up-next']);
    expect(response.status).toBe(502);
    expect(await response.json()).toMatchObject({ error: { code: 'backend_auth' } });
  });

  it('streams Server-Sent Events through unbuffered', async () => {
    const response = await proxyToBackend(request('/api/recommendations/ask', { method: 'POST', body: '{}' }), ['recommendations', 'ask']);
    expect(response.headers.get('content-type')).toContain('text/event-stream');
    expect(response.headers.get('cache-control')).toBe('no-cache, no-transform');
    expect(response.headers.get('x-accel-buffering')).toBe('no');
    expect(await response.text()).toContain('"delta":"hi"');
  });

  it('forwards HDRezka (#63), query included', async () => {
    const response = await proxyToBackend(request('/api/hdrezka/search?media_type=movie&tmdb_id=157336'), ['hdrezka', 'search']);
    expect(response.status).toBe(200);
    expect(seen[0].url).toBe(`${BACKEND}/api/hdrezka/search?media_type=movie&tmdb_id=157336`);
  });

  it('only exposes the contract prefixes', async () => {
    const response = await proxyToBackend(request('/api/debug/pprof'), ['debug', 'pprof']);
    expect(response.status).toBe(404);
    expect(seen).toHaveLength(0);
  });

  it('rejects path traversal', async () => {
    const response = await proxyToBackend(request('/api/status/../x'), ['status', '..', 'x']);
    expect(response.status).toBe(400);
    expect(seen).toHaveLength(0);
  });

  it('reports an unreachable backend as 502', async () => {
    server.use(http.all(`${BACKEND}/api/*`, () => HttpResponse.error()));
    const spy = jest.spyOn(console, 'error').mockImplementation(() => {});
    const response = await proxyToBackend(request('/api/status'), ['status']);
    spy.mockRestore();
    expect(response.status).toBe(502);
    expect(await response.json()).toMatchObject({ error: { code: 'backend_unreachable' } });
  });

  it('requires a session even if the proxy file is bypassed', async () => {
    const { getSession } = jest.requireMock('@/lib/auth/session') as { getSession: jest.Mock };
    getSession.mockResolvedValueOnce(null);
    const response = await proxyToBackend(request('/api/status'), ['status']);
    expect(response.status).toBe(401);
  });
});
