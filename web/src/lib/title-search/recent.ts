import { cleanQuery } from './text';

/**
 * Recent searches, newest first, for the search page and the palette. They
 * live in this browser's localStorage, which can be missing, full or
 * blocked (private windows, previews): every access is wrapped, and a
 * failure only means the list isn't kept. A small store, so every surface
 * (and other tabs, through `storage` events) sees one list.
 */

export const RECENT_SEARCHES_KEY = 'momoshtrem:recent-searches';
export const MAX_RECENT_SEARCHES = 8;

const NONE: readonly string[] = Object.freeze([]);

let cache: readonly string[] | null = null;
const listeners = new Set<() => void>();

function read(): readonly string[] {
  try {
    const raw = window.localStorage.getItem(RECENT_SEARCHES_KEY);
    const parsed: unknown = raw ? JSON.parse(raw) : [];
    if (!Array.isArray(parsed)) return NONE;
    return parsed.filter((entry): entry is string => typeof entry === 'string' && cleanQuery(entry) !== null).slice(0, MAX_RECENT_SEARCHES);
  } catch {
    return NONE;
  }
}

function write(next: readonly string[]): void {
  cache = next;
  try {
    window.localStorage.setItem(RECENT_SEARCHES_KEY, JSON.stringify(next));
  } catch {
    // Not kept past this page; the list still updates
  }
  listeners.forEach((listener) => listener());
}

/** The list now (the same array until it changes, as useSyncExternalStore needs). */
export function recentSearches(): readonly string[] {
  if (cache === null) cache = typeof window === 'undefined' ? NONE : read();
  return cache;
}

/** The server's (and the first render's) list: none, so hydration matches. */
export const serverRecentSearches = (): readonly string[] => NONE;

const same = (a: string, b: string) => a.toLocaleLowerCase() === b.toLocaleLowerCase();

/** Put a query first (once, whatever its case); blank or over-long queries are ignored. */
export function addRecentSearch(raw: string): void {
  const query = cleanQuery(raw);
  if (!query) return;
  const current = recentSearches();
  if (current[0] === query) return;
  write([query, ...current.filter((entry) => !same(entry, query))].slice(0, MAX_RECENT_SEARCHES));
}

export function removeRecentSearch(query: string): void {
  write(recentSearches().filter((entry) => entry !== query));
}

export function clearRecentSearches(): void {
  write([]);
}

function onStorage(event: StorageEvent): void {
  if (event.key !== null && event.key !== RECENT_SEARCHES_KEY) return;
  cache = null;
  listeners.forEach((listener) => listener());
}

export function subscribeRecentSearches(listener: () => void): () => void {
  if (listeners.size === 0 && typeof window !== 'undefined') window.addEventListener('storage', onStorage);
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
    if (listeners.size === 0 && typeof window !== 'undefined') window.removeEventListener('storage', onStorage);
  };
}

/** Tests: forget the cached list so the next read goes to storage. */
export function resetRecentSearchesCache(): void {
  cache = null;
}
