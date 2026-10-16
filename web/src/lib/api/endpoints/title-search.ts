import { keepPreviousData, queryOptions } from '@tanstack/react-query';
import type { TrendingResults } from '@/types/tmdb';
import { apiJson } from '../client';
import type { MediaType, TitleSearchResult } from '../contract';

/**
 * Title search (#101): the web route GET /api/title-search (web-side, not
 * PLAN §6), and trending from the legacy web route GET /api/tmdb/trending
 * for the search page's idle and none states.
 */
export const titleSearchKeys = {
  all: ['title-search'] as const,
  search: (query: string) => ['title-search', query] as const,
  trending: ['tmdb', 'trending'] as const,
};

export const titleSearchApi = {
  search: (query: string) => apiJson<TitleSearchResult>(`/api/title-search?q=${encodeURIComponent(query)}`),
  trending: () => apiJson<TrendingResults>('/api/tmdb/trending'),
};

/** Titles for a query (from `cleanQuery`); a blank one asks nothing. Earlier results stay while the next query loads. */
export const titleSearchQuery = (query: string) =>
  queryOptions({
    queryKey: titleSearchKeys.search(query),
    queryFn: () => titleSearchApi.search(query),
    enabled: query !== '',
    staleTime: 5 * 60_000,
    placeholderData: keepPreviousData,
  });

/** A trending title as the search page's cards need it. */
export interface TrendingTitle {
  media_type: MediaType;
  tmdb_id: number;
  title: string;
  year: number | null;
  poster_path: string | null;
}

const yearOf = (date: string) => Number.parseInt(date.slice(0, 4), 10) || null;

/** This week's trending movies and shows, taken in turns so both show. */
export function trendingTitles({ movies, tvShows }: TrendingResults): TrendingTitle[] {
  const titles: TrendingTitle[] = [];
  for (let i = 0; i < Math.max(movies.length, tvShows.length); i++) {
    const show = tvShows[i];
    const movie = movies[i];
    if (show) titles.push({ media_type: 'tv', tmdb_id: show.id, title: show.name, year: yearOf(show.firstAirDate), poster_path: show.posterPath });
    if (movie) titles.push({ media_type: 'movie', tmdb_id: movie.id, title: movie.title, year: yearOf(movie.releaseDate), poster_path: movie.posterPath });
  }
  return titles;
}

export const trendingTitlesQuery = () =>
  queryOptions({ queryKey: titleSearchKeys.trending, queryFn: titleSearchApi.trending, select: trendingTitles, staleTime: 30 * 60_000 });
