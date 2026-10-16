'use client';

import { useCallback, useMemo } from 'react';
import { keepPreviousData, useQuery } from '@tanstack/react-query';
import type { LibraryItem, LibraryRef, MediaType } from '@/lib/api/contract';
import { libraryIds, libraryLookupQuery, MAX_LIBRARY_IDS } from '@/lib/api/endpoints';
import { visibleItems } from '@/lib/library/pending';
import { usePendingRemovals } from './useUndoableRemoval';

interface LibraryLookupOptions {
  /**
   * While a new set of titles loads, answer from the last set's lookup
   * (`isPlaceholderData`) instead of from nothing. Ask's set grows with each
   * answered turn, and a title added a moment ago mustn't show Add again.
   */
  keepPrevious?: boolean;
}

/**
 * Which of these TMDB titles are in your library, for TMDB cards (Discover,
 * Search, the palette, Ask): one `/library?ids=` call per set of titles (a
 * page of results), cached by the ids. Titles waiting out a removal's Undo
 * window count as gone. Pass at most 200 refs; more are ignored.
 */
export function useLibraryLookup(refs: LibraryRef[], { keepPrevious = false }: LibraryLookupOptions = {}) {
  const ids = libraryIds(refs.slice(0, MAX_LIBRARY_IDS));
  const query = useQuery({ ...libraryLookupQuery(ids), placeholderData: keepPrevious ? keepPreviousData : undefined });
  const pending = usePendingRemovals();

  const byKey = useMemo(() => {
    const items = visibleItems(query.data?.items ?? [], pending);
    return new Map(items.map((item) => [`${item.media_type}:${item.tmdb_id}`, item]));
  }, [query.data, pending]);

  /** The library item for a TMDB title, or undefined when it isn't in the library (or isn't known yet). */
  const get = useCallback((mediaType: MediaType, tmdbId: number): LibraryItem | undefined => byKey.get(`${mediaType}:${tmdbId}`), [byKey]);

  return { get, isPending: ids !== '' && query.isPending, isPlaceholderData: query.isPlaceholderData, error: query.error };
}
