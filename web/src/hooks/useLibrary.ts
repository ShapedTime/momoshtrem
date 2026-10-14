'use client';

import { useState, useEffect, useCallback } from 'react';
import type {
  LibraryMovie,
  LibraryShow,
  LibraryStatus,
} from '@/types/momoshtrem';

// ============================================================================
// Types
// ============================================================================

interface LibraryState {
  movies: LibraryMovie[];
  shows: LibraryShow[];
  isLoading: boolean;
  error: string | null;
}

interface UseLibraryResult {
  movies: LibraryMovie[];
  shows: LibraryShow[];
  isLoading: boolean;
  error: string | null;
  refresh: () => Promise<void>;
}

interface LibraryStatusState {
  status: LibraryStatus;
  libraryId?: number;
  hasAssignment: boolean;
  isLoading: boolean;
  error: string | null;
}

interface UseLibraryStatusResult extends LibraryStatusState {
  refresh: () => Promise<void>;
}

// ============================================================================
// useLibrary Hook
// ============================================================================

/**
 * Hook for fetching and managing the full library (movies and shows).
 */
export function useLibrary(): UseLibraryResult {
  const [state, setState] = useState<LibraryState>({
    movies: [],
    shows: [],
    isLoading: true,
    error: null,
  });

  const fetchLibrary = useCallback(async () => {
    setState((prev) => ({ ...prev, isLoading: true, error: null }));

    try {
      const [moviesRes, showsRes] = await Promise.all([
        fetch('/api/library/movies'),
        fetch('/api/library/shows'),
      ]);

      if (!moviesRes.ok || !showsRes.ok) {
        throw new Error('Failed to fetch library');
      }

      const [moviesData, showsData] = await Promise.all([
        moviesRes.json(),
        showsRes.json(),
      ]);

      setState({
        movies: moviesData.movies || [],
        shows: showsData.shows || [],
        isLoading: false,
        error: null,
      });
    } catch (error) {
      setState((prev) => ({
        ...prev,
        isLoading: false,
        error: error instanceof Error ? error.message : 'Failed to fetch library',
      }));
    }
  }, []);

  useEffect(() => {
    fetchLibrary();
  }, [fetchLibrary]);

  return {
    movies: state.movies,
    shows: state.shows,
    isLoading: state.isLoading,
    error: state.error,
    refresh: fetchLibrary,
  };
}

// ============================================================================
// useLibraryStatus Hook
// ============================================================================

/**
 * Hook for checking if a specific item is in the library.
 */
export function useLibraryStatus(
  mediaType: 'movie' | 'tv',
  tmdbId: number
): UseLibraryStatusResult {
  const [state, setState] = useState<LibraryStatusState>({
    status: 'not_in_library',
    hasAssignment: false,
    isLoading: true,
    error: null,
  });

  const fetchStatus = useCallback(async () => {
    if (!tmdbId) {
      setState({
        status: 'not_in_library',
        hasAssignment: false,
        isLoading: false,
        error: null,
      });
      return;
    }

    setState((prev) => ({ ...prev, isLoading: true, error: null }));

    try {
      const response = await fetch(
        `/api/library/status?media_type=${mediaType}&tmdb_id=${tmdbId}`
      );

      if (!response.ok) {
        throw new Error('Failed to check library status');
      }

      const data = await response.json();

      setState({
        status: data.status,
        libraryId: data.library_id,
        hasAssignment: data.has_assignment,
        isLoading: false,
        error: null,
      });
    } catch (error) {
      setState((prev) => ({
        ...prev,
        isLoading: false,
        error: error instanceof Error ? error.message : 'Failed to check status',
      }));
    }
  }, [mediaType, tmdbId]);

  useEffect(() => {
    fetchStatus();
  }, [fetchStatus]);

  return {
    ...state,
    refresh: fetchStatus,
  };
}

// ============================================================================
// useRemoveFromLibrary Hook
// ============================================================================

interface RemoveState {
  isRemoving: boolean;
  error: string | null;
}

interface UseRemoveFromLibraryResult {
  removeMovie: (libraryId: number) => Promise<boolean>;
  removeShow: (libraryId: number) => Promise<boolean>;
  isRemoving: boolean;
  error: string | null;
}

/**
 * Hook for removing items from the library.
 */
export function useRemoveFromLibrary(): UseRemoveFromLibraryResult {
  const [state, setState] = useState<RemoveState>({
    isRemoving: false,
    error: null,
  });

  const removeMovie = useCallback(async (libraryId: number): Promise<boolean> => {
    setState({ isRemoving: true, error: null });

    try {
      const response = await fetch(`/api/library/movies/${libraryId}`, {
        method: 'DELETE',
      });

      if (!response.ok) {
        const data = await response.json().catch(() => ({}));
        throw new Error(data.error || 'Failed to remove movie');
      }

      setState({ isRemoving: false, error: null });
      return true;
    } catch (error) {
      setState({
        isRemoving: false,
        error: error instanceof Error ? error.message : 'Failed to remove movie',
      });
      return false;
    }
  }, []);

  const removeShow = useCallback(async (libraryId: number): Promise<boolean> => {
    setState({ isRemoving: true, error: null });

    try {
      const response = await fetch(`/api/library/shows/${libraryId}`, {
        method: 'DELETE',
      });

      if (!response.ok) {
        const data = await response.json().catch(() => ({}));
        throw new Error(data.error || 'Failed to remove show');
      }

      setState({ isRemoving: false, error: null });
      return true;
    } catch (error) {
      setState({
        isRemoving: false,
        error: error instanceof Error ? error.message : 'Failed to remove show',
      });
      return false;
    }
  }, []);

  return {
    removeMovie,
    removeShow,
    isRemoving: state.isRemoving,
    error: state.error,
  };
}
