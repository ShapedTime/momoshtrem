'use client';

import { useCallback, useMemo, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { useLibraryLookup } from '@/hooks/useLibraryLookup';
import { usePendingRemovals } from '@/hooks/useUndoableRemoval';
import { libraryQuery, titleSearchQuery } from '@/lib/api/endpoints';
import { visibleItems } from '@/lib/library/pending';
import { type SearchSections, searchSections } from '@/lib/title-search/results';

/** What a search shows: the sections for `query`, and what failed on the way. */
export interface ShownSearch {
  query: string;
  sections: SearchSections;
  /** TMDB failed: only your library's matches are in. */
  tmdbError: Error | null;
  /** /library failed: TMDB's titles are in, but some may be yours. */
  libraryError: Error | null;
}

/**
 * One search for the page and the palette: TMDB's titles
 * (`/api/title-search`), your library's (`/library?q=`), and which TMDB
 * titles you have (`/library?ids=`). Sections change only once all three
 * have answered, so a library title never shows in the TMDB grid first and
 * then jumps; until then the last search's sections stay. A blank query
 * asks nothing and shows nothing.
 */
export function useTitleSearch(query: string, { enabled = true }: { enabled?: boolean } = {}) {
  const active = enabled && query !== '';
  const tmdb = useQuery({ ...titleSearchQuery(query), enabled: active });
  const library = useQuery({ ...libraryQuery({ q: query }), enabled: active });
  const pending = usePendingRemovals();
  const lookup = useLibraryLookup(active && !tmdb.isPlaceholderData ? (tmdb.data?.items ?? []) : []);

  const tmdbSettled = tmdb.isError || (tmdb.data !== undefined && !tmdb.isPlaceholderData);
  const librarySettled = library.isError || (library.data !== undefined && !library.isPlaceholderData);
  const settled = active && tmdbSettled && librarySettled && !lookup.isPending;

  const tmdbItems = tmdb.isError ? undefined : tmdb.data?.items;
  const libraryItems = library.isError ? undefined : library.data?.items;
  const sections = useMemo(
    () => searchSections(query, tmdbItems ?? [], visibleItems(libraryItems ?? [], pending), lookup.get),
    [query, tmdbItems, libraryItems, pending, lookup.get]
  );

  const [shown, setShown] = useState<ShownSearch | null>(null);
  if (!active && shown !== null) setShown(null);
  else if (settled && shown?.sections !== sections) {
    setShown({ query, sections, tmdbError: tmdb.isError ? tmdb.error : null, libraryError: library.isError ? library.error : null });
  }

  const { refetch: refetchTmdb } = tmdb;
  const { refetch: refetchLibrary } = library;
  const retry = useCallback(() => {
    void refetchTmdb();
    void refetchLibrary();
  }, [refetchTmdb, refetchLibrary]);

  return {
    shown: active ? shown : null,
    /** Waiting for the first answer to this query. */
    loading: active && !settled,
    retrying: tmdb.isRefetching || library.isRefetching,
    retry,
  };
}
