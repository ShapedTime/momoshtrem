'use client';

import { useState, useEffect, useCallback, useRef } from 'react';
import type {
  SubtitleSearchResult,
  Subtitle,
  SubtitleSearchContext,
  DownloadSubtitleRequest,
} from '@/types/subtitle';

// ============================================================================
// Types
// ============================================================================

interface SubtitlesState {
  subtitles: Subtitle[];
  isLoading: boolean;
  error: string | null;
}

interface UseSubtitlesResult {
  subtitles: Subtitle[];
  isLoading: boolean;
  error: string | null;
  refresh: () => Promise<void>;
  downloadSubtitle: (params: DownloadSubtitleRequest) => Promise<Subtitle | null>;
  deleteSubtitle: (subtitleId: number) => Promise<boolean>;
  /** Check if a language is already downloaded */
  hasLanguage: (languageCode: string) => boolean;
}

interface SubtitleSearchState {
  results: SubtitleSearchResult[];
  isSearching: boolean;
  error: string | null;
}

interface UseSubtitleSearchResult extends SubtitleSearchState {
  search: (languages?: string[]) => Promise<void>;
  clearResults: () => void;
}

// ============================================================================
// useSubtitles Hook
// ============================================================================

/**
 * Hook for fetching and managing subtitles for a media item.
 */
export function useSubtitles(
  itemType: 'movie' | 'episode',
  itemId: number | null | undefined
): UseSubtitlesResult {
  const [state, setState] = useState<SubtitlesState>({
    subtitles: [],
    isLoading: !!itemId,
    error: null,
  });

  const mountedRef = useRef(true);

  const fetchSubtitles = useCallback(async () => {
    if (!itemId) {
      setState({
        subtitles: [],
        isLoading: false,
        error: null,
      });
      return;
    }

    setState((prev) => ({ ...prev, isLoading: true, error: null }));

    try {
      const endpoint =
        itemType === 'movie'
          ? `/api/movies/${itemId}/subtitles`
          : `/api/episodes/${itemId}/subtitles`;

      const response = await fetch(endpoint);

      if (!response.ok) {
        throw new Error('Failed to fetch subtitles');
      }

      const data = await response.json();

      if (mountedRef.current) {
        setState({
          subtitles: data.subtitles || [],
          isLoading: false,
          error: null,
        });
      }
    } catch (error) {
      if (mountedRef.current) {
        setState((prev) => ({
          ...prev,
          isLoading: false,
          error: error instanceof Error ? error.message : 'Failed to fetch subtitles',
        }));
      }
    }
  }, [itemType, itemId]);

  const downloadSubtitle = useCallback(
    async (params: DownloadSubtitleRequest): Promise<Subtitle | null> => {
      try {
        const response = await fetch('/api/subtitles/download', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(params),
        });

        if (!response.ok) {
          const data = await response.json().catch(() => ({}));
          throw new Error(data.error || 'Failed to download subtitle');
        }

        const data = await response.json();
        const subtitle = data.subtitle;

        // Add to local state if still mounted
        if (mountedRef.current) {
          setState((prev) => ({
            ...prev,
            subtitles: [...prev.subtitles, subtitle],
          }));
        }

        return subtitle;
      } catch (error) {
        if (mountedRef.current) {
          console.error('Failed to download subtitle:', error);
        }
        return null;
      }
    },
    []
  );

  const deleteSubtitle = useCallback(async (subtitleId: number): Promise<boolean> => {
    try {
      const response = await fetch(`/api/subtitles/${subtitleId}`, {
        method: 'DELETE',
      });

      if (!response.ok && response.status !== 204) {
        const data = await response.json().catch(() => ({}));
        throw new Error(data.error || 'Failed to delete subtitle');
      }

      // Remove from local state if still mounted
      if (mountedRef.current) {
        setState((prev) => ({
          ...prev,
          subtitles: prev.subtitles.filter((s) => s.id !== subtitleId),
        }));
      }

      return true;
    } catch (error) {
      if (mountedRef.current) {
        console.error('Failed to delete subtitle:', error);
      }
      return false;
    }
  }, []);

  const hasLanguage = useCallback(
    (languageCode: string): boolean => {
      return state.subtitles.some((s) => s.language_code === languageCode);
    },
    [state.subtitles]
  );

  // Initial fetch
  useEffect(() => {
    mountedRef.current = true;
    fetchSubtitles();

    return () => {
      mountedRef.current = false;
    };
  }, [fetchSubtitles]);

  return {
    subtitles: state.subtitles,
    isLoading: state.isLoading,
    error: state.error,
    refresh: fetchSubtitles,
    downloadSubtitle,
    deleteSubtitle,
    hasLanguage,
  };
}

// ============================================================================
// useSubtitleSearch Hook
// ============================================================================

/**
 * Hook for searching subtitles on OpenSubtitles.
 */
export function useSubtitleSearch(
  context: SubtitleSearchContext | null
): UseSubtitleSearchResult {
  const [state, setState] = useState<SubtitleSearchState>({
    results: [],
    isSearching: false,
    error: null,
  });

  const mountedRef = useRef(true);
  const abortControllerRef = useRef<AbortController | null>(null);

  const search = useCallback(
    async (languages: string[] = ['en']) => {
      if (!context) {
        setState({
          results: [],
          isSearching: false,
          error: 'No search context provided',
        });
        return;
      }

      // Cancel any pending search
      abortControllerRef.current?.abort();
      abortControllerRef.current = new AbortController();

      setState({ results: [], isSearching: true, error: null });

      try {
        const params = new URLSearchParams({
          tmdb_id: context.tmdbId.toString(),
          type: context.mediaType,
          languages: languages.join(','),
        });

        if (context.mediaType === 'episode') {
          if (context.season) params.set('season', context.season.toString());
          if (context.episode) params.set('episode', context.episode.toString());
        }

        const response = await fetch(`/api/subtitles/search?${params}`, {
          signal: abortControllerRef.current.signal,
        });

        if (!response.ok) {
          const data = await response.json().catch(() => ({}));
          throw new Error(data.error || 'Search failed');
        }

        const data = await response.json();

        if (mountedRef.current) {
          setState({
            results: data.results || [],
            isSearching: false,
            error: null,
          });
        }
      } catch (error) {
        // Ignore abort errors
        if (error instanceof Error && error.name === 'AbortError') {
          return;
        }
        if (mountedRef.current) {
          setState({
            results: [],
            isSearching: false,
            error: error instanceof Error ? error.message : 'Search failed',
          });
        }
      }
    },
    [context]
  );

  const clearResults = useCallback(() => {
    abortControllerRef.current?.abort();
    setState({
      results: [],
      isSearching: false,
      error: null,
    });
  }, []);

  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
      abortControllerRef.current?.abort();
    };
  }, []);

  return {
    results: state.results,
    isSearching: state.isSearching,
    error: state.error,
    search,
    clearResults,
  };
}
