import type { SearchScope, SearchSort } from '@/lib/api/contract';
import type { SearchFilters } from './filters';

/**
 * "Close never loses work" (HANDOFF §4.1): the torrent search keeps its query,
 * scope, sort and filters per context (a movie, a season, an episode) for the
 * rest of the session, and reopening it picks up there. Module state, so it
 * lasts until a reload (like `lib/hdrezka/session.ts`).
 */
export interface SearchDraft {
  /** The field as the user typed it; unused until `dirty`, while the field shows the server's primary query. */
  query: string;
  /** The user edited the field: only then is `q` sent. */
  dirty: boolean;
  /** Null: the context's default (`defaultScope`). */
  scope: SearchScope | null;
  sort: SearchSort;
  filters: SearchFilters;
}

export const freshDraft = (): SearchDraft => ({ query: '', dirty: false, scope: null, sort: 'best', filters: {} });

const drafts = new Map<string, SearchDraft>();

export const loadDraft = (key: string): SearchDraft | null => drafts.get(key) ?? null;

export function saveDraft(key: string, draft: SearchDraft): void {
  drafts.set(key, draft);
}

export function clearDraft(key: string): void {
  drafts.delete(key);
}
