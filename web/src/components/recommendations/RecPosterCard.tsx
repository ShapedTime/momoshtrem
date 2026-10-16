'use client';

import { libraryPill } from '@/components/library/LibraryPill';
import { PosterCard, StatusPill } from '@/components/ui';
import { buildImageUrl } from '@/config/tmdb';
import type { RecFeedbackSource, RecItem } from '@/lib/api/contract';
import { recCardSubtitle } from '@/lib/for-you/shelves';
import { titleHref } from '@/lib/title/href';
import { FeedbackRow } from './FeedbackRow';
import { useRecFeedback } from './useRecFeedback';

interface RecPosterCardProps {
  item: RecItem;
  source: RecFeedbackSource;
  /** A Coming soon label ("Jan 16"): it takes the library pill's place on the poster. */
  comingSoon?: string | null;
}

/**
 * A recommended title on a shelf (HANDOFF's PosterCard "with reason +
 * feedback"): its library pill, the reason in two lines, and the feedback
 * row. Dislike, Not interested and Already seen dim it at once.
 */
export function RecPosterCard({ item, source, comingSoon = null }: RecPosterCardProps) {
  const feedback = useRecFeedback(item, source);
  return (
    <PosterCard
      href={titleHref(item.media_type, item.tmdb_id)}
      title={item.title}
      subtitle={recCardSubtitle(item, comingSoon)}
      reason={item.reason}
      posterSrc={buildImageUrl(item.poster_path, 'poster', 'medium')}
      badge={comingSoon ? <StatusPill status="upcoming" label={comingSoon} onArt /> : libraryPill(item.library)}
      footer={<FeedbackRow feedback={feedback} inLibrary={item.library !== null} />}
      dismissed={feedback.dismissed}
    />
  );
}
