import { Banner, ButtonLink } from '@/components/ui';
import { plural } from '@/lib/title/format';

interface NeedsReviewBannerProps {
  count: number;
  jobId: number | null;
  /** A movie's job waits for one pick among its files. */
  movie?: boolean;
}

/**
 * The way back to an open mapping review (B2, #5): files the matcher wasn't
 * sure about, or (count 0) a job with nothing to decide that waits for Apply,
 * or for its AI check (`review_job_id` stays set, PLAN §6 "Review rules").
 * Nothing when nothing waits.
 */
export function NeedsReviewBanner({ count, jobId, movie = false }: NeedsReviewBannerProps) {
  const action = jobId !== null && (
    <ButtonLink href={`/mapping/${jobId}`} size="sm">
      Review mapping
    </ButtonLink>
  );
  if (count <= 0) {
    if (jobId === null) return null;
    return (
      <Banner tone="review" title="Finish this torrent’s mapping" action={action}>
        Open it to check and apply. Infuse shows {movie ? 'the movie' : 'the episodes'} once it’s applied.
      </Banner>
    );
  }
  if (movie && jobId === null) return null;
  if (movie) {
    return (
      <Banner tone="review" title="Pick the file to use" action={action}>
        More than one file in the torrent could be the movie. Infuse shows it once you pick one.
      </Banner>
    );
  }
  return (
    <Banner tone="review" title={`${plural(count, 'file')} ${count === 1 ? 'needs' : 'need'} a quick review`} action={action}>
      {count === 1 ? 'It didn’t map cleanly to an episode.' : 'They didn’t map cleanly to episodes.'} Infuse shows them once they’re sorted.
    </Banner>
  );
}
