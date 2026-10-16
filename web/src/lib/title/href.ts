import type { MediaType } from '@/lib/api/contract';

/** A title's page: /movie/{tmdb_id} or /tv/{tmdb_id}. */
export const titleHref = (mediaType: MediaType, tmdbId: number) => `/${mediaType === 'movie' ? 'movie' : 'tv'}/${tmdbId}`;
