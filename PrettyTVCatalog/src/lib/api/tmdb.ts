import { TMDB_BASE_URL, TRENDING_TIME_WINDOW, SEARCH_MAX_RESULTS } from '@/config/tmdb';
import { APIError, NotFoundError } from '@/lib/errors';
import type {
  Movie,
  MovieDetails,
  TVShow,
  TVShowDetails,
  SeasonDetails,
  SearchResult,
  MovieSearchResult,
  TVSearchResult,
  TrendingResults,
  CastMember,
  CrewMember,
} from '@/types/tmdb';

// ============================================
// Raw TMDB Response Types (snake_case)
// ============================================

interface TMDBRawMovie {
  id: number;
  title: string;
  overview: string;
  poster_path: string | null;
  backdrop_path: string | null;
  release_date: string;
  vote_average: number;
  vote_count: number;
  runtime: number | null;
  genres: Array<{ id: number; name: string }>;
  tagline: string | null;
}

interface TMDBRawTVShow {
  id: number;
  name: string;
  overview: string;
  poster_path: string | null;
  backdrop_path: string | null;
  first_air_date: string;
  vote_average: number;
  vote_count: number;
  number_of_seasons: number;
  number_of_episodes: number;
  genres: Array<{ id: number; name: string }>;
  tagline: string | null;
}

interface TMDBRawSearchResult {
  id: number;
  media_type: 'movie' | 'tv' | 'person';
  poster_path: string | null;
  backdrop_path: string | null;
  vote_average: number;
  overview: string;
  title?: string;
  release_date?: string;
  name?: string;
  first_air_date?: string;
}

interface TMDBRawCastMember {
  id: number;
  name: string;
  character: string;
  profile_path: string | null;
  order: number;
}

interface TMDBRawCrewMember {
  id: number;
  name: string;
  job: string;
  department: string;
  profile_path: string | null;
}

interface TMDBRawEpisode {
  id: number;
  name: string;
  overview: string;
  episode_number: number;
  season_number: number;
  air_date: string | null;
  still_path: string | null;
  vote_average: number;
  runtime: number | null;
}

interface TMDBRawSeason {
  id: number;
  name: string;
  season_number: number;
  air_date: string | null;
  overview: string;
  poster_path: string | null;
  episodes: TMDBRawEpisode[];
}

interface TMDBPaginatedResponse<T> {
  page: number;
  results: T[];
  total_pages: number;
  total_results: number;
}

// ============================================
// TMDB Client
// ============================================

class TMDBClient {
  private apiKey: string;

  constructor() {
    const apiKey = process.env.TMDB_API_KEY;
    if (!apiKey) {
      throw new Error('TMDB_API_KEY environment variable is not set');
    }
    this.apiKey = apiKey;
  }

  /**
   * Make a request to the TMDB API.
   */
  private async request<T>(
    endpoint: string,
    params: Record<string, string> = {}
  ): Promise<T> {
    const url = new URL(`${TMDB_BASE_URL}${endpoint}`);

    Object.entries(params).forEach(([key, value]) => {
      url.searchParams.set(key, value);
    });

    const response = await fetch(url.toString(), {
      headers: {
        Authorization: `Bearer ${this.apiKey}`,
        'Content-Type': 'application/json',
      },
      next: { revalidate: 300 }, // Cache for 5 minutes
    });

    if (!response.ok) {
      if (response.status === 404) {
        throw new NotFoundError('Resource');
      }
      throw new APIError(
        `TMDB API request failed: ${response.statusText}`,
        response.status
      );
    }

    return response.json() as Promise<T>;
  }

  // ----------------------------------------
  // Transform Functions
  // ----------------------------------------

  private transformMovie(raw: TMDBRawMovie): Movie {
    return {
      id: raw.id,
      title: raw.title,
      overview: raw.overview,
      posterPath: raw.poster_path,
      backdropPath: raw.backdrop_path,
      releaseDate: raw.release_date || '',
      voteAverage: raw.vote_average,
      voteCount: raw.vote_count,
      runtime: raw.runtime,
      genres: raw.genres,
      tagline: raw.tagline,
    };
  }

  private transformTVShow(raw: TMDBRawTVShow): TVShow {
    return {
      id: raw.id,
      name: raw.name,
      overview: raw.overview,
      posterPath: raw.poster_path,
      backdropPath: raw.backdrop_path,
      firstAirDate: raw.first_air_date || '',
      voteAverage: raw.vote_average,
      voteCount: raw.vote_count,
      numberOfSeasons: raw.number_of_seasons,
      numberOfEpisodes: raw.number_of_episodes,
      genres: raw.genres,
      tagline: raw.tagline,
    };
  }

  private transformCast(raw: TMDBRawCastMember[]): CastMember[] {
    return raw.map((member) => ({
      id: member.id,
      name: member.name,
      character: member.character,
      profilePath: member.profile_path,
      order: member.order,
    }));
  }

