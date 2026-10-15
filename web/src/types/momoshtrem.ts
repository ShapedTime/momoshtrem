/**
 * momoshtrem API Types
 *
 * Types for the library-first media streaming service.
 * momoshtrem manages a SQLite library of movies/shows and assigns torrents to them.
 */

// =============================================================================
// Library Models
// =============================================================================

export interface LibraryMovie {
  id: number;
  tmdb_id: number;
  title: string;
  year: number;
  created_at: string;
  has_assignment: boolean;
  assignment?: TorrentAssignment;
}

export interface LibraryShow {
  id: number;
  tmdb_id: number;
  title: string;
  year: number;
  created_at: string;
  /** Optional - not included in list endpoint response for performance */
  seasons?: LibrarySeason[];
}

export interface LibrarySeason {
  id: number;
  season_number: number;
  /** Optional - backend omits empty episode lists (Go `omitempty`). */
  episodes?: LibraryEpisode[];
}

export interface LibraryEpisode {
  id: number;
  episode_number: number;
  name: string;
  has_assignment: boolean;
  assignment?: TorrentAssignment;
}

export interface TorrentAssignment {
  id: number;
  info_hash: string;
  file_path: string;
  file_size: number;
  resolution?: string;
  source?: string;
}

// =============================================================================
// API Request Types
// =============================================================================

export interface AddMovieRequest {
  tmdb_id: number;
}

export interface AddShowRequest {
  tmdb_id: number;
}

// =============================================================================
// Library Status Types (for UI)
// =============================================================================

export type LibraryStatus =
  | 'not_in_library'
  | 'in_library'
  | 'has_assignment';

export interface LibraryItemStatus {
  status: LibraryStatus;
  libraryId?: number;
  hasAssignment: boolean;
}

// =============================================================================
// API Error Types
// =============================================================================

/**
 * Error body returned by momoshtrem. Current backends send
 * `{ error: { code, message } }`; older ones sent `{ error: "message" }`.
 */
export type MomoshtremError =
  | { error: { code: string; message: string } }
  | { error: string };

/**
 * Extract code and message from a momoshtrem error body of either shape.
 * Returns null when the body isn't a recognisable error.
 */
export function parseMomoshtremError(
  body: unknown
): { code?: string; message: string } | null {
  if (typeof body !== 'object' || body === null || !('error' in body)) {
    return null;
  }
  const { error } = body as { error: unknown };
  if (typeof error === 'string') {
    return error ? { message: error } : null;
  }
  if (typeof error === 'object' && error !== null) {
    const { code, message } = error as { code?: unknown; message?: unknown };
    if (typeof message === 'string' && message) {
      return { code: typeof code === 'string' ? code : undefined, message };
    }
  }
  return null;
}

// =============================================================================
// Type Guards
// =============================================================================

export function isLibraryMovie(item: LibraryMovie | LibraryShow): item is LibraryMovie {
  return 'has_assignment' in item && !('seasons' in item);
}

export function isLibraryShow(item: LibraryMovie | LibraryShow): item is LibraryShow {
  return 'seasons' in item;
}

// =============================================================================
// Recently Aired Types
// =============================================================================

export interface RecentlyAiredEpisode {
  show_id: number;
  show_tmdb_id: number;
  show_title: string;
  show_year: number;
  season_number: number;
  episode_id: number;
  episode_number: number;
  episode_name: string;
  air_date: string;
  has_assignment: boolean;
}

export interface RecentlyAiredResponse {
  episodes: RecentlyAiredEpisode[];
  last_sync_time: string;
  sync_status: 'ok' | 'pending' | 'in_progress' | 'error' | 'disabled';
}
