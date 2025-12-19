'use client';

import { useState } from 'react';
import { useGenres, useDiscover } from '@/hooks/useTMDB';
import { MediaCard, MediaCardSkeleton } from '@/components/media';
import { FilterSidebar } from '@/components/browse';
import { Button, AlertCircleIcon } from '@/components/ui';
import type { SortOption } from '@/types/tmdb';

function TVShowsPageSkeleton() {
  return (
    <div className="px-4 sm:px-6 lg:px-12 py-6 sm:py-8 max-w-screen-2xl mx-auto">
      <div className="h-8 w-40 bg-bg-hover rounded animate-pulse mb-6 sm:mb-8" />
      <div className="flex gap-6">
        <div className="hidden lg:block w-64 flex-shrink-0">
          <div className="space-y-4">
            <div className="h-6 w-24 bg-bg-hover rounded animate-pulse" />
            <div className="h-10 w-full bg-bg-hover rounded animate-pulse" />
            <div className="h-6 w-20 bg-bg-hover rounded animate-pulse mt-6" />
            {Array.from({ length: 8 }).map((_, i) => (
              <div key={i} className="h-8 w-full bg-bg-hover rounded animate-pulse" />
            ))}
          </div>
        </div>
        <div className="flex-1">
          <div className="grid gap-4 sm:gap-6 grid-cols-2 sm:grid-cols-2 md:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5">
            {Array.from({ length: 20 }).map((_, i) => (
              <MediaCardSkeleton key={i} />
            ))}
          </div>
        </div>
      </div>
    </div>
  );
}

function ErrorState({ message, onRetry }: { message: string; onRetry: () => void }) {
  return (
    <div className="flex flex-col items-center justify-center min-h-[50vh] px-4">
      <div className="text-center max-w-md">
        <AlertCircleIcon size={48} className="mx-auto mb-4 text-text-muted" />
        <h2 className="text-xl font-semibold text-white mb-2">
          Unable to load TV shows
        </h2>
        <p className="text-text-secondary mb-6">{message}</p>
        <Button variant="primary" onClick={onRetry}>
          Try Again
        </Button>
      </div>
    </div>
  );
}

export default function TVShowsPage() {
  const [selectedGenre, setSelectedGenre] = useState<number | null>(null);
  const [sortBy, setSortBy] = useState<SortOption>('popularity.desc');

  const { data: genres, isLoading: genresLoading } = useGenres('tv');
  const {
    data: discoverData,
    isLoading,
    error,
    loadMore,
    hasMore,
  } = useDiscover('tv', { genreId: selectedGenre ?? undefined, sortBy });

  const tvShows = discoverData?.results || [];

  // Show skeleton for initial load only
  if (isLoading && tvShows.length === 0) {
    return <TVShowsPageSkeleton />;
  }

  if (error && tvShows.length === 0) {
    return <ErrorState message={error} onRetry={() => window.location.reload()} />;
  }

  const selectedGenreName = selectedGenre
    ? genres?.find((g) => g.id === selectedGenre)?.name
    : null;

  return (
    <div className="px-4 sm:px-6 lg:px-12 py-6 sm:py-8 max-w-screen-2xl mx-auto">
      {/* Header */}
      <div className="mb-6 sm:mb-8">
        <h1 className="text-2xl sm:text-3xl font-bold text-white">
          {selectedGenreName ? `${selectedGenreName} TV Shows` : 'TV Shows'}
        </h1>
        {discoverData && (
          <p className="text-text-secondary mt-1">
            {discoverData.totalResults.toLocaleString()} titles
          </p>
        )}
      </div>

      <div className="flex gap-6">
        {/* Filter Sidebar */}
        <FilterSidebar
          mediaType="tv"
          genres={genres || []}
          genresLoading={genresLoading}
          selectedGenre={selectedGenre}
          onSelectGenre={setSelectedGenre}
          sortBy={sortBy}
          onSortChange={setSortBy}
        />

        {/* Results Grid */}
        <div className="flex-1 min-w-0">
          {tvShows.length === 0 ? (
            <div className="flex items-center justify-center min-h-[40vh]">
              <p className="text-text-secondary">No TV shows found</p>
            </div>
          ) : (
            <>
              <div className="grid gap-4 sm:gap-6 grid-cols-2 sm:grid-cols-2 md:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5">
                {tvShows.map((show) => (
                  <MediaCard key={show.id} media={show} />
                ))}
              </div>

              {/* Load More */}
              {hasMore && (
                <div className="mt-8 text-center">
                  <Button
                    variant="secondary"
                    onClick={loadMore}
                    disabled={isLoading}
                  >
                    {isLoading ? 'Loading...' : 'Load More'}
                  </Button>
                </div>
              )}

              {/* Loading indicator for infinite scroll */}
              {isLoading && tvShows.length > 0 && (
                <div className="mt-8 grid gap-4 sm:gap-6 grid-cols-2 sm:grid-cols-2 md:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5">
                  {Array.from({ length: 5 }).map((_, i) => (
                    <MediaCardSkeleton key={`loading-${i}`} />
                  ))}
                </div>
              )}
            </>
          )}
        </div>
      </div>
    </div>
  );
}
