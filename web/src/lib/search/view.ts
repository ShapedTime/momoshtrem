import type { Release, SearchScope, SearchSort, SearchTorrentsResponse } from '@/lib/api/contract';

/**
 * Which state the results area shows (HANDOFF §5.2):
 * - `searching`: no answer yet, or the search runs and nothing has arrived;
 * - `error`: the request failed and there's nothing to show;
 * - `failed`: the search finished, every indexer failed, nothing found;
 * - `filtered-empty`: filters hide everything the scope found (#93 dresses it up);
 * - `none`: nothing in scope at all;
 * - `results`: something to pick.
 */
export type SearchViewKind = 'searching' | 'error' | 'failed' | 'filtered-empty' | 'none' | 'results';

/** `first`: the latest page 1; `items`: every page so far. */
export function searchViewKind(first: SearchTorrentsResponse | undefined, items: number, requestFailed: boolean): SearchViewKind {
  if (!first) return requestFailed ? 'error' : 'searching';
  if (items > 0) return 'results';
  if (!first.complete) return 'searching';
  if (first.indexers.length > 0 && first.indexers.every((indexer) => indexer.status === 'timeout' || indexer.status === 'error')) return 'failed';
  return first.total_unfiltered > 0 ? 'filtered-empty' : 'none';
}

/** The best match leads page 1 when sorted by best: it gets the highlighted card. */
export function bestRelease(first: SearchTorrentsResponse | undefined, sort: SearchSort): Release | null {
  const lead = first?.items[0];
  return sort === 'best' && lead && first.best_id === lead.id ? lead : null;
}

/** The "N episodes are missing" pack banner: episode scope only, and only for a run of more than one. */
export function packBanner(first: SearchTorrentsResponse | undefined, scope: SearchScope | null) {
  const missing = first?.missing;
  return scope === 'episode' && missing && missing.count > 1 ? missing : null;
}
