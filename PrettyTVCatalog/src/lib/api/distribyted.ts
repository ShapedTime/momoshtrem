import { DISTRIBYTED_CONFIG, DISTRIBYTED_ENDPOINTS } from '@/config/distribyted';
import { APIError, ValidationError } from '@/lib/errors';
import type { AddTorrentRequest, DistribytedError } from '@/types/distribyted';

/**
 * Distribyted API client.
 * Handles communication with the distribyted torrent streaming service.
 */
class DistribytedClient {
  // ----------------------------------------
  // Private Helpers
  // ----------------------------------------

  /**
   * Build full URL for an endpoint.
   */
  private buildUrl(path: string): string {
    return new URL(path, DISTRIBYTED_CONFIG.baseUrl).toString();
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
      DISTRIBYTED_CONFIG.timeout
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

  // ----------------------------------------
  // Public API Methods
  // ----------------------------------------

  /**
   * Add a torrent to distribyted via magnet URI.
   * @param magnetUri - Valid magnet URI starting with "magnet:?"
   * @param route - Route name (defaults to config value)
   * @throws ValidationError if magnet URI is invalid
   * @throws APIError if request fails
   */
  async addTorrent(
    magnetUri: string,
    route: string = DISTRIBYTED_CONFIG.defaultRoute
  ): Promise<void> {
    // Validate magnet URI
    if (!magnetUri || !magnetUri.startsWith('magnet:?')) {
      throw new ValidationError('Invalid magnet URI');
    }

    const url = this.buildUrl(DISTRIBYTED_ENDPOINTS.addTorrent(route));
    const body: AddTorrentRequest = { magnet: magnetUri };

    try {
      const response = await this.fetchWithTimeout(url, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify(body),
      });

      if (!response.ok) {
        const data = (await response.json().catch(() => ({}))) as DistribytedError;
        throw new APIError(
          data.error || `Failed to add torrent: ${response.statusText}`,
          response.status
        );
      }
    } catch (error) {
      if (error instanceof Error && error.name === 'AbortError') {
        throw new APIError('Request timed out', 504);
      }
      if (error instanceof APIError) throw error;
      if (error instanceof ValidationError) throw error;

      throw new APIError(
        `Failed to add torrent: ${error instanceof Error ? error.message : 'Unknown error'}`,
        500
      );
    }
  }
}

// Export singleton instance
export const distribytedClient = new DistribytedClient();

// Export class for testing
export { DistribytedClient };
