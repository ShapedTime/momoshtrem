import type { LibraryItem, MediaType, TitleSearchItem } from '@/lib/api/contract';
import { MATCH_SCORE, titleMatch } from './rank';

/**
 * The search page's and the palette's sections (js/screens/search.js
 * `resultsBody`, `palBody`): your library first, then the TMDB titles you
 * don't have, with counts per tab. Pure, so Jest checks it.
 */

export const SEARCH_TABS = ['all', 'tv', 'movie'] as const;
export type SearchTab = (typeof SEARCH_TABS)[number];

/** A library title that matched, and why when it wasn't by its title. */
export interface LibraryHit {
  item: LibraryItem;
  reason: string | null;
}

export interface SearchSections {
  library: LibraryHit[];
  /** TMDB's titles that aren't in the library, best first. */
  titles: TitleSearchItem[];
  counts: Record<SearchTab, number>;
}

const keyOf = (item: { media_type: MediaType; tmdb_id: number }) => `${item.media_type}:${item.tmdb_id}`;

interface Scored {
  hit: LibraryHit;
  score: number;
  order: number;
}

/**
 * Your library's matches are `/library?q=` (titles holding the query) plus
 * the TMDB titles `inLibrary` knows (an original title, a person), ranked by
 * the better of the two matches; the TMDB titles you don't have follow.
 * Library titles always come first, as their own group: that's their boost.
 */
export function searchSections(
  query: string,
  tmdb: TitleSearchItem[],
  library: LibraryItem[],
  inLibrary: (mediaType: MediaType, tmdbId: number) => LibraryItem | undefined
): SearchSections {
  const hits = new Map<string, Scored>();
  library.forEach((item, index) => {
    const match = titleMatch(query, item.title, null) ?? 'words';
    hits.set(keyOf(item), { hit: { item, reason: null }, score: MATCH_SCORE[match], order: tmdb.length + index });
  });

  const titles: TitleSearchItem[] = [];
  tmdb.forEach((title, index) => {
    const key = keyOf(title);
    const known = hits.get(key);
    const item = known?.hit.item ?? inLibrary(title.media_type, title.tmdb_id);
    if (!item) {
      titles.push(title);
      return;
    }
    const score = MATCH_SCORE[title.match];
    const better = !known || score > known.score;
    hits.set(key, {
      hit: { item, reason: better ? title.match_reason : known.hit.reason },
      score: better ? score : known.score,
      order: Math.min(index, known?.order ?? index),
    });
  });

  const ranked = [...hits.values()].sort((a, b) => b.score - a.score || a.order - b.order).map((scored) => scored.hit);
  const types = [...ranked.map((hit) => hit.item.media_type), ...titles.map((title) => title.media_type)];
  return {
    library: ranked,
    titles,
    counts: { all: types.length, tv: types.filter((type) => type === 'tv').length, movie: types.filter((type) => type === 'movie').length },
  };
}

/** The sections a tab shows. */
export function inTab(sections: SearchSections, tab: SearchTab): Pick<SearchSections, 'library' | 'titles'> {
  if (tab === 'all') return sections;
  return { library: sections.library.filter((hit) => hit.item.media_type === tab), titles: sections.titles.filter((title) => title.media_type === tab) };
}

/** The grid's heading per tab: "Shows and movies", "Shows", "Movies". */
export const TMDB_HEADING: Record<SearchTab, string> = { all: 'Shows and movies', tv: 'Shows', movie: 'Movies' };

/** An empty tab's words: "shows", "movies". */
export const TAB_NOUN: Record<Exclude<SearchTab, 'all'>, string> = { tv: 'shows', movie: 'movies' };
