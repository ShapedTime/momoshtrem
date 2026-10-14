'use client';

import { useState, useCallback, useRef, useEffect } from 'react';
import type {
  TorrentResult,
  TorrentSortField,
  SortDirection,
  VideoQuality,
} from '@/types/jackett';

// ============================================
// Hook State Types
// ============================================

interface TorrentSearchState {
  results: TorrentResult[];
  isLoading: boolean;
  error: string | null;
  query: string;
}

interface UseTorrentSearchResult extends TorrentSearchState {
  search: (query: string) => void;
  clearResults: () => void;
}

// ============================================
// Fetch Helper
// ============================================

async function fetchTorrents(
  query: string,
  signal?: AbortSignal
): Promise<TorrentResult[]> {
  const response = await fetch(
    `/api/jackett/search?q=${encodeURIComponent(query)}`,
    { signal }
  );

  if (!response.ok) {
    const errorData = await response.json().catch(() => ({}));
    throw new Error(
      errorData.error || `Search failed: ${response.statusText}`
    );
  }

  const data = await response.json();
  return data.results;
}

// ============================================
// useTorrentSearch Hook
// ============================================

export function useTorrentSearch(): UseTorrentSearchResult {
  const [state, setState] = useState<TorrentSearchState>({
    results: [],
    isLoading: false,
    error: null,
    query: '',
  });
  const abortControllerRef = useRef<AbortController | null>(null);

  const search = useCallback(async (query: string) => {
    if (!query.trim()) {
      setState({ results: [], isLoading: false, error: null, query: '' });
      return;
    }

    // Cancel any in-flight request
    if (abortControllerRef.current) {
      abortControllerRef.current.abort();
    }

    abortControllerRef.current = new AbortController();
    setState((prev) => ({ ...prev, isLoading: true, error: null, query }));

    try {
      const results = await fetchTorrents(
        query,
        abortControllerRef.current.signal
      );
      setState({ results, isLoading: false, error: null, query });
    } catch (error) {
      if (error instanceof Error && error.name === 'AbortError') {
        return;
      }
      setState({
        results: [],
        isLoading: false,
        error: error instanceof Error ? error.message : 'Search failed',
        query,
      });
    }
  }, []);

  const clearResults = useCallback(() => {
    if (abortControllerRef.current) {
      abortControllerRef.current.abort();
    }
    setState({ results: [], isLoading: false, error: null, query: '' });
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
// Sorting & Filtering Utilities
// ============================================

/**
 * Sort torrents by a given field and direction.
 */
export function sortTorrents(
  results: TorrentResult[],
  field: TorrentSortField,
  direction: SortDirection
): TorrentResult[] {
  return [...results].sort((a, b) => {
    let comparison = 0;

    switch (field) {
      case 'seeders':
        comparison = a.seeders - b.seeders;
        break;
      case 'size':
        comparison = a.size - b.size;
        break;
      case 'publishDate': {
        const dateA = a.publishDate ? new Date(a.publishDate).getTime() : 0;
        const dateB = b.publishDate ? new Date(b.publishDate).getTime() : 0;
        comparison = dateA - dateB;
        break;
      }
    }

    return direction === 'desc' ? -comparison : comparison;
  });
}

/**
 * Filter torrents by quality.
 */
export function filterByQuality(
  results: TorrentResult[],
  qualities: VideoQuality[]
): TorrentResult[] {
  if (qualities.length === 0) return results;
  return results.filter((r) => qualities.includes(r.quality));
}
