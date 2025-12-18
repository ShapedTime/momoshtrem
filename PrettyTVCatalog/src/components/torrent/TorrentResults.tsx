import { TorrentCard, TorrentCardSkeleton } from './TorrentCard';
import { AlertCircleIcon } from '@/components/ui';
import type { TorrentResult, TorrentSortField, SortDirection } from '@/types/jackett';

interface TorrentResultsProps {
  results: TorrentResult[];
  isLoading: boolean;
  error: string | null;
  sortField: TorrentSortField;
  sortDirection: SortDirection;
  onSortChange: (field: TorrentSortField, direction: SortDirection) => void;
  onAddTorrent?: (magnetUri: string) => void;
  /** Function to check if a specific magnet URI is currently being added */
  isAdding?: (magnetUri: string) => boolean;
  /** Function to check if a specific magnet URI has already been added */
  isAdded?: (magnetUri: string) => boolean;
}

const SORT_OPTIONS: Array<{ field: TorrentSortField; label: string }> = [
  { field: 'seeders', label: 'Seeders' },
  { field: 'size', label: 'Size' },
];

export function TorrentResults({
  results,
  isLoading,
  error,
  sortField,
  sortDirection,
  onSortChange,
  onAddTorrent,
  isAdding,
  isAdded,
}: TorrentResultsProps) {
  // Loading state
  if (isLoading) {
    return (
      <div className="space-y-3" role="status" aria-label="Loading results">
        {[...Array(5)].map((_, i) => (
          <TorrentCardSkeleton key={i} />
        ))}
      </div>
    );
  }

  // Error state
  if (error) {
    return (
      <div className="flex flex-col items-center justify-center py-8 text-center">
        <AlertCircleIcon size={32} className="text-text-muted mb-3" />
        <p className="text-text-secondary">{error}</p>
      </div>
    );
  }

  // Empty state
  if (results.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center py-8 text-center">
        <p className="text-text-secondary">
          No torrents found. Try a different search.
        </p>
      </div>
    );
  }

  // Results with sort controls
  return (
    <div>
      {/* Sort Controls */}
      <div className="flex items-center justify-between mb-3">
        <p className="text-sm text-text-secondary">
          {results.length} result{results.length !== 1 ? 's' : ''}
        </p>
        <div className="flex gap-2">
          {SORT_OPTIONS.map((option) => (
            <button
              key={option.field}
              type="button"
              onClick={() => {
                if (sortField === option.field) {
                  onSortChange(
                    option.field,
                    sortDirection === 'desc' ? 'asc' : 'desc'
                  );
                } else {
                  onSortChange(option.field, 'desc');
                }
              }}
              className={`
                px-2 py-1 text-xs rounded
                transition-colors duration-200
                focus:outline-none focus-visible:ring-2 focus-visible:ring-accent-blue
                ${
                  sortField === option.field
                    ? 'bg-white/20 text-white'
                    : 'bg-white/5 text-text-secondary hover:bg-white/10'
                }
              `}
              aria-pressed={sortField === option.field}
            >
              {option.label}
              {sortField === option.field && (
                <span className="ml-1">
                  {sortDirection === 'desc' ? '↓' : '↑'}
                </span>
              )}
            </button>
          ))}
        </div>
      </div>

      {/* Results List */}
      <div className="space-y-2 max-h-[50vh] overflow-y-auto">
        {results.map((result) => (
          <TorrentCard
            key={result.guid}
            result={result}
            onAdd={onAddTorrent}
            isAdding={isAdding?.(result.magnetUri)}
            isAdded={isAdded?.(result.magnetUri)}
          />
        ))}
      </div>
    </div>
  );
}
