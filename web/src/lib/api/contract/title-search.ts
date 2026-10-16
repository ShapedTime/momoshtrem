import type { MediaType } from './common';

/**
 * Title search (#101). **Web-side, not PLAN §6:** TMDB browsing stays in the
 * web app, so `GET /api/title-search?q=` is the web route
 * `app/api/title-search/route.ts` (not proxied to the backend).
 *
 * One query asks TMDB three times (search movie, search tv, search person),
 * page 1 each, and ranks the merged titles (lib/title-search/rank.ts):
 * exact title, prefix, every word, an original or alternative title, then
 * the titles a matching person is known for, with a little for the rating.
 * Each title appears once. The search page and the palette split off the
 * titles you already have (`/library?q=` and `/library?ids=`), so library
 * titles always come first, as their own group.
 *
 * Query: `q`, 1–100 characters after trimming.
 *
 * Errors are `{error: {code, message}}`: 400 `bad_request` (no `q`, or too
 * long), 401 `unauthorized`, 503 `tmdb_not_configured` (no TMDB_API_KEY),
 * 502 `tmdb_auth` (TMDB rejected the key) and 502 `tmdb_unavailable`
 * (TMDB failed or took over 10 s).
 */

/**
 * Why a title matched, best first: its title equals the query, starts with
 * it, or holds every word; its original title holds every word (or TMDB
 * matched an alternative title or translation, which the route can't see);
 * or a person the query names is known for it.
 */
export type TitleMatch = 'exact' | 'prefix' | 'words' | 'original' | 'person';

export interface TitleSearchItem {
  media_type: MediaType;
  tmdb_id: number;
  title: string;
  /** TMDB's original title, when it differs from `title`. */
  original_title: string | null;
  /** Release (movies) or first-air (shows) year. */
  year: number | null;
  /** TMDB poster path ("/abc.jpg"), or null. */
  poster_path: string | null;
  match: TitleMatch;
  /**
   * "Original title match", "Directed by Denis Villeneuve", "Created by Vince
   * Gilligan", "Written by …", "With Zendaya"; null for a match on the title itself.
   */
  match_reason: string | null;
}

export interface TitleSearchCounts {
  all: number;
  movie: number;
  tv: number;
}

/** GET /api/title-search?q= */
export interface TitleSearchResult {
  /** `q`, trimmed. */
  query: string;
  /** Ranked, best first. */
  items: TitleSearchItem[];
  /** The items per type. */
  counts: TitleSearchCounts;
}
