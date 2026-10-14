// Cookie name for session
export const SESSION_COOKIE_NAME = 'momoshtrem_session';

// Session duration: 7 days in milliseconds
export const SESSION_DURATION_MS = 7 * 24 * 60 * 60 * 1000;

// Session duration in seconds (for cookie maxAge)
export const SESSION_DURATION_SECONDS = 7 * 24 * 60 * 60;

// Paths reachable without a session (exact matches only)
export const PUBLIC_PATHS = new Set([
  '/login',
  '/api/auth',
  '/api/health',
  // App metadata and PWA files browsers fetch before signing in
  '/manifest.webmanifest',
  '/icon.svg',
  '/apple-icon.png',
  '/opengraph-image.png',
  '/mockServiceWorker.js',
]);

// Static directories reachable without a session
export const PUBLIC_PREFIXES = ['/_next/', '/icons/'];

// Route to redirect to after login
export const DEFAULT_REDIRECT = '/';

// Route to redirect to when not authenticated
export const LOGIN_ROUTE = '/login';
