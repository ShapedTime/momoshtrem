import { NextRequest, NextResponse } from 'next/server';
import { applySessionCookie, clearSessionCookie, createSessionToken, safeRedirect, verifyCredentials } from '@/lib/auth';
import { getClientIp } from '@/lib/auth/client-ip';
import { checkRateLimit, clearRateLimit, recordFailedAttempt } from '@/lib/auth/rate-limit';
import { LOGIN_ROUTE } from '@/config/auth';
import type { ApiErrorBody } from '@/lib/api/contract';
import type { LoginErrorCode, LoginResponse } from '@/types/auth';

const MESSAGES: Record<LoginErrorCode, string> = {
  invalid_credentials: 'Wrong username or password',
  missing_credentials: 'Enter your username and password.',
  rate_limited: 'Too many attempts',
  server_error: 'Couldn’t sign in',
};

interface Credentials {
  username: string;
  password: string;
  redirect: string;
}

async function readCredentials(request: NextRequest): Promise<Credentials> {
  const field = (value: unknown) => (typeof value === 'string' ? value : '');
  try {
    if ((request.headers.get('content-type') ?? '').includes('application/json')) {
      const body = (await request.json()) as Record<string, unknown>;
      return { username: field(body.username), password: field(body.password), redirect: field(body.redirect) };
    }
    const form = await request.formData();
    return { username: field(form.get('username')), password: field(form.get('password')), redirect: field(form.get('redirect')) };
  } catch {
    return { username: '', password: '', redirect: '' };
  }
}

/**
 * Answer in the form the client asked for: the JS-enhanced form sends
 * `Accept: application/json`; a plain form post (no JS) gets a 303 back to
 * the login page or on to the destination. Locations stay relative so they
 * work behind any proxy.
 */
function respond(
  wantsJson: boolean,
  target: string,
  outcome: { status: number; code?: LoginErrorCode; remaining?: number; retryAfter?: number }
): NextResponse {
  const { status, code, remaining, retryAfter } = outcome;
  if (wantsJson) {
    const body: LoginResponse = code
      ? { ok: false, error: { code, message: MESSAGES[code] }, remaining_attempts: remaining, retry_after: retryAfter }
      : { ok: true, redirect: target };
    return NextResponse.json(body, {
      status,
      headers: retryAfter ? { 'Retry-After': String(retryAfter) } : undefined,
    });
  }
  let location = target;
  if (code) {
    const params = new URLSearchParams({ error: code, redirect: target });
    if (retryAfter) params.set('retry', String(retryAfter));
    location = `${LOGIN_ROUTE}?${params}`;
  }
  return new NextResponse(null, {
    status: 303,
    headers: { Location: location, ...(retryAfter ? { 'Retry-After': String(retryAfter) } : {}) },
  });
}

// POST /api/auth: sign in
export async function POST(request: NextRequest): Promise<NextResponse> {
  // Another site posting here could sign the browser into this account, or
  // burn the global failure ceiling and lock the owner out. Browsers send
  // Sec-Fetch-Site; no-JS form posts are same-origin and other clients send none.
  if (request.headers.get('sec-fetch-site') === 'cross-site') {
    const body: ApiErrorBody = { error: { code: 'forbidden', message: 'Sign in from this site.' } };
    return NextResponse.json(body, { status: 403 });
  }

  const wantsJson = (request.headers.get('accept') ?? '').includes('application/json');
  const { username, password, redirect } = await readCredentials(request);
  const target = safeRedirect(redirect);
  const client = getClientIp(request.headers);

  const limit = checkRateLimit(client);
  if (!limit.allowed) {
    return respond(wantsJson, target, { status: 429, code: 'rate_limited', retryAfter: limit.retryAfterSeconds });
  }

  if (!username.trim() || !password) {
    return respond(wantsJson, target, { status: 400, code: 'missing_credentials' });
  }

  if (!verifyCredentials(username, password)) {
    const after = recordFailedAttempt(client);
    if (!after.allowed) {
      return respond(wantsJson, target, { status: 429, code: 'rate_limited', retryAfter: after.retryAfterSeconds });
    }
    return respond(wantsJson, target, { status: 401, code: 'invalid_credentials', remaining: after.remaining });
  }

  try {
    clearRateLimit(client);
    const token = await createSessionToken();
    return applySessionCookie(respond(wantsJson, target, { status: 200 }), token);
  } catch (error) {
    console.error('[auth] sign-in failed:', error);
    return respond(wantsJson, target, { status: 500, code: 'server_error' });
  }
}

// DELETE /api/auth: sign out
export async function DELETE(): Promise<NextResponse> {
  return clearSessionCookie(NextResponse.json({ ok: true }));
}
