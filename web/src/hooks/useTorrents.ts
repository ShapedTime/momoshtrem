'use client';

import { useState, useEffect, useCallback, useMemo } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { apiFetch, parseErrorBody } from '@/lib/api/client';
import { legacyTorrentsQuery, torrentKeys } from '@/lib/api/endpoints/torrents';
import type { TorrentListResponse, TorrentStatus } from '@/types/torrent';

// ============================================================================
// Types
// ============================================================================

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

const NO_TORRENTS: TorrentStatus[] = [];

// ============================================================================
// useTorrents Hook
// ============================================================================

/**
 * Active torrents from the shared query cache. With `autoRefresh` it polls,
 * pausing while the tab is hidden (TanStack Query). Every caller shares one
 * request and one cache entry.
 */
export function useTorrents(options: UseTorrentsOptions = {}): UseTorrentsResult {
  const { autoRefresh = false, refreshInterval = 5000 } = options;
  const queryClient = useQueryClient();
  const query = useQuery(legacyTorrentsQuery(autoRefresh ? refreshInterval : false));
  const torrents = query.data?.torrents ?? NO_TORRENTS;

  const updateTorrents = useCallback(
    (update: (list: TorrentStatus[]) => TorrentStatus[]) =>
      queryClient.setQueryData<TorrentListResponse>(torrentKeys.legacy, (old) => (old ? { torrents: update(old.torrents) } : old)),
    [queryClient]
  );

  const act = useCallback(async (url: string, method: string, failure: string, onSuccess: () => void) => {
    try {
      const response = await apiFetch(url, { method });
      if (!response.ok) {
        const { message } = parseErrorBody(await response.json().catch(() => undefined));
        throw new Error(message || failure);
      }
      onSuccess();
      return true;
    } catch (error) {
      console.error(`${failure}:`, error);
      return false;
    }
  }, []);

  const pauseTorrent = useCallback(
    (hash: string) =>
      act(`/api/torrents/${hash}/pause`, 'POST', 'Failed to pause torrent', () =>
        updateTorrents((list) => list.map((t) => (t.info_hash === hash ? { ...t, is_paused: true } : t)))
      ),
    [act, updateTorrents]
  );

  const resumeTorrent = useCallback(
    (hash: string) =>
      act(`/api/torrents/${hash}/resume`, 'POST', 'Failed to resume torrent', () =>
        updateTorrents((list) => list.map((t) => (t.info_hash === hash ? { ...t, is_paused: false } : t)))
      ),
    [act, updateTorrents]
  );

  const removeTorrent = useCallback(
    (hash: string, deleteData = false) =>
      act(`/api/torrents/${hash}${deleteData ? '?delete_data=true' : ''}`, 'DELETE', 'Failed to remove torrent', () =>
        updateTorrents((list) => list.filter((t) => t.info_hash !== hash))
      ),
    [act, updateTorrents]
  );

  const torrentMap = useMemo(() => new Map(torrents.map((t) => [t.info_hash, t])), [torrents]);
  const { refetch } = query;
  const refresh = useCallback(async () => {
    await refetch();
  }, [refetch]);

  return {
    torrents,
    isLoading: query.isPending,
    error: query.error ? query.error.message || 'Failed to fetch torrents' : null,
    refresh,
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
      const response = await apiFetch(`/api/torrents/${infoHash}`);

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
