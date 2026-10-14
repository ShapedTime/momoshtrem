/** @jest-environment node */
import { NextRequest } from 'next/server';
import { proxy } from '@/proxy';
import { createSessionToken } from '@/lib/auth/session';
import { SESSION_COOKIE_NAME } from '@/config/auth';

process.env.SESSION_SECRET = 'test-secret-that-is-at-least-32-characters-long';

function request(path: string, cookie?: string) {
  return new NextRequest(`http://localhost:3000${path}`, {
    headers: cookie ? { cookie: `${SESSION_COOKIE_NAME}=${cookie}` } : {},
  });
}

describe('proxy', () => {
  it('answers unauthenticated API calls with 401 JSON, not a redirect', async () => {
    const response = await proxy(request('/api/torrents'));
    expect(response.status).toBe(401);
    expect(await response.json()).toEqual({ error: { code: 'unauthorized', message: expect.any(String) } });
  });

  it('redirects unauthenticated pages to /login, keeping path and query', async () => {
    const response = await proxy(request('/library?type=tv'));
    expect(response.status).toBe(307);
    const location = new URL(response.headers.get('location')!);
    expect(location.pathname).toBe('/login');
    expect(location.searchParams.get('redirect')).toBe('/library?type=tv');
  });

  it('lets public paths through without a session', async () => {
    for (const path of ['/login', '/api/auth', '/api/health', '/manifest.webmanifest', '/icons/icon-192.png', '/_next/static/x.js']) {
      const response = await proxy(request(path));
      expect(response.headers.get('x-middleware-next')).toBe('1');
    }
  });

  it('does not treat look-alike paths as public', async () => {
    for (const path of ['/api/authx', '/api/auth/../torrents', '/api/health.json', '/loginx']) {
      const response = await proxy(request(path));
      expect(response.headers.get('x-middleware-next')).toBeNull();
    }
  });

  it('lets a valid session through', async () => {
    const response = await proxy(request('/api/torrents', await createSessionToken()));
    expect(response.headers.get('x-middleware-next')).toBe('1');
  });

  it('rejects and clears a tampered cookie', async () => {
    const response = await proxy(request('/api/torrents', 'not-a-jwt'));
    expect(response.status).toBe(401);
    expect(response.headers.get('set-cookie')).toContain(`${SESSION_COOKIE_NAME}=;`);
  });
});
