// Search result from OpenSubtitles API
export interface SubtitleSearchResult {
  file_id: number;
  language_code: string;
  language_name: string;
  release_name: string;
  download_count: number;
  file_name: string;
  ratings: number;
}

// Stored subtitle in our system
export interface Subtitle {
  id: number;
  language_code: string;
  language_name: string;
  format: string; // srt or vtt
  file_size: number;
  created_at: string;
}

export interface SubtitleSearchContext {
  mediaType: 'movie' | 'episode';
  tmdbId: number;
  title: string;
  itemId: number; // Library item ID
  season?: number;
  episode?: number;
}

export interface SubtitleSearchResponse {
  results: SubtitleSearchResult[];
}

export interface SubtitleListResponse {
  subtitles: Subtitle[];
}

export interface DownloadSubtitleRequest {
  item_type: 'movie' | 'episode';
  item_id: number;
  file_id: number;
  language_code: string;
  language_name: string;
}

export interface DownloadSubtitleResponse {
  success: boolean;
  subtitle: Subtitle;
}

// Supported subtitle languages
export const SUBTITLE_LANGUAGES = [
  { code: 'en', name: 'English' },
  { code: 'ru', name: 'Russian' },
  { code: 'tr', name: 'Turkish' },
  { code: 'az', name: 'Azerbaijani' },
] as const;

export type SubtitleLanguageCode = (typeof SUBTITLE_LANGUAGES)[number]['code'];
