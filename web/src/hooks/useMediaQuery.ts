'use client';

import { useSyncExternalStore } from 'react';

/**
 * Subscribe to a CSS media query. Returns `false` during server rendering
 * and hydration, then the live value.
 */
export function useMediaQuery(query: string): boolean {
  return useSyncExternalStore(
    (onChange) => {
      const mql = window.matchMedia(query);
      mql.addEventListener('change', onChange);
      return () => mql.removeEventListener('change', onChange);
    },
    () => window.matchMedia(query).matches,
    () => false
  );
}

/** Desktop layout (top bar, right-hand panels) starts at 1024 px (HANDOFF §2). */
export const DESKTOP_QUERY = '(min-width: 1024px)';
