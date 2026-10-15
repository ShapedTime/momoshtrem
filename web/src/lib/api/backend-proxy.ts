import { NextResponse, type NextRequest } from 'next/server';
import { MOMOSHTREM_CONFIG } from '@/config/momoshtrem';
import { getSession } from '@/lib/auth/session';
import { BACKEND_AUTH_CODE, BACKEND_AUTH_MESSAGE, TOKEN_HEADER } from './momoshtrem';

/**
 * Top-level path segments of the PLAN §6 contract that the browser may reach
 * through /api/*. Anything else 404s here. The allowlist is by first segment
 * only, so older backend routes under these prefixes are reachable too (still
 * behind the session check).
 */
export const BACKEND_PREFIXES = new Set([
  'titles',
  'library',
  'shows',
  'movies',
  'episodes',
  'assignment-jobs',
  'torrents',
  'search',
  'settings',
  'integrations',
  'me',
  'recommendations',
  'status',
  'hdrezka',
]);

/** Response headers worth passing back to the browser. */
const PASS_HEADERS = ['content-type', 'cache-control', 'retry-after', 'etag', 'last-modified'];

function error(status: number, code: string, message: string): NextResponse {
  return NextResponse.json({ error: { code, message } }, { status });
}

/**
 * Forward a session-checked /api/* request to the backend's /api/*, adding
 * the shared-secret header. Bodies are forwarded as-is and responses stream
 * back unbuffered (Server-Sent Events for Ask work as-is). Cookies never
 * leave the web app.
 *
 * - A backend 401/403 becomes 502 `backend_auth`: the user's session is fine,
 *   the web app's token isn't. Passing 401 through would loop via /login.
 * - Unreachable backend → 502 `backend_unreachable`; no headers within 30 s → 504.
 */
export async function proxyToBackend(request: NextRequest, segments: string[]): Promise<Response> {
  const session = await getSession();
  if (!session?.authenticated) return error(401, 'unauthorized', 'Your session has ended. Sign in again.');

  if (segments.length === 0 || segments.some((s) => s === '' || s === '.' || s === '..' || s.includes('/') || s.includes('\\'))) {
    return error(400, 'bad_request', 'Invalid path.');
  }
  if (!BACKEND_PREFIXES.has(segments[0])) return error(404, 'not_found', 'No such API route.');

  const target = new URL(`/api/${segments.map(encodeURIComponent).join('/')}`, MOMOSHTREM_CONFIG.baseUrl);
  target.search = request.nextUrl.search;

  const headers = new Headers({ Accept: request.headers.get('accept') ?? 'application/json' });
  const contentType = request.headers.get('content-type');
  if (contentType) headers.set('Content-Type', contentType);
  const token = process.env.BACKEND_API_TOKEN;
  if (token) headers.set(TOKEN_HEADER, token);

  const hasBody = request.method !== 'GET' && request.method !== 'HEAD';
  const body = hasBody ? await request.arrayBuffer() : undefined;

  // Abort when the browser goes away, or when the backend doesn't answer at all
  const controller = new AbortController();
  const onAbort = () => controller.abort();
  request.signal.addEventListener('abort', onAbort);
  const timer = setTimeout(() => controller.abort(new DOMException('timeout', 'TimeoutError')), MOMOSHTREM_CONFIG.timeout);

  let upstream: Response;
  try {
    upstream = await fetch(target, {
      method: request.method,
      headers,
      body: body && body.byteLength > 0 ? body : undefined,
      signal: controller.signal,
      cache: 'no-store',
      redirect: 'manual',
    });
  } catch (cause) {
    request.signal.removeEventListener('abort', onAbort);
    if (controller.signal.reason instanceof DOMException && controller.signal.reason.name === 'TimeoutError') {
      return error(504, 'timeout', 'momoshtrem didn’t answer in time.');
    }
    if (request.signal.aborted) return new Response(null, { status: 499 });
    console.error('[backend-proxy] momoshtrem unreachable:', cause instanceof Error ? cause.message : cause);
    return error(502, 'backend_unreachable', 'Can’t reach momoshtrem.');
  } finally {
    clearTimeout(timer);
  }

  if (upstream.status === 401 || upstream.status === 403) {
    // Release the connection; no need to wait for it
    void upstream.body?.cancel().catch(() => undefined);
    return error(502, BACKEND_AUTH_CODE, BACKEND_AUTH_MESSAGE);
  }

  const responseHeaders = new Headers();
  for (const name of PASS_HEADERS) {
    const value = upstream.headers.get(name);
    if (value) responseHeaders.set(name, value);
  }
  if (responseHeaders.get('content-type')?.includes('text/event-stream')) {
    // Stream through every hop unbuffered (PLAN R3)
    responseHeaders.set('Cache-Control', 'no-cache, no-transform');
    responseHeaders.set('X-Accel-Buffering', 'no');
  }

  return new Response(upstream.body, { status: upstream.status, headers: responseHeaders });
}
