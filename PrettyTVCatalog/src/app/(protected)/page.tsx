'use client';

import { useMemo } from 'react';
import { useTrending } from '@/hooks/useTMDB';
import type { SearchResult } from '@/types/tmdb';
import {
  HeroBanner,
  HeroBannerSkeleton,
  MediaCarousel,
  MediaCarouselSkeleton,
} from '@/components/media';
import { Button } from '@/components/ui';

function getRandomItem<T>(items: T[]): T | null {
  if (items.length === 0) return null;
  const randomIndex = Math.floor(Math.random() * items.length);
  return items[randomIndex];
}

function ErrorState({ message, onRetry }: { message: string; onRetry: () => void }) {
  return (
    <div className="flex flex-col items-center justify-center min-h-[50vh] px-4">
      <div className="text-center max-w-md">
        <svg
          xmlns="http://www.w3.org/2000/svg"
          width="48"
          height="48"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="2"
          strokeLinecap="round"
          strokeLinejoin="round"
          className="mx-auto mb-4 text-text-muted"
          aria-hidden="true"
        >
          <circle cx="12" cy="12" r="10" />
          <line x1="12" y1="8" x2="12" y2="12" />
          <line x1="12" y1="16" x2="12.01" y2="16" />
        </svg>
        <h2 className="text-xl font-semibold text-white mb-2">
          Unable to load content
        </h2>
        <p className="text-text-secondary mb-6">{message}</p>
        <Button variant="primary" onClick={onRetry}>
          Try Again
        </Button>
      </div>
    </div>
  );
}

function LoadingState() {
  return (
    <>
      <HeroBannerSkeleton />
      <MediaCarouselSkeleton title="Trending Movies" />
      <MediaCarouselSkeleton title="Trending TV Shows" />
    </>
  );
}

export default function HomePage() {
  const { data: trending, isLoading, error } = useTrending();

  // Select a random featured item from all trending content
  const featuredItem = useMemo<SearchResult | null>(() => {
    if (!trending) return null;
    const allItems: SearchResult[] = [...trending.movies, ...trending.tvShows];
    // Filter to items with backdrops for better hero display
    const itemsWithBackdrop = allItems.filter((item) => item.backdropPath);
    return getRandomItem(itemsWithBackdrop.length > 0 ? itemsWithBackdrop : allItems);
  }, [trending]);

  if (isLoading) {
    return <LoadingState />;
  }

  if (error) {
    return (
      <ErrorState
        message={error}
        onRetry={() => window.location.reload()}
      />
    );
  }

  if (!trending || (!trending.movies.length && !trending.tvShows.length)) {
    return (
      <div className="flex items-center justify-center min-h-[50vh]">
        <p className="text-text-secondary">No trending content available</p>
      </div>
    );
  }

  return (
    <>
      {/* Hero Banner */}
      {featuredItem && <HeroBanner media={featuredItem} />}

      {/* Trending Movies Carousel */}
      {trending.movies.length > 0 && (
        <MediaCarousel title="Trending Movies" items={trending.movies} />
      )}

      {/* Trending TV Shows Carousel */}
      {trending.tvShows.length > 0 && (
        <MediaCarousel title="Trending TV Shows" items={trending.tvShows} />
      )}
    </>
  );
}
