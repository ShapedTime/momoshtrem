'use client';

import { useState, useEffect, useCallback, useRef } from 'react';
import type { RecentlyAiredEpisode, RecentlyAiredResponse } from '@/types/momoshtrem';

// ============================================================================
// Constants
// ============================================================================

const MAX_POLL_DURATION_MS = 5 * 60 * 1000; // 5 minutes max polling time
const POLL_INTERVAL_MS = 2000;

// ============================================================================
// Types
// ============================================================================

interface RecentlyAiredState {
  episodes: RecentlyAiredEpisode[];
  lastSyncTime: string | null;
  syncStatus: RecentlyAiredResponse['sync_status'] | null;
  isLoading: boolean;
  error: string | null;
}

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

// ============================================================================
// useRecentlyAired Hook
// ============================================================================

/**
 * Hook for fetching recently aired episodes from library shows.
 * @param lookbackDays Number of days to look back (default: 30)
 */
export function useRecentlyAired(lookbackDays = 30): UseRecentlyAiredResult {
  const [state, setState] = useState<RecentlyAiredState>({
    episodes: [],
    lastSyncTime: null,
    syncStatus: null,
    isLoading: true,
    error: null,
  });
  const [isSyncing, setIsSyncing] = useState(false);
  const pollIntervalRef = useRef<NodeJS.Timeout | null>(null);

  const fetchData = useCallback(async () => {
    setState((prev) => ({ ...prev, isLoading: true, error: null }));

    try {
      const response = await fetch(`/api/library/recently-aired?lookback_days=${lookbackDays}`);

      if (!response.ok) {
        const data = await response.json().catch(() => ({}));
        throw new Error(data.error || 'Failed to fetch recently aired episodes');
      }

      const data: RecentlyAiredResponse = await response.json();

      setState({
        episodes: data.episodes || [],
        lastSyncTime: data.last_sync_time || null,
        syncStatus: data.sync_status,
        isLoading: false,
        error: null,
      });
    } catch (error) {
      setState((prev) => ({
        ...prev,
        isLoading: false,
        error: error instanceof Error ? error.message : 'Failed to fetch recently aired episodes',
      }));
    }
  }, [lookbackDays]);

  const triggerSync = useCallback(async () => {
    setIsSyncing(true);

    try {
      const response = await fetch('/api/library/sync-air-dates', {
        method: 'POST',
      });

      if (!response.ok) {
        const data = await response.json().catch(() => ({}));
        throw new Error(data.error || 'Failed to trigger sync');
      }

      // Poll for completion with timeout
      const pollStartTime = Date.now();
      pollIntervalRef.current = setInterval(async () => {
        // Check for timeout
        if (Date.now() - pollStartTime > MAX_POLL_DURATION_MS) {
          if (pollIntervalRef.current) {
            clearInterval(pollIntervalRef.current);
            pollIntervalRef.current = null;
          }
          setIsSyncing(false);
          setState((prev) => ({
            ...prev,
            error: 'Sync timed out. Please try again.',
          }));
          return;
        }

        try {
          const statusResponse = await fetch(`/api/library/recently-aired?lookback_days=${lookbackDays}`);
          if (statusResponse.ok) {
            const data: RecentlyAiredResponse = await statusResponse.json();

            if (data.sync_status !== 'in_progress') {
              // Sync complete
              if (pollIntervalRef.current) {
                clearInterval(pollIntervalRef.current);
                pollIntervalRef.current = null;
              }
              setIsSyncing(false);

              setState({
                episodes: data.episodes || [],
                lastSyncTime: data.last_sync_time || null,
                syncStatus: data.sync_status,
                isLoading: false,
                error: null,
              });
            }
          }
        } catch {
          // Ignore poll errors, keep waiting
        }
      }, POLL_INTERVAL_MS);
    } catch (error) {
      setIsSyncing(false);
      setState((prev) => ({
        ...prev,
        error: error instanceof Error ? error.message : 'Failed to trigger sync',
      }));
    }
  }, [lookbackDays]);

  // Cleanup poll interval on unmount
  useEffect(() => {
    return () => {
      if (pollIntervalRef.current) {
        clearInterval(pollIntervalRef.current);
      }
    };
  }, []);

  // Fetch on mount
  useEffect(() => {
    fetchData();
  }, [fetchData]);

  return {
    episodes: state.episodes,
    lastSyncTime: state.lastSyncTime,
    syncStatus: state.syncStatus,
    isLoading: state.isLoading,
    error: state.error,
    refresh: fetchData,
    triggerSync,
    isSyncing,
  };
}
