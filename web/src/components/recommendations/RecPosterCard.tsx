'use client';

import { libraryPill } from '@/components/library/LibraryPill';
import { PosterCard, StatusPill } from '@/components/ui';
import { buildImageUrl } from '@/config/tmdb';
import type { RecFeedbackSource, RecItem } from '@/lib/api/contract';
import { recCardSubtitle } from '@/lib/recommendations/cards';
import { titleHref } from '@/lib/title/href';
import { FeedbackRow } from './FeedbackRow';
import { useRecFeedback } from './useRecFeedback';

interface RecPosterCardProps {
  item: RecItem;
  source: RecFeedbackSource;
  /** A Coming soon label ("Jan 16", `comingSoonBadge`): top right on the poster, in place of the library pill. */
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
      // The design's Coming soon cards carry their date top right instead of the library pill (core.js posterCard)
      badge={comingSoon ? undefined : libraryPill(item.library)}
      badgeEnd={comingSoon ? <StatusPill status="upcoming" label={comingSoon} onArt /> : undefined}
      footer={<FeedbackRow feedback={feedback} inLibrary={item.library !== null} />}
      dismissed={feedback.dismissed}
    />
  );
}
