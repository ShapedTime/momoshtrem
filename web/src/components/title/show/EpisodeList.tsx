import type { EpisodeOrdering, EpisodeSummary } from '@/lib/api/contract';
import { EpisodeRow, type EpisodeRowProps } from './EpisodeRow';

interface EpisodeListProps extends Pick<EpisodeRowProps, 'onFind' | 'onOpen'> {
  id: string;
  /** Names the list, e.g. "Season 1 episodes". */
  label: string;
  episodes: EpisodeSummary[];
  ordering: EpisodeOrdering;
  nextUpId: number | null;
  reviewHref: string | null;
}

/**
 * A season's episodes as compact rows. Long seasons (slice 6) swap the
 * plain list for a window virtualiser here; rows keep `episodeRowId` ids so
 * jump-to and range chips can find them.
 */
export function EpisodeList({ id, label, episodes, ordering, nextUpId, reviewHref, onFind, onOpen }: EpisodeListProps) {
  return (
    <ol id={id} className="ep-list" aria-label={label}>
      {episodes.map((episode) => (
        <EpisodeRow
          key={episode.id}
          episode={episode}
          ordering={ordering}
          isNext={episode.id === nextUpId}
          reviewHref={reviewHref}
          onFind={onFind}
          onOpen={onOpen}
        />
      ))}
    </ol>
  );
}
