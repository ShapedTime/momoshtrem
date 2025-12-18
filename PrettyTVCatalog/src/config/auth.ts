// Cookie name for session
export const SESSION_COOKIE_NAME = 'prettytv_session';

// Session duration: 7 days in milliseconds
export const SESSION_DURATION_MS = 7 * 24 * 60 * 60 * 1000;

// Session duration in seconds (for cookie maxAge)
export const SESSION_DURATION_SECONDS = 7 * 24 * 60 * 60;

// Routes that don't require authentication
export const PUBLIC_ROUTES = ['/login', '/api/auth', '/api/health'];

// Route to redirect to after login
export const DEFAULT_REDIRECT = '/';

// Route to redirect to when not authenticated
export const LOGIN_ROUTE = '/login';
