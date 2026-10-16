'use client';

import { useCallback, useMemo, useState } from 'react';
import { type InfiniteData, useInfiniteQuery, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import type { Release, SearchTorrentsParams, SearchTorrentsResponse } from '@/lib/api/contract';
import { assignmentJobQuery, SEARCH_POLL_MS, searchApi, searchKeys, searchTorrentPagesQuery, searchTorrentsQuery } from '@/lib/api/endpoints';
import { sameFilters, type SearchFilters } from '@/lib/search/filters';
import { inspectRequest } from '@/lib/search/inspect';
import { samePlan } from '@/lib/search/query';
import { magnetOf } from '@/lib/search/release';
import type { SourceContext } from '@/lib/source/context';
import { useDebounce } from './useDebounce';

/**
 * The torrent search sheet's results (PLAN §6 "Search"): page 1, then Load
 * more appends. While the search runs (`complete: false`) every loaded page is
 * reread each second. Changing only filters, sort or paging keeps the old
 * results up meanwhile; a new plan (scope, `q`) starts from the searching state.
 * Retry re-runs the search once with `refresh=1` and starts the pages over.
 */
export function useReleaseSearch(params: SearchTorrentsParams) {
  const queryClient = useQueryClient();
  const query = useInfiniteQuery({
    ...searchTorrentPagesQuery(params),
    placeholderData: (previous, previousQuery) => {
      const previousParams = previousQuery?.queryKey[3];
      return previousParams && samePlan(previousParams, params) ? previous : undefined;
    },
  });
  const retry = useMutation({
    mutationFn: () => searchApi.torrents({ ...params, page: 1, refresh: 1 }),
    onSuccess: (page) =>
      queryClient.setQueryData<InfiniteData<SearchTorrentsResponse, number>>(searchKeys.torrentPages(params), { pages: [page], pageParams: [1] }),
  });
  const pages = query.data?.pages;
  const items = useMemo(() => pages?.flatMap((page) => page.items) ?? [], [pages]);
  return { query, first: pages?.[0], last: pages?.at(-1), items, retry };
}

/** The Filters sheet waits this long after the last change before counting. */
const COUNT_DEBOUNCE_MS = 300;

/**
 * "Show N results" and the Filters sheet's facet counts for its draft (PLAN
 * §6: the same endpoint with the draft filters and `page_size=1`), counted a
 * moment after the last change. Until the draft differs from what's applied,
 * the results' own answer counts; meanwhile the previous count stays up.
 */
export function useFilterCount({ paramsFor, applied, draft, current, enabled }: {
  /** The search's params with these filters. */
  paramsFor: (filters: SearchFilters) => SearchTorrentsParams;
  applied: SearchFilters;
  draft: SearchFilters;
  /** The results' latest page 1. */
  current: SearchTorrentsResponse | undefined;
  enabled: boolean;
}) {
  const counted = useDebounce(draft, COUNT_DEBOUNCE_MS);
  const own = sameFilters(counted, applied);
  const query = useQuery({
    ...searchTorrentsQuery({ ...paramsFor(counted), page: 1, page_size: 1 }),
    enabled: enabled && !own,
    // A search still running keeps counting what arrives
    refetchInterval: (q) => (q.state.data?.complete === false ? SEARCH_POLL_MS : false),
  });
  const answer = own ? current : (query.data ?? current);
  return { total: answer?.total, facets: answer?.facets, counting: counted !== draft || (!own && query.isFetching) };
}

/**
 * Inspect (#93): a dry-run mapping job for one release, in a nested sheet.
 * `open` starts it (a new job, or the server's from the last hour), the job
 * is polled while it works, and Retry starts another. Closing keeps the
 * release for the sheet's exit and leaves the dry run to the server's
 * janitor: discarding it would stop the server reusing it for an hour.
 */
export function useInspect(context: SourceContext) {
  const [state, setState] = useState<{ release: Release | null; isOpen: boolean }>({ release: null, isOpen: false });
  const start = useMutation({ mutationFn: (release: Release) => searchApi.inspect(inspectRequest(context, release)) });
  const jobId = start.data?.job_id ?? null;
  const job = useQuery({ ...assignmentJobQuery(jobId ?? 0), enabled: jobId !== null && state.isOpen });
  const { mutate, reset } = start;

  const open = useCallback(
    (release: Release) => {
      reset();
      setState({ release, isOpen: true });
      mutate(release);
    },
    [mutate, reset]
  );
  const close = useCallback(() => setState((current) => ({ ...current, isOpen: false })), []);
  const retry = useCallback(() => {
    if (state.release) mutate(state.release);
  }, [mutate, state.release]);

  return { ...state, open, close, retry, start, job: jobId !== null ? job : undefined };
}

/**
 * The magnet behind Add and Copy magnet. A release without one
 * (`needs_torrent_fetch`) is resolved first; the answer is kept for the visit.
 */
export function useReleaseMagnet() {
  const [resolved, setResolved] = useState<Readonly<Record<string, string>>>({});
  const resolve = useMutation({ mutationFn: (release: Release) => searchApi.resolve({ release_id: release.id }) });
  const { mutateAsync, isPending, variables } = resolve;

  const magnetFor = useCallback(
    async (release: Release): Promise<string> => {
      const known = magnetOf(release, resolved);
      if (known) return known;
      const { magnet_uri } = await mutateAsync(release);
      setResolved((current) => ({ ...current, [release.id]: magnet_uri }));
      return magnet_uri;
    },
    [mutateAsync, resolved]
  );

  return {
    magnetFor,
    /** The magnet once known (to match Add's progress), else null. */
    knownMagnet: (release: Release) => magnetOf(release, resolved),
    isResolving: (release: Release) => isPending && variables?.id === release.id,
  };
}
