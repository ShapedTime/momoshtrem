/**
 * Jackett API configuration.
 * Note: baseUrl uses a getter to read env var at runtime, not build time.
 */
export const JACKETT_CONFIG = {
  /** Base URL for Jackett API (from environment) - read at runtime */
  get baseUrl(): string {
    return process.env.JACKETT_URL || 'http://localhost:9117';
  },

  /** Torznab search endpoint path */
  searchPath: '/api/v2.0/indexers/all/results/torznab/',

  /** Maximum results to return */
  maxResults: 50,

  /** Request timeout in milliseconds */
  timeout: 30000,
} as const;

/**
 * Torznab category mappings for filtering by media type.
 */
export const TORZNAB_CATEGORIES = {
  movies: '2000',
  moviesHD: '2040',
  movies4K: '2045',
  tv: '5000',
  tvHD: '5040',
  tv4K: '5045',
} as const;

export type TorznabCategory = keyof typeof TORZNAB_CATEGORIES;
