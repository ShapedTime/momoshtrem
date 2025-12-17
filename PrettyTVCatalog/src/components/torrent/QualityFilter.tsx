import type { VideoQuality } from '@/types/jackett';

interface QualityFilterProps {
  selected: VideoQuality[];
  onToggle: (quality: VideoQuality) => void;
}

const QUALITY_OPTIONS: VideoQuality[] = ['4K', '1080p', '720p', '480p'];

export function QualityFilter({ selected, onToggle }: QualityFilterProps) {
  return (
    <div
      className="flex flex-wrap gap-2"
      role="group"
      aria-label="Filter by quality"
    >
      {QUALITY_OPTIONS.map((quality) => {
        const isSelected = selected.includes(quality);
        return (
          <button
            key={quality}
            type="button"
            onClick={() => onToggle(quality)}
            className={`
              px-3 py-1.5 rounded-full text-sm font-medium
              transition-colors duration-200
              focus:outline-none focus-visible:ring-2 focus-visible:ring-accent-blue
              ${
                isSelected
                  ? 'bg-accent-red text-white'
                  : 'bg-white/10 text-text-secondary hover:bg-white/20'
              }
            `}
            aria-pressed={isSelected}
          >
            {quality}
          </button>
        );
      })}
    </div>
  );
}
