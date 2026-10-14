'use client';

import { useQuery } from '@tanstack/react-query';
import { apiJson } from '@/lib/api/client';
import type { TorrentDebugInfo } from '@/types/torrent-debug';

const POLL_INTERVAL = 1500; // ms

interface UseTorrentDebugResult {
  data: TorrentDebugInfo | null;
  error: string | null;
  isLoading: boolean;
}

/**
 * Torrent debug data, polled every 1.5 s while the tab is visible.
 */
export function useTorrentDebug(infoHash: string): UseTorrentDebugResult {
  const query = useQuery({
    queryKey: ['torrents', infoHash, 'debug'],
    queryFn: () => apiJson<TorrentDebugInfo>(`/api/torrents/${infoHash}/debug`),
    refetchInterval: POLL_INTERVAL,
  });
  return {
    data: query.data ?? null,
    error: query.error ? query.error.message || 'Failed to fetch debug info' : null,
    isLoading: query.isPending,
  };
}
