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
  seasons: LibrarySeason[];
}

export interface LibrarySeason {
  id: number;
  season_number: number;
  episodes: LibraryEpisode[];
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

export interface AssignTorrentRequest {
  magnet_uri: string;
}

/**
 * Combined request for adding torrent (auto-adds to library if needed)
 */
export interface AddTorrentRequest {
  magnet_uri: string;
  media_type: 'movie' | 'tv';
  tmdb_id: number;
  title?: string;
  year?: number;
}

// =============================================================================
// API Response Types
// =============================================================================

export interface MovieAssignmentResponse {
  success: boolean;
  assignment: TorrentAssignment;
}

export interface ShowAssignmentResponse {
  success: boolean;
  summary: AssignmentSummary;
  matched: EpisodeMatch[];
  unmatched: UnmatchedFile[];
}

export interface AssignmentSummary {
  total_files: number;
  matched: number;
  unmatched: number;
  skipped: number;
}

export interface EpisodeMatch {
  episode_id: number;
  season: number;
  episode: number;
  file_path: string;
  file_size: number;
  resolution?: string;
  confidence: 'high' | 'medium' | 'low';
}

export interface UnmatchedFile {
  file_path: string;
  reason: string;
  season?: number;
  episode?: number;
}

/**
 * Combined response for add-torrent endpoint
 */
export interface AddTorrentResponse {
  success: boolean;
  added_to_library: boolean;
  library_id: number;
  media_type: 'movie' | 'tv';
  // For movies
  assignment?: TorrentAssignment;
  // For shows
  summary?: AssignmentSummary;
  matched?: EpisodeMatch[];
  unmatched?: UnmatchedFile[];
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

export interface MomoshtremError {
  error: string;
  code?: string;
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

export function isMovieAssignmentResponse(
  response: MovieAssignmentResponse | ShowAssignmentResponse
): response is MovieAssignmentResponse {
  return 'assignment' in response && !('summary' in response);
}

export function isShowAssignmentResponse(
  response: MovieAssignmentResponse | ShowAssignmentResponse
): response is ShowAssignmentResponse {
  return 'summary' in response && 'matched' in response;
}
