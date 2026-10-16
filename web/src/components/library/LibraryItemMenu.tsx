'use client';

import { Activity, Copy, Ellipsis, EllipsisVertical, Info, ListChecks, type LucideIcon, Search, Trash2 } from 'lucide-react';
import { ActionMenu, type ActionMenuItem, IconButton, useCopy } from '@/components/ui';
import type { LibraryItem } from '@/lib/api/contract';
import { librarySize, statusLine } from '@/lib/library/display';
import { type LibraryAction, libraryActions } from '@/lib/library/actions';
import { getSheet } from '@/lib/sheets/registry';
import { useSheet } from '@/lib/sheets/useSheet';
import { formatNumber } from '@/lib/title/format';

const ICONS: Record<LibraryAction['kind'], LucideIcon> = {
  open: Info,
  review: ListChecks,
  find: Search,
  copy: Copy,
  download: Activity,
  remove: Trash2,
};

/** The phone sheet's line under the title: "28/38 ready · 37.8 GB". */
function facts(item: LibraryItem): string {
  const line = statusLine(item);
  const state =
    line.kind === 'episodes'
      ? `${formatNumber(line.ready)}/${formatNumber(line.total)} ready`
      : line.kind === 'needs_torrent'
        ? 'Needs a torrent'
        : line.kind === 'downloading'
          ? `Downloading ${line.pct}%`
          : 'Ready';
  return item.size_bytes > 0 ? `${state} · ${librarySize(item.size_bytes)}` : state;
}

/**
 * A library title's "…" menu (library.js): a popover on desktop, a sheet on
 * phone. `card` is the glass button on the poster, `row` the list's ⋮.
 */
export function LibraryItemMenu({ item, onRemove, variant }: { item: LibraryItem; onRemove: (item: LibraryItem) => void; variant: 'card' | 'row' }) {
  const copy = useCopy();
  const sheet = useSheet();
  const actions = libraryActions(item, { torrentSheet: getSheet('torrent') !== undefined });

  const items = actions.map((action): ActionMenuItem => {
    const base = { key: action.kind, label: action.label, icon: ICONS[action.kind] };
    switch (action.kind) {
      case 'open':
      case 'download':
        return { ...base, href: action.href };
      case 'review':
        return { ...base, href: action.href, hint: action.hint };
      case 'find': {
        const { target } = action;
        return target.kind === 'page'
          ? { ...base, hint: action.hint, href: target.href }
          : { ...base, hint: action.hint, onSelect: () => sheet.open('torrent', target.params) };
      }
      case 'copy':
        return { ...base, onSelect: () => void copy(action.url, 'Paste it into Infuse › Add files › WebDAV') };
      case 'remove':
        return { ...base, danger: true, separator: true, onSelect: () => onRemove(item) };
    }
  });

  return (
    <ActionMenu items={items} label="More actions" sheetTitle={item.title} sheetDescription={facts(item)}>
      <IconButton aria-label={`${item.title}: more actions`} size="sm" className={variant === 'row' ? 'lrow-more' : undefined}>
        {variant === 'card' ? <Ellipsis size={18} aria-hidden="true" /> : <EllipsisVertical size={18} aria-hidden="true" />}
      </IconButton>
    </ActionMenu>
  );
}
