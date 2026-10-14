// ============================================
// Core Media Types
// ============================================

export interface Genre {
  id: number;
  name: string;
}

export interface Movie {
  id: number;
  title: string;
  overview: string;
  posterPath: string | null;
  backdropPath: string | null;
  releaseDate: string;
  voteAverage: number;
  voteCount: number;
  runtime: number | null;
  genres: Genre[];
  tagline: string | null;
}

export interface TVShow {
  id: number;
  name: string;
  overview: string;
  posterPath: string | null;
  backdropPath: string | null;
  firstAirDate: string;
  voteAverage: number;
  voteCount: number;
  numberOfSeasons: number;
  numberOfEpisodes: number;
  genres: Genre[];
  tagline: string | null;
}

// ============================================
// Credits & Cast
// ============================================

export interface CastMember {
  id: number;
  name: string;
  character: string;
  profilePath: string | null;
  order: number;
}

export interface CrewMember {
  id: number;
  name: string;
  job: string;
  department: string;
  profilePath: string | null;
}

export interface Credits {
  cast: CastMember[];
  crew: CrewMember[];
}

// ============================================
// Movie/TV with Credits (detail views)
// ============================================

export interface MovieDetails extends Movie {
  credits: Credits;
}

export interface TVShowDetails extends TVShow {
  credits: Credits;
  seasons: Season[];
}

// ============================================
// TV Seasons & Episodes
// ============================================

export interface Season {
  id: number;
  name: string;
  seasonNumber: number;
  episodeCount: number;
  airDate: string | null;
  overview: string;
  posterPath: string | null;
}

export interface Episode {
  id: number;
  name: string;
  overview: string;
  episodeNumber: number;
  seasonNumber: number;
  airDate: string | null;
  stillPath: string | null;
  voteAverage: number;
  runtime: number | null;
}

export interface SeasonDetails {
  id: number;
  name: string;
  seasonNumber: number;
  airDate: string | null;
  overview: string;
  posterPath: string | null;
  episodes: Episode[];
}

// ============================================
// Search Results
// ============================================

export type MediaType = 'movie' | 'tv';

export interface SearchResultBase {
  id: number;
  mediaType: MediaType;
  posterPath: string | null;
  backdropPath: string | null;
  voteAverage: number;
  overview: string;
}

export interface MovieSearchResult extends SearchResultBase {
  mediaType: 'movie';
  title: string;
  releaseDate: string;
}

export interface TVSearchResult extends SearchResultBase {
  mediaType: 'tv';
  name: string;
  firstAirDate: string;
}

export type SearchResult = MovieSearchResult | TVSearchResult;

// ============================================
// Trending Results
// ============================================

export interface TrendingResults {
  movies: MovieSearchResult[];
  tvShows: TVSearchResult[];
}

// ============================================
// Type Guards
// ============================================

export function isMovie(item: SearchResult): item is MovieSearchResult {
  return item.mediaType === 'movie';
}

export function isTVShow(item: SearchResult): item is TVSearchResult {
  return item.mediaType === 'tv';
}

// ============================================
// Helper Functions
// ============================================

export function getMediaTitle(item: SearchResult): string {
  return isMovie(item) ? item.title : item.name;
}

export function getMediaReleaseYear(item: SearchResult): number | null {
  const dateStr = isMovie(item) ? item.releaseDate : item.firstAirDate;
  if (!dateStr) return null;
  const year = parseInt(dateStr.substring(0, 4), 10);
  return isNaN(year) ? null : year;
}

// ============================================
// Discover & Browse Types
// ============================================

export type MovieSortOption =
  | 'popularity.desc'
  | 'popularity.asc'
  | 'vote_average.desc'
  | 'vote_average.asc'
  | 'primary_release_date.desc'
  | 'primary_release_date.asc'
  | 'original_title.asc'
  | 'original_title.desc'
  | 'vote_count.desc'
  | 'vote_count.asc';

export type TVSortOption =
  | 'popularity.desc'
  | 'popularity.asc'
  | 'vote_average.desc'
  | 'vote_average.asc'
  | 'first_air_date.desc'
  | 'first_air_date.asc'
  | 'name.asc'
  | 'name.desc'
  | 'vote_count.desc'
  | 'vote_count.asc';

export type SortOption = MovieSortOption | TVSortOption;

export interface SortOptionConfig {
  value: SortOption;
  label: string;
  movieValue?: MovieSortOption;
  tvValue?: TVSortOption;
}

export interface DiscoverOptions {
  genreId?: number;
  sortBy?: SortOption;
  page?: number;
  voteCountGte?: number;
}

export interface DiscoverResults<T> {
  results: T[];
  page: number;
  totalPages: number;
  totalResults: number;
}

export interface GenreList {
  genres: Genre[];
}
