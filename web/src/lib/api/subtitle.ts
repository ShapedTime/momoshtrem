import type {
  SubtitleSearchResult,
  Subtitle,
  DownloadSubtitleRequest,
} from '@/types/subtitle';

// Type guards for API responses
function isSubtitleSearchResult(obj: unknown): obj is SubtitleSearchResult {
  return (
    typeof obj === 'object' &&
    obj !== null &&
    'file_id' in obj &&
    typeof (obj as SubtitleSearchResult).file_id === 'number' &&
    'language_code' in obj &&
    typeof (obj as SubtitleSearchResult).language_code === 'string'
  );
}

function isSubtitle(obj: unknown): obj is Subtitle {
  return (
    typeof obj === 'object' &&
    obj !== null &&
    'id' in obj &&
    typeof (obj as Subtitle).id === 'number' &&
    'language_code' in obj &&
    typeof (obj as Subtitle).language_code === 'string'
  );
}

class SubtitleClient {
  private baseUrl: string;

  constructor() {
    this.baseUrl = '/api/subtitles';
  }

  /**
   * Search for subtitles on OpenSubtitles
   */
  async search(
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

    const response = await fetch(`${this.baseUrl}/search?${params}`);
    if (!response.ok) {
      const error = await response.json().catch(() => ({ error: 'Search failed' }));
      throw new Error(error.error || 'Subtitle search failed');
    }

    const data = await response.json();
    const results = data?.results;
    if (!Array.isArray(results)) {
      return [];
    }
    return results.filter(isSubtitleSearchResult);
  }

  /**
   * Download a subtitle from OpenSubtitles and save to library
   */
  async download(params: DownloadSubtitleRequest): Promise<Subtitle> {
    const response = await fetch(`${this.baseUrl}/download`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(params),
    });

    if (!response.ok) {
      const error = await response.json().catch(() => ({ error: 'Download failed' }));
      throw new Error(error.error || 'Subtitle download failed');
    }

    const data = await response.json();
    const subtitle = data?.subtitle;
    if (!isSubtitle(subtitle)) {
      throw new Error('Invalid subtitle response from server');
    }
    return subtitle;
  }

  /**
   * Get all subtitles for a movie
   */
  async getForMovie(movieId: number): Promise<Subtitle[]> {
    const response = await fetch(`/api/movies/${movieId}/subtitles`);
    if (!response.ok) {
      const error = await response.json().catch(() => ({ error: 'Failed to fetch subtitles' }));
      throw new Error(error.error || 'Failed to fetch movie subtitles');
    }

    const data = await response.json();
    const subtitles = data?.subtitles;
    if (!Array.isArray(subtitles)) {
      return [];
    }
    return subtitles.filter(isSubtitle);
  }

  /**
   * Get all subtitles for an episode
   */
  async getForEpisode(episodeId: number): Promise<Subtitle[]> {
    const response = await fetch(`/api/episodes/${episodeId}/subtitles`);
    if (!response.ok) {
      const error = await response.json().catch(() => ({ error: 'Failed to fetch subtitles' }));
      throw new Error(error.error || 'Failed to fetch episode subtitles');
    }

    const data = await response.json();
    const subtitles = data?.subtitles;
    if (!Array.isArray(subtitles)) {
      return [];
    }
    return subtitles.filter(isSubtitle);
  }

  /**
   * Delete a subtitle
   */
  async delete(subtitleId: number): Promise<void> {
    const response = await fetch(`${this.baseUrl}/${subtitleId}`, {
      method: 'DELETE',
    });

    if (!response.ok && response.status !== 204) {
      const error = await response.json().catch(() => ({ error: 'Delete failed' }));
      throw new Error(error.error || 'Failed to delete subtitle');
    }
  }
}

export const subtitleClient = new SubtitleClient();
