import type { Episode } from '@/types/tmdb';
import { EpisodeCard, EpisodeCardSkeleton } from './EpisodeCard';

interface EpisodeListProps {
  episodes: Episode[];
  showName: string;
  seasonNumber: number;
  onSearchTorrents?: (query: string) => void;
}

/**
 * Container for episode cards.
 * Displays episodes sorted by episode number.
 */
export function EpisodeList({
  episodes,
  showName,
  seasonNumber,
  onSearchTorrents,
}: EpisodeListProps) {
  if (episodes.length === 0) {
    return (
      <p className="text-text-secondary py-8 text-center">
        No episodes available for this season.
      </p>
    );
  }

  return (
    <section aria-label="Episodes">
      <h2 className="text-xl sm:text-2xl font-semibold text-white mb-4 sm:mb-6">
        Episodes
      </h2>
      <div className="space-y-4">
        {episodes
          .sort((a, b) => a.episodeNumber - b.episodeNumber)
          .map((episode) => (
            <EpisodeCard
              key={episode.id}
              episode={episode}
              showName={showName}
              seasonNumber={seasonNumber}
              onSearchTorrents={onSearchTorrents}
            />
          ))}
      </div>
    </section>
  );
}

interface EpisodeListSkeletonProps {
  count?: number;
}

export function EpisodeListSkeleton({ count = 5 }: EpisodeListSkeletonProps) {
  return (
    <section aria-label="Loading episodes">
      <div className="h-7 sm:h-8 w-24 bg-bg-hover rounded mb-4 sm:mb-6 animate-pulse motion-reduce:animate-none" />
      <div className="space-y-4">
        {Array.from({ length: count }).map((_, index) => (
          <EpisodeCardSkeleton key={index} />
        ))}
      </div>
    </section>
  );
}
