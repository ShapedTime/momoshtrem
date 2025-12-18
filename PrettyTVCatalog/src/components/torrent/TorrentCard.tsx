import { Button } from '@/components/ui';
import { formatFileSize } from '@/types/jackett';
import type { TorrentResult, VideoQuality } from '@/types/jackett';

interface TorrentCardProps {
  result: TorrentResult;
  onAdd?: (magnetUri: string) => void;
  /** Whether a torrent add operation is currently in progress */
  isAdding?: boolean;
  /** Whether this specific torrent has already been added */
  isAdded?: boolean;
}

const QUALITY_COLORS: Record<VideoQuality, string> = {
  '4K': 'bg-purple-600',
  '1080p': 'bg-blue-600',
  '720p': 'bg-green-600',
  '480p': 'bg-yellow-600',
  Unknown: 'bg-gray-600',
};

export function TorrentCard({
  result,
  onAdd,
  isAdding = false,
  isAdded = false,
}: TorrentCardProps) {
  const handleAdd = () => {
    if (!isAdded && !isAdding) {
      onAdd?.(result.magnetUri);
    }
  };

  const isDisabled = !onAdd || isAdded || isAdding;
  const buttonText = isAdded ? 'Added' : 'Add';

  return (
    <article
      className="
        p-3 rounded-lg bg-bg-hover
        hover:bg-white/10
        transition-colors duration-200 motion-reduce:transition-none
      "
    >
      {/* Title Row */}
      <div className="flex items-start gap-2 mb-2">
        {/* Quality Badge */}
        <span
          className={`
            flex-shrink-0
            px-2 py-0.5 rounded text-xs font-medium text-white
            ${QUALITY_COLORS[result.quality]}
          `}
        >
          {result.quality}
        </span>

        {/* Title */}
        <h3
          className="flex-1 text-sm text-white line-clamp-2"
          title={result.title}
        >
          {result.title}
        </h3>
      </div>

      {/* Meta Row */}
      <div className="flex items-center justify-between gap-3">
        <div className="flex items-center gap-3 text-xs text-text-secondary">
          {/* Size */}
          <span>{formatFileSize(result.size)}</span>

          {/* Seeders */}
          <span className="text-green-500" title="Seeders">
            ↑ {result.seeders}
          </span>

          {/* Leechers */}
          <span className="text-yellow-500" title="Leechers">
            ↓ {result.leechers}
          </span>

          {/* Indexer */}
          <span className="hidden sm:inline text-text-muted">
            {result.indexer}
          </span>
        </div>

        {/* Add Button */}
        <Button
          variant={isAdded ? 'ghost' : 'secondary'}
          size="sm"
          onClick={handleAdd}
          aria-label={isAdded ? `${result.title} already added` : `Add ${result.title}`}
          disabled={isDisabled}
          isLoading={isAdding && !isAdded}
        >
          {isAdded && (
            <svg
              xmlns="http://www.w3.org/2000/svg"
              width="14"
              height="14"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth="2"
              strokeLinecap="round"
              strokeLinejoin="round"
              aria-hidden="true"
              className="mr-1"
            >
              <polyline points="20 6 9 17 4 12" />
            </svg>
          )}
          {buttonText}
        </Button>
      </div>
    </article>
  );
}

export function TorrentCardSkeleton() {
  return (
    <div className="p-3 rounded-lg bg-bg-hover animate-pulse motion-reduce:animate-none">
      <div className="flex items-start gap-2 mb-2">
        <div className="w-12 h-5 rounded bg-white/10" />
        <div className="flex-1 h-5 rounded bg-white/10" />
      </div>
      <div className="flex items-center justify-between">
        <div className="flex gap-3">
          <div className="w-16 h-4 rounded bg-white/10" />
          <div className="w-8 h-4 rounded bg-white/10" />
          <div className="w-8 h-4 rounded bg-white/10" />
        </div>
        <div className="w-14 h-9 rounded bg-white/10" />
      </div>
    </div>
  );
}
