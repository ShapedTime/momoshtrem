/**
 * Top-level path segments of the PLAN §6 contract that the browser may reach
 * through /api/*. Anything else 404s in the proxy. The allowlist is by first
 * segment only, so older backend routes under these prefixes are reachable
 * too (still behind the session check). No server imports, so the mocks can
 * read it.
 */
export const BACKEND_PREFIXES: ReadonlySet<string> = new Set([
  'titles',
  'library',
  'shows',
  'movies',
  'episodes',
  'assignment-jobs',
  'torrents',
  'downloads',
  'search',
  'settings',
  'integrations',
  'me',
  'recommendations',
  'status',
  'hdrezka',
  'members',
]);
