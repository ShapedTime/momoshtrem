'use client';

import { MediaCard, MediaCardSkeleton } from '@/components/media';
import type { SearchResult } from '@/types/tmdb';

interface SearchResultsProps {
  results: SearchResult[];
  emptyMessage?: string;
}

function TypeBadge({ type }: { type: 'movie' | 'tv' }) {
  return (
    <span
      className="
        absolute top-2 left-2 z-10
        bg-black/70 backdrop-blur-sm
        px-2 py-0.5 rounded
        text-xs font-medium text-white uppercase tracking-wide
      "
    >
      {type === 'movie' ? 'Movie' : 'TV'}
    </span>
  );
}

function EmptyStateIcon() {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      width="64"
      height="64"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      className="text-text-muted"
    >
      <circle cx="11" cy="11" r="8" />
      <line x1="21" y1="21" x2="16.65" y2="16.65" />
    </svg>
  );
}

function ResultCard({ media }: { media: SearchResult }) {
  return (
    <div className="relative">
      <TypeBadge type={media.mediaType} />
      <MediaCard media={media} />
    </div>
  );
}

export function SearchResults({
  results,
  emptyMessage = 'No results found',
}: SearchResultsProps) {
  if (results.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center py-16">
        <EmptyStateIcon />
        <p className="text-text-secondary text-lg mt-4">{emptyMessage}</p>
      </div>
    );
  }

  return (
    <div
      className="
        grid gap-4 sm:gap-6
        grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6
      "
    >
      {results.map((result) => (
        <ResultCard key={`${result.mediaType}-${result.id}`} media={result} />
      ))}
    </div>
  );
}

export function SearchResultsSkeleton({ count = 12 }: { count?: number }) {
  return (
    <div
      className="
        grid gap-4 sm:gap-6
        grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6
      "
    >
      {Array.from({ length: count }).map((_, i) => (
        <MediaCardSkeleton key={i} />
      ))}
    </div>
  );
}
