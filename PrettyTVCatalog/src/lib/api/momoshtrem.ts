import { MOMOSHTREM_CONFIG } from '@/config/momoshtrem';
import { APIError, ValidationError } from '@/lib/errors';
import type {
  LibraryMovie,
  LibraryShow,
  MovieAssignmentResponse,
  ShowAssignmentResponse,
  MomoshtremError,
  RecentlyAiredResponse,
} from '@/types/momoshtrem';
import type { TorrentStatus, TorrentListResponse } from '@/types/torrent';
import type {
  SubtitleSearchResult,
  Subtitle,
  DownloadSubtitleRequest,
} from '@/types/subtitle';

/**
 * momoshtrem API client.
 * Handles communication with the momoshtrem library-first streaming service.
 */
class MomoshtremClient {
  // ============================================================================
  // Private Helpers
  // ============================================================================

  /**
   * Build full URL for an endpoint.
   */
  private buildUrl(path: string): string {
    return new URL(path, MOMOSHTREM_CONFIG.baseUrl).toString();
  }

  /**
   * Execute fetch with timeout.
   */
  private async fetchWithTimeout(
    url: string,
    options: RequestInit
  ): Promise<Response> {
    const controller = new AbortController();
    const timeoutId = setTimeout(
      () => controller.abort(),
      MOMOSHTREM_CONFIG.timeout
    );

    try {
      const response = await fetch(url, {
        ...options,
        signal: controller.signal,
      });
      return response;
    } finally {
      clearTimeout(timeoutId);
    }
  }

  /**
   * Handle API errors consistently.
   */
  private async handleError(response: Response, operation: string): Promise<never> {
    const data = (await response.json().catch(() => ({}))) as MomoshtremError;
    throw new APIError(
      data.error || `Failed to ${operation}: ${response.statusText}`,
      response.status
    );
  }

  /**
   * Generic request method with error handling.
   */
  private async request<T>(
    method: string,
    path: string,
    body?: unknown,
    operation: string = 'perform operation'
  ): Promise<T> {
    const url = this.buildUrl(path);

    try {
      const response = await this.fetchWithTimeout(url, {
        method,
        headers: {
          'Content-Type': 'application/json',
        },
        body: body ? JSON.stringify(body) : undefined,
      });

      if (!response.ok) {
        await this.handleError(response, operation);
      }

      // Handle 204 No Content
      if (response.status === 204) {
        return undefined as T;
      }

      return (await response.json()) as T;
    } catch (error) {
      if (error instanceof Error && error.name === 'AbortError') {
        throw new APIError('Request timed out', 504);
      }
      if (error instanceof APIError) throw error;
      if (error instanceof ValidationError) throw error;

      throw new APIError(
        `Failed to ${operation}: ${error instanceof Error ? error.message : 'Unknown error'}`,
        500
      );
    }
  }

  // ============================================================================
  // Movies API
  // ============================================================================

  /**
   * Add a movie to the library by TMDB ID.
   */
  async addMovie(tmdbId: number): Promise<LibraryMovie> {
    return this.request<LibraryMovie>(
      'POST',
      '/api/movies',
      { tmdb_id: tmdbId },
      'add movie to library'
    );
  }

  /**
   * Get all movies in the library.
   */
  async getMovies(): Promise<LibraryMovie[]> {
    const result = await this.request<LibraryMovie[]>(
      'GET',
      '/api/movies',
      undefined,
      'get movies'
    );
    return result || [];
  }

  /**
   * Get a single movie by library ID.
   */
  async getMovie(id: number): Promise<LibraryMovie> {
    return this.request<LibraryMovie>(
      'GET',
      `/api/movies/${id}`,
      undefined,
      'get movie'
    );
  }

  /**
   * Delete a movie from the library.
   */
  async deleteMovie(id: number): Promise<void> {
    await this.request<void>(
      'DELETE',
      `/api/movies/${id}`,
      undefined,
      'delete movie'
    );
  }

  /**
   * Assign a torrent to a movie (auto-detects best file).
   */
  async assignMovieTorrent(
    id: number,
    magnetUri: string
  ): Promise<MovieAssignmentResponse> {
    this.validateMagnetUri(magnetUri);

    return this.request<MovieAssignmentResponse>(
      'POST',
      `/api/movies/${id}/assign-torrent`,
      { magnet_uri: magnetUri },
      'assign torrent to movie'
    );
  }

  /**
   * Unassign torrent from a movie.
   */
  async unassignMovie(id: number): Promise<void> {
    await this.request<void>(
      'DELETE',
      `/api/movies/${id}/assign`,
      undefined,
      'unassign movie torrent'
    );
  }

  // ============================================================================
  // Shows API
  // ============================================================================

  /**
   * Add a show to the library by TMDB ID.
   * This automatically fetches all seasons and episodes from TMDB.
   */
  async addShow(tmdbId: number): Promise<LibraryShow> {
    return this.request<LibraryShow>(
      'POST',
      '/api/shows',
      { tmdb_id: tmdbId },
      'add show to library'
    );
  }

