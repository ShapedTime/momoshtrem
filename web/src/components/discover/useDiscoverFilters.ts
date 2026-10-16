'use client';

import { useCallback, useMemo } from 'react';
import { useQueryStates } from 'nuqs';
import { type DiscoverFilters, normalizeFilters } from '@/lib/discover/filters';
import { moodMatches } from '@/lib/discover/moods';
import { discoverParsers } from '@/lib/discover/url';

/**
 * Discover's filters, kept in the URL by nuqs. Changes replace the history
 * entry (desktop applies live, so Back leaves Discover instead of stepping
 * through every chip) and don't scroll. A mood stays in the URL only while
 * the filters still equal its preset.
 */
export function useDiscoverFilters() {
  const [raw, setRaw] = useQueryStates(discoverParsers, { history: 'replace', scroll: false });
  const filters = useMemo(() => normalizeFilters({ ...raw, genres: [...raw.genres] }), [raw]);

  const setFilters = useCallback(
    (next: DiscoverFilters) => {
      const normalized = normalizeFilters(next);
      const mood = normalized.mood && moodMatches(normalized.mood, normalized) ? normalized.mood : null;
      void setRaw({ ...normalized, mood });
    },
    [setRaw]
  );

  return [filters, setFilters] as const;
}
