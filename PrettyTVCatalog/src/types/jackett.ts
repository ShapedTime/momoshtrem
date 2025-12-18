// ============================================
// Video Quality Types
// ============================================

/**
 * Video quality parsed from torrent title.
 */
export type VideoQuality = '4K' | '1080p' | '720p' | '480p' | 'Unknown';

// ============================================
// Torrent Result Types
// ============================================

/**
 * Individual torrent search result from Jackett.
 */
export interface TorrentResult {
  /** Unique identifier (info hash or indexer-specific ID) */
  guid: string;
  /** Full torrent title */
  title: string;
  /** File size in bytes */
  size: number;
  /** Number of seeders */
  seeders: number;
  /** Number of leechers */
  leechers: number;
  /** Magnet URI (guaranteed to be a valid magnet link) */
  magnetUri: string;
  /** Indexer/tracker name */
  indexer: string;
  /** Publication date */
  publishDate: string | null;
  /** Parsed video quality from title */
  quality: VideoQuality;
}

/**
 * Jackett search API response.
 */
export interface JackettSearchResponse {
  results: TorrentResult[];
  query: string;
}

// ============================================
// Search Context Types
// ============================================

/**
 * Context for initiating a torrent search from different sources.
 */
export interface TorrentSearchContext {
  /** Media type being searched */
  mediaType: 'movie' | 'tv' | 'episode';
  /** Pre-filled search query */
  query: string;
  /** Media title for display in modal header */
  title: string;
  /** Release year (for movies) */
  year?: number;
  /** Season number (for episodes) */
  season?: number;
  /** Episode number (for episodes) */
  episode?: number;
}

// ============================================
// Sort Options
// ============================================

export type TorrentSortField = 'seeders' | 'size' | 'publishDate';
export type SortDirection = 'asc' | 'desc';

// ============================================
// Helper Functions
// ============================================

/**
 * Parse video quality from torrent title.
 */
export function parseQualityFromTitle(title: string): VideoQuality {
  const titleLower = title.toLowerCase();

  // Check for 4K variants first (most specific)
  if (
    titleLower.includes('2160p') ||
    titleLower.includes('4k') ||
    titleLower.includes('uhd')
  ) {
    return '4K';
  }
  if (titleLower.includes('1080p') || titleLower.includes('1080i')) {
    return '1080p';
  }
  if (titleLower.includes('720p')) {
    return '720p';
  }
  if (
    titleLower.includes('480p') ||
    titleLower.includes('dvd') ||
    titleLower.includes('sd')
  ) {
    return '480p';
  }

  return 'Unknown';
}

/**
 * Format file size for display (bytes to human-readable).
 */
export function formatFileSize(bytes: number): string {
  if (bytes === 0) return '0 B';

  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  const k = 1024;
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  const size = bytes / Math.pow(k, i);

  return `${size.toFixed(i > 0 ? 1 : 0)} ${units[i]}`;
}
