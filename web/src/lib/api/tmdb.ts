import { MovieDb } from 'moviedb-promise';
import type { MovieResponse, ShowResponse, MovieResult, TvResult } from 'moviedb-promise';
import { TRENDING_TIME_WINDOW, SEARCH_MAX_RESULTS } from '@/config/tmdb';
import { APIError, NotFoundError } from '@/lib/errors';
import type { DiscoverPage, DiscoverRelaxed } from '@/lib/api/contract';
import { type DiscoverFilters, withoutFilter } from '@/lib/discover/filters';
import {
  type DiscoverRow,
  type DiscoverStream,
  discoverStreams,
  isoDay,
  MAX_TMDB_PAGE,
  mergeStreams,
  relaxKeys,
  toDiscoverItem,
  yearOf,
} from '@/lib/discover/tmdb-query';
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
  private client: MovieDb | null = null;

  private getClient(): MovieDb {
    if (!this.client) {
      const apiKey = process.env.TMDB_API_KEY;
      if (!apiKey) {
        throw new APIError('TMDB_API_KEY isn’t set, so TMDB can’t be reached. Add it to .env and restart.', 503, 'tmdb_not_configured');
      }
      this.client = new MovieDb(apiKey);
    }
    return this.client;
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

      // TMDB rejecting our API key is a server problem, not the user's session:
      // a 401 here would send the browser to /login.
      if (statusCode === 401 || statusCode === 403) {
        throw new APIError('TMDB rejected the API key. Check TMDB_API_KEY.', 502, 'tmdb_auth');
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
      const client = this.getClient();
      const [moviesResponse, tvResponse] = await Promise.all([
        client.trending({
          media_type: 'movie',
          time_window: TRENDING_TIME_WINDOW,
        }),
        client.trending({
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
      const response = await this.getClient().searchMulti({ query: query.trim() });

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
      const response = (await this.getClient().movieInfo({
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
      const response = (await this.getClient().tvInfo({
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
      const response = await this.getClient().seasonInfo({
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

  // ----------------------------------------
  // Discover (#100)
  // ----------------------------------------

  /**
   * One page of Discover: a TMDB query per type that can match, interleaved
   * by the sort key (lib/discover/tmdb-query.ts). `total` is the streams'
   * sum and `total_pages` the larger, capped at TMDB's 500.
   */
  async discover(filters: DiscoverFilters, page: number): Promise<DiscoverPage> {
    const streams = discoverStreams(filters, page, isoDay());
    if (!streams.length) return { items: [], total: 0, page, total_pages: 0 };
    const answers = await Promise.all(streams.map((stream) => this.discoverStream(stream)));
    return {
      items: mergeStreams(filters.sort, answers.map((answer) => answer.rows)).map(toDiscoverItem),
      total: answers.reduce((sum, answer) => sum + answer.total, 0),
      page,
      total_pages: Math.min(MAX_TMDB_PAGE, Math.max(...answers.map((answer) => answer.totalPages))),
    };
  }

  /** `relax=1`: the total without each active filter, page 1 only, all at once. */
  async discoverRelaxed(filters: DiscoverFilters): Promise<DiscoverRelaxed> {
    const relaxed = await Promise.all(
      relaxKeys(filters).map(async (filter) => {
        const streams = discoverStreams(withoutFilter(filters, filter), 1, isoDay());
        const answers = await Promise.all(streams.map((stream) => this.discoverStream(stream)));
        return { filter, would_be: answers.reduce((sum, answer) => sum + answer.total, 0) };
      })
    );
    return { relaxed };
  }

  private async discoverStream({ mediaType, params }: DiscoverStream): Promise<{ rows: DiscoverRow[]; total: number; totalPages: number }> {
    const client = this.getClient();
    const config = { timeout: TMDB_DISCOVER_TIMEOUT_MS };
    try {
      if (mediaType === 'movie') {
        const response = await client.discoverMovie(params, config);
        const rows = ((response.results ?? []) as MovieResult[]).map((item) => ({
          media_type: 'movie' as const,
          tmdb_id: item.id ?? 0,
          title: item.title ?? '',
          year: yearOf(item.release_date),
          poster_path: item.poster_path ?? null,
          popularity: item.popularity ?? 0,
          vote_average: item.vote_average ?? 0,
          date: item.release_date ?? '',
        }));
        return { rows, total: response.total_results ?? 0, totalPages: response.total_pages ?? 0 };
      }
      const response = await client.discoverTv(params, config);
      const rows = ((response.results ?? []) as TvResult[]).map((item) => ({
        media_type: 'tv' as const,
        tmdb_id: item.id ?? 0,
        title: item.name ?? '',
        year: yearOf(item.first_air_date),
        poster_path: item.poster_path ?? null,
        popularity: item.popularity ?? 0,
        vote_average: item.vote_average ?? 0,
        date: item.first_air_date ?? '',
      }));
      return { rows, total: response.total_results ?? 0, totalPages: response.total_pages ?? 0 };
    } catch (error) {
      throw discoverError(error);
    }
  }
}

/** A hung TMDB mustn't hold the page: give up and say so. */
const TMDB_DISCOVER_TIMEOUT_MS = 10_000;

/**
 * Discover's errors: a rejected key is 502 `tmdb_auth` (a 401 would send the
 * browser to sign in), anything else TMDB does wrong is 502 `tmdb_unavailable`.
 */
function discoverError(error: unknown): APIError {
  const status = Number(/status code (\d+)/.exec(error instanceof Error ? error.message : '')?.[1]);
  if (status === 401 || status === 403) return new APIError('TMDB rejected the API key. Check TMDB_API_KEY.', 502, 'tmdb_auth');
  return new APIError('TMDB didn’t answer. Try again in a moment.', 502, 'tmdb_unavailable');
}

// Export singleton instance
export const tmdbClient = new TMDBClient();

// Export class for testing
export { TMDBClient };
