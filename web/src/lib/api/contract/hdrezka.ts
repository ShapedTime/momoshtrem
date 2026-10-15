import type { MediaType } from './common';
import type { EpisodeStream, FileSource } from './titles';

/**
 * HDRezka as a second source (PLAN §6 "HDRezka (#63)"): find the title on
 * HDRezka, pick a voice-over (translator) and a quality, and a movie or an
 * episode plays that stream in Infuse instead of a torrent file. Opt-in on the
 * backend (`status.hdrezka.configured`). Errors are `404 not_found`,
 * `409 premium_only`, `422 validation_error`, `502 hdrezka_unavailable`,
 * `503 hdrezka_blocked` (the anti-bot page) or `unavailable` (off), and
 * `504 timeout`; never 401 or 403.
 */

/** How well a hit matches the TMDB title: exact (same title, year ±1, kind), likely, or weak. */
export type HDRezkaMatch = 'exact' | 'likely' | 'weak';
export type HDRezkaKind = 'movie' | 'series';
/** The first segment of the title's path. */
export type HDRezkaCategory = 'films' | 'series' | 'cartoons' | 'animation';

export type HDRezkaErrorCode =
  | 'not_found'
  | 'premium_only'
  | 'validation_error'
  | 'hdrezka_unavailable'
  | 'hdrezka_blocked'
  | 'unavailable'
  | 'timeout';

/** One search hit, best first. */
export interface HDRezkaCandidate {
  /** The title page on any mirror; send it back as is. */
  url: string;
  /** "/films/fiction/2259-interstellar-2014.html" */
  path: string;
  /** The site's (Russian) title. */
  title: string;
  /** Null when the site gives none (Russian titles, full-search hits). */
  original_title: string | null;
  /** The first year ("2008" for 2008–2013); null when unknown. */
  year: number | null;
  /** Null when the category doesn't tell (cartoons, anime). */
  kind: HDRezkaKind | null;
  category: HDRezkaCategory;
  /** Kinopoisk; null when the hit has none. */
  rating: number | null;
  match: HDRezkaMatch;
}

/** GET /hdrezka/search?media_type=&tmdb_id=[&q=]. Without `q` the backend searches by the TMDB title. */
export interface HDRezkaSearchParams {
  media_type: MediaType;
  tmdb_id: number;
  q?: string;
}

export interface HDRezkaSearchResult {
  items: HDRezkaCandidate[];
  /** What was searched. */
  query: string;
}

/**
 * One translation: a dub, a voice-over or the original audio ("Оригинал
 * (+субтитры)"). Names are the site's own; one id can come in several cuts,
 * told apart by the flags.
 */
export interface HDRezkaTranslator {
  id: number;
  name: string;
  /** The one the page plays first. */
  default: boolean;
  /** Needs an HDRezka Premium account, which momoshtrem doesn't have. */
  premium: boolean;
  camrip: boolean;
  /** Ads burned into the video. */
  ads: boolean;
  /** Director's cut. */
  director: boolean;
}

/** The library episode an HDRezka episode maps to, with its file's source. */
export interface HDRezkaLibraryEpisode {
  episode_id: number;
  season: number;
  episode: number;
  name: string;
  air_date: string | null;
  has_file: boolean;
  source: FileSource | null;
  stream: EpisodeStream | null;
}

export interface HDRezkaEpisode {
  /** HDRezka's numbers. */
  number: number;
  /** Null when it maps to no library episode (or no `show_id` was sent). */
  library: HDRezkaLibraryEpisode | null;
}

export interface HDRezkaSeason {
  number: number;
  episodes: HDRezkaEpisode[];
}

/** GET /hdrezka/titles?url=[&translator_id=][&show_id=|&movie_id=] */
export interface HDRezkaTitleParams {
  url: string;
  /** Series: the translator whose episodes to list. */
  translator_id?: number;
  /** Library ids: annotate episodes with the library's (shows). */
  show_id?: number;
  movie_id?: number;
}

export interface HDRezkaTitle {
  url: string;
  path: string;
  post_id: number;
  kind: HDRezkaKind;
  title: string;
  original_title: string | null;
  year: number | null;
  translators: HDRezkaTranslator[];
  /** The translator the answer is for (the requested one, else the default). */
  translator_id: number;
  /** Empty for a movie. */
  seasons: HDRezkaSeason[];
}

/** GET /hdrezka/streams?url=&translator_id=[&season=&episode=] */
export interface HDRezkaStreamsParams {
  url: string;
  translator_id: number;
  season?: number;
  episode?: number;
}

export interface HDRezkaQuality {
  /** The site's label, sent back as `quality`: "1080p", "1080p Ultra", "4K". */
  label: string;
  /** Lines (1080; 2K is 1440, 4K 2160); 0 when unknown. */
  resolution: number;
  /** Needs Premium: the CDN serves a teaser without it. */
  premium: boolean;
  /** There's a progressive MP4 to stream (some qualities are HLS only). */
  available: boolean;
  /** Measured from the CDN; null when it couldn't be. */
  size_bytes: number | null;
}

export interface HDRezkaSubtitle {
  /** ISO 639-1; "" when the site gives none. */
  language_code: string;
  /** The site's name for it: "English", "Русский". */
  language_name: string;
}

export interface HDRezkaStreams {
  qualities: HDRezkaQuality[];
  subtitles: HDRezkaSubtitle[];
}

/** POST /movies/{id}/assign-hdrezka → 204 */
export interface AssignMovieStreamRequest {
  url: string;
  translator_id: number;
  /** A quality's `label`. */
  quality: string;
}

/** One episode in HDRezka's numbers, and the library episode it fills. */
export interface StreamEpisodeTarget {
  episode_id: number;
  season: number;
  episode: number;
}

/** POST /shows/{id}/assign-hdrezka: 1–30 items a request (the web sends chunks). */
export interface AssignShowStreamRequest extends AssignMovieStreamRequest {
  items: StreamEpisodeTarget[];
}

export interface AssignShowStreamResult {
  assigned: Array<{ episode_id: number; quality: string; size_bytes: number }>;
  failed: Array<{ episode_id: number; code: string; message: string }>;
}
