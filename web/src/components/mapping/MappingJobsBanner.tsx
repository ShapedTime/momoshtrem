'use client';

import { useId } from 'react';
import { useQuery } from '@tanstack/react-query';
import { CircleAlert } from 'lucide-react';
import { Banner, ButtonLink, ReleaseName } from '@/components/ui';
import type { AssignmentJob } from '@/lib/api/contract';
import { assignmentJobsQuery } from '@/lib/api/endpoints';
import { openJobLine, shortTitle } from '@/lib/mapping/review';

const reviewHref = (job: AssignmentJob) => `/mapping/${job.id}`;

/**
 * Open mapping jobs on Activity (#5): one
 * review-tone banner, or a compact list when there are several, each going
 * to its review. Nothing while there are none (or the list can't load).
 */
export function MappingJobsBanner() {
  const headingId = useId();
  const { data } = useQuery(assignmentJobsQuery('open'));
  const jobs = data?.items ?? [];
  if (jobs.length === 0) return null;

  if (jobs.length === 1) {
    const [job] = jobs;
    return (
      <Banner
        tone="review"
        title={openJobLine(job)}
        action={
          <ButtonLink href={reviewHref(job)} size="sm">
            Review mapping
          </ButtonLink>
        }
      >
        <ReleaseName as="span" name={job.torrent.name} className="t-small" />
      </Banner>
    );
  }

  return (
    <section className="banner banner--review" aria-labelledby={headingId}>
      <CircleAlert size={20} aria-hidden="true" />
      <div className="grow">
        <strong id={headingId}>Episode mappings to check</strong>
        <ul className="flex flex-col divide-y divide-line-1">
          {jobs.map((job) => (
            <li key={job.id} className="flex flex-wrap items-center justify-between gap-x-3 gap-y-1 py-2 last:pb-0">
              <div className="flex min-w-0 flex-col">
                <span className="font-medium text-fg-1">{openJobLine(job)}</span>
                <ReleaseName as="span" name={job.torrent.name} className="t-small text-fg-2" />
              </div>
              <ButtonLink href={reviewHref(job)} size="sm" aria-label={`Review mapping for ${shortTitle(job.item.title)}`}>
                Review mapping
              </ButtonLink>
            </li>
          ))}
        </ul>
      </div>
    </section>
  );
}
