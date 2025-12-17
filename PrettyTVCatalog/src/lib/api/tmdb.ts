import { MovieDb } from 'moviedb-promise';
import type { MovieResponse, ShowResponse, MovieResult, TvResult } from 'moviedb-promise';
import { TRENDING_TIME_WINDOW, SEARCH_MAX_RESULTS } from '@/config/tmdb';
import { APIError, NotFoundError } from '@/lib/errors';
import type {
  Movie,
  MovieDetails,
  TVShow,
  TVShowDetails,
  Season,
  SeasonDetails,
  SearchResult,
  MovieSearchResult,
  TVSearchResult,
  TrendingResults,
  CastMember,
  CrewMember,
} from '@/types/tmdb';

// ============================================
// TMDB Client using moviedb-promise
// ============================================

class TMDBClient {
  private client: MovieDb;

  constructor() {
    const apiKey = process.env.TMDB_API_KEY;
    if (!apiKey) {
      throw new Error('TMDB_API_KEY environment variable is not set');
    }
    this.client = new MovieDb(apiKey);
  }

  // ----------------------------------------
  // Error Handling
  // ----------------------------------------

  /**
   * Convert library errors to our error types.
   */
  private handleError(error: unknown, resource?: string): never {
    if (error instanceof Error) {
      const statusMatch = error.message.match(/status code (\d+)/);
      const statusCode = statusMatch ? parseInt(statusMatch[1], 10) : 500;

      if (statusCode === 404) {
        throw new NotFoundError(resource ?? 'Resource');
      }

      throw new APIError(`TMDB API request failed: ${error.message}`, statusCode);
    }

    throw new APIError('TMDB API request failed: Unknown error', 500);
  }

  // ----------------------------------------
  // Transform Functions
  // ----------------------------------------

  private transformMovie(raw: MovieResponse): Movie {
    return {
      id: raw.id ?? 0,
      title: raw.title ?? '',
      overview: raw.overview ?? '',
      posterPath: raw.poster_path ?? null,
      backdropPath: raw.backdrop_path ?? null,
      releaseDate: raw.release_date ?? '',
      voteAverage: raw.vote_average ?? 0,
      voteCount: raw.vote_count ?? 0,
      runtime: raw.runtime ?? null,
      genres: (raw.genres ?? []).map((g) => ({ id: g.id ?? 0, name: g.name ?? '' })),
      tagline: raw.tagline ?? null,
    };
  }

  private transformTVShow(raw: ShowResponse): TVShow {
    return {
      id: raw.id ?? 0,
      name: raw.name ?? '',
      overview: raw.overview ?? '',
      posterPath: raw.poster_path ?? null,
      backdropPath: raw.backdrop_path ?? null,
      firstAirDate: raw.first_air_date ?? '',
      voteAverage: raw.vote_average ?? 0,
      voteCount: raw.vote_count ?? 0,
      numberOfSeasons: raw.number_of_seasons ?? 0,
      numberOfEpisodes: raw.number_of_episodes ?? 0,
      genres: (raw.genres ?? []).map((g) => ({ id: g.id ?? 0, name: g.name ?? '' })),
      tagline: raw.tagline ?? null,
    };
  }

  private transformCast(
    raw: Array<{
      id?: number;
      name?: string;
      character?: string;
      profile_path?: string | null;
      order?: number;
    }>
  ): CastMember[] {
    return raw.map((member) => ({
      id: member.id ?? 0,
      name: member.name ?? '',
      character: member.character ?? '',
      profilePath: member.profile_path ?? null,
      order: member.order ?? 0,
    }));
  }

  private transformCrew(
    raw: Array<{
      id?: number;
      name?: string;
      job?: string;
      department?: string;
      profile_path?: string | null;
    }>
  ): CrewMember[] {
    return raw.map((member) => ({
      id: member.id ?? 0,
      name: member.name ?? '',
      job: member.job ?? '',
      department: member.department ?? '',
      profilePath: member.profile_path ?? null,
    }));
  }

  private transformSearchResult(raw: {
    id?: number;
    media_type?: string;
    poster_path?: string | null;
    backdrop_path?: string | null;
    vote_average?: number;
    overview?: string;
    title?: string;
    release_date?: string;
    name?: string;
    first_air_date?: string;
  }): SearchResult | null {
    if (raw.media_type === 'person') {
      return null;
    }

    const base = {
      id: raw.id ?? 0,
      posterPath: raw.poster_path ?? null,
      backdropPath: raw.backdrop_path ?? null,
      voteAverage: raw.vote_average ?? 0,
      overview: raw.overview ?? '',
    };

    if (raw.media_type === 'movie') {
      return {
        ...base,
        mediaType: 'movie',
        title: raw.title ?? '',
        releaseDate: raw.release_date ?? '',
      } as MovieSearchResult;
    }

    return {
      ...base,
      mediaType: 'tv',
      name: raw.name ?? '',
      firstAirDate: raw.first_air_date ?? '',
    } as TVSearchResult;
  }

  // ----------------------------------------
  // Public API Methods
  // ----------------------------------------

