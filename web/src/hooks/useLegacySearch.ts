'use client';

import { useEffect, useSyncExternalStore } from 'react';
import type { TorrentSearchContext } from '@/types/jackett';

/**
 * Which search the legacy TorrentSearchModal shows, shared by a title page
 * and the URL-driven sheets (which render outside the page), until the
 * torrent search sheet (#6) replaces it. Only a mounted host (the title
 * page) can show it: without one, opening does nothing, so a search never
 * waits around to pop up on the next title (and assign to the wrong one).
 */
let current: TorrentSearchContext | null = null;
let hosts = 0;
const listeners = new Set<() => void>();

function set(next: TorrentSearchContext | null) {
  current = next;
  for (const listener of listeners) listener();
}

function subscribe(listener: () => void) {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

const getSnapshot = () => current;
const getServerSnapshot = () => null;

/** Open the search on the current title page; does nothing when no page hosts it. */
export function openLegacySearch(context: TorrentSearchContext): void {
  if (hosts > 0) set(context);
}

export const closeLegacySearch = () => set(null);

/**
 * For the host component: counts it as mounted and starts and ends with no
 * search open, then returns the search to show.
 */
export function useLegacySearchHost(): TorrentSearchContext | null {
  useEffect(() => {
    hosts += 1;
    set(null);
    return () => {
      hosts -= 1;
      set(null);
    };
  }, []);
  return useSyncExternalStore(subscribe, getSnapshot, getServerSnapshot);
}
