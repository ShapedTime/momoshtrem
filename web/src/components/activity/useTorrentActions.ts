'use client';

import { useCallback } from 'react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { Pause, Play } from 'lucide-react';
import { useConfirm, useToast } from '@/components/ui';
import { useUndoableRemoval } from '@/hooks/useUndoableRemoval';
import { apiKeepalive, isApiError } from '@/lib/api/client';
import type { TorrentDetail, TorrentListItem } from '@/lib/api/contract';
import { downloadKeys, libraryKeys, removeTorrentUrl, titleKeys, torrentKeys, torrentsApi } from '@/lib/api/endpoints';
import { torrentLabel } from '@/lib/activity/format';
import { removeTorrentCopy, torrentRemovalKey } from '@/lib/activity/remove';
import { plural } from '@/lib/title/format';

/** What a torrent action changes: the torrents, the library and title pages that show its state, and its downloads (#77). */
const AFFECTED = [torrentKeys.all, libraryKeys.all, titleKeys.all, downloadKeys.all] as const;

/** The proxy gave up after 30 s; the delete may still finish (it waits for an add in flight, up to 60 s). */
const mayStillBeRemoving = (error: unknown) => isApiError(error) && (error.status === 504 || error.code === 'timeout');

/**
 * Pause, resume, Pause all and Remove for Activity's cards and the detail page,
 * with their toasts (activity.js actions). Pause and resume write the answer
 * into the list at once, so the card moves to its section without waiting for
 * the next poll. Remove confirms, then hides the torrent for a 10 s Undo; the
 * DELETE goes out when the toast closes.
 */
export function useTorrentActions() {
  const queryClient = useQueryClient();
  const { toast } = useToast();
  const confirm = useConfirm();
  const removeLater = useUndoableRemoval();

  const apply = useCallback(
    (updated: TorrentListItem) => {
      queryClient.setQueryData<{ items: TorrentListItem[] }>(torrentKeys.list, (old) =>
        old ? { items: old.items.map((item) => (item.info_hash === updated.info_hash ? updated : item)) } : old
      );
      queryClient.setQueryData<TorrentDetail>(torrentKeys.detail(updated.info_hash), (old) => (old ? { ...old, ...updated } : old));
      return Promise.all(AFFECTED.map((queryKey) => queryClient.invalidateQueries({ queryKey })));
    },
    [queryClient]
  );

  const pause = useMutation({
    mutationFn: (torrent: TorrentListItem) => torrentsApi.pause(torrent.info_hash),
    onSuccess: (updated, torrent) => {
      void apply(updated);
      toast({
        tone: 'info',
        icon: Pause,
        title: `Paused ${torrentLabel(torrent).text}`,
        body: torrent.state === 'streaming' ? 'Infuse will stall once its buffer runs out.' : undefined,
      });
    },
    onError: (error, torrent) => toast({ tone: 'error', title: `Couldn’t pause ${torrentLabel(torrent).text}`, body: error.message }),
  });

  const resume = useMutation({
    mutationFn: (torrent: TorrentListItem) => torrentsApi.resume(torrent.info_hash),
    onSuccess: (updated, torrent) => {
      void apply(updated);
      toast({ tone: 'ready', icon: Play, title: `Resumed ${torrentLabel(torrent).text}` });
    },
    onError: (error, torrent) => toast({ tone: 'error', title: `Couldn’t resume ${torrentLabel(torrent).text}`, body: error.message }),
  });

  const pauseAll = useMutation({
    mutationFn: () => torrentsApi.pauseAll(),
    onSuccess: async ({ count }) => {
      await Promise.all(AFFECTED.map((queryKey) => queryClient.invalidateQueries({ queryKey })));
      toast({
        tone: 'info',
        icon: Pause,
        title: count > 0 ? `Paused ${plural(count, 'torrent')}` : 'Nothing to pause',
        body: count > 0 ? 'Infuse keeps playing anything already downloaded.' : undefined,
      });
    },
    onError: (error) => toast({ tone: 'error', title: 'Couldn’t pause everything', body: error.message }),
  });

  /**
   * Confirm what goes and what stays (with "Also delete downloaded data",
   * ticked, when the size is known), then remove with Undo. `beforeUndo` runs
   * once it's confirmed, before the toast: the detail page goes back to Activity.
   */
  const remove = useCallback(
    async (torrent: TorrentListItem, { beforeUndo }: { beforeUndo?: () => void } = {}) => {
      const copy = removeTorrentCopy(torrent);
      const base = { title: copy.title, description: copy.body, confirmLabel: 'Remove torrent' };
      let deleteData = false;
      if (copy.checkbox) {
        const answer = await confirm({ ...base, checkbox: { label: copy.checkbox, defaultChecked: true } });
        if (!answer.confirmed) return;
        deleteData = answer.checked;
      } else if (!(await confirm(base))) {
        return;
      }

      beforeUndo?.();
      const hash = torrent.info_hash;
      removeLater({
        key: torrentRemovalKey(hash),
        label: torrentLabel(torrent).text,
        payload: torrent,
        commit: () => torrentsApi.remove(hash, { deleteData }),
        keepalive: () => apiKeepalive(removeTorrentUrl(hash, { deleteData }), 'DELETE'),
        invalidate: AFFECTED,
        mayStillSucceed: mayStillBeRemoving,
      });
    },
    [confirm, removeLater]
  );

  const busy = (torrent: Pick<TorrentListItem, 'info_hash'>) =>
    (pause.isPending && pause.variables?.info_hash === torrent.info_hash) || (resume.isPending && resume.variables?.info_hash === torrent.info_hash);

  return {
    pause: (torrent: TorrentListItem) => pause.mutate(torrent),
    resume: (torrent: TorrentListItem) => resume.mutate(torrent),
    /** Pauses the streaming and downloading torrents. */
    pauseAll: () => pauseAll.mutate(),
    pausingAll: pauseAll.isPending,
    remove,
    /** A pause or resume of this torrent is on its way. */
    busy,
  };
}
