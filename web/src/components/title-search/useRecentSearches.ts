'use client';

import { useSyncExternalStore } from 'react';
import { recentSearches, serverRecentSearches, subscribeRecentSearches } from '@/lib/title-search/recent';

/** Recent searches, newest first; none on the server and in the first render. */
export function useRecentSearches(): readonly string[] {
  return useSyncExternalStore(subscribeRecentSearches, recentSearches, serverRecentSearches);
}
