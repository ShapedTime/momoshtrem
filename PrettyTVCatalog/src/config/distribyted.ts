/**
 * Distribyted API configuration.
 * Note: Uses getters to read env vars at runtime, not build time.
 */
export const DISTRIBYTED_CONFIG = {
  /** Base URL for Distribyted API (from environment) - read at runtime */
  get baseUrl(): string {
    return process.env.DISTRIBYTED_URL || 'http://localhost:4444';
  },

  /** Default route for adding torrents - read at runtime */
  get defaultRoute(): string {
    return process.env.DISTRIBYTED_ROUTE || 'default';
  },

  /** Request timeout in milliseconds */
  timeout: 15000,
} as const;

/**
 * API endpoint paths.
 */
export const DISTRIBYTED_ENDPOINTS = {
  /** Add torrent to route - POST /api/routes/{route}/torrent */
  addTorrent: (route: string) => `/api/routes/${route}/torrent`,
} as const;
