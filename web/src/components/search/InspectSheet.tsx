'use client';

import { useId } from 'react';
import { ArrowRight, Plus } from 'lucide-react';
import { Banner, Button, FilePath, ReleaseName, Sheet, Skeleton, Steps } from '@/components/ui';
import { type AssignmentJobDetail, isJobWorking, type Proposal, type Release } from '@/lib/api/contract';
import { displayPath, ignoredLabel, targetCode, targetName } from '@/lib/mapping/review';
import type { SearchProblem } from '@/lib/search/copy';
import { inspectJobProblem, inspectProblem, type InspectSection, inspectSections, inspectSteps, inspectTitle } from '@/lib/search/inspect';
import { STATUS } from '@/lib/status';

const ReviewIcon = STATUS.needs_review.icon;

/** One file: its path (without the torrent's folder), then where it goes, why it waits, or what it is. */
function FileRow({ proposal: p, kind, torrentName, movieTitle }: { proposal: Proposal; kind: InspectSection['kind']; torrentName: string; movieTitle: string }) {
  const path = <FilePath path={displayPath(p.file_path, torrentName)} />;
  if (kind === 'ignored') {
    return (
      <li className="is-ignored">
        {path}
        <span className="insp-to">{ignoredLabel(p)}</span>
      </li>
    );
  }
  if (kind === 'review' || kind === 'pick') {
    return (
      <li>
        {path}
        <span className="insp-to review">
          <ReviewIcon size={14} aria-hidden="true" className="shrink-0" />
          <span>
            {p.source === 'ai' && <b className="font-semibold">AI: </b>}
            {p.reason}
          </span>
        </span>
      </li>
    );
  }
  return (
    <li>
      {path}
      <span className="insp-to">
        <ArrowRight size={14} aria-hidden="true" className="shrink-0" />
        <span className="sr-only">maps to </span>
        {p.kind === 'movie' ? (
          <b>{movieTitle}</b>
        ) : (
          <>
            <b>{targetCode(p.targets)}</b> <span className="min-w-0">{targetName(p.targets)}</span>
          </>
        )}
      </span>
    </li>
  );
}

function Section({ section, torrentName, movieTitle }: { section: InspectSection; torrentName: string; movieTitle: string }) {
  const id = useId();
  return (
    <section className="flex flex-col gap-2">
      <h3 id={id} className="t-micro text-fg-3">
        {section.title}
      </h3>
      <ul className="insp-list" aria-labelledby={id}>
        {section.rows.map((p) => (
          <FileRow key={p.id} proposal={p} kind={section.kind} torrentName={torrentName} movieTitle={movieTitle} />
        ))}
        {section.more && <li className="insp-more">{section.more}</li>}
      </ul>
    </section>
  );
}

/** Rows to come while the job works. */
function RowSkeletons() {
  return (
    <ul className="insp-list" aria-hidden="true">
      {[78, 64, 84, 70].map((width, i) => (
        <li key={i}>
          <Skeleton shape="line" width={`${width}%`} />
          <Skeleton shape="line" width="42%" />
        </li>
      ))}
    </ul>
  );
}

function Problem({ problem, onRetry }: { problem: SearchProblem; onRetry: () => void }) {
  return (
    <Banner
      tone="error"
      role="alert"
      title={problem.title}
      action={
        problem.retry ? (
          <Button size="sm" onClick={onRetry}>
            Retry
          </Button>
        ) : undefined
      }
    >
      {problem.body}
    </Banner>
  );
}

interface InspectSheetProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Kept while the sheet closes. */
  release: Release | null;
  /** POST /search/torrents/inspect is on its way. */
  starting: boolean;
  /** It failed (an expired release, or resolve's 502 / 504). */
  startError: Error | null;
  /** The dry run, polled while it works. */
  detail: AssignmentJobDetail | undefined;
  /** Reading the job failed. */
  readError: Error | null;
  /** A new inspect. */
  onRetry: () => void;
  /** "Add & review mapping" (shows) or "Add to library & download" (movies). */
  addLabel: string;
  adding: boolean;
  onAdd: () => void;
}

/**
 * Inspect (HANDOFF §5.2; PLAN §6 "Search"): a dry run of the mapping for one
 * release, nested over the search, before anything downloads. The steps, then
 * the files as they'd land: what will map, what you'd review, specials, and
 * what stays out. Read-only: the foot adds the release, as the card's Add does.
 */
export function InspectSheet({ open, onOpenChange, release, starting, startError, detail, readError, onRetry, addLabel, adding, onAdd }: InspectSheetProps) {
  if (!release) return null;
  const job = detail?.job;
  const failed = job && (job.status === 'failed' || job.status === 'cancelled') ? inspectJobProblem(job) : null;
  const problem = startError ? inspectProblem(startError) : readError ? inspectProblem(readError) : failed;
  const working = starting || !job || isJobWorking(job.status);
  return (
    <Sheet
      open={open}
      onOpenChange={onOpenChange}
      nested
      className="ts-inspect"
      kicker="Inspect · nothing downloads yet"
      title={inspectTitle(job)}
      closeLabel="Back to results"
      bodyLabel="Files in this torrent"
      footer={
        <>
          <Button onClick={() => onOpenChange(false)}>Back</Button>
          <Button variant="primary" className="grow" loading={adding} onClick={onAdd}>
            <Plus size={18} aria-hidden="true" />
            {addLabel}
          </Button>
        </>
      }
    >
      <div className="flex flex-col gap-4">
        <ReleaseName name={release.raw_title} />
        {problem ? (
          <Problem problem={problem} onRetry={onRetry} />
        ) : (
          <>
            <div aria-live="polite">
              <Steps inline steps={inspectSteps(starting ? undefined : job)} />
            </div>
            {working || !detail ? (
              <RowSkeletons />
            ) : (
              <div className="flex flex-col gap-4">
                {inspectSections(detail.proposals).map((section) => (
                  <Section key={section.kind} section={section} torrentName={detail.job.torrent.name} movieTitle={detail.job.item.title} />
                ))}
              </div>
            )}
          </>
        )}
      </div>
    </Sheet>
  );
}
