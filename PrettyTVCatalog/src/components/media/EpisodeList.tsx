import type { Episode } from '@/types/tmdb';
import { EpisodeCard, EpisodeCardSkeleton, type EpisodeAssignmentInfo } from './EpisodeCard';

interface EpisodeListProps {
  episodes: Episode[];
  showName: string;
  seasonNumber: number;
  onSearchTorrents?: (query: string) => void;
  /** Map of episode number to assignment info */
  episodeAssignments?: Map<number, EpisodeAssignmentInfo>;
  /** Handler for unassigning torrent from episode */
  onUnassignEpisode?: (episodeId: number) => Promise<void>;
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
  episodeAssignments,
  onUnassignEpisode,
}: EpisodeListProps) {
  if (episodes.length === 0) {
    return (
      <p className="text-text-secondary py-8 text-center">
        No episodes available for this season.
      </p>
    );
  }

  // Count episodes with assignments for summary
  const assignedCount = episodeAssignments?.size || 0;

  return (
    <section aria-label="Episodes">
      <div className="flex items-center justify-between mb-4 sm:mb-6">
        <h2 className="text-xl sm:text-2xl font-semibold text-white">
          Episodes
        </h2>
        {assignedCount > 0 && (
          <span className="text-sm text-text-secondary">
            <span className="text-accent-green">{assignedCount}</span>
            {' / '}{episodes.length} assigned
          </span>
        )}
      </div>
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
              assignmentInfo={episodeAssignments?.get(episode.episodeNumber)}
              onUnassign={onUnassignEpisode}
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
