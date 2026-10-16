'use client';

import { useCallback, useSyncExternalStore } from 'react';
import { type QueryKey, useQueryClient } from '@tanstack/react-query';
import { type LucideIcon, Trash2 } from 'lucide-react';
import { useToast } from '@/components/ui';
import {
  addPendingRemoval,
  commitPendingRemoval,
  type PendingRemoval,
  pendingRemovals,
  releasePendingRemoval,
  subscribePendingRemovals,
  undoPendingRemoval,
} from '@/lib/undo/removals';

/** How long Undo is offered (HANDOFF §4.4). */
export const UNDO_MS = 10_000;
/**
 * How long an uncertain removal stays hidden while the server may still be at
 * it (a torrent delete waits up to `add_timeout`, 60 s, for an add in flight).
 */
export const UNCERTAIN_HOLD_MS = 60_000;

const NONE: ReadonlyMap<string, PendingRemoval> = new Map();

/** The removals waiting out their Undo window; lists hide their items. */
export function usePendingRemovals(): ReadonlyMap<string, PendingRemoval> {
  return useSyncExternalStore(subscribePendingRemovals, pendingRemovals, () => NONE);
}

export interface UndoableRemoval extends PendingRemoval {
  /** Queries to refresh once it's sent (or failed), e.g. `libraryKeys.all`. */
  invalidate?: readonly QueryKey[];
  /**
   * The toasts' words when "Removed {label}" isn't what happens (Stop keeping a
   * download): the Undo toast's title and line, and the failure's title.
   */
  copy?: { title: string; body: string; failed: string; icon?: LucideIcon };
}

/**
 * Remove with a 10 s Undo (a delayed commit). The item hides at once and the
 * toast offers Undo. The removal goes out when the toast closes: on timeout,
 * on dismiss, or when another toast replaces it (one toast at a time, so a
 * replaced Undo toast commits at once), or on `pagehide` (keepalive). If it
 * fails, an error toast says so and the item comes back.
 */
export function useUndoableRemoval() {
  const { toast } = useToast();
  const queryClient = useQueryClient();

  return useCallback(
    (removal: UndoableRemoval) => {
      const { invalidate = [], copy, ...pending } = removal;
      // Inactive caches are dropped rather than marked stale: a page opened later
      // must not show the removed item from its old cache while it refetches.
      const refresh = () =>
        Promise.all(
          invalidate.map((queryKey) => {
            queryClient.removeQueries({ queryKey, type: 'inactive' });
            return queryClient.invalidateQueries({ queryKey });
          })
        );

      addPendingRemoval(pending);
      toast({
        tone: 'info',
        icon: copy?.icon ?? Trash2,
        title: copy?.title ?? `Removed ${removal.label}`,
        body: copy ? `${copy.body} Undo within 10 seconds.` : 'Undo within 10 seconds',
        durationMs: UNDO_MS,
        action: { label: 'Undo', onClick: () => undoPendingRemoval(removal.key) },
        onClose: async () => {
          const outcome = await commitPendingRemoval(removal.key);
          if (outcome.result === 'committed') {
            await refresh().catch(() => undefined);
            releasePendingRemoval(removal.key);
          } else if (outcome.result === 'uncertain') {
            // The proxy stopped waiting, not the server: say so, refetch, and keep it hidden a while
            toast({ tone: 'info', title: `Still removing ${removal.label}…`, body: 'momoshtrem is taking its time. It drops off the list once it’s done.' });
            await refresh().catch(() => undefined);
            window.setTimeout(() => {
              void refresh()
                .catch(() => undefined)
                .finally(() => releasePendingRemoval(removal.key));
            }, UNCERTAIN_HOLD_MS);
          } else if (outcome.result === 'failed') {
            const cause = outcome.error instanceof Error ? outcome.error.message : '';
            toast({ tone: 'error', title: copy?.failed ?? `Couldn’t remove ${removal.label}`, body: [cause, 'It’s back where it was.'].filter(Boolean).join(' · ') });
            await refresh().catch(() => undefined);
          }
        },
      });
    },
    [queryClient, toast]
  );
}
