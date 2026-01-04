'use client';

import { useState, useEffect, useCallback, useRef } from 'react';
import type { TorrentStatus } from '@/types/torrent';

// ============================================================================
// Types
// ============================================================================

interface TorrentsState {
  torrents: TorrentStatus[];
  isLoading: boolean;
  error: string | null;
}

interface UseTorrentsOptions {
  /** Enable auto-refresh. Defaults to false. */
  autoRefresh?: boolean;
  /** Refresh interval in milliseconds. Defaults to 5000. */
  refreshInterval?: number;
}

interface UseTorrentsResult {
  torrents: TorrentStatus[];
  isLoading: boolean;
  error: string | null;
  refresh: () => Promise<void>;
  pauseTorrent: (hash: string) => Promise<boolean>;
  resumeTorrent: (hash: string) => Promise<boolean>;
  removeTorrent: (hash: string, deleteData?: boolean) => Promise<boolean>;
  /** Map of info_hash to TorrentStatus for quick lookups */
  torrentMap: Map<string, TorrentStatus>;
}

// ============================================================================
// useTorrents Hook
// ============================================================================

/**
 * Hook for fetching and managing active torrents.
 * Supports auto-refresh for live status updates.
 */
export function useTorrents(options: UseTorrentsOptions = {}): UseTorrentsResult {
  const { autoRefresh = false, refreshInterval = 5000 } = options;

  const [state, setState] = useState<TorrentsState>({
    torrents: [],
    isLoading: true,
    error: null,
  });

  const intervalRef = useRef<NodeJS.Timeout | null>(null);
  const mountedRef = useRef(true);

  const fetchTorrents = useCallback(async () => {
    if (!mountedRef.current) return;

    // Don't set loading state on refresh to avoid flickering
    const isInitialLoad = state.torrents.length === 0;
    if (isInitialLoad) {
      setState((prev) => ({ ...prev, isLoading: true, error: null }));
    }

    try {
      const response = await fetch('/api/torrents');

      if (!response.ok) {
        throw new Error('Failed to fetch torrents');
      }

      const data = await response.json();

      if (mountedRef.current) {
        setState({
          torrents: data.torrents || [],
          isLoading: false,
          error: null,
        });
      }
    } catch (error) {
      if (mountedRef.current) {
        setState((prev) => ({
          ...prev,
          isLoading: false,
          error: error instanceof Error ? error.message : 'Failed to fetch torrents',
        }));
      }
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const pauseTorrent = useCallback(async (hash: string): Promise<boolean> => {
    try {
      const response = await fetch(`/api/torrents/${hash}/pause`, {
        method: 'POST',
      });

      if (!response.ok) {
        const data = await response.json().catch(() => ({}));
        throw new Error(data.error || 'Failed to pause torrent');
      }

      // Optimistically update the local state
      setState((prev) => ({
        ...prev,
        torrents: prev.torrents.map((t) =>
          t.info_hash === hash ? { ...t, is_paused: true } : t
        ),
      }));

      return true;
    } catch (error) {
      console.error('Failed to pause torrent:', error);
      return false;
    }
  }, []);

  const resumeTorrent = useCallback(async (hash: string): Promise<boolean> => {
    try {
      const response = await fetch(`/api/torrents/${hash}/resume`, {
        method: 'POST',
      });

      if (!response.ok) {
        const data = await response.json().catch(() => ({}));
        throw new Error(data.error || 'Failed to resume torrent');
      }

      // Optimistically update the local state
      setState((prev) => ({
        ...prev,
        torrents: prev.torrents.map((t) =>
          t.info_hash === hash ? { ...t, is_paused: false } : t
        ),
      }));

      return true;
    } catch (error) {
      console.error('Failed to resume torrent:', error);
      return false;
    }
  }, []);

  const removeTorrent = useCallback(async (hash: string, deleteData = false): Promise<boolean> => {
    try {
      const query = deleteData ? '?delete_data=true' : '';
      const response = await fetch(`/api/torrents/${hash}${query}`, {
        method: 'DELETE',
      });

      if (!response.ok) {
        const data = await response.json().catch(() => ({}));
        throw new Error(data.error || 'Failed to remove torrent');
      }

      // Optimistically update the local state
      setState((prev) => ({
        ...prev,
        torrents: prev.torrents.filter((t) => t.info_hash !== hash),
      }));

      return true;
    } catch (error) {
      console.error('Failed to remove torrent:', error);
      return false;
    }
  }, []);

  // Build a map for quick lookups
  const torrentMap = new Map(
    state.torrents.map((t) => [t.info_hash, t])
  );

  // Initial fetch
  useEffect(() => {
    mountedRef.current = true;
    fetchTorrents();

    return () => {
      mountedRef.current = false;
    };
  }, [fetchTorrents]);

  // Auto-refresh setup
  useEffect(() => {
    if (autoRefresh) {
      intervalRef.current = setInterval(fetchTorrents, refreshInterval);
    }

    return () => {
      if (intervalRef.current) {
        clearInterval(intervalRef.current);
        intervalRef.current = null;
      }
    };
  }, [autoRefresh, refreshInterval, fetchTorrents]);

  return {
    torrents: state.torrents,
    isLoading: state.isLoading,
    error: state.error,
    refresh: fetchTorrents,
    pauseTorrent,
    resumeTorrent,
    removeTorrent,
    torrentMap,
  };
}

// ============================================================================
// useTorrentStatus Hook
// ============================================================================

interface TorrentStatusState {
  status: TorrentStatus | null;
  isLoading: boolean;
  error: string | null;
}

interface UseTorrentStatusResult extends TorrentStatusState {
  refresh: () => Promise<void>;
}

/**
 * Hook for fetching status of a single torrent by info_hash.
 */
export function useTorrentStatus(
  infoHash: string | null | undefined
): UseTorrentStatusResult {
  const [state, setState] = useState<TorrentStatusState>({
    status: null,
    isLoading: !!infoHash,
    error: null,
  });

  const fetchStatus = useCallback(async () => {
    if (!infoHash) {
      setState({
        status: null,
        isLoading: false,
        error: null,
      });
      return;
    }

    setState((prev) => ({ ...prev, isLoading: true, error: null }));

    try {
      const response = await fetch(`/api/torrents/${infoHash}`);

      if (!response.ok) {
        if (response.status === 404) {
          // Torrent not found is not an error, just means it's not active
          setState({
            status: null,
            isLoading: false,
            error: null,
          });
          return;
        }
        throw new Error('Failed to fetch torrent status');
      }

      const data = await response.json();

      setState({
        status: data,
        isLoading: false,
        error: null,
      });
    } catch (error) {
      setState((prev) => ({
        ...prev,
        isLoading: false,
        error: error instanceof Error ? error.message : 'Failed to fetch status',
      }));
    }
  }, [infoHash]);

  useEffect(() => {
    fetchStatus();
  }, [fetchStatus]);

  return {
    ...state,
    refresh: fetchStatus,
  };
}
