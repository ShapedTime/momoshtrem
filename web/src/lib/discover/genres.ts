import type { MediaType } from '@/lib/api/contract';

/**
 * Discover's genres (js/screens/discover.js `GENRES`) and their TMDB ids per
 * type, checked against TMDB's /genre/movie/list and /genre/tv/list.
 *
 * TMDB's TV list merges some movie genres: "Action & Adventure" (10759),
 * "Sci-Fi & Fantasy" (10765) and "War & Politics" (10768). It has no History,
 * Romance or Thriller, so those are `null` for TV, and a search that needs
 * one of them skips the TV stream.
 */

export const GENRE_SLUGS = [
  'action',
  'adventure',
  'animation',
  'comedy',
  'documentary',
  'drama',
  'family',
  'fantasy',
  'history',
  'mystery',
  'romance',
  'sci-fi',
  'thriller',
  'war',
] as const;

export type GenreSlug = (typeof GENRE_SLUGS)[number];

export interface DiscoverGenre {
  slug: GenreSlug;
  label: string;
  /** TMDB genre id per type; null when TMDB has no such genre for that type. */
  ids: Record<MediaType, number | null>;
}

export const GENRES: DiscoverGenre[] = [
  { slug: 'action', label: 'Action', ids: { movie: 28, tv: 10759 } },
  { slug: 'adventure', label: 'Adventure', ids: { movie: 12, tv: 10759 } },
  { slug: 'animation', label: 'Animation', ids: { movie: 16, tv: 16 } },
  { slug: 'comedy', label: 'Comedy', ids: { movie: 35, tv: 35 } },
  { slug: 'documentary', label: 'Documentary', ids: { movie: 99, tv: 99 } },
  { slug: 'drama', label: 'Drama', ids: { movie: 18, tv: 18 } },
  { slug: 'family', label: 'Family', ids: { movie: 10751, tv: 10751 } },
  { slug: 'fantasy', label: 'Fantasy', ids: { movie: 14, tv: 10765 } },
  { slug: 'history', label: 'History', ids: { movie: 36, tv: null } },
  { slug: 'mystery', label: 'Mystery', ids: { movie: 9648, tv: 9648 } },
  { slug: 'romance', label: 'Romance', ids: { movie: 10749, tv: null } },
  { slug: 'sci-fi', label: 'Sci-Fi', ids: { movie: 878, tv: 10765 } },
  { slug: 'thriller', label: 'Thriller', ids: { movie: 53, tv: null } },
  { slug: 'war', label: 'War', ids: { movie: 10752, tv: 10768 } },
];

/** Animation: anime is Animation plus Japanese as the original language. */
export const ANIMATION_GENRE_ID = 16;

const BY_SLUG = new Map(GENRES.map((genre) => [genre.slug, genre]));

export function genreBySlug(slug: GenreSlug): DiscoverGenre {
  const genre = BY_SLUG.get(slug);
  if (!genre) throw new Error(`Unknown genre: ${slug}`);
  return genre;
}

export const isGenreSlug = (value: string): value is GenreSlug => BY_SLUG.has(value as GenreSlug);

/**
 * TMDB `with_genres` ids for these genres on one type, each once (Action and
 * Adventure are both 10759 on TV). Null when one of them has no id for the type.
 */
export function genreIds(slugs: readonly GenreSlug[], mediaType: MediaType): number[] | null {
  const ids: number[] = [];
  for (const slug of slugs) {
    const id = genreBySlug(slug).ids[mediaType];
    if (id === null) return null;
    if (!ids.includes(id)) ids.push(id);
  }
  return ids;
}
