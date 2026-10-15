import type { HDRezkaCandidate } from '@/lib/api/contract';

/**
 * "Close never loses work" (HANDOFF §4.1): what the HDRezka sheet had for a
 * context (a movie, a season, an episode) stays for the rest of the session,
 * and reopening it picks up there. Module state, so it lasts until a reload.
 */

/** The match in use; the hit's fields show while the title page loads. */
export type HDRezkaPick = Pick<HDRezkaCandidate, 'url'> & Partial<Pick<HDRezkaCandidate, 'title' | 'original_title' | 'year' | 'category'>>;

export interface HDRezkaDraft {
  /** The search field as typed. */
  input: string;
  /** The last search sent; null searches by the TMDB title. */
  query: string | null;
  pick: HDRezkaPick | null;
  /** `translatorKey` of the voice-over. */
  translator: string | null;
  /** A quality's label. */
  quality: string | null;
  /** Shows: the HDRezka season tab; null opens the scope's. */
  season?: number | null;
  /** Shows: the selected episodes (`episodeKey`); null selects the default (missing ones, or the scope's episode). */
  episodes?: string[] | null;
}

const drafts = new Map<string, HDRezkaDraft>();

export const loadDraft = (key: string): HDRezkaDraft | null => drafts.get(key) ?? null;

export function saveDraft(key: string, draft: HDRezkaDraft): void {
  drafts.set(key, draft);
}

/** After the stream is assigned: the next visit starts from what plays now. */
export function clearDraft(key: string): void {
  drafts.delete(key);
}
