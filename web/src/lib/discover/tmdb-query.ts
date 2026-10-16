import type { DiscoverItem, MediaType } from '@/lib/api/contract';
import { activeFilters, type DiscoverFilters, type DiscoverSort, type FilterKey, type RuntimeBand, type ShowStatus } from './filters';
import { ANIMATION_GENRE_ID, genreIds } from './genres';

/**
 * Discover's filters as TMDB /discover/movie and /discover/tv queries. Pure,
 * so the mapping is tested without TMDB; lib/api/tmdb.ts runs the queries.
 */

export type TmdbDiscoverParams = Record<string, string | number | boolean>;

/** One type's query; `type=all` has two, merged by `mergeStreams`. */
export interface DiscoverStream {
  mediaType: MediaType;
  params: TmdbDiscoverParams;
}

/** TMDB serves pages 1–500 only. */
export const MAX_TMDB_PAGE = 500;

/** Without a floor, TMDB's lists fill with titles nobody has rated. */
export const VOTE_FLOOR = 50;
/** Highest rated needs more votes, or a 10/10 from three people leads. */
export const RATED_VOTE_FLOOR = 200;

/** Runtime bands in minutes (TV: per episode). */
export const RUNTIME_RANGES: Record<MediaType, Record<RuntimeBand, { gte?: number; lte?: number }>> = {
  movie: { short: { lte: 99 }, standard: { gte: 100, lte: 140 }, long: { gte: 141 } },
  tv: { short: { lte: 30 }, standard: { gte: 31, lte: 50 }, long: { gte: 51 } },
};

/** TMDB TV statuses: 0 Returning, 1 Planned, 2 In production, 3 Ended, 4 Cancelled (5 Pilot isn't offered). */
export const STATUS_CODES: Record<ShowStatus, string> = { returning: '0', ended: '3|4', upcoming: '1|2' };

/**
 * TMDB's sort per type. Title A–Z is `title.asc` / `name.asc`: TMDB sorts the
 * English title as stored, so names that start with punctuation or digits
 * ("'Allo 'Allo!", "#Alive", "10 Things…") come first.
 */
export const SORT_BY: Record<DiscoverSort, Record<MediaType, string>> = {
  popular: { movie: 'popularity.desc', tv: 'popularity.desc' },
  rating: { movie: 'vote_average.desc', tv: 'vote_average.desc' },
  newest: { movie: 'primary_release_date.desc', tv: 'first_air_date.desc' },
  title: { movie: 'title.asc', tv: 'name.asc' },
};

const DATE_FIELD: Record<MediaType, string> = { movie: 'primary_release_date', tv: 'first_air_date' };

/**
 * TV genres Discover leaves out: News (10763), Reality (10764), Soap (10766)
 * and Talk (10767). Daily talk shows otherwise lead TMDB's popular list, and
 * none of them is something you'd stream from a torrent.
 */
export const TV_EXCLUDED_GENRES = [10763, 10764, 10766, 10767];

export type SkipReason = 'genre' | 'status' | 'anime_language';

/**
 * Why one type can't match these filters, or null when it can: a genre TMDB's
 * list for the type lacks (History, Romance, Thriller on TV), a show status
 * (movies), or anime with a language other than Japanese.
 */
export function skipReason(filters: DiscoverFilters, mediaType: MediaType): SkipReason | null {
  if (genreIds(filters.genres, mediaType) === null) return 'genre';
  if (filters.status !== null && mediaType === 'movie') return 'status';
  if (filters.anime && filters.lang !== null && filters.lang !== 'ja') return 'anime_language';
  return null;
}

/** The vote floor for these filters. Upcoming shows have no votes yet, so they get none unless you ask for a rating. */
export function voteFloor(filters: DiscoverFilters): number {
  if (filters.sort === 'rating') return RATED_VOTE_FLOOR;
  if (filters.status === 'upcoming' && filters.rating === null) return 0;
  return VOTE_FLOOR;
}