  /**
   * Get all shows in the library.
   */
  async getShows(): Promise<LibraryShow[]> {
    const result = await this.request<LibraryShow[]>(
      'GET',
      '/api/shows',
      undefined,
      'get shows'
    );
    return result || [];
  }

  /**
   * Get a single show with all seasons and episodes.
   */
  async getShow(id: number): Promise<LibraryShow> {
    return this.request<LibraryShow>(
      'GET',
      `/api/shows/${id}`,
      undefined,
      'get show'
    );
  }

  /**
   * Delete a show from the library.
   */
  async deleteShow(id: number): Promise<void> {
    await this.request<void>(
      'DELETE',
      `/api/shows/${id}`,
      undefined,
      'delete show'
    );
  }

  /**
   * Assign a torrent to a show (auto-matches episodes by filename).
   * Returns detailed results of which episodes were matched.
   */
  async assignShowTorrent(
    id: number,
    magnetUri: string
  ): Promise<ShowAssignmentResponse> {
    this.validateMagnetUri(magnetUri);

    return this.request<ShowAssignmentResponse>(
      'POST',
      `/api/shows/${id}/assign-torrent`,
      { magnet_uri: magnetUri },
      'assign torrent to show'
    );
  }

  // ============================================================================
  // Library Lookup Methods
  // ============================================================================

  /**
   * Find a movie in the library by TMDB ID.
   * Returns null if not found.
   */
  async findMovieByTmdbId(tmdbId: number): Promise<LibraryMovie | null> {
    const movies = await this.getMovies();
    return movies.find((m) => m.tmdb_id === tmdbId) || null;
  }

  /**
   * Find a show in the library by TMDB ID.
   * Returns null if not found.
   * Fetches full show data including seasons and episodes.
   */
  async findShowByTmdbId(tmdbId: number): Promise<LibraryShow | null> {
    const shows = await this.getShows();
    const show = shows.find((s) => s.tmdb_id === tmdbId);
    if (!show) return null;
    // Fetch full show data including seasons
    return this.getShow(show.id);
  }

  /**
   * Get combined library (movies and shows).
   */
  async getLibrary(): Promise<{ movies: LibraryMovie[]; shows: LibraryShow[] }> {
    const [movies, shows] = await Promise.all([
      this.getMovies(),
      this.getShows(),
    ]);
    return { movies, shows };
  }

  // ============================================================================
  // Combined Add & Assign Flow
  // ============================================================================

  /**
   * Add a movie torrent with combined flow:
   * 1. Check if movie exists in library
   * 2. Add to library if not
   * 3. Assign the torrent
   *
   * @returns Object with assignment result and whether item was newly added
   */
  async addMovieTorrent(
    tmdbId: number,
    magnetUri: string
  ): Promise<{
    addedToLibrary: boolean;
    libraryId: number;
    assignment: MovieAssignmentResponse;
  }> {
    this.validateMagnetUri(magnetUri);

    // Check if already in library
    let movie = await this.findMovieByTmdbId(tmdbId);
    const addedToLibrary = !movie;

    // Add to library if needed
    if (!movie) {
      movie = await this.addMovie(tmdbId);
    }

    // Assign torrent
    const assignment = await this.assignMovieTorrent(movie.id, magnetUri);

    return {
      addedToLibrary,
      libraryId: movie.id,
      assignment,
    };
  }

  /**
   * Add a show torrent with combined flow:
   * 1. Check if show exists in library
   * 2. Add to library if not (creates all episodes from TMDB)
   * 3. Assign the torrent (auto-matches episodes)
   *
   * @returns Object with assignment result and whether item was newly added
   */
  async addShowTorrent(
    tmdbId: number,
    magnetUri: string
  ): Promise<{
    addedToLibrary: boolean;
    libraryId: number;
    assignment: ShowAssignmentResponse;
  }> {
    this.validateMagnetUri(magnetUri);

    // Check if already in library
    let show = await this.findShowByTmdbId(tmdbId);
    const addedToLibrary = !show;

    // Add to library if needed
    if (!show) {
      show = await this.addShow(tmdbId);
    }

    // Assign torrent
    const assignment = await this.assignShowTorrent(show.id, magnetUri);

    return {
      addedToLibrary,
      libraryId: show.id,
      assignment,
    };
  }

  // ============================================================================
  // Validation
  // ============================================================================

  /**
   * Validate magnet URI format.
   */
  private validateMagnetUri(magnetUri: string): void {
    if (!magnetUri || !magnetUri.startsWith('magnet:?')) {
      throw new ValidationError('Invalid magnet URI');
    }
  }

  // ============================================================================
  // Torrent Management API
  // ============================================================================

