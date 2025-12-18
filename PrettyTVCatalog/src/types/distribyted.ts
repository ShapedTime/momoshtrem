/**
 * Distribyted API types.
 * Based on distribyted REST API (http/api.go)
 */

// ============================================
// Request Types
// ============================================

/**
 * Request body for adding a torrent via magnet URI.
 * Matches distribyted's RouteAdd struct.
 */
export interface AddTorrentRequest {
  /** Magnet URI (required) - must start with "magnet:?" */
  magnet: string;
}

// ============================================
// Response Types
// ============================================

/**
 * Error response from distribyted API.
 */
export interface DistribytedError {
  error: string;
}
