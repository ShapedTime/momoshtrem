'use client';

import { useState, useCallback, useRef } from 'react';
import type { TMDBMetadata } from '@/types/distribyted';

// ============================================
// Hook State Types
// ============================================

interface AddTorrentState {
  /** The magnet URI currently being added, or null if not adding */
  addingMagnet: string | null;
  error: string | null;
}

interface UseAddTorrentResult {
  /** Add a torrent via magnet URI with optional metadata. Returns true on success. */
  addTorrent: (magnetUri: string, metadata?: TMDBMetadata) => Promise<boolean>;
  /** Check if a specific magnet URI is currently being added. */
  isAdding: (magnetUri: string) => boolean;
  /** Check if a magnet URI has already been added in this session. */
  isAdded: (magnetUri: string) => boolean;
  /** Clear all tracked added magnets and reset state. */
  reset: () => void;
  /** The last error message, if any. */
  error: string | null;
}

// ============================================
// useAddTorrent Hook
// ============================================

/**
 * Hook for adding torrents to distribyted.
 * Tracks which magnets have been added to prevent duplicates.
 */
export function useAddTorrent(): UseAddTorrentResult {
  const [state, setState] = useState<AddTorrentState>({
    addingMagnet: null,
    error: null,
  });

  // Track added magnet URIs (persists across searches in same session)
  // Using ref to avoid re-renders when adding to the set
  const addedMagnetsRef = useRef<Set<string>>(new Set());

  const addTorrent = useCallback(async (magnetUri: string, metadata?: TMDBMetadata): Promise<boolean> => {
    // Check if already added
    if (addedMagnetsRef.current.has(magnetUri)) {
      return true;
    }

    setState({ addingMagnet: magnetUri, error: null });

    try {
      const response = await fetch('/api/distribyted/add', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({ magnetUri, metadata }),
      });

      if (!response.ok) {
        const data = await response.json().catch(() => ({}));
        throw new Error(data.error || 'Failed to add torrent');
      }

      // Mark as added
      addedMagnetsRef.current.add(magnetUri);
      setState({ addingMagnet: null, error: null });
      return true;
    } catch (error) {
      const errorMessage =
        error instanceof Error ? error.message : 'Failed to add torrent';
      setState({ addingMagnet: null, error: errorMessage });
      return false;
    }
  }, []);

  const isAdding = useCallback((magnetUri: string): boolean => {
    return state.addingMagnet === magnetUri;
  }, [state.addingMagnet]);

  const isAdded = useCallback((magnetUri: string): boolean => {
    return addedMagnetsRef.current.has(magnetUri);
  }, []);

  const reset = useCallback(() => {
    addedMagnetsRef.current.clear();
    setState({ addingMagnet: null, error: null });
  }, []);

  return { addTorrent, isAdding, isAdded, reset, error: state.error };
}
