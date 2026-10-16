import type { LibraryCounts, LibraryItem, LibraryPage, LibraryParams, LibrarySort, LibraryStatusFilter, MediaType } from '@/lib/api/contract';

/**
 * GET /library's rules (PLAN §6) over a list of items, for the mocks and
 * Jest: filters, sorts, pages, counts (honouring `q` and `ids`, never `type`
 * or `status`) and the summary (the whole library). The backend does the
 * same in SQL (internal/library/library_list_repo.go).
 */

export const DEFAULT_PAGE_SIZE = 50;
export const MAX_PAGE_SIZE = 200;
export const MAX_IDS = 200;

const TYPES: MediaType[] = ['movie', 'tv'];
const STATUSES: LibraryStatusFilter[] = ['ready', 'incomplete', 'needs_review', 'watching'];
const SORTS: LibrarySort[] = ['added', 'title', 'last_watched', 'size'];

const ID_PATTERN = /^(movie|tv):([1-9]\d*)$/;

/** `movie:603,tv:1399` → the set of `media_type:tmdb_id` keys, or null when malformed or over 200. */
export function parseLibraryIds(raw: string): Set<string> | null {
  const parts = raw.split(',');
  if (parts.length > MAX_IDS) return null;
  const keys = new Set<string>();
  for (const part of parts) {
    const match = ID_PATTERN.exec(part.trim());
    if (!match) return null;
    keys.add(`${match[1]}:${Number(match[2])}`);
  }
  return keys;
}

export type ParsedLibrarySearch = { ok: true; params: LibraryParams } | { ok: false; message: string };

const positive = (raw: string | null): number | null | undefined => {
  if (raw === null || raw === '') return undefined;
  const n = Number(raw);
  return Number.isInteger(n) && n >= 1 ? n : null;
};

/** Read and check GET /library's query string; unknown values are a 400, as on the backend. */
export function parseLibrarySearch(search: URLSearchParams): ParsedLibrarySearch {
  const type = search.get('type') || undefined;
  const status = search.get('status') || undefined;
  const sort = search.get('sort') || undefined;
  const ids = search.get('ids') || undefined;
  const page = positive(search.get('page'));
  const pageSize = positive(search.get('page_size'));
  if (type !== undefined && !TYPES.some((t) => t === type)) return { ok: false, message: 'type must be movie or tv' };
  if (status !== undefined && !STATUSES.some((s) => s === status)) {
    return { ok: false, message: 'status must be ready, incomplete, needs_review or watching' };
  }
  if (sort !== undefined && !SORTS.some((s) => s === sort)) return { ok: false, message: 'sort must be added, title, last_watched or size' };
  if (page === null) return { ok: false, message: 'page must be a positive number' };
  if (pageSize === null) return { ok: false, message: 'page_size must be a positive number' };
  if (ids !== undefined && parseLibraryIds(ids) === null) return { ok: false, message: 'ids must look like movie:603,tv:1399' };
  return {
    ok: true,
    params: {
      type: type as MediaType | undefined,
      status: status as LibraryStatusFilter | undefined,
      sort: sort as LibrarySort | undefined,
      q: search.get('q') ?? undefined,
      ids,
      page,
      page_size: pageSize,
    },
  };
}

const isReady = (item: LibraryItem) => item.availability_stage === 'ready';

/**
 * Watching (PLAN §6): a show with some of its episodes watched, not all.
 * The backend also leaves out shows dropped or hidden on Trakt, which items
 * don't say.
 */
export function isWatching(item: Pick<LibraryItem, 'media_type' | 'episodes_watched' | 'episodes_total'>): boolean {
  const watched = item.episodes_watched ?? 0;
  return item.media_type === 'tv' && watched > 0 && watched < (item.episodes_total ?? 0);
}

function matchesStatus(item: LibraryItem, status: LibraryStatusFilter | undefined): boolean {
  switch (status) {
    case undefined:
      return true;
    case 'ready':
      return isReady(item);
    case 'incomplete':
      return !isReady(item);
    case 'needs_review':
      return item.needs_review > 0;
    case 'watching':
      return isWatching(item);
  }
}

/** Newest first, then a stable tie-break (the backend's `media_type, id`). */
const byAdded = (a: LibraryItem, b: LibraryItem) =>
  b.added_at.localeCompare(a.added_at) || a.media_type.localeCompare(b.media_type) || a.id - b.id;

/** `last_watched_at DESC NULLS LAST`, then as added. */
function byLastWatched(a: LibraryItem, b: LibraryItem): number {
  if (a.last_watched_at !== b.last_watched_at) {
    if (a.last_watched_at === null) return 1;
    if (b.last_watched_at === null) return -1;
    const order = Date.parse(b.last_watched_at) - Date.parse(a.last_watched_at);
    if (order !== 0) return order;
  }
  return byAdded(a, b);
}

const SORTERS: Record<LibrarySort, (a: LibraryItem, b: LibraryItem) => number> = {
  added: byAdded,
  title: (a, b) =>
    a.title.toLowerCase().localeCompare(b.title.toLowerCase()) || (a.year ?? 0) - (b.year ?? 0) || a.media_type.localeCompare(b.media_type) || a.id - b.id,
  last_watched: byLastWatched,
  size: (a, b) => b.size_bytes - a.size_bytes || byAdded(a, b),
};

/** Case-insensitive substring of the title, like the backend's ILIKE. */
export function matchesQuery(item: Pick<LibraryItem, 'title'>, q: string | undefined): boolean {
  const needle = q?.trim().toLowerCase() ?? '';
  return needle === '' || item.title.toLowerCase().includes(needle);
}

export function libraryCounts(items: LibraryItem[]): LibraryCounts {
  return {
    all: items.length,
    movie: items.filter((item) => item.media_type === 'movie').length,
    tv: items.filter((item) => item.media_type === 'tv').length,
    watching: items.filter(isWatching).length,
    ready: items.filter(isReady).length,
    incomplete: items.filter((item) => !isReady(item)).length,
    needs_review: items.filter((item) => item.needs_review > 0).length,
  };
}

/** GET /library over `items` (the whole library); `params` are already checked (parseLibrarySearch). */
export function queryLibrary(items: LibraryItem[], params: LibraryParams = {}): LibraryPage {
  const ids = params.ids ? parseLibraryIds(params.ids) : null;
  const inScope = items.filter((item) => matchesQuery(item, params.q) && (!ids || ids.has(`${item.media_type}:${item.tmdb_id}`)));
  const matching = inScope
    .filter((item) => (!params.type || item.media_type === params.type) && matchesStatus(item, params.status))
    .sort(SORTERS[params.sort ?? 'added']);
  const page = ids ? 1 : (params.page ?? 1);
  const pageSize = ids ? ids.size : Math.min(params.page_size ?? DEFAULT_PAGE_SIZE, MAX_PAGE_SIZE);
  return {
    items: matching.slice((page - 1) * pageSize, page * pageSize),
    total: matching.length,
    page,
    page_size: pageSize,
    counts: libraryCounts(inScope),
    summary: {
      total: items.length,
      size_bytes: items.reduce((sum, item) => sum + item.size_bytes, 0),
      ready: items.filter(isReady).length,
    },
  };
}
