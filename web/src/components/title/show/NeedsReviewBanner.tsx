import { Banner, ButtonLink } from '@/components/ui';
import { plural } from '@/lib/title/format';

interface NeedsReviewBannerProps {
  count: number;
  jobId: number | null;
  /** A movie's job waits for one pick among its files. */
  movie?: boolean;
}

/** Files the matcher wasn't sure about (B2): one link to the mapping review (#5). Nothing when none need review. */
export function NeedsReviewBanner({ count, jobId, movie = false }: NeedsReviewBannerProps) {
  if (count <= 0 || (movie && jobId === null)) return null;
  const action = jobId !== null && (
    <ButtonLink href={`/mapping/${jobId}`} size="sm">
      Review mapping
    </ButtonLink>
  );
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
