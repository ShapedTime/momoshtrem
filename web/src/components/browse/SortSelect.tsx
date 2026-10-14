'use client';

import type { MediaType, SortOption } from '@/types/tmdb';
import { ChevronDownIcon } from '@/components/ui';

interface SortOptionConfig {
  value: SortOption;
  label: string;
}

const MOVIE_SORT_OPTIONS: SortOptionConfig[] = [
  { value: 'popularity.desc', label: 'Most Popular' },
  { value: 'popularity.asc', label: 'Least Popular' },
  { value: 'vote_average.desc', label: 'Highest Rated' },
  { value: 'vote_average.asc', label: 'Lowest Rated' },
  { value: 'primary_release_date.desc', label: 'Newest' },
  { value: 'primary_release_date.asc', label: 'Oldest' },
  { value: 'original_title.asc', label: 'Title A-Z' },
  { value: 'original_title.desc', label: 'Title Z-A' },
  { value: 'vote_count.desc', label: 'Most Votes' },
  { value: 'vote_count.asc', label: 'Least Votes' },
];

const TV_SORT_OPTIONS: SortOptionConfig[] = [
  { value: 'popularity.desc', label: 'Most Popular' },
  { value: 'popularity.asc', label: 'Least Popular' },
  { value: 'vote_average.desc', label: 'Highest Rated' },
  { value: 'vote_average.asc', label: 'Lowest Rated' },
  { value: 'first_air_date.desc', label: 'Newest' },
  { value: 'first_air_date.asc', label: 'Oldest' },
  { value: 'name.asc', label: 'Title A-Z' },
  { value: 'name.desc', label: 'Title Z-A' },
  { value: 'vote_count.desc', label: 'Most Votes' },
  { value: 'vote_count.asc', label: 'Least Votes' },
];

interface SortSelectProps {
  mediaType: MediaType;
  value: SortOption;
  onChange: (value: SortOption) => void;
}

export function SortSelect({ mediaType, value, onChange }: SortSelectProps) {
  const options = mediaType === 'movie' ? MOVIE_SORT_OPTIONS : TV_SORT_OPTIONS;

  return (
    <div>
      <h3 className="text-sm font-medium text-text-secondary uppercase tracking-wider mb-3">
        Sort By
      </h3>
      <div className="relative">
        <select
          value={value}
          onChange={(e) => onChange(e.target.value as SortOption)}
          className="
            w-full appearance-none bg-bg-elevated border border-white/10 rounded-lg
            px-3 py-2 pr-10 text-sm text-white
            focus:outline-none focus:ring-2 focus:ring-accent-blue focus:border-transparent
            cursor-pointer
          "
        >
          {options.map((option) => (
            <option key={option.value} value={option.value}>
              {option.label}
            </option>
          ))}
        </select>
        <ChevronDownIcon
          size={16}
          className="absolute right-3 top-1/2 -translate-y-1/2 text-text-muted pointer-events-none"
        />
      </div>
    </div>
  );
}