  /**
   * Get all active torrents with their status.
   */
  async getTorrents(): Promise<TorrentStatus[]> {
    const result = await this.request<TorrentListResponse>(
      'GET',
      '/api/torrents',
      undefined,
      'get torrents'
    );
    return result?.torrents || [];
  }

  /**
   * Get detailed status for a specific torrent.
   */
  async getTorrentStatus(infoHash: string): Promise<TorrentStatus> {
    return this.request<TorrentStatus>(
      'GET',
      `/api/torrents/${infoHash}`,
      undefined,
      'get torrent status'
    );
  }

  /**
   * Remove a torrent.
   * @param infoHash - The torrent info hash
   * @param deleteData - If true, also deletes downloaded data
   */
  async removeTorrent(infoHash: string, deleteData = false): Promise<void> {
    const query = deleteData ? '?delete_data=true' : '';
    await this.request<void>(
      'DELETE',
      `/api/torrents/${infoHash}${query}`,
      undefined,
      'remove torrent'
    );
  }

  /**
   * Pause a torrent.
   */
  async pauseTorrent(infoHash: string): Promise<void> {
    await this.request<void>(
      'POST',
      `/api/torrents/${infoHash}/pause`,
      undefined,
      'pause torrent'
    );
  }

  /**
   * Resume a paused torrent.
   */
  async resumeTorrent(infoHash: string): Promise<void> {
    await this.request<void>(
      'POST',
      `/api/torrents/${infoHash}/resume`,
      undefined,
      'resume torrent'
    );
  }

  /**
   * Unassign torrent from a specific episode.
   */
  async unassignEpisodeTorrent(episodeId: number): Promise<void> {
    await this.request<void>(
      'DELETE',
      `/api/episodes/${episodeId}/assign`,
      undefined,
      'unassign episode torrent'
    );
  }

  // ============================================================================
  // Subtitles API
  // ============================================================================

  /**
   * Search for subtitles on OpenSubtitles.
   */
  async searchSubtitles(
    tmdbId: number,
    type: 'movie' | 'episode',
    languages: string[],
    season?: number,
    episode?: number
  ): Promise<SubtitleSearchResult[]> {
    const params = new URLSearchParams({
      tmdb_id: tmdbId.toString(),
      type,
      languages: languages.join(','),
    });

    if (type === 'episode') {
      if (season) params.set('season', season.toString());
      if (episode) params.set('episode', episode.toString());
    }

    const result = await this.request<{ results: SubtitleSearchResult[] }>(
      'GET',
      `/api/subtitles/search?${params}`,
      undefined,
      'search subtitles'
    );
    return result.results || [];
  }

  /**
   * Download and save a subtitle from OpenSubtitles.
   */
  async downloadSubtitle(params: DownloadSubtitleRequest): Promise<Subtitle> {
    const result = await this.request<{ subtitle: Subtitle }>(
      'POST',
      '/api/subtitles/download',
      params,
      'download subtitle'
    );
    return result.subtitle;
  }

  /**
   * Get all subtitles for a movie.
   */
  async getMovieSubtitles(movieId: number): Promise<Subtitle[]> {
    const result = await this.request<{ subtitles: Subtitle[] }>(
      'GET',
      `/api/movies/${movieId}/subtitles`,
      undefined,
      'get movie subtitles'
    );
    return result.subtitles || [];
  }

  /**
   * Get all subtitles for an episode.
   */
  async getEpisodeSubtitles(episodeId: number): Promise<Subtitle[]> {
    const result = await this.request<{ subtitles: Subtitle[] }>(
      'GET',
      `/api/episodes/${episodeId}/subtitles`,
      undefined,
      'get episode subtitles'
    );
    return result.subtitles || [];
  }

  /**
   * Delete a subtitle.
   */
  async deleteSubtitle(subtitleId: number): Promise<void> {
    await this.request<void>(
      'DELETE',
      `/api/subtitles/${subtitleId}`,
      undefined,
      'delete subtitle'
    );
  }

  // ============================================================================
  // Recently Aired API
  // ============================================================================

  /**
   * Get recently aired episodes from library shows.
   * @param lookbackDays Number of days to look back (default: 30, max: 90)
   */
  async getRecentlyAiredEpisodes(lookbackDays = 30): Promise<RecentlyAiredResponse> {
    return this.request<RecentlyAiredResponse>(
      'GET',
      `/api/shows/recently-aired?lookback_days=${lookbackDays}`,
      undefined,
      'get recently aired episodes'
    );
  }

  /**
   * Trigger a manual air date sync from TMDB.
   * Returns immediately; sync runs in background.
   */
  async triggerAirDateSync(): Promise<void> {
    await this.request<void>(
      'POST',
      '/api/shows/sync-air-dates',
      undefined,
      'trigger air date sync'
    );
  }
}

// Export singleton instance
export const momoshtremClient = new MomoshtremClient();

// Export class for testing
export { MomoshtremClient };
