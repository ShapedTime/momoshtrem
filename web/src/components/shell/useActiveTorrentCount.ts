'use client';

import { useQuery } from '@tanstack/react-query';
import { legacyTorrentsQuery } from '@/lib/api/endpoints/torrents';

/**
 * Torrents still downloading, for the Activity badge. Shares the torrents
 * query with the Activity page (#7 moves it to GET /torrents/summary).
 */
export function useActiveTorrentCount(): number {
  const { data } = useQuery({ ...legacyTorrentsQuery(15_000), retry: false });
  return data?.torrents.filter((torrent) => !torrent.is_paused && torrent.progress < 1).length ?? 0;
}
