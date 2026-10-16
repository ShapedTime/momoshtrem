import type { MediaType, RecSignal } from '@/lib/api/contract';

/**
 * Feedback on recommended titles (PLAN §6 "Feedback", HANDOFF §4.4): For
 * You's shelves and hero, and Ask's cards (#156). It's saved at once; Undo
 * restores the previous signal, as a POST of it or a DELETE when there was
 * none. The pure rules live here; `components/recommendations` has the hook
 * and the controls.
 */

/** `dislike`, `not_interested` and `seen` drop a title from every AI surface: its card dims at once, with Undo. */
const DISMISSING: ReadonlySet<RecSignal> = new Set<RecSignal>(['dislike', 'not_interested', 'seen']);

export const isDismissing = (signal: RecSignal | null | undefined): boolean => signal != null && DISMISSING.has(signal);

/** One title, whatever surface shows it: `movie:666277`. */
export const recFeedbackKey = (mediaType: MediaType, tmdbId: number): string => `${mediaType}:${tmdbId}`;

/**
 * The signal a card shows: what this page set last (`null` once cleared),
 * else the server's. The server only ever says `like` or nothing: a title
 * with negative feedback is left out of what it sends.
 */
export function shownSignal(local: RecSignal | null | undefined, server: 'like' | null): RecSignal | null {
  return local === undefined ? server : local;
}

/** Choosing a signal that's already on clears it (a second tap on 👍 un-likes); any other replaces it. */
export const toggledSignal = (current: RecSignal | null, chosen: RecSignal): RecSignal | null => (current === chosen ? null : chosen);

/** What puts a title back to `signal` on the server: POST it, or DELETE when there's none. */
export type FeedbackRequest = { method: 'POST'; signal: RecSignal } | { method: 'DELETE' };

export const feedbackRequest = (signal: RecSignal | null): FeedbackRequest => (signal === null ? { method: 'DELETE' } : { method: 'POST', signal });

export interface FeedbackChange {
  from: RecSignal | null;
  to: RecSignal | null;
  /** Saves `to` at once. */
  request: FeedbackRequest;
  /** Undo: brings back the previous signal (`from`), or clears it when there was none. */
  undo: FeedbackRequest;
  /** The card dims and the Undo toast shows. */
  dismisses: boolean;
}

/** Going from the signal on screen to another: what to send, and what Undo sends. Null when nothing changes. */
export function feedbackChange(from: RecSignal | null, to: RecSignal | null): FeedbackChange | null {
  if (from === to) return null;
  return { from, to, request: feedbackRequest(to), undo: feedbackRequest(from), dismisses: isDismissing(to) };
}

export interface FeedbackToastCopy {
  title: string;
  body: string;
}

/** The Undo toast after a dismissing signal (HANDOFF §4.4: "Got it, fewer like this"); null for `like` and clearing. */
export function dismissToast(signal: RecSignal | null, title: string): FeedbackToastCopy | null {
  switch (signal) {
    case 'dislike':
      return { title: 'Got it, fewer like this', body: `${title} won’t be picked again.` };
    case 'not_interested':
      return { title: 'Got it, fewer like this', body: `${title} is hidden from your picks and Ask.` };
    case 'seen':
      return { title: 'Got it, fewer like this', body: `${title} counts as watched.` };
    default:
      return null;
  }
}
