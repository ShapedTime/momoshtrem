'use client';

import { useSyncExternalStore } from 'react';
import type { TorrentSearchContext } from '@/types/jackett';

/**
 * Which search the legacy TorrentSearchModal shows, shared by the page and
 * the URL-driven sheets (which render outside the page), until the torrent
 * search sheet (#6) replaces it. Open it after closing any sheet: a sheet
 * traps focus, so the modal must not sit on top of one.
 */
let current: TorrentSearchContext | null = null;
const listeners = new Set<() => void>();

function set(next: TorrentSearchContext | null) {
  current = next;
  for (const listener of listeners) listener();
}

export const openLegacySearch = (context: TorrentSearchContext) => set(context);
export const closeLegacySearch = () => set(null);

export function useLegacySearch(): TorrentSearchContext | null {
  return useSyncExternalStore(
    (listener) => {
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
    () => current,
    () => null
  );
}
