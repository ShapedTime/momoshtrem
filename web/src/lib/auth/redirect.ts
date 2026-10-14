import { DEFAULT_REDIRECT, LOGIN_ROUTE } from '@/config/auth';

const BASE = 'http://momoshtrem.local';

/**
 * Where to send someone after signing in. Only same-origin, relative paths
 * are allowed: "/library?type=tv" passes; "//evil.com", "/\\evil.com",
 * "/..//evil.com" (dot segments that normalise to "//evil.com"),
 * "https://evil.com", "javascript:…", "/login" and "/api/…" fall back to
 * the default.
 */
export function safeRedirect(value: unknown, fallback: string = DEFAULT_REDIRECT): string {
  if (typeof value !== 'string' || value.length === 0 || value.length > 2048) return fallback;
  if (!value.startsWith('/') || value.startsWith('//') || value.startsWith('/\\')) return fallback;
  // Control characters (incl. tab/newline, which browsers strip) can smuggle a scheme or host
  if (/[\u0000-\u001f\u007f\\]/.test(value)) return fallback;

  let url: URL;
  try {
    url = new URL(value, BASE);
  } catch {
    return fallback;
  }
  if (url.origin !== BASE) return fallback;
  if (url.pathname === LOGIN_ROUTE || url.pathname.startsWith(`${LOGIN_ROUTE}/`) || url.pathname.startsWith('/api/')) {
    return fallback;
  }
  const result = `${url.pathname}${url.search}${url.hash}`;
  // URL resolves dot segments, so "/..//evil.com" comes out protocol-relative
  if (result.startsWith('//') || result.startsWith('/\\')) return fallback;
  return result;
}
