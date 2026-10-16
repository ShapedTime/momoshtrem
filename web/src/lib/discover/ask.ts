import { genreBySlug } from './genres';
import { COUNTRY_LABELS, decadeOf, type DiscoverFilters, LANGUAGE_LABELS } from './filters';

/**
 * What Discover's empty state asks Noir (discover.js: "an anime comedy that
 * has finished airing"): the active filters in words. Empty without
 * filters, so the button opens a blank ask.
 */

const STATUS_WORDS = { returning: 'that’s still airing', ended: 'that has finished airing', upcoming: 'that’s coming soon' } as const;
const RUNTIME_WORDS = { short: 'on the short side', standard: 'of a standard length', long: 'on the long side' } as const;

function years({ from, to }: Pick<DiscoverFilters, 'from' | 'to'>): string | null {
  const decade = decadeOf({ from, to });
  if (decade) return decade.from === null ? `from before ${(decade.to ?? 0) + 1}` : `from the ${decade.label}`;
  if (from !== null && to !== null) return from === to ? `from ${from}` : `from ${from} to ${to}`;
  if (from !== null) return `from ${from} on`;
  if (to !== null) return `from ${to} or earlier`;
  return null;
}

const article = (phrase: string) => (/^[aeiou]/i.test(phrase) ? `an ${phrase}` : `a ${phrase}`);

export function discoverAskQuery(filters: DiscoverFilters): string {
  // "Japanese anime" says Japanese twice
  const language = filters.lang && !(filters.anime && filters.lang === 'ja') ? LANGUAGE_LABELS[filters.lang] : null;
  const genres = filters.genres.map((slug) => genreBySlug(slug).label.toLowerCase());
  const noun = filters.type === 'movie' ? 'movie' : filters.type === 'tv' ? 'show' : null;
  const words = [filters.anime ? 'anime' : null, ...genres, noun].filter((word): word is string => word !== null);
  // "a Korean comedy", but "something in Korean"
  if (language && words.length > 0) words.unshift(language);

  const qualifiers = [
    language && words.length === 0 ? `in ${language}` : null,
    years(filters),
    filters.country ? `made in ${COUNTRY_LABELS[filters.country]}` : null,
    filters.rating !== null ? `rated ${filters.rating}+` : null,
    filters.runtime ? RUNTIME_WORDS[filters.runtime] : null,
    filters.type !== 'movie' && filters.status ? STATUS_WORDS[filters.status] : null,
  ].filter((qualifier): qualifier is string => qualifier !== null);

  if (words.length === 0 && qualifiers.length === 0) return '';
  const head = words.length > 0 ? article(words.join(' ')) : 'something';
  return qualifiers.length > 0 ? `${head} ${qualifiers.join(', ')}` : head;
}
