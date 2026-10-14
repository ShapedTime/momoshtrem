'use client';

import { useSearchParams } from 'next/navigation';
import { useEffect } from 'react';

/**
 * The browser tab names what the page shows (a route's metadata only knows
 * "Movie" or "Show"). Next re-applies that metadata on every navigation,
 * sheets' `?sheet=` included, so this runs again whenever the query changes.
 */
export function useDocumentTitle(name: string) {
  const searchParams = useSearchParams();
  useEffect(() => {
    document.title = `${name} · momoshtrem`;
  }, [name, searchParams]);
}