/** One type's TMDB query. `today` (YYYY-MM-DD) caps Newest first, so unreleased titles don't lead. */
export function tmdbDiscoverParams(filters: DiscoverFilters, mediaType: MediaType, page: number, today: string): TmdbDiscoverParams {
  const params: TmdbDiscoverParams = { include_adult: false, page, sort_by: SORT_BY[filters.sort][mediaType] };
  if (mediaType === 'tv') {
    params.include_null_first_air_dates = false;
    params.without_genres = TV_EXCLUDED_GENRES.join('|');
  }

  const floor = voteFloor(filters);
  if (floor > 0) params['vote_count.gte'] = floor;
  if (filters.rating !== null) params['vote_average.gte'] = filters.rating;

  // A comma means AND in with_genres
  const genres = genreIds(filters.genres, mediaType) ?? [];
  if (filters.anime && !genres.includes(ANIMATION_GENRE_ID)) genres.push(ANIMATION_GENRE_ID);
  if (genres.length) params.with_genres = genres.join(',');

  const language = filters.anime ? 'ja' : filters.lang;
  if (language) params.with_original_language = language;
  if (filters.country) params.with_origin_country = filters.country;

  if (filters.runtime) {
    const { gte, lte } = RUNTIME_RANGES[mediaType][filters.runtime];
    if (gte !== undefined) params['with_runtime.gte'] = gte;
    if (lte !== undefined) params['with_runtime.lte'] = lte;
  }

  const field = DATE_FIELD[mediaType];
  if (filters.from !== null) params[`${field}.gte`] = `${filters.from}-01-01`;
  let until = filters.to !== null ? `${filters.to}-12-31` : null;
  if (filters.sort === 'newest' && filters.status !== 'upcoming' && (until === null || until > today)) until = today;
  if (until !== null) params[`${field}.lte`] = until;

  if (mediaType === 'tv' && filters.status) params.with_status = STATUS_CODES[filters.status];
  return params;
}

/** The queries for one page of results: one per type that can match (none when nothing can). */
export function discoverStreams(filters: DiscoverFilters, page: number, today: string): DiscoverStream[] {
  const types: MediaType[] = filters.type === 'all' ? ['movie', 'tv'] : [filters.type];
  return types
    .filter((mediaType) => skipReason(filters, mediaType) === null)
    .map((mediaType) => ({ mediaType, params: tmdbDiscoverParams(filters, mediaType, page, today) }));
}

/** The filters `relax=1` drops one at a time: every active filter. */
export const relaxKeys = (filters: DiscoverFilters): FilterKey[] => activeFilters(filters).map((filter) => filter.key);

// ---- Merging the movie and TV streams --------------------------------------------

/** A TMDB result plus the keys the streams are merged by. */
export interface DiscoverRow extends DiscoverItem {
  popularity: number;
  vote_average: number;
  /** Release or first-air date, YYYY-MM-DD, or '' when TMDB has none. */
  date: string;
}

const titleCollator = new Intl.Collator('en', { sensitivity: 'base', numeric: true });

/** Negative when `a` comes first. */
const COMPARE: Record<DiscoverSort, (a: DiscoverRow, b: DiscoverRow) => number> = {
  popular: (a, b) => b.popularity - a.popularity,
  rating: (a, b) => b.vote_average - a.vote_average,
  newest: (a, b) => (a.date === b.date ? 0 : a.date === '' ? 1 : b.date === '' ? -1 : a.date > b.date ? -1 : 1),
  title: (a, b) => titleCollator.compare(a.title, b.title),
};

/**
 * Interleave streams that TMDB already sorted, by the sort key. A merge, not
 * a sort: each stream keeps TMDB's own order, and a tie goes to the earlier
 * stream (movies).
 */
export function mergeStreams(sort: DiscoverSort, streams: DiscoverRow[][]): DiscoverRow[] {
  const compare = COMPARE[sort];
  const heads = streams.map(() => 0);
  const merged: DiscoverRow[] = [];
  for (;;) {
    let best = -1;
    streams.forEach((rows, index) => {
      if (heads[index] >= rows.length) return;
      if (best === -1 || compare(rows[heads[index]], streams[best][heads[best]]) < 0) best = index;
    });
    if (best === -1) return merged;
    merged.push(streams[best][heads[best]]);
    heads[best] += 1;
  }
}

export const toDiscoverItem = ({ media_type, tmdb_id, title, year, poster_path }: DiscoverRow): DiscoverItem => ({ media_type, tmdb_id, title, year, poster_path });

/** "2024-03-01" → 2024; null when there's no date. */
export function yearOf(date: string | null | undefined): number | null {
  const year = Number.parseInt(date?.slice(0, 4) ?? '', 10);
  return Number.isFinite(year) ? year : null;
}

/** Today in UTC as YYYY-MM-DD (TMDB's dates have no time zone). */
export const isoDay = (now: Date = new Date()) => now.toISOString().slice(0, 10);