  private transformCrew(raw: TMDBRawCrewMember[]): CrewMember[] {
    return raw.map((member) => ({
      id: member.id,
      name: member.name,
      job: member.job,
      department: member.department,
      profilePath: member.profile_path,
    }));
  }

  private transformSearchResult(raw: TMDBRawSearchResult): SearchResult | null {
    // Filter out person results
    if (raw.media_type === 'person') {
      return null;
    }

    const base = {
      id: raw.id,
      posterPath: raw.poster_path,
      backdropPath: raw.backdrop_path,
      voteAverage: raw.vote_average,
      overview: raw.overview,
    };

    if (raw.media_type === 'movie') {
      return {
        ...base,
        mediaType: 'movie',
        title: raw.title || '',
        releaseDate: raw.release_date || '',
      } as MovieSearchResult;
    }

    return {
      ...base,
      mediaType: 'tv',
      name: raw.name || '',
      firstAirDate: raw.first_air_date || '',
    } as TVSearchResult;
  }

  // ----------------------------------------
  // Public API Methods
  // ----------------------------------------

  /**
   * Get trending movies and TV shows.
   */
  async getTrending(): Promise<TrendingResults> {
    const [moviesResponse, tvResponse] = await Promise.all([
      this.request<TMDBPaginatedResponse<TMDBRawSearchResult>>(
        `/trending/movie/${TRENDING_TIME_WINDOW}`
      ),
      this.request<TMDBPaginatedResponse<TMDBRawSearchResult>>(
        `/trending/tv/${TRENDING_TIME_WINDOW}`
      ),
    ]);

    const movies: MovieSearchResult[] = moviesResponse.results
      .slice(0, SEARCH_MAX_RESULTS)
      .map((item) => ({
        id: item.id,
        mediaType: 'movie' as const,
        posterPath: item.poster_path,
        backdropPath: item.backdrop_path,
        voteAverage: item.vote_average,
        overview: item.overview,
        title: item.title || '',
        releaseDate: item.release_date || '',
      }));

    const tvShows: TVSearchResult[] = tvResponse.results
      .slice(0, SEARCH_MAX_RESULTS)
      .map((item) => ({
        id: item.id,
        mediaType: 'tv' as const,
        posterPath: item.poster_path,
        backdropPath: item.backdrop_path,
        voteAverage: item.vote_average,
        overview: item.overview,
        name: item.name || '',
        firstAirDate: item.first_air_date || '',
      }));

    return { movies, tvShows };
  }

  /**
   * Search for movies and TV shows.
   */
  async search(query: string): Promise<SearchResult[]> {
    if (!query.trim()) {
      return [];
    }

    const response = await this.request<TMDBPaginatedResponse<TMDBRawSearchResult>>(
      '/search/multi',
      { query: query.trim() }
    );

    return response.results
      .map((item) => this.transformSearchResult(item))
      .filter((item): item is SearchResult => item !== null)
      .slice(0, SEARCH_MAX_RESULTS);
  }

  /**
   * Get movie details with credits.
   */
  async getMovie(id: number): Promise<MovieDetails> {
    const response = await this.request<
      TMDBRawMovie & {
        credits: { cast: TMDBRawCastMember[]; crew: TMDBRawCrewMember[] };
      }
    >(`/movie/${id}`, { append_to_response: 'credits' });

    return {
      ...this.transformMovie(response),
      credits: {
        cast: this.transformCast(response.credits.cast),
        crew: this.transformCrew(response.credits.crew),
      },
    };
  }

  /**
   * Get TV show details with credits.
   */
  async getTVShow(id: number): Promise<TVShowDetails> {
    const response = await this.request<
      TMDBRawTVShow & {
        credits: { cast: TMDBRawCastMember[]; crew: TMDBRawCrewMember[] };
      }
    >(`/tv/${id}`, { append_to_response: 'credits' });

    return {
      ...this.transformTVShow(response),
      credits: {
        cast: this.transformCast(response.credits.cast),
        crew: this.transformCrew(response.credits.crew),
      },
    };
  }

  /**
   * Get season details with episodes.
   */
  async getSeason(showId: number, seasonNumber: number): Promise<SeasonDetails> {
    const response = await this.request<TMDBRawSeason>(
      `/tv/${showId}/season/${seasonNumber}`
    );

    return {
      id: response.id,
      name: response.name,
      seasonNumber: response.season_number,
      airDate: response.air_date,
      overview: response.overview,
      posterPath: response.poster_path,
      episodes: response.episodes.map((ep) => ({
        id: ep.id,
        name: ep.name,
        overview: ep.overview,
        episodeNumber: ep.episode_number,
        seasonNumber: ep.season_number,
        airDate: ep.air_date,
        stillPath: ep.still_path,
        voteAverage: ep.vote_average,
        runtime: ep.runtime,
      })),
    };
  }
}

// Export singleton instance
export const tmdbClient = new TMDBClient();

// Export class for testing
export { TMDBClient };
