'use client';

import { useEffect, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { useTaskPage } from '@/components/shell/ShellContext';
import { NotFoundView } from '@/components/system/NotFoundView';
import { Banner, Button, ButtonLink, EmptyState, LoadingRegion, Skeleton } from '@/components/ui';
import { useMappingActions } from '@/hooks/useMappingJob';
import type { AssignmentJobDetail } from '@/lib/api/contract';
import { isApiError } from '@/lib/api/client';
import { assignmentJobQuery } from '@/lib/api/endpoints';
import { nextStage, shortTitle, titleHref, type ViewStage } from '@/lib/mapping/review';
import { MappingApplied } from './MappingApplied';
import { MappingHeader, MappingMoreMenu } from './MappingHeader';
import { MappingProgress } from './MappingProgress';
import { ReviewScreen } from './ReviewScreen';

/** How long "Done. Here's the plan." shows before the review. */
const DONE_MS = 1200;

function MappingSkeleton() {
  return (
    <LoadingRegion label="Loading the mapping">
      <div className="map-skel-head page">
        <Skeleton shape="line" width={96} />
        <Skeleton shape="line" width="40%" />
        <Skeleton height={40} width="min(520px, 100%)" />
        <Skeleton height={44} width="min(720px, 100%)" />
        <Skeleton shape="line" width={220} />
      </div>
      <div className="page flex flex-col gap-4" aria-hidden="true">
        {[180, 120, 260].map((height, i) => (
          <Skeleton key={i} height={height} className="rounded-lg" />
        ))}
      </div>
    </LoadingRegion>
  );
}

function LoadError({ message, onRetry }: { message: string; onRetry: () => void }) {
  return (
    <div className="page map-skel-head">
      <Banner
        tone="error"
        role="alert"
        title="Couldn’t load this mapping"
        action={
          <Button size="sm" onClick={onRetry}>
            Try again
          </Button>
        }
      >
        {message}
      </Banner>
    </div>
  );
}

function OpenJobMenu({ detail }: { detail: AssignmentJobDetail }) {
  const actions = useMappingActions(detail.job);
  return <MappingMoreMenu onDiscard={() => void actions.discard()} />;
}

/** A job that failed: why, and the way back. */
function FailedJob({ detail }: { detail: AssignmentJobDetail }) {
  const { job } = detail;
  return (
    <>
      <MappingHeader job={job} />
      <div className="page map-state">
        <Banner
          tone="error"
          role="alert"
          title="Mapping failed"
          action={
            <ButtonLink href={titleHref(job)} size="sm">
              Back to {shortTitle(job.item.title)}
            </ButtonLink>
          }
        >
          {job.error ?? 'Something went wrong while matching the files.'} Nothing was added to Infuse.
        </Banner>
      </div>
    </>
  );
}

/** A job discarded earlier. */
function CancelledJob({ detail }: { detail: AssignmentJobDetail }) {
  const { job } = detail;
  return (
    <div className="page">
      <EmptyState
        cat="sleep"
        headingLevel="h1"
        title="This mapping was discarded"
        actions={
          <ButtonLink href={titleHref(job)} variant="secondary">
            Back to {shortTitle(job.item.title)}
          </ButtonLink>
        }
      >
        Nothing from {job.torrent.name} was added to Infuse.
      </EmptyState>
    </div>
  );
}

/**
 * /mapping/[id] (PLAN F4): a mapping job from start to finish. Named steps
 * while the worker maps the files, "Done. Here's the plan.", the review, and
 * "Ready in Infuse." once applied; failed, discarded and missing jobs say so.
 */
export function MappingReview({ jobId }: { jobId: number }) {
  useTaskPage();
  const query = useQuery(assignmentJobQuery(jobId));
  const detail = query.data;
  const status = detail?.job.status;

  // The first run's progress, then a beat of "Done", then the plan. A re-check keeps the review.
  const [stage, setStage] = useState<ViewStage | null>(null);
  const next = nextStage(stage, status);
  if (next !== stage) setStage(next);
  useEffect(() => {
    if (stage !== 'done') return;
    const timer = window.setTimeout(() => setStage('settled'), DONE_MS);
    return () => window.clearTimeout(timer);
  }, [stage]);

  // Applied: the result starts at the top
  useEffect(() => {
    if (status === 'applied') window.scrollTo(0, 0);
  }, [status]);

  if (!detail) {
    if (query.isPending) return <MappingSkeleton />;
    if (isApiError(query.error) && query.error.status === 404) return <NotFoundView />;
    return <LoadError message={query.error?.message ?? 'Something went wrong.'} onRetry={() => void query.refetch()} />;
  }

  const { job } = detail;
  let body;
  if (next === 'progress' || next === 'done') {
    body = (
      <>
        <MappingHeader job={job} more={next === 'progress' ? <OpenJobMenu detail={detail} /> : undefined} />
        <MappingProgress job={job} done={next === 'done'} />
      </>
    );
  } else if (job.status === 'applied') body = <MappingApplied detail={detail} />;
  else if (job.status === 'failed') body = <FailedJob detail={detail} />;
  else if (job.status === 'cancelled') body = <CancelledJob detail={detail} />;
  else body = <ReviewScreen detail={detail} />;

  return <div className="map-page">{body}</div>;
}
