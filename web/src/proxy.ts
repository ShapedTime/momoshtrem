import { NextResponse } from 'next/server';
import type { NextRequest } from 'next/server';
import { LOGIN_ROUTE, PUBLIC_PATHS, PUBLIC_PREFIXES, SESSION_COOKIE_NAME } from '@/config/auth';
import { verifySessionToken } from '@/lib/auth/session';

function isPublic(pathname: string): boolean {
  return PUBLIC_PATHS.has(pathname) || PUBLIC_PREFIXES.some((prefix) => pathname.startsWith(prefix));
}

/**
 * Session gate for every route. Pages redirect to /login (keeping where you
 * were going); API routes answer 401 JSON so client fetches can send you to
 * /login instead of parsing an HTML redirect.
 */
export async function proxy(request: NextRequest) {
  const { pathname, search } = request.nextUrl;
  if (isPublic(pathname)) return NextResponse.next();

  const token = request.cookies.get(SESSION_COOKIE_NAME)?.value;
  const session = token ? await verifySessionToken(token) : null;
  if (session?.authenticated) return NextResponse.next();

  let response: NextResponse;
  if (pathname.startsWith('/api/')) {
    response = NextResponse.json(
      { error: { code: 'unauthorized', message: 'Your session has ended. Sign in again.' } },
      { status: 401 }
    );
  } else {
    const loginUrl = new URL(LOGIN_ROUTE, request.url);
    loginUrl.searchParams.set('redirect', `${pathname}${search}`);
    response = NextResponse.redirect(loginUrl);
  }
  // Drop an expired or tampered cookie
  if (token) response.cookies.delete(SESSION_COOKIE_NAME);
  return response;
}

export const config = {
  matcher: ['/((?!_next/static|_next/image|favicon.ico).*)'],
};