  /**
   * Get trending movies and TV shows.
   */
  async getTrending(): Promise<TrendingResults> {
    try {
      const [moviesResponse, tvResponse] = await Promise.all([
        this.client.trending({
          media_type: 'movie',
          time_window: TRENDING_TIME_WINDOW,
        }),
        this.client.trending({
          media_type: 'tv',
          time_window: TRENDING_TIME_WINDOW,
        }),
      ]);

      const movieResults = (moviesResponse.results ?? []) as MovieResult[];
      const movies: MovieSearchResult[] = movieResults
        .slice(0, SEARCH_MAX_RESULTS)
        .map((item) => ({
          id: item.id ?? 0,
          mediaType: 'movie' as const,
          posterPath: item.poster_path ?? null,
          backdropPath: item.backdrop_path ?? null,
          voteAverage: item.vote_average ?? 0,
          overview: item.overview ?? '',
          title: item.title ?? '',
          releaseDate: item.release_date ?? '',
        }));

      const tvResults = (tvResponse.results ?? []) as TvResult[];
      const tvShows: TVSearchResult[] = tvResults
        .slice(0, SEARCH_MAX_RESULTS)
        .map((item) => ({
          id: item.id ?? 0,
          mediaType: 'tv' as const,
          posterPath: item.poster_path ?? null,
          backdropPath: item.backdrop_path ?? null,
          voteAverage: item.vote_average ?? 0,
          overview: item.overview ?? '',
          name: item.name ?? '',
          firstAirDate: item.first_air_date ?? '',
        }));

      return { movies, tvShows };
    } catch (error) {
      throw this.handleError(error);
    }
  }

  /**
   * Search for movies and TV shows.
   */
  async search(query: string): Promise<SearchResult[]> {
    if (!query.trim()) {
      return [];
    }

    try {
      const response = await this.client.searchMulti({ query: query.trim() });

      return (response.results ?? [])
        .filter((item) => item.media_type === 'movie' || item.media_type === 'tv')
        .slice(0, SEARCH_MAX_RESULTS)
        .map((item) => this.transformSearchResult(item))
        .filter((item): item is SearchResult => item !== null);
    } catch (error) {
      throw this.handleError(error);
    }
  }

  /**
   * Get movie details with credits.
   */
  async getMovie(id: number): Promise<MovieDetails> {
    try {
      const response = (await this.client.movieInfo({
        id,
        append_to_response: 'credits',
      })) as MovieResponse & {
        credits?: { cast?: Array<Record<string, unknown>>; crew?: Array<Record<string, unknown>> };
      };

      return {
        ...this.transformMovie(response),
        credits: {
          cast: this.transformCast(response.credits?.cast ?? []),
          crew: this.transformCrew(response.credits?.crew ?? []),
        },
      };
    } catch (error) {
      throw this.handleError(error, 'Movie');
    }
  }

  private transformSeasons(
    raw: Array<{
      id?: number;
      name?: string;
      season_number?: number;
      episode_count?: number;
      air_date?: string | null;
      overview?: string;
      poster_path?: string | null;
    }>
  ): Season[] {
    return raw.map((season) => ({
      id: season.id ?? 0,
      name: season.name ?? '',
      seasonNumber: season.season_number ?? 0,
      episodeCount: season.episode_count ?? 0,
      airDate: season.air_date ?? null,
      overview: season.overview ?? '',
      posterPath: season.poster_path ?? null,
    }));
  }

  /**
   * Get TV show details with credits.
   */
  async getTVShow(id: number): Promise<TVShowDetails> {
    try {
      const response = (await this.client.tvInfo({
        id,
        append_to_response: 'credits',
      })) as ShowResponse & {
        credits?: { cast?: Array<Record<string, unknown>>; crew?: Array<Record<string, unknown>> };
        seasons?: Array<Record<string, unknown>>;
      };

      return {
        ...this.transformTVShow(response),
        credits: {
          cast: this.transformCast(response.credits?.cast ?? []),
          crew: this.transformCrew(response.credits?.crew ?? []),
        },
        seasons: this.transformSeasons(response.seasons ?? []),
      };
    } catch (error) {
      throw this.handleError(error, 'TV show');
    }
  }

  /**
   * Get season details with episodes.
   */
  async getSeason(showId: number, seasonNumber: number): Promise<SeasonDetails> {
    try {
      const response = await this.client.seasonInfo({
        id: showId,
        season_number: seasonNumber,
      });

      return {
        id: response.id ?? 0,
        name: response.name ?? '',
        seasonNumber: response.season_number ?? 0,
        airDate: response.air_date ?? null,
        overview: response.overview ?? '',
        posterPath: response.poster_path ?? null,
        episodes: (response.episodes ?? []).map((ep) => ({
          id: ep.id ?? 0,
          name: ep.name ?? '',
          overview: ep.overview ?? '',
          episodeNumber: ep.episode_number ?? 0,
          seasonNumber: ep.season_number ?? 0,
          airDate: ep.air_date ?? null,
          stillPath: ep.still_path ?? null,
          voteAverage: ep.vote_average ?? 0,
          runtime: ep.runtime ?? null,
        })),
      };
    } catch (error) {
      throw this.handleError(error, 'Season');
    }
  }
}

// Export singleton instance
export const tmdbClient = new TMDBClient();

// Export class for testing
export { TMDBClient };
