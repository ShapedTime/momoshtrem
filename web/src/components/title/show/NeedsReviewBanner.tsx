import { Banner, ButtonLink } from '@/components/ui';
import { plural } from '@/lib/title/format';

/** Files the matcher wasn't sure about (B2): one link to the mapping review (#5). Nothing when none need review. */
export function NeedsReviewBanner({ count, jobId }: { count: number; jobId: number | null }) {
  if (count <= 0) return null;
  return (
    <Banner
      tone="review"
      title={`${plural(count, 'file')} ${count === 1 ? 'needs' : 'need'} a quick review`}
      action={
        jobId !== null && (
          <ButtonLink href={`/mapping/${jobId}`} size="sm">
            Review mapping
          </ButtonLink>
        )
      }
    >
      {count === 1 ? 'It didn’t map cleanly to an episode.' : 'They didn’t map cleanly to episodes.'} Infuse shows them once they’re sorted.
    </Banner>
  );
}
