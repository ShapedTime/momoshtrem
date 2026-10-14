'use client';

import { useState, useEffect, useCallback, useRef } from 'react';
import type { TorrentDebugInfo } from '@/types/torrent-debug';

const POLL_INTERVAL = 1500; // ms

interface UseTorrentDebugResult {
  data: TorrentDebugInfo | null;
  error: string | null;
  isLoading: boolean;
}

/**
 * Polling hook for torrent debug data.
 * Fetches debug info every 1.5 seconds.
 */
export function useTorrentDebug(infoHash: string): UseTorrentDebugResult {
  const [data, setData] = useState<TorrentDebugInfo | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const mountedRef = useRef(true);

  const fetchDebug = useCallback(async () => {
    try {
      const res = await fetch(`/api/torrents/${infoHash}/debug`);
      if (!res.ok) {
        const body = await res.json().catch(() => ({}));
        throw new Error((body as { error?: string }).error || `HTTP ${res.status}`);
      }
      const info = (await res.json()) as TorrentDebugInfo;
      if (mountedRef.current) {
        setData(info);
        setError(null);
      }
    } catch (err) {
      if (mountedRef.current) {
        setError(err instanceof Error ? err.message : 'Failed to fetch debug info');
      }
    } finally {
      if (mountedRef.current) {
        setIsLoading(false);
      }
    }
  }, [infoHash]);

  useEffect(() => {
    mountedRef.current = true;
    setIsLoading(true);
    fetchDebug();

    const interval = setInterval(fetchDebug, POLL_INTERVAL);
    return () => {
      mountedRef.current = false;
      clearInterval(interval);
    };
  }, [fetchDebug]);

  return { data, error, isLoading };
}
