import { createSerializer, parseAsString, parseAsStringLiteral } from 'nuqs/server';
import { SEARCH_TABS } from './results';

/**
 * The search page's URL (js/screens/search.js): `q` and `tab` (`tv` or
 * `movie`; All is the default and left out). `nuqs/server` has no
 * "use client", so the palette and pure code can build links too.
 */
export const searchParsers = {
  q: parseAsString.withDefault(''),
  tab: parseAsStringLiteral(SEARCH_TABS).withDefault('all'),
};

const serialize = createSerializer(searchParsers);

/** `/search?q=dune`, or `/search` for no query. */
export const searchHref = (query: string) => serialize('/search', { q: query.trim() || null });
