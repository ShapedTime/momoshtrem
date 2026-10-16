'use client';

import { useCallback, useMemo, useSyncExternalStore } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { Ban, CheckCheck, type LucideIcon, ThumbsDown } from 'lucide-react';
import { useToast } from '@/components/ui';
import { UNDO_MS } from '@/hooks/useUndoableRemoval';
import type { MediaType, RecFeedbackSource, RecSignal } from '@/lib/api/contract';
import { recApi, recKeys, titleKeys } from '@/lib/api/endpoints';
import { dismissToast, feedbackChange, type FeedbackRequest, isDismissing, recFeedbackKey, shownSignal, toggledSignal } from '@/lib/recommendations/feedback';
import {
  clearUndoToast,
  confirmedSignal,
  inOrder,
  localSignal,
  setConfirmedSignal,
  setLocalSignal,
  setUndoToast,
  subscribeLocalSignals,
  undoToast,
} from '@/lib/recommendations/feedback-store';

/** A title that takes feedback: a shelf's `RecItem`, an Ask card's `AskItem`, or a hero pick with its title's name. */
export interface RecFeedbackTarget {
  media_type: MediaType;
  tmdb_id: number;
  /** For the toasts and the controls' names. */
  title: string;
  /** The server's: `RecItem.feedback`, `HeroPick.feedback` (only ever `like` or null). */
  feedback: 'like' | null;
}

/** What `useRecFeedback` gives a card: the signal on screen and the ways to change it. */
export interface RecFeedbackControls {
  target: RecFeedbackTarget;
  /** What this page set last, else the server's. */
  signal: RecSignal | null;
  /** 👍 is on (`aria-pressed`). A like never dims the card. */
  liked: boolean;
  /** 👎 is on (`aria-pressed`). */
  disliked: boolean;
  /** Dislike, Not interested or Already seen: dim the card (`PosterCard dismissed`). */
  dismissed: boolean;
  /** Choose a signal; choosing the one that's on clears it (a second tap on 👍 un-likes). */
  toggle: (signal: RecSignal) => void;
}

const TOAST_ICON: Partial<Record<RecSignal, LucideIcon>> = { dislike: ThumbsDown, not_interested: Ban, seen: CheckCheck };

/**
 * Feedback on one recommended title (PLAN §6 "Feedback", HANDOFF §4.4). It's
 * saved at once. A dismissing signal (dislike, not interested, seen) dims the
 * card and shows "Got it, fewer like this" with Undo for 10 s, and Undo
 * restores the previous signal: a POST of it, or a DELETE when there was
 * none. Every card showing the title agrees (the store is per title, not
 * per card). A newer change to the title closes its Undo toast, so an Undo
 * never wipes it. A request that fails goes back to what the server last
 * accepted, and says so.
 *
 *   const feedback = useRecFeedback(item, 'shelf');
 *   <PosterCard dismissed={feedback.dismissed} footer={<FeedbackRow feedback={feedback} inLibrary={item.library !== null} />} />
 */
export function useRecFeedback(target: RecFeedbackTarget, source: RecFeedbackSource): RecFeedbackControls {
  const { media_type: mediaType, tmdb_id: tmdbId, title, feedback: server } = target;
  const key = recFeedbackKey(mediaType, tmdbId);
  const local = useSyncExternalStore(
    subscribeLocalSignals,
    () => localSignal(key),
    () => undefined
  );
  const signal = shownSignal(local, server);
  const { toast, dismiss } = useToast();
  const queryClient = useQueryClient();

  /**
   * Show `to` at once and send `request` after the title's earlier requests.
   * Once the server accepts it, it's the confirmed signal; if it fails, the
   * last confirmed one comes back (unless something newer replaced it).
   * Resolves to whether it was saved.
   */
  const apply = useCallback(
    (to: RecSignal | null, request: FeedbackRequest, failure: string): Promise<boolean> => {
      setLocalSignal(key, to);
      return inOrder(key, () =>
        request.method === 'DELETE'
          ? recApi.clearFeedback(mediaType, tmdbId)
          : recApi.feedback({ media_type: mediaType, tmdb_id: tmdbId, signal: request.signal, source })
      ).then(
        () => {
          setConfirmedSignal(key, to);
          return true;
        },
        (error: Error) => {
          if (localSignal(key) === to) {
            const saved = confirmedSignal(key);
            setLocalSignal(key, saved === undefined ? server : saved);
          }
          toast({ tone: 'error', title: `${failure} ${title}`, body: error.message });
          return false;
        }
      );
    },
    [key, mediaType, tmdbId, source, server, title, toast]
  );

  const toggle = useCallback(
    (chosen: RecSignal) => {
      // Read the store, not the render's value: two quick taps must see each other
      const from = shownSignal(localSignal(key), server);
      const change = feedbackChange(from, toggledSignal(from, chosen));
      if (!change) return;

      // A newer change makes the title's last Undo stale: close it, so it can't wipe this one
      const stale = undoToast(key);
      if (stale !== undefined) dismiss(stale);

      // The title page's "Why it's for you" goes with negative feedback, and comes back with Undo
      void apply(change.to, change.request, 'Couldn’t save your feedback on').then((saved) => {
        if (saved) void queryClient.invalidateQueries({ queryKey: titleKeys.detail(mediaType, tmdbId) });
      });

      const copy = change.dismisses ? dismissToast(change.to, title) : null;
      if (!copy) return;
      let id = 0;
      id = toast({
        tone: 'info',
        icon: change.to ? TOAST_ICON[change.to] : undefined,
        title: copy.title,
        body: copy.body,
        durationMs: UNDO_MS,
        onClose: () => clearUndoToast(key, id),
        action: {
          label: 'Undo',
          onClick: () => {
            // Only while this change still stands (a failed save has already put the old signal back)
            if (localSignal(key) !== change.to) return;
            void apply(change.from, change.undo, 'Couldn’t undo your feedback on').then((saved) => {
              if (!saved) return;
              // A poll may have dropped the card meanwhile: read /home again to bring it back
              void queryClient.invalidateQueries({ queryKey: recKeys.home });
              void queryClient.invalidateQueries({ queryKey: titleKeys.detail(mediaType, tmdbId) });
            });
          },
        },
      });
      setUndoToast(key, id);
    },
    [key, server, apply, dismiss, queryClient, mediaType, tmdbId, title, toast]
  );

  return useMemo(
    () => ({
      target,
      signal,
      liked: signal === 'like',
      disliked: signal === 'dislike',
      dismissed: isDismissing(signal),
      toggle,
    }),
    [target, signal, toggle]
  );
}
