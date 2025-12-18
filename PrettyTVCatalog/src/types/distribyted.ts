/**
 * Distribyted API types.
 * Based on distribyted REST API (http/api.go)
 */

// ============================================
// Media Type
// ============================================

/**
 * Type of media (movie or TV show).
 */
export type MediaType = 'movie' | 'tv';

// ============================================
// TMDB Metadata
// ============================================

/**
 * TMDB metadata to associate with a torrent.
 * Used for Infuse media identification.
 */
export interface TMDBMetadata {
  /** Media type (movie or tv) */
  type: MediaType;
  /** TMDB ID */
  tmdb_id: number;
  /** Media title */
  title: string;
  /** Release year */
  year: number;
  /** Season number (for TV season packs) */
  season?: number;
  /** Episode number (for single TV episodes) */
  episode?: number;
}

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
  /** Optional TMDB metadata for media identification */
  metadata?: TMDBMetadata;
}

// ============================================
// Response Types
// ============================================

/**
 * Information about a torrent stored in distribyted.
 */
export interface TorrentInfo {
  /** Torrent info hash */
  hash: string;
  /** Torrent name */
  name: string;
  /** Optional TMDB metadata */
  metadata?: TMDBMetadata;
}

/**
 * Error response from distribyted API.
 */
export interface DistribytedError {
  error: string;
}
