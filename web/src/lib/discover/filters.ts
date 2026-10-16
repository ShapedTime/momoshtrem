import { formatNumber } from '@/lib/title/format';
import { GENRE_SLUGS, genreBySlug, type GenreSlug } from './genres';

/**
 * Discover's filters (js/screens/discover.js): the vocabulary, the defaults,
 * and the active filters as removable chips. The URL keys and their parsers
 * live in url.ts; what each filter asks TMDB lives in tmdb-query.ts.
 */

export const DISCOVER_TYPES = ['all', 'movie', 'tv'] as const;
export type DiscoverType = (typeof DISCOVER_TYPES)[number];

export const DISCOVER_SORTS = ['popular', 'rating', 'newest', 'title'] as const;
export type DiscoverSort = (typeof DISCOVER_SORTS)[number];

export const RATINGS = [6, 7, 8] as const;
export type RatingFloor = (typeof RATINGS)[number];

export const RUNTIMES = ['short', 'standard', 'long'] as const;
export type RuntimeBand = (typeof RUNTIMES)[number];

export const LANGUAGES = ['en', 'ja', 'ko', 'fr', 'es'] as const;
export type LanguageCode = (typeof LANGUAGES)[number];

export const COUNTRIES = ['US', 'GB', 'JP', 'KR', 'AU', 'FR'] as const;
export type CountryCode = (typeof COUNTRIES)[number];

/** Shows only: Airing (returning), Ended (ended or cancelled), Upcoming (planned or in production). */
export const STATUSES = ['returning', 'ended', 'upcoming'] as const;
export type ShowStatus = (typeof STATUSES)[number];

export const MOOD_IDS = ['cosy', 'mind', 'slow', 'laugh', 'short', 'epic'] as const;
export type MoodId = (typeof MOOD_IDS)[number];

export interface DiscoverFilters {
  type: DiscoverType;
  /** Matched with AND, in table order. */
  genres: GenreSlug[];
  from: number | null;
  to: number | null;
  rating: RatingFloor | null;
  runtime: RuntimeBand | null;
  lang: LanguageCode | null;
  country: CountryCode | null;
  anime: boolean;
  status: ShowStatus | null;
  /** For a readable URL only; whether a mood is selected comes from the filters (moods.ts). */
  mood: MoodId | null;
  sort: DiscoverSort;
}

export const DEFAULT_FILTERS: DiscoverFilters = {
  type: 'all',
  genres: [],
  from: null,
  to: null,
  rating: null,
  runtime: null,
  lang: null,
  country: null,
  anime: false,
  status: null,
  mood: null,
  sort: 'popular',
};

// ---- Labels -------------------------------------------------------------------

export const TYPE_LABELS: Record<DiscoverType, string> = { all: 'All', movie: 'Movies', tv: 'Shows' };
export const SORT_LABELS: Record<DiscoverSort, string> = { popular: 'Most popular', rating: 'Highest rated', newest: 'Newest first', title: 'Title A–Z' };
export const RUNTIME_LABELS: Record<RuntimeBand, string> = { short: 'Short', standard: 'Standard', long: 'Long' };
export const LANGUAGE_LABELS: Record<LanguageCode, string> = { en: 'English', ja: 'Japanese', ko: 'Korean', fr: 'French', es: 'Spanish' };
export const COUNTRY_LABELS: Record<CountryCode, string> = {
  US: 'United States',
  GB: 'United Kingdom',
  JP: 'Japan',
  KR: 'South Korea',
  AU: 'Australia',
  FR: 'France',
};
export const STATUS_LABELS: Record<ShowStatus, string> = { returning: 'Airing', ended: 'Ended', upcoming: 'Upcoming' };

// ---- Years ----------------------------------------------------------------------

/** The year selects run from this year back to YEAR_MIN; the URL accepts a wider range. */
export const YEAR_MIN = 1950;
export const URL_YEAR_MIN = 1874;
export const URL_YEAR_SPAN_AHEAD = 5;

export const currentYear = (now: Date = new Date()) => now.getFullYear();

export interface Decade {
  label: string;
  from: number | null;
  to: number | null;
}

/** Decade chips. The current decade is open-ended, so the link stays right as years pass. */
export const DECADES: Decade[] = [
  { label: '2020s', from: 2020, to: null },
  { label: '2010s', from: 2010, to: 2019 },
  { label: '2000s', from: 2000, to: 2009 },
  { label: 'Before 2000', from: null, to: 1999 },
];

export const decadeOf = (filters: Pick<DiscoverFilters, 'from' | 'to'>): Decade | undefined =>
  DECADES.find((decade) => decade.from === filters.from && decade.to === filters.to);

