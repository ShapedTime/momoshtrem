'use client';

import { useState, useEffect, useCallback, useRef } from 'react';
import type {
  TrendingResults,
  SearchResult,
  MovieDetails,
  TVShowDetails,
  SeasonDetails,
} from '@/types/tmdb';

// ============================================
// Hook State Type
// ============================================

interface FetchState<T> {
  data: T | null;
  isLoading: boolean;
  error: string | null;
}

interface FetchStateWithRefetch<T> extends FetchState<T> {
  refetch: () => void;
}

// ============================================
// Fetch Helper with AbortController
// ============================================

async function fetchAPI<T>(url: string, signal?: AbortSignal): Promise<T> {
  const response = await fetch(url, { signal });

  if (!response.ok) {
    const errorData = await response.json().catch(() => ({}));
    throw new Error(
      errorData.error || `Request failed: ${response.statusText}`
    );
  }

  return response.json() as Promise<T>;
}

// ============================================
// Generic Hook for ID-based fetching
// ============================================

/**
 * Generic hook for fetching data based on an ID.
 * Handles loading state, errors, and request cancellation.
 */
function useFetchOnId<T>(
  id: number | null,
  urlBuilder: (id: number) => string,
  resourceName: string
): FetchState<T> {
  const [state, setState] = useState<FetchState<T>>({
    data: null,
    isLoading: !!id,
    error: null,
  });

  useEffect(() => {
    if (!id) {
      setState({ data: null, isLoading: false, error: null });
      return;
    }

    // Capture non-null id for the closure
    const currentId = id;
    const abortController = new AbortController();

    async function fetchData() {
      setState((prev) => ({ ...prev, isLoading: true, error: null }));

      try {
        const data = await fetchAPI<T>(urlBuilder(currentId), abortController.signal);
        setState({ data, isLoading: false, error: null });
      } catch (error) {
        // Don't update state if request was aborted
        if (error instanceof Error && error.name === 'AbortError') {
          return;
        }
        setState({
          data: null,
          isLoading: false,
          error:
            error instanceof Error
              ? error.message
              : `Failed to fetch ${resourceName}`,
        });
      }
    }

    fetchData();

    return () => {
      abortController.abort();
    };
  }, [id, urlBuilder, resourceName]);

  return state;
}

// ============================================
// useTrending
// ============================================

export function useTrending(): FetchStateWithRefetch<TrendingResults> {
  const [state, setState] = useState<FetchState<TrendingResults>>({
    data: null,
    isLoading: true,
    error: null,
  });
  const abortControllerRef = useRef<AbortController | null>(null);

  const fetchTrending = useCallback(async () => {
    // Cancel any in-flight request
    if (abortControllerRef.current) {
      abortControllerRef.current.abort();
    }

    abortControllerRef.current = new AbortController();
    setState((prev) => ({ ...prev, isLoading: true, error: null }));

    try {
      const data = await fetchAPI<TrendingResults>(
        '/api/tmdb/trending',
        abortControllerRef.current.signal
      );
      setState({ data, isLoading: false, error: null });
    } catch (error) {
      if (error instanceof Error && error.name === 'AbortError') {
        return;
      }
      setState({
        data: null,
        isLoading: false,
        error:
          error instanceof Error ? error.message : 'Failed to fetch trending',
      });
    }
  }, []);

  useEffect(() => {
    fetchTrending();

    return () => {
      if (abortControllerRef.current) {
        abortControllerRef.current.abort();
      }
    };
  }, [fetchTrending]);

  return { ...state, refetch: fetchTrending };
}

// ============================================
// useSearch
// ============================================

interface UseSearchResult extends FetchState<SearchResult[]> {
  search: (query: string) => void;
  clearResults: () => void;
}

export function useSearch(): UseSearchResult {
  const [state, setState] = useState<FetchState<SearchResult[]>>({
    data: null,
    isLoading: false,
    error: null,
  });
  const abortControllerRef = useRef<AbortController | null>(null);

  const search = useCallback(async (query: string) => {
    if (!query.trim()) {
      setState({ data: null, isLoading: false, error: null });
      return;
    }

    // Cancel any in-flight request
    if (abortControllerRef.current) {
      abortControllerRef.current.abort();
    }

    abortControllerRef.current = new AbortController();
    setState((prev) => ({ ...prev, isLoading: true, error: null }));

    try {
      const data = await fetchAPI<SearchResult[]>(
        `/api/tmdb/search?q=${encodeURIComponent(query)}`,
        abortControllerRef.current.signal
      );
      setState({ data, isLoading: false, error: null });
    } catch (error) {
      if (error instanceof Error && error.name === 'AbortError') {
        return;
      }
      setState({
        data: null,
        isLoading: false,
        error: error instanceof Error ? error.message : 'Search failed',
      });
    }
  }, []);

  const clearResults = useCallback(() => {
    if (abortControllerRef.current) {
      abortControllerRef.current.abort();
    }
    setState({ data: null, isLoading: false, error: null });
  }, []);

  // Cleanup on unmount
  useEffect(() => {
    return () => {
      if (abortControllerRef.current) {
        abortControllerRef.current.abort();
      }
    };
  }, []);

  return { ...state, search, clearResults };
}

// ============================================
// useMovie
// ============================================

const buildMovieUrl = (id: number) => `/api/tmdb/movie/${id}`;

export function useMovie(id: number | null): FetchState<MovieDetails> {
  return useFetchOnId<MovieDetails>(id, buildMovieUrl, 'movie');
}

// ============================================
// useTVShow
// ============================================

const buildTVShowUrl = (id: number) => `/api/tmdb/tv/${id}`;

export function useTVShow(id: number | null): FetchState<TVShowDetails> {
  return useFetchOnId<TVShowDetails>(id, buildTVShowUrl, 'TV show');
}

// ============================================
// useSeason
// ============================================

export function useSeason(
  showId: number | null,
  seasonNumber: number | null
): FetchState<SeasonDetails> {
  const [state, setState] = useState<FetchState<SeasonDetails>>({
    data: null,
    isLoading: !!(showId && seasonNumber !== null),
    error: null,
  });

  useEffect(() => {
    if (!showId || seasonNumber === null) {
      setState({ data: null, isLoading: false, error: null });
      return;
    }

    const abortController = new AbortController();

    async function fetchSeason() {
      setState((prev) => ({ ...prev, isLoading: true, error: null }));

      try {
        const data = await fetchAPI<SeasonDetails>(
          `/api/tmdb/tv/${showId}/season/${seasonNumber}`,
          abortController.signal
        );
        setState({ data, isLoading: false, error: null });
      } catch (error) {
        if (error instanceof Error && error.name === 'AbortError') {
          return;
        }
        setState({
          data: null,
          isLoading: false,
          error:
            error instanceof Error ? error.message : 'Failed to fetch season',
        });
      }
    }

    fetchSeason();

    return () => {
      abortController.abort();
    };
  }, [showId, seasonNumber]);

  return state;
}
