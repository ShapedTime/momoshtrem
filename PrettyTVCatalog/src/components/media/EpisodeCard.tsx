import Image from 'next/image';
import { buildImageUrl } from '@/config/tmdb';
import { formatRuntime } from '@/lib/utils';
import { Button, FilmIcon, StarIcon } from '@/components/ui';
import type { Episode } from '@/types/tmdb';

interface EpisodeCardProps {
  episode: Episode;
  showName: string;
  seasonNumber: number;
  onSearchTorrents?: (query: string) => void;
}

/**
 * Format season and episode numbers into standard code (S01E05).
 */
function formatEpisodeCode(season: number, episode: number): string {
  return `S${season.toString().padStart(2, '0')}E${episode.toString().padStart(2, '0')}`;
}

/**
 * Horizontal episode card with 16:9 still image.
 * Shows episode metadata and "Search Torrents" button.
 */
export function EpisodeCard({
  episode,
  showName,
  seasonNumber,
  onSearchTorrents,
}: EpisodeCardProps) {
  const stillUrl = buildImageUrl(episode.stillPath, 'still', 'large');
  const episodeCode = formatEpisodeCode(seasonNumber, episode.episodeNumber);
  const runtime = formatRuntime(episode.runtime);
  const searchQuery = `${showName} ${episodeCode}`;

  const handleSearchClick = () => {
    onSearchTorrents?.(searchQuery);
  };

  return (
    <article
      className="
        flex flex-col sm:flex-row gap-4
        p-4 rounded-lg
        bg-bg-elevated
        hover:bg-bg-hover
        transition-colors duration-200 motion-reduce:transition-none
      "
    >
      {/* Episode Still Image - 16:9 aspect ratio */}
      <div className="flex-shrink-0 w-full sm:w-[200px] lg:w-[260px]">
        <div className="relative aspect-video rounded-md overflow-hidden bg-bg-hover">
          {stillUrl ? (
            <Image
              src={stillUrl}
              alt={episode.name}
              fill
              sizes="(max-width: 640px) 100vw, 260px"
              className="object-cover"
            />
          ) : (
            <div className="absolute inset-0 flex items-center justify-center text-text-muted">
              <FilmIcon size={32} />
            </div>
          )}
          {/* Episode number overlay */}
          <div className="absolute top-2 left-2 px-2 py-1 bg-black/70 rounded text-xs font-medium text-white">
            {episodeCode}
          </div>
        </div>
      </div>

      {/* Episode Info */}
      <div className="flex-1 min-w-0">
        <div className="flex items-start justify-between gap-4">
          <div className="min-w-0">
            {/* Episode Title */}
            <h3 className="text-lg font-semibold text-white line-clamp-1">
              {episode.name}
            </h3>

            {/* Meta info row */}
            <div className="flex flex-wrap items-center gap-3 mt-1 text-sm text-text-secondary">
              {episode.airDate && (
                <span>{new Date(episode.airDate).toLocaleDateString()}</span>
              )}
              {runtime && <span>{runtime}</span>}
              {episode.voteAverage > 0 && (
                <div className="flex items-center gap-1 text-accent-yellow">
                  <StarIcon size={14} />
                  <span className="font-medium">{episode.voteAverage.toFixed(1)}</span>
                </div>
              )}
            </div>
          </div>

          {/* Search Torrents Button - Desktop */}
          <div className="hidden sm:block flex-shrink-0">
            <Button
              variant="secondary"
              size="sm"
              onClick={handleSearchClick}
              aria-label={`Search torrents for ${showName} ${episodeCode}`}
            >
              Search Torrents
            </Button>
          </div>
        </div>

        {/* Overview */}
        {episode.overview && (
          <p className="mt-2 text-sm text-text-secondary line-clamp-2 sm:line-clamp-3">
            {episode.overview}
          </p>
        )}

        {/* Search Torrents Button - Mobile */}
        <div className="sm:hidden mt-3">
          <Button
            variant="secondary"
            size="sm"
            onClick={handleSearchClick}
            className="w-full"
            aria-label={`Search torrents for ${showName} ${episodeCode}`}
          >
            Search Torrents
          </Button>
        </div>
      </div>
    </article>
  );
}

export function EpisodeCardSkeleton() {
  return (
    <div className="flex flex-col sm:flex-row gap-4 p-4 rounded-lg bg-bg-elevated">
      {/* Still skeleton */}
      <div className="flex-shrink-0 w-full sm:w-[200px] lg:w-[260px]">
        <div className="aspect-video rounded-md bg-bg-hover animate-pulse motion-reduce:animate-none" />
      </div>
      {/* Info skeleton */}
      <div className="flex-1 min-w-0">
        <div className="h-6 w-3/4 bg-bg-hover rounded animate-pulse motion-reduce:animate-none" />
        <div className="h-4 w-1/3 bg-bg-hover rounded mt-2 animate-pulse motion-reduce:animate-none" />
        <div className="space-y-2 mt-3">
          <div className="h-4 w-full bg-bg-hover rounded animate-pulse motion-reduce:animate-none" />
          <div className="h-4 w-2/3 bg-bg-hover rounded animate-pulse motion-reduce:animate-none" />
        </div>
      </div>
    </div>
  );
}
