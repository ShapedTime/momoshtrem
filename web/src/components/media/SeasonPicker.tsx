import { ChevronDownIcon } from '@/components/ui';
import type { Season } from '@/types/tmdb';

interface SeasonPickerProps {
  seasons: Season[];
  selectedSeason: number;
  onSeasonChange: (seasonNumber: number) => void;
  disabled?: boolean;
}

/**
 * Dropdown to select a season from a TV show.
 * Shows season name and episode count.
 */
export function SeasonPicker({
  seasons,
  selectedSeason,
  onSeasonChange,
  disabled = false,
}: SeasonPickerProps) {
  return (
    <div className="mb-6">
      <label
        htmlFor="season-select"
        className="block text-sm font-medium text-text-secondary mb-2"
      >
        Season
      </label>
      <div className="relative inline-block">
        <select
          id="season-select"
          value={selectedSeason}
          onChange={(e) => onSeasonChange(parseInt(e.target.value, 10))}
          disabled={disabled}
          className="
            appearance-none
            bg-bg-elevated border border-border rounded-md
            px-4 py-3 pr-10
            text-white font-medium
            min-w-[200px]
            cursor-pointer
            focus:outline-none focus-visible:ring-2 focus-visible:ring-accent-blue
            focus-visible:ring-offset-2 focus-visible:ring-offset-bg-primary
            disabled:opacity-50 disabled:cursor-not-allowed
            transition-colors duration-200 motion-reduce:transition-none
            hover:bg-bg-hover
          "
          aria-label="Select season"
        >
          {seasons.map((season) => (
            <option key={season.id} value={season.seasonNumber}>
              {season.name} ({season.episodeCount} episodes)
            </option>
          ))}
        </select>
        <div className="absolute inset-y-0 right-0 flex items-center pr-3 pointer-events-none">
          <ChevronDownIcon size={20} className="text-text-secondary" />
        </div>
      </div>
    </div>
  );
}

export function SeasonPickerSkeleton() {
  return (
    <div className="mb-6">
      <div className="h-4 w-16 bg-bg-hover rounded mb-2 animate-pulse motion-reduce:animate-none" />
      <div className="h-12 w-[200px] bg-bg-hover rounded-md animate-pulse motion-reduce:animate-none" />
    </div>
  );
}
