import type { MediaType } from './common';

/**
 * Discover (#100). **Web-side, not PLAN §6:** TMDB browsing stays in the web
 * app, so `GET /api/discover` is the web route `app/api/discover/route.ts`
 * (not proxied to the backend).
 *
 * Query: the Discover page's own URL keys (lib/discover/url.ts) without
 * `mood`, plus `page` (1–500, default 1) and `relax=1`.
 * - `type=all` merges the movie and TV streams: the same page of both,
 *   interleaved by the sort key. `total` is their sum and `total_pages` the
 *   larger, so a page holds up to 40 titles and the order across pages is
 *   approximate. A filter one type can't match (a genre TMDB's TV list
 *   lacks, a show status) skips that type's stream.
 * - TMDB reports at most about 20,000 results per type (20,001 when there
 *   are more), so a `total` from 20,000 up is a floor ("20,000+" in the UI).
 *   It serves 500 pages at most, so `total_pages` is never over 500.
 * - TV leaves out News, Reality, Soap and Talk (tmdb-query.ts
 *   `TV_EXCLUDED_GENRES`).
 *
 * Errors are `{error: {code, message}}`: 400 `bad_request` (a bad page),
 * 401 `unauthorized`, 503 `tmdb_not_configured` (no TMDB_API_KEY),
 * 502 `tmdb_auth` (TMDB rejected the key) and 502 `tmdb_unavailable`.
 */

export interface DiscoverItem {
  media_type: MediaType;
  tmdb_id: number;
  title: string;
  /** Release (movies) or first-air (shows) year. */
  year: number | null;
  /** TMDB poster path ("/abc.jpg"), or null. */
  poster_path: string | null;
}

export interface DiscoverPage {
  items: DiscoverItem[];
  total: number;
  page: number;
  total_pages: number;
}

/**
 * `relax=1`: for each active filter, how many titles there'd be without it
 * (page 1 only, all computed in one call). Only the empty state asks.
 */
export interface DiscoverRelaxed {
  relaxed: DiscoverRelaxedFilter[];
}

export interface DiscoverRelaxedFilter {
  /** `genre:<slug>`, `anime`, `year`, `rating`, `runtime`, `lang`, `country` or `status`. */
  filter: string;
  would_be: number;
}
