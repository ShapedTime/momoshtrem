'use client';

import { useCallback } from 'react';
import { TriangleAlert } from 'lucide-react';
import { useConfirm } from '@/components/ui';
import type { LibraryItem } from '@/lib/api/contract';
import { apiKeepalive } from '@/lib/api/client';
import { libraryKeys, removeTitleUrl, titleKeys, titlesApi } from '@/lib/api/endpoints';
import { removeCopy } from '@/lib/library/display';
import { libraryRemovalKey } from '@/lib/library/pending';
import { useUndoableRemoval } from './useUndoableRemoval';

/**
 * "Remove from library…": confirm what goes and what stays (with "Also
 * delete downloaded data", ticked, when there's data), then remove with a
 * 10 s Undo. The DELETE goes out when the Undo toast closes.
 */
export function useLibraryRemove() {
  const confirm = useConfirm();
  const removeLater = useUndoableRemoval();

  return useCallback(
    async (item: LibraryItem) => {
      const copy = removeCopy(item);
      const description = (
        <div className="flex flex-col gap-3">
          <p>{copy.body}</p>
          {copy.warning && (
            <p className="flex items-start gap-2 font-medium text-warning">
              <TriangleAlert size={16} className="mt-0.5 shrink-0" aria-hidden="true" />
              <span>{copy.warning}</span>
            </p>
          )}
        </div>
      );
      const base = { title: copy.title, description, confirmLabel: 'Remove' };

      let deleteData: boolean | undefined;
      if (copy.checkbox) {
        const answer = await confirm({ ...base, checkbox: { label: copy.checkbox, defaultChecked: true } });
        if (!answer.confirmed) return;
        deleteData = answer.checked;
      } else if (!(await confirm(base))) {
        return;
      }

      const options = { deleteData };
      removeLater({
        key: libraryRemovalKey(item),
        label: item.title,
        payload: item,
        commit: () => (item.media_type === 'movie' ? titlesApi.removeMovie(item.id, options) : titlesApi.removeShow(item.id, options)),
        keepalive: () => apiKeepalive(removeTitleUrl(item.media_type, item.id, options), 'DELETE'),
        invalidate: [libraryKeys.all, titleKeys.all],
      });
    },
    [confirm, removeLater]
  );
}
