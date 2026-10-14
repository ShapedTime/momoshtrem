'use client';

import { useCallback, useEffect, useRef, useState } from 'react';
import { queryOptions, useQuery, useQueryClient } from '@tanstack/react-query';
import { apiJson } from '@/lib/api/client';
import type { RecentlyAiredEpisode, RecentlyAiredResponse } from '@/types/momoshtrem';

// ============================================================================
// Constants
// ============================================================================

const MAX_POLL_DURATION_MS = 5 * 60 * 1000; // 5 minutes max polling time
const POLL_INTERVAL_MS = 2000;

const recentlyAiredQuery = (lookbackDays: number) =>
  queryOptions({
    queryKey: ['library', 'recently-aired', lookbackDays],
    queryFn: () => apiJson<RecentlyAiredResponse>(`/api/library/recently-aired?lookback_days=${lookbackDays}`),
  });

const sleep = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms));

// ============================================================================
// Types
// ============================================================================

interface UseRecentlyAiredResult {
  episodes: RecentlyAiredEpisode[];
  lastSyncTime: string | null;
  syncStatus: RecentlyAiredResponse['sync_status'] | null;
  isLoading: boolean;
  error: string | null;
  refresh: () => Promise<void>;
  triggerSync: () => Promise<void>;
  isSyncing: boolean;
}

const NO_EPISODES: RecentlyAiredEpisode[] = [];

// ============================================================================
// useRecentlyAired Hook
// ============================================================================

/**
 * Recently aired episodes from library shows.
 * `triggerSync` starts an air-date sync and polls until it finishes
 * (every 2 s, skipped while the tab is hidden, for at most 5 minutes).
 * @param lookbackDays Number of days to look back (default: 30)
 */
export function useRecentlyAired(lookbackDays = 30): UseRecentlyAiredResult {
  const queryClient = useQueryClient();
  const query = useQuery(recentlyAiredQuery(lookbackDays));
  const [isSyncing, setIsSyncing] = useState(false);
  const [syncError, setSyncError] = useState<string | null>(null);
  const mountedRef = useRef(true);

  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
    };
  }, []);

  const triggerSync = useCallback(async () => {
    setIsSyncing(true);
    setSyncError(null);
    try {
      await apiJson<unknown>('/api/library/sync-air-dates', { method: 'POST' });
      const startedAt = Date.now();
      while (Date.now() - startedAt < MAX_POLL_DURATION_MS) {
        await sleep(POLL_INTERVAL_MS);
        if (!mountedRef.current) return;
        if (document.hidden) continue;
        try {
          const data = await queryClient.fetchQuery({ ...recentlyAiredQuery(lookbackDays), staleTime: 0 });
          if (data.sync_status !== 'in_progress') return;
        } catch {
          // Ignore poll errors, keep waiting
        }
      }
      setSyncError('Sync timed out. Please try again.');
    } catch (error) {
      setSyncError(error instanceof Error ? error.message : 'Failed to trigger sync');
    } finally {
      if (mountedRef.current) setIsSyncing(false);
    }
  }, [lookbackDays, queryClient]);

  const { refetch } = query;
  const refresh = useCallback(async () => {
    await refetch();
  }, [refetch]);

  return {
    episodes: query.data?.episodes ?? NO_EPISODES,
    lastSyncTime: query.data?.last_sync_time ?? null,
    syncStatus: query.data?.sync_status ?? null,
    isLoading: query.isPending,
    error: syncError ?? (query.error ? query.error.message || 'Failed to fetch recently aired episodes' : null),
    refresh,
    triggerSync,
    isSyncing,
  };
}
