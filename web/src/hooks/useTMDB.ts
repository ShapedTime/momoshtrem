'use client';

import { useState, useEffect, useCallback, useRef } from 'react';
import type { TrendingResults } from '@/types/tmdb';
import { apiFetch } from '@/lib/api/client';

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
  const response = await apiFetch(url, { signal });

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
