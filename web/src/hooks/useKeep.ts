'use client';

import { useMutation, useQueryClient } from '@tanstack/react-query';
import { Pin, PinOff } from 'lucide-react';
import { useConfirm, useToast } from '@/components/ui';
import type { Download, DownloadCreate, DownloadPatch, Keep } from '@/lib/api/contract';
import { downloadKeys, downloadsApi, libraryKeys, settingsKeys, titleKeys, torrentKeys } from '@/lib/api/endpoints';
import { stopKeepingCopy } from '@/lib/title/keep';
import { changedToast, keptToast } from '@/lib/title/keep-sheet';

/**
 * Download & keep on title pages (#206): Keep (POST /downloads), Change
 * duration (PATCH, kept downloads only) and Stop keeping (DELETE, confirmed).
 * Each refetches the titles and their seasons (`keep`, the stage), the
 * library, the torrents (`wanted`, `holds`), the downloads and the storage
 * usage before it settles, then says what happened.
 *
 * `keep` and `change` reject with the ApiError, so the sheet can show a 409
 * `storage_full` or 422 `not_downloadable` in place; `stop` toasts its own error.
 */
export function useKeep() {
  const queryClient = useQueryClient();
  const { toast } = useToast();
  const confirm = useConfirm();

  const refetch = () =>
    Promise.all(
      [titleKeys.all, libraryKeys.all, torrentKeys.all, downloadKeys.all, settingsKeys.storage].map((queryKey) =>
        queryClient.invalidateQueries({ queryKey })
      )
    );

  const create = useMutation({ mutationFn: (request: DownloadCreate) => downloadsApi.create(request), onSuccess: refetch });
  const change = useMutation({ mutationFn: ({ id, patch }: { id: number; patch: DownloadPatch }) => downloadsApi.update(id, patch), onSuccess: refetch });
  const stop = useMutation({ mutationFn: (id: number) => downloadsApi.remove(id), onSuccess: refetch });

  return {
    /** Resolves the download (a new one, one already kept, or a smart pick turned kept). */
    keep: async (request: DownloadCreate, name: string): Promise<Download> => {
      const download = await create.mutateAsync(request);
      toast({ tone: 'ready', icon: Pin, ...keptToast(download, name) });
      return download;
    },
    keeping: create.isPending,
    change: async (id: number, patch: DownloadPatch, name: string): Promise<Download> => {
      const download = await change.mutateAsync({ id, patch });
      toast({ tone: 'ready', icon: Pin, ...changedToast(download, name) });
      return download;
    },
    changing: change.isPending,
    /** Confirms first, saying what goes and what stays. Resolves true once it's released. */
    stop: async (keep: Pick<Keep, 'download_id' | 'reason'>, name: string): Promise<boolean> => {
      const copy = stopKeepingCopy(keep.reason, name);
      if (!(await confirm(copy.confirm))) return false;
      try {
        await stop.mutateAsync(keep.download_id);
        toast({ tone: 'info', icon: PinOff, title: copy.title, body: copy.body });
        return true;
      } catch (error) {
        toast({ tone: 'error', title: copy.failed, body: error instanceof Error ? error.message : undefined });
        return false;
      }
    },
    stopping: stop.isPending,
  };
}
