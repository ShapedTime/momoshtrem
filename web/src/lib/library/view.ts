import type { LibraryCounts, LibraryItem, LibraryPage, LibraryParams, LibrarySort } from '@/lib/api/contract';
import { isWatching, matchesQuery } from './query';

/**
 * The Library page's view state, kept in the URL (`filter`, `sort`, `view`,
 * `q`; defaults are left out so a plain /library stays plain), and how it maps
 * to GET /library.
 */

export type LibraryFilter = 'all' | 'movies' | 'shows' | 'watching' | 'ready' | 'incomplete' | 'review';
export type LibraryLayout = 'grid' | 'list';

export interface LibraryViewState {
  filter: LibraryFilter;
  sort: LibrarySort;
  view: LibraryLayout;
  q: string;
}

export const DEFAULT_VIEW: LibraryViewState = { filter: 'all', sort: 'added', view: 'grid', q: '' };

export interface FilterDefinition {
  value: LibraryFilter;
  label: string;
  /** Its chip's count. */
  count: (counts: LibraryCounts) => number;
  params: Pick<LibraryParams, 'type' | 'status'>;
  matches: (item: LibraryItem) => boolean;
}

const ready = (item: LibraryItem) => item.availability_stage === 'ready';

export const FILTERS: FilterDefinition[] = [
  { value: 'all', label: 'All', count: (c) => c.all, params: {}, matches: () => true },
  { value: 'movies', label: 'Movies', count: (c) => c.movie, params: { type: 'movie' }, matches: (i) => i.media_type === 'movie' },
  { value: 'shows', label: 'Shows', count: (c) => c.tv, params: { type: 'tv' }, matches: (i) => i.media_type === 'tv' },
  { value: 'watching', label: 'Watching', count: (c) => c.watching, params: { status: 'watching' }, matches: isWatching },
  { value: 'ready', label: 'Ready', count: (c) => c.ready, params: { status: 'ready' }, matches: ready },
  { value: 'incomplete', label: 'Incomplete', count: (c) => c.incomplete, params: { status: 'incomplete' }, matches: (i) => !ready(i) },
  { value: 'review', label: 'Needs review', count: (c) => c.needs_review, params: { status: 'needs_review' }, matches: (i) => i.needs_review > 0 },
];

export const SORTS: Array<{ value: LibrarySort; label: string }> = [
  { value: 'added', label: 'Recently added' },
  { value: 'title', label: 'Title' },
  { value: 'last_watched', label: 'Last watched' },
  { value: 'size', label: 'Size' },
];

export const filterDefinition = (filter: LibraryFilter): FilterDefinition => FILTERS.find((f) => f.value === filter) ?? FILTERS[0];

/** The view state in a URL's query; unknown values fall back to the defaults. */
export function parseLibraryView(search: URLSearchParams): LibraryViewState {
  const filter = FILTERS.find((f) => f.value === search.get('filter'))?.value ?? DEFAULT_VIEW.filter;
  const sort = SORTS.find((s) => s.value === search.get('sort'))?.value ?? DEFAULT_VIEW.sort;
  const view = search.get('view') === 'list' ? 'list' : 'grid';
  return { filter, sort, view, q: search.get('q') ?? '' };
}

/**
 * The URL query for a view state: `search` with the view's keys set (or
 * removed at their defaults), so other params (an open sheet) stay.
 */
export function libraryViewQuery(state: LibraryViewState, search: URLSearchParams = new URLSearchParams()): string {
  const next = new URLSearchParams(search.toString());
  const set = (key: keyof LibraryViewState, value: string) => {
    if (value === DEFAULT_VIEW[key]) next.delete(key);
    else next.set(key, value);
  };
  set('filter', state.filter);
  set('sort', state.sort);
  set('view', state.view);
  set('q', state.q.trim() === '' ? '' : state.q);
  return next.toString();
}

/** GET /library's params for a view state (without paging). */
export function libraryParams(state: LibraryViewState): Omit<LibraryParams, 'page'> {
  const q = state.q.trim();
  return { ...filterDefinition(state.filter).params, sort: state.sort, ...(q ? { q } : {}) };
}

/**
 * The page without the titles waiting out their Undo window: they leave the
 * list, the chip counts (when they match the search), the total (when they
 * match the filter too) and the summary right away.
 */
export function withoutPending(page: Pick<LibraryPage, 'counts' | 'summary' | 'total'>, pending: LibraryItem[], state: LibraryViewState) {
  const counts = { ...page.counts };
  const summary = { ...page.summary };
  let total = page.total;
  for (const item of pending) {
    summary.total -= 1;
    summary.size_bytes -= item.size_bytes;
    if (ready(item)) summary.ready -= 1;
    if (!matchesQuery(item, state.q)) continue;
    for (const filter of FILTERS) {
      if (filter.value !== 'all' && filter.matches(item)) {
        const key = COUNT_KEYS[filter.value];
        counts[key] -= 1;
      }
    }
    counts.all -= 1;
    if (filterDefinition(state.filter).matches(item)) total -= 1;
  }
  const floor = <T extends Record<string, number>>(values: T): T =>
    Object.fromEntries(Object.entries(values).map(([key, value]) => [key, Math.max(0, value)])) as T;
  return { counts: floor(counts), summary: floor(summary), total: Math.max(0, total) };
}

const COUNT_KEYS: Record<Exclude<LibraryFilter, 'all'>, keyof LibraryCounts> = {
  movies: 'movie',
  shows: 'tv',
  watching: 'watching',
  ready: 'ready',
  incomplete: 'incomplete',
  review: 'needs_review',
};
