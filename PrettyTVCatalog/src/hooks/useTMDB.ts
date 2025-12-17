'use client';

import { useState, useEffect, useCallback } from 'react';
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

// ============================================
// Fetch Helper
// ============================================

async function fetchAPI<T>(url: string): Promise<T> {
  const response = await fetch(url);

  if (!response.ok) {
    const errorData = await response.json().catch(() => ({}));
    throw new Error(
      errorData.error || `Request failed: ${response.statusText}`
    );
  }

  return response.json() as Promise<T>;
}

// ============================================
// useTrending
// ============================================

export function useTrending(): FetchState<TrendingResults> {
  const [state, setState] = useState<FetchState<TrendingResults>>({
    data: null,
    isLoading: true,
    error: null,
  });

  useEffect(() => {
    let cancelled = false;

    async function fetchTrending() {
      try {
        const data = await fetchAPI<TrendingResults>('/api/tmdb/trending');
        if (!cancelled) {
          setState({ data, isLoading: false, error: null });
        }
      } catch (error) {
        if (!cancelled) {
          setState({
            data: null,
            isLoading: false,
            error:
              error instanceof Error ? error.message : 'Failed to fetch trending',
          });
        }
      }
    }

    fetchTrending();

    return () => {
      cancelled = true;
    };
  }, []);

  return state;
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

  const search = useCallback(async (query: string) => {
    if (!query.trim()) {
      setState({ data: null, isLoading: false, error: null });
      return;
    }

    setState((prev) => ({ ...prev, isLoading: true, error: null }));

    try {
      const data = await fetchAPI<SearchResult[]>(
        `/api/tmdb/search?q=${encodeURIComponent(query)}`
      );
      setState({ data, isLoading: false, error: null });
    } catch (error) {
      setState({
        data: null,
        isLoading: false,
        error: error instanceof Error ? error.message : 'Search failed',
      });
    }
  }, []);

  const clearResults = useCallback(() => {
    setState({ data: null, isLoading: false, error: null });
  }, []);

  return { ...state, search, clearResults };
}

// ============================================
// useMovie
// ============================================

export function useMovie(id: number | null): FetchState<MovieDetails> {
  const [state, setState] = useState<FetchState<MovieDetails>>({
    data: null,
    isLoading: !!id,
    error: null,
  });

  useEffect(() => {
    if (!id) {
      setState({ data: null, isLoading: false, error: null });
      return;
    }

    let cancelled = false;

    async function fetchMovie() {
      setState((prev) => ({ ...prev, isLoading: true, error: null }));

      try {
        const data = await fetchAPI<MovieDetails>(`/api/tmdb/movie/${id}`);
        if (!cancelled) {
          setState({ data, isLoading: false, error: null });
        }
      } catch (error) {
        if (!cancelled) {
          setState({
            data: null,
            isLoading: false,
            error:
              error instanceof Error ? error.message : 'Failed to fetch movie',
          });
        }
      }
    }

    fetchMovie();

    return () => {
      cancelled = true;
    };
  }, [id]);

  return state;
}

// ============================================
// useTVShow
// ============================================

export function useTVShow(id: number | null): FetchState<TVShowDetails> {
  const [state, setState] = useState<FetchState<TVShowDetails>>({
    data: null,
    isLoading: !!id,
    error: null,
  });

  useEffect(() => {
    if (!id) {
      setState({ data: null, isLoading: false, error: null });
      return;
    }

    let cancelled = false;

    async function fetchTVShow() {
      setState((prev) => ({ ...prev, isLoading: true, error: null }));

      try {
        const data = await fetchAPI<TVShowDetails>(`/api/tmdb/tv/${id}`);
        if (!cancelled) {
          setState({ data, isLoading: false, error: null });
        }
      } catch (error) {
        if (!cancelled) {
          setState({
            data: null,
            isLoading: false,
            error:
              error instanceof Error ? error.message : 'Failed to fetch TV show',
          });
        }
      }
    }

    fetchTVShow();

    return () => {
      cancelled = true;
    };
  }, [id]);

  return state;
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

    let cancelled = false;

    async function fetchSeason() {
      setState((prev) => ({ ...prev, isLoading: true, error: null }));

      try {
        const data = await fetchAPI<SeasonDetails>(
          `/api/tmdb/tv/${showId}/season/${seasonNumber}`
        );
        if (!cancelled) {
          setState({ data, isLoading: false, error: null });
        }
      } catch (error) {
        if (!cancelled) {
          setState({
            data: null,
            isLoading: false,
            error:
              error instanceof Error ? error.message : 'Failed to fetch season',
          });
        }
      }
    }

    fetchSeason();

    return () => {
      cancelled = true;
    };
  }, [showId, seasonNumber]);

  return state;
}
