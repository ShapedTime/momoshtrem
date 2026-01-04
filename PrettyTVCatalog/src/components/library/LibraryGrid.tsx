'use client';

import { useState, useCallback } from 'react';
import { LibraryCard, LibraryCardSkeleton } from './LibraryCard';
import { EmptyLibrary } from './EmptyLibrary';
import { useToast } from '@/components/ui/Toast';
import type { LibraryMovie, LibraryShow } from '@/types/momoshtrem';

type FilterType = 'all' | 'movies' | 'shows';

interface LibraryGridProps {
  movies: LibraryMovie[];
  shows: LibraryShow[];
  isLoading: boolean;
  /** Map of TMDB IDs to poster paths (fetched from TMDB) */
  posterPaths?: Record<string, string | null>;
  /** Called when library is updated (item removed) */
  onRefresh?: () => void;
}

const filterTabs: { value: FilterType; label: string }[] = [
  { value: 'all', label: 'All' },
  { value: 'movies', label: 'Movies' },
  { value: 'shows', label: 'TV Shows' },
];

export function LibraryGrid({
  movies,
  shows,
  isLoading,
  posterPaths = {},
  onRefresh,
}: LibraryGridProps) {
  const [filter, setFilter] = useState<FilterType>('all');
  const { showToast } = useToast();

  const handleRemove = useCallback(async (id: number, mediaType: 'movie' | 'tv') => {
    const endpoint = mediaType === 'movie'
      ? `/api/library/movies/${id}`
      : `/api/library/shows/${id}`;

    try {
      const response = await fetch(endpoint, { method: 'DELETE' });

      if (!response.ok) {
        throw new Error('Failed to remove from library');
      }

      showToast('success', 'Removed from library');
      onRefresh?.();
    } catch (error) {
      showToast(
        'error',
        error instanceof Error ? error.message : 'Failed to remove'
      );
    }
  }, [showToast, onRefresh]);

  // Filter items
  const filteredMovies = filter === 'shows' ? [] : movies;
  const filteredShows = filter === 'movies' ? [] : shows;
  const totalItems = filteredMovies.length + filteredShows.length;

  // Loading state
  if (isLoading) {
    return (
      <div className="space-y-6">
        {/* Filter tabs skeleton */}
        <div className="flex gap-2">
          {filterTabs.map((tab) => (
            <div
              key={tab.value}
              className="h-9 w-20 bg-bg-hover rounded-md animate-pulse motion-reduce:animate-none"
            />
          ))}
        </div>

        {/* Grid skeleton */}
        <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 gap-4 sm:gap-6">
          {Array.from({ length: 12 }).map((_, i) => (
            <LibraryCardSkeleton key={i} />
          ))}
        </div>
      </div>
    );
  }

  // Empty state
  if (movies.length === 0 && shows.length === 0) {
    return <EmptyLibrary />;
  }

  return (
    <div className="space-y-6">
      {/* Filter tabs */}
      <div className="flex gap-2" role="tablist" aria-label="Filter library">
        {filterTabs.map((tab) => {
          const isActive = filter === tab.value;
          const count =
            tab.value === 'all'
              ? movies.length + shows.length
              : tab.value === 'movies'
              ? movies.length
              : shows.length;

          return (
            <button
              key={tab.value}
              role="tab"
              aria-selected={isActive}
              onClick={() => setFilter(tab.value)}
              className={`
                px-4 py-2 text-sm font-medium rounded-md
                transition-colors duration-200 motion-reduce:transition-none
                focus:outline-none focus-visible:ring-2 focus-visible:ring-accent-blue
                ${isActive
                  ? 'bg-white text-black'
                  : 'bg-bg-hover text-text-secondary hover:text-white hover:bg-bg-active'
                }
              `}
            >
              {tab.label}
              <span className="ml-1.5 text-xs opacity-70">({count})</span>
            </button>
          );
        })}
      </div>

      {/* Empty filter state */}
      {totalItems === 0 && (
        <div className="text-center py-12">
          <p className="text-text-secondary">
            No {filter === 'movies' ? 'movies' : 'TV shows'} in your library yet.
          </p>
        </div>
      )}

      {/* Grid */}
      {totalItems > 0 && (
        <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 gap-4 sm:gap-6">
          {/* Movies */}
          {filteredMovies.map((movie) => (
            <LibraryCard
              key={`movie-${movie.id}`}
              item={movie}
              mediaType="movie"
              posterPath={posterPaths[`movie-${movie.tmdb_id}`]}
              onRemove={handleRemove}
            />
          ))}

          {/* Shows */}
          {filteredShows.map((show) => (
            <LibraryCard
              key={`show-${show.id}`}
              item={show}
              mediaType="tv"
              posterPath={posterPaths[`tv-${show.tmdb_id}`]}
              onRemove={handleRemove}
            />
          ))}
        </div>
      )}
    </div>
  );
}
