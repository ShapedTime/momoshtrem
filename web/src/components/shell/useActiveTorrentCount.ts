'use client';

import { useQuery } from '@tanstack/react-query';
import { torrentsSummaryQuery } from '@/lib/api/endpoints/torrents';

/** How often the badge refreshes away from Activity (which polls the same query every 5 s). */
const BADGE_POLL_MS = 15_000;

/**
 * Streaming and downloading torrents, for the Activity badge: the summary's
 * counts, shared with the Activity page. Polling pauses in hidden tabs.
 */
export function useActiveTorrentCount(): number {
  const { data } = useQuery({ ...torrentsSummaryQuery(), refetchInterval: BADGE_POLL_MS, retry: false });
  return data ? data.counts.streaming + data.counts.downloading : 0;
}
