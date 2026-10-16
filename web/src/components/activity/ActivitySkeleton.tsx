import { CatEyes } from '@/components/brand';
import { Skeleton } from '@/components/ui';
import { LOADING_LINE } from '@/lib/activity/status';
import { cx } from '@/lib/utils/cx';

function TileSkeleton() {
  return (
    <div className="act-tile">
      <Skeleton shape="line" width="50%" />
      <Skeleton height={26} width="70%" className="rounded-sm" />
      <Skeleton shape="line" width="80%" />
    </div>
  );
}

function CardSkeleton() {
  return (
    <div className="tcard" aria-hidden="true">
      <Skeleton shape="poster" width={56} />
      <div className="flex flex-col gap-3">
        <Skeleton shape="line" width="60%" />
        <Skeleton shape="line" width="90%" />
        <Skeleton height={4} />
        <Skeleton shape="line" width="45%" />
      </div>
    </div>
  );
}

/** The strip and two sections in their final sizes (activity.js `loadingView`). */
export function ActivitySkeletonBody() {
  return (
    <>
      <section className="act-summary" aria-hidden="true">
        <div className="act-now is-quiet">
          <Skeleton width={44} height={44} className="rounded-md" />
          <div className="flex grow flex-col gap-2">
            <Skeleton shape="line" width="30%" />
            <Skeleton shape="line" width="65%" />
          </div>
        </div>
        <TileSkeleton />
        <TileSkeleton />
        <TileSkeleton />
        <TileSkeleton />
      </section>
      <div className="act-group" aria-hidden="true">
        <Skeleton shape="line" width={140} height={16} />
        <div className="act-list">
          <CardSkeleton />
          <CardSkeleton />
        </div>
      </div>
      <div className="act-group" aria-hidden="true">
        <Skeleton shape="line" width={110} height={16} />
        <div className="act-list">
          <CardSkeleton />
          <CardSkeleton />
          <CardSkeleton />
        </div>
      </div>
    </>
  );
}

/**
 * While the first answer is on its way: Noir's eyes and "Checking on your
 * torrents…", then the skeletons after 150 ms (`skel-delay`).
 */
export function ActivityLoading({ className }: { className?: string }) {
  return (
    <div className={cx('flex flex-col gap-8', className)} aria-busy="true">
      <p className="act-verdict text-fg-3" role="status">
        <span className="act-eyes" aria-hidden="true">
          <CatEyes width={30} />
        </span>
        <span>{LOADING_LINE}</span>
      </p>
      <div className="skel-delay flex flex-col gap-8">
        <ActivitySkeletonBody />
      </div>
    </div>
  );
}
