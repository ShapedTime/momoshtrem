'use client';

import { useState, useCallback, useRef } from 'react';
import type {
  AddTorrentResponse,
  ShowAssignmentResponse,
} from '@/types/momoshtrem';
import { apiFetch } from '@/lib/api/client';

// ============================================================================
// Types
// ============================================================================

interface AddTorrentState {
  addingMagnet: string | null;
  error: string | null;
}

interface UseAddTorrentResult {
  /** Add a torrent with combined flow (auto-adds to library if needed). */
  addTorrent: (
    magnetUri: string,
    mediaType: 'movie' | 'tv',
    tmdbId: number
  ) => Promise<AddTorrentResponse | null>;
  /** Check if a specific magnet is being added. */
  isAdding: (magnetUri: string) => boolean;
  /** Check if a magnet has been added in this session. */
  isAdded: (magnetUri: string) => boolean;
  /** Last error message. */
  error: string | null;
  /** Reset tracking state. */
  reset: () => void;
}

// ============================================================================
// useAddTorrent Hook
// ============================================================================

/**
 * Hook for adding torrents with the combined flow.
 * Automatically adds to library if needed, then assigns the torrent.
 */
export function useAddTorrent(): UseAddTorrentResult {
  const [state, setState] = useState<AddTorrentState>({
    addingMagnet: null,
    error: null,
  });

  // Track added magnets to prevent duplicates
  const addedMagnetsRef = useRef<Set<string>>(new Set());

  const addTorrent = useCallback(
    async (
      magnetUri: string,
      mediaType: 'movie' | 'tv',
      tmdbId: number
    ): Promise<AddTorrentResponse | null> => {
      // Check if already added
      if (addedMagnetsRef.current.has(magnetUri)) {
        return null;
      }

      setState({ addingMagnet: magnetUri, error: null });

      try {
        const response = await apiFetch('/api/library/add-torrent', {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
          },
          body: JSON.stringify({
            magnet_uri: magnetUri,
            media_type: mediaType,
            tmdb_id: tmdbId,
          }),
        });

        if (!response.ok) {
          const data = await response.json().catch(() => ({}));
          throw new Error(data.error || 'Failed to add torrent');
        }

        const data = (await response.json()) as AddTorrentResponse;

        // Mark as added
        addedMagnetsRef.current.add(magnetUri);
        setState({ addingMagnet: null, error: null });

        return data;
      } catch (error) {
        const errorMessage =
          error instanceof Error ? error.message : 'Failed to add torrent';
        setState({ addingMagnet: null, error: errorMessage });
        return null;
      }
    },
    []
  );

  const isAdding = useCallback(
    (magnetUri: string): boolean => {
      return state.addingMagnet === magnetUri;
    },
    [state.addingMagnet]
  );

  const isAdded = useCallback((magnetUri: string): boolean => {
    return addedMagnetsRef.current.has(magnetUri);
  }, []);

  const reset = useCallback(() => {
    addedMagnetsRef.current.clear();
    setState({ addingMagnet: null, error: null });
  }, []);

  return {
    addTorrent,
    isAdding,
    isAdded,
    error: state.error,
    reset,
  };
}

// ============================================================================
// Helper Types for UI
// ============================================================================

/**
 * Type guard to check if response is for a TV show (has episode matches).
 */
export function isShowResponse(
  response: AddTorrentResponse
): response is AddTorrentResponse & { summary: ShowAssignmentResponse['summary'] } {
  return response.media_type === 'tv' && response.summary !== undefined;
}
