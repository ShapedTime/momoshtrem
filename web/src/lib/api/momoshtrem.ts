import { MOMOSHTREM_CONFIG } from '@/config/momoshtrem';
import { APIError, ValidationError } from '@/lib/errors';
import type { LibraryMovie, LibraryShow } from '@/types/momoshtrem';
import { parseMomoshtremError } from '@/types/momoshtrem';

/** Header carrying the shared secret (BACKEND_API_TOKEN) to momoshtrem. */
export const TOKEN_HEADER = 'X-Momoshtrem-Token';

/** Error code for a backend that rejected the web app's API token. */
export const BACKEND_AUTH_CODE = 'backend_auth';

export const BACKEND_AUTH_MESSAGE =
  "momoshtrem rejected the web app's API token. Check BACKEND_API_TOKEN on both services.";

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
   * Request headers, including the shared secret when configured.
   */
  private buildHeaders(): Record<string, string> {
    const headers: Record<string, string> = {
      'Content-Type': 'application/json',
    };
    // Read at call time (not module load) so runtime env changes apply.
    const token = process.env.BACKEND_API_TOKEN;
    if (token) {
      headers[TOKEN_HEADER] = token;
    }
    return headers;
  }

  /**
   * Handle API errors consistently.
   * A backend 401/403 means the web app's token was rejected, not that the
   * user's session expired, so it becomes a 502: passing the 401 through
   * would send the browser to the login page in a loop.
   */
  private async handleError(response: Response, operation: string): Promise<never> {
    if (response.status === 401 || response.status === 403) {
      throw new APIError(BACKEND_AUTH_MESSAGE, 502, BACKEND_AUTH_CODE);
    }

    const body: unknown = await response.json().catch(() => null);
    const parsed = parseMomoshtremError(body);
    throw new APIError(
      parsed?.message || `Failed to ${operation}: ${response.statusText}`,
      response.status,
      parsed?.code
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
        headers: this.buildHeaders(),
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
}

// Export singleton instance
export const momoshtremClient = new MomoshtremClient();

// Export class for testing
export { MomoshtremClient };
