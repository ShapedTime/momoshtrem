'use client';

import { useSyncExternalStore } from 'react';

const subscribe = () => () => {};

/**
 * False during server rendering and hydration, true after. Render data a
 * query may already hold on the client (but not on the server) only once
 * it's true, so the first client render matches the server's HTML (#148).
 */
export function useHydrated(): boolean {
  return useSyncExternalStore(
    subscribe,
    () => true,
    () => false
  );
}
