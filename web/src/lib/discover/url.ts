import {
  createLoader,
  createParser,
  createSerializer,
  parseAsArrayOf,
  parseAsNumberLiteral,
  parseAsStringLiteral,
} from 'nuqs/server';
import { GENRE_SLUGS } from './genres';
import {
  COUNTRIES,
  currentYear,
  DEFAULT_FILTERS,
  DISCOVER_SORTS,
  DISCOVER_TYPES,
  type DiscoverFilters,
  LANGUAGES,
  MOOD_IDS,
  normalizeFilters,
  RATINGS,
  RUNTIMES,
  STATUSES,
  URL_YEAR_MIN,
  URL_YEAR_SPAN_AHEAD,
} from './filters';

/**
 * Discover's URL (js/screens/discover.js): `type, genres (comma-separated
 * slugs), from, to, rating, runtime, lang, country, anime=1, status, mood,
 * sort`, with default values left out. Unknown values are dropped, so an old
 * or hand-edited link still opens. The same parsers read GET /api/discover's
 * query (without `mood`), so the page and the route can't disagree.
 * `nuqs/server` has no "use client", so the route can import these too.
 */

/** `anime=1`; absent means off (with a default of false, false is never written). */
const parseAsFlag = createParser<boolean>({
  parse: (value: string) => (value === '1' ? true : null),
  serialize: () => '1',
});

/** A plausible release year; anything else is dropped. */
const parseAsYear = createParser({
  parse: (value: string) => {
    if (!/^\d{4}$/.test(value)) return null;
    const year = Number(value);
    return year >= URL_YEAR_MIN && year <= currentYear() + URL_YEAR_SPAN_AHEAD ? year : null;
  },
  serialize: (year: number) => String(year),
});

export const discoverParsers = {
  type: parseAsStringLiteral(DISCOVER_TYPES).withDefault(DEFAULT_FILTERS.type),
  genres: parseAsArrayOf(parseAsStringLiteral(GENRE_SLUGS), ',').withDefault([]),
  from: parseAsYear,
  to: parseAsYear,
  rating: parseAsNumberLiteral(RATINGS),
  runtime: parseAsStringLiteral(RUNTIMES),
  lang: parseAsStringLiteral(LANGUAGES),
  country: parseAsStringLiteral(COUNTRIES),
  anime: parseAsFlag.withDefault(false),
  status: parseAsStringLiteral(STATUSES),
  mood: parseAsStringLiteral(MOOD_IDS),
  sort: parseAsStringLiteral(DISCOVER_SORTS).withDefault(DEFAULT_FILTERS.sort),
};

/** The page-local filter sheet on phone: `?filters=1`. */
export const FILTER_SHEET_KEY = 'filters';
export const filterSheetParser = parseAsFlag;

const load = createLoader(discoverParsers);
const serialize = createSerializer(discoverParsers);

/** Filters from a URL query (the page's, or the route's), normalised. */
export function parseDiscoverSearch(search: URLSearchParams | string): DiscoverFilters {
  const parsed = load(typeof search === 'string' ? new URLSearchParams(search) : search);
  return normalizeFilters({ ...parsed, genres: [...parsed.genres] });
}

/** `/discover?type=tv&genres=comedy`, or `/discover` with every default. */
export const discoverHref = (filters: DiscoverFilters) => serialize('/discover', normalizeFilters(filters));

/**
 * GET /api/discover's query for these filters, without `?` and without `mood`
 * (it changes nothing): also the TanStack Query key, so equal filters share
 * a cache entry.
 */
export const discoverApiQuery = (filters: DiscoverFilters) => serialize({ ...normalizeFilters(filters), mood: null }).replace(/^\?/, '');
