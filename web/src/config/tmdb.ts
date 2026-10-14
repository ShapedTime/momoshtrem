// TMDB API base URL
export const TMDB_BASE_URL = 'https://api.themoviedb.org/3';

// TMDB Image base URL
export const TMDB_IMAGE_BASE_URL = 'https://image.tmdb.org/t/p';

// Image size presets for different use cases
export const TMDB_IMAGE_SIZES = {
  poster: {
    small: 'w185',
    medium: 'w342',
    large: 'w500',
    original: 'original',
  },
  backdrop: {
    small: 'w300',
    medium: 'w780',
    large: 'w1280',
    original: 'original',
  },
  profile: {
    small: 'w45',
    medium: 'w185',
    large: 'h632',
    original: 'original',
  },
  still: {
    small: 'w92',
    medium: 'w185',
    large: 'w300',
    original: 'original',
  },
} as const;

/**
 * Build a complete TMDB image URL.
 * Returns null if path is null.
 */
export function buildImageUrl(
  path: string | null,
  type: keyof typeof TMDB_IMAGE_SIZES,
  size: 'small' | 'medium' | 'large' | 'original' = 'medium'
): string | null {
  if (!path) return null;
  const sizeValue = TMDB_IMAGE_SIZES[type][size];
  return `${TMDB_IMAGE_BASE_URL}/${sizeValue}${path}`;
}

// Time window for trending content
export const TRENDING_TIME_WINDOW = 'week' as const;

// Maximum results to return from search/trending
export const SEARCH_MAX_RESULTS = 20;
