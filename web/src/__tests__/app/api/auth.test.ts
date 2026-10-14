/** @jest-environment node */
import { NextRequest } from 'next/server';
import { DELETE, POST } from '@/app/api/auth/route';
import { resetRateLimits } from '@/lib/auth/rate-limit';
import { SESSION_COOKIE_NAME } from '@/config/auth';

const env = process.env;

function formRequest(fields: Record<string, string>, { json = false, xff = '203.0.113.5' } = {}) {
  return new NextRequest('http://localhost:3000/api/auth', {
    method: 'POST',
    headers: { 'content-type': 'application/x-www-form-urlencoded', 'x-forwarded-for': xff, ...(json ? { accept: 'application/json' } : {}) },
    body: new URLSearchParams(fields).toString(),
  });
}

beforeEach(() => {
  process.env = {
    ...env,
    APP_USERNAME: 'teymur',
    APP_PASSWORD: 'correct horse',
    SESSION_SECRET: 'test-secret-that-is-at-least-32-characters-long',
    AUTH_MAX_ATTEMPTS: '3',
    AUTH_GLOBAL_MAX_FAILURES: '100',
  };
  resetRateLimits();
});
afterAll(() => {
  process.env = env;
});

describe('POST /api/auth (JS-enhanced, JSON)', () => {
  it('signs in, sets the session cookie and returns the validated redirect', async () => {
    const response = await POST(formRequest({ username: 'teymur', password: 'correct horse', redirect: '/library' }, { json: true }));
    expect(response.status).toBe(200);
    expect(await response.json()).toEqual({ ok: true, redirect: '/library' });
    const cookie = response.headers.get('set-cookie') ?? '';
    expect(cookie).toContain(`${SESSION_COOKIE_NAME}=`);
    expect(cookie.toLowerCase()).toContain('httponly');
  });

  it('never redirects off-site', async () => {
    const response = await POST(formRequest({ username: 'teymur', password: 'correct horse', redirect: '//evil.com' }, { json: true }));
    expect(await response.json()).toEqual({ ok: true, redirect: '/' });
  });

  it('accepts a JSON body too', async () => {
    const request = new NextRequest('http://localhost:3000/api/auth', {
      method: 'POST',
      headers: { 'content-type': 'application/json', accept: 'application/json' },
      body: JSON.stringify({ username: 'teymur', password: 'correct horse' }),
    });
    expect((await POST(request)).status).toBe(200);
  });

  it('rejects a wrong username even with the right password', async () => {
    const response = await POST(formRequest({ username: 'admin', password: 'correct horse' }, { json: true }));
    expect(response.status).toBe(401);
    expect(await response.json()).toMatchObject({ ok: false, error: { code: 'invalid_credentials' }, remaining_attempts: 2 });
  });

  it('asks for both fields', async () => {
    const response = await POST(formRequest({ username: 'teymur', password: '' }, { json: true }));
    expect(response.status).toBe(400);
    expect(await response.json()).toMatchObject({ error: { code: 'missing_credentials' } });
  });

  it('pauses sign-in after too many failures, with Retry-After', async () => {
    await POST(formRequest({ username: 'teymur', password: 'x' }, { json: true }));
    await POST(formRequest({ username: 'teymur', password: 'x' }, { json: true }));
    const third = await POST(formRequest({ username: 'teymur', password: 'x' }, { json: true }));
    expect(third.status).toBe(429);
    expect(Number(third.headers.get('retry-after'))).toBeGreaterThan(0);
    // Even the right password waits out the pause
    const fourth = await POST(formRequest({ username: 'teymur', password: 'correct horse' }, { json: true }));
    expect(fourth.status).toBe(429);
    // A different client isn't affected
    const other = await POST(formRequest({ username: 'teymur', password: 'correct horse' }, { json: true, xff: '198.51.100.1' }));
    expect(other.status).toBe(200);
  });
});

describe('POST /api/auth (plain form, no JS)', () => {
  it('answers success with a 303 to the destination', async () => {
    const response = await POST(formRequest({ username: 'teymur', password: 'correct horse', redirect: '/tv/1' }));
    expect(response.status).toBe(303);
    expect(response.headers.get('location')).toBe('/tv/1');
    expect(response.headers.get('set-cookie')).toContain(`${SESSION_COOKIE_NAME}=`);
  });

  it('answers failure with a 303 back to /login carrying the error and destination', async () => {
    const response = await POST(formRequest({ username: 'teymur', password: 'nope', redirect: '/tv/1' }));
    expect(response.status).toBe(303);
    const location = new URL(response.headers.get('location')!, 'http://localhost');
    expect(location.pathname).toBe('/login');
    expect(location.searchParams.get('error')).toBe('invalid_credentials');
    expect(location.searchParams.get('redirect')).toBe('/tv/1');
    expect(response.headers.get('set-cookie')).toBeNull();
  });
});

describe('DELETE /api/auth', () => {
  it('expires the session cookie', async () => {
    const response = await DELETE();
    expect(response.status).toBe(200);
    expect(response.headers.get('set-cookie')).toMatch(new RegExp(`${SESSION_COOKIE_NAME}=;.*Max-Age=0`, 'i'));
  });
});
