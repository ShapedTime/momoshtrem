'use client';

import type { Genre } from '@/types/tmdb';
import { Skeleton } from '@/components/ui';

interface GenreFilterProps {
  genres: Genre[];
  selectedGenre: number | null;
  onSelectGenre: (genreId: number | null) => void;
  isLoading?: boolean;
}

export function GenreFilter({
  genres,
  selectedGenre,
  onSelectGenre,
  isLoading = false,
}: GenreFilterProps) {
  if (isLoading) {
    return (
      <div className="space-y-2">
        <div className="h-4 w-16 bg-bg-hover rounded animate-pulse mb-3" />
        {Array.from({ length: 8 }).map((_, i) => (
          <Skeleton key={i} className="h-8 w-full rounded" />
        ))}
      </div>
    );
  }

  return (
    <div>
      <h3 className="text-sm font-medium text-text-secondary uppercase tracking-wider mb-3">
        Genres
      </h3>
      <div className="space-y-1">
        <button
          onClick={() => onSelectGenre(null)}
          className={`
            w-full text-left px-3 py-2 rounded-lg text-sm transition-colors
            ${
              selectedGenre === null
                ? 'bg-accent-blue text-white'
                : 'text-text-secondary hover:bg-bg-hover hover:text-white'
            }
          `}
        >
          All Genres
        </button>
        {genres.map((genre) => (
          <button
            key={genre.id}
            onClick={() => onSelectGenre(genre.id)}
            className={`
              w-full text-left px-3 py-2 rounded-lg text-sm transition-colors
              ${
                selectedGenre === genre.id
                  ? 'bg-accent-blue text-white'
                  : 'text-text-secondary hover:bg-bg-hover hover:text-white'
              }
            `}
          >
            {genre.name}
          </button>
        ))}
      </div>
    </div>
  );
}
