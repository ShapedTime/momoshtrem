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
import { Button, AlertCircleIcon } from '@/components/ui';
import { getRandomItem } from '@/lib/utils';

function ErrorState({ message, onRetry }: { message: string; onRetry: () => void }) {
  return (
    <div className="flex flex-col items-center justify-center min-h-[50vh] px-4">
      <div className="text-center max-w-md">
        <AlertCircleIcon size={48} className="mx-auto mb-4 text-text-muted" />
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
  const { data: trending, isLoading, error, refetch } = useTrending();

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
        onRetry={refetch}
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