/** Tapping a decade selects its range; tapping the active one clears the years. */
export function toggleDecade(filters: DiscoverFilters, decade: Decade): DiscoverFilters {
  const on = decadeOf(filters) === decade;
  return { ...filters, from: on ? null : decade.from, to: on ? null : decade.to };
}

/** "2020s", "2015–2026" (from only, as the design), "Until 1985", "2001–2010". */
export function yearLabel(filters: Pick<DiscoverFilters, 'from' | 'to'>, now: Date = new Date()): string | null {
  const { from, to } = filters;
  if (from === null && to === null) return null;
  const decade = decadeOf(filters);
  if (decade) return decade.label;
  if (from === null) return `Until ${to}`;
  return `${from}–${to ?? currentYear(now)}`;
}

// ---- Normalising --------------------------------------------------------------

/**
 * One canonical shape for equal filters: genres once each in table order,
 * years in order, and no status for movies (it's for shows only).
 */
export function normalizeFilters(filters: DiscoverFilters): DiscoverFilters {
  const genres = GENRE_SLUGS.filter((slug) => filters.genres.includes(slug));
  let { from, to } = filters;
  if (from !== null && to !== null && from > to) [from, to] = [to, from];
  const status = filters.type === 'movie' ? null : filters.status;
  return { ...filters, genres, from, to, status };
}

// ---- Active filters (chips) ----------------------------------------------------

/** One removable filter. Type isn't one: it's the noun ("42 shows"). */
export type FilterKey = `genre:${GenreSlug}` | 'anime' | 'year' | 'rating' | 'runtime' | 'lang' | 'country' | 'status';

export interface ActiveFilter {
  key: FilterKey;
  label: string;
}

/** The active filters in the design's chip order. */
export function activeFilters(filters: DiscoverFilters, now: Date = new Date()): ActiveFilter[] {
  const chips: ActiveFilter[] = filters.genres.map((slug) => ({ key: `genre:${slug}`, label: genreBySlug(slug).label }));
  if (filters.anime) chips.push({ key: 'anime', label: 'Anime' });
  const years = yearLabel(filters, now);
  if (years) chips.push({ key: 'year', label: years });
  if (filters.rating !== null) chips.push({ key: 'rating', label: `Rated ${filters.rating}+` });
  if (filters.runtime) chips.push({ key: 'runtime', label: `${RUNTIME_LABELS[filters.runtime]} runtime` });
  if (filters.lang) chips.push({ key: 'lang', label: LANGUAGE_LABELS[filters.lang] });
  if (filters.country) chips.push({ key: 'country', label: COUNTRY_LABELS[filters.country] });
  if (filters.status) chips.push({ key: 'status', label: STATUS_LABELS[filters.status] });
  return chips;
}

/** The filters with one removed. */
export function withoutFilter(filters: DiscoverFilters, key: FilterKey): DiscoverFilters {
  if (key.startsWith('genre:')) return { ...filters, genres: filters.genres.filter((slug) => `genre:${slug}` !== key) };
  switch (key) {
    case 'anime':
      return { ...filters, anime: false };
    case 'year':
      return { ...filters, from: null, to: null };
    case 'rating':
      return { ...filters, rating: null };
    case 'runtime':
      return { ...filters, runtime: null };
    case 'lang':
      return { ...filters, lang: null };
    case 'country':
      return { ...filters, country: null };
    case 'status':
      return { ...filters, status: null };
    default:
      return filters;
  }
}

/** "Clear all": every filter goes; the type and the sort stay. */
export const clearFilters = (filters: DiscoverFilters): DiscoverFilters => ({ ...DEFAULT_FILTERS, type: filters.type, sort: filters.sort });

/** The phone's Filters badge: the active filters plus a chosen type. */
export const filterBadgeCount = (filters: DiscoverFilters) => activeFilters(filters).length + (filters.type === 'all' ? 0 : 1);

// ---- Copy ---------------------------------------------------------------------

/** The noun follows the type: titles, movies or shows. */
export function noun(type: DiscoverType, count: number): string {
  if (type === 'movie') return count === 1 ? 'movie' : 'movies';
  if (type === 'tv') return count === 1 ? 'show' : 'shows';
  return count === 1 ? 'title' : 'titles';
}

/**
 * TMDB reports at most about 20,000 results per type (20,001 when there are
 * more), so a total from there up is a floor, not a count.
 */
export const TMDB_TOTAL_CAP = 20_000;

/** "10,296 titles", "1 show", "20,000+ movies". */
export function countLabel(type: DiscoverType, count: number): string {
  if (count >= TMDB_TOTAL_CAP) return `${formatNumber(TMDB_TOTAL_CAP)}+ ${noun(type, 2)}`;
  return `${formatNumber(count)} ${noun(type, count)}`;
}
