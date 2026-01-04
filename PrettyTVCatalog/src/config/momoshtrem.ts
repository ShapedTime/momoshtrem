/**
 * momoshtrem API configuration.
 * Note: Uses getters to read env vars at runtime, not build time.
 */
export const MOMOSHTREM_CONFIG = {
  /** Base URL for momoshtrem API (from environment) - read at runtime */
  get baseUrl(): string {
    return process.env.MOMOSHTREM_URL || 'http://localhost:4444';
  },

  /** Request timeout in milliseconds (longer for torrent metadata fetching) */
  timeout: 30000,
} as const;
