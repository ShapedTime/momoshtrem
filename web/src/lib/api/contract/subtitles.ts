import type { Timestamp } from './common';

/** Subtitles (B7; #4), item-scoped under /movies/{id} and /episodes/{id}. */

export type SubtitleItemType = 'movie' | 'episode';
export type SubtitleSource = 'opensubtitles' | 'torrent';
/** exact_release: score ≥ 90; same_source: ≥ 60; different_timing: lower; unknown: a release name is missing. */
export type SubtitleMatch = 'exact_release' | 'same_source' | 'different_timing' | 'unknown';

/** GET …/subtitles (items); POST …/subtitles answers one. */
export interface Subtitle {
  id: number;
  language_code: string;
  language_name: string;
  /** e.g. "srt", "ass". */
  format: string;
  /** Torrent subtitles can't be deleted (409). */
  source: SubtitleSource;
  release_name: string | null;
  /** Against the current release; torrent subtitles are always exact_release. */
  match: SubtitleMatch;
  created_at: Timestamp;
}

/** One OpenSubtitles result, ranked against the assigned release. */
export interface SubtitleCandidate {
  file_id: number;
  language_code: string;
  language_name: string;
  release_name: string;
  file_name: string;
  downloads: number;
  hearing_impaired: boolean;
  machine_translated: boolean;
  match: SubtitleMatch;
  /** 0–100. */
  match_score: number;
  /** One line, e.g. "Same release group (FLUX) and source (WEB-DL)". */
  match_reason: string;
}

/** GET …/subtitles/search?languages=en,az */
export interface SubtitleSearchResult {
  items: SubtitleCandidate[];
  /** The release the candidates were ranked against; null when nothing is assigned. */
  release_name: string | null;
}

/** POST …/subtitles. Replaces the downloaded subtitle in that language. */
export interface DownloadSubtitleRequest {
  file_id: number;
  language_code: string;
  /** The candidate's release name, kept so the match can be shown later. */
  release_name?: string;
}
