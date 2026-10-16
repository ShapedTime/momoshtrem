import type { LibraryItem } from '@/lib/api/contract';
import type { PendingRemoval } from '@/lib/undo/removals';

/** A library title's key in the pending-removals store. */
export const libraryRemovalKey = (item: Pick<LibraryItem, 'media_type' | 'id'>) => `library:${item.media_type}:${item.id}`;

function isLibraryItem(value: unknown): value is LibraryItem {
  return (
    typeof value === 'object' &&
    value !== null &&
    'media_type' in value &&
    'id' in value &&
    'availability_stage' in value &&
    'size_bytes' in value
  );
}

/** The library titles waiting out their Undo window (their payloads). */
export function pendingLibraryItems(pending: ReadonlyMap<string, PendingRemoval>): LibraryItem[] {
  return [...pending.values()].flatMap((removal) => (removal.key.startsWith('library:') && isLibraryItem(removal.payload) ? [removal.payload] : []));
}

/** Lists without the titles waiting out their Undo window. */
export function visibleItems(items: LibraryItem[], pending: ReadonlyMap<string, PendingRemoval>): LibraryItem[] {
  return pending.size === 0 ? items : items.filter((item) => !pending.has(libraryRemovalKey(item)));
}
