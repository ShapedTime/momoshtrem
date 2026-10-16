'use client';

import { useMemo } from 'react';
import { useQuery } from '@tanstack/react-query';
import { DEBOUNCE_DELAY } from '@/config/ui';
import { useDebounce } from '@/hooks/useDebounce';
import { usePendingRemovals } from '@/hooks/useUndoableRemoval';
import { assignmentJobsQuery, libraryQuery, torrentsSummaryQuery, traktIntegrationQuery } from '@/lib/api/endpoints';
import { visibleItems } from '@/lib/library/pending';
import { getSheet } from '@/lib/sheets/registry';
import { paletteActions } from '@/lib/title-search/actions';
import { cleanQuery } from '@/lib/title-search/text';
import { useRecentSearches } from './useRecentSearches';
import { useTitleSearch } from './useTitleSearch';

/** Rows the palette shows before you type (search.js `palBody`). */
const RECENT_QUERIES = 2;
const RECENT_TITLES = 4;
/** How many `/library?status=incomplete` titles are looked at for Find actions. */
const INCOMPLETE_PAGE = 20;

/**
 * Everything the ⌘K palette lists, fetched only while it's open: the
 * search (TMDB plus your library), your recent searches and titles, and
 * what its actions need (open mapping jobs, titles lacking a file, the
 * torrent counts, the library's size and the Trakt integration).
 */
export function usePaletteData(rawQuery: string, open: boolean) {
  const query = cleanQuery(useDebounce(rawQuery, DEBOUNCE_DELAY)) ?? '';
  const typing = rawQuery.trim() !== '';
  const search = useTitleSearch(typing ? query : '', { enabled: open });
  const pending = usePendingRemovals();
  const recent = useRecentSearches();

  const latest = useQuery({ ...libraryQuery({ sort: 'added', page_size: RECENT_TITLES }), enabled: open });
  const incomplete = useQuery({ ...libraryQuery({ status: 'incomplete', page_size: INCOMPLETE_PAGE }), enabled: open });
  const jobs = useQuery({ ...assignmentJobsQuery('open'), enabled: open });
  const summary = useQuery({ ...torrentsSummaryQuery(), enabled: open, refetchInterval: false, retry: false });
  const trakt = useQuery({ ...traktIntegrationQuery(), enabled: open, refetchInterval: false, retry: false });

  const actions = useMemo(
    () =>
      paletteActions({
        jobs: jobs.data?.items ?? [],
        incomplete: visibleItems(incomplete.data?.items ?? [], pending),
        torrentCounts: summary.data?.counts ?? null,
        libraryTotal: latest.data?.summary.total ?? null,
        torrentSheet: getSheet('torrent') !== undefined,
        trakt: trakt.data ?? null,
      }),
    [jobs.data, incomplete.data, pending, summary.data, latest.data, trakt.data]
  );

  const recentTitles = useMemo(() => visibleItems(latest.data?.items ?? [], pending).slice(0, RECENT_TITLES), [latest.data, pending]);

  return {
    typing,
    shown: search.shown,
    recentQueries: recent.slice(0, RECENT_QUERIES),
    recentTitles,
    actions,
  };
}
