'use client';

import { useCallback, useMemo, useState } from 'react';
import { type InfiniteData, useInfiniteQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import type { Release, SearchTorrentsParams, SearchTorrentsResponse } from '@/lib/api/contract';
import { searchApi, searchKeys, searchTorrentPagesQuery } from '@/lib/api/endpoints';
import { samePlan } from '@/lib/search/query';
import { magnetOf } from '@/lib/search/release';

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
